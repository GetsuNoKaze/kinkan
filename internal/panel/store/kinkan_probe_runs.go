package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// Kinkan: the history of protocol checks (Nodes → the check). A run is kept with the
// time it started, for ProbeRunKeep, and at most ProbeRunsPerNode per node.
const (
	ProbeRunKeep     = 30 * 24 * time.Hour
	ProbeRunsPerNode = 100
	// ProbeRunMaxBytes bounds one stored report; a larger one is not kept.
	ProbeRunMaxBytes = 2 << 20
)

// ProbeRun is one stored check without its report.
type ProbeRun struct {
	ID        int64
	NodeID    int64
	StartedAt int64
	InboundID int64
	Verdict   string
	Inbounds  int
}

// ErrProbeRunTooLarge: the report is over ProbeRunMaxBytes.
var ErrProbeRunTooLarge = errors.New("probe report too large to keep")

// AddProbeRun keeps a check's report and drops the node's runs beyond ProbeRunsPerNode.
func (s *Store) AddProbeRun(ctx context.Context, run ProbeRun, report any) (int64, error) {
	raw, err := json.Marshal(report)
	if err != nil {
		return 0, err
	}
	if len(raw) > ProbeRunMaxBytes {
		return 0, ErrProbeRunTooLarge
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var id int64
	if err := tx.QueryRowContext(ctx, `INSERT INTO kinkan_probe_runs(node_id,started_at,inbound_id,verdict,inbounds,report) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`,
		run.NodeID, run.StartedAt, run.InboundID, run.Verdict, run.Inbounds, string(raw)).Scan(&id); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM kinkan_probe_runs WHERE node_id=$1 AND id IN
 (SELECT id FROM kinkan_probe_runs WHERE node_id=$1 ORDER BY started_at DESC, id DESC OFFSET $2)`, run.NodeID, ProbeRunsPerNode); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

// ProbeRuns lists a node's kept checks, newest first.
func (s *Store) ProbeRuns(ctx context.Context, nodeID int64) ([]ProbeRun, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,node_id,started_at,inbound_id,verdict,inbounds FROM kinkan_probe_runs WHERE node_id=$1 ORDER BY started_at DESC, id DESC`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProbeRun{}
	for rows.Next() {
		var r ProbeRun
		if err := rows.Scan(&r.ID, &r.NodeID, &r.StartedAt, &r.InboundID, &r.Verdict, &r.Inbounds); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ProbeRunReport is a kept check's report; sql.ErrNoRows when there is none.
func (s *Store) ProbeRunReport(ctx context.Context, id int64) (ProbeRun, []byte, error) {
	var r ProbeRun
	var raw []byte
	err := s.DB.QueryRowContext(ctx, `SELECT id,node_id,started_at,inbound_id,verdict,inbounds,report FROM kinkan_probe_runs WHERE id=$1`, id).
		Scan(&r.ID, &r.NodeID, &r.StartedAt, &r.InboundID, &r.Verdict, &r.Inbounds, &raw)
	return r, raw, err
}

// DeleteProbeRun removes one kept check; sql.ErrNoRows when there is none.
func (s *Store) DeleteProbeRun(ctx context.Context, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM kinkan_probe_runs WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteProbeRuns removes all of a node's kept checks and says how many.
func (s *Store) DeleteProbeRuns(ctx context.Context, nodeID int64) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM kinkan_probe_runs WHERE node_id=$1`, nodeID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// PruneProbeRuns drops the checks older than ProbeRunKeep.
func (s *Store) PruneProbeRuns(ctx context.Context, now time.Time) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM kinkan_probe_runs WHERE started_at<$1`, now.Add(-ProbeRunKeep).Unix())
	return err
}

// DeleteScanners removes a node's scanner journal from the panel and zeroes the dropped
// count of its collection status; the node's own journal is reset by the caller first.
func (s *Store) DeleteScanners(ctx context.Context, nodeID int64) error {
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM kinkan_scanners WHERE node_id=$1`, nodeID); err != nil {
		return err
	}
	st, err := s.ScannerStatusOf(ctx, nodeID)
	if err != nil {
		return err
	}
	st.Dropped = 0
	return s.SetScannerStatus(ctx, nodeID, st)
}
