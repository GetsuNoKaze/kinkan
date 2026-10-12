package nodeprobe

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	quic "github.com/metacubex/quic-go"
)

func TestWrongKeyAgainstOrdinaryHTTPS(t *testing.T) {
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("cover")) }))
	defer s.Close()
	ref := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("cover")) }))
	defer ref.Close()
	roots := x509.NewCertPool()
	roots.AddCert(s.Certificate())
	for _, protocol := range []string{"vless", "vmess", "trojan", "anytls", "trusttunnel"} {
		t.Run(protocol, func(t *testing.T) {
			cfg := Config{Protocol: protocol, Target: s.URL, Reference: ref.URL, Timeout: time.Second, TLS: &tls.Config{RootCAs: roots}}
			u, _ := origin(s.URL)
			r := Report{}
			r.scanWrongKey(context.Background(), cfg, u)
			if r.WrongKey == nil || r.WrongKey.Verdict != "site" {
				t.Fatalf("ordinary site mismatched: %+v", r.WrongKey)
			}
			if protocol != "trusttunnel" && r.WrongKey.Target.Status != 400 {
				t.Fatal(r.WrongKey.Target)
			}
		})
	}
}

func TestWrongKeyComparisonDoesNotConfuseTimingAndDisclosure(t *testing.T) {
	a := AuthObservation{Kind: "response", Status: 400, Digest: "body", Close: "eof", ElapsedMS: 1}
	b := a
	b.ElapsedMS = 9000
	if v, _ := compareAuth(a, &b); v != "site" {
		t.Fatal("timing alone became a fingerprint", v)
	}
	b.Close = "reset"
	if v, _ := compareAuth(a, &b); v != "different" {
		t.Fatal("lost close difference", v)
	}
	if v, _ := compareAuth(a, nil); v != "inconclusive" {
		t.Fatal("no reference became success", v)
	}
	b.Kind = "timeout"
	if v, _ := compareAuth(a, &b); v != "inconclusive" {
		t.Fatal("reference timeout became success", v)
	}
	a.Detail = "proxy authentication response"
	if v, _ := compareAuth(a, &b); v != "exposed" {
		t.Fatal("lost positive leak", v)
	}
	for _, remote := range []bool{true, false} {
		o := remoteAuthClose(&quic.ApplicationError{Remote: remote, ErrorCode: 0xfffffff1})
		if (o.Detail == "TUIC authentication close") != remote {
			t.Fatal("local QUIC error became disclosure", o)
		}
	}
}

func TestWrongKeyCancellationAndMissingTransport(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		c, err := l.Accept()
		if err == nil {
			accepted <- c
		}
	}()
	u, _ := origin("https://" + l.Addr().String())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan AuthObservation, 1)
	go func() {
		done <- authTCP(ctx, Config{KinkanConfig: KinkanConfig{PlainTCP: true}, Timeout: 5 * time.Second}, u, []byte{0})
	}()
	c := <-accepted
	defer c.Close()
	cancel()
	select {
	case o := <-done:
		if o.Kind != "error" {
			t.Fatal(o)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation did not interrupt read")
	}
	r := Report{}
	r.scanWrongKey(context.Background(), Config{Protocol: "vless", KinkanConfig: KinkanConfig{Transport: "xhttp"}}, u)
	if r.WrongKey.Verdict != "inconclusive" {
		t.Fatal(r)
	}
	if _, err := wrongKeyPayload("sudoku"); err == nil || errors.Is(err, context.Canceled) {
		t.Fatal("unsupported protocol accepted", err)
	}
}
