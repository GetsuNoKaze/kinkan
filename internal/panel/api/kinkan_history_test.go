package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"testing"
	"time"

	"mikan/internal/nodeapi"
	"mikan/internal/nodeprobe"
	"mikan/internal/panel/auth"
	"mikan/internal/panel/domain"
	"mikan/internal/panel/nodesync"
	"mikan/internal/panel/settings"
	"mikan/internal/panel/store"
	"mikan/internal/panel/store/db"
	"mikan/internal/panel/store/storetest"
)

type resettingNodes struct {
	NodeRuntime // only ClearScanners is called
	st          *store.Store
	resets      int
	fail        error
}

func (n *resettingNodes) Health(int64) (nodesync.HealthView, bool) {
	return nodesync.HealthView{}, false
}

func (n *resettingNodes) ClearScanners(ctx context.Context, id int64) error {
	n.resets++
	if n.fail != nil {
		return n.fail
	}
	return n.st.DeleteScanners(ctx, id)
}

// A finished check is kept and can be read back, deleted alone or with the node's whole
// history; clearing the journal resets the node's own first, then the panel's copy.
func TestProbeHistoryAndJournalClearing(t *testing.T) {
	ctx := context.Background()
	st, err := storetest.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	now := time.Now()
	clock := func() time.Time { return now }
	if err := domain.Seed(ctx, st, now); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Q.CreateAdmin(ctx, db.CreateAdminParams{Username: "admin", PasswordHash: "x", CreatedAt: now.Unix()}); err != nil {
		t.Fatal(err)
	}
	set := settings.New(st.Q)
	if err := settings.Set(ctx, set, settings.KeyDomain, "node.example"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.ExecContext(ctx, `DELETE FROM inbounds WHERE node_id=1`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Q.CreateInbound(ctx, db.CreateInboundParams{NodeID: 1, Name: "tuic", Preset: "tuic", Port: "24444", Config: "type: tuic\n", CreatedAt: now.Unix(), UpdatedAt: now.Unix()}); err != nil {
		t.Fatal(err)
	}
	sessions := auth.NewSessions(st.Q, clock, nil)
	token, sess, err := sessions.Create(ctx, 1, "127.0.0.1", "test")
	if err != nil {
		t.Fatal(err)
	}
	nodes := &resettingNodes{st: st}
	deps := Deps{Store: st, Settings: set, Sessions: sessions, Now: clock, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), IPLimit: auth.NewLimiter(10, time.Minute, time.Minute, time.Hour),
		Resolve: func(context.Context, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
		},
		Nodes: nodes}
	deps.NodeProbeScan = func(_ context.Context, cfg nodeprobe.Config) (nodeprobe.Report, error) {
		return nodeprobe.Report{Protocol: cfg.Protocol, Target: cfg.Target}, nil
	}
	handler, _, err := New(deps)
	if err != nil {
		t.Fatal(err)
	}
	call := func(method, path, body string, out any) int {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		req.Header.Set("X-CSRF-Token", sess.CsrfToken)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if out != nil && w.Code < 300 {
			if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
				t.Fatal(err)
			}
		}
		return w.Code
	}
	for range 2 {
		if code := call("POST", "/api/v1/nodes/1/protocol-probe", `{}`, nil); code != 200 {
			t.Fatalf("probe: %d", code)
		}
	}
	var runs []ProbeRunView
	if code := call("GET", "/api/v1/nodes/1/protocol-probes", "", &runs); code != 200 || len(runs) != 2 || runs[0].Inbounds != 1 {
		t.Fatalf("history: %d %+v", code, runs)
	}
	var kept NodeProbeView
	if code := call("GET", "/api/v1/protocol-probes/"+itoa(runs[0].ID), "", &kept); code != 200 || len(kept.Inbounds) != 1 || kept.NodeID != 1 {
		t.Fatalf("kept run: %d %+v", code, kept)
	}
	if code := call("DELETE", "/api/v1/protocol-probes/"+itoa(runs[0].ID), "", nil); code != 204 {
		t.Fatalf("delete one: %d", code)
	}
	if code := call("GET", "/api/v1/protocol-probes/"+itoa(runs[0].ID), "", nil); code != 404 {
		t.Fatalf("deleted run still there: %d", code)
	}
	var cleared struct{ Deleted int64 }
	if code := call("DELETE", "/api/v1/nodes/1/protocol-probes", "", &cleared); code != 200 || cleared.Deleted != 1 {
		t.Fatalf("clear history: %d %+v", code, cleared)
	}

	day := now.UTC().Truncate(24 * time.Hour).Unix()
	snap := nodeapi.ScannerSnapshot{Epoch: strings.Repeat("c", 32), Rows: []nodeapi.ScannerRecord{{Day: day, IP: "198.51.100.9", Inbound: "tuic", Protocol: "tuic", Reason: "wrong_credentials", Count: 3, First: now.Unix(), Last: now.Unix(), Scanner: "unknown"}}}
	if err := st.SaveScanners(ctx, 1, snap, now); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		err  error
		code int
	}{
		{"unavailable", nodeapi.ErrUnavailable, 502},
		{"unsupported", nodeapi.ErrScannersUnsupported, 409},
		{"old node", &nodeapi.StatusError{Method: "DELETE", Path: "/v1/scanners", Status: 404}, 409},
	} {
		nodes.fail = tc.err
		if code := call("DELETE", "/api/v1/nodes/1/scanners", "", nil); code != tc.code {
			t.Fatalf("%s: %d, want %d", tc.name, code, tc.code)
		}
		if rows, err := st.ScannerRows(ctx, 1, now); err != nil || len(rows) != 1 {
			t.Fatalf("the panel's copy changed after %s: %+v %v", tc.name, rows, err)
		}
	}
	nodes.fail = nil
	if code := call("DELETE", "/api/v1/nodes/1/scanners", "", nil); code != 204 || nodes.resets != 4 {
		t.Fatalf("clear journal: %d resets=%d", code, nodes.resets)
	}
	if rows, _ := st.ScannerRows(ctx, 1, now); len(rows) != 0 {
		t.Fatalf("rows after clearing: %+v", rows)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
