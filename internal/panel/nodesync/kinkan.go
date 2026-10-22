package nodesync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	"mikan/internal/nodeapi"
)

// Kinkan's part of the syncer. Mikan's files call into it with one line each, so that
// merges from upstream meet as few of the fork's lines as possible.

type kinkanManager struct {
	// Locks stay with the node ID when its syncer is replaced or reconnected.
	scannerLocks sync.Map // node ID -> *sync.Mutex
}

func (m *Manager) scannerLock(id int64) *sync.Mutex {
	lock, _ := m.scannerLocks.LoadOrStore(id, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

// runKinkan starts the fork's own loops for the node; the function it returns waits for
// them once ctx is done.
func (s *Syncer) runKinkan(ctx context.Context) (wait func()) {
	done := make(chan struct{}, 2)
	go func() {
		defer func() { done <- struct{}{} }()
		kinkanEvery(ctx, 30*time.Second, s.pullScanners)
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
	if err := m.st.PruneProbeRuns(ctx, now); err != nil {
		m.log.Error("prune protocol check history", "err", err)
	}
}

// ClearScanners waits for an in-flight collection, then resets the node and removes
// the panel's copy before the next collection can start. An old snapshot cannot be
// stored after clearing has succeeded.
func (m *Manager) ClearScanners(ctx context.Context, id int64) error {
	lock := m.scannerLock(id)
	lock.Lock()
	defer lock.Unlock()
	s, ok := m.Syncer(id)
	if !ok {
		return nodeapi.ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	c, ok := s.node.(interface{ ResetScanners(context.Context) error })
	if !ok {
		return nodeapi.ErrScannersUnsupported
	}
	if err := c.ResetScanners(ctx); err != nil {
		return err
	}
	return m.st.DeleteScanners(ctx, id)
}

// kinkanEvery runs fn every d until ctx is done (Mikan's own helper of this name is gone).
func kinkanEvery(ctx context.Context, d time.Duration, fn func(context.Context)) {
	t := time.NewTicker(d)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			fn(ctx)
		}
	}
}
