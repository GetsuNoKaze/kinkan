package ttprobe

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/http2"
)

func site(t *testing.T, handler http.Handler) (*httptest.Server, Config) {
	t.Helper()
	s := httptest.NewUnstartedServer(handler)
	s.EnableHTTP2 = true
	s.TLS = &tls.Config{NextProtos: []string{"h2", "http/1.1"}}
	s.Config.ErrorLog = log.New(io.Discard, "", 0)
	s.StartTLS()
	t.Cleanup(s.Close)
	roots := x509.NewCertPool()
	roots.AddCert(s.Certificate())
	return s, Config{Target: s.URL, Timeout: time.Second, TLS: &tls.Config{RootCAs: roots}}
}

func TestOrdinaryHTTPSAndReference(t *testing.T) {
	s, cfg := site(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "cover %s %s %s", r.Method, r.URL.Path, r.Host)
	}))
	cfg.Reference = s.URL
	r, err := Scan(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Findings) != 36 {
		t.Fatalf("got %d checks, want 36", len(r.Findings))
	}
	for _, f := range r.Findings {
		if f.Level == "FAIL" || f.Level == "ERROR" || f.Level == "WARN" {
			t.Errorf("%+v", f)
		}
	}
	last := r.Findings[len(r.Findings)-1]
	if last.Name != "HTTP/2 fingerprint" || last.Compare == nil || len(last.Compare.Target.Settings) == 0 {
		t.Fatalf("fingerprint comparison is not structured: %+v", last)
	}
}

func TestProxyDisclosureEvenWithOversizedBody(t *testing.T) {
	_, cfg := site(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Proxy-Authorization") != "" {
			w.Header().Set("Proxy-Authenticate", "")
			w.WriteHeader(407)
			io.Copy(w, strings.NewReader(strings.Repeat("x", maxBody+1)))
			return
		}
		io.WriteString(w, "cover")
	}))
	r, err := Scan(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	fails := 0
	for _, f := range r.Findings {
		if strings.Contains(f.Detail, "proxy disclosure") {
			if f.Level != "FAIL" {
				t.Fatalf("%+v", f)
			}
			fails++
		}
	}
	if fails != 8 {
		t.Fatalf("wrong/malformed credentials should disclose proxy in both protocols, got %d", fails)
	}
}

func TestCoverMismatchIsWarningAndRedirectsStayOnOrigin(t *testing.T) {
	otherHits := 0
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { otherHits++; io.WriteString(w, "unexpected") }))
	defer other.Close()
	s, cfg := site(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, other.URL, 302) }))
	ref, _ := site(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "cover") }))
	cfg.Reference = ref.URL
	r, err := Scan(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	warnings := 0
	for _, f := range r.Findings {
		if strings.Contains(f.Detail, "cover differs") {
			warnings++
		}
		if f.Level == "FAIL" || f.Level == "ERROR" {
			t.Errorf("%+v", f)
		}
	}
	if warnings != 27 || otherHits != 0 {
		t.Fatalf("warnings=%d redirected requests=%d target=%s", warnings, otherHits, s.URL)
	}
}

func TestInvalidTargetsAndTimeouts(t *testing.T) {
	for _, raw := range []string{"http://example.org", "https://u:p@example.org", "https://example.org/path", "https://example.org?x=1", "https://example.org?", "https://example.org#fragment", "https://example.org:", "https://example.org:0", "https://example.org:65536"} {
		if _, err := Scan(context.Background(), Config{Target: raw, Timeout: time.Second}); err == nil {
			t.Errorf("accepted %q", raw)
		}
	}
	for _, duration := range []time.Duration{0, 99 * time.Millisecond, 31 * time.Second} {
		if _, err := Scan(context.Background(), Config{Target: "https://example.org", Timeout: duration}); err == nil {
			t.Errorf("accepted timeout %s", duration)
		}
	}
	var out, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"--help"}, &out, &stderr); err != nil || !strings.Contains(stderr.String(), "Usage:") {
		t.Fatalf("help: %v %s", err, stderr.String())
	}
}

func TestCertificateValidationAndJSONFailure(t *testing.T) {
	s, _ := site(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "cover") }))
	var out, stderr bytes.Buffer
	err := Run(context.Background(), []string{"--json", s.URL}, &out, &stderr)
	var failed *CheckError
	if !errors.As(err, &failed) || failed.Incomplete == 0 || !strings.Contains(out.String(), `"findings"`) {
		t.Fatalf("untrusted certificate was not reported: %v %s", err, out.String())
	}
}

func TestNoSNIReallyAbsentAndH2WithoutALPNDetected(t *testing.T) {
	certServer, cfg := site(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	var mu sync.Mutex
	var names []string
	tlsCfg := certServer.TLS.Clone()
	tlsCfg.NextProtos = []string{"h2", "http/1.1"}
	tlsCfg.GetConfigForClient = func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
		mu.Lock()
		names = append(names, hello.ServerName)
		mu.Unlock()
		return nil, nil
	}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", tlsCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	var workers sync.WaitGroup
	acceptDone := make(chan struct{})
	go func() {
		defer close(acceptDone)
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			workers.Add(1)
			go func() {
				defer workers.Done()
				defer c.Close()
				c.SetDeadline(time.Now().Add(time.Second))
				if err := c.(*tls.Conn).Handshake(); err != nil {
					return
				}
				var preface [24]byte
				if _, err := io.ReadFull(c, preface[:]); err != nil {
					return
				}
				if string(preface[:]) == http2.ClientPreface {
					_ = http2.NewFramer(c, c).WriteSettings()
				}
			}()
		}
	}()
	cfg.Target = "https://" + ln.Addr().String()
	u, _ := endpoint(cfg.Target)
	// DNS host matters: tls.Dialer would auto-fill "localhost" and send SNI.
	diagnosticURL := *u
	diagnosticURL.Host = "localhost:" + u.Port()
	c, err := dial(context.Background(), cfg, &diagnosticURL, []string{"http/1.1"}, "", true)
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	c, err = dial(context.Background(), cfg, &diagnosticURL, []string{"http/1.1"}, "kinkan-probe.invalid", true)
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	mu.Lock()
	got := append([]string(nil), names...)
	mu.Unlock()
	if len(got) != 2 || got[0] != "" || got[1] != "kinkan-probe.invalid" {
		t.Fatalf("SNI=%v", got)
	}
	// Exercise the actual full report, including a deliberately leaky raw listener.
	cfg.Timeout = 100 * time.Millisecond
	r, err := Scan(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	leaks := 0
	for _, f := range r.Findings {
		if f.Level == "FAIL" && strings.Contains(f.Detail, "without negotiated h2") {
			leaks++
		}
	}
	if leaks != 2 {
		t.Fatalf("raw h2 leaks=%d", leaks)
	}
	ln.Close()
	<-acceptDone
	workers.Wait()
}

func TestRequestBoundAndCancellation(t *testing.T) {
	_, cfg := site(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, strings.Repeat("x", maxBody+1)) }))
	u, _ := endpoint(cfg.Target)
	if _, err := request(context.Background(), cfg, u, u.Host, "http/1.1", probes[0]); err == nil || !strings.Contains(err.Error(), "2 MiB") {
		t.Fatalf("unbounded response: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := request(ctx, cfg, u, u.Host, "h2", probes[0]); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled request: %v", err)
	}
	if _, err := dial(ctx, cfg, u, nil, u.Hostname(), false); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled handshake: %v", err)
	}
}

func TestPinnedDialerUsedByEveryTransport(t *testing.T) {
	s, cfg := site(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "cover") }))
	u, _ := endpoint(s.URL)
	cfg.Target = "https://example.com:443"
	cfg.Reference = "https://example.com:8444"
	var mu sync.Mutex
	var addresses []string
	cfg.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		mu.Lock()
		addresses = append(addresses, addr)
		mu.Unlock()
		return (&net.Dialer{}).DialContext(ctx, network, u.Host)
	}
	r, err := Scan(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range r.Findings {
		if f.Level == "FAIL" || f.Level == "ERROR" || f.Level == "WARN" {
			t.Errorf("%+v", f)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(addresses) != 67 {
		t.Fatalf("not every probe used the pinned dialer: calls=%d", len(addresses))
	}
	for _, a := range addresses {
		if a != "example.com:443" && a != "example.com:8444" {
			t.Errorf("unexpected address %s", a)
		}
	}
}

func findings(r Report, name string) []Finding {
	var out []Finding
	for _, f := range r.Findings {
		if f.Name == name {
			out = append(out, f)
		}
	}
	return out
}

func TestAbsoluteFormProxyDisclosure(t *testing.T) {
	_, cfg := site(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.IsAbs() {
			w.Header().Set("Proxy-Authenticate", `Basic realm="proxy"`)
			w.WriteHeader(407)
			return
		}
		io.WriteString(w, "cover")
	}))
	r, err := Scan(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	got := findings(r, "http/1.1 absolute-form request")
	if len(got) != 1 || got[0].Level != "FAIL" {
		t.Fatalf("absolute-form proxy answer not reported: %+v", got)
	}
}

func TestForeignHostAndAbsoluteFormComparedWithCover(t *testing.T) {
	// The target serves every name, the reference only its own: the default virtual
	// host shows the difference a scanner would see.
	_, cfg := site(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "cover") }))
	ref, _ := site(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host == foreignHost {
			http.NotFound(w, r)
			return
		}
		io.WriteString(w, "cover")
	}))
	cfg.Reference = ref.URL
	r, err := Scan(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"http/1.1 foreign Host", "h2 foreign Host", "http/1.1 absolute-form request"} {
		got := findings(r, name)
		if len(got) != 1 || got[0].Level != "WARN" {
			t.Errorf("%s: %+v", name, got)
		}
	}
	if got := findings(r, "http/1.1 GET /"); len(got) != 1 || got[0].Level != "PASS" {
		t.Errorf("own host: %+v", got)
	}
}

// rawSite answers every TLS connection with a fixed HTTP/1 response, like a front
// server other than Go's would answer a bare HTTP/2 preface.
func rawSite(t *testing.T, cert tls.Certificate, answer string) string {
	t.Helper()
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}, NextProtos: []string{"h2", "http/1.1"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				c.SetDeadline(time.Now().Add(time.Second))
				buf := make([]byte, 64)
				if _, err := c.Read(buf); err != nil {
					return
				}
				io.WriteString(c, answer)
			}()
		}
	}()
	return "https://" + ln.Addr().String()
}

func TestPrefaceAnswerComparedWithCover(t *testing.T) {
	s, cfg := site(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	cfg.Reference = rawSite(t, s.TLS.Certificates[0], "HTTP/1.1 400 Bad Request\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
	u, _ := endpoint(cfg.Reference)
	ref, err := raw(context.Background(), cfg, u, nil, http2.ClientPreface+emptySettings)
	if err != nil || ref.statusLine != "HTTP/1.1 400 Bad Request" {
		t.Fatalf("raw reference answer: %+v %v", ref, err)
	}
	r, err := Scan(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"raw HTTP/2 preface without h2: ", "raw HTTP/2 preface without h2: http/1.1"} {
		got := findings(r, name)
		if len(got) != 1 || got[0].Level != "WARN" || !strings.Contains(got[0].Detail, "differs from cover") {
			t.Errorf("%s: %+v", name, got)
		}
	}
}

func TestUnreachableReferenceSkipsComparisonsOnce(t *testing.T) {
	_, cfg := site(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "cover") }))
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Reference = "https://" + ln.Addr().String()
	ln.Close()
	r, err := Scan(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	errorsSeen := 0
	for _, f := range r.Findings {
		if f.Level == "ERROR" {
			errorsSeen++
			if f.Name != "reference" {
				t.Errorf("unexpected error: %+v", f)
			}
		}
	}
	if errorsSeen != 1 || r.Reference != cfg.Reference {
		t.Fatalf("errors=%d reference=%q", errorsSeen, r.Reference)
	}
	if got := findings(r, "HTTP/2 fingerprint"); len(got) != 1 || got[0].Level != "INFO" {
		t.Fatalf("fingerprint after an unreachable reference: %+v", got)
	}
}

func TestAddressFlagPinsConnections(t *testing.T) {
	var out, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"--address", "not-an-ip", "https://example.org"}, &out, &stderr); err == nil || !strings.Contains(err.Error(), "--address") {
		t.Fatalf("bad address accepted: %v", err)
	}
	// The name does not resolve; only the pinned IP makes the handshake reach the site.
	s, cfg := site(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "cover") }))
	u, _ := endpoint(s.URL)
	cfg.Target = "https://unresolvable.invalid:" + u.Port()
	cfg.TLS.ServerName = "example.com" // in httptest's certificate
	cfg.DialContext = addressDialer(netip.MustParseAddr(u.Hostname()))
	r, err := Scan(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range r.Findings {
		if f.Level == "ERROR" || f.Level == "FAIL" {
			t.Errorf("%+v", f)
		}
	}
}
