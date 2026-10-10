package http_test

import (
	"crypto/tls"
	"encoding/binary"
	"io"
	"testing"
	"time"

	. "github.com/metacubex/http"
	"github.com/metacubex/http/httptest"
)

// TestKinkanServerSettingsLikeGo checks the server's first SETTINGS frame ends with
// NO_RFC7540_PRIORITIES=1, as a current Go server (and Caddy) sends it.
func TestKinkanServerSettingsLikeGo(t *testing.T) {
	s := httptest.NewUnstartedServer(HandlerFunc(func(w ResponseWriter, r *Request) {}))
	s.EnableHTTP2 = true
	s.StartTLS()
	defer s.Close()

	conn, err := tls.Dial("tcp", s.Listener.Addr().String(), &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"h2"}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if got := conn.ConnectionState().NegotiatedProtocol; got != "h2" {
		t.Fatalf("negotiated %q, want h2", got)
	}
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.WriteString(conn, "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	var header [9]byte
	if _, err := io.ReadFull(conn, header[:]); err != nil {
		t.Fatal(err)
	}
	length := int(header[0])<<16 | int(header[1])<<8 | int(header[2])
	if header[3] != 0x4 || header[4] != 0 || length%6 != 0 {
		t.Fatalf("first frame is not SETTINGS: % x", header)
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(conn, payload); err != nil {
		t.Fatal(err)
	}
	var ids []uint16
	var last uint32
	for i := 0; i < length; i += 6 {
		ids = append(ids, binary.BigEndian.Uint16(payload[i:]))
		last = binary.BigEndian.Uint32(payload[i+2:])
	}
	if len(ids) == 0 || ids[len(ids)-1] != 0x9 || last != 1 {
		t.Fatalf("SETTINGS ids %v (last value %d), want NO_RFC7540_PRIORITIES=1 last", ids, last)
	}
}
