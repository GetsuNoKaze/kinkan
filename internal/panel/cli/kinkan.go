package cli

import (
	"context"
	"fmt"

	"mikan/internal/panel/store"
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
