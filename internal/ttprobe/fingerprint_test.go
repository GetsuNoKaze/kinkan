package ttprobe

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

func TestFingerprintKeepsWireOrderAndContinuation(t *testing.T) {
	s, cfg := site(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	ln, err := tls.Listen("tcp", "127.0.0.1:0", s.TLS.Clone())
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	done := make(chan error, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			done <- err
			return
		}
		defer c.Close()
		c.SetDeadline(time.Now().Add(time.Second))
		var preface [24]byte
		if _, err = io.ReadFull(c, preface[:]); err != nil {
			done <- err
			return
		}
		f := http2.NewFramer(c, c)
		// Read both initial client frames so the response is not timing-dependent.
		for i := 0; i < 2; i++ {
			if _, err = f.ReadFrame(); err != nil {
				done <- err
				return
			}
		}
		if err = f.WriteSettings(http2.Setting{ID: http2.SettingInitialWindowSize, Val: 65535}, http2.Setting{ID: http2.SettingMaxConcurrentStreams, Val: 17}); err != nil {
			done <- err
			return
		}
		if err = f.WriteWindowUpdate(0, 12345); err != nil {
			done <- err
			return
		}
		var buf bytes.Buffer
		encoder := hpack.NewEncoder(&buf)
		for _, field := range []hpack.HeaderField{{Name: ":status", Value: "200"}, {Name: "x-second", Value: "b"}, {Name: "x-first", Value: "a"}} {
			if err = encoder.WriteField(field); err != nil {
				done <- err
				return
			}
		}
		block := buf.Bytes()
		middle := len(block) / 2
		if err = f.WriteHeaders(http2.HeadersFrameParam{StreamID: 1, BlockFragment: block[:middle], EndStream: true}); err != nil {
			done <- err
			return
		}
		err = f.WriteContinuation(1, true, block[middle:])
		done <- err
	}()
	u, _ := endpoint("https://" + ln.Addr().String())
	got, err := fingerprint(context.Background(), cfg, u, u.Host)
	if err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(got)
	want := `{"settings":[{"id":4,"value":65535},{"id":3,"value":17}],"initial_window_updates":[{"stream":0,"increment":12345}],"header_order":[":status","x-second","x-first"]}`
	if string(raw) != want {
		t.Fatalf("fingerprint changed wire order:\n%s\nwant %s", raw, want)
	}
}
