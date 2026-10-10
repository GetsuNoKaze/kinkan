package api

import (
	"context"
	"github.com/danielgtaylor/huma/v2"
	"mikan/internal/nodeapi"
	"net/http"
	"time"
)

type scannerInput struct {
	ID int64 `path:"id" minimum:"1"`
}
type ScannerDay struct {
	Day     int64 `json:"day"`
	Count   int64 `json:"count"`
	Clients int64 `json:"clients"`
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
	byDay := map[int64]*ScannerDay{}
	for _, r := range records {
		d := byDay[r.Day]
		if d == nil {
			d = &ScannerDay{Day: r.Day}
			byDay[r.Day] = d
		}
		if r.Client {
			d.Clients += r.Count
		} else {
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
	return &scannerOutput{Body: ScannerView{Records: records, Days: days, Spike: current >= 100 && current > 3*max(1, prior/7), Coverage: "TrustTunnel unauthenticated HTTP; REALITY rejected handshakes; TUIC timeout/wrong auth; Hysteria2 rejected /auth; AnyTLS wrong auth. Ordinary Hysteria2 HTTP, pre-QUIC/obfs failures and other protocols are not recorded yet."}}, nil
}
