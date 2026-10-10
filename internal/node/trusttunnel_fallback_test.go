package node

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/listener"

	"mikan/internal/nodeapi"
	"mikan/internal/nodetls"
	"mikan/internal/proto"
	"mikan/internal/ttprobe"
)

// Exercise the running embedded listener with credentials rendered by mikan, not only
// a hand-written mihomo configuration. The test certificate is verified through a root pool.
func TestTrustTunnelFallbackOnNode(t *testing.T) {
	cover := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "cover "+r.Method)
	}))
	t.Cleanup(cover.Close)
	home := t.TempDir()
	previous := C.Path.HomeDir()
	C.SetHomeDir(home)
	t.Cleanup(func() { C.SetHomeDir(previous) })
	pair, err := nodetls.Generate("tt.test", x509.ExtKeyUsageServerAuth, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	cert := proto.Cert{CertPath: filepath.Join(home, "node.crt"), KeyPath: filepath.Join(home, "node.key")}
	for file, data := range map[string]string{cert.CertPath: pair.CertPEM, cert.KeyPath: pair.KeyPEM} {
		if err := os.WriteFile(file, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	config, _ := json.Marshal(map[string]any{"type": "trusttunnel", "fallback": strings.TrimPrefix(cover.URL, "http://")})
	rendered, err := listenerFor(nodeapi.Inbound{Name: "tt", Listen: "127.0.0.1", Port: "0", Config: config},
		[]nodeapi.Slot{{Name: "slot1", Secret: "node-secret"}}, cert, proto.Options{})
	if err != nil {
		t.Fatal(err)
	}
	in, err := listener.ParseListener(rendered)
	if err != nil {
		t.Fatal(err)
	}
	if err := in.Listen(&recordingTunnel{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = in.Close() })
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(pair.CertPEM)) {
		t.Fatal("test certificate is invalid")
	}
	for _, h2 := range []bool{false, true} {
		transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: "tt.test"}, ForceAttemptHTTP2: h2}
		if !h2 {
			transport.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
		}
		t.Cleanup(transport.CloseIdleConnections)
		client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
		for _, probe := range []struct {
			method, auth, body string
		}{
			{http.MethodGet, "", "cover GET"},
			{http.MethodGet, "Basic invalid", "cover GET"},
			{http.MethodConnect, "", "cover CONNECT"},
			{http.MethodConnect, "Basic " + base64.StdEncoding.EncodeToString([]byte("slot1:node-secret")), ""},
		} {
			req, err := http.NewRequest(probe.method, "https://"+in.Address()+"/", nil)
			if err != nil {
				t.Fatal(err)
			}
			if probe.method == http.MethodConnect {
				req.Host = "_check"
			}
			req.Header.Set("Proxy-Authorization", probe.auth)
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("h2=%v %s: %v", h2, probe.method, err)
			}
			body, err := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			wantProto := 1
			if h2 {
				wantProto = 2
			}
			if err != nil || resp.StatusCode != 200 || string(body) != probe.body || resp.ProtoMajor != wantProto || resp.Header.Get("Proxy-Authenticate") != "" {
				t.Fatalf("h2=%v %s: status=%d proto=%s body=%q err=%v", h2, probe.method, resp.StatusCode, resp.Proto, body, err)
			}
		}
	}
	// Run the scanner against the actual patched mihomo listener and an HTTPS
	// reference serving the same cover, with certificate verification enabled.
	certificate, err := tls.LoadX509KeyPair(cert.CertPath, cert.KeyPath)
	if err != nil {
		t.Fatal(err)
	}
	reference := httptest.NewUnstartedServer(cover.Config.Handler)
	reference.EnableHTTP2 = true
	reference.TLS = &tls.Config{Certificates: []tls.Certificate{certificate}, NextProtos: []string{"h2", "http/1.1"}}
	reference.StartTLS()
	t.Cleanup(reference.Close)
	report, err := ttprobe.Scan(context.Background(), ttprobe.Config{
		Target: "https://" + in.Address(), Reference: reference.URL, Timeout: 5 * time.Second,
		TLS: &tls.Config{RootCAs: roots, ServerName: "tt.test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Findings) != 33 {
		t.Fatalf("scanner checks=%d", len(report.Findings))
	}
	for _, finding := range report.Findings {
		if finding.Name == "HTTP/2 fingerprint" {
			t.Logf("scanner: %+v", finding)
		}
		if finding.Level == "FAIL" || finding.Level == "ERROR" || strings.Contains(finding.Detail, "cover differs") {
			t.Errorf("scanner: %+v", finding)
		}
	}
}
