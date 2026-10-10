package store

import (
	"context"
	"mikan/internal/nodeapi"
	"strings"
	"testing"
	"time"
)

func TestScannerSnapshotsAreIdempotentAndExpire(t *testing.T) {
	ctx := context.Background()
	s, err := OpenTest(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var node int64
	if err := s.DB.QueryRowContext(ctx, `INSERT INTO nodes(created_at,updated_at) VALUES(1,1) RETURNING id`).Scan(&node); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	r := nodeapi.ScannerRecord{Day: now.UTC().Truncate(24 * time.Hour).Unix(), IP: "192.0.2.1", Inbound: "tt", Protocol: "trusttunnel", Reason: "no_credentials", Count: 3, First: now.Unix(), Last: now.Unix(), Scanner: "unknown"}
	snap := nodeapi.ScannerSnapshot{Epoch: strings.Repeat("a", 32), Rows: []nodeapi.ScannerRecord{r}}
	for i := 0; i < 2; i++ {
		if err := s.SaveScanners(ctx, node, snap, now); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := s.ScannerRows(ctx, node, now)
	if err != nil || len(rows) != 1 || rows[0].Count != 3 {
		t.Fatal(rows, err)
	}
	snap.Rows[0].Count = 2
	if err := s.SaveScanners(ctx, node, snap, now); err != nil {
		t.Fatal(err)
	}
	rows, _ = s.ScannerRows(ctx, node, now)
	if rows[0].Count != 3 {
		t.Fatal("stale snapshot reduced count")
	}
	snap.Epoch = strings.Repeat("b", 32)
	if err := s.SaveScanners(ctx, node, snap, now); err != nil {
		t.Fatal(err)
	}
	rows, _ = s.ScannerRows(ctx, node, now)
	if rows[0].Count != 5 {
		t.Fatal("new epoch lost", rows)
	}
	if err := s.SaveScanners(ctx, node, snap, now.Add(32*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	rows, _ = s.ScannerRows(ctx, node, now.Add(32*24*time.Hour))
	if len(rows) != 0 {
		t.Fatal("retention failed")
	}
}
