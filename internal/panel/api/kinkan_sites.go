package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"mikan/internal/nodeapi"
	"mikan/internal/panel/store"
	"mikan/internal/site"
)

// Kinkan: node sites (ROADMAP item 2). The admin uploads a static website as a zip; the
// panel checks it (internal/site), keeps it and records which node shows which site.

// SiteView is a kept site and what the admin should know about it.
type SiteView struct {
	store.Site
	// SeveralNodes: more than one node shows it, which ties those nodes together for
	// anyone who indexes page titles (Censys, Shodan).
	SeveralNodes bool `json:"several_nodes"`
	// SameTitle: another kept site has the same front page title.
	SameTitle bool `json:"same_title"`
}

type sitesOutput struct{ Body []SiteView }
type siteOutput struct {
	Body struct {
		SiteView
		// Existing: this content was kept already, under the name shown.
		Existing bool `json:"existing"`
	}
}

type uploadSiteInput struct {
	Name    string `query:"name" maxLength:"100" doc:"Имя сайта в панели; пусто — заголовок главной страницы"`
	RawBody []byte `contentType:"application/zip"`
}

type siteIDInput struct {
	ID int64 `path:"id" minimum:"1"`
}

type nodeSiteInput struct {
	ID   int64 `path:"id" minimum:"1"`
	Body struct {
		SiteID int64 `json:"site_id" minimum:"0" doc:"Сайт ноды; 0 — без сайта"`
	}
}

type nodeSiteOutput struct {
	Body struct {
		NodeID int64 `json:"node_id"`
		SiteID int64 `json:"site_id"`
	}
}

func (h *handlers) registerSites() {
	huma.Register(h.api, huma.Operation{OperationID: "list-sites", Method: http.MethodGet, Path: "/api/v1/sites", Tags: []string{"sites"}, Summary: "Сайты нод"}, h.listSites)
	huma.Register(h.api, huma.Operation{OperationID: "upload-site", Method: http.MethodPost, Path: "/api/v1/sites", Tags: []string{"sites"}, Summary: "Загрузить сайт (zip)",
		MaxBodyBytes: site.MaxArchive + 1, DefaultStatus: http.StatusCreated}, h.uploadSite)
	huma.Register(h.api, huma.Operation{OperationID: "delete-site", Method: http.MethodDelete, Path: "/api/v1/sites/{id}", Tags: []string{"sites"}, Summary: "Удалить сайт",
		DefaultStatus: http.StatusNoContent}, h.deleteSite)
	huma.Register(h.api, huma.Operation{OperationID: "set-node-site", Method: http.MethodPut, Path: "/api/v1/nodes/{id}/site", Tags: []string{"sites"}, Summary: "Сайт ноды"}, h.setNodeSite)
	huma.Register(h.api, huma.Operation{OperationID: "node-site-served", Method: http.MethodGet, Path: "/api/v1/nodes/{id}/site", Tags: []string{"sites"}, Summary: "Что отдаёт нода"}, h.nodeSiteServed)
}

type nodeSiteServedOutput struct {
	Body struct {
		// Served is what the node says it serves; nil: nothing, or a node that does not
		// answer or knows nothing of sites (Mikan's). Kept apart from NodeInfo, Mikan's.
		Served *nodeapi.SiteStatus `json:"served,omitempty" doc:"Сайт, который отдаёт нода: хеш, адреса на 127.0.0.1 и ошибка (например, порт занят); нет поля — ничего"`
	}
}

func (h *handlers) nodeSiteServed(ctx context.Context, in *siteIDInput) (*nodeSiteServedOutput, error) {
	if _, err := h.nodeOf(ctx, in.ID); err != nil {
		return nil, err
	}
	out := &nodeSiteServedOutput{}
	if h.d.Nodes != nil {
		if hv, ok := h.d.Nodes.Health(in.ID); ok && hv.OK {
			out.Body.Served = hv.Health.Site
		}
	}
	return out, nil
}

func (h *handlers) listSites(ctx context.Context, _ *struct{}) (*sitesOutput, error) {
	sites, err := h.d.Store.Sites(ctx)
	if err != nil {
		return nil, err
	}
	return &sitesOutput{Body: siteViews(sites)}, nil
}

// siteViews adds the warnings, which depend on the other sites.
func siteViews(sites []store.Site) []SiteView {
	titles := map[string]int{}
	for _, s := range sites {
		if t := strings.ToLower(s.Title); t != "" {
			titles[t]++
		}
	}
	out := make([]SiteView, len(sites))
	for i, s := range sites {
		out[i] = SiteView{Site: s, SeveralNodes: len(s.Nodes) > 1, SameTitle: s.Title != "" && titles[strings.ToLower(s.Title)] > 1}
	}
	return out
}

func (h *handlers) uploadSite(ctx context.Context, in *uploadSiteInput) (*siteOutput, error) {
	st, err := site.Unpack(in.RawBody)
	if err != nil {
		var e *site.Error
		if errors.As(err, &e) {
			return nil, huma.Error422UnprocessableEntity("invalid_site", &huma.ErrorDetail{Location: "body", Message: e.Code, Value: e.Path})
		}
		return nil, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = st.Title
	}
	if name == "" {
		name = st.Hash[:12]
	}
	kept, existing, err := h.d.Store.AddSite(ctx, name, st, h.d.Now().Unix())
	if err != nil {
		return nil, err
	}
	if !existing {
		h.audit(ctx, sessionOf(ctx).AdminID, "site.upload", "site", strconv.FormatInt(kept.ID, 10), map[string]any{"name": kept.Name, "hash": kept.Hash, "files": kept.Files, "size": kept.Size})
	}
	all, err := h.d.Store.Sites(ctx)
	if err != nil {
		return nil, err
	}
	out := &siteOutput{}
	out.Body.Existing = existing
	for _, v := range siteViews(all) {
		if v.ID == kept.ID {
			out.Body.SiteView = v
		}
	}
	return out, nil
}

func (h *handlers) deleteSite(ctx context.Context, in *siteIDInput) (*struct{}, error) {
	switch err := h.d.Store.DeleteSite(ctx, in.ID); {
	case errors.Is(err, store.ErrSiteInUse):
		return nil, huma.Error409Conflict("site_in_use")
	case errors.Is(err, sql.ErrNoRows):
		return nil, huma.Error404NotFound("not_found")
	case err != nil:
		return nil, err
	}
	h.audit(ctx, sessionOf(ctx).AdminID, "site.delete", "site", strconv.FormatInt(in.ID, 10), nil)
	return nil, nil
}

func (h *handlers) setNodeSite(ctx context.Context, in *nodeSiteInput) (*nodeSiteOutput, error) {
	node, err := h.nodeOf(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	if err := h.d.Store.SetNodeSite(ctx, node.ID, in.Body.SiteID); errors.Is(err, sql.ErrNoRows) {
		return nil, huma.Error422UnprocessableEntity("unknown_site", &huma.ErrorDetail{Location: "body.site_id", Message: "unknown_site"})
	} else if err != nil {
		return nil, err
	}
	h.audit(ctx, sessionOf(ctx).AdminID, "node.site", "node", strconv.FormatInt(node.ID, 10), map[string]any{"site_id": in.Body.SiteID})
	if h.d.Changes != nil {
		h.d.Changes.SlotsChanged() // the nodes' states are built again, with the site
	}
	out := &nodeSiteOutput{}
	out.Body.NodeID, out.Body.SiteID = node.ID, in.Body.SiteID
	return out, nil
}
