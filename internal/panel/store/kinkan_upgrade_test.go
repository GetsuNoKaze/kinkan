package store

import (
	"context"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

// tt.13 shipped PostgreSQL migrations 1..15 and the same four Kinkan
// migrations. Preserve real old-schema rows when applying the beta's migrations,
// rather than only checking that an empty database can be created.
func TestUpgradeFromTT13PreservesData(t *testing.T) {
	ctx := context.Background()
	s, err := OpenTest(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var schema string
	if err := s.DB.QueryRowContext(ctx, "SELECT current_schema()").Scan(&schema); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.ExecContext(ctx, "DROP SCHEMA "+quote(schema)+" CASCADE; CREATE SCHEMA "+quote(schema)); err != nil {
		t.Fatal(err)
	}
	old := fstest.MapFS{}
	names, err := fs.Glob(postgresFS, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		// Historical migrations are immutable; these files are byte-identical
		// to v0.5.0.5-tt.13. Exclude every migration introduced after that tag.
		if strings.SplitN(name, "_", 2)[0] > "0015" {
			continue
		}
		raw, err := fs.ReadFile(postgresFS, name)
		if err != nil {
			t.Fatal(err)
		}
		old[name] = &fstest.MapFile{Data: raw}
	}
	if err := migrateUp(ctx, s.DB, old); err != nil {
		t.Fatal("create tt.13 schema:", err)
	}
	if v, err := schemaVersion(ctx, s.DB); err != nil || v != 15 {
		t.Fatalf("old schema version = %d, %v; want 15", v, err)
	}
	const seed = `
INSERT INTO admins(username,password_hash,created_at) VALUES('old-admin','old-password-hash',1);
INSERT INTO settings(key,value) VALUES('brand','old-brand');
INSERT INTO nodes(name,domain,cert_sha256,created_at,updated_at) VALUES('old-node','node.example.com','old-cert',1,1);
INSERT INTO slots(name,uuid,secret,state,created_at) VALUES('s000001','00000000-0000-4000-8000-000000000001','old-secret','assigned',1);
INSERT INTO slot_counter(id,last) VALUES(1,1);
INSERT INTO users(name,period_start,used_up,used_down,total_up,total_down,sub_token,slot_id,created_at,updated_at,unbound_at)
 VALUES('old-user',1,123,456,789,987,'old-sub-token',1,1,1,100);
INSERT INTO bound_devices(user_id,hwid,slot_id,created_at,last_seen) VALUES(1,'old-phone',1,1,1);
INSERT INTO inbounds(node_id,name,preset,port,settings,config,created_at,updated_at)
 VALUES(2,'old-tt','trusttunnel','4443','{}','{"type":"trusttunnel","fallback":"127.0.0.1:8444"}',1,1);
INSERT INTO kinkan_sites(name,hash,size,files,archive,created_at) VALUES('old-site','site-hash',3,1,decode('616263','hex'),1);
INSERT INTO kinkan_node_sites(node_id,site_id) VALUES(2,1);
INSERT INTO kinkan_scanners(node_id,epoch,record_key,day,count,payload) VALUES(2,'old-epoch','old-key',1,7,'{"kind":"auth"}');
`
	if _, err := s.DB.ExecContext(ctx, seed); err != nil {
		t.Fatal("seed tt.13 rows:", err)
	}
	for range 2 { // A second startup must not duplicate the unbind-history import.
		if err := migrateUp(ctx, s.DB, postgresFS); err != nil {
			t.Fatal("upgrade:", err)
		}
	}
	checks := []string{
		`SELECT username='old-admin' AND password_hash='old-password-hash' FROM admins WHERE id=1`,
		`SELECT value='old-brand' FROM settings WHERE key='brand'`,
		`SELECT name='old-user' AND sub_token='old-sub-token' AND slot_id=1 AND used_up=123 AND used_down=456 AND total_up=789 AND total_down=987 AND device_mode='bound' AND speed_limit IS NULL FROM users WHERE id=1`,
		`SELECT uuid='00000000-0000-4000-8000-000000000001' AND secret='old-secret' AND state='assigned' FROM slots WHERE id=1`,
		`SELECT hwid='old-phone' AND slot_id=1 AND name='' FROM bound_devices WHERE id=1`,
		`SELECT domain='node.example.com' AND cert_sha256='old-cert' AND fair_share=0 AND channel_mbps IS NULL FROM nodes WHERE id=2`,
		`SELECT config='{"type":"trusttunnel","fallback":"127.0.0.1:8444"}' AND port='4443' FROM inbounds WHERE id=1`,
		`SELECT s.hash='site-hash' AND s.archive=decode('616263','hex') FROM kinkan_sites s JOIN kinkan_node_sites n ON n.site_id=s.id WHERE n.node_id=2`,
		`SELECT count=7 AND payload->>'kind'='auth' FROM kinkan_scanners WHERE node_id=2 AND epoch='old-epoch' AND record_key='old-key'`,
		`SELECT count(*)=1 AND min(at)=100 FROM device_unbinds WHERE user_id=1`,
	}
	for i, query := range checks {
		var preserved bool
		if err := s.DB.QueryRowContext(ctx, query).Scan(&preserved); err != nil || !preserved {
			t.Errorf("preservation check %d: result=%v, error=%v", i+1, preserved, err)
		}
	}
}
