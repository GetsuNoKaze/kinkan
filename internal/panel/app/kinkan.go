package app

import (
	"context"

	"mikan/internal/panel/api"
	"mikan/internal/panel/geoip"
)

// Kinkan's part of the panel. Mikan's files call into it with one line each, so that
// merges from upstream meet as few of the fork's lines as possible.

// KinkanOptions is embedded in Options.
type KinkanOptions struct {
	// GeoIP is where the scanner journal's country and network databases come from
	// (geoip.BaseURL); "" never fetches them.
	GeoIP string
}

// kinkanOptions are the fork's options of a running panel (Serve); tests leave them out.
func kinkanOptions() KinkanOptions {
	return KinkanOptions{GeoIP: geoip.BaseURL}
}

// KinkanPanel is embedded in Panel.
type KinkanPanel struct {
	GeoIP *geoip.DB // the scanner journal's countries and networks
}

// newKinkan makes the fork's services and hands the API what it needs of them.
func (p *Panel) newKinkan(o Options, deps *api.Deps) {
	p.GeoIP = geoip.New(o.DataDir, o.GeoIP, o.Log, o.Now)
	deps.GeoIP = p.GeoIP
}

// kinkanWorkers are the fork's loops that Run keeps.
func (p *Panel) kinkanWorkers() []func(context.Context) {
	return []func(context.Context){p.GeoIP.Run}
}
