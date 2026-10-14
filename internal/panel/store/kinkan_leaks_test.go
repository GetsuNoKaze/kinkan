package store

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func leakUser(t *testing.T, s *Store, name string) int64 {
	t.Helper()
	var id int64
	err := s.DB.QueryRow(`INSERT INTO users(name,period_start,sub_token,created_at,updated_at) VALUES($1,1,$1,1,1) RETURNING id`, name).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestLeakRollingWindowDedupRetentionAndPrivacy(t *testing.T) {
	ctx := context.Background()
	s, err := OpenTest(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	uid := leakUser(t, s, "leak-user")
	other := leakUser(t, s, "other-user")
	now := time.Unix(1800000000, 0)
	p, err := s.LeakPolicy(ctx)
	if err != nil || !p.Enabled || p.Networks != 3 || p.WindowMinutes != 10 {
		t.Fatal(p, err)
	}
	add := func(user int64, at time.Time, v ...LeakSource) {
		t.Helper()
		if err := s.RecordLeakSources(ctx, user, v, at, p); err != nil {
			t.Fatal(err)
		}
	}
	add(uid, now, LeakSource{ASN: "AS1", Country: "US"}, LeakSource{ASN: "AS1", Country: "US"}, LeakSource{})
	add(uid, now.Add(time.Minute), LeakSource{ASN: "AS2", Country: "US"})
	add(other, now, LeakSource{ASN: "AS3", Country: "US"})
	e, _ := s.LeakEvents(ctx, uid, now.Add(time.Minute))
	if len(e) != 0 {
		t.Fatal("duplicate ASN or other user counted", e)
	}
	add(uid, now.Add(2*time.Minute), LeakSource{ASN: "AS3", Country: "US"})
	e, err = s.LeakEvents(ctx, uid, now.Add(2*time.Minute))
	if err != nil || len(e) != 1 || len(e[0].Networks) != 3 {
		t.Fatal(e, err)
	}
	id := e[0].ID
	add(uid, now.Add(3*time.Minute), LeakSource{ASN: "AS3", Country: "US"})
	e, _ = s.LeakEvents(ctx, uid, now.Add(3*time.Minute))
	if len(e) != 1 || e[0].ID != id {
		t.Fatal("repeat created event spam", e)
	}
	add(uid, now.Add(15*time.Minute), LeakSource{ASN: "AS4", Country: "US"})
	e, _ = s.LeakEvents(ctx, uid, now.Add(15*time.Minute))
	if len(e) != 1 {
		t.Fatal("expired activity counted", e)
	}
	raw, _ := json.Marshal(e)
	if strings.Contains(string(raw), "ip") || strings.Contains(string(raw), "destination") {
		t.Fatal("private address field in events", string(raw))
	}
	if err := s.PruneLeaks(ctx, now.Add(31*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	e, _ = s.LeakEvents(ctx, uid, now.Add(31*24*time.Hour))
	if len(e) != 0 {
		t.Fatal("events did not expire", e)
	}
	var n int
	if err := s.DB.QueryRow(`SELECT count(*) FROM kinkan_leak_observations`).Scan(&n); err != nil || n != 0 {
		t.Fatal("observations did not expire", n, err)
	}
}

func TestLeakCountriesDisabledPolicyAndUserDeletion(t *testing.T) {
	ctx := context.Background()
	s, err := OpenTest(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	uid := leakUser(t, s, "countries")
	now := time.Unix(1800000000, 0)
	p := LeakPolicy{Enabled: true, Networks: 4, Countries: 2, WindowMinutes: 5}
	if err := s.SetLeakPolicy(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordLeakSources(ctx, uid, []LeakSource{{ASN: "AS1", Country: "US"}, {ASN: "AS1", Country: "DE"}}, now, p); err != nil {
		t.Fatal(err)
	}
	e, _ := s.LeakEvents(ctx, uid, now)
	if len(e) != 1 || len(e[0].Countries) != 2 || len(e[0].Networks) != 1 {
		t.Fatal("country threshold failed", e)
	}
	p.Enabled = false
	if err := s.RecordLeakSources(ctx, uid, []LeakSource{{ASN: "AS2", Country: "FR"}}, now.Add(time.Minute), p); err != nil {
		t.Fatal(err)
	}
	e, _ = s.LeakEvents(ctx, uid, now.Add(time.Minute))
	if e[0].LastSeen != now.Unix() {
		t.Fatal("disabled detector changed history", e)
	}
	p.Networks = 1
	if err := s.SetLeakPolicy(ctx, p); err == nil {
		t.Fatal("invalid policy accepted")
	}
	if _, err := s.DB.Exec(`DELETE FROM users WHERE id=$1`, uid); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = s.DB.QueryRow(`SELECT count(*) FROM kinkan_leak_events`).Scan(&n)
	if n != 0 {
		t.Fatal("deleted user's events remain")
	}
}
