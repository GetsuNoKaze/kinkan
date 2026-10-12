package node

import (
	"net/http/httptest"
	"testing"
	"time"
)

// What Caddy 2.11.6 with `file_server` and `handle_errors { rewrite * /404.html; file_server }`
// answered on the nodes (12.10.2026), for a site whose address is the node's name.
func TestSiteAnswersOddRequestsLikeCaddy(t *testing.T) {
	mod := time.Date(2026, 10, 12, 10, 52, 57, 0, time.UTC)
	s := &siteServer{hosts: map[string]bool{"node.example": true}, files: map[string]siteFile{}}
	for name, body := range map[string]string{"index.html": "front", "404.html": "own not found"} {
		s.files[name] = siteFile{name: name, data: []byte(body), mod: mod, etag: caddyETag(mod, len(body))}
	}
	s.notFnd = s.files["404.html"]
	do := func(method, target, host string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, target, nil)
		r.Host = host
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	empty := func(name string, w *httptest.ResponseRecorder, code int) {
		t.Helper()
		if w.Code != code || w.Body.Len() != 0 || w.Header().Get("Content-Length") != "0" || w.Header().Get("Server") != "Caddy" || w.Header().Get("Etag") != "" {
			t.Errorf("%s: %d %v %q", name, w.Code, w.Header(), w.Body)
		}
	}
	empty("foreign Host", do("GET", "/", "example.com"), 200)
	empty("no Host (bare HTTP/2 preface)", do("PRI", "*", ""), 200)
	empty("absolute form to another host", do("GET", "http://example.com/", "example.com"), 200)
	empty("CONNECT elsewhere", do("CONNECT", "example.com:443", "example.com:443"), 200)
	w := do("CONNECT", "node.example:443", "node.example:443")
	empty("CONNECT to the site", w, 308)
	if w.Header().Get("Location") != "/" {
		t.Errorf("CONNECT redirects to %q", w.Header().Get("Location"))
	}
	if w := do("GET", "/", "Node.Example:8444"); w.Code != 200 || w.Body.String() != "front" {
		t.Errorf("own name with a port: %d", w.Code)
	}
	for _, c := range []struct {
		method string
		code   int
		vary   int
	}{{"OPTIONS", 405, 2}, {"POST", 405, 2}, {"GET", 404, 1}} {
		target := "/"
		if c.code == 404 {
			target = "/missing"
		}
		w := do(c.method, target, "node.example")
		h := w.Header()
		if w.Code != c.code || w.Body.String() != "own not found" || len(h.Values("Vary")) != c.vary || h.Get("Accept-Ranges") != "bytes" || h.Get("Etag") != s.notFnd.etag || h.Get("Last-Modified") != "Mon, 12 Oct 2026 10:52:57 GMT" {
			t.Errorf("%s %s: %d %v %q", c.method, target, w.Code, h, w.Body)
		}
		if c.code == 405 && h.Get("Allow") != "GET, HEAD" {
			t.Errorf("%s: Allow %q", c.method, h.Get("Allow"))
		}
	}
	if e := caddyETag(time.Unix(0, 1791802377310345903), 9195); e != `"dm2sxkis5kfj-73f"` {
		t.Errorf("ETag %s, Caddy's was \"dm2sxkis5kfj-73f\"", e)
	}
}
