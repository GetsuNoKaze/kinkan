package nodesync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"mikan/internal/nodeapi"
)

// Kinkan's part of the syncer. Mikan's files call into it with one line each, so that
// merges from upstream meet as few of the fork's lines as possible.

// runKinkan starts the fork's own loops for the node; the function it returns waits for
// them once ctx is done.
func (s *Syncer) runKinkan(ctx context.Context) (wait func()) {
	done := make(chan struct{}, 2)
	go func() {
		defer func() { done <- struct{}{} }()
		every(ctx, 30*time.Second, s.pullScanners)
	}()
	go func() {
		defer func() { done <- struct{}{} }()
		s.watchSite(ctx)
	}()
	return func() { <-done; <-done }
}

// siteWatchEvery is how often the health the syncer keeps is looked at for the site: as
// often as the health check runs.
var siteWatchEvery = 5 * time.Second

// watchSite follows what the node says it serves as its site: when it changes, the
// inbounds fitted to the site (fitToSite) follow. Only answers count; a node that does
// not answer keeps what it had.
func (s *Syncer) watchSite(ctx context.Context) {
	last := servedSite(s.health.Load())
	t := time.NewTicker(siteWatchEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			now := s.health.Load()
			if now == nil || !now.OK {
				continue
			}
			if cur := servedSite(now); cur != last {
				last = cur
				signal(s.stateDirty)
			}
		}
	}
}

// kinkanStateKey is the fork's part of the state's key: a new site is a new state.
func kinkanStateKey(st nodeapi.DesiredState) string {
	if st.Site == nil {
		return ""
	}
	raw, _ := json.Marshal(st.KinkanState)
	sum := sha256.Sum256(raw)
	return "/" + hex.EncodeToString(sum[:])
}

// pruneKinkan drops the fork's expired records, the disabled nodes' too, with the rest of
// the database's upkeep. A failure is logged and keeps no other pruning from running.
func (m *Manager) pruneKinkan(ctx context.Context, now time.Time) {
	if err := m.st.PruneScanners(ctx, now); err != nil {
		m.log.Error("prune scanner journal", "err", err)
	}
}
