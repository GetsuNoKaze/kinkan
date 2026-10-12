package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/danielgtaylor/huma/v2"
	"mikan/internal/panel/store"
)

type leakUserInput struct {
	ID int64 `path:"id" minimum:"1"`
}
type leakUserOutput struct {
	Body struct {
		Events []store.LeakEvent `json:"events"`
	}
}
type leakPolicyInput struct{ Body store.LeakPolicy }
type leakPolicyOutput struct{ Body store.LeakPolicy }

func (h *handlers) registerLeaks() {
	meta := func(id, method, path, summary string) huma.Operation {
		return huma.Operation{OperationID: id, Method: method, Path: path, Summary: summary, Tags: []string{"user"}, Metadata: sessionOnly, Extensions: sessionOnlyExt}
	}
	huma.Register(h.api, meta("get-user-leak-events", http.MethodGet, "/api/v1/users/{id}/leak-events", "Подозрительное использование подписки"), h.getUserLeaks)
	huma.Register(h.api, meta("get-leak-policy", http.MethodGet, "/api/v1/kinkan/leak-policy", "Настройки детектора подписок"), h.getLeakPolicy)
	huma.Register(h.api, meta("set-leak-policy", http.MethodPut, "/api/v1/kinkan/leak-policy", "Изменить настройки детектора подписок"), h.setLeakPolicy)
}

func (h *handlers) getUserLeaks(ctx context.Context, in *leakUserInput) (*leakUserOutput, error) {
	if _, err := h.d.Store.Q.GetUser(ctx, in.ID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, huma.Error404NotFound("not_found")
		}
		return nil, err
	}
	events, err := h.d.Store.LeakEvents(ctx, in.ID, h.d.Now())
	if err != nil {
		return nil, err
	}
	out := &leakUserOutput{}
	out.Body.Events = events
	return out, nil
}
func (h *handlers) getLeakPolicy(ctx context.Context, _ *struct{}) (*leakPolicyOutput, error) {
	p, err := h.d.Store.LeakPolicy(ctx)
	if err != nil {
		return nil, err
	}
	return &leakPolicyOutput{Body: p}, nil
}
func (h *handlers) setLeakPolicy(ctx context.Context, in *leakPolicyInput) (*leakPolicyOutput, error) {
	if !in.Body.Valid() {
		return nil, huma.Error422UnprocessableEntity("leak_policy_invalid")
	}
	if err := h.d.Store.SetLeakPolicy(ctx, in.Body); err != nil {
		return nil, err
	}
	h.audit(ctx, sessionOf(ctx).AdminID, "kinkan.leak_policy", "settings", strconv.Itoa(1), map[string]any{"enabled": in.Body.Enabled, "networks": in.Body.Networks, "countries": in.Body.Countries, "window_minutes": in.Body.WindowMinutes})
	return &leakPolicyOutput{Body: in.Body}, nil
}
