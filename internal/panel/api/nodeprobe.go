package api

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"mikan/internal/nodeprobe"
	"mikan/internal/panel/domain"
	"mikan/internal/panel/settings"
	"mikan/internal/proto"
	"mikan/internal/ttprobe"
)

type nodeProbeInput struct {
	ID   int64 `path:"id" minimum:"1"`
	Body struct {
		InboundID     int64  `json:"inbound_id,omitempty" minimum:"0" doc:"0 scans all enabled inbounds"`
		ReferenceHost string `json:"reference_host,omitempty" maxLength:"253" doc:"TLS name of the cover site on the same node; defaults to the inbound SNI"`
		ReferencePort int    `json:"reference_port,omitempty" minimum:"0" maximum:"65535"`
	}
}

type InboundProbeView struct {
	InboundID int64               `json:"inbound_id"`
	Name      string              `json:"name"`
	Port      string              `json:"port"`
	Network   string              `json:"network"`
	Report    ProtocolProbeReport `json:"report"`
}

type ProtocolProbeReport nodeprobe.Report

type NodeProbeView struct {
	StartedAt time.Time          `json:"started_at"`
	NodeID    int64              `json:"node_id"`
	Vantage   string             `json:"vantage" enum:"panel"`
	Address   string             `json:"address"`
	LocalNode bool               `json:"local_node"`
	Inbounds  []InboundProbeView `json:"inbounds"`
	Quiet     QuietView          `json:"quiet" doc:"Вердикт ноды целиком и что сделать (kinkan_quiet.go)"`
}
type nodeProbeOutput struct{ Body NodeProbeView }

func (h *handlers) registerNodeProbe() {
	huma.Register(h.api, huma.Operation{OperationID: "probe-node-protocols", Method: http.MethodPost, Path: "/api/v1/nodes/{id}/protocol-probe", Tags: []string{"node"}, Summary: "Проверить протоколы глазами сканера", Metadata: sessionOnly, Extensions: sessionOnlyExt}, h.probeNode)
}

func (h *handlers) probeNode(ctx context.Context, in *nodeProbeInput) (*nodeProbeOutput, error) {
	if !ttProbing.TryLock() {
		return nil, huma.Error409Conflict("probe_busy")
	}
	defer ttProbing.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 240*time.Second)
	defer cancel()
	referenceHost := strings.TrimSpace(in.Body.ReferenceHost)
	if referenceHost != "" && (!proto.PublicHost(referenceHost) || strings.ContainsAny(referenceHost, ":/?#@[]%\\ \t\r\n")) {
		return nil, huma.Error422UnprocessableEntity("probe_public_host")
	}
	node, err := h.nodeOf(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	if node.Enabled == 0 {
		return nil, huma.Error422UnprocessableEntity("probe_disabled_node")
	}
	rows, err := h.d.Store.Q.ListInbounds(ctx)
	if err != nil {
		return nil, err
	}
	views := []InboundProbeView{}
	configs := []nodeprobe.Config{}
	templates := []proto.Template{}
	host := domain.NodeHost(node)
	if node.Address == "" {
		host, err = h.d.Settings.String(ctx, settings.KeyDomain)
		if err != nil {
			return nil, err
		}
		if host == "" {
			host, err = h.d.Settings.String(ctx, settings.KeyPublicHost)
			if err != nil {
				return nil, err
			}
		}
	}
	resolveCtx, stop := context.WithTimeout(ctx, 5*time.Second)
	_, ip, err := publicProbeDialer(resolveCtx, host, h.d.Resolve)
	stop()
	if err != nil {
		return nil, huma.Error422UnprocessableEntity("probe_public_host")
	}
	for _, ib := range rows {
		if ib.NodeID != node.ID || ib.Enabled == 0 || (in.Body.InboundID != 0 && ib.ID != in.Body.InboundID) {
			continue
		}
		template, err := proto.Parse(ib.Config)
		cfg := nodeprobe.Config{Timeout: 2 * time.Second, Address: ip.String()}
		view := InboundProbeView{InboundID: ib.ID, Name: ib.Name, Port: ib.Port}
		if err != nil {
			cfg.Protocol = "invalid"
		} else {
			cfg.Protocol = template.Type()
			view.Network = template.Network()
		}
		// Ranges remain explicitly incomplete; never silently scan a different port.
		port := ib.Port
		if _, err := strconv.Atoi(port); err != nil {
			cfg.Protocol = "unsupported-port-range"
			port = "443"
		}
		sni := host
		if ext := template.Ext(); ext.Client.SNI != "" {
			sni = ext.Client.SNI
		}
		if reality, ok := template["reality-config"].(map[string]any); ok {
			switch names := reality["server-names"].(type) {
			case []any:
				if len(names) > 0 {
					if name, ok := names[0].(string); ok {
						sni = name
					}
				}
			case []string:
				if len(names) > 0 {
					sni = names[0]
				}
			case string:
				sni = names
			}
		}
		// SNI/Host may name a REALITY cover elsewhere; all packets still go to
		// the validated node IP, never the cover's destination or client's server.
		cfg.Target = "https://" + net.JoinHostPort(sni, port)
		if in.Body.ReferencePort != 0 {
			refName := sni
			if referenceHost != "" {
				refName = referenceHost
			}
			cfg.Reference = "https://" + net.JoinHostPort(refName, strconv.Itoa(in.Body.ReferencePort))
		}
		if obfs, ok := template["obfs"].(string); ok && obfs != "" {
			cfg.Obfuscated = true
		}
		switch values := template["alpn"].(type) {
		case []any:
			for _, v := range values {
				if value, ok := v.(string); ok {
					cfg.ALPN = append(cfg.ALPN, value)
				}
			}
		case []string:
			cfg.ALPN = values
		}
		views = append(views, view)
		configs = append(configs, cfg)
		if err != nil {
			template = nil
		}
		templates = append(templates, template)
	}
	if len(views) == 0 {
		return nil, huma.Error404NotFound("probe_no_inbounds")
	}
	if len(views) > 64 {
		return nil, huma.Error422UnprocessableEntity("probe_too_many_inbounds")
	}
	started := h.d.Now()
	h.audit(ctx, sessionOf(ctx).AdminID, "node.protocol_probe", "node", strconv.FormatInt(node.ID, 10), map[string]any{"inbound_id": in.Body.InboundID, "reference_port": in.Body.ReferencePort})
	scan := h.d.NodeProbeScan
	if scan == nil {
		scan = nodeprobe.Scan
	}
	for i, cfg := range configs {
		part, done := context.WithTimeout(ctx, 75*time.Second)
		report, err := scan(part, cfg)
		done()
		if err != nil {
			if report.Protocol == "" {
				report.Protocol = cfg.Protocol
			}
			if report.Target == "" {
				report.Target = cfg.Target
			}
			if report.Reference == "" {
				report.Reference = cfg.Reference
			}
			report.Findings = append(report.Findings, ttprobe.Finding{Name: "scan", Level: "ERROR", Detail: err.Error()})
		}
		report.Summarize()
		views[i].Report = ProtocolProbeReport(report)
	}
	items := make([]quietItem, len(views))
	for i := range views {
		items[i] = quietItem{view: views[i], template: templates[i]}
	}
	quiet := adviseQuiet(items, h.siteServedBy(node.ID))
	return &nodeProbeOutput{Body: NodeProbeView{StartedAt: started, NodeID: node.ID, Vantage: "panel", Address: ip.String(), LocalNode: node.Address == "", Inbounds: views, Quiet: quiet}}, nil
}
