//go:build e2e

package e2e

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestTrustTunnelTCP(t *testing.T) {
	for _, slot := range []int{1, 2} {
		if _, err := download(clientA, socksPort(slot, "tt"), 256<<10); err != nil {
			t.Fatalf("slot %d download: %v", slot, err)
		}
		if err := upload(clientA, socksPort(slot, "tt"), 256<<10); err != nil {
			t.Fatalf("slot %d upload: %v", slot, err)
		}
	}
	if _, err := download(clientA, socksPort(4, "tt"), 1024); err == nil {
		t.Fatal("disabled subscriber could download through TrustTunnel")
	}
}

func TestTrustTunnelCover(t *testing.T) {
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(state.TLS.CertPEM)) {
		t.Fatal("missing test certificate")
	}
	for _, h2 := range []bool{false, true} {
		transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: "node"}, ForceAttemptHTTP2: h2}
		if !h2 {
			transport.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
		}
		t.Cleanup(transport.CloseIdleConnections)
		client := &http.Client{Transport: transport, Timeout: 10 * time.Second}
		for _, probe := range []struct {
			method, auth string
			status       int
		}{
			{http.MethodGet, "", 200},
			{http.MethodGet, "Basic invalid", 200},
			{http.MethodConnect, "", 405},
		} {
			req, err := http.NewRequest(probe.method, "https://node:8447/", nil)
			if err != nil {
				t.Fatal(err)
			}
			if probe.method == http.MethodConnect {
				req.Host = "example.invalid:443"
			}
			req.Header.Set("Proxy-Authorization", probe.auth)
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			major := 1
			if h2 {
				major = 2
			}
			if err != nil || resp.StatusCode != probe.status || resp.ProtoMajor != major || string(body) != "KINKAN COVER" || resp.Header.Get("Proxy-Authenticate") != "" {
				t.Fatalf("h2=%v %s: %d %s %q %v", h2, probe.method, resp.StatusCode, resp.Proto, body, err)
			}
		}
	}
}
