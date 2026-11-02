package app

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"
)

func kinkanTestCert(t *testing.T, dns []string, ips []net.IP) *tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "panel"}, DNSNames: dns, IPAddresses: ips,
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return &tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// kinkanHandshake runs one handshake against the panel's GetCertificate from addr.
func kinkanHandshake(t *testing.T, cert *tls.Certificate, from, sni string) error {
	t.Helper()
	ln, err := net.Listen("tcp", from+":0")
	if err != nil {
		t.Skip(err)
	}
	defer ln.Close()
	srv := tls.NewListener(ln, &tls.Config{GetCertificate: kinkanNamedOnly(func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return cert, nil })})
	go func() {
		if c, err := srv.Accept(); err == nil {
			_ = c.(*tls.Conn).Handshake()
			c.Close()
		}
	}()
	c, err := tls.Dial("tcp", ln.Addr().String(), &tls.Config{ServerName: sni, InsecureSkipVerify: true})
	if err == nil {
		c.Close()
	}
	return err
}

func TestKinkanPanelAnswersItsNameOnly(t *testing.T) {
	named := kinkanTestCert(t, []string{"panel.example", "*.sub.example"}, nil)
	ext := kinkanOutboundIP(t)
	for _, tc := range []struct {
		sni string
		ok  bool
		// Cloudflare in front of a subscription domain names that domain, not the panel's.
	}{{"panel.example", true}, {"PANEL.example.", true}, {"a.sub.example", true}, {"files.example", true}, {"", false}} {
		err := kinkanHandshake(t, named, ext, tc.sni)
		if tc.ok && err != nil {
			t.Errorf("SNI %q from %s: %v, want the handshake", tc.sni, ext, err)
		}
		if !tc.ok && (err == nil || !strings.Contains(err.Error(), "internal error")) {
			t.Errorf("SNI %q from %s: %v, want Caddy's internal_error alert", tc.sni, ext, err)
		}
	}
	// The container's health check dials 127.0.0.1 with no name.
	if err := kinkanHandshake(t, named, "127.0.0.1", ""); err != nil {
		t.Errorf("loopback with no SNI: %v, want the handshake", err)
	}
	// A panel without a domain is reached by its address.
	byIP := kinkanTestCert(t, nil, []net.IP{net.ParseIP(ext)})
	if err := kinkanHandshake(t, byIP, ext, ""); err != nil {
		t.Errorf("a certificate for an address, no SNI: %v, want the handshake", err)
	}
}

// kinkanOutboundIP is an address of this machine that is not loopback, so the panel sees
// a stranger; the test is skipped on a machine without one.
func kinkanOutboundIP(t *testing.T) string {
	t.Helper()
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		t.Skip(err)
	}
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil && !n.IP.IsLoopback() {
			return n.IP.String()
		}
	}
	t.Skip("no address but loopback")
	return ""
}
