package nodeprobe

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	quic "github.com/metacubex/quic-go"
	mtls "github.com/metacubex/tls"
	"mikan/internal/ttprobe"
)

func TestRemoteTUICCloseIsExposed(t *testing.T) {
	certServer := httptest.NewTLSServer(http.NotFoundHandler())
	defer certServer.Close()
	cert := certServer.TLS.Certificates[0]
	roots := x509.NewCertPool()
	roots.AddCert(certServer.Certificate())
	l, err := quic.ListenAddr("127.0.0.1:0", &mtls.Config{Certificates: []mtls.Certificate{{Certificate: cert.Certificate, PrivateKey: cert.PrivateKey}}, NextProtos: []string{"h3"}}, &quic.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		for {
			c, err := l.Accept(context.Background())
			if err != nil {
				return
			}
			go func() { time.Sleep(20 * time.Millisecond); _ = c.CloseWithError(0xfffffff2, "AuthenticationTimeout") }()
		}
	}()
	r, err := Scan(context.Background(), Config{Protocol: "tuic", Target: "https://" + l.Addr().String(), Timeout: 300 * time.Millisecond, TLS: &tls.Config{RootCAs: roots}})
	if err != nil {
		t.Fatal(err)
	}
	if r.Verdict != "exposed" {
		t.Fatalf("lost TUIC signature: %+v", r)
	}
	found := false
	for _, f := range r.Findings {
		if f.Level == "FAIL" && strings.Contains(f.Detail, "0xfffffff2") {
			found = true
		}
	}
	if !found {
		t.Fatal("missing numeric remote close code")
	}
	for _, remote := range []bool{false, true} {
		level, _ := quicError(&quic.ApplicationError{Remote: remote, ErrorCode: 0xfffffff1})
		if (level == "FAIL") != remote {
			t.Fatal("local errors must not disclose the remote protocol")
		}
	}
}

func TestNoiseCloseRemainsInconclusive(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() { defer c.Close(); b := make([]byte, 2048); _, _ = c.Read(b) }()
		}
	}()
	r, err := Scan(context.Background(), Config{Protocol: "shadowsocks", Target: "https://" + l.Addr().String(), Timeout: 200 * time.Millisecond})
	if err != nil || r.Verdict != "inconclusive" || !r.Incomplete {
		t.Fatalf("timing became a clean verdict: %+v %v", r, err)
	}
	if len(r.Findings) != 3 {
		t.Fatalf("missing random-byte observations: %+v", r)
	}
}

func TestCancelledScanAndInvalidEndpoints(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r, err := Scan(ctx, Config{Protocol: "tuic", Target: "https://example.org", Timeout: time.Second})
	if err != nil || !r.Incomplete || r.Verdict != "inconclusive" {
		t.Fatal(r, err)
	}
	for _, target := range []string{"http://example.org", "https://user:pass@example.org", "https://example.org/a", "https://example.org?x=1", "https://example.org:0"} {
		if _, err := Scan(ctx, Config{Target: target, Timeout: time.Second}); err == nil {
			t.Fatal("accepted", target)
		}
	}
}

func TestHTTPSCoverAndLegacyCLI(t *testing.T) {
	s := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("cover")) }))
	s.EnableHTTP2 = true
	s.StartTLS()
	defer s.Close()
	roots := x509.NewCertPool()
	roots.AddCert(s.Certificate())
	r, err := Scan(context.Background(), Config{Protocol: "vless", Target: s.URL, Reference: s.URL, Timeout: time.Second, TLS: &tls.Config{RootCAs: roots}})
	if err != nil {
		t.Fatal(err)
	}
	if r.Incomplete || r.Verdict == "exposed" || len(r.Findings) < 39 {
		t.Fatalf("ordinary HTTPS: %+v", r)
	}
	// Parsing --protocol must not pass an unknown flag to the legacy scanner.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out, stderr bytes.Buffer
	err = Run(ctx, []string{"--protocol", "trusttunnel", "--json", s.URL}, &out, &stderr)
	if !errors.Is(err, context.Canceled) || strings.Contains(stderr.String(), "flag provided") {
		t.Fatalf("legacy dispatch: %v %s %s", err, &out, &stderr)
	}
}

func TestManifest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inbounds.json")
	if err := os.WriteFile(path, []byte(`[{"protocol":"unknown","target":"https://example.org"}]`), 0600); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	err := Run(context.Background(), []string{"--inbounds", path, "--json"}, &out, &stderr)
	var check *ttprobe.CheckError
	if !errors.As(err, &check) || !json.Valid(out.Bytes()) {
		t.Fatal(fmt.Sprint(err), out.String())
	}
}

// A cover on a separate TLS name/port must receive its own ordinary Host header.
// Foreign-Host probes still send the same deliberately invalid host to both ends.
func TestReferenceUsesOwnHTTPHost(t *testing.T) {
	server := func() *httptest.Server {
		var expected string
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Host != expected {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte("same cover"))
		})
		s := httptest.NewUnstartedServer(h)
		s.EnableHTTP2 = true
		s.StartTLS()
		expected = strings.TrimPrefix(s.URL, "https://")
		t.Cleanup(s.Close)
		return s
	}
	target, reference := server(), server()
	roots := x509.NewCertPool()
	roots.AddCert(target.Certificate())
	roots.AddCert(reference.Certificate())
	r, err := Scan(context.Background(), Config{Protocol: "vless", Target: target.URL, Reference: reference.URL, Timeout: time.Second, TLS: &tls.Config{RootCAs: roots}})
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, f := range r.Findings {
		if f.Name == "http/1.1 GET /" || f.Name == "h2 GET /" || f.Name == "HTTP/2 fingerprint" || strings.HasSuffix(f.Name, "foreign Host") {
			checked++
			if f.Level != "PASS" {
				t.Errorf("reference used the wrong Host: %+v", f)
			}
		}
	}
	if checked != 5 {
		t.Fatalf("missing comparisons: %d", checked)
	}
}
