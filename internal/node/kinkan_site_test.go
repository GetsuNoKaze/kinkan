package node

import (
	"archive/zip"
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"mikan/internal/nodeapi"
	"mikan/internal/nodetls"
	"mikan/internal/site"
)

func packedSite(t *testing.T, title string) (*site.Site, []byte) {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, data := range map[string]string{
		"index.html": "<title>" + title + "</title>front", "404.html": "own not found", "robots.txt": "User-agent: *",
		"favicon.ico": "ico", "blog/index.html": "blog front", "css/a.css": "body{}",
	} {
		f, _ := w.Create(name)
		_, _ = f.Write([]byte(data))
	}
	_ = w.Close()
	s, err := site.Unpack(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.Archive()
	if err != nil {
		t.Fatal(err)
	}
	return s, a
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func testEngine(t *testing.T) *Engine {
	return &Engine{dataDir: t.TempDir(), log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func TestPutSiteChecksTheArchive(t *testing.T) {
	e := testEngine(t)
	s, archive := packedSite(t, "Night")
	other, _ := packedSite(t, "Day")
	for name, err := range map[string]error{
		"bad hash":       e.PutSite("nothex", archive),
		"another's hash": e.PutSite(other.Hash, archive),
		"not a site":     e.PutSite(s.Hash, []byte("junk")),
	} {
		var ne *nodeapi.Error
		if !errors.As(err, &ne) || ne.Code != "invalid_state" {
			t.Errorf("%s: %v", name, err)
		}
	}
	if err := e.PutSite(s.Hash, archive); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(e.sitePath(s.Hash)); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("kept archive: %v %v", fi, err)
	}
}

func TestSiteServing(t *testing.T) {
	e := testEngine(t)
	s, archive := packedSite(t, "Night")
	port := freePort(t)
	st := nodeapi.DesiredState{Site: &nodeapi.SiteState{Hash: s.Hash, HTTPPort: port}}

	if !e.applySite(st) {
		t.Fatal("a site the node does not have is not reported missing")
	}
	if err := e.PutSite(s.Hash, archive); err != nil {
		t.Fatal(err)
	}
	if e.applySite(st) {
		t.Fatal("the site is still missing after PutSite")
	}
	defer e.stopSite()
	status := e.siteStatus()
	if status == nil || status.Hash != s.Hash || status.HTTP == "" || status.Error != "" {
		t.Fatalf("status = %+v", status)
	}
	base := "http://" + status.HTTP
	get := func(method, p string, header map[string]string) *http.Response {
		req, _ := http.NewRequest(method, base+p, nil)
		for k, v := range header {
			req.Header.Set(k, v)
		}
		resp, err := (&http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { resp.Body.Close() })
		return resp
	}
	body := func(r *http.Response) string { b, _ := io.ReadAll(r.Body); return string(b) }

	front := get("GET", "/", nil)
	if front.StatusCode != 200 || !strings.HasSuffix(body(front), "front") || front.Header.Get("Server") != "Caddy" {
		t.Errorf("front page: %d %v", front.StatusCode, front.Header)
	}
	etag := front.Header.Get("Etag")
	if !regexp.MustCompile(`^"[0-9a-z]+"$`).MatchString(etag) || front.Header.Get("Last-Modified") == "" || front.Header.Get("Accept-Ranges") != "bytes" {
		t.Errorf("file headers: %v", front.Header)
	}
	if r := get("GET", "/", map[string]string{"If-None-Match": etag}); r.StatusCode != 304 {
		t.Errorf("conditional: %d", r.StatusCode)
	}
	if r := get("GET", "/robots.txt", map[string]string{"Range": "bytes=0-3"}); r.StatusCode != 206 || body(r) != "User" {
		t.Errorf("range: %d", r.StatusCode)
	}
	if r := get("GET", "/missing", nil); r.StatusCode != 404 || body(r) != "own not found" || r.Header.Get("Server") != "Caddy" {
		t.Errorf("not found: %d", r.StatusCode)
	}
	if r := get("GET", "/blog", nil); r.StatusCode != 308 || r.Header.Get("Location") != "/blog/" {
		t.Errorf("directory without slash: %d %s", r.StatusCode, r.Header.Get("Location"))
	}
	if r := get("GET", "/blog/", nil); r.StatusCode != 200 || body(r) != "blog front" {
		t.Errorf("directory: %d", r.StatusCode)
	}
	if r := get("POST", "/", nil); r.StatusCode != 405 || r.Header.Get("Allow") != "GET, HEAD" {
		t.Errorf("POST: %d %v", r.StatusCode, r.Header)
	}
	if r := get("HEAD", "/missing", nil); r.StatusCode != 404 || body(r) != "" {
		t.Errorf("HEAD not found: %d", r.StatusCode)
	}
	if r := get("GET", "/../../etc/passwd", nil); r.StatusCode != 404 {
		t.Errorf("traversal: %d", r.StatusCode)
	}

	// Another site takes its place, and the old archive goes; no site stops serving.
	s2, archive2 := packedSite(t, "Day")
	if err := e.PutSite(s2.Hash, archive2); err != nil {
		t.Fatal(err)
	}
	if e.applySite(nodeapi.DesiredState{Site: &nodeapi.SiteState{Hash: s2.Hash, HTTPPort: port}}) {
		t.Fatal("second site missing")
	}
	if r := get("GET", "/", nil); !strings.Contains(body(r), "Day") {
		t.Error("the second site is not served")
	}
	if _, err := os.Stat(e.sitePath(s.Hash)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the old archive stayed: %v", err)
	}
	e.applySite(nodeapi.DesiredState{})
	if e.siteStatus() != nil {
		t.Error("status after no site")
	}
	if _, err := net.DialTimeout("tcp", status.HTTP, time.Second); err == nil {
		t.Error("still listening without a site")
	}
	if entries, _ := os.ReadDir(filepath.Join(e.dataDir, siteDir)); len(entries) != 0 {
		t.Errorf("archives left: %v", entries)
	}
}

// Over TLS the site has the node's certificate and negotiates HTTP/2, like a real site.
func TestSiteServingTLS(t *testing.T) {
	e := testEngine(t)
	s, archive := packedSite(t, "Night")
	pair, err := nodetls.Generate("node.example", x509.ExtKeyUsageServerAuth, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := e.PutSite(s.Hash, archive); err != nil {
		t.Fatal(err)
	}
	st := nodeapi.DesiredState{TLS: &nodeapi.TLSFiles{CertPEM: pair.CertPEM, KeyPEM: pair.KeyPEM},
		Site: &nodeapi.SiteState{Hash: s.Hash, HTTPPort: freePort(t), HTTPSPort: freePort(t)}}
	if e.applySite(st) {
		t.Fatal("missing")
	}
	defer e.stopSite()
	status := e.siteStatus()
	if status.HTTPS == "" || status.Error != "" {
		t.Fatalf("status = %+v", status)
	}
	conn, err := tls.Dial("tcp", status.HTTPS, &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"h2", "http/1.1"}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if p := conn.ConnectionState().NegotiatedProtocol; p != "h2" {
		t.Errorf("negotiated %q, want h2", p)
	}
	// The same state again changes nothing; a port another program holds is reported.
	if e.applySite(st) || e.siteStatus().HTTPS != status.HTTPS {
		t.Error("reapplying the same state changed the site")
	}
	held, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	st.Site.HTTPPort = held.Addr().(*net.TCPAddr).Port
	e.applySite(st)
	if e.siteStatus().Error == "" {
		t.Error("a port in use is not reported")
	}
}

// The site's HTTP/2 announces Caddy's header limit, as the TrustTunnel front does.
func TestSiteHeaderLimitLikeCaddy(t *testing.T) {
	e := testEngine(t)
	s, archive := packedSite(t, "Night")
	pair, err := nodetls.Generate("node.example", x509.ExtKeyUsageServerAuth, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := e.PutSite(s.Hash, archive); err != nil {
		t.Fatal(err)
	}
	e.applySite(nodeapi.DesiredState{TLS: &nodeapi.TLSFiles{CertPEM: pair.CertPEM, KeyPEM: pair.KeyPEM}, Site: &nodeapi.SiteState{Hash: s.Hash, HTTPPort: freePort(t), HTTPSPort: freePort(t)}})
	defer e.stopSite()
	conn, err := tls.Dial("tcp", e.siteStatus().HTTPS, &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"h2"}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.WriteString(conn, "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n\x00\x00\x00\x04\x00\x00\x00\x00\x00"); err != nil {
		t.Fatal(err)
	}
	var head [9]byte
	if _, err := io.ReadFull(conn, head[:]); err != nil || head[3] != 0x4 {
		t.Fatalf("no SETTINGS frame: %x %v", head, err)
	}
	payload := make([]byte, int(head[0])<<16|int(head[1])<<8|int(head[2]))
	if _, err := io.ReadFull(conn, payload); err != nil {
		t.Fatal(err)
	}
	for i := 0; i+6 <= len(payload); i += 6 {
		if id := int(payload[i])<<8 | int(payload[i+1]); id == 6 {
			v := int(payload[i+2])<<24 | int(payload[i+3])<<16 | int(payload[i+4])<<8 | int(payload[i+5])
			if v != 16704 {
				t.Fatalf("MAX_HEADER_LIST_SIZE = %d, want 16704 like Caddy", v)
			}
			return
		}
	}
	t.Fatal("no MAX_HEADER_LIST_SIZE in SETTINGS")
}
