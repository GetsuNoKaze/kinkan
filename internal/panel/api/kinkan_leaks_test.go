package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mikan/internal/panel/auth"
	"mikan/internal/panel/domain"
	"mikan/internal/panel/settings"
	"mikan/internal/panel/store"
	"mikan/internal/panel/store/db"
	"mikan/internal/panel/store/storetest"
)

func TestLeakAPIRequiresSessionCSRFAndPersistsPolicy(t *testing.T) {
	ctx := context.Background()
	st, err := storetest.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	now := time.Unix(1800000000, 0)
	clock := func() time.Time { return now }
	if err := domain.Seed(ctx, st, now); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Q.CreateAdmin(ctx, db.CreateAdminParams{Username: "admin", PasswordHash: "x", CreatedAt: now.Unix()}); err != nil {
		t.Fatal(err)
	}
	var uid int64
	if err := st.DB.QueryRow(`INSERT INTO users(name,period_start,sub_token,created_at,updated_at) VALUES('leak',1,'leak',1,1) RETURNING id`).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	p, _ := st.LeakPolicy(ctx)
	if err := st.RecordLeakSources(ctx, uid, []store.LeakSource{{ASN: "AS1"}, {ASN: "AS2"}, {ASN: "AS3"}}, now, p); err != nil {
		t.Fatal(err)
	}
	sessions := auth.NewSessions(st.Q, clock, nil)
	token, sess, err := sessions.Create(ctx, 1, "127.0.0.1", "test")
	if err != nil {
		t.Fatal(err)
	}
	h, _, err := New(Deps{Store: st, Settings: settings.New(st.Q), Sessions: sessions, Now: clock, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), IPLimit: auth.NewLimiter(10, time.Minute, time.Minute, time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	call := func(method, path, body string, session, csrf bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if session {
			r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		}
		if csrf {
			r.Header.Set("X-CSRF-Token", sess.CsrfToken)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	for _, path := range []string{"/api/v1/kinkan/leak-policy", "/api/v1/users/1/leak-events"} {
		if w := call("GET", path, "", false, false); w.Code == 200 {
			t.Fatal("unauthenticated events visible", w.Body)
		}
	}
	policy := `{"enabled":true,"networks":4,"countries":2,"window_minutes":15}`
	if w := call("PUT", "/api/v1/kinkan/leak-policy", policy, true, false); w.Code == 200 {
		t.Fatal("CSRF bypass", w.Body)
	}
	if w := call("PUT", "/api/v1/kinkan/leak-policy", policy, true, true); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	got, _ := st.LeakPolicy(ctx)
	if got.Networks != 4 || got.WindowMinutes != 15 {
		t.Fatal(got)
	}
	if w := call("PUT", "/api/v1/kinkan/leak-policy", `{"enabled":true,"networks":1,"countries":2,"window_minutes":15}`, true, true); w.Code != 422 {
		t.Fatal("bad threshold", w.Code, w.Body)
	}
	w := call("GET", "/api/v1/users/1/leak-events", "", true, false)
	if uid != 1 {
		t.Fatal("fixture user ID changed", uid)
	}
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	var v struct {
		Events []store.LeakEvent `json:"events"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil || len(v.Events) != 1 {
		t.Fatal(v, err, w.Body)
	}
	if w := call("GET", "/api/v1/users/999999/leak-events", "", true, false); w.Code != 404 {
		t.Fatal(w.Code, w.Body)
	}
}
