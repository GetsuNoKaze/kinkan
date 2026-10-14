// Package leakwatch observes the panel's existing authenticated online snapshots.
// It sends no requests and persists no addresses.
package leakwatch

import (
	"context"
	"log/slog"
	"net/netip"
	"time"

	"mikan/internal/nodeapi"
	"mikan/internal/panel/geoip"
	"mikan/internal/panel/store"
)

type Watcher struct {
	Store  *store.Store
	Online func() map[string]nodeapi.Online
	GeoIP  interface{ Lookup(netip.Addr) geoip.Info }
	Now    func() time.Time
	Log    *slog.Logger
}

func Sources(online map[string]nodeapi.Online, owners map[string]int64, lookup func(netip.Addr) geoip.Info) map[int64][]store.LeakSource {
	out := map[int64][]store.LeakSource{}
	seen := map[int64]map[store.LeakSource]bool{}
	for slot, on := range online {
		uid := owners[slot]
		if uid == 0 || on.Conns <= 0 {
			continue
		}
		for _, raw := range on.IPs {
			ip, err := netip.ParseAddr(raw)
			if err != nil || ip.Zone() != "" {
				continue
			}
			ip = ip.Unmap()
			if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}
			info := lookup(ip)
			v := store.LeakSource{ASN: info.ASN, Country: info.Country}
			if v.ASN == "" && v.Country == "" {
				continue
			}
			if seen[uid] == nil {
				seen[uid] = map[store.LeakSource]bool{}
			}
			if !seen[uid][v] {
				seen[uid][v] = true
				out[uid] = append(out[uid], v)
			}
		}
	}
	return out
}

func (w *Watcher) Collect(ctx context.Context) error {
	now := w.Now()
	if err := w.Store.PruneLeaks(ctx, now); err != nil {
		return err
	}
	p, err := w.Store.LeakPolicy(ctx)
	if err != nil {
		return err
	}
	if !p.Enabled || w.Online == nil || w.GeoIP == nil {
		return nil
	}
	rows, err := w.Store.Q.ListSlotUsers(ctx)
	if err != nil {
		return err
	}
	owners := map[string]int64{}
	for _, r := range rows {
		owners[r.SlotName] = r.UserID
	}
	for user, sources := range Sources(w.Online(), owners, w.GeoIP.Lookup) {
		if err := w.Store.RecordLeakSources(ctx, user, sources, now, p); err != nil {
			return err
		}
	}
	return nil
}

func (w *Watcher) Run(ctx context.Context) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		part, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := w.Collect(part)
		cancel()
		if err != nil && ctx.Err() == nil {
			w.Log.Error("subscription leak detector", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
