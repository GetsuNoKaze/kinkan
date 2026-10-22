package targets

import (
	"context"
	"net"
	"strconv"

	"mikan/internal/nodeapi"
	"mikan/internal/scan"
)

// kinkanSite is the node's own website over TLS on its loopback (Kinkan's node site), a
// REALITY target like the panel's own port is for self-steal. Only the node can dial it, so
// only the node checks it.
type kinkanSite struct {
	// SitePort is the port of the node's site, 0 when the node shows none or the check is
	// not tied to a node.
	SitePort int
	// SiteCheck asks that node; nil when it cannot be reached.
	SiteCheck func(ctx context.Context, req nodeapi.TargetCheckRequest) (nodeapi.TargetResult, error)
}

// WithSite is d for a node whose site listens on port (0: none), checked by check.
func (d Deps) WithSite(port int, check func(ctx context.Context, req nodeapi.TargetCheckRequest) (nodeapi.TargetResult, error)) Deps {
	d.kinkanSite = kinkanSite{SitePort: port, SiteCheck: check}
	return d
}

func (d Deps) ownSite(host, port string) bool {
	return d.SitePort > 0 && (host == "127.0.0.1" || host == "localhost") && port == strconv.Itoa(d.SitePort)
}

// checkSite has the node check its own site; a node that cannot be asked is a failed check,
// not a reason for the panel to dial its own loopback instead.
func (d Deps) checkSite(ctx context.Context, port, sni string) scan.Result {
	dest := net.JoinHostPort("127.0.0.1", port)
	if d.SiteCheck == nil {
		return scan.Result{Dest: dest, SNI: sni, Error: "node_unreachable"}
	}
	r, err := d.SiteCheck(ctx, nodeapi.TargetCheckRequest{Dest: dest, SNI: sni})
	if err != nil {
		return scan.Result{Dest: dest, SNI: sni, Error: "node_unreachable"}
	}
	return r
}
