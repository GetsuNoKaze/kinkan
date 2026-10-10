package api

import (
	"context"
	"errors"
	"net/netip"
	"testing"

	"mikan/internal/proto"
)

func quietCase(id int64, protocol, verdict, reference string, t proto.Template) quietItem {
	return quietItem{view: InboundProbeView{InboundID: id, Name: protocol, Report: ProtocolProbeReport{Protocol: protocol, Verdict: verdict, Reference: reference}}, template: t}
}

// The node is as quiet as its loudest inbound, and each inbound that is not quiet gets
// the advice that fits it, worst first.
func TestQuietNodeAdvice(t *testing.T) {
	reality := func(dest string) proto.Template {
		return proto.Template{"type": "vless", "reality-config": map[string]any{"dest": dest}}
	}
	items := []quietItem{
		quietCase(1, "vless", "quiet", "https://n:443", reality("127.0.0.1:17443")),
		quietCase(2, "trusttunnel", "noticeable", "https://n:443", proto.Template{"type": "trusttunnel"}),
		quietCase(3, "tuic", "exposed", "", proto.Template{"type": "tuic"}),
		quietCase(4, "hysteria2", "noticeable", "", proto.Template{"type": "hysteria2"}),
		quietCase(5, "hysteria2", "inconclusive", "", proto.Template{"type": "hysteria2", "obfs": "salamander"}),
		quietCase(6, "vless", "noticeable", "https://n:443", reality("127.0.0.1:21973")),
		quietCase(7, "anytls", "inconclusive", "", proto.Template{"type": "anytls"}),
		quietCase(8, "trojan", "exposed", "https://n:443", nil),
	}
	got := adviseQuiet(items, false)
	if got.Verdict != "exposed" {
		t.Errorf("node verdict %q, want exposed: the loudest inbound decides", got.Verdict)
	}
	want := []struct {
		id           int64
		code, action string
	}{
		{3, "tuic_auth", "disable"}, {8, "exposed", "disable"},
		{2, "tt_site", "site"}, {4, "quic_obfs", "none"}, {6, "reality_site", "site"},
		{5, "obfuscated", "none"}, {7, "no_reference", "reference"},
	}
	if len(got.Advice) != len(want) {
		t.Fatalf("advice %+v", got.Advice)
	}
	for i, w := range want {
		a := got.Advice[i]
		if a.InboundID != w.id || a.Code != w.code || a.Action != w.action {
			t.Errorf("advice %d = %+v, want inbound %d %s/%s", i, a, w.id, w.code, w.action)
		}
	}

	// Once the node serves its site, TrustTunnel and the panel's self-steal already point
	// at it: what is left to see is in the findings.
	served := adviseQuiet(items, true)
	for _, a := range served.Advice {
		if a.Action == "site" {
			t.Errorf("site advice while the site is served: %+v", a)
		}
	}
	// TrustTunnel with a fallback of its own needs no site either.
	own := adviseQuiet([]quietItem{quietCase(9, "trusttunnel", "noticeable", "https://n:443", proto.Template{"type": "trusttunnel", "fallback": "127.0.0.1:8080"})}, false)
	if own.Advice[0].Code != "tt_differs" {
		t.Errorf("own fallback: %+v", own.Advice[0])
	}
	if q := adviseQuiet([]quietItem{quietCase(1, "vless", "quiet", "https://n:443", nil)}, false); q.Verdict != "quiet" || len(q.Advice) != 0 {
		t.Errorf("a quiet node: %+v", q)
	}
}

// A REALITY target in another network than the node makes the node noticeable even when
// the target answers well: a big site's certificate on a hosting address stands out.
func TestQuietNodeFarRealityTarget(t *testing.T) {
	ctx := context.Background()
	node := netip.MustParseAddr("198.51.100.10")
	geo := fakeGeo{
		node:                                 {ASN: "64500", Organization: "Hosting"},
		netip.MustParseAddr("203.0.113.80"):  {ASN: "8075", Organization: "Microsoft"},
		netip.MustParseAddr("198.51.100.99"): {ASN: "64500", Organization: "Hosting"},
	}
	h := &handlers{d: Deps{KinkanDeps: KinkanDeps{GeoIP: geo}, Resolve: func(_ context.Context, host string) ([]netip.Addr, error) {
		switch host {
		case "www.microsoft.com":
			return []netip.Addr{netip.MustParseAddr("203.0.113.80")}, nil
		case "neighbour.example":
			return []netip.Addr{netip.MustParseAddr("198.51.100.99")}, nil
		}
		return nil, errors.New("no such host")
	}}}
	reality := func(dest string) proto.Template {
		return proto.Template{"type": "vless", "reality-config": map[string]any{"dest": dest}}
	}
	far := h.farTargetOf(ctx, reality("www.microsoft.com:443"), node)
	if far == nil || far.TargetAS != "8075" || far.NodeAS != "64500" || far.Host != "www.microsoft.com" {
		t.Fatalf("far target = %+v", far)
	}
	for name, tpl := range map[string]proto.Template{
		"neighbour":   reality("neighbour.example:443"),
		"own site":    reality("127.0.0.1:17443"),
		"unresolved":  reality("nowhere.example:443"),
		"not REALITY": {"type": "trusttunnel"},
	} {
		if f := h.farTargetOf(ctx, tpl, node); f != nil {
			t.Errorf("%s: %+v", name, f)
		}
	}
	if f := (&handlers{}).farTargetOf(ctx, reality("www.microsoft.com:443"), node); f != nil {
		t.Errorf("without a GeoIP database: %+v", f)
	}

	q := adviseQuiet([]quietItem{{view: InboundProbeView{InboundID: 1, Name: "vless", Report: ProtocolProbeReport{Protocol: "vless", Verdict: "quiet"}}, template: reality("www.microsoft.com:443"), far: far}}, true)
	if q.Verdict != "noticeable" || len(q.Advice) != 1 || q.Advice[0].Code != "reality_far" || q.Advice[0].Params["target_org"] != "Microsoft" {
		t.Errorf("advice for a far target: %+v", q)
	}
}
