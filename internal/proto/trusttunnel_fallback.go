package proto

import (
	"net"
	"net/netip"
	"strconv"
	"strings"
)

// validateFallback checks the TrustTunnel fallback (Kinkan): the host:port of a site
// that answers plain HTTP, since TLS ends at mihomo. Anyone on the internet reaches it
// through the inbound without credentials, so a link-local address, where cloud metadata
// services live (169.254.169.254), is refused. Loopback and private addresses are fine:
// a site on the node itself is the usual setup.
func validateFallback(t Template) error {
	raw, set := t["fallback"]
	if !set {
		return nil
	}
	s, _ := raw.(string)
	host, port, err := net.SplitHostPort(s)
	if err != nil || host == "" || strings.ContainsAny(host, "/@?#") {
		return fail("config_fallback", "fallback")
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		return fail("config_fallback", "fallback")
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		ip = ip.Unmap()
		if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
			return fail("config_fallback_address", "fallback")
		}
	}
	return nil
}
