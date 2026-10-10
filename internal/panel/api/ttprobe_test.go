package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"mikan/internal/panel/auth"
	"mikan/internal/panel/domain"
	"mikan/internal/panel/settings"
	"mikan/internal/panel/store/db"
	"mikan/internal/panel/store/storetest"
	"mikan/internal/ttprobe"
)

func TestTTProbeRejectsPrivateDNSAndPinsResolution(t *testing.T) {
	for _, host := range []string{"localhost", "127.0.0.1", "169.254.169.254", "10.0.0.1", "[::1]"} {
		if _, err := publicProbeDialer(context.Background(), host, nil); err == nil {
			t.Errorf("accepted %s", host)
		}
	}
	for _, ips := range [][]netip.Addr{nil, {netip.MustParseAddr("127.0.0.1")}, {netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("10.0.0.1")}, {netip.MustParseAddr("::ffff:127.0.0.1")}, {netip.MustParseAddr("100.64.0.1")}} {
		if _, err := publicProbeDialer(context.Background(), "node.example", func(context.Context, string) ([]netip.Addr, error) { return ips, nil }); err == nil {
			t.Errorf("accepted DNS %v", ips)
		}
	}
	calls := 0
	dial, err := publicProbeDialer(context.Background(), "node.example", func(context.Context, string) ([]netip.Addr, error) {
		calls++
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, addr := range []string{"node.example:443", "node.example:8444"} {
		if _, err := dial(ctx, "tcp", addr); !errors.Is(err, context.Canceled) {
			t.Errorf("cancellation: %v", err)
		}
	}
	if calls != 1 {
		t.Fatalf("DNS resolved %d times", calls)
	}
	if _, err := dial(context.Background(), "tcp", "another.example:443"); err == nil {
		t.Fatal("dialed unrelated host")
	}
}

func TestTTProbeAdminRoute(t *testing.T) {
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
	ib, err := st.Q.CreateInbound(ctx, db.CreateInboundParams{NodeID: 1, Name: "probe-tt", Preset: "trusttunnel", Port: "24443", Config: "type: trusttunnel\nfallback: 127.0.0.1:8080\n", CreatedAt: now.Unix(), UpdatedAt: now.Unix()})
	if err != nil {
		t.Fatal(err)
	}
	other, err := st.Q.CreateInbound(ctx, db.CreateInboundParams{NodeID: 1, Name: "not-tt", Preset: "vless", Port: "24444", Config: "type: vless\n", CreatedAt: now.Unix(), UpdatedAt: now.Unix()})
	if err != nil {
		t.Fatal(err)
	}
	sessions := auth.NewSessions(st.Q, clock, nil)
	token, sess, err := sessions.Create(ctx, 1, "127.0.0.1", "test")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	handler, _, err := New(Deps{Store: st, Settings: set, Sessions: sessions, Now: clock, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		IPLimit: auth.NewLimiter(10, time.Minute, time.Minute, time.Hour),
		Resolve: func(context.Context, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
		},
		TTProbeScan: func(ctx context.Context, cfg ttprobe.Config) (ttprobe.Report, error) {
			calls++
			if cfg.Target != "https://node.example:24443" || cfg.Reference != "https://node.example:8444" || cfg.DialContext == nil {
				t.Errorf("wrong scanner config: %+v", cfg)
			}
			if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 75*time.Second {
				t.Error("unbounded scan")
			}
			return ttprobe.Report{Target: cfg.Target, Reference: cfg.Reference, Findings: []ttprobe.Finding{{Name: "HTTP/2", Level: "WARN", Detail: "different"}}}, context.DeadlineExceeded
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	call := func(authenticated, csrf bool, id int64) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/api/v1/nodes/1/trusttunnel-probe", strings.NewReader(fmt.Sprintf(`{"inbound_id":%d,"reference_port":8444}`, id)))
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
	if r := call(false, false, ib.ID); r.Code != 401 {
		t.Fatalf("unauthenticated %d %s", r.Code, r.Body)
	}
	if r := call(true, false, ib.ID); r.Code != 403 {
		t.Fatalf("no CSRF %d %s", r.Code, r.Body)
	}
	key := "mk_" + strings.Repeat("a", 40)
	if _, err := st.Q.CreateAPIKey(ctx, db.CreateAPIKeyParams{AdminID: 1, Name: "test", Prefix: key[:10], Hash: hashAPIKey(key), Scope: "full", CreatedAt: now.Unix()}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/api/v1/nodes/1/trusttunnel-probe", strings.NewReader(fmt.Sprintf(`{"inbound_id":%d}`, ib.ID)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	keyResult := httptest.NewRecorder()
	handler.ServeHTTP(keyResult, req)
	if keyResult.Code != 403 || !strings.Contains(keyResult.Body.String(), "session_only") {
		t.Fatalf("API key allowed: %d %s", keyResult.Code, keyResult.Body)
	}
	if r := call(true, true, other.ID); r.Code != 422 {
		t.Fatalf("not TT %d %s", r.Code, r.Body)
	}
	ttProbing.Lock()
	r := call(true, true, ib.ID)
	ttProbing.Unlock()
	if r.Code != 409 {
		t.Fatalf("concurrent scan %d %s", r.Code, r.Body)
	}
	r = call(true, true, ib.ID)
	if r.Code != 200 {
		t.Fatalf("admin scan %d %s", r.Code, r.Body)
	}
	var result TTProbeView
	if err := json.Unmarshal(r.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || result.Vantage != "panel" || len(result.Report.Findings) != 2 || result.Report.Findings[1].Level != "ERROR" {
		t.Fatalf("partial report lost: calls=%d %+v", calls, result)
	}
	rows, err := st.Q.ListAudit(ctx, db.ListAuditParams{BeforeID: 1 << 62, Lim: 100})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range rows {
		if row.Action == "node.trusttunnel_probe" {
			found = true
		}
	}
	if !found {
		t.Fatal("admin scan was not audited")
	}
}
