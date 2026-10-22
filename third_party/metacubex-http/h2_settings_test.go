package http_test

import (
	"bytes"
	"crypto/tls"
	"io"
	"testing"
	"time"

	. "github.com/metacubex/http"
	"github.com/metacubex/http/httptest"
)

// readServerGreeting returns the payloads of the server's first SETTINGS
// frame and of the frame that follows it.
func readServerGreeting(t *testing.T, s *httptest.Server) (settings []byte, nextType byte, next []byte) {
	t.Helper()
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
	readFrame := func() (byte, byte, []byte) {
		var h [9]byte
		if _, err := io.ReadFull(conn, h[:]); err != nil {
			t.Fatal(err)
		}
		p := make([]byte, int(h[0])<<16|int(h[1])<<8|int(h[2]))
		if _, err := io.ReadFull(conn, p); err != nil {
			t.Fatal(err)
		}
		return h[3], h[4], p
	}
	typ, flags, settings := readFrame()
	if typ != 0x4 || flags != 0 {
		t.Fatalf("first frame type %#x flags %#x, want SETTINGS without ACK", typ, flags)
	}
	nextType, _, next = readFrame()
	return settings, nextType, next
}

// TestServerSettingsFrameLikeGo pins the server's SETTINGS frame, values and
// order, to what a Go net/http server configured by golang.org/x/net/http2
// sends, Caddy among them: Go's five settings followed by
// NO_RFC7540_PRIORITIES=1, then a connection WINDOW_UPDATE to 1 MiB.
func TestServerSettingsFrameLikeGo(t *testing.T) {
	tests := []struct {
		name           string
		maxHeaderBytes int
		want           []byte
	}{{
		name: "defaults",
		want: []byte{
			0x00, 0x05, 0x00, 0x10, 0x00, 0x00, // MAX_FRAME_SIZE 1048576
			0x00, 0x03, 0x00, 0x00, 0x00, 0xfa, // MAX_CONCURRENT_STREAMS 250
			0x00, 0x06, 0x00, 0x10, 0x01, 0x40, // MAX_HEADER_LIST_SIZE 1048896
			0x00, 0x01, 0x00, 0x00, 0x10, 0x00, // HEADER_TABLE_SIZE 4096
			0x00, 0x04, 0x00, 0x10, 0x00, 0x00, // INITIAL_WINDOW_SIZE 1048576
			0x00, 0x09, 0x00, 0x00, 0x00, 0x01, // NO_RFC7540_PRIORITIES 1
		},
	}, {
		name:           "caddy header limit",
		maxHeaderBytes: 16 << 10,
		want: []byte{
			0x00, 0x05, 0x00, 0x10, 0x00, 0x00,
			0x00, 0x03, 0x00, 0x00, 0x00, 0xfa,
			0x00, 0x06, 0x00, 0x00, 0x41, 0x40, // MAX_HEADER_LIST_SIZE 16704
			0x00, 0x01, 0x00, 0x00, 0x10, 0x00,
			0x00, 0x04, 0x00, 0x10, 0x00, 0x00,
			0x00, 0x09, 0x00, 0x00, 0x00, 0x01,
		},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := httptest.NewUnstartedServer(HandlerFunc(func(w ResponseWriter, r *Request) {}))
			s.EnableHTTP2 = true
			s.Config.MaxHeaderBytes = tt.maxHeaderBytes
			s.StartTLS()
			defer s.Close()

			settings, nextType, next := readServerGreeting(t, s)
			if !bytes.Equal(settings, tt.want) {
				t.Errorf("SETTINGS payload\n got % x\nwant % x", settings, tt.want)
			}
			wantUpdate := []byte{0x00, 0x0f, 0x00, 0x01} // 1<<20 - 65535
			if nextType != 0x8 || !bytes.Equal(next, wantUpdate) {
				t.Errorf("next frame type %#x payload % x, want WINDOW_UPDATE % x", nextType, next, wantUpdate)
			}
		})
	}
}
