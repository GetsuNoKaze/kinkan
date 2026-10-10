package api

import (
	"context"
	"net/http"
	"net/netip"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"mikan/internal/nodeapi"
	"mikan/internal/panel/domain"
	"mikan/internal/panel/settings"
	"mikan/internal/proto"
)

type scannerInput struct {
	ID int64 `path:"id" minimum:"1"`
}
type ScannerDay struct {
	Day     int64 `json:"day"`
	Count   int64 `json:"count"`
	Clients int64 `json:"clients"`
	Own     int64 `json:"own" doc:"Проверки самой панели"`
}
type ScannerView struct {
	Records  []nodeapi.ScannerRecord `json:"records"`
	Days     []ScannerDay            `json:"days"`
	Spike    bool                    `json:"spike"`
	Coverage string                  `json:"coverage"` // explicit about what the core currently observes
}
type scannerOutput struct{ Body ScannerView }

func (h *handlers) registerScanners() {
	huma.Register(h.api, huma.Operation{OperationID: "node-scanners", Method: http.MethodGet, Path: "/api/v1/nodes/{id}/scanners", Tags: []string{"node"}, Summary: "Журнал неудачных обращений", Metadata: sessionOnly, Extensions: sessionOnlyExt}, h.scannersOf)
}
func (h *handlers) scannersOf(ctx context.Context, in *scannerInput) (*scannerOutput, error) {
	if _, err := h.nodeOf(ctx, in.ID); err != nil {
		return nil, err
	}
	now := h.d.Now()
	records, err := h.d.Store.ScannerRows(ctx, in.ID, now)
	if err != nil {
		return nil, err
	}
	own := h.panelAddrs(ctx)
	for i := range records {
		r := &records[i]
		ip, err := netip.ParseAddr(r.IP)
		if err != nil {
			continue
		}
		ip = ip.Unmap()
		// Whatever the node says, only the panel knows its own address.
		r.Own = own[ip]
		if h.d.GeoIP != nil && (r.Country == "" || r.ASN == "") && proto.PublicAddr(ip) {
			info := h.d.GeoIP.Lookup(ip)
			if r.Country == "" {
				r.Country = info.Country
			}
			if r.ASN == "" && info.ASN != "" {
				r.ASN, r.Organization = info.ASN, info.Organization
			}
		}
	}
	byDay := map[int64]*ScannerDay{}
	for _, r := range records {
		d := byDay[r.Day]
		if d == nil {
			d = &ScannerDay{Day: r.Day}
			byDay[r.Day] = d
		}
		switch {
		case r.Own:
			d.Own += r.Count
		case r.Client:
			d.Clients += r.Count
		default:
			d.Count += r.Count
		}
	}
	today := now.UTC().Truncate(24 * time.Hour).Unix()
	days := make([]ScannerDay, 0, 30)
	var prior int64
	for i := 29; i >= 0; i-- {
		day := today - int64(i)*86400
		d := byDay[day]
		if d == nil {
			d = &ScannerDay{Day: day}
		}
		days = append(days, *d)
		if i >= 1 && i <= 7 {
			prior += d.Count
		}
	}
	current := days[len(days)-1].Count
	// The panel's own checks (Nodes → the check) come from its address and are not
	// scanners: they stay out of Count and so out of the spike.
	return &scannerOutput{Body: ScannerView{Records: records, Days: days, Spike: current >= 100 && current > 3*max(1, prior/7), Coverage: "TrustTunnel unauthenticated HTTP; REALITY rejected handshakes; TUIC timeout/wrong auth; Hysteria2 rejected /auth; AnyTLS wrong auth. Ordinary Hysteria2 HTTP, pre-QUIC/obfs failures and other protocols are not recorded yet."}}, nil
}

// panelAddrs are the panel's public addresses: what its domain and public host lead to.
// The panel's checks of its nodes come from there.
func (h *handlers) panelAddrs(ctx context.Context) map[netip.Addr]bool {
	out := map[netip.Addr]bool{}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	for _, key := range []string{settings.KeyDomain, settings.KeyPublicHost} {
		host, err := h.d.Settings.String(ctx, key)
		if err != nil || host == "" {
			continue
		}
		if ip, err := netip.ParseAddr(host); err == nil {
			out[ip.Unmap()] = true
			continue
		}
		resolve := h.d.Resolve
		if resolve == nil {
			resolve = domain.SystemResolve
		}
		addrs, _ := resolve(ctx, host)
		for _, ip := range addrs {
			out[ip.Unmap()] = true
		}
	}
	return out
}
