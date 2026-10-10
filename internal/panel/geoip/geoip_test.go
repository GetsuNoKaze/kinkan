package geoip

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// mmdb builds a one-node IPv4 database in which every address has the same record:
// enough to check reading, updating and refusing, without a writer library.
func mmdb(dbType string, record map[string]any) []byte {
	var buf bytes.Buffer
	ptr := uint32(1 + 16) // node_count + separator + data offset 0
	for range 2 {
		buf.Write([]byte{byte(ptr >> 16), byte(ptr >> 8), byte(ptr)})
	}
	buf.Write(make([]byte, 16))
	buf.Write(encode(record))
	buf.WriteString("\xab\xcd\xefMaxMind.com")
	buf.Write(encode(map[string]any{
		"node_count": uint32(1), "record_size": uint16(24), "ip_version": uint16(4),
		"database_type": dbType, "languages": []any{"en"}, "description": map[string]any{"en": "test"},
		"binary_format_major_version": uint16(2), "binary_format_minor_version": uint16(0), "build_epoch": uint64(1),
	}))
	return buf.Bytes()
}

// encode writes MaxMind DB data: the types the tests need, sizes under 285.
func encode(v any) []byte {
	ctrl := func(typ, size int) []byte {
		low, extra := size, []byte(nil)
		if size >= 29 {
			low, extra = 29, []byte{byte(size - 29)}
		}
		out := []byte{byte(typ<<5 | low)}
		if typ > 7 {
			out = []byte{byte(low), byte(typ - 7)}
		}
		return append(out, extra...)
	}
	uint := func(typ int, n uint64) []byte {
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], n)
		s := bytes.TrimLeft(b[:], "\x00")
		return append(ctrl(typ, len(s)), s...)
	}
	switch x := v.(type) {
	case string:
		return append(ctrl(2, len(x)), x...)
	case uint16:
		return uint(5, uint64(x))
	case uint32:
		return uint(6, uint64(x))
	case uint64:
		return uint(9, x)
	case []any:
		out := ctrl(11, len(x))
		for _, e := range x {
			out = append(out, encode(e)...)
		}
		return out
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := ctrl(7, len(x))
		for _, k := range keys {
			out = append(append(out, encode(k)...), encode(x[k])...)
		}
		return out
	}
	panic("type")
}

func gz(b []byte) []byte {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	_, _ = w.Write(b)
	_ = w.Close()
	return buf.Bytes()
}

var (
	countryFile = mmdb("DBIP-Country-Lite", map[string]any{"country": map[string]any{"iso_code": "DE"}})
	asnFile     = mmdb("DBIP-ASN-Lite (compat=GeoLite2-ASN)", map[string]any{"autonomous_system_number": uint32(64500), "autonomous_system_organization": "Example GmbH"})
)

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestRefreshFetchesAndLooksUp(t *testing.T) {
	now := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.Path)
		switch r.URL.Path {
		// This month's country file is not out yet: last month's is taken.
		case "/dbip-country-lite-2026-09.mmdb.gz":
			_, _ = w.Write(gz(countryFile))
		case "/dbip-asn-lite-2026-10.mmdb.gz":
			_, _ = w.Write(gz(asnFile))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	d := New(dir, srv.URL, quiet(), func() time.Time { return now })
	if got := d.Lookup(netip.MustParseAddr("192.0.2.1")); got != (Info{}) {
		t.Fatalf("before any file: %+v", got)
	}
	if err := d.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := Info{Country: "DE", ASN: "64500", Organization: "Example GmbH"}
	if got := d.Lookup(netip.MustParseAddr("::ffff:192.0.2.1")); got != want {
		t.Fatalf("lookup = %+v, want %+v", got, want)
	}
	// Fresh files are not fetched again; a new DB opens the kept ones.
	n := len(asked)
	if err := d.Refresh(context.Background()); err != nil || len(asked) != n {
		t.Fatalf("second refresh: %v, %d requests", err, len(asked)-n)
	}
	again := New(dir, "", quiet(), func() time.Time { return now })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	again.Run(ctx)
	if got := again.Lookup(netip.MustParseAddr("192.0.2.1")); got.Country != "DE" {
		t.Fatalf("kept files: %+v", got)
	}
}

// A file of another kind, a broken one or an endless one is refused, and the kept
// database stays.
func TestRefreshRefusesWrongFiles(t *testing.T) {
	now := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	var body atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "asn") {
			_, _ = w.Write(gz(asnFile))
			return
		}
		_, _ = w.Write(body.Load().([]byte))
	}))
	defer srv.Close()
	dir := t.TempDir()
	d := New(dir, srv.URL, quiet(), func() time.Time { return now })
	body.Store(gz(countryFile))
	if err := d.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	kept := filepath.Join(dir, "geoip", "dbip-country-lite.mmdb")
	old := now.Add(-40 * 24 * time.Hour)
	for name, b := range map[string][]byte{
		"another kind": gz(asnFile),
		"not gzip":     countryFile,
		"broken":       gz(countryFile[:len(countryFile)/2]),
		"too large":    gz(make([]byte, maxSize+1)),
	} {
		_ = os.Chtimes(kept, old, old)
		body.Store(b)
		if err := d.Refresh(context.Background()); err == nil {
			t.Errorf("%s: taken", name)
		}
		if got := d.Lookup(netip.MustParseAddr("192.0.2.1")); got.Country != "DE" {
			t.Errorf("%s: the kept database is gone: %+v", name, got)
		}
	}
}

func TestNilAndOff(t *testing.T) {
	var d *DB
	if d.Lookup(netip.MustParseAddr("192.0.2.1")) != (Info{}) {
		t.Error("nil DB found something")
	}
	off := New("", BaseURL, quiet(), time.Now)
	off.Run(context.Background()) // returns at once: no data directory
}
