package http2

import (
	"bytes"
	"io"
	"net"
	"testing"
	"time"

	"github.com/metacubex/http"
)

// serverSettings serves one connection with s and returns the payload of the
// server's first SETTINGS frame.
func serverSettings(t *testing.T, s *Server) []byte {
	t.Helper()
	client, server := net.Pipe()
	defer client.Close()
	go s.ServeConn(server, &ServeConnOpts{Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})})
	_ = client.SetDeadline(time.Now().Add(5 * time.Second))
	go io.WriteString(client, ClientPreface)
	var h [9]byte
	if _, err := io.ReadFull(client, h[:]); err != nil {
		t.Fatal(err)
	}
	if h[3] != byte(FrameSettings) || h[4] != 0 {
		t.Fatalf("first frame header % x, want SETTINGS without ACK", h)
	}
	p := make([]byte, int(h[0])<<16|int(h[1])<<8|int(h[2]))
	if _, err := io.ReadFull(client, p); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestServerSettingsFrameLikeGo pins the server's SETTINGS frame, values and
// order, to golang.org/x/net/http2's (and so Caddy's).
func TestServerSettingsFrameLikeGo(t *testing.T) {
	goSettings := []byte{
		0x00, 0x05, 0x00, 0x10, 0x00, 0x00, // MAX_FRAME_SIZE 1048576
		0x00, 0x03, 0x00, 0x00, 0x00, 0xfa, // MAX_CONCURRENT_STREAMS 250
		0x00, 0x06, 0x00, 0x10, 0x01, 0x40, // MAX_HEADER_LIST_SIZE 1048896
		0x00, 0x01, 0x00, 0x00, 0x10, 0x00, // HEADER_TABLE_SIZE 4096
		0x00, 0x04, 0x00, 0x10, 0x00, 0x00, // INITIAL_WINDOW_SIZE 1048576
	}
	noRFC7540 := []byte{0x00, 0x09, 0x00, 0x00, 0x00, 0x01} // NO_RFC7540_PRIORITIES 1
	tests := []struct {
		name string
		ws   func() WriteScheduler
		want []byte
	}{
		{"default", nil, append(append([]byte{}, goSettings...), noRFC7540...)},
		{"rfc9218", newPriorityWriteSchedulerRFC9218, append(append([]byte{}, goSettings...), noRFC7540...)},
		{"random", NewRandomWriteScheduler, append(append([]byte{}, goSettings...), noRFC7540...)},
		{"rfc7540", func() WriteScheduler { return NewPriorityWriteScheduler(nil) }, goSettings},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := serverSettings(t, &Server{NewWriteScheduler: tt.ws})
			if !bytes.Equal(got, tt.want) {
				t.Errorf("SETTINGS payload\n got % x\nwant % x", got, tt.want)
			}
		})
	}
}

func TestSettingNoRFC7540PrioritiesValid(t *testing.T) {
	for _, v := range []uint32{0, 1} {
		if err := (Setting{SettingNoRFC7540Priorities, v}).Valid(); err != nil {
			t.Errorf("value %d: %v", v, err)
		}
	}
	if err := (Setting{SettingNoRFC7540Priorities, 2}).Valid(); err == nil {
		t.Error("value 2 accepted")
	}
	if got := SettingNoRFC7540Priorities.String(); got != "NO_RFC7540_PRIORITIES" {
		t.Errorf("String() = %q", got)
	}
}
