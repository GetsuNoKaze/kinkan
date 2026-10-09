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

// The fallback is host:port of a plain-HTTP site; anything else, or a link-local address
// that would hand the cloud metadata service to anyone without credentials, is refused.
func TestTrustTunnelFallbackValidate(t *testing.T) {
	cases := map[string]string{
		"127.0.0.1:8080":          "",
		"10.0.0.5:80":             "",
		"[::1]:8080":              "",
		"cover.internal:8080":     "",
		"http://127.0.0.1:8080":   "config_fallback",
		"127.0.0.1":               "config_fallback",
		"127.0.0.1:0":             "config_fallback",
		"127.0.0.1:65536":         "config_fallback",
		"127.0.0.1:8080/path":     "config_fallback",
		":8080":                   "config_fallback",
		"169.254.169.254:80":      "config_fallback_address",
		"[fe80::1]:80":            "config_fallback_address",
		"[::ffff:169.254.1.1]:80": "config_fallback_address",
		"0.0.0.0:80":              "config_fallback_address",
	}
	for value, want := range cases {
		err := Validate(Template{"type": "trusttunnel", "fallback": value}, Options{})
		got := ""
		if e, ok := err.(*Error); ok {
			got = e.Code
		} else if err != nil {
			got = err.Error()
		}
		if got != want {
			t.Errorf("fallback %q: got %q, want %q", value, got, want)
		}
	}
	if err := Validate(Template{"type": "trusttunnel", "fallback": 8080}, Options{}); err == nil {
		t.Error("a fallback that is not a string was accepted")
	}
}
