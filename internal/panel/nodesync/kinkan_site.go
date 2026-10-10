package nodesync

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"mikan/internal/nodeapi"
	"mikan/internal/proto"
)

// Kinkan: the website a node shows (ROADMAP item 2). The desired state names the node's
// site by hash; a node that does not have it says so (ApplyResult.SiteMissing), gets the
// archive by PutSite and the state again.

// sitePutter is a node that takes sites: the Node API client does; Mikan's nodes answer
// its call with 404.
type sitePutter interface {
	PutSite(ctx context.Context, hash string, archive []byte) error
}

// site is the node's site for its desired state; nil when it shows none.
func (s *Syncer) site(ctx context.Context) (*nodeapi.SiteState, error) {
	_, hash, ok, err := s.m.st.NodeSite(ctx, s.id)
	if err != nil || !ok {
		return nil, err
	}
	return &nodeapi.SiteState{Hash: hash, HTTPPort: nodeapi.SiteHTTPPort, HTTPSPort: nodeapi.SiteHTTPSPort}, nil
}

// sendSite gives the node the state's site and applies the state again.
func (s *Syncer) sendSite(ctx context.Context, st nodeapi.DesiredState) (nodeapi.ApplyResult, error) {
	p, ok := s.node.(sitePutter)
	if !ok || st.Site == nil {
		return nodeapi.ApplyResult{}, errors.New("the node cannot take a site")
	}
	id, hash, ok, err := s.m.st.NodeSite(ctx, s.id)
	if err != nil {
		return nodeapi.ApplyResult{}, err
	}
	if !ok || hash != st.Site.Hash {
		return nodeapi.ApplyResult{}, errors.New("the node's site changed meanwhile")
	}
	_, archive, err := s.m.st.SiteArchive(ctx, id)
	if err != nil {
		return nodeapi.ApplyResult{}, err
	}
	if err := p.PutSite(ctx, hash, archive); err != nil {
		return nodeapi.ApplyResult{}, fmt.Errorf("send the site: %w", err)
	}
	res, err := s.node.Apply(ctx, st)
	if err == nil && res.SiteMissing {
		err = errors.New("the node still misses the site it was sent")
	}
	return res, err
}

func sitePortOf(site *nodeapi.SiteState) int {
	if site == nil {
		return 0
	}
	return site.HTTPSPort
}

// servedSite is what the node says it serves, as one comparable value: the site's hash and
// the addresses it listens on.
func servedSite(v *HealthView) nodeapi.SiteStatus {
	if v == nil || !v.OK || v.Health.Site == nil {
		return nodeapi.SiteStatus{}
	}
	return nodeapi.SiteStatus{Hash: v.Health.Site.Hash, HTTP: v.Health.Site.HTTP, HTTPS: v.Health.Site.HTTPS}
}

// fitToSite points a template at the node's site once the node says it serves it, never
// before: a node that cannot (Mikan's, a port another program holds) keeps working as it
// did. TrustTunnel without a fallback of its own falls back to the site. REALITY aimed at
// the panel's own HTTPS (self-steal on the panel's node) aims at the site instead — the
// same certificate and name, a website instead of the panel's empty 404.
func (s *Syncer) fitToSite(t proto.Template, st nodeapi.DesiredState) proto.Template {
	served := servedSite(s.health.Load())
	if st.Site == nil || served.Hash != st.Site.Hash {
		return t
	}
	out := make(proto.Template, len(t))
	for k, v := range t {
		out[k] = v
	}
	if t.Type() == "trusttunnel" && served.HTTP != "" {
		if _, own := t["fallback"]; !own {
			out["fallback"] = "127.0.0.1:" + strconv.Itoa(st.Site.HTTPPort)
		}
	}
	if r, ok := t["reality-config"].(map[string]any); ok && served.HTTPS != "" && st.SelfStealPort > 0 {
		self := strconv.Itoa(st.SelfStealPort)
		if dest, _ := r["dest"].(string); dest == "127.0.0.1:"+self || dest == "localhost:"+self {
			rc := make(map[string]any, len(r))
			for k, v := range r {
				rc[k] = v
			}
			rc["dest"] = "127.0.0.1:" + strconv.Itoa(st.Site.HTTPSPort)
			out["reality-config"] = rc
		}
	}
	return out
}
