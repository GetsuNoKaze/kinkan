// Package filters holds the ingress and egress filters' settings. The nodes apply them
// (see internal/node/filters.go): egress as routing rules that refuse users' traffic,
// ingress as a check of where a connection comes from.
package filters

import (
	"context"
	"errors"
	"net/netip"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"mikan/internal/nodeapi"
	"mikan/internal/panel/settings"
)

const KeyConfig = "traffic_filters"

// MaxItems bounds each list.
const MaxItems = 1000

// MailPorts are what the mail preset closes besides 25, which the nodes always close:
// submission and SMTPS, and 2525, the alternative submission port.
var MailPorts = []string{"465", "587", "2525"}

var ErrConfig = errors.New("filters_config")

type Config struct {
	Egress  Egress  `json:"egress"`
	Ingress Ingress `json:"ingress"`
}

type Egress struct {
	Enabled bool `json:"enabled"`
	// Mail closes MailPorts: a shared VPN address that sends mail is on blacklists within
	// hours.
	Mail     bool     `json:"mail"`
	Ports    []string `json:"ports"`
	Networks []string `json:"networks"`
	Domains  []string `json:"domains"`
}

type Ingress struct {
	Enabled bool `json:"enabled"`
	// Allow: only Networks may connect; otherwise Networks may not.
	Allow    bool     `json:"allow"`
	Networks []string `json:"networks"`
}

func Default() Config {
	return Config{Egress: Egress{Mail: true, Ports: []string{}, Networks: []string{}, Domains: []string{}}, Ingress: Ingress{Networks: []string{}}}
}

func Load(ctx context.Context, s *settings.Settings) (Config, error) {
	c, _, err := settings.GetOver(ctx, s, KeyConfig, Default())
	c.fill()
	return c, err
}

func (c *Config) fill() {
	for _, l := range []*[]string{&c.Egress.Ports, &c.Egress.Networks, &c.Egress.Domains, &c.Ingress.Networks} {
		if *l == nil {
			*l = []string{}
		}
	}
}

// Problem is what is wrong with one entry of a list: the list ("egress.ports") and the
// entry's index.
type Problem struct {
	List  string
	Index int
	Value string
}

func (p *Problem) Error() string {
	return "filters: " + p.List + "[" + strconv.Itoa(p.Index) + "] " + p.Value
}

// Validate puts each list in its canonical form (trimmed, networks masked, domains in
// lower case, sorted, without repeats) and returns a *Problem for an entry that is not a
// port, a network or a domain, ErrConfig for a list too long.
func (c *Config) Validate() error {
	c.fill()
	lists := []struct {
		name  string
		l     *[]string
		canon func(string) (string, bool)
	}{
		{"egress.ports", &c.Egress.Ports, canonPorts},
		{"egress.networks", &c.Egress.Networks, canonNetwork},
		{"egress.domains", &c.Egress.Domains, canonDomain},
		{"ingress.networks", &c.Ingress.Networks, canonNetwork},
	}
	for _, x := range lists {
		if len(*x.l) > MaxItems {
			return ErrConfig
		}
		out := make([]string, 0, len(*x.l))
		for i, v := range *x.l {
			s, ok := x.canon(strings.TrimSpace(v))
			if !ok {
				return &Problem{List: x.name, Index: i, Value: v}
			}
			out = append(out, s)
		}
		slices.Sort(out)
		*x.l = slices.Compact(out)
	}
	return nil
}

func canonPorts(s string) (string, bool) {
	lo, hi, isRange := strings.Cut(s, "-")
	a, errA := strconv.Atoi(strings.TrimSpace(lo))
	b := a
	var errB error
	if isRange {
		b, errB = strconv.Atoi(strings.TrimSpace(hi))
	}
	if errA != nil || errB != nil || a < 1 || b > 65535 || a > b {
		return "", false
	}
	if a == b {
		return strconv.Itoa(a), true
	}
	return strconv.Itoa(a) + "-" + strconv.Itoa(b), true
}

func canonNetwork(s string) (string, bool) {
	if p, err := netip.ParsePrefix(s); err == nil {
		return p.Masked().String(), true
	}
	if a, err := netip.ParseAddr(s); err == nil && a.Zone() == "" {
		a = a.Unmap()
		return netip.PrefixFrom(a, a.BitLen()).String(), true
	}
	return "", false
}

var domainRe = regexp.MustCompile(`^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)*[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

// canonDomain takes "example.com", ".example.com" or "*.example.com": the domain and its
// subdomains.
func canonDomain(s string) (string, bool) {
	s = strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(s, "*."), "."), "."))
	return s, len(s) <= 253 && domainRe.MatchString(s)
}

// State is what the nodes get: nil when neither filter is on or has anything to do.
func (c Config) State() *nodeapi.Filters {
	f := &nodeapi.Filters{}
	if c.Egress.Enabled {
		ports := slices.Clone(c.Egress.Ports)
		if c.Egress.Mail {
			ports = append(ports, MailPorts...)
		}
		slices.Sort(ports)
		f.Egress = nodeapi.Egress{Ports: slices.Compact(ports), Networks: c.Egress.Networks, Domains: c.Egress.Domains}
	}
	if c.Ingress.Enabled && len(c.Ingress.Networks) > 0 {
		f.Ingress = nodeapi.Ingress{Allow: c.Ingress.Allow, Networks: c.Ingress.Networks}
	}
	if len(f.Egress.Ports)+len(f.Egress.Networks)+len(f.Egress.Domains)+len(f.Ingress.Networks) == 0 {
		return nil
	}
	return f
}
