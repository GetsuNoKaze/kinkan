package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"mikan/internal/panel/auth"
	"mikan/internal/panel/domain"
	"mikan/internal/panel/settings"
	"mikan/internal/panel/store/db"
	"mikan/internal/panel/store/storetest"
)

func siteZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, data := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = f.Write([]byte(data))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestSitesAdminRoutes(t *testing.T) {
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
	sessions := auth.NewSessions(st.Q, clock, nil)
	token, sess, err := sessions.Create(ctx, 1, "127.0.0.1", "test")
	if err != nil {
		t.Fatal(err)
	}
	handler, _, err := New(Deps{Store: st, Settings: settings.New(st.Q), Sessions: sessions, Now: clock, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		IPLimit: auth.NewLimiter(10, time.Minute, time.Minute, time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	call := func(method, path, contentType string, body []byte) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		req.Header.Set("X-CSRF-Token", sess.CsrfToken)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	good := map[string]string{"index.html": "<title>Night Archive</title>", "404.html": "x", "robots.txt": "x", "favicon.ico": "x"}

	// A site without its 404 page is refused, saying what is missing.
	bad := map[string]string{"index.html": "x", "robots.txt": "x", "favicon.ico": "x"}
	if r := call("POST", "/api/v1/sites", "application/zip", siteZip(t, bad)); r.Code != 422 || !strings.Contains(r.Body.String(), "site_missing") || !strings.Contains(r.Body.String(), "404.html") {
		t.Fatalf("incomplete site: %d %s", r.Code, r.Body)
	}

	r := call("POST", "/api/v1/sites?name=night", "application/zip", siteZip(t, good))
	if r.Code != 201 {
		t.Fatalf("upload: %d %s", r.Code, r.Body)
	}
	var up struct {
		SiteView
		Existing bool `json:"existing"`
	}
	if err := json.Unmarshal(r.Body.Bytes(), &up); err != nil {
		t.Fatal(err)
	}
	if up.Existing || up.Name != "night" || up.Title != "Night Archive" || up.Files != 4 {
		t.Fatalf("uploaded: %+v", up)
	}
	r = call("POST", "/api/v1/sites", "application/zip", siteZip(t, good))
	if err := json.Unmarshal(r.Body.Bytes(), &up); err != nil || !up.Existing || up.Name != "night" {
		t.Fatalf("same content again: %d %s", r.Code, r.Body)
	}

	if r := call("PUT", "/api/v1/nodes/1/site", "application/json", []byte(`{"site_id":999999}`)); r.Code != 422 || !strings.Contains(r.Body.String(), "unknown_site") {
		t.Fatalf("unknown site: %d %s", r.Code, r.Body)
	}
	if r := call("PUT", "/api/v1/nodes/1/site", "application/json", []byte(`{"site_id":`+strconv.FormatInt(up.ID, 10)+`}`)); r.Code != 200 {
		t.Fatalf("set node site: %d %s", r.Code, r.Body)
	}
	var list []SiteView
	r = call("GET", "/api/v1/sites", "", nil)
	if err := json.Unmarshal(r.Body.Bytes(), &list); err != nil || len(list) != 1 || len(list[0].Nodes) != 1 || list[0].Nodes[0] != 1 || list[0].SeveralNodes {
		t.Fatalf("list: %d %s", r.Code, r.Body)
	}
	if r := call("DELETE", "/api/v1/sites/"+strconv.FormatInt(up.ID, 10), "", nil); r.Code != 409 || !strings.Contains(r.Body.String(), "site_in_use") {
		t.Fatalf("delete shown site: %d %s", r.Code, r.Body)
	}
	if r := call("PUT", "/api/v1/nodes/1/site", "application/json", []byte(`{"site_id":0}`)); r.Code != 200 {
		t.Fatalf("clear node site: %d %s", r.Code, r.Body)
	}
	if r := call("DELETE", "/api/v1/sites/"+strconv.FormatInt(up.ID, 10), "", nil); r.Code != 204 {
		t.Fatalf("delete: %d %s", r.Code, r.Body)
	}

	rows, err := st.Q.ListAudit(ctx, db.ListAuditParams{BeforeID: 1 << 62, Lim: 100})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for _, row := range rows {
		seen[row.Action]++
	}
	if seen["site.upload"] != 1 || seen["node.site"] != 2 || seen["site.delete"] != 1 {
		t.Fatalf("audit: %v", seen)
	}
}
