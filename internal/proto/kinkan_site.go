package proto

import "strconv"

// ownTarget says whether a loopback REALITY dest is one of the server's own: the panel's
// HTTPS next to its node (self-steal) or the node's website (Kinkan).
func ownTarget(host, port string, o Options) bool {
	if host != "127.0.0.1" && host != "localhost" {
		return false
	}
	return o.SelfStealPort > 0 && port == strconv.Itoa(o.SelfStealPort) || o.SitePort > 0 && port == strconv.Itoa(o.SitePort)
}
