package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"mikan/internal/panel/subs"
)

// The routing of the Clash profiles: what the admin can send where (the catalog), and the
// profile it makes before it is saved (the preview).

type routeService struct {
	ID   string `json:"id"`
	Icon string `json:"icon"`
	Name string `json:"name" doc:"По-русски"`
	En   string `json:"name_en"`
}

type routeDirectSet struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	En   string `json:"name_en"`
}

type routesCatalogOutput struct {
	Body struct {
		Services []routeService   `json:"services"`
		Direct   []routeDirectSet `json:"direct"`
	}
}

type routesPreviewInput struct {
	Body struct {
		SubRouting string      `json:"sub_routing" enum:"ru_direct,all,blocked"`
		SubRoutes  subs.Routes `json:"sub_routes"`
	}
}

type routesPreviewOutput struct {
	Body struct {
		Profile string `json:"profile" doc:"Профиль Clash (YAML) пользователя со всеми подключениями; ключи — заглушки"`
	}
}

func (h *handlers) registerRoutes() {
	huma.Register(h.api, huma.Operation{OperationID: "routes-catalog", Method: http.MethodGet, Path: "/api/v1/settings/routes/catalog", Summary: "Сервисы и списки для маршрутизации", Tags: []string{"settings"}}, h.routesCatalog)
	huma.Register(h.api, huma.Operation{OperationID: "routes-preview", Method: http.MethodPost, Path: "/api/v1/settings/routes/preview", Summary: "Профиль Clash с этой маршрутизацией, без сохранения", Tags: []string{"settings"}}, h.routesPreview)
}

func (h *handlers) routesCatalog(context.Context, *struct{}) (*routesCatalogOutput, error) {
	out := &routesCatalogOutput{}
	out.Body.Services = []routeService{}
	for _, s := range subs.Services {
		out.Body.Services = append(out.Body.Services, routeService{ID: s.ID, Icon: s.Icon, Name: s.Name, En: s.NameEN})
	}
	out.Body.Direct = []routeDirectSet{}
	for _, d := range subs.DirectSets {
		out.Body.Direct = append(out.Body.Direct, routeDirectSet{ID: d.ID, Name: d.Name, En: d.NameEN})
	}
	return out, nil
}

func (h *handlers) routesPreview(ctx context.Context, in *routesPreviewInput) (*routesPreviewOutput, error) {
	if h.d.RoutesPreview == nil {
		return nil, huma.Error503ServiceUnavailable("preview_unavailable")
	}
	exists := func(id int64) bool {
		_, err := h.d.Store.Q.GetNode(ctx, id)
		return err == nil
	}
	if err := in.Body.SubRoutes.Check(exists); err != nil {
		return nil, huma.Error422UnprocessableEntity("validation", &huma.ErrorDetail{Location: "body.sub_routes", Message: err.Error()})
	}
	raw, err := h.d.RoutesPreview(ctx, subs.ParseRouting(in.Body.SubRouting), in.Body.SubRoutes)
	if errors.Is(err, subs.ErrNoProxies) {
		return nil, huma.Error409Conflict("no_proxies")
	}
	if err != nil {
		return nil, err
	}
	out := &routesPreviewOutput{}
	out.Body.Profile = string(raw)
	return out, nil
}
