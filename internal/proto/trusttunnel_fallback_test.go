package proto

import (
	"testing"

	"github.com/metacubex/mihomo/listener"
)

// A TrustTunnel template may carry mihomo's fallback, and it reaches the listener as is.
func TestTrustTunnelFallback(t *testing.T) {
	tpl := Template{"type": "trusttunnel", "fallback": "127.0.0.1:8080"}
	l, err := Listener(tpl, "tt", "", "4443", nil, Cert{CertPath: "node.crt", KeyPath: "node.key"}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if l["fallback"] != "127.0.0.1:8080" {
		t.Fatalf("fallback = %v, want 127.0.0.1:8080", l["fallback"])
	}
	if _, err := listener.ParseListener(l); err != nil {
		t.Fatalf("mihomo rejects the listener: %v", err)
	}
}
