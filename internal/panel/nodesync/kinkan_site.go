package nodesync

import (
	"context"
	"errors"
	"fmt"

	"mikan/internal/nodeapi"
)

// Kinkan: the website a node shows (ROADMAP item 2). The desired state names the node's
// site by hash; a node that does not have it says so (ApplyResult.SiteMissing), gets the
// archive by PutSite and the state again.

// The ports a node serves its site on, on 127.0.0.1: below the panel's random ports
// (20000-60000) and off domain.PortPool. A node that finds one taken says so in its health.
const (
	SiteHTTPPort  = 17080
	SiteHTTPSPort = 17443
)

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
	return &nodeapi.SiteState{Hash: hash, HTTPPort: SiteHTTPPort, HTTPSPort: SiteHTTPSPort}, nil
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
