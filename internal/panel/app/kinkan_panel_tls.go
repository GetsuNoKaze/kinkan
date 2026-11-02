package app

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
)

// errPanelNoName fails a handshake that does not name the panel.
var errPanelNoName = errors.New("panel: no certificate for this name")

// kinkanNamedOnly hands the panel's certificate only to a client that names the panel,
// as Caddy does with a site under its domain alone: a handshake by bare IP, with no SNI
// or a stranger's, fails with internal_error instead of telling a scanner the panel's
// domain (checked against Caddy 2.11.7, 02.11.2026). The panel's own checks (the
// container's health, which dials 127.0.0.1) and a panel reached by its address (a
// certificate with an IP in it) are let through.
func kinkanNamedOnly(get func(*tls.ClientHelloInfo) (*tls.Certificate, error)) func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		c, err := get(hello)
		if err != nil || c == nil || len(c.Certificate) == 0 || kinkanLoopback(hello) {
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
		if hello.ServerName != "" && leaf.VerifyHostname(hello.ServerName) == nil {
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
