package domain

import (
	"context"

	"mikan/internal/nodeapi"
	"mikan/internal/panel/store/db"
)

// sitePort is the port of the node's own website over TLS, which REALITY may aim at, when
// the node shows a site (Kinkan); 0 when it shows none.
func (s *Inbounds) sitePort(ctx context.Context, node db.Node) (int, error) {
	_, _, ok, err := s.st.NodeSite(ctx, node.ID)
	if err != nil || !ok {
		return 0, err
	}
	return nodeapi.SiteHTTPSPort, nil
}
