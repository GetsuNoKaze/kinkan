package store

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"errors"
	"testing"

	"mikan/internal/site"
)

func testSite(t *testing.T, title string) *site.Site {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, data := range map[string]string{
		"index.html": "<title>" + title + "</title>", "404.html": "x", "robots.txt": "x", "favicon.ico": "x",
	} {
		f, _ := w.Create(name)
		_, _ = f.Write([]byte(data))
	}
	_ = w.Close()
	s, err := site.Unpack(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// The fork's migrations run on their own goose table, after Mikan's, and again as a no-op.
func TestKinkanMigrations(t *testing.T) {
	ctx := context.Background()
	s, err := OpenTest(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var version int64
	if err := s.DB.QueryRowContext(ctx, `SELECT max(version_id) FROM `+kinkanTable).Scan(&version); err != nil || version < 1 {
		t.Fatalf("kinkan version = %d, %v", version, err)
	}
	if err := migrateKinkan(ctx, s.DB); err != nil {
		t.Fatalf("second run: %v", err)
	}
	// Mikan's own version table knows nothing of the fork's migrations.
	var mikan int64
	if err := s.DB.QueryRowContext(ctx, `SELECT max(version_id) FROM goose_db_version`).Scan(&mikan); err != nil {
		t.Fatal(err)
	}
	if mikan >= 1000 {
		t.Errorf("a fork migration landed in Mikan's sequence: %d", mikan)
	}
}

func TestKinkanSites(t *testing.T) {
	ctx := context.Background()
	s, err := OpenTest(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var node int64
	if err := s.DB.QueryRowContext(ctx, `INSERT INTO nodes (created_at, updated_at) VALUES (1, 1) RETURNING id`).Scan(&node); err != nil {
		t.Fatal(err)
	}

	a, found, err := s.AddSite(ctx, "night", testSite(t, "Night Archive"), 100)
	if err != nil || found {
		t.Fatalf("add: %+v %v %v", a, found, err)
	}
	if a.Title != "Night Archive" || a.Files != 4 || len(a.Hash) != 64 || len(a.Nodes) != 0 {
		t.Errorf("site = %+v", a)
	}
	// The same content again is the site already kept, under its first name.
	again, found, err := s.AddSite(ctx, "other name", testSite(t, "Night Archive"), 200)
	if err != nil || !found || again.ID != a.ID || again.Name != "night" {
		t.Errorf("same content: %+v %v %v", again, found, err)
	}
	b, _, err := s.AddSite(ctx, "day", testSite(t, "Day Archive"), 300)
	if err != nil {
		t.Fatal(err)
	}

	hash, archive, err := s.SiteArchive(ctx, a.ID)
	if err != nil || hash != a.Hash {
		t.Fatalf("archive: %s %v", hash, err)
	}
	if back, err := site.Unpack(archive); err != nil || back.Hash != a.Hash {
		t.Errorf("kept archive does not unpack to the site: %v", err)
	}

	if err := s.SetNodeSite(ctx, node, a.ID); err != nil {
		t.Fatal(err)
	}
	if id, h, ok, err := s.NodeSite(ctx, node); err != nil || !ok || id != a.ID || h != a.Hash {
		t.Errorf("node site: %d %s %v %v", id, h, ok, err)
	}
	if err := s.SetNodeSite(ctx, node, b.ID); err != nil {
		t.Fatal(err)
	}
	sites, err := s.Sites(ctx)
	if err != nil || len(sites) != 2 || sites[0].ID != b.ID || len(sites[0].Nodes) != 1 || sites[0].Nodes[0] != node || len(sites[1].Nodes) != 0 {
		t.Errorf("sites: %+v %v", sites, err)
	}
	if err := s.DeleteSite(ctx, b.ID); !errors.Is(err, ErrSiteInUse) {
		t.Errorf("deleting a shown site: %v", err)
	}
	if err := s.SetNodeSite(ctx, node, 999999); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("an unknown site: %v", err)
	}
	if err := s.SetNodeSite(ctx, 999999, a.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("an unknown node: %v", err)
	}
	if err := s.SetNodeSite(ctx, node, 0); err != nil {
		t.Fatal(err)
	}
	if _, _, ok, err := s.NodeSite(ctx, node); err != nil || ok {
		t.Errorf("node shows a site after clearing: %v %v", ok, err)
	}
	if err := s.DeleteSite(ctx, b.ID); err != nil {
		t.Errorf("deleting a site no node shows: %v", err)
	}
	if err := s.DeleteSite(ctx, b.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("deleting it again: %v", err)
	}

	// Deleting the node forgets what it showed, and the site can go.
	if err := s.SetNodeSite(ctx, node, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM nodes WHERE id = $1`, node); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteSite(ctx, a.ID); err != nil {
		t.Errorf("deleting the site of a deleted node: %v", err)
	}
}
