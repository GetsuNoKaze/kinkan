package node

import (
	"encoding/json"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"

	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/hub/executor"

	"mikan/internal/nodeapi"
	"mikan/internal/proto"
)

func TestEgressRules(t *testing.T) {
	st := nodeapi.DesiredState{
		Filters: &nodeapi.Filters{Egress: nodeapi.Egress{
			// "+2525" and "-1" pass strconv.Atoi, yet mihomo refuses the whole config over
			// them; " 1194 " is still a port.
			Ports:    []string{"465", "1000-2000", "0", "70000", "9-8", "x", "587,REJECT", "+2525", "-1", "1-+5", " 1194 "},
			Networks: []string{"203.0.113.7/24", "198.51.100.9", "2001:db8::1/32", "nope", "::ffff:192.0.2.0/120", "fe80::1%eth0"},
			Domains:  []string{"*.Mail.Example", ".spam.example", "bad,REJECT", "", "1.2.3.4", "bad)", "ex ample.com"},
		}},
		Warp: &nodeapi.Warp{Domains: []string{"openai.com"}},
	}
	got := strings.Join(rules(st, true), "\n")
	want := strings.Join([]string{
		"DST-PORT,25,REJECT",
		"DST-PORT,465,REJECT",
		"DST-PORT,1000-2000,REJECT",
		"DST-PORT,1194,REJECT",
		"IP-CIDR,203.0.113.0/24,REJECT",
		"IP-CIDR,198.51.100.9/32,REJECT",
		"IP-CIDR6,2001:db8::/32,REJECT",
		"IP-CIDR,192.0.2.0/24,REJECT",
		"DOMAIN-SUFFIX,mail.example,REJECT",
		"DOMAIN-SUFFIX,spam.example,REJECT",
		"DOMAIN-SUFFIX,openai.com,WARP",
		"MATCH,DIRECT",
	}, "\n")
	if got != want {
		t.Fatalf("rules:\n%s\nwant:\n%s", got, want)
	}
	// The rules are part of what swaps the tunnel's routes.
	if routesKey(st, true) == routesKey(nodeapi.DesiredState{Warp: st.Warp}, true) {
		t.Fatal("the egress filter does not change the routes key")
	}
	if strings.Join(rules(nodeapi.DesiredState{}, true), "\n") != "DST-PORT,25,REJECT\nMATCH,DIRECT" {
		t.Fatal("no filters, yet rules changed")
	}
}

// mihomo's own parser takes a config with every kind of filter entry, the junk the node
// leaves out included: one rule it cannot parse would cost the node the whole state.
func TestFiltersConfigParses(t *testing.T) {
	for _, f := range []*nodeapi.Filters{
		nil,
		{},
		{
			Egress: nodeapi.Egress{
				Ports:    []string{"465", "587", "2525", "6881-6889", "+25", "-1", "65536", "1-+5", "x"},
				Networks: []string{"203.0.113.0/24", "198.51.100.7", "2001:db8::/32", "::ffff:192.0.2.0/120", "junk", "10.0.0.0/33"},
				Domains:  []string{"example.com", "*.mail.example", "пример.рф", "a,b", "bad)", "#x", "1.2.3.4"},
			},
			Ingress: nodeapi.Ingress{Allow: true, Networks: []string{"203.0.113.0/24", "junk"}},
		},
	} {
		st := cascadeState(t)
		st.Filters = f
		raw, _, err := buildConfig(st, proto.Cert{CertPath: "/tmp/c.pem", KeyPath: "/tmp/k.pem"}, false)
		if err != nil {
			t.Fatal(err)
		}
		var cfg struct {
			Rules []string `json:"rules"`
		}
		if err := json.Unmarshal(raw, &cfg); err != nil {
			t.Fatal(err)
		}
		parsed, err := executor.ParseWithBytes(raw)
		if err != nil {
			t.Fatalf("mihomo refuses the config with %+v: %v", f, err)
		}
		if len(parsed.Rules) != len(cfg.Rules) {
			t.Fatalf("parsed %d rules of %d", len(parsed.Rules), len(cfg.Rules))
		}
		if f == nil || len(f.Egress.Ports) == 0 {
			continue
		}
		got := strings.Join(cfg.Rules, "\n")
		// Only what parses, after the node's own REJECT rules, before the cascade and WARP.
		at := strings.Index(got, "DST-PORT,6881-6889,REJECT")
		if at < 0 || at < strings.Index(got, "IP-CIDR,127.0.0.0/8,REJECT") || at > strings.Index(got, "IN-NAME,") || at > strings.Index(got, ",WARP") {
			t.Fatalf("order:\n%s", got)
		}
		n := 0
		for _, r := range cfg.Rules {
			if strings.HasSuffix(r, ",REJECT") {
				n++
			}
		}
		// The node's own: 11 private ranges and port 25; the filter's: 4 ports, 4
		// networks, 3 domains.
		if n != len(privateRules)+1+11 || !strings.Contains(got, "DOMAIN-SUFFIX,xn--e1afmkfd.xn--p1ai,REJECT") {
			t.Fatalf("REJECT rules: %d\n%s", n, got)
		}
	}
}

func TestIngress(t *testing.T) {
	r := NewRegistry("e", 0, time.Minute, time.Now)
	in, out := netip.MustParseAddr("203.0.113.5"), netip.MustParseAddr("198.51.100.5")
	mapped := netip.MustParseAddr("::ffff:203.0.113.5")
	if !r.admitFrom(in) || !r.admitFrom(out) {
		t.Fatal("no filter, yet refused")
	}

	r.SetIngress(&nodeapi.Filters{Ingress: nodeapi.Ingress{Networks: []string{"203.0.113.0/24", "junk"}}})
	if r.admitFrom(in) || r.admitFrom(mapped) || !r.admitFrom(out) {
		t.Fatal("the deny list")
	}

	r.SetIngress(&nodeapi.Filters{Ingress: nodeapi.Ingress{Allow: true, Networks: []string{"203.0.113.0/24"}}})
	if !r.admitFrom(in) || !r.admitFrom(mapped) || r.admitFrom(out) {
		t.Fatal("the allow list")
	}

	// An allow list where nothing parses would shut everyone out: it is no filter.
	r.SetIngress(&nodeapi.Filters{Ingress: nodeapi.Ingress{Allow: true, Networks: []string{"junk"}}})
	if !r.admitFrom(out) {
		t.Fatal("an empty allow list shut users out")
	}
	r.SetIngress(nil)
	if !r.admitFrom(in) {
		t.Fatal("off, yet refused")
	}
}

// The ingress filter turns a connection away before its user is looked at; another node
// of the panel relaying its users is never turned away.
func TestIngressInTunnel(t *testing.T) {
	inner := &recordingTunnel{}
	reg := NewRegistry("e", 0, time.Minute, time.Now)
	reg.SetIngress(&nodeapi.Filters{Ingress: nodeapi.Ingress{Networks: []string{"198.51.100.0/24"}}})
	tun := &Tunnel{inner: inner, reg: reg}
	a, b := net.Pipe()
	defer b.Close()
	tun.HandleTCPConn(a, &C.Metadata{Type: C.VLESS, InName: nodeapi.RelayListener, InUser: "relay-2", SrcIP: mustAddr("198.51.100.20")})
	if inner.tcp.Load() != 1 {
		t.Fatal("the relay was turned away")
	}
	c, d := net.Pipe()
	defer d.Close()
	tun.HandleTCPConn(c, &C.Metadata{Type: C.VLESS, InName: "in-vless", InUser: "u1", SrcIP: mustAddr("198.51.100.21")})
	if inner.tcp.Load() != 1 {
		t.Fatal("a connection from a denied network reached mihomo")
	}
	if _, err := d.Write([]byte("x")); err == nil {
		t.Fatal("the denied connection is still open")
	}
}
