package updates

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func rollbackChecker(t *testing.T) *Checker {
	t.Helper()
	c := New(t.TempDir(), "0.5.0.5-tt.9", nil, nil, time.Now)
	if err := c.write("rollback.json", []byte(`{"current":"0.5.0.5-tt.9","version":"0.5.0.5-tt.8","available":true,"reason":""}`)); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestRollbackRejectsStaleAndIncompatibleRecords(t *testing.T) {
	c := rollbackChecker(t)
	for _, tc := range []struct{ name, raw, reason string }{
		{"missing", "", "no_previous"},
		{"broken", "{", "no_previous"},
		{"stale", `{"current":"0.5.0.5-tt.10","version":"0.5.0.5-tt.8","available":true}`, "version_changed"},
		{"migration", `{"current":"0.5.0.5-tt.9","version":"0.5.0.5-tt.8","available":false,"reason":"schema_changed"}`, "schema_changed"},
		{"future", `{"current":"0.5.0.5-tt.9","version":"0.5.0.5-tt.10","available":true}`, "version_unsupported"},
		{"used", `{"current":"0.5.0.5-tt.9","version":"0.5.0.5-tt.9","available":false,"reason":"used"}`, "used"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := c.write("rollback.json", []byte(tc.raw)); err != nil {
				t.Fatal(err)
			}
			v := c.Rollback()
			if v.Available || v.Reason != tc.reason {
				t.Fatalf("%+v", v)
			}
			if err := c.RequestRollback("0.5.0.5-tt.8"); !errors.Is(err, ErrRollbackUnavailable) {
				t.Fatal(err)
			}
			if _, ok := c.Requested(); ok {
				t.Fatal("unsafe request queued")
			}
		})
	}
}

func TestRollbackQueuesOnlyRecordedVersionAndCannotOverwriteUpdate(t *testing.T) {
	c := rollbackChecker(t)
	if err := c.RequestRollback("0.5.0.5-tt.7"); !errors.Is(err, ErrRollbackUnavailable) {
		t.Fatal(err)
	}
	if err := c.RequestRollback("0.5.0.5-tt.8"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(c.dir, "request"))
	if err != nil {
		t.Fatal(err)
	}
	var req map[string]string
	if err = json.Unmarshal(raw, &req); err != nil {
		t.Fatal(err)
	}
	if req["do"] != "rollback" || req["from"] != c.version || req["version"] != "0.5.0.5-tt.8" || req["image"] != "" {
		t.Fatal(req)
	}
	if err = c.Request(); !errors.Is(err, ErrUpdateBusy) {
		t.Fatal(err)
	}
	if err = c.RequestRollback(req["version"]); !errors.Is(err, ErrUpdateBusy) {
		t.Fatal(err)
	}
	if err = os.Remove(filepath.Join(c.dir, "request")); err != nil {
		t.Fatal(err)
	}
	if err = c.write("status.json", []byte(`{"state":"running"}`)); err != nil {
		t.Fatal(err)
	}
	if err = c.RequestRollback(req["version"]); !errors.Is(err, ErrUpdateBusy) {
		t.Fatal(err)
	}
	if err = os.Remove(filepath.Join(c.dir, "status.json")); err != nil {
		t.Fatal(err)
	}
	if err = c.Request(); err != nil {
		t.Fatal(err)
	}
	if err = c.RequestRollback(req["version"]); !errors.Is(err, ErrUpdateBusy) {
		t.Fatal(err)
	}
}

func TestConcurrentUpdateAndRollbackQueueOneRequest(t *testing.T) {
	c := rollbackChecker(t)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	wg.Add(2)
	go func() { defer wg.Done(); results <- c.Request() }()
	go func() { defer wg.Done(); results <- c.RequestRollback("0.5.0.5-tt.8") }()
	wg.Wait()
	close(results)
	success, busy := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, ErrUpdateBusy) {
			busy++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || busy != 1 {
		t.Fatalf("success=%d busy=%d", success, busy)
	}
}
