package nodesync

import (
	"context"
	"mikan/internal/nodeapi"
)

type scannerSource interface {
	Scanners(context.Context) (nodeapi.ScannerSnapshot, error)
}

func (s *Syncer) pullScanners(ctx context.Context) {
	src, ok := s.node.(scannerSource)
	if !ok {
		return
	}
	snap, err := src.Scanners(ctx)
	if err != nil || snap.Epoch == "" {
		return
	}
	if err := s.m.st.SaveScanners(ctx, s.id, snap, s.m.now()); err != nil {
		s.log.Warn("store scanner journal", "err", err)
	}
}
