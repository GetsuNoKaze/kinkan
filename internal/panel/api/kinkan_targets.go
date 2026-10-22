package api

import (
	"context"

	"mikan/internal/nodeapi"
	"mikan/internal/panel/targets"
)

// kinkanSiteDeps lets the check form know the node's own website (Kinkan's node site) when
// it says which node it is about: REALITY may aim at 127.0.0.1:<site port> there, and only
// that node can dial it, so the node checks it. Any other site the panel checks as before.
func (h *handlers) kinkanSiteDeps(ctx context.Context, nodeID int64, d targets.Deps) targets.Deps {
	if nodeID <= 0 || h.d.Store == nil {
		return d
	}
	if _, _, ok, err := h.d.Store.NodeSite(ctx, nodeID); err != nil || !ok {
		return d
	}
	var check func(ctx context.Context, req nodeapi.TargetCheckRequest) (nodeapi.TargetResult, error)
	if c, ok := h.d.Nodes.(interface {
		CheckTarget(ctx context.Context, id int64, req nodeapi.TargetCheckRequest) (nodeapi.TargetResult, error)
	}); ok {
		check = func(ctx context.Context, req nodeapi.TargetCheckRequest) (nodeapi.TargetResult, error) {
			return c.CheckTarget(ctx, nodeID, req)
		}
	}
	return d.WithSite(nodeapi.SiteHTTPSPort, check)
}
