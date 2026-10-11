package nodesync

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"mikan/internal/nodeapi"
	"mikan/internal/panel/store"
)

type scannerTestNode struct {
	*fakeNode
	snapshot nodeapi.ScannerSnapshot
	err      error
	resetErr error
	captured chan struct{}
	release  chan struct{}
}

func (n *scannerTestNode) Scanners(ctx context.Context) (nodeapi.ScannerSnapshot, error) {
	snap := n.snapshot
	if n.captured != nil {
		close(n.captured)
		select {
		case <-n.release:
		case <-ctx.Done():
			return snap, ctx.Err()
		}
	}
	return snap, n.err
}

func (n *scannerTestNode) ResetScanners(context.Context) error {
	if n.resetErr != nil {
		return n.resetErr
	}
	n.snapshot = nodeapi.ScannerSnapshot{Epoch: strings.Repeat("b", 32), Rows: []nodeapi.ScannerRecord{}}
	return nil
}

func scannerTestSnapshot(now time.Time) nodeapi.ScannerSnapshot {
	return nodeapi.ScannerSnapshot{Epoch: strings.Repeat("a", 32), Dropped: 7, Rows: []nodeapi.ScannerRecord{{
		Day: now.UTC().Truncate(24 * time.Hour).Unix(), IP: "198.51.100.9", Inbound: "tt",
		Protocol: "trusttunnel", Reason: "no_credentials", Count: 3,
		First: now.Unix(), Last: now.Unix(), Scanner: "unknown",
	}}}
}

// Hold a fetched snapshot until a concurrent clear has started. Clearing must wait
// for it to be stored, remove it, and allow subsequent new-epoch records to be kept.
func TestClearScannerJournalWaitsForCollection(t *testing.T) {
	s, f, st, _, now := setup(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	n := &scannerTestNode{fakeNode: f, snapshot: scannerTestSnapshot(*now), captured: make(chan struct{}), release: make(chan struct{})}
	s.node = n
	done := make(chan struct{})
	old := s
	go func() { old.pullScanners(ctx); close(done) }()
	select {
	case <-n.captured:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	// Reconnecting replaces the syncer, but it must retain the same journal lock.
	s = s.m.attach(t, s.id, Target{Node: n, TLS: fakeTLS, Local: true})
	cleared := make(chan error, 1)
	go func() { cleared <- s.m.ClearScanners(ctx, s.id) }()
	early := false
	select {
	case err := <-cleared:
		early = true
		t.Errorf("clear finished while an old snapshot was in flight: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(n.release)
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if !early {
		select {
		case err := <-cleared:
			if err != nil {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	n.captured = nil
	s.pullScanners(ctx)
	rows, err := st.ScannerRows(ctx, s.id, *now)
	if err != nil || len(rows) != 0 {
		t.Fatalf("cleared records came back: %+v %v", rows, err)
	}
	status, err := st.ScannerStatusOf(ctx, s.id)
	if err != nil || status.Dropped != 0 {
		t.Fatalf("old dropped count returned: %+v %v", status, err)
	}
	n.snapshot.Rows = scannerTestSnapshot(*now).Rows
	n.snapshot.Rows[0].Count = 1
	s.pullScanners(ctx)
	rows, err = st.ScannerRows(ctx, s.id, *now)
	if err != nil || len(rows) != 1 || rows[0].Count != 1 {
		t.Fatalf("new records were lost: %+v %v", rows, err)
	}
}

func TestScannerCollectionDistinguishesUnsupportedAndOffline(t *testing.T) {
	for _, tc := range []struct {
		name        string
		err         error
		unsupported bool
	}{
		{"old node", &nodeapi.StatusError{Method: "GET", Path: "/v1/scanners", Status: 404}, true},
		{"no collector", nil, true},
		{"offline", nodeapi.ErrUnavailable, false},
		{"server error", &nodeapi.StatusError{Path: "/v1/scanners", Status: 500}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, f, st, _, _ := setup(t)
			ctx := context.Background()
			if err := st.SetScannerStatus(ctx, s.id, store.ScannerStatus{Error: "previous error", ErrorAt: 1}); err != nil {
				t.Fatal(err)
			}
			s.node = &scannerTestNode{fakeNode: f, err: tc.err}
			s.pullScanners(ctx)
			status, err := st.ScannerStatusOf(ctx, s.id)
			if err != nil || status.Unsupported != tc.unsupported || (status.Error != "") == tc.unsupported {
				t.Fatalf("status=%+v err=%v", status, err)
			}
		})
	}
}

func TestFailedScannerClearKeepsPanelRecords(t *testing.T) {
	s, f, st, _, now := setup(t)
	ctx := context.Background()
	if err := st.SaveScanners(ctx, s.id, scannerTestSnapshot(*now), *now); err != nil {
		t.Fatal(err)
	}
	for _, failure := range []error{nodeapi.ErrUnavailable, &nodeapi.StatusError{Method: "DELETE", Path: "/v1/scanners", Status: 404}} {
		s.node = &scannerTestNode{fakeNode: f, resetErr: failure}
		if err := s.m.ClearScanners(ctx, s.id); !errors.Is(err, failure) {
			t.Fatalf("clear failure: %v", err)
		}
		rows, err := st.ScannerRows(ctx, s.id, *now)
		if err != nil || len(rows) != 1 {
			t.Fatalf("records changed after failed reset: %+v %v", rows, err)
		}
	}
	s.node = f // no optional journal/reset methods
	if err := s.m.ClearScanners(ctx, s.id); !nodeapi.IsScannerUnsupported(err) {
		t.Fatalf("node without reset support: %v", err)
	}
}
