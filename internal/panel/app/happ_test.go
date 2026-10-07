package app

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"mikan/internal/panel/domain"
)

// Happ over the API: the settings are checked and session only, Happ alone gets its
// headers, and the page's and the card's crypt links hide the address.
func TestHappOverHTTP(t *testing.T) {
	k := newKeyHarness(t)
	ctx := t.Context()
	patch := func(body map[string]any) (int, string) {
		t.Helper()
		resp, raw := k.do(http.MethodPatch, k.api+"/settings", body, k.csrf)
		return resp.StatusCode, string(raw)
	}
	for _, c := range []struct {
		body      map[string]any
		want, loc string
	}{
		{map[string]any{"happ_routing": "https://example.com/routing"}, "happ_routing", "body.happ_routing"},
		{map[string]any{"happ_provider_id": "two words"}, "happ_provider_id", "body.happ_provider_id"},
		{map[string]any{"happ_hide_settings": true}, "happ_needs_provider", "body.happ_hide_settings"},
	} {
		if code, body := patch(c.body); code != http.StatusUnprocessableEntity || !strings.Contains(body, c.want) || !strings.Contains(body, c.loc) {
			t.Errorf("%v: %d %s", c.body, code, body)
		}
	}
	routing := "happ://routing/onadd/" + base64.StdEncoding.EncodeToString([]byte(`{"Name":"RU"}`))
	if code, body := patch(map[string]any{"happ_routing": routing, "happ_provider_id": "prov-1", "happ_hide_settings": true, "happ_crypt": "local"}); code != http.StatusOK ||
		!strings.Contains(body, `"happ_provider_id":"prov-1"`) || !strings.Contains(body, `"happ_hide_settings":true`) || !strings.Contains(body, `"happ_crypt":"local"`) {
		t.Fatalf("save: %d %s", code, body)
	}
	for field, value := range map[string]any{"happ_routing": "", "happ_provider_id": "x", "happ_hide_settings": false, "happ_crypt": "off"} {
		if resp, body := k.asKey(k.full, http.MethodPatch, "/settings", map[string]any{field: value}); resp.StatusCode != http.StatusForbidden || !strings.Contains(string(body), "session_only") {
			t.Errorf("%s with a full key: %d %s", field, resp.StatusCode, body)
		}
	}

	clock := func() time.Time { return k.now }
	tariffs, _ := k.st.Q.ListTariffs(ctx)
	u, err := domain.NewUsers(k.st, domain.NewPool(k.st, clock), noChanges{}, clock).Create(ctx, domain.CreateInput{Name: "a", TariffID: tariffs[1].ID})
	if err != nil {
		t.Fatal(err)
	}
	fetch := func(ua string) http.Header {
		t.Helper()
		resp, _ := k.do(http.MethodGet, "/"+subPath+"/"+u.SubToken, nil, map[string]string{"User-Agent": ua})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("subscription for %s: %d", ua, resp.StatusCode)
		}
		return resp.Header
	}
	if hd := fetch("Happ/3.6.0/Android/1"); hd.Get("routing") != routing || hd.Get("providerid") != "prov-1" || hd.Get("hide-settings") != "1" {
		t.Errorf("Happ headers: %v", hd)
	}
	if hd := fetch("v2RayTun/5.1.0"); hd.Get("routing") != "" || hd.Get("providerid") != "" || hd.Get("hide-settings") != "" {
		t.Errorf("another app got Happ's headers: %v", hd)
	}

	resp, raw := k.do(http.MethodGet, "/"+subPath+"/"+u.SubToken+"/info", nil, nil)
	var info struct {
		HappLink string `json:"happ_link"`
	}
	if resp.StatusCode != http.StatusOK || json.Unmarshal(raw, &info) != nil || !strings.HasPrefix(info.HappLink, "happ://crypt5/") || strings.Contains(info.HappLink, u.SubToken) {
		t.Fatalf("the page's Happ link: %d %s", resp.StatusCode, raw)
	}

	path := "/users/" + idOf(u.ID) + "/happ-link"
	if resp, body := k.do(http.MethodGet, k.api+path, nil, nil); resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "happ://crypt5/") {
		t.Errorf("the card's link: %d %s", resp.StatusCode, body)
	}
	if resp, body := k.asKey(k.read, http.MethodGet, path, nil); resp.StatusCode != http.StatusForbidden {
		t.Errorf("a read key got the link: %d %s", resp.StatusCode, body)
	}
	if resp, _ := k.asKey(k.full, http.MethodGet, path, nil); resp.StatusCode != http.StatusOK {
		t.Errorf("a full key: %d", resp.StatusCode)
	}

	// Without a provider id the switch would do nothing: it goes off with it.
	if code, body := patch(map[string]any{"happ_provider_id": ""}); code != http.StatusOK || !strings.Contains(body, `"happ_hide_settings":false`) {
		t.Fatalf("provider cleared: %d %s", code, body)
	}
	if code, body := patch(map[string]any{"happ_crypt": "off"}); code != http.StatusOK || !strings.Contains(body, `"happ_crypt":"off"`) {
		t.Fatalf("crypt off: %d %s", code, body)
	}
	if resp, body := k.do(http.MethodGet, k.api+path, nil, nil); resp.StatusCode != http.StatusOK || strings.Contains(string(body), "happ://") {
		t.Errorf("crypt off, no link: %d %s", resp.StatusCode, body)
	}
}
