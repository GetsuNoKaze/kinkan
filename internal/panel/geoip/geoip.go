// Package geoip is Kinkan's: the country and network of the scanner journal's addresses,
// looked up on the panel in DB-IP's free databases (IP to Country Lite and IP to ASN
// Lite, CC BY 4.0, https://db-ip.com) so that nodes need none. The panel keeps one copy
// under its data directory and fetches the new month's once a month; without them the
// journal shows what the nodes found, as before.
package geoip

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/oschwald/maxminddb-golang"

	"mikan/internal/fsutil"
)

// BaseURL is where DB-IP publishes the free databases, one file per month.
const BaseURL = "https://download.db-ip.com/free"

// maxSize bounds one unpacked database; the Lite ones are about 10 MB.
const maxSize = 128 << 20

// How often the files are looked at, and when a kept one counts as old: DB-IP publishes
// a month's at its start, so a file older than that has a newer one.
var (
	checkEvery = 24 * time.Hour
	retryAfter = time.Hour
	maxAge     = 32 * 24 * time.Hour
)

type kind struct {
	name   string // DB-IP's name and the kept file's
	dbType string // the database_type the file must have
}

var (
	countryDB = kind{"dbip-country-lite", "DBIP-Country-Lite"}
	asnDB     = kind{"dbip-asn-lite", "DBIP-ASN-Lite"}
)

// DB looks addresses up; a nil *DB or one without files finds nothing.
type DB struct {
	dir    string
	base   string
	client *http.Client
	log    *slog.Logger
	now    func() time.Time

	mu      sync.RWMutex
	readers map[string]*maxminddb.Reader
}

// New keeps the files under dataDir/geoip and fetches them from base; dataDir or base ""
// turns fetching off.
func New(dataDir, base string, log *slog.Logger, now func() time.Time) *DB {
	d := &DB{base: strings.TrimSuffix(base, "/"), client: &http.Client{Timeout: 2 * time.Minute}, log: log, now: now, readers: map[string]*maxminddb.Reader{}}
	if dataDir != "" {
		d.dir = filepath.Join(dataDir, "geoip")
	}
	return d
}

// Info is what is known of an address.
type Info struct {
	Country      string
	ASN          string
	Organization string
}

// Lookup finds an address's country (ISO code) and network.
func (d *DB) Lookup(ip netip.Addr) Info {
	var info Info
	if d == nil || !ip.IsValid() {
		return info
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	addr := net.IP(ip.Unmap().AsSlice())
	if r := d.readers[countryDB.name]; r != nil {
		var rec struct {
			Country struct {
				ISOCode string `maxminddb:"iso_code"`
			} `maxminddb:"country"`
		}
		if r.Lookup(addr, &rec) == nil {
			info.Country = rec.Country.ISOCode
		}
	}
	if r := d.readers[asnDB.name]; r != nil {
		var rec struct {
			Number       uint   `maxminddb:"autonomous_system_number"`
			Organization string `maxminddb:"autonomous_system_organization"`
		}
		if r.Lookup(addr, &rec) == nil && rec.Number != 0 {
			info.ASN = strconv.FormatUint(uint64(rec.Number), 10)
			info.Organization = rec.Organization
		}
	}
	return info
}

// Run opens the kept files, then fetches missing and old ones: soon after the start,
// then once a day (an hour after a failure).
func (d *DB) Run(ctx context.Context) {
	if d.dir == "" {
		return
	}
	for _, k := range []kind{countryDB, asnDB} {
		if err := d.open(k); err != nil && !errors.Is(err, os.ErrNotExist) {
			d.log.Warn("geoip: a kept database does not open", "db", k.name, "err", err)
		}
	}
	if d.base == "" {
		return
	}
	t := time.NewTimer(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			d.close()
			return
		case <-t.C:
			next := checkEvery
			if err := d.Refresh(ctx); err != nil {
				d.log.Warn("geoip: update", "err", err)
				next = retryAfter
			}
			t.Reset(next)
		}
	}
}

// Refresh fetches every database that is missing or older than a month.
func (d *DB) Refresh(ctx context.Context) error {
	var errs []error
	for _, k := range []kind{countryDB, asnDB} {
		if fi, err := os.Stat(d.path(k)); err == nil && d.now().Sub(fi.ModTime()) < maxAge {
			continue
		}
		if err := d.fetch(ctx, k); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", k.name, err))
		}
	}
	return errors.Join(errs...)
}

func (d *DB) path(k kind) string { return filepath.Join(d.dir, k.name+".mmdb") }

// fetch takes this month's file, or last month's while this month's is not out yet.
func (d *DB) fetch(ctx context.Context, k kind) error {
	now := d.now().UTC()
	var err error
	for _, month := range []time.Time{now, now.AddDate(0, 0, -now.Day())} {
		var data []byte
		if data, err = d.download(ctx, fmt.Sprintf("%s/%s-%s.mmdb.gz", d.base, k.name, month.Format("2006-01"))); err != nil {
			continue
		}
		if err = check(data, k); err != nil {
			return err
		}
		if err = os.MkdirAll(d.dir, 0o700); err != nil {
			return err
		}
		if err = fsutil.WriteFileAtomic(d.path(k), data, 0o600); err != nil {
			return err
		}
		d.log.Info("geoip: database updated", "db", k.name, "month", month.Format("2006-01"))
		return d.open(k)
	}
	return err
}

func (d *DB) download(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	zr, err := gzip.NewReader(io.LimitReader(resp.Body, maxSize))
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(zr, maxSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxSize {
		return nil, errors.New("database too large")
	}
	return data, nil
}

// check takes only a database of the expected kind whose whole tree and data read back.
func check(data []byte, k kind) error {
	r, err := maxminddb.FromBytes(data)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(r.Metadata.DatabaseType, k.dbType) {
		return fmt.Errorf("database type %q", r.Metadata.DatabaseType)
	}
	return r.Verify()
}

// open reads a kept file into memory (no mapping: a reader that is swapped out
// must not leave a lookup on an unmapped file) and puts it in place.
func (d *DB) open(k kind) error {
	data, err := os.ReadFile(d.path(k))
	if err != nil {
		return err
	}
	if len(data) > maxSize {
		return errors.New("database too large")
	}
	if err := check(data, k); err != nil {
		return err
	}
	r, err := maxminddb.FromBytes(data)
	if err != nil {
		return err
	}
	d.mu.Lock()
	d.readers[k.name] = r
	d.mu.Unlock()
	return nil
}

func (d *DB) close() {
	d.mu.Lock()
	d.readers = map[string]*maxminddb.Reader{}
	d.mu.Unlock()
}
