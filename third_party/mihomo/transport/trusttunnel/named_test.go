package trusttunnel

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/metacubex/tls"
)

func namedTestCert(t *testing.T, dns []string, ips []net.IP) *tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "node"}, DNSNames: dns, IPAddresses: ips,
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return &tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

func TestNamedOnly(t *testing.T) {
	named := namedTestCert(t, []string{"node.example", "*.site.example"}, nil)
	get := NamedOnly(func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return named, nil })
	for sni, want := range map[string]bool{
		"node.example": true, "NODE.example.": true, "a.site.example": true,
		"": false, "203.0.113.5": false, "other.example": false, "site.example": false, "a.b.site.example": false,
	} {
		c, err := get(&tls.ClientHelloInfo{ServerName: sni})
		if want && (err != nil || c != named) {
			t.Errorf("SNI %q: %v %v, want the certificate", sni, c, err)
		}
		if !want && err == nil {
			t.Errorf("SNI %q: the certificate was handed out", sni)
		}
	}
	byIP := namedTestCert(t, nil, []net.IP{net.ParseIP("203.0.113.5")})
	if c, err := NamedOnly(func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return byIP, nil })(&tls.ClientHelloInfo{}); err != nil || c != byIP {
		t.Errorf("a certificate for an address: %v %v, want it for anyone", c, err)
	}
}
