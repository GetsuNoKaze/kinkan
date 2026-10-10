// Package scannerlog keeps a bounded journal of rejected authentication, with no
// credentials or traffic. Snapshots have monotonic counts within a persistent epoch.
package scannerlog

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/netip"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"mikan/internal/fsutil"
	"mikan/internal/nodeapi"
)

const MaxRows = 10000
const Keep = 30 * 24 * time.Hour

type Metadata struct{ ASN, Organization, Country, PTR, Scanner, Evidence string }
type key struct {
	Day                                   int64
	IP, Inbound, Protocol, Reason, Method string
	Client                                bool
}
type Journal struct {
	mu        sync.Mutex
	persistMu sync.Mutex
	rows      map[key]nodeapi.ScannerRecord
	meta      map[string]Metadata
	epoch     string
	dropped   int64
	path      string
	lastSave  time.Time
}

func Open(path string) (*Journal, error) {
	j := &Journal{rows: map[key]nodeapi.ScannerRecord{}, meta: map[string]Metadata{}, path: path}
	file, err := os.Open(path)
	var b []byte
	if err == nil {
		defer file.Close()
		b, err = io.ReadAll(io.LimitReader(file, (16<<20)+1))
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if len(b) > 16<<20 {
		return nil, errors.New("scanner snapshot too large")
	}
	if err == nil {
		var s nodeapi.ScannerSnapshot
		if err := json.Unmarshal(b, &s); err != nil {
			return nil, err
		}
		if len(s.Rows) > MaxRows {
			return nil, errors.New("too many scanner rows")
		}
		j.epoch = s.Epoch
		j.dropped = s.Dropped
		for _, r := range s.Rows {
			if _, err := netip.ParseAddr(r.IP); err != nil {
				continue
			}
			j.rows[keyOf(r)] = r
		}
	}
	if j.epoch == "" {
		b := make([]byte, 16)
		if _, err := rand.Read(b); err != nil {
			return nil, err
		}
		j.epoch = hex.EncodeToString(b)
	}
	return j, nil
}
func keyOf(r nodeapi.ScannerRecord) key {
	return key{r.Day, r.IP, r.Inbound, r.Protocol, r.Reason, r.Method, r.Client}
}
func (j *Journal) Record(r nodeapi.ScannerRecord, now time.Time) bool {
	if len(r.Inbound) > 128 || len(r.Protocol) > 32 || len(r.Reason) > 48 || len(r.Method) > 16 {
		return false
	}
	ip, err := netip.ParseAddr(r.IP)
	if err != nil {
		return false
	}
	r.IP = ip.Unmap().String()
	r.Day = now.UTC().Truncate(24 * time.Hour).Unix()
	k := keyOf(r)
	j.mu.Lock()
	defer j.mu.Unlock()
	if old, ok := j.rows[k]; ok {
		old.Count++
		old.Last = now.Unix()
		j.rows[k] = old
		return true
	}
	if len(j.rows) >= MaxRows {
		j.prune(now)
		if len(j.rows) >= MaxRows {
			j.dropped++
			return false
		}
	}
	r.Count = 1
	r.First = now.Unix()
	r.Last = r.First
	r.Scanner = "unknown"
	j.rows[k] = r
	return true
}
func (j *Journal) SetMetadata(ip string, m Metadata) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if _, exists := j.meta[ip]; exists || len(j.meta) < MaxRows {
		j.meta[ip] = m
	}
}
func (j *Journal) prune(now time.Time) {
	cut := now.Add(-29 * 24 * time.Hour).UTC().Truncate(24 * time.Hour).Unix()
	alive := map[string]bool{}
	for k := range j.rows {
		if k.Day < cut {
			delete(j.rows, k)
		} else {
			alive[k.IP] = true
		}
	}
	for ip := range j.meta {
		if !alive[ip] {
			delete(j.meta, ip)
		}
	}
}
func (j *Journal) Snapshot(now time.Time) nodeapi.ScannerSnapshot {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.prune(now)
	s := nodeapi.ScannerSnapshot{Epoch: j.epoch, Dropped: j.dropped, Rows: make([]nodeapi.ScannerRecord, 0, len(j.rows))}
	for _, r := range j.rows {
		if m, ok := j.meta[r.IP]; ok {
			r.ASN = m.ASN
			r.Organization = m.Organization
			r.Country = m.Country
			r.PTR = m.PTR
			r.Scanner = m.Scanner
			r.Evidence = m.Evidence
		}
		s.Rows = append(s.Rows, r)
	}
	sort.Slice(s.Rows, func(i, k int) bool {
		a, b := s.Rows[i], s.Rows[k]
		if a.Day != b.Day {
			return a.Day > b.Day
		}
		if a.IP != b.IP {
			return a.IP < b.IP
		}
		return a.Inbound < b.Inbound
	})
	return s
}
func (j *Journal) Persist(now time.Time) error {
	j.mu.Lock()
	if !j.lastSave.IsZero() && now.Sub(j.lastSave) < time.Minute {
		j.mu.Unlock()
		return nil
	}
	j.lastSave = now
	j.mu.Unlock()
	_, err := j.Export(now)
	return err
}

// Export durably saves the exact counters before the panel can acknowledge them
// by storing a snapshot. A restart cannot rewind counts below what it received.
func (j *Journal) Export(now time.Time) (nodeapi.ScannerSnapshot, error) {
	j.persistMu.Lock()
	defer j.persistMu.Unlock()
	s := j.Snapshot(now)
	b, err := json.Marshal(s)
	if err != nil {
		return s, err
	}
	return s, fsutil.WriteFileAtomic(j.path, b, 0600)
}

// AttributePTR requires a forward-confirmed reverse name and exact DNS suffix
// boundaries. It is an attribution hint, never an authentication or firewall rule.
func AttributePTR(ptr string, confirmed bool) (string, string) {
	if !confirmed {
		return "unknown", ""
	}
	name := strings.TrimSuffix(strings.ToLower(ptr), ".")
	for _, r := range []struct{ suffix, label string }{{"censys-scanner.com", "Censys"}, {"shodan.io", "Shodan"}, {"rapid7.com", "Rapid7"}, {"binaryedge.ninja", "BinaryEdge"}, {"binaryedge.io", "BinaryEdge"}} {
		if name == r.suffix || strings.HasSuffix(name, "."+r.suffix) {
			return r.label, "forward-confirmed PTR"
		}
	}
	return "unknown", ""
}
