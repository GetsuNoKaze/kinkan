package nodesync

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"mikan/internal/nodeapi"
	"mikan/internal/panel/store/db"
	"mikan/internal/proto"
)

// certChain issues a certificate for names from a CA of its own; trusted puts that CA
// into the roots publiclyTrusted checks against.
func certChain(t *testing.T, trusted bool, names ...string) string {
	t.Helper()
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test root"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caCert, _ := x509.ParseCertificate(caDER)
	if trusted {
		pool := x509.NewCertPool()
		pool.AddCert(caCert)
		siteRoots = pool
		t.Cleanup(func() { siteRoots = nil })
	}
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), DNSNames: names, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, leaf, caCert, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

// REALITY that masks a web server on the node's own address (a Caddy beside the node)
// moves to the node's site when the node's certificate is publicly trusted for the name.
func TestFitToSiteTakesTheNodesOwnWebServer(t *testing.T) {
	n := db.Node{PublicHost: "203.0.113.7", Domain: "node.example"}
	st := nodeapi.DesiredState{Site: &nodeapi.SiteState{Hash: "h1", HTTPPort: nodeapi.SiteHTTPPort, HTTPSPort: nodeapi.SiteHTTPSPort}}
	reality := func(dest string, names ...any) proto.Template {
		return proto.Template{"type": "vless", "reality-config": map[string]any{"dest": dest, "server-names": names}}
	}
	dest := func(t *testing.T, s *Syncer, tpl proto.Template) string {
		return s.fitToSite(tpl, st, n)["reality-config"].(map[string]any)["dest"].(string)
	}
	syncer := func(chain string) *Syncer {
		s := &Syncer{tls: func() (*nodeapi.TLSFiles, error) { return &nodeapi.TLSFiles{CertPEM: chain}, nil }}
		s.health.Store(&HealthView{OK: true, Health: nodeapi.Health{Site: &nodeapi.SiteStatus{Hash: "h1", HTTPS: "127.0.0.1:17443"}}})
		return s
	}

	trusted := syncer(certChain(t, true, "node.example"))
	for _, d := range []string{"203.0.113.7:8444", "node.example:8444"} {
		if got := dest(t, trusted, reality(d, "node.example")); got != "127.0.0.1:17443" {
			t.Errorf("%s: dest %s, want the node's site", d, got)
		}
	}
	if got := dest(t, trusted, reality("198.51.100.9:443", "node.example")); got != "198.51.100.9:443" {
		t.Errorf("another address moved: %s", got)
	}
	if got := dest(t, trusted, reality("203.0.113.7:8444", "panel.example")); got != "203.0.113.7:8444" {
		t.Errorf("a name the certificate lacks moved: %s", got)
	}
	if got := dest(t, trusted, reality("203.0.113.7:8444")); got != "203.0.113.7:8444" {
		t.Errorf("no names moved: %s", got)
	}
	selfSigned := syncer(certChain(t, false, "node.example"))
	if got := dest(t, selfSigned, reality("203.0.113.7:8444", "node.example")); got != "203.0.113.7:8444" {
		t.Errorf("a certificate nobody trusts moved REALITY: %s", got)
	}
}
