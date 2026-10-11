package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"mikan/internal/nodeapi"
	"mikan/internal/panel/store"
)

// Kinkan: the history of protocol checks, kept 30 days, and clearing the scanner journal.

type ProbeRunView struct {
	ID        int64     `json:"id"`
	StartedAt time.Time `json:"started_at"`
	InboundID int64     `json:"inbound_id" doc:"0: все подключения ноды"`
	Verdict   string    `json:"verdict" enum:"quiet,noticeable,exposed,inconclusive" doc:"Вердикт ноды целиком"`
	Inbounds  int       `json:"inbounds"`
}

type probeRunsOutput struct{ Body []ProbeRunView }
type probeRunInput struct {
	ID int64 `path:"id" minimum:"1"`
}
type probeRunOutput struct{ Body NodeProbeView }
type clearedOutput struct {
	Body struct {
		Deleted int64 `json:"deleted"`
	}
}

func (h *handlers) registerHistory() {
	huma.Register(h.api, huma.Operation{OperationID: "node-protocol-probes", Method: http.MethodGet, Path: "/api/v1/nodes/{id}/protocol-probes", Tags: []string{"node"}, Summary: "История проверок протоколов", Metadata: sessionOnly, Extensions: sessionOnlyExt}, h.probeRuns)
	huma.Register(h.api, huma.Operation{OperationID: "protocol-probe", Method: http.MethodGet, Path: "/api/v1/protocol-probes/{id}", Tags: []string{"node"}, Summary: "Сохранённая проверка протоколов", Metadata: sessionOnly, Extensions: sessionOnlyExt}, h.probeRun)
	huma.Register(h.api, huma.Operation{OperationID: "delete-protocol-probe", Method: http.MethodDelete, Path: "/api/v1/protocol-probes/{id}", Tags: []string{"node"}, Summary: "Удалить проверку из истории",
		DefaultStatus: http.StatusNoContent, Metadata: sessionOnly, Extensions: sessionOnlyExt}, h.deleteProbeRun)
	huma.Register(h.api, huma.Operation{OperationID: "delete-node-protocol-probes", Method: http.MethodDelete, Path: "/api/v1/nodes/{id}/protocol-probes", Tags: []string{"node"}, Summary: "Очистить историю проверок ноды", Metadata: sessionOnly, Extensions: sessionOnlyExt}, h.deleteProbeRuns)
	huma.Register(h.api, huma.Operation{OperationID: "delete-node-scanners", Method: http.MethodDelete, Path: "/api/v1/nodes/{id}/scanners", Tags: []string{"node"}, Summary: "Очистить журнал сканеров ноды",
		DefaultStatus: http.StatusNoContent, Metadata: sessionOnly, Extensions: sessionOnlyExt}, h.clearScanners)
}

// keepProbeRun stores a finished check; a failure to store is logged, the check stands.
func (h *handlers) keepProbeRun(ctx context.Context, inboundID int64, view NodeProbeView) {
	run := store.ProbeRun{NodeID: view.NodeID, StartedAt: view.StartedAt.Unix(), InboundID: inboundID, Verdict: view.Quiet.Verdict, Inbounds: len(view.Inbounds)}
	if _, err := h.d.Store.AddProbeRun(ctx, run, view); err != nil && h.d.Log != nil {
		h.d.Log.Warn("keep protocol check", "node", view.NodeID, "err", err)
	}
}

func (h *handlers) probeRuns(ctx context.Context, in *nodeIDInput) (*probeRunsOutput, error) {
	if _, err := h.nodeOf(ctx, in.ID); err != nil {
		return nil, err
	}
	runs, err := h.d.Store.ProbeRuns(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	out := &probeRunsOutput{Body: make([]ProbeRunView, 0, len(runs))}
	for _, r := range runs {
		out.Body = append(out.Body, ProbeRunView{ID: r.ID, StartedAt: time.Unix(r.StartedAt, 0).UTC(), InboundID: r.InboundID, Verdict: r.Verdict, Inbounds: r.Inbounds})
	}
	return out, nil
}

func (h *handlers) probeRun(ctx context.Context, in *probeRunInput) (*probeRunOutput, error) {
	_, raw, err := h.d.Store.ProbeRunReport(ctx, in.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, huma.Error404NotFound("probe_run_not_found")
	}
	if err != nil {
		return nil, err
	}
	out := &probeRunOutput{}
	if err := json.Unmarshal(raw, &out.Body); err != nil {
		return nil, err
	}
	return out, nil
}

func (h *handlers) deleteProbeRun(ctx context.Context, in *probeRunInput) (*struct{}, error) {
	run, _, err := h.d.Store.ProbeRunReport(ctx, in.ID)
	if err == nil {
		err = h.d.Store.DeleteProbeRun(ctx, in.ID)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil, huma.Error404NotFound("probe_run_not_found")
	}
	if err != nil {
		return nil, err
	}
	h.audit(ctx, sessionOf(ctx).AdminID, "node.protocol_probe_delete", "node", strconv.FormatInt(run.NodeID, 10), map[string]any{"run": in.ID})
	return nil, nil
}

func (h *handlers) deleteProbeRuns(ctx context.Context, in *nodeIDInput) (*clearedOutput, error) {
	if _, err := h.nodeOf(ctx, in.ID); err != nil {
		return nil, err
	}
	n, err := h.d.Store.DeleteProbeRuns(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	h.audit(ctx, sessionOf(ctx).AdminID, "node.protocol_probes_clear", "node", strconv.FormatInt(in.ID, 10), map[string]any{"deleted": n})
	out := &clearedOutput{}
	out.Body.Deleted = n
	return out, nil
}

// clearScanners empties the node's journal first, under a new epoch, and then the
// panel's copy: cleared only on the panel, the next snapshot would bring the rows back.
func (h *handlers) clearScanners(ctx context.Context, in *nodeIDInput) (*struct{}, error) {
	if _, err := h.nodeOf(ctx, in.ID); err != nil {
		return nil, err
	}
	resetter, ok := h.d.Nodes.(interface {
		ClearScanners(ctx context.Context, id int64) error
	})
	if !ok || h.d.Nodes == nil {
		return nil, huma.Error409Conflict("nodes_disabled")
	}
	if err := resetter.ClearScanners(ctx, in.ID); err != nil {
		if nodeapi.IsScannerUnsupported(err) {
			return nil, huma.Error409Conflict("node_too_old")
		}
		return nil, huma.Error502BadGateway("scanner_reset_failed", err)
	}
	h.audit(ctx, sessionOf(ctx).AdminID, "node.scanners_clear", "node", strconv.FormatInt(in.ID, 10), nil)
	return nil, nil
}
