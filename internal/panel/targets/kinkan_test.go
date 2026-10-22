package targets

import (
	"context"
	"errors"
	"testing"

	"mikan/internal/nodeapi"
	"mikan/internal/panel/settings"
	"mikan/internal/scan"
)

// The node's own site (Kinkan) is checked by the node: the panel cannot dial the node's
// loopback, and refusing it as internal made autotune and the form take it for a dead site.
func TestCheckTheNodesOwnSite(t *testing.T) {
	ctx := context.Background()
	d := newDeps(t, map[string]any{settings.KeyPanelPort: 1})
	if _, err := d.Check(ctx, "127.0.0.1:17443", "night.example"); !errors.Is(err, ErrPrivateDest) {
		t.Fatalf("without a site: %v, want ErrPrivateDest", err)
	}
	var asked nodeapi.TargetCheckRequest
	ok := d.WithSite(17443, func(_ context.Context, req nodeapi.TargetCheckRequest) (nodeapi.TargetResult, error) {
		asked = req
		return scan.Result{Dest: req.Dest, SNI: req.SNI, OK: true}, nil
	})
	for _, dest := range []string{"127.0.0.1:17443", "localhost:17443"} {
		r, err := ok.Check(ctx, dest, "night.example")
		if err != nil || !r.OK {
			t.Fatalf("%s: %+v, %v", dest, r, err)
		}
		if asked.Dest != "127.0.0.1:17443" || asked.SNI != "night.example" {
			t.Fatalf("the node was asked %+v", asked)
		}
	}
	if _, err := ok.Check(ctx, "127.0.0.1:17444", "night.example"); !errors.Is(err, ErrPrivateDest) {
		t.Fatalf("another loopback port: %v, want ErrPrivateDest", err)
	}
	for _, check := range []func(context.Context, nodeapi.TargetCheckRequest) (nodeapi.TargetResult, error){
		nil,
		func(context.Context, nodeapi.TargetCheckRequest) (nodeapi.TargetResult, error) {
			return scan.Result{}, errors.New("down")
		},
	} {
		r, err := d.WithSite(17443, check).Check(ctx, "127.0.0.1:17443", "night.example")
		if err != nil || r.OK || r.Error != "node_unreachable" {
			t.Fatalf("node not asked: %+v, %v", r, err)
		}
	}
}
