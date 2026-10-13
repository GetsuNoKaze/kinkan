package subs

import (
	"testing"

	"go.yaml.in/yaml/v3"

	"mikan/internal/panel/store/db"
)

// TrustTunnel clients get the admin's pool limit as an integer, 1 when the inbound has none;
// other protocols get nothing.
func TestKinkanTrustTunnelMaxConnectionsYAML(t *testing.T) {
	for _, pin := range []string{"", "ab12"} {
		t.Run("pin="+pin, func(t *testing.T) {
			prof := profile(t, pin)
			prof.Inbounds = []db.Inbound{
				{ID: 1, NodeID: 1, Name: "TT", Port: "4443", Enabled: 1, Config: "type: trusttunnel\n"},
				{ID: 2, NodeID: 1, Name: "TT3", Port: "4445", Enabled: 1, Config: "type: trusttunnel\nmikan:\n  client:\n    max-connections: 3\n"},
				{ID: 3, NodeID: 1, Name: "AnyTLS", Port: "4444", Enabled: 1, Config: "type: anytls\n"},
			}
			raw, err := Mihomo(prof, Groups{}, RoutingAll)
			if err != nil {
				t.Fatal(err)
			}
			var cfg struct {
				Proxies []map[string]any `yaml:"proxies"`
			}
			if err := yaml.Unmarshal(raw, &cfg); err != nil {
				t.Fatal(err)
			}
			want := map[any]int{"4443": 1, 4443: 1, "4445": 3, 4445: 3}
			seen := 0
			for _, proxy := range cfg.Proxies {
				limit, exists := proxy["max-connections"]
				if proxy["type"] != "trusttunnel" {
					if exists {
						t.Errorf("max-connections leaked to %v", proxy["type"])
					}
					continue
				}
				seen++
				if limit != want[proxy["port"]] {
					t.Errorf("port %v: want max-connections %d, got %#v", proxy["port"], want[proxy["port"]], limit)
				}
				if proxy["username"] != prof.Slot.Name || proxy["password"] != prof.Slot.Secret || proxy["udp"] != true {
					t.Errorf("TrustTunnel credentials or UDP changed: %v", proxy)
				}
			}
			if seen != 2 {
				t.Fatalf("want two TrustTunnel proxies, got %d:\n%s", seen, raw)
			}
		})
	}
}
