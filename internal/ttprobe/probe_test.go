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
	"net/http"
	"net/http/httptest"
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
	if len(r.Findings) != 33 {
		t.Fatalf("got %d checks, want 33", len(r.Findings))
	}
	for _, f := range r.Findings {
		if f.Level == "FAIL" || f.Level == "ERROR" || f.Level == "WARN" {
			t.Errorf("%+v", f)
		}
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
	if warnings != 24 || otherHits != 0 {
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
