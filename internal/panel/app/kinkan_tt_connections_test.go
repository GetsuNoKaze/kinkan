package app

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"mikan/internal/panel/domain"
)

// The admin sets how many connections TrustTunnel clients may keep; it lands in the
// template's mikan.client section, the default leaves no trace, and other protocols and
// values out of range are refused.
func TestTTConnectionsOverHTTP(t *testing.T) {
	h := newHarness(t)
	if err := domain.Seed(t.Context(), h.st, h.now); err != nil {
		t.Fatal(err)
	}
	if resp, _ := h.login(password, ""); resp.StatusCode != http.StatusOK {
		t.Fatal("login")
	}
	api := "/" + adminPath + "/api/v1/inbounds"
	csrf := map[string]string{"X-CSRF-Token": h.csrf}
	type view struct {
		ID     int64  `json:"id"`
		Name   string `json:"name"`
		Config string `json:"config"`
	}
	resp, body := h.do(http.MethodGet, api, nil, nil)
	var all []view
	if resp.StatusCode != http.StatusOK || json.Unmarshal(body, &all) != nil {
		t.Fatalf("list: %d %s", resp.StatusCode, body)
	}
	var tt, other view
	for _, v := range all {
		switch {
		case strings.HasPrefix(v.Config, "type: trusttunnel"):
			tt = v
		case other.ID == 0:
			other = v
		}
	}
	if tt.ID == 0 {
		resp, body = h.do(http.MethodPost, api, map[string]any{"preset": "trusttunnel"}, csrf)
		if resp.StatusCode != http.StatusCreated || json.Unmarshal(body, &tt) != nil {
			t.Fatalf("create trusttunnel: %d %s", resp.StatusCode, body)
		}
	}
	path := api + "/" + strconv.FormatInt(tt.ID, 10) + "/tt-connections"
	type conns struct {
		Value   int `json:"value"`
		Default int `json:"default"`
		Max     int `json:"max"`
	}
	get := func() conns {
		t.Helper()
		resp, body := h.do(http.MethodGet, path, nil, nil)
		var c conns
		if resp.StatusCode != http.StatusOK || json.Unmarshal(body, &c) != nil {
			t.Fatalf("get: %d %s", resp.StatusCode, body)
		}
		return c
	}
	config := func() string {
		t.Helper()
		resp, body := h.do(http.MethodGet, api, nil, nil)
		var all []view
		if resp.StatusCode != http.StatusOK || json.Unmarshal(body, &all) != nil {
			t.Fatalf("list: %d %s", resp.StatusCode, body)
		}
		for _, v := range all {
			if v.ID == tt.ID {
				return v.Config
			}
		}
		t.Fatal("trusttunnel inbound gone")
		return ""
	}

	if c := get(); c.Value != 1 || c.Default != 1 || c.Max != 8 {
		t.Fatalf("default: %+v", c)
	}
	if resp, body := h.do(http.MethodPut, path, map[string]any{"value": 3}, csrf); resp.StatusCode != http.StatusOK {
		t.Fatalf("set 3: %d %s", resp.StatusCode, body)
	}
	if c := get(); c.Value != 3 {
		t.Fatalf("after set 3: %+v", c)
	}
	if cfg := config(); !strings.Contains(cfg, "max-connections: 3") || !strings.Contains(cfg, "congestion-controller: bbr") {
		t.Fatalf("template after set 3:\n%s", cfg)
	}
	if resp, body := h.do(http.MethodPut, path, map[string]any{"value": 1}, csrf); resp.StatusCode != http.StatusOK {
		t.Fatalf("set 1: %d %s", resp.StatusCode, body)
	}
	if cfg := config(); strings.Contains(cfg, "max-connections") || strings.Contains(cfg, "mikan") {
		t.Fatalf("the default must leave no trace:\n%s", cfg)
	}
	for _, v := range []int{0, 9} {
		if resp, body := h.do(http.MethodPut, path, map[string]any{"value": v}, csrf); resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("value %d: %d %s", v, resp.StatusCode, body)
		}
	}
	if resp, body := h.do(http.MethodGet, api+"/"+strconv.FormatInt(other.ID, 10)+"/tt-connections", nil, nil); resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(string(body), "not_trusttunnel") {
		t.Fatalf("%s: %d %s", other.Name, resp.StatusCode, body)
	}
	// The config tab goes through the same check.
	bad := "type: trusttunnel\ncongestion-controller: bbr\nmikan:\n  client:\n    max-connections: 20\n"
	if resp, body := h.do(http.MethodPatch, api+"/"+strconv.FormatInt(tt.ID, 10), map[string]any{"config": bad}, csrf); resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(string(body), "config_tt_connections") {
		t.Fatalf("config tab, 20: %d %s", resp.StatusCode, body)
	}
}
