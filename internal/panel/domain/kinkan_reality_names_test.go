package domain

import (
	"context"
	"errors"
	"testing"
	"time"

	"mikan/internal/panel/settings"
	"mikan/internal/panel/store/db"
	"mikan/internal/proto"
)

// A remote node's REALITY may not send the panel's or another node's name; its own name
// and any outside site are fine, and so is the panel's name on the panel's own node.
func TestKinkanForeignSNI(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	st, s := inbounds(t, &now, nil)
	ctx := context.Background()
	set := settings.New(st.Q)
	if err := settings.Set(ctx, set, settings.KeyDomain, "panel.example.com"); err != nil {
		t.Fatal(err)
	}
	if err := settings.Set(ctx, set, settings.KeyPanelPort, 21355); err != nil {
		t.Fatal(err)
	}
	fin, err := st.Q.CreateNode(ctx, db.CreateNodeParams{Name: "FI", Address: "203.0.113.7:25305", PublicHost: "203.0.113.7", Domain: "fi.example.com", CreatedAt: now.Unix(), UpdatedAt: now.Unix()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Q.CreateNode(ctx, db.CreateNodeParams{Name: "NL", Address: "198.51.100.9:25305", PublicHost: "198.51.100.9", Domain: "nl.example.com", CreatedAt: now.Unix(), UpdatedAt: now.Unix()}); err != nil {
		t.Fatal(err)
	}
	remote, err := s.Create(ctx, NewInbound{NodeID: fin.ID, Preset: "vless_reality_xhttp", Port: "443"})
	if err != nil {
		t.Fatal(err)
	}
	var pe *proto.Error
	for _, c := range []struct {
		dest, sni string
		foreign   bool
	}{
		{"203.0.113.7:8444", "panel.example.com", true},
		{"203.0.113.7:8444", "PANEL.example.com", true},
		{"203.0.113.7:8444", "nl.example.com", true},
		{"203.0.113.7:8444", "fi.example.com", false},
		{"203.0.113.20:443", "www.example.org", false},
	} {
		_, _, err := s.Update(ctx, remote.ID, InboundPatch{Dest: ptr(c.dest), ServerName: ptr(c.sni)})
		got := errors.As(err, &pe) && pe.Code == "reality_sni_foreign"
		if got != c.foreign {
			t.Errorf("%s with %s: %v", c.dest, c.sni, err)
		}
	}
	// The panel's own node may use the panel's name (self-steal).
	if _, _, err := s.Update(ctx, find(t, s, 1, "vless-vision").ID, InboundPatch{Dest: ptr("127.0.0.1:21355"), ServerName: ptr("panel.example.com")}); err != nil {
		t.Fatalf("self-steal on the panel's node: %v", err)
	}
}
