package node

import (
	"context"
	"crypto/x509"
	"io"
	"testing"
	"time"

	http "github.com/metacubex/http"
	"github.com/metacubex/mihomo/component/authevent"
	"github.com/metacubex/mihomo/listener/inbound"
	"github.com/metacubex/quic-go"
	"github.com/metacubex/quic-go/http3"
	tls "github.com/metacubex/tls"

	"mikan/internal/nodetls"
)

// Check the actual sing-quic authentication and masquerade paths, including the
// bandwidth refusal that reaches masquerade after a successful password check.
func TestHysteria2ScannerAuthOnNode(t *testing.T) {
	pair, err := nodetls.Generate("hy2.test", x509.ExtKeyUsageServerAuth, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(pair.CertPEM)) {
		t.Fatal("invalid certificate")
	}
	events := make(chan authevent.Event, 32)
	authevent.SetHandler(func(ev authevent.Event) { events <- ev })
	t.Cleanup(func() { authevent.SetHandler(nil) })
	for _, disabledBBR := range []bool{false, true} {
		name := "normal"
		if disabledBBR {
			name = "disabled_BBR"
		}
		t.Run(name, func(t *testing.T) {
			options := inbound.Hysteria2Option{
				BaseOption:  inbound.BaseOption{NameStr: "hy2", Listen: "127.0.0.1", Port: "0"},
				Users:       map[string]string{"slot": "node-secret"},
				Certificate: pair.CertPEM, PrivateKey: pair.KeyPEM,
				IgnoreClientBandwidth: disabledBBR,
			}
			if disabledBBR {
				options.Down = "10 Mbps"
			}
			in, err := inbound.NewHysteria2(&options)
			if err != nil {
				t.Fatal(err)
			}
			if err := in.Listen(&recordingTunnel{}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = in.Close() })
			for _, tc := range []struct {
				name, auth, rx, reason string
				status                 int
			}{
				{"missing", "", "", "no_credentials", 404},
				{"wrong", "wrong-secret", "1000000", "wrong_credentials", 404},
				{"valid", "node-secret", "1000000", "", 233},
				{"valid_zero_rx", "node-secret", "0", "", 233},
				{"valid_invalid_rx", "node-secret", "invalid", "", 233},
				{"valid_missing_rx", "node-secret", "", "", 233},
				{"wrong_zero_rx", "wrong-secret", "0", "wrong_credentials", 404},
			} {
				t.Run(tc.name, func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					conn, err := quic.DialAddr(ctx, in.Address(), &tls.Config{RootCAs: roots, ServerName: "hy2.test", NextProtos: []string{"h3"}}, nil)
					if err != nil {
						t.Fatal(err)
					}
					defer conn.CloseWithError(0, "")
					transport := &http3.Transport{}
					req, err := http.NewRequestWithContext(ctx, "POST", "https://"+in.Address()+"/auth", nil)
					if err != nil {
						t.Fatal(err)
					}
					req.Host = "hysteria"
					req.Header.Set("Hysteria-Auth", tc.auth)
					req.Header.Set("Hysteria-CC-RX", tc.rx)
					resp, err := transport.NewClientConn(conn).RoundTrip(req)
					if err != nil {
						t.Fatal(err)
					}
					_, _ = io.Copy(io.Discard, resp.Body)
					_ = resp.Body.Close()
					status, reason := tc.status, tc.reason
					if disabledBBR && tc.rx != "1000000" && tc.auth != "" {
						status, reason = 404, ""
					}
					if resp.StatusCode != status {
						t.Fatalf("status=%d, want %d", resp.StatusCode, status)
					}
					select {
					case ev := <-events:
						if reason == "" || ev.Reason != reason || ev.Protocol != "hysteria2" || ev.Remote == "" {
							t.Fatalf("unexpected event: %+v, expected reason %q", ev, reason)
						}
					default:
						if reason != "" {
							t.Fatalf("missing event: %s", reason)
						}
					}
				})
			}
		})
	}
}
