package app

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
)

// errPanelNoName fails a handshake that does not name the panel.
var errPanelNoName = errors.New("panel: no certificate for this name")

// kinkanNamedOnly fails a handshake by bare IP, with no SNI, with internal_error, as Caddy
// does with a site under its domain alone, instead of telling a scanner that walks IP
// addresses the panel's domain (checked against Caddy 2.11.7, 02.11.2026). Any host name
// passes, the panel's or not: Cloudflare in front of a subscription domain dials the
// panel under that domain, not the panel's. The panel's own checks (the container's
// health, which dials 127.0.0.1) and a panel reached by its address (a certificate with
// an IP in it) are let through.
func kinkanNamedOnly(get func(*tls.ClientHelloInfo) (*tls.Certificate, error)) func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		c, err := get(hello)
		named := hello.ServerName != "" && net.ParseIP(hello.ServerName) == nil // an IP for a name names nothing
		if err != nil || c == nil || len(c.Certificate) == 0 || kinkanLoopback(hello) || named {
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
		return nil, errPanelNoName
	}
}

func kinkanLoopback(hello *tls.ClientHelloInfo) bool {
	if hello == nil || hello.Conn == nil {
		return false
	}
	host, _, err := net.SplitHostPort(hello.Conn.RemoteAddr().String())
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
