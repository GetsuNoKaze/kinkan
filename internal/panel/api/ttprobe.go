package api

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"mikan/internal/panel/domain"
	"mikan/internal/panel/settings"
	"mikan/internal/proto"
	"mikan/internal/ttprobe"
)

type ttProbeInput struct {
	ID   int64 `path:"id" minimum:"1"`
	Body struct {
		InboundID     int64 `json:"inbound_id" minimum:"1"`
		ReferencePort int   `json:"reference_port,omitempty" minimum:"0" maximum:"65535" doc:"HTTPS cover-сайт на той же ноде; 0 — без сравнения"`
	}
}

type TTProbeView struct {
	StartedAt time.Time     `json:"started_at"`
	NodeID    int64         `json:"node_id"`
	InboundID int64         `json:"inbound_id"`
	Vantage   string        `json:"vantage" enum:"panel" doc:"Запросы идут с сервера панели, а не из браузера или сети клиента"`
	Address   string        `json:"address" doc:"IP, по которому шли все запросы проверки"`
	LocalNode bool          `json:"local_node" doc:"Нода на сервере панели: панель проверяет свой же адрес снаружи"`
	Report    TTProbeReport `json:"report"`
}

// Keep the API schema name distinct from panelimport.Report.
type TTProbeReport ttprobe.Report
type ttProbeOutput struct{ Body TTProbeView }

// One bounded, sequential scan per panel process, also when several admins click.
var ttProbing sync.Mutex

func (h *handlers) registerTTProbe() {
	huma.Register(h.api, huma.Operation{OperationID: "probe-node-trusttunnel", Method: http.MethodPost, Path: "/api/v1/nodes/{id}/trusttunnel-probe", Tags: []string{"node"}, Summary: "Проверить TrustTunnel глазами сканера", Metadata: sessionOnly, Extensions: sessionOnlyExt}, h.probeTrustTunnel)
}

func (h *handlers) probeTrustTunnel(ctx context.Context, in *ttProbeInput) (*ttProbeOutput, error) {
	if !ttProbing.TryLock() {
		return nil, huma.Error409Conflict("probe_busy")
	}
	defer ttProbing.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 75*time.Second)
	defer cancel()
	node, err := h.nodeOf(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	ib, err := h.d.Store.Q.GetInbound(ctx, in.Body.InboundID)
	if errors.Is(err, sql.ErrNoRows) || err == nil && ib.NodeID != node.ID {
		return nil, huma.Error404NotFound("not_found")
	}
	if err != nil {
		return nil, err
	}
	template, err := proto.Parse(ib.Config)
	if err != nil || template.Type() != "trusttunnel" || ib.Enabled == 0 || node.Enabled == 0 {
		return nil, huma.Error422UnprocessableEntity("probe_not_trusttunnel")
	}
	port, err := strconv.Atoi(ib.Port)
	if err != nil || port < 1 || port > 65535 {
		return nil, huma.Error422UnprocessableEntity("probe_bad_port")
	}
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
	dialer, ip, err := publicProbeDialer(resolveCtx, host, h.d.Resolve)
	stop()
	if err != nil {
		return nil, huma.Error422UnprocessableEntity("probe_public_host")
	}
	cfg := ttprobe.Config{Target: "https://" + net.JoinHostPort(host, strconv.Itoa(port)), Timeout: 2 * time.Second, DialContext: dialer}
	if in.Body.ReferencePort != 0 {
		cfg.Reference = "https://" + net.JoinHostPort(host, strconv.Itoa(in.Body.ReferencePort))
	}
	started := h.d.Now()
	h.audit(ctx, sessionOf(ctx).AdminID, "node.trusttunnel_probe", "node", strconv.FormatInt(node.ID, 10), map[string]any{"inbound_id": ib.ID, "reference_port": in.Body.ReferencePort})
	scan := h.d.TTProbeScan
	if scan == nil {
		scan = ttprobe.Scan
	}
	report, err := scan(ctx, cfg)
	if err != nil {
		report.Findings = append(report.Findings, ttprobe.Finding{Name: "scan", Level: "ERROR", Detail: err.Error()})
	}
	return &ttProbeOutput{Body: TTProbeView{StartedAt: started, NodeID: node.ID, InboundID: ib.ID, Vantage: "panel", Address: ip.String(), LocalNode: node.Address == "", Report: TTProbeReport(report)}}, nil
}

// Resolve once, reject every non-public answer, then pin the IP for ALL probes.
// Neither the target nor the reference accepts arbitrary hosts from the request.
func publicProbeDialer(ctx context.Context, host string, resolve func(context.Context, string) ([]netip.Addr, error)) (func(context.Context, string, string) (net.Conn, error), netip.Addr, error) {
	if !proto.PublicHost(host) {
		return nil, netip.Addr{}, errors.New("not a public host")
	}
	if resolve == nil {
		resolve = func(ctx context.Context, host string) ([]netip.Addr, error) {
			return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		}
	}
	addrs, err := resolve(ctx, host)
	if err != nil || len(addrs) == 0 {
		return nil, netip.Addr{}, errors.New("no public address")
	}
	for _, ip := range addrs {
		if !ip.IsValid() || !proto.PublicAddr(ip) {
			return nil, netip.Addr{}, errors.New("non-public DNS answer")
		}
	}
	ip := addrs[0].Unmap()
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		name, port, err := net.SplitHostPort(addr)
		if err != nil || name != host {
			return nil, errors.New("unexpected probe host")
		}
		return (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
	}, ip, nil
}
