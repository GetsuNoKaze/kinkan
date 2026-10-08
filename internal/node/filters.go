package node

import (
	"net/netip"

	"mikan/internal/nodeapi"
)

// egressRules refuse users' traffic to the ports, networks and domains of the egress
// filter. They come after the node's own REJECT rules and before the cascade and WARP, so
// no way out reaches what the filter closes. Each entry is read as the panel reads it
// and written in that canonical form; what does not parse is left out: mihomo refuses
// the whole config over one rule it cannot parse, and an older or broken panel must not
// take the node's users down with it.
func egressRules(f *nodeapi.Filters) []string {
	if f == nil {
		return nil
	}
	var r []string
	for _, p := range f.Egress.Ports {
		if c, ok := nodeapi.CanonPorts(p); ok {
			r = append(r, "DST-PORT,"+c+",REJECT")
		}
	}
	for _, n := range f.Egress.Networks {
		p, ok := nodeapi.CanonNetwork(n)
		if !ok {
			continue
		}
		kind := "IP-CIDR"
		if p.Addr().Is6() {
			kind = "IP-CIDR6"
		}
		// Without no-resolve, as the private ranges: a domain that resolves into the
		// network is refused too.
		r = append(r, kind+","+p.String()+",REJECT")
	}
	for _, d := range f.Egress.Domains {
		if c, ok := nodeapi.CanonDomain(d); ok {
			r = append(r, "DOMAIN-SUFFIX,"+c+",REJECT")
		}
	}
	return r
}

// ingress is the ingress filter as the tunnel checks it.
type ingress struct {
	allow bool
	nets  []netip.Prefix
}

func (in *ingress) admits(a netip.Addr) bool {
	a = a.Unmap()
	for _, p := range in.nets {
		if p.Contains(a) {
			return in.allow
		}
	}
	return !in.allow
}

// SetIngress takes the ingress filter of the state; without networks it lets everyone in.
func (r *Registry) SetIngress(f *nodeapi.Filters) {
	if f == nil {
		r.ingress.Store(nil)
		return
	}
	in := &ingress{allow: f.Ingress.Allow}
	for _, n := range f.Ingress.Networks {
		if p, ok := nodeapi.CanonNetwork(n); ok {
			in.nets = append(in.nets, p)
		}
	}
	// An allow list with nothing that parses would shut every user out; nothing listed
	// is no filter.
	if len(in.nets) == 0 {
		r.ingress.Store(nil)
		return
	}
	r.ingress.Store(in)
}

// admitFrom says whether the ingress filter lets a connection from the address in.
func (r *Registry) admitFrom(a netip.Addr) bool {
	in := r.ingress.Load()
	return in == nil || in.admits(a)
}
