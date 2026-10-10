package api

import (
	"context"
	"net/netip"

	"mikan/internal/nodeprobe"
	"mikan/internal/panel/geoip"
	"mikan/internal/ttprobe"
)

// Kinkan's part of the API. Mikan's files call into it with one line each, so that
// merges from upstream meet as few of the fork's lines as possible.

// KinkanDeps is embedded in Deps.
type KinkanDeps struct {
	// GeoIP finds the country and network of the scanner journal's addresses; nil: what
	// the nodes found is shown as it is.
	GeoIP interface{ Lookup(netip.Addr) geoip.Info }
	// TTProbeScan uses the real scanner when nil; supplied by isolated API tests.
	TTProbeScan   func(context.Context, ttprobe.Config) (ttprobe.Report, error)
	NodeProbeScan func(context.Context, nodeprobe.Config) (nodeprobe.Report, error)
}

// registerKinkan registers the fork's endpoints. It runs after api.UseMiddleware: huma
// binds an operation to the middleware there is when it is registered, and one registered
// before would answer without a session.
func (h *handlers) registerKinkan() {
	h.registerTTProbe()
	h.registerNodeProbe()
	h.registerScanners()
	h.registerSites()
}
