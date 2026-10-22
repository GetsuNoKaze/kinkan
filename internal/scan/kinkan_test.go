package scan

import (
	"net/netip"
	"testing"
)

// The node's own site on loopback is a target the node may dial (Kinkan), its port alone.
func TestKinkanSiteOnLoopback(t *testing.T) {
	lo := netip.MustParseAddr("127.0.0.1")
	o := Options{SitePort: 17443}
	if !o.allowed(lo, "17443") {
		t.Fatal("the node's site on loopback refused")
	}
	for _, c := range []struct {
		ip   string
		port string
	}{{"127.0.0.1", "17444"}, {"127.0.0.2", "17443"}, {"10.0.0.1", "17443"}} {
		if o.allowed(netip.MustParseAddr(c.ip), c.port) {
			t.Errorf("%s:%s allowed", c.ip, c.port)
		}
	}
	if (Options{}).allowed(lo, "17443") {
		t.Fatal("loopback allowed without a site")
	}
	if o := (Options{LoopbackPort: 21973, SitePort: 17443}); !o.allowed(lo, "21973") || !o.allowed(lo, "17443") {
		t.Fatal("self-steal and the site together")
	}
}
