package trusttunnel

import (
	"bytes"
	"io"
	"testing"
	"time"

	"github.com/metacubex/http"
	"github.com/metacubex/http/httptest"
	"github.com/metacubex/tls"
)

// Run from the parent module too: its replace selects the patched HTTP server.
// From mihomo's module this also verifies compatibility with upstream HTTP.
func TestKinkanPaddingTrustTunnelFront(t *testing.T) {
	for _, setting := range []string{"", "0"} {
		t.Run("padding="+setting, func(t *testing.T) {
			t.Setenv("KINKAN_TT_PADDING", setting)
			payload := bytes.Repeat([]byte("TT HTTPS DATA padding\x00"), 160000)
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if _, err := w.Write(payload); err != nil {
					t.Errorf("backend: %v", err)
				}
			}))
			defer backend.Close()
			service, address := startFallbackServer(t, backend, 10*time.Second)
			marker, ok := service.httpServer.Handler.(interface{ KinkanTrustTunnelHTTP2Padding() bool })
			if !ok || !marker.KinkanTrustTunnelHTTP2Padding() {
				t.Fatal("TCP front did not opt into padding")
			}
			transport := &http.Transport{
				ForceAttemptHTTP2: true,
				TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
				HTTP2:             &http.HTTP2Config{MaxReceiveBufferPerStream: 16 << 10},
			}
			defer transport.CloseIdleConnections()
			client := &http.Client{Transport: transport, Timeout: 15 * time.Second}
			for i := 0; i < 3; i++ {
				response, err := client.Get("https://" + address + "/file")
				if err != nil {
					t.Fatal(err)
				}
				body, err := io.ReadAll(response.Body)
				_ = response.Body.Close()
				if err != nil || response.ProtoMajor != 2 || !bytes.Equal(body, payload) {
					t.Fatalf("HTTP/2 fallback download: proto=%s bytes=%d err=%v", response.Proto, len(body), err)
				}
			}
		})
	}
}
