package node

import (
	"net/netip"
	"strconv"
	"strings"

	"mikan/internal/nodeapi"
)

// egressRules refuse users' traffic to the ports, networks and domains of the egress
// filter. They come after the node's own REJECT rules and before the cascade and WARP, so
// no way out reaches what the filter closes. What does not parse is left out: the panel
// checks the lists, an older or broken one must not take the node's rules down.
func egressRules(f *nodeapi.Filters) []string {
	if f == nil {
		return nil
	}
	var r []string
	for _, p := range f.Egress.Ports {
		if validPorts(p) {
			r = append(r, "DST-PORT,"+p+",REJECT")
		}
	}
	for _, n := range f.Egress.Networks {
		p, ok := parseNetwork(n)
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
		d = strings.ToLower(strings.TrimPrefix(strings.TrimPrefix(d, "*."), "."))
		if safeRuleValue(d) {
			r = append(r, "DOMAIN-SUFFIX,"+d+",REJECT")
		}
	}
	return r
}

// validPorts takes a port ("465") or a range ("1000-2000") of 1–65535.
func validPorts(s string) bool {
	lo, hi, isRange := strings.Cut(s, "-")
	if !isRange {
		hi = lo
	}
	a, errA := strconv.Atoi(lo)
	b, errB := strconv.Atoi(hi)
	return errA == nil && errB == nil && a >= 1 && b <= 65535 && a <= b
}

// parseNetwork takes a network ("203.0.113.0/24") or one address.
func parseNetwork(s string) (netip.Prefix, bool) {
	if p, err := netip.ParsePrefix(s); err == nil {
		return p.Masked(), true
	}
	if a, err := netip.ParseAddr(s); err == nil {
		a = a.Unmap()
		return netip.PrefixFrom(a, a.BitLen()), true
	}
	return netip.Prefix{}, false
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
		if p, ok := parseNetwork(n); ok {
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
