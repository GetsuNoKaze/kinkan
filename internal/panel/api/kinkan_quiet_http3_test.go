package api

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	mhttp "github.com/metacubex/http"
	"github.com/metacubex/quic-go/http3"
	mtls "github.com/metacubex/tls"

	"mikan/internal/nodeprobe"
	"mikan/internal/proto"
)

// A configured obfs inbound may still answer ordinary QUIC (for example, when the
// node has not applied the setting). Successful HTTP/3 replies must not become a
// recommendation claiming that the port is silent.
func TestQuietNodeObfsWithRealHTTP3Responses(t *testing.T) {
	certs := httptest.NewTLSServer(http.NotFoundHandler())
	defer certs.Close()
	cert := certs.TLS.Certificates[0]
	roots := x509.NewCertPool()
	roots.AddCert(certs.Certificate())
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	server := &http3.Server{
		TLSConfig: &mtls.Config{Certificates: []mtls.Certificate{{Certificate: cert.Certificate, PrivateKey: cert.PrivateKey}}},
		Handler: mhttp.HandlerFunc(func(w mhttp.ResponseWriter, _ *mhttp.Request) {
			w.WriteHeader(404)
			_, _ = w.Write([]byte("cover"))
		}),
	}
	go server.Serve(pc)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r, err := nodeprobe.Scan(ctx, nodeprobe.Config{Protocol: "hysteria2", Target: "https://" + pc.LocalAddr().String(),
		Obfuscated: true, Timeout: 400 * time.Millisecond, TLS: &tls.Config{RootCAs: roots}})
	if err != nil {
		t.Fatal(err)
	}
	responses := 0
	for _, f := range r.Findings {
		if f.Level == "ERROR" {
			t.Fatalf("unexpected probe error: %+v", f)
		}
		if f.Name == "HTTP/3 GET /" || f.Name == "HTTP/3 POST /auth" {
			responses++
		}
	}
	code, action, _ := adviceFor(ProtocolProbeReport(r), proto.Template{"type": "hysteria2", "obfs": "salamander"}, true, false, "24443")
	if responses != 2 || r.Verdict != "inconclusive" || code != "incomplete" || action != "none" {
		t.Fatalf("successful HTTP/3 replies: report=%+v advice=%s/%s", r, code, action)
	}
}
