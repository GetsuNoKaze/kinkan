package leakwatch

import (
	"context"
	"io"
	"log/slog"
	"net/netip"
	"testing"
	"time"

	"mikan/internal/nodeapi"
	"mikan/internal/panel/geoip"
	"mikan/internal/panel/store/storetest"
)

type testGeo struct{}

func (testGeo) Lookup(ip netip.Addr) geoip.Info {
	return geoip.Info{ASN: "AS" + ip.String(), Country: "US"}
}

func TestCollectorMapsOwnAndBoundSlotsAndPersistsAcrossRestart(t *testing.T) {
	ctx := context.Background()
	st, err := storetest.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var slot, device, user int64
	if err := st.DB.QueryRow(`INSERT INTO slots(name,uuid,secret,state,created_at) VALUES('own','uuid-own','secret','assigned',1) RETURNING id`).Scan(&slot); err != nil {
		t.Fatal(err)
	}
	if err := st.DB.QueryRow(`INSERT INTO slots(name,uuid,secret,state,created_at) VALUES('device','uuid-device','secret','assigned',1) RETURNING id`).Scan(&device); err != nil {
		t.Fatal(err)
	}
	if err := st.DB.QueryRow(`INSERT INTO users(name,period_start,sub_token,slot_id,created_at,updated_at) VALUES('u',1,'token',$1,1,1) RETURNING id`, slot).Scan(&user); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.Exec(`INSERT INTO bound_devices(user_id,hwid,slot_id,created_at,last_seen) VALUES($1,'hwid',$2,1,1)`, user, device); err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1800000000, 0)
	online := func() map[string]nodeapi.Online {
		return map[string]nodeapi.Online{"own": {Conns: 1, IPs: []string{"8.8.8.8"}}, "device": {Conns: 2, IPs: []string{"1.1.1.1", "9.9.9.9"}}, "unknown": {Conns: 1, IPs: []string{"4.4.4.4"}}}
	}
	w := Watcher{Store: st, Online: online, GeoIP: testGeo{}, Now: func() time.Time { return now }, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if err := w.Collect(ctx); err != nil {
		t.Fatal(err)
	}
	e, err := st.LeakEvents(ctx, user, now)
	if err != nil || len(e) != 1 || len(e[0].Networks) != 3 {
		t.Fatal(e, err)
	}
	// Recreating the watcher must not forget activity or duplicate the event.
	w2 := w
	now = now.Add(time.Minute)
	if err := w2.Collect(ctx); err != nil {
		t.Fatal(err)
	}
	e2, _ := st.LeakEvents(ctx, user, now)
	if len(e2) != 1 || e2[0].ID != e[0].ID {
		t.Fatal(e2)
	}
	w2.Online = nil
	now = now.Add(31 * 24 * time.Hour)
	if err := w2.Collect(ctx); err != nil {
		t.Fatal(err)
	}
	e2, _ = st.LeakEvents(ctx, user, now)
	if len(e2) != 0 {
		t.Fatal("retention stopped without nodes", e2)
	}
}
