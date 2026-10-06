package app

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"mikan/internal/panel/domain"
	"mikan/internal/panel/settings"
)

// Routing over the API: the catalog, a preview before saving, refusals with a code, and
// the saved routes in the Clash profiles.
func TestRoutesOverHTTP(t *testing.T) {
	h := newHarness(t)
	ctx := t.Context()
	if err := domain.Seed(ctx, h.st, h.now); err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]any{settings.KeyPublicHost: "203.0.113.10", settings.KeyPanelPort: 21355} {
		if err := settings.Set(ctx, settings.New(h.st.Q), k, v); err != nil {
			t.Fatal(err)
		}
	}
	if resp, _ := h.login(password, ""); resp.StatusCode != http.StatusOK {
		t.Fatal("login")
	}
	api := "/" + adminPath + "/api/v1/settings"
	csrf := map[string]string{"X-CSRF-Token": h.csrf}

	resp, body := h.do(http.MethodGet, api+"/routes/catalog", nil, nil)
	var cat struct {
		Services []struct{ ID string } `json:"services"`
		Direct   []struct{ ID string } `json:"direct"`
	}
	if resp.StatusCode != http.StatusOK || json.Unmarshal(body, &cat) != nil || len(cat.Services) < 10 || len(cat.Direct) == 0 {
		t.Fatalf("catalog: %d %s", resp.StatusCode, body)
	}

	routes := map[string]any{"services": map[string]string{"youtube": "vpn", "ads": "block", "telegram": "node:1"}, "direct": []string{"ru"}}
	resp, body = h.do(http.MethodPost, api+"/routes/preview", map[string]any{"sub_routing": "blocked", "sub_routes": routes}, csrf)
	var pv struct {
		Profile string `json:"profile"`
	}
	if resp.StatusCode != http.StatusOK || json.Unmarshal(body, &pv) != nil || !strings.Contains(pv.Profile, "GEOSITE,category-ads-all,REJECT") ||
		!strings.Contains(pv.Profile, "MATCH,DIRECT") || !strings.Contains(pv.Profile, "00000000-0000-0000-0000-000000000000") {
		t.Fatalf("preview: %d %s", resp.StatusCode, body)
	}
	// The preview saves nothing.
	if v, _, _ := settings.Get[string](ctx, settings.New(h.st.Q), settings.KeyRouting); v == "blocked" {
		t.Fatal("the preview saved the mode")
	}

	for body, code := range map[string]string{
		`{"sub_routes":{"services":{"youtube":"node:99"}}}`: "routes_target",
		`{"sub_routes":{"dns":{"nameserver":["ftp://x"]}}}`: "routes_dns",
	} {
		var b map[string]any
		_ = json.Unmarshal([]byte(body), &b)
		resp, out := h.do(http.MethodPatch, api, b, csrf)
		if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(string(out), code) {
			t.Fatalf("%s: %d %s", body, resp.StatusCode, out)
		}
	}
	resp, body = h.do(http.MethodPatch, api, map[string]any{"sub_routing": "blocked", "sub_routes": routes}, csrf)
	var v struct {
		SubRouting string `json:"sub_routing"`
		SubRoutes  struct {
			Services map[string]string `json:"services"`
		} `json:"sub_routes"`
	}
	if resp.StatusCode != http.StatusOK || json.Unmarshal(body, &v) != nil || v.SubRouting != "blocked" || v.SubRoutes.Services["telegram"] != "node:1" {
		t.Fatalf("save: %d %s", resp.StatusCode, body)
	}

	clock := func() time.Time { return h.now }
	tariffs, _ := h.st.Q.ListTariffs(ctx)
	u, err := domain.NewUsers(h.st, domain.NewPool(h.st, clock), noChanges{}, clock).Create(ctx, domain.CreateInput{Name: "a", TariffID: tariffs[1].ID})
	if err != nil {
		t.Fatal(err)
	}
	h.now = h.now.Add(time.Minute) // past the profile cache
	resp, body = h.do(http.MethodGet, "/"+subPath+"/"+u.SubToken, nil, map[string]string{"User-Agent": "mihomo/1.19.31"})
	var cfg struct {
		Rules []string `json:"rules"`
	}
	if resp.StatusCode != http.StatusOK || unmarshalProfile(body, &cfg) != nil || cfg.Rules[len(cfg.Rules)-1] != "MATCH,DIRECT" ||
		!strings.Contains(strings.Join(cfg.Rules, "\n"), "RULE-SET,mikan-ru-apps,DIRECT") {
		t.Fatalf("profile: %d %s", resp.StatusCode, body)
	}
}
