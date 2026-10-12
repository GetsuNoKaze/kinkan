package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"time"
)

// Only network, country and time enter these tables. No IP, URL or traffic destination.
type LeakPolicy struct {
	Enabled       bool `json:"enabled"`
	Networks      int  `json:"networks" minimum:"2" maximum:"100"`
	Countries     int  `json:"countries" minimum:"2" maximum:"100"`
	WindowMinutes int  `json:"window_minutes" minimum:"1" maximum:"60"`
}

func (p LeakPolicy) Valid() bool {
	return p.Networks >= 2 && p.Networks <= 100 && p.Countries >= 2 && p.Countries <= 100 && p.WindowMinutes >= 1 && p.WindowMinutes <= 60
}

type LeakSource struct{ ASN, Country string }
type LeakEvent struct {
	ID               int64    `json:"id"`
	UserID           int64    `json:"user_id"`
	FirstSeen        int64    `json:"first_seen"`
	LastSeen         int64    `json:"last_seen"`
	WindowMinutes    int      `json:"window_minutes"`
	NetworkThreshold int      `json:"network_threshold"`
	CountryThreshold int      `json:"country_threshold"`
	Networks         []string `json:"networks"`
	Countries        []string `json:"countries"`
}

func (s *Store) LeakPolicy(ctx context.Context) (p LeakPolicy, err error) {
	err = s.DB.QueryRowContext(ctx, `SELECT enabled,networks,countries,window_minutes FROM kinkan_leak_policy WHERE id=1`).Scan(&p.Enabled, &p.Networks, &p.Countries, &p.WindowMinutes)
	return
}
func (s *Store) SetLeakPolicy(ctx context.Context, p LeakPolicy) error {
	if !p.Valid() {
		return errors.New("invalid leak detector policy")
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE kinkan_leak_policy SET enabled=$1,networks=$2,countries=$3,window_minutes=$4 WHERE id=1`, p.Enabled, p.Networks, p.Countries, p.WindowMinutes)
	return err
}

func sortedLeakSet(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for k := range values {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// RecordLeakSources uses a rolling activity window. A rapid network switch can be a
// legitimate user: events are suspicion, never proof, and never revoke any key.
func (s *Store) RecordLeakSources(ctx context.Context, userID int64, sources []LeakSource, now time.Time, p LeakPolicy) error {
	if !p.Valid() {
		return errors.New("invalid leak detector policy")
	}
	if !p.Enabled || len(sources) == 0 {
		return nil
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Serialize collectors for one user; deletion also cannot race this snapshot.
	var owner int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, userID).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, v := range sources {
		if v.ASN == "" && v.Country == "" {
			continue
		}
		if len(v.ASN) > 32 || len(v.Country) > 2 {
			continue
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO kinkan_leak_observations(user_id,asn,country,last_seen) VALUES($1,$2,$3,$4)
 ON CONFLICT(user_id,asn,country) DO UPDATE SET last_seen=greatest(kinkan_leak_observations.last_seen,EXCLUDED.last_seen)`, userID, v.ASN, v.Country, now.Unix())
		if err != nil {
			return err
		}
	}
	cutoff := now.Add(-time.Duration(p.WindowMinutes) * time.Minute).Unix()
	rows, err := tx.QueryContext(ctx, `SELECT asn,country FROM kinkan_leak_observations WHERE user_id=$1 AND last_seen>$2 AND last_seen<=$3`, userID, cutoff, now.Unix())
	if err != nil {
		return err
	}
	networks, countries := map[string]bool{}, map[string]bool{}
	for rows.Next() {
		var a, c string
		if err = rows.Scan(&a, &c); err != nil {
			rows.Close()
			return err
		}
		if a != "" {
			networks[a] = true
		}
		if c != "" {
			countries[c] = true
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(networks) < p.Networks && len(countries) < p.Countries {
		return tx.Commit()
	}
	nraw, _ := json.Marshal(sortedLeakSet(networks))
	craw, _ := json.Marshal(sortedLeakSet(countries))
	var lastID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM kinkan_leak_events WHERE user_id=$1 AND last_seen>$2 AND last_seen<=$3 AND window_minutes=$4 AND network_threshold=$5 AND country_threshold=$6 ORDER BY last_seen DESC,id DESC LIMIT 1`, userID, cutoff, now.Unix(), p.WindowMinutes, p.Networks, p.Countries).Scan(&lastID)
	if err == nil {
		_, err = tx.ExecContext(ctx, `UPDATE kinkan_leak_events SET last_seen=$2,networks=$3,countries=$4 WHERE id=$1`, lastID, now.Unix(), string(nraw), string(craw))
	} else if errors.Is(err, sql.ErrNoRows) {
		_, err = tx.ExecContext(ctx, `INSERT INTO kinkan_leak_events(user_id,first_seen,last_seen,window_minutes,network_threshold,country_threshold,networks,countries) VALUES($1,$2,$2,$3,$4,$5,$6,$7)`, userID, now.Unix(), p.WindowMinutes, p.Networks, p.Countries, string(nraw), string(craw))
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) LeakEvents(ctx context.Context, userID int64, now time.Time) ([]LeakEvent, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,user_id,first_seen,last_seen,window_minutes,network_threshold,country_threshold,networks,countries FROM kinkan_leak_events WHERE user_id=$1 AND last_seen>$2 ORDER BY last_seen DESC,id DESC LIMIT 100`, userID, now.Add(-30*24*time.Hour).Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LeakEvent{}
	for rows.Next() {
		var e LeakEvent
		var n, c string
		if err = rows.Scan(&e.ID, &e.UserID, &e.FirstSeen, &e.LastSeen, &e.WindowMinutes, &e.NetworkThreshold, &e.CountryThreshold, &n, &c); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(n), &e.Networks); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(c), &e.Countries); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) PruneLeaks(ctx context.Context, now time.Time) error {
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM kinkan_leak_observations WHERE last_seen<=$1`, now.Add(-time.Hour).Unix()); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx, `DELETE FROM kinkan_leak_events WHERE last_seen<=$1`, now.Add(-30*24*time.Hour).Unix())
	return err
}
