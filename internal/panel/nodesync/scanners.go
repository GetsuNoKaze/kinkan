package nodesync

import (
	"context"
	"mikan/internal/nodeapi"
	"mikan/internal/panel/store"
)

type scannerSource interface {
	Scanners(context.Context) (nodeapi.ScannerSnapshot, error)
}

// pullScanners stores the node's scanner journal and how its collection goes, so that a
// node that stopped answering, or one that dropped events, is not shown as a quiet one.
func (s *Syncer) pullScanners(ctx context.Context) {
	lock := s.m.scannerLock(s.id)
	lock.Lock()
	defer lock.Unlock()
	if ctx.Err() != nil {
		return
	}
	st, err := s.m.st.ScannerStatusOf(ctx, s.id)
	if err != nil {
		s.log.Warn("read scanner journal status", "err", err)
		return
	}
	now := s.m.now().Unix()
	src, ok := s.node.(scannerSource)
	if !ok {
		st.Unsupported = true
		st.Error, st.ErrorAt = "", 0
		s.saveScannerStatus(ctx, st)
		return
	}
	st.Unsupported = false
	snap, err := src.Scanners(ctx)
	switch {
	case nodeapi.IsScannerUnsupported(err):
		st.Unsupported, st.Error, st.ErrorAt = true, "", 0
	case err != nil:
		st.Error, st.ErrorAt = err.Error(), now
	case snap.Epoch == "":
		st.Unsupported, st.Error, st.ErrorAt = true, "", 0
	default:
		if err := s.m.st.SaveScanners(ctx, s.id, snap, s.m.now()); err != nil {
			s.log.Warn("store scanner journal", "err", err)
			st.Error, st.ErrorAt = "store: "+err.Error(), now
			break
		}
		st.LastOK, st.Error, st.ErrorAt, st.Dropped = now, "", 0, snap.Dropped
		if st.Since == 0 {
			st.Since = now
		}
		if snap.Observer != "" {
			st.Observer = snap.Observer
		}
	}
	s.saveScannerStatus(ctx, st)
}

func (s *Syncer) saveScannerStatus(ctx context.Context, st store.ScannerStatus) {
	if err := s.m.st.SetScannerStatus(ctx, s.id, st); err != nil {
		s.log.Warn("store scanner journal status", "err", err)
	}
}
