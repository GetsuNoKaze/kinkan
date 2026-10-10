package api

import (
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
