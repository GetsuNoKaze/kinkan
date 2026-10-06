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

// An own Clash profile over the API: started from the built-in one, refused with the rule
// that points nowhere, served to mihomo apps, and the built-in one again once cleared.
func TestOwnProfileOverHTTP(t *testing.T) {
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

	resp, body := h.do(http.MethodPost, api+"/routes/preview", map[string]any{"sub_routing": "all", "sub_routes": map[string]any{}, "starter": true}, csrf)
	var pv struct {
		Profile string `json:"profile"`
	}
	if resp.StatusCode != http.StatusOK || json.Unmarshal(body, &pv) != nil || !strings.Contains(pv.Profile, "include-all-proxies: true") || strings.Contains(pv.Profile, "uuid") {
		t.Fatalf("starter: %d %s", resp.StatusCode, body)
	}
	own := pv.Profile + "\n# mine\n"
	own = strings.Replace(own, "rules:\n", "rules:\n  - DOMAIN-SUFFIX,mine.example,DIRECT\n", 1)

	bad := strings.Replace(own, "MATCH,VPN", "MATCH,Nowhere", 1)
	resp, body = h.do(http.MethodPatch, api, map[string]any{"sub_template": bad}, csrf)
	if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(string(body), "template_rule_target") || !strings.Contains(string(body), "Nowhere") {
		t.Fatalf("bad template: %d %s", resp.StatusCode, body)
	}
	resp, body = h.do(http.MethodPatch, api, map[string]any{"sub_template": own}, csrf)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "mine.example") {
		t.Fatalf("save: %d %s", resp.StatusCode, body)
	}

	clock := func() time.Time { return h.now }
	tariffs, _ := h.st.Q.ListTariffs(ctx)
	u, err := domain.NewUsers(h.st, domain.NewPool(h.st, clock), noChanges{}, clock).Create(ctx, domain.CreateInput{Name: "a", TariffID: tariffs[1].ID})
	if err != nil {
		t.Fatal(err)
	}
	fetch := func(ua string) string {
		t.Helper()
		h.now = h.now.Add(time.Minute) // past the profile cache
		resp, body := h.do(http.MethodGet, "/"+subPath+"/"+u.SubToken, nil, map[string]string{"User-Agent": ua})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: %d %s", ua, resp.StatusCode, body)
		}
		return string(body)
	}
	if p := fetch("mihomo/1.19.31"); !strings.Contains(p, "DOMAIN-SUFFIX,mine.example,DIRECT") || strings.Contains(p, "include-all-proxies") || !strings.Contains(p, "IP-CIDR,203.0.113.10/32,DIRECT") {
		t.Fatalf("mihomo app:\n%s", p)
	}
	if p := fetch("Stash/3.1.1 Clash/1.9.0"); strings.Contains(p, "mine.example") {
		t.Fatalf("Stash gets the built-in profile:\n%s", p)
	}
	if resp, body := h.do(http.MethodPatch, api, map[string]any{"sub_template": "  "}, csrf); resp.StatusCode != http.StatusOK {
		t.Fatalf("clear: %d %s", resp.StatusCode, body)
	}
	if p := fetch("mihomo/1.19.31"); strings.Contains(p, "mine.example") {
		t.Fatalf("cleared, still the own profile:\n%s", p)
	}
}
