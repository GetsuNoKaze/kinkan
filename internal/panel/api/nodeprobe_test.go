package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mikan/internal/nodeprobe"
	"mikan/internal/panel/auth"
	"mikan/internal/panel/domain"
	"mikan/internal/panel/settings"
	"mikan/internal/panel/store/db"
	"mikan/internal/panel/store/storetest"
	"mikan/internal/ttprobe"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestNodeProbeAllInboundsArePinnedAndSessionOnly(t *testing.T) {
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
	for _, p := range []struct{ typ, port string }{{"trusttunnel", "24443"}, {"tuic", "24444"}} {
		if _, err := st.Q.CreateInbound(ctx, db.CreateInboundParams{NodeID: 1, Name: p.typ, Preset: p.typ, Port: p.port, Config: "type: " + p.typ + "\n", CreatedAt: now.Unix(), UpdatedAt: now.Unix()}); err != nil {
			t.Fatal(err)
		}
	}
	sessions := auth.NewSessions(st.Q, clock, nil)
	token, sess, err := sessions.Create(ctx, 1, "127.0.0.1", "test")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	lookups := 0
	handler, _, err := New(Deps{Store: st, Settings: set, Sessions: sessions, Now: clock, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), IPLimit: auth.NewLimiter(10, time.Minute, time.Minute, time.Hour), Resolve: func(context.Context, string) ([]netip.Addr, error) {
		lookups++
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	}, NodeProbeScan: func(ctx context.Context, cfg nodeprobe.Config) (nodeprobe.Report, error) {
		calls++
		if cfg.Address != "8.8.8.8" || cfg.Reference != "https://node.example:8444" {
			t.Errorf("unpinned scan: %+v", cfg)
		}
		if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 75*time.Second {
			t.Error("unbounded scan")
		}
		return nodeprobe.Report{Protocol: cfg.Protocol, Target: cfg.Target, Reference: cfg.Reference, Findings: []ttprobe.Finding{{Name: "close", Level: "FAIL", Detail: "TUIC code"}}}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	call := func(authenticated, csrf bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/api/v1/nodes/1/protocol-probe", strings.NewReader(`{"reference_port":8444}`))
		req.Header.Set("Content-Type", "application/json")
		if authenticated {
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		}
		if csrf {
			req.Header.Set("X-CSRF-Token", sess.CsrfToken)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	if r := call(false, false); r.Code != 401 {
		t.Fatal(r.Code, r.Body)
	}
	if r := call(true, false); r.Code != 403 {
		t.Fatal(r.Code, r.Body)
	}
	r := call(true, true)
	if r.Code != 200 {
		t.Fatal(r.Code, r.Body)
	}
	var result NodeProbeView
	if err := json.Unmarshal(r.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || lookups != 1 || len(result.Inbounds) != 2 || result.Inbounds[1].Report.Verdict != "exposed" {
		t.Fatalf("incomplete node scan: %+v calls=%d lookups=%d", result, calls, lookups)
	}
}
