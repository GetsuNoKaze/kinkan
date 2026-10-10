package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"mikan/internal/nodeapi"
	"mikan/internal/panel/auth"
	"mikan/internal/panel/domain"
	"mikan/internal/panel/geoip"
	"mikan/internal/panel/settings"
	"mikan/internal/panel/store/db"
	"mikan/internal/panel/store/storetest"
)

type fakeGeo map[netip.Addr]geoip.Info

func (f fakeGeo) Lookup(ip netip.Addr) geoip.Info { return f[ip] }

// The panel's own checks come from its address: they are marked and kept out of the
// day's count (and so out of the spike); addresses get a country and a network from the
// panel's database where the node had none.
func TestScannerJournalMarksOwnChecksAndFillsNetworks(t *testing.T) {
	ctx := context.Background()
	st, err := storetest.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	now := time.Now()
	clock := func() time.Time { return now }
	if err := domain.Seed(ctx, st, now); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Q.CreateAdmin(ctx, db.CreateAdminParams{Username: "admin", PasswordHash: "x", CreatedAt: now.Unix()}); err != nil {
		t.Fatal(err)
	}
	set := settings.New(st.Q)
	if err := settings.Set(ctx, set, settings.KeyDomain, "panel.example"); err != nil {
		t.Fatal(err)
	}
	day := now.UTC().Truncate(24 * time.Hour).Unix()
	rec := func(ip string, count int64, mod func(*nodeapi.ScannerRecord)) nodeapi.ScannerRecord {
		r := nodeapi.ScannerRecord{Day: day, IP: ip, Inbound: "tt", Protocol: "trusttunnel", Reason: "no_credentials", Count: count, First: now.Unix(), Last: now.Unix(), Scanner: "unknown"}
		if mod != nil {
			mod(&r)
		}
		return r
	}
	snap := nodeapi.ScannerSnapshot{Epoch: strings.Repeat("b", 32), Rows: []nodeapi.ScannerRecord{
		rec("203.0.113.7", 150, nil), // the panel's own check
		rec("198.51.100.9", 5, nil),  // a scanner
		rec("192.0.2.4", 3, func(r *nodeapi.ScannerRecord) { r.Country, r.ASN, r.Organization = "NL", "64501", "Node's" }),
		// A node cannot call itself the panel.
		rec("192.0.2.5", 2, func(r *nodeapi.ScannerRecord) { r.Own = true }),
	}}
	if err := st.SaveScanners(ctx, 1, snap, now); err != nil {
		t.Fatal(err)
	}
	sessions := auth.NewSessions(st.Q, clock, nil)
	token, _, err := sessions.Create(ctx, 1, "127.0.0.1", "test")
	if err != nil {
		t.Fatal(err)
	}
	geo := fakeGeo{
		netip.MustParseAddr("198.51.100.9"): {Country: "US", ASN: "398324", Organization: "Censys, Inc."},
		netip.MustParseAddr("192.0.2.4"):    {Country: "DE", ASN: "64500", Organization: "Panel's"},
	}
	handler, _, err := New(Deps{Store: st, Settings: set, Sessions: sessions, Now: clock, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), IPLimit: auth.NewLimiter(10, time.Minute, time.Minute, time.Hour),
		Resolve: func(_ context.Context, host string) ([]netip.Addr, error) {
			if host != "panel.example" {
				t.Errorf("resolved %q", host)
			}
			return []netip.Addr{netip.MustParseAddr("203.0.113.7")}, nil
		}, GeoIP: geo})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/api/v1/nodes/1/scanners", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	var view ScannerView
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	byIP := map[string]nodeapi.ScannerRecord{}
	for _, r := range view.Records {
		byIP[r.IP] = r
	}
	if !byIP["203.0.113.7"].Own || byIP["198.51.100.9"].Own || byIP["192.0.2.5"].Own {
		t.Errorf("own marks: %+v", view.Records)
	}
	if r := byIP["198.51.100.9"]; r.Country != "US" || r.ASN != "398324" || r.Organization != "Censys, Inc." {
		t.Errorf("filled network: %+v", r)
	}
	if r := byIP["192.0.2.4"]; r.Country != "NL" || r.ASN != "64501" || r.Organization != "Node's" {
		t.Errorf("the node's own lookup was replaced: %+v", r)
	}
	today := view.Days[len(view.Days)-1]
	if today.Own != 150 || today.Count != 10 || view.Spike {
		t.Errorf("today = %+v spike=%v", today, view.Spike)
	}
}
