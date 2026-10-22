package cli

import (
	"context"
	"fmt"

	"mikan/internal/nodeapi"
	"mikan/internal/panel/store"
	"mikan/internal/panel/store/db"
	"mikan/internal/panel/targets"
)

// kinkanSchemaReport reads the fork's schema version before a migration; the function it
// returns prints the change after it, next to Mikan's line, for the installer: a moved
// fork schema keeps a failed update from going back to an image that would refuse it.
func kinkanSchemaReport(ctx context.Context, dsn string) func() {
	before, err := store.KinkanSchemaVersion(ctx, dsn)
	return func() {
		after, aerr := store.KinkanSchemaVersion(ctx, dsn)
		if err != nil || aerr != nil {
			// Unknown: say it moved, so the installer does not go back on a guess.
			fmt.Println("Kinkan schema version: unknown -> unknown")
			return
		}
		fmt.Printf("Kinkan schema version: %d -> %d\n", before, after)
	}
}

// kinkanSiteDeps lets the target checks know the node's own website (Kinkan's node site):
// REALITY may aim at it on the node's loopback, and the node checks it itself.
func kinkanSiteDeps(ctx context.Context, st *store.Store, n db.Node, d targets.Deps) targets.Deps {
	if _, _, ok, err := st.NodeSite(ctx, n.ID); err != nil || !ok {
		return d
	}
	return d.WithSite(nodeapi.SiteHTTPSPort, d.CheckTarget)
}
