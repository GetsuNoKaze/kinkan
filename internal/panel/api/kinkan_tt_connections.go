package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"mikan/internal/proto"
)

// The most TLS connections a TrustTunnel client keeps to an inbound. It lives in the
// template's mikan.client section and goes out with the subscription; the node never sees
// it. Setting it rewrites the template through the usual inbound update, so the checks,
// the audit log and the sync stay the same as for an edit on the config tab.

type ttConnectionsBody struct {
	Value   int `json:"value" doc:"Сколько TLS-соединений клиент может держать к подключению"`
	Default int `json:"default"`
	Max     int `json:"max"`
}

type ttConnectionsOutput struct {
	Body ttConnectionsBody
}

type getTTConnectionsInput struct {
	ID int64 `path:"id" minimum:"1"`
}

type setTTConnectionsInput struct {
	ID   int64 `path:"id" minimum:"1"`
	Body struct {
		Value int `json:"value" minimum:"1" maximum:"8" doc:"Максимум TLS-соединений у клиента, 1–8"`
	}
}

func (h *handlers) registerTTConnections() {
	huma.Register(h.api, huma.Operation{OperationID: "get-tt-connections", Method: http.MethodGet, Path: "/api/v1/inbounds/{id}/tt-connections",
		Summary: "Соединения клиента TrustTunnel", Tags: []string{"inbounds"}}, h.getTTConnections)
	huma.Register(h.api, huma.Operation{OperationID: "set-tt-connections", Method: http.MethodPut, Path: "/api/v1/inbounds/{id}/tt-connections",
		Summary: "Изменить соединения клиента TrustTunnel", Tags: []string{"inbounds"}}, h.setTTConnections)
}

func (h *handlers) ttTemplate(ctx context.Context, id int64) (proto.Template, error) {
	row, err := h.d.Store.Q.GetInbound(ctx, id)
	if err != nil {
		return nil, huma.Error404NotFound("not_found")
	}
	t, err := proto.Parse(row.Config)
	if err != nil || t.Type() != "trusttunnel" {
		return nil, huma.Error422UnprocessableEntity("not_trusttunnel")
	}
	return t, nil
}

func ttConnections(n int) *ttConnectionsOutput {
	return &ttConnectionsOutput{Body: ttConnectionsBody{Value: n, Default: proto.KinkanTTConnsDefault, Max: proto.KinkanTTConnsMaxLimit}}
}

func (h *handlers) getTTConnections(ctx context.Context, in *getTTConnectionsInput) (*ttConnectionsOutput, error) {
	t, err := h.ttTemplate(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	return ttConnections(proto.KinkanTTConnections(t)), nil
}

func (h *handlers) setTTConnections(ctx context.Context, in *setTTConnectionsInput) (*ttConnectionsOutput, error) {
	t, err := h.ttTemplate(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	if proto.KinkanTTConnections(t) == in.Body.Value {
		return ttConnections(in.Body.Value), nil
	}
	if err := proto.KinkanSetTTConnections(t, in.Body.Value); err != nil {
		var pe *proto.Error
		if errors.As(err, &pe) {
			return nil, huma.Error422UnprocessableEntity(pe.Code, &huma.ErrorDetail{Location: "body.value", Message: pe.Code})
		}
		return nil, err
	}
	config := proto.Marshal(t)
	patch := &patchInboundInput{ID: in.ID}
	patch.Body.Config = &config
	if _, err := h.updateInbound(ctx, patch); err != nil {
		return nil, err
	}
	return ttConnections(in.Body.Value), nil
}
