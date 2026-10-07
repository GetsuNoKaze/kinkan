package acme

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mikan/internal/panel/settings"
	"mikan/internal/panel/store/storetest"
	"mikan/internal/panel/tlscert"
)

// The local node takes the panel's certificate only while clients trust it unpinned
// (GitHub issue #67): Let's Encrypt yes, the self-signed fallback no.
func TestPublic(t *testing.T) {
	ctx := context.Background()
	t.Setenv("MIKAN_ACME_DIRECTORY", "http://127.0.0.1:1/directory") // Let's Encrypt is unreachable here
	st, err := storetest.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	set := settings.New(st.Q)
	if err := settings.Set(ctx, set, settings.KeyDomain, "vpn.example.com"); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	fallback, err := tlscert.LoadOrCreateSelfSigned(t.TempDir(), "vpn.example.com", now)
	if err != nil {
		t.Fatal(err)
	}
	holder := &tlscert.Holder{}
	holder.Set(fallback)
	data := t.TempDir()
	m := New(data, holder, fallback, set, slog.New(slog.NewTextHandler(io.Discard, nil)), func() time.Time { return now })
	m.ensure(ctx)
	if c := m.Public(); c != nil {
		t.Fatalf("the self-signed fallback is not public: %v", c.Leaf.Subject)
	}

	certPEM, keyPEM := ownCert(t, "vpn.example.com", now.Add(90*24*time.Hour))
	dir := filepath.Join(data, "tls", "acme")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cert.pem"), certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "key.pem"), keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	m.ensure(ctx)
	if c := m.Public(); c == nil || !tlscert.Covers(c.Leaf, "vpn.example.com") || m.Status().Kind != "letsencrypt" {
		t.Fatalf("Let's Encrypt: %+v", m.Status())
	}
}
