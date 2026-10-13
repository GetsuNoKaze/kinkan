package proto

import (
	"encoding/json"
	"testing"
)

func TestKinkanTTConnections(t *testing.T) {
	tpl := Template{"type": "trusttunnel"}
	if n := KinkanTTConnections(tpl); n != 1 {
		t.Fatalf("unset: %d", n)
	}
	if err := KinkanSetTTConnections(tpl, 3); err != nil || KinkanTTConnections(tpl) != 3 {
		t.Fatalf("set 3: %v %v", err, tpl)
	}
	if err := Validate(tpl, Options{}); err != nil {
		t.Fatalf("validate 3: %v", err)
	}
	// The panel keeps templates as JSON: numbers come back as float64.
	raw, _ := json.Marshal(tpl)
	back, err := FromJSON(raw)
	if err != nil || KinkanTTConnections(back) != 3 {
		t.Fatalf("json: %v %v", err, back)
	}
	if err := KinkanSetTTConnections(tpl, 1); err != nil || tpl[extKey] != nil {
		t.Fatalf("set 1 must remove the key: %v %v", err, tpl)
	}
	for _, n := range []int{0, 9, -1} {
		if code(KinkanSetTTConnections(tpl, n)) != "config_tt_connections" {
			t.Fatalf("set %d accepted", n)
		}
	}
	if code(KinkanSetTTConnections(Template{"type": "anytls"}, 2)) != "config_key" {
		t.Fatal("anytls accepted a TrustTunnel pool limit")
	}
	for _, c := range []struct {
		tpl  Template
		code string
	}{
		{Template{"type": "trusttunnel", extKey: map[string]any{"client": map[string]any{"max-connections": 9}}}, "config_tt_connections"},
		{Template{"type": "trusttunnel", extKey: map[string]any{"client": map[string]any{"max-connections": "2"}}}, "config_tt_connections"},
		{Template{"type": "anytls", extKey: map[string]any{"client": map[string]any{"max-connections": 2}}}, "config_key"},
	} {
		if got := code(Validate(c.tpl, Options{})); got != c.code {
			t.Errorf("%v: got %q, want %q", c.tpl, got, c.code)
		}
	}
}
