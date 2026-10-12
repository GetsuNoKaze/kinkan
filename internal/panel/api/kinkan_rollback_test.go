package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mikan/internal/panel/auth"
	"mikan/internal/panel/domain"
	"mikan/internal/panel/settings"
	"mikan/internal/panel/store/db"
	"mikan/internal/panel/store/storetest"
	"mikan/internal/panel/updates"
)

func TestRollbackIsSessionOnlyAndDisablesAutomaticUpdates(t *testing.T) {
	ctx := context.Background()
	st, err := storetest.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	now := time.Now()
	clock := func() time.Time { return now }
	if err = domain.Seed(ctx, st, now); err != nil {
		t.Fatal(err)
	}
	hash, err := auth.HashPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = st.Q.CreateAdmin(ctx, db.CreateAdminParams{Username: "admin", PasswordHash: hash, CreatedAt: now.Unix()}); err != nil {
		t.Fatal(err)
	}
	set := settings.New(st.Q)
	if err = settings.Set(ctx, set, settings.KeyAutoUpdate, true); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	u := updates.New(dir, "0.5.0.5-tt.9", nil, nil, clock)
	if err = u.SetPolicy(updates.Policy{Auto: true, Channel: "stable"}); err != nil {
		t.Fatal(err)
	}
	record := filepath.Join(dir, "update", "rollback.json")
	if err = os.WriteFile(record, []byte(`{"current":"0.5.0.5-tt.9","version":"0.5.0.5-tt.8","available":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	sessions := auth.NewSessions(st.Q, clock, nil)
	token, sess, err := sessions.Create(ctx, 1, "127.0.0.1", "test")
	if err != nil {
		t.Fatal(err)
	}
	handler, _, err := New(Deps{Store: st, Settings: set, Sessions: sessions, Now: clock, Version: "0.5.0.5-tt.9", Updates: u, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), IPLimit: auth.NewLimiter(100, time.Minute, time.Minute, time.Hour), UserLimit: auth.NewLimiter(10, time.Minute, time.Minute, time.Hour), TOTP: auth.NewTOTPGuard()})
	if err != nil {
		t.Fatal(err)
	}
	call := func(method, path, body string, session, csrf bool, key string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if session {
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		}
		if csrf {
			req.Header.Set("X-CSRF-Token", sess.CsrfToken)
		}
		if key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	path := "/api/v1/updates/rollback/request"
	body := `{"version":"0.5.0.5-tt.8","password":"correct horse"}`
	if r := call("GET", "/api/v1/updates/rollback", "", false, false, ""); r.Code != 401 {
		t.Fatalf("public record: %d %s", r.Code, r.Body)
	}
	if r := call("POST", path, body, false, false, ""); r.Code != 401 {
		t.Fatalf("public request: %d %s", r.Code, r.Body)
	}
	if r := call("POST", path, body, true, false, ""); r.Code != 403 {
		t.Fatalf("no CSRF: %d %s", r.Code, r.Body)
	}
	key := "mk_" + strings.Repeat("a", 40)
	if _, err = st.Q.CreateAPIKey(ctx, db.CreateAPIKeyParams{AdminID: 1, Name: "test", Prefix: key[:10], Hash: hashAPIKey(key), Scope: "full", CreatedAt: now.Unix()}); err != nil {
		t.Fatal(err)
	}
	if r := call("POST", path, body, false, false, key); r.Code != 403 || !strings.Contains(r.Body.String(), "session_only") {
		t.Fatalf("API key: %d %s", r.Code, r.Body)
	}
	if r := call("POST", path, `{"version":"0.5.0.5-tt.7","password":"correct horse"}`, true, true, ""); r.Code != 409 {
		t.Fatalf("arbitrary target: %d %s", r.Code, r.Body)
	}
	if on, _ := set.On(ctx, settings.AutoUpdate); !on {
		t.Fatal("rejected request changed settings")
	}
	status := filepath.Join(dir, "update", "status.json")
	if err = os.WriteFile(status, []byte(`{"state":"running"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if r := call("POST", path, body, true, true, ""); r.Code != 409 || !strings.Contains(r.Body.String(), "update_busy") {
		t.Fatalf("busy: %d %s", r.Code, r.Body)
	}
	if on, _ := set.On(ctx, settings.AutoUpdate); !on {
		t.Fatal("busy request changed settings")
	}
	if err = os.Remove(status); err != nil {
		t.Fatal(err)
	}
	// A stolen session alone does not roll the panel back.
	if r := call("POST", path, `{"version":"0.5.0.5-tt.8"}`, true, true, ""); r.Code != 422 {
		t.Fatalf("no password: %d %s", r.Code, r.Body)
	}
	if r := call("POST", path, `{"version":"0.5.0.5-tt.8","password":"wrong"}`, true, true, ""); r.Code != 422 || !strings.Contains(r.Body.String(), "wrong_password") {
		t.Fatalf("wrong password: %d %s", r.Code, r.Body)
	}
	if on, _ := set.On(ctx, settings.AutoUpdate); !on {
		t.Fatal("a request with the wrong password changed settings")
	}
	if _, err = os.Stat(filepath.Join(dir, "update", "request")); !os.IsNotExist(err) {
		t.Fatalf("a request with the wrong password was queued: %v", err)
	}
	if r := call("POST", path, body, true, true, ""); r.Code != 202 {
		t.Fatalf("request: %d %s", r.Code, r.Body)
	}
	if on, err := set.On(ctx, settings.AutoUpdate); err != nil || on {
		t.Fatalf("auto remained enabled: %v", err)
	}
	policy, err := os.ReadFile(filepath.Join(dir, "update", "policy.json"))
	if err != nil || !strings.Contains(string(policy), `"auto":false`) {
		t.Fatalf("host auto remained enabled: %s %v", policy, err)
	}
	if r := call("POST", path, body, true, true, ""); r.Code != 409 || !strings.Contains(r.Body.String(), "update_busy") {
		t.Fatalf("duplicate: %d %s", r.Code, r.Body)
	}
}
