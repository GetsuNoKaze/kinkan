package api

import (
	"context"
	"net"
	"net/netip"
	"strconv"
	"time"

	"mikan/internal/nodeapi"
	"mikan/internal/panel/domain"
	"mikan/internal/proto"
)

// Kinkan: the quiet node (ROADMAP). The protocol check says how each inbound answers a
// stranger; this turns it into what the node as a whole looks like and what to do about
// each inbound that gives it away. A scanner that recognises one port has the node's
// address, and a block of the address takes every protocol on it: the node is as quiet as
// its loudest inbound.

// QuietView is the node's verdict and the advice, worst first.
type QuietView struct {
	Verdict string        `json:"verdict" enum:"quiet,noticeable,exposed,inconclusive" doc:"Нода не тише самого заметного подключения"`
	Advice  []QuietAdvice `json:"advice"`
}

// QuietAdvice is one inbound that keeps the node from being quiet, why, and what helps.
type QuietAdvice struct {
	InboundID int64  `json:"inbound_id"`
	Name      string `json:"name"`
	Level     string `json:"level" enum:"exposed,noticeable,inconclusive"`
	Code      string `json:"code" enum:"tuic_auth,quic_obfs,quic_auth,tt_site,tt_differs,reality_site,reality_differs,reality_far,exposed,noticeable,no_reference,obfuscated,incomplete" doc:"Что не так; текст — quietNode.advice.<code>"`
	Action    string `json:"action" enum:"disable,site,reference,none" doc:"Что предложить: выключить подключение, дать ноде сайт, повторить с эталоном или ничего"`
	// Params fill the advice's text: for reality_far the target and both networks.
	Params map[string]string `json:"params,omitempty"`
}

type quietItem struct {
	view     InboundProbeView
	template proto.Template // nil when it does not parse
	far      *farTarget     // a REALITY target in another network than the node's
}

// farTarget is a REALITY target whose address is in another network (AS) than the node:
// a big site's certificate on a hosting provider's address is a known sign of REALITY.
type farTarget struct {
	Host, TargetAS, TargetOrg, NodeAS, NodeOrg string
}

// verdictRank orders verdicts from quiet to exposed: an unknown answer is worse than a
// quiet one, a difference worse than an unknown, a giveaway the worst.
func verdictRank(v string) int {
	switch v {
	case "quiet":
		return 0
	case "inconclusive":
		return 1
	case "noticeable":
		return 2
	case "exposed":
		return 3
	}
	return 1
}

// adviseQuiet makes the node's verdict and the advice. siteServed: the node serves its
// site now, so TrustTunnel without a fallback of its own and REALITY on the panel's
// self-steal already point at it (nodesync.fitToSite).
func adviseQuiet(items []quietItem, siteServed bool) QuietView {
	out := QuietView{Verdict: "quiet", Advice: []QuietAdvice{}}
	for _, it := range items {
		r := it.view.Report
		if verdictRank(r.Verdict) > verdictRank(out.Verdict) {
			out.Verdict = r.Verdict
		}
		if f := it.far; f != nil {
			if verdictRank("noticeable") > verdictRank(out.Verdict) {
				out.Verdict = "noticeable"
			}
			out.Advice = append(out.Advice, QuietAdvice{InboundID: it.view.InboundID, Name: it.view.Name, Level: "noticeable", Code: "reality_far", Action: "none",
				Params: map[string]string{"target": f.Host, "target_as": f.TargetAS, "target_org": f.TargetOrg, "node_as": f.NodeAS, "node_org": f.NodeOrg}})
		}
		if r.Verdict == "quiet" {
			continue
		}
		a := QuietAdvice{InboundID: it.view.InboundID, Name: it.view.Name, Level: r.Verdict, Action: "none"}
		a.Code, a.Action = adviceFor(r, it.template, siteServed)
		out.Advice = append(out.Advice, a)
	}
	// Worst first; the order of the inbounds otherwise.
	for i := 1; i < len(out.Advice); i++ {
		for j := i; j > 0 && verdictRank(out.Advice[j].Level) > verdictRank(out.Advice[j-1].Level); j-- {
			out.Advice[j], out.Advice[j-1] = out.Advice[j-1], out.Advice[j]
		}
	}
	return out
}

func adviceFor(r ProtocolProbeReport, t proto.Template, siteServed bool) (code, action string) {
	obfuscated := false
	if t != nil {
		obfs, _ := t["obfs"].(string)
		obfuscated = obfs != ""
	}
	if r.Verdict == "inconclusive" {
		switch {
		case obfuscated:
			// Silence is what obfuscation is for; no check from outside can say more.
			return "obfuscated", "none"
		case r.Reference == "" && r.Protocol != "tuic" && r.Protocol != "hysteria2":
			return "no_reference", "reference"
		}
		return "incomplete", "none"
	}
	switch r.Protocol {
	case "tuic":
		if r.Verdict == "exposed" {
			// TUIC answers a wrong password with its own close code: no setting hides it.
			return "tuic_auth", "disable"
		}
	case "hysteria2":
		if !obfuscated {
			// Salamander (obfs) keeps the port silent to whoever lacks the password.
			return "quic_obfs", "none"
		}
		if r.Verdict == "exposed" {
			return "quic_auth", "disable"
		}
	case "trusttunnel":
		if fallback, _ := t["fallback"].(string); fallback == "" && !siteServed {
			return "tt_site", "site"
		}
		return "tt_differs", "none"
	}
	if t != nil {
		if reality, ok := t["reality-config"].(map[string]any); ok {
			dest, _ := reality["dest"].(string)
			if selfSteal(dest) && !siteServed {
				return "reality_site", "site"
			}
			return "reality_differs", "none"
		}
	}
	if r.Verdict == "exposed" {
		return "exposed", "disable"
	}
	return "noticeable", "none"
}

// selfSteal says whether a REALITY dest is the panel's own HTTPS on loopback rather than
// the node's site (nodeapi.SiteHTTPSPort) or a website elsewhere.
func selfSteal(dest string) bool {
	host, port, err := net.SplitHostPort(dest)
	if err != nil || (host != "127.0.0.1" && host != "localhost") {
		return false
	}
	return port != strconv.Itoa(nodeapi.SiteHTTPSPort)
}

// siteServedBy says whether the node serves its site now, as its last health says.
func (h *handlers) siteServedBy(nodeID int64) bool {
	if h.d.Nodes == nil {
		return false
	}
	hv, ok := h.d.Nodes.Health(nodeID)
	if !ok || !hv.OK || hv.Health.Site == nil {
		return false
	}
	s := hv.Health.Site
	return s.Error == "" && (s.HTTP != "" || s.HTTPS != "")
}

// farTargetOf looks up whether an inbound's REALITY target lies in another network than
// the node (nodeIP). nil when it does not, when either network is not known (no GeoIP
// database yet) or the target is the node's own (loopback).
func (h *handlers) farTargetOf(ctx context.Context, t proto.Template, nodeIP netip.Addr) *farTarget {
	if h.d.GeoIP == nil || t == nil {
		return nil
	}
	reality, ok := t["reality-config"].(map[string]any)
	if !ok {
		return nil
	}
	dest, _ := reality["dest"].(string)
	host, _, err := net.SplitHostPort(dest)
	if err != nil || host == "127.0.0.1" || host == "localhost" {
		return nil
	}
	var ip netip.Addr
	if a, err := netip.ParseAddr(host); err == nil {
		ip = a
	} else {
		resolve := h.d.Resolve
		if resolve == nil {
			resolve = domain.SystemResolve
		}
		lookup, cancel := context.WithTimeout(ctx, 3*time.Second)
		addrs, err := resolve(lookup, host)
		cancel()
		if err != nil || len(addrs) == 0 {
			return nil
		}
		ip = addrs[0]
	}
	if !proto.PublicAddr(ip.Unmap()) {
		return nil
	}
	target, node := h.d.GeoIP.Lookup(ip.Unmap()), h.d.GeoIP.Lookup(nodeIP.Unmap())
	if target.ASN == "" || node.ASN == "" || target.ASN == node.ASN {
		return nil
	}
	return &farTarget{Host: host, TargetAS: target.ASN, TargetOrg: target.Organization, NodeAS: node.ASN, NodeOrg: node.Organization}
}
