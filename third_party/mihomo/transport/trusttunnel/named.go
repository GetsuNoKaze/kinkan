package trusttunnel

import (
	"crypto/x509"
	"errors"
	"strings"

	"github.com/metacubex/tls"
)

// ErrNoCertificateForName fails a handshake that does not name the server.
var ErrNoCertificateForName = errors.New("trusttunnel: no certificate for this name")

// NamedOnly hands the certificate out only to a client that names one of the names in
// it, a wildcard covering one label. Without SNI or with a stranger's name the handshake
// fails with internal_error, as on a Caddy site configured under its domain alone. A
// certificate without DNS names (one for an IP address) is handed to anyone.
func NamedOnly(get func(*tls.ClientHelloInfo) (*tls.Certificate, error)) func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		c, err := get(hello)
		if err != nil || c == nil || len(c.Certificate) == 0 {
			return c, err
		}
		leaf := c.Leaf
		if leaf == nil {
			if leaf, err = x509.ParseCertificate(c.Certificate[0]); err != nil {
				return c, nil
			}
		}
		if len(leaf.DNSNames) == 0 || len(leaf.IPAddresses) > 0 {
			return c, nil
		}
		if hello != nil && coversName(leaf.DNSNames, hello.ServerName) {
			return c, nil
		}
		return nil, ErrNoCertificateForName
	}
}

func coversName(names []string, name string) bool {
	name = strings.ToLower(strings.TrimSuffix(name, "."))
	if name == "" {
		return false
	}
	i := strings.IndexByte(name, '.')
	for _, n := range names {
		n = strings.ToLower(n)
		if n == name || (i > 0 && n == "*"+name[i:]) {
			return true
		}
	}
	return false
}
