package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"

	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

// Kinkan's own migrations live apart from Mikan's: their own directory and goose version
// table, applied after Mikan's. Mikan's provider allows no out-of-order versions, so a
// fork migration numbered into Mikan's sequence would stop the panel once upstream adds
// its next one; apart, the two never meet, and a rollback to Mikan, which does not know
// this table, leaves the fork's tables alone.
//
//go:embed kinkan/*.sql
var kinkanMigrations embed.FS

// kinkanTable is goose's version table for the fork's migrations.
const kinkanTable = "kinkan_db_version"

func kinkanFS() fs.FS {
	fsys, err := fs.Sub(kinkanMigrations, "kinkan")
	if err != nil {
		panic(err) // a fixed embedded directory
	}
	return fsys
}

// migrateKinkan applies the fork's pending migrations, under a lock of their own.
func migrateKinkan(ctx context.Context, conn *sql.DB) error {
	_, lockID, err := schemaLock(ctx, conn, "kinkan-goose")
	if err != nil {
		return err
	}
	locker, err := lock.NewPostgresSessionLocker(lock.WithLockID(lockID))
	if err != nil {
		return err
	}
	p, err := goose.NewProvider(goose.DialectPostgres, conn, kinkanFS(), goose.WithSessionLocker(locker), goose.WithTableName(kinkanTable))
	if err != nil {
		return fmt.Errorf("Kinkan migrations: %w", err)
	}
	current, err := p.GetDBVersion(ctx)
	if err != nil {
		return fmt.Errorf("Kinkan migration versions: %w", err)
	}
	sources := p.ListSources()
	if target := sources[len(sources)-1].Version; current > target {
		return fmt.Errorf("Kinkan schema %d is newer than this binary supports (%d); downgrade refused", current, target)
	}
	if _, err := p.Up(ctx); err != nil {
		return fmt.Errorf("Kinkan migrations: %w", err)
	}
	return nil
}

// KinkanSchemaVersion is the fork's applied schema version: 0 before its first migration
// (no version table yet). The installer compares it before and after a migration, since
// an older Kinkan refuses a newer fork schema just as an older mikan refuses a newer one.
func KinkanSchemaVersion(ctx context.Context, dsn string) (int64, error) {
	conn, err := connectPostgres(ctx, dsn, maintenanceSession)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	var exists bool
	if err := conn.QueryRowContext(ctx, `SELECT to_regclass(current_schema() || '.`+kinkanTable+`') IS NOT NULL`).Scan(&exists); err != nil || !exists {
		return 0, err
	}
	var v sql.NullInt64
	err = conn.QueryRowContext(ctx, `SELECT max(version_id) FROM `+kinkanTable+` WHERE is_applied`).Scan(&v)
	return v.Int64, err
}
