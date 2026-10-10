package store

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"mikan/internal/nodeapi"
	"mikan/internal/scannerlog"
	"net/netip"
	"time"
)

func (s *Store) PruneScanners(ctx context.Context, now time.Time) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM kinkan_scanners WHERE day<$1`, now.Add(-29*24*time.Hour).UTC().Truncate(24*time.Hour).Unix())
	return err
}

func (s *Store) SaveScanners(ctx context.Context, nodeID int64, snap nodeapi.ScannerSnapshot, now time.Time) error {
	if len(snap.Epoch) != 32 || len(snap.Rows) > scannerlog.MaxRows {
		return errors.New("invalid scanner snapshot")
	}
	cutoff := now.Add(-29 * 24 * time.Hour).UTC().Truncate(24 * time.Hour).Unix()
	type item struct {
		Key     string                `json:"key"`
		Day     int64                 `json:"day"`
		Count   int64                 `json:"count"`
		Payload nodeapi.ScannerRecord `json:"payload"`
	}
	batch := make([]item, 0, len(snap.Rows))
	for _, r := range snap.Rows {
		if r.Day < cutoff || r.Day > now.Add(24*time.Hour).Unix() {
			continue
		}
		if _, err := netip.ParseAddr(r.IP); err != nil || r.Count <= 0 || r.Count > 1<<50 || len(r.Inbound) > 128 || len(r.Protocol) > 32 || len(r.Reason) > 48 || len(r.Method) > 16 || len(r.PTR) > 253 || len(r.Organization) > 512 || len(r.Country) > 16 || len(r.Scanner) > 32 || len(r.ASN) > 16 || len(r.Evidence) > 64 {
			return errors.New("invalid scanner record")
		}
		key := fmt.Sprintf("%d\x00%s\x00%s\x00%s\x00%s\x00%s\x00%t", r.Day, r.IP, r.Inbound, r.Protocol, r.Reason, r.Method, r.Client)
		batch = append(batch, item{Key: fmt.Sprintf("%x", sha256.Sum256([]byte(key))), Day: r.Day, Count: r.Count, Payload: r})
	}
	raw, err := json.Marshal(batch)
	if err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended(current_schema() || ':scanner:' || $1::bigint::text,0))`, nodeID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM kinkan_scanners WHERE node_id=$1 AND day<$2`, nodeID, cutoff); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO kinkan_scanners(node_id,epoch,record_key,day,count,payload)
 SELECT $1,$2,x.key,x.day,x.count,x.payload FROM jsonb_to_recordset($3::jsonb) AS x(key TEXT,day BIGINT,count BIGINT,payload JSONB)
 ON CONFLICT(node_id,epoch,record_key) DO UPDATE SET count=GREATEST(kinkan_scanners.count,EXCLUDED.count),payload=EXCLUDED.payload || jsonb_build_object('first',LEAST((kinkan_scanners.payload->>'first')::bigint,(EXCLUDED.payload->>'first')::bigint),'last',GREATEST((kinkan_scanners.payload->>'last')::bigint,(EXCLUDED.payload->>'last')::bigint))`, nodeID, snap.Epoch, string(raw)); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM kinkan_scanners WHERE node_id=$1 AND (epoch,record_key) IN
 (SELECT epoch,record_key FROM kinkan_scanners WHERE node_id=$1 ORDER BY day DESC,record_key OFFSET $2)`, nodeID, scannerlog.MaxRows); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ScannerRows(ctx context.Context, nodeID int64, now time.Time) ([]nodeapi.ScannerRecord, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT payload,count FROM kinkan_scanners WHERE node_id=$1 AND day>=$2 ORDER BY day DESC,record_key LIMIT $3`, nodeID, now.Add(-29*24*time.Hour).UTC().Truncate(24*time.Hour).Unix(), scannerlog.MaxRows)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []nodeapi.ScannerRecord{}
	byKey := map[string]int{}
	for rows.Next() {
		var raw []byte
		var count int64
		if err := rows.Scan(&raw, &count); err != nil {
			return nil, err
		}
		var r nodeapi.ScannerRecord
		if err := json.Unmarshal(raw, &r); err != nil {
			return nil, err
		}
		r.Count = count
		key := fmt.Sprintf("%d\x00%s\x00%s\x00%s\x00%s\x00%s\x00%t", r.Day, r.IP, r.Inbound, r.Protocol, r.Reason, r.Method, r.Client)
		if i, ok := byKey[key]; ok {
			out[i].Count += r.Count
			out[i].First = min(out[i].First, r.First)
			out[i].Last = max(out[i].Last, r.Last)
		} else {
			byKey[key] = len(out)
			out = append(out, r)
		}
	}
	return out, rows.Err()
}
