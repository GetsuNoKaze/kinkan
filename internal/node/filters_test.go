package node

import (
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"

	C "github.com/metacubex/mihomo/constant"

	"mikan/internal/nodeapi"
)

func TestEgressRules(t *testing.T) {
	st := nodeapi.DesiredState{
		Filters: &nodeapi.Filters{Egress: nodeapi.Egress{
			Ports:    []string{"465", "1000-2000", "0", "70000", "9-8", "x", "587,REJECT"},
			Networks: []string{"203.0.113.7/24", "198.51.100.9", "2001:db8::1/32", "nope"},
			Domains:  []string{"*.Mail.Example", ".spam.example", "bad,REJECT", ""},
		}},
		Warp: &nodeapi.Warp{Domains: []string{"openai.com"}},
	}
	got := strings.Join(rules(st, true), "\n")
	want := strings.Join([]string{
		"DST-PORT,25,REJECT",
		"DST-PORT,465,REJECT",
		"DST-PORT,1000-2000,REJECT",
		"IP-CIDR,203.0.113.0/24,REJECT",
		"IP-CIDR,198.51.100.9/32,REJECT",
		"IP-CIDR6,2001:db8::/32,REJECT",
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
