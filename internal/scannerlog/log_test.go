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
	for i := 0; i < MaxRows+5; i++ {
		r.IP = fmt.Sprintf("198.18.%d.%d", i/256, i%256)
		j.Record(r, now)
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
