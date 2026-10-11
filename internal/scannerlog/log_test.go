package scannerlog

import (
	"fmt"
	"mikan/internal/nodeapi"
	"net/netip"
	"path/filepath"
	"testing"
	"time"
)

func TestJournalBoundsRetentionAndRestart(t *testing.T) {
	now := time.Now()
	path := filepath.Join(t.TempDir(), "scanners.json")
	j, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	r := nodeapi.ScannerRecord{IP: "::ffff:192.0.2.1", Inbound: "tt", Protocol: "trusttunnel", Reason: "wrong_credentials", Method: "GET"}
	j.Record(r, now)
	j.Record(r, now.Add(time.Second))
	j.Record(r, now.Add(-31*24*time.Hour))
	s := j.Snapshot(now)
	if len(s.Rows) != 1 || s.Rows[0].Count != 2 || s.Rows[0].IP != "192.0.2.1" {
		t.Fatal(s)
	}
	if err := j.Persist(now); err != nil {
		t.Fatal(err)
	}
	restored, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := restored.Snapshot(now); got.Epoch != s.Epoch || got.Rows[0].Count != 2 {
		t.Fatal(got)
	}
	// Spread over days: one day holds at most MaxRowsPerDay.
	for i := 0; i < MaxRows+5; i++ {
		r.IP = fmt.Sprintf("198.18.%d.%d", i/256, i%256)
		j.Record(r, now.Add(-time.Duration(i/MaxRowsPerDay)*24*time.Hour))
	}
	s = j.Snapshot(now)
	if len(s.Rows) != MaxRows || s.Dropped < 5 {
		t.Fatalf("unbounded journal: %d dropped=%d", len(s.Rows), s.Dropped)
	}
	if got := j.Snapshot(now.Add(32 * 24 * time.Hour)); len(got.Rows) != 0 {
		t.Fatal("expired rows retained")
	}
}
func TestAttributionDoesNotTrustForgedPTR(t *testing.T) {
	for _, ptr := range []string{"scanner.shodan.io.attacker.example", "evilshodan.io", "scanner.shodan.io"} {
		if name, _ := AttributePTR(ptr, false); name != "unknown" {
			t.Fatal("unverified PTR")
		}
	}
	if name, _ := AttributePTR("scanner.shodan.io.attacker.example", true); name != "unknown" {
		t.Fatal("suffix confusion")
	}
	if name, _ := AttributePTR("SCANNER.SHODAN.IO.", true); name != "Shodan" {
		t.Fatal(name)
	}
	if name, _ := AttributeNetwork(netip.MustParseAddr("167.94.138.4")); name != "Censys" {
		t.Fatal(name)
	}
	if name, _ := AttributeNetwork(netip.MustParseAddr("8.8.8.8")); name != "unknown" {
		t.Fatal(name)
	}
}

// A flood cannot keep other sources out: made-up methods fold into OTHER, one source
// (an IPv4 address or an IPv6 /64) gets at most MaxRowsPerSourceDay rows a day, and one
// day at most MaxRowsPerDay, so the next day is recorded again.
func TestJournalFloodLeavesRoomForOthers(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	j, err := Open(filepath.Join(t.TempDir(), "scanners.json"))
	if err != nil {
		t.Fatal(err)
	}
	rec := func(ip, method, inbound string, at time.Time) bool {
		return j.Record(nodeapi.ScannerRecord{IP: ip, Inbound: inbound, Protocol: "trusttunnel", Reason: "no_credentials", Method: method}, at)
	}
	for i := range 10000 {
		rec("198.51.100.7", fmt.Sprintf("M%05d", i), "tt", now)
	}
	rows := j.Snapshot(now).Rows
	if len(rows) != 1 || rows[0].Method != "OTHER" || rows[0].Count != 10000 {
		t.Fatalf("made-up methods: %d rows, first %+v", len(rows), rows[0])
	}
	// One source spread over many inbounds stops at its share.
	for i := range 100 {
		rec("198.51.100.7", "GET", fmt.Sprintf("in%d", i), now)
	}
	// Many addresses of one IPv6 /64 count as one source.
	for i := range 100 {
		rec(fmt.Sprintf("2001:db8:1:2::%x", i+1), "GET", "tt", now)
	}
	per := map[string]int{}
	for _, r := range j.Snapshot(now).Rows {
		ip := netip.MustParseAddr(r.IP)
		per[sourceOf(ip).String()]++
	}
	if per["198.51.100.7/32"] != MaxRowsPerSourceDay || per["2001:db8:1:2::/64"] != MaxRowsPerSourceDay {
		t.Fatalf("per source: %v", per)
	}
	// Another address is still recorded the same day.
	if !rec("203.0.113.9", "GET", "tt", now) {
		t.Fatal("another source was kept out")
	}
	// A day full of distinct sources leaves the next day open.
	for i := range MaxRowsPerDay {
		rec(fmt.Sprintf("10.%d.%d.1", i/250, i%250), "GET", "tt", now)
	}
	if rec("203.0.113.10", "GET", "tt", now) {
		t.Error("the day's cap let one more row in")
	}
	if !rec("203.0.113.10", "GET", "tt", now.Add(24*time.Hour)) {
		t.Error("the next day is kept out")
	}
}
