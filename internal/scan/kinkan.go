package scan

import (
	"net/netip"
	"strconv"
)

// kinkanSite: the node's own website over TLS on loopback (Options.SitePort), a REALITY
// target of Kinkan's like the panel's own port is for self-steal. The node dials it when it
// checks that target, as autotune does before it decides a target is down.
func (o Options) kinkanSite(ip netip.Addr, port string) bool {
	return o.SitePort > 0 && ip == netip.MustParseAddr("127.0.0.1") && port == strconv.Itoa(o.SitePort)
}
