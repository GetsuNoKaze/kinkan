package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"mikan/internal/panel/settings"
	"mikan/internal/panel/updates"
)

type rollbackOutput struct{ Body updates.Rollback }
type rollbackInput struct {
	Body struct {
		Version string `json:"version" minLength:"1" maxLength:"80"`
	}
}

func (h *handlers) registerRollback() {
	huma.Register(h.api, huma.Operation{OperationID: "get-rollback", Method: http.MethodGet, Path: "/api/v1/updates/rollback", Tags: []string{"settings"}, Summary: "Доступный откат панели"}, h.getRollback)
	huma.Register(h.api, huma.Operation{OperationID: "request-rollback", Method: http.MethodPost, Path: "/api/v1/updates/rollback/request", Tags: []string{"settings"}, Summary: "Откатить панель к предыдущей совместимой версии", Metadata: sessionOnly, Extensions: sessionOnlyExt, DefaultStatus: http.StatusAccepted}, h.requestRollback)
}

func (h *handlers) getRollback(_ context.Context, _ *struct{}) (*rollbackOutput, error) {
	if h.d.Updates == nil {
		return &rollbackOutput{Body: updates.Rollback{Current: h.d.Version, Reason: "no_previous"}}, nil
	}
	return &rollbackOutput{Body: h.d.Updates.Rollback()}, nil
}

func (h *handlers) requestRollback(ctx context.Context, in *rollbackInput) (*rollbackOutput, error) {
	u := h.d.Updates
	if u == nil {
		return nil, huma.Error409Conflict("updates_unavailable")
	}
	if u.Busy() {
		return nil, huma.Error409Conflict("update_busy")
	}
	v := u.Rollback()
	if !v.Available || v.Version != in.Body.Version {
		return nil, huma.Error409Conflict("rollback_unavailable")
	}
	// Persist before queuing: the restarted old panel must not re-enable automatic
	// updates from settings and immediately undo the rollback.
	channel, err := h.d.Settings.UpdateChannel(ctx)
	if err != nil {
		return nil, err
	}
	if err = settings.Set(ctx, h.d.Settings, settings.KeyAutoUpdate, false); err != nil {
		return nil, err
	}
	if err = u.SetPolicy(updates.Policy{Auto: false, Channel: channel}); err != nil {
		return nil, err
	}
	if err = u.RequestRollback(in.Body.Version); err != nil {
		if errors.Is(err, updates.ErrUpdateBusy) {
			return nil, huma.Error409Conflict("update_busy")
		}
		if errors.Is(err, updates.ErrRollbackUnavailable) {
			return nil, huma.Error409Conflict("rollback_unavailable")
		}
		return nil, err
	}
	h.audit(ctx, sessionOf(ctx).AdminID, "updates.rollback", "", "", map[string]any{"from": h.d.Version, "version": v.Version, "auto": false})
	return &rollbackOutput{Body: v}, nil
}
