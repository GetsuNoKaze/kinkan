package node

import (
	"context"
	"github.com/metacubex/mihomo/component/authevent"
	"github.com/metacubex/mihomo/component/mmdb"
	C "github.com/metacubex/mihomo/constant"
	"github.com/oschwald/maxminddb-golang"
	"mikan/internal/nodeapi"
	"mikan/internal/proto"
	"mikan/internal/scannerlog"
	"net"
	"net/netip"
	"path/filepath"
	"strings"
	"time"
)

func (e *Engine) startScannerJournal() {
	j, err := scannerlog.Open(filepath.Join(e.dataDir, "node-scanners.json"))
	if err != nil {
		e.log.Warn("scanner journal unavailable", "err", err)
		return
	}
	e.scanners = j
	e.scannerEvents = make(chan authevent.Event, 1024)
	authevent.SetHandler(func(ev authevent.Event) {
		select {
		case e.scannerEvents <- ev:
		default:
			e.scannerDropped.Add(1)
		}
	})
	queue := make(chan string, 128)
	go func() {
		for ip := range queue {
			j.SetMetadata(ip, scannerMetadata(ip))
		}
	}()
	go func() {
		seen := map[string]time.Time{}
		for ev := range e.scannerEvents {
			host, _, err := net.SplitHostPort(ev.Remote)
			if err != nil {
				continue
			}
			ip, err := netip.ParseAddr(host)
			if err != nil {
				continue
			}
			host = ip.Unmap().String()
			_, port, err := net.SplitHostPort(ev.Local)
			if err != nil {
				continue
			}
			name := ""
			e.mu.Lock()
			for _, ib := range e.applied.Inbounds {
				if ib.Port != port {
					continue
				}
				t, err := proto.FromJSON(ib.Config)
				if err == nil && (t.Type() == ev.Protocol || ev.Protocol == "reality" && t["reality-config"] != nil) {
					name = ib.Name
					break
				}
			}
			e.mu.Unlock()
			if name == "" {
				continue
			}
			r := nodeapi.ScannerRecord{IP: host, Inbound: name, Protocol: ev.Protocol, Reason: ev.Reason, Method: ev.Method, Client: e.Reg.knownClientIP(host)}
			if !j.Record(r, time.Now()) {
				continue
			}
			now := time.Now()
			if len(seen) >= scannerlog.MaxRows {
				for ip, at := range seen {
					if now.Sub(at) > scannerlog.Keep {
						delete(seen, ip)
					}
				}
			}
			if at, ok := seen[host]; (!ok || now.Sub(at) > 24*time.Hour) && (ok || len(seen) < scannerlog.MaxRows) {
				select {
				case queue <- host:
					seen[host] = now
				default:
				}
			}
		}
	}()
}
func (r *Registry) knownClientIP(ip string) bool {
	r.mu.RLock()
	slots := values(r.byName)
	r.mu.RUnlock()
	cut := r.now().Add(-activityKeep)
	for _, s := range slots {
		s.mu.Lock()
		known := s.otherIPs[ip]
		for k, t := range s.seen {
			if k.ip == ip && t.After(cut) {
				known = true
				break
			}
		}
		s.mu.Unlock()
		if known {
			return true
		}
	}
	return false
}
func (e *Engine) Scanners(ctx context.Context) (nodeapi.ScannerSnapshot, error) {
	if e.scanners == nil {
		return nodeapi.ScannerSnapshot{Rows: []nodeapi.ScannerRecord{}}, nil
	}
	s, err := e.scanners.Export(time.Now())
	if err != nil {
		return nodeapi.ScannerSnapshot{}, err
	}
	s.Dropped += e.scannerDropped.Load()
	return s, nil
}
func scannerMetadata(ip string) scannerlog.Metadata {
	m := scannerlog.Metadata{Scanner: "unknown"}
	a, err := netip.ParseAddr(ip)
	if err != nil || !proto.PublicAddr(a) {
		return m
	}
	// Read existing assets directly: no downloads or fatal-on-missing singletons.
	if db, err := maxminddb.Open(C.Path.MMDB()); err == nil {
		switch db.Metadata.DatabaseType {
		case "sing-geoip":
			_ = db.Lookup(net.ParseIP(ip), &m.Country)
		case "Meta-geoip0":
			var v any
			if db.Lookup(net.ParseIP(ip), &v) == nil {
				if code, ok := v.(string); ok {
					m.Country = code
				}
			}
		default:
			codes := (mmdb.IPReader{Reader: db}).LookupCode(net.ParseIP(ip))
			if len(codes) > 0 {
				m.Country = codes[0]
			}
		}
		_ = db.Close()
	}
	if db, err := maxminddb.Open(C.Path.ASN()); err == nil {
		m.ASN, m.Organization = (mmdb.ASNReader{Reader: db}).LookupASN(net.ParseIP(ip))
		_ = db.Close()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ptrs, err := net.DefaultResolver.LookupAddr(ctx, ip)
	if err == nil && len(ptrs) > 0 {
		m.PTR = strings.TrimSuffix(ptrs[0], ".")
		if len(m.PTR) > 253 {
			m.PTR = ""
			return m
		}
		addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip", m.PTR)
		confirmed := false
		if err == nil {
			for _, v := range addrs {
				if v.Unmap() == a.Unmap() {
					confirmed = true
					break
				}
			}
		}
		m.Scanner, m.Evidence = scannerlog.AttributePTR(m.PTR, confirmed)
	}
	if scanner, evidence := scannerlog.AttributeNetwork(a); scanner != "unknown" {
		m.Scanner = scanner
		m.Evidence = evidence
	}
	return m
}
