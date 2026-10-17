package http

import (
	"bytes"
	"testing"
	"time"

	"golang.org/x/net/http2/hpack"
)

type kinkanBlockedHeaders struct {
	started chan struct{}
	release chan struct{}
}

func (*kinkanBlockedHeaders) staysWithinBuffer(int) bool { return false }

func (w *kinkanBlockedHeaders) writeFrame(ctx http2writeContext) error {
	enc, _ := ctx.HeaderEncoder()
	close(w.started)
	<-w.release
	return enc.WriteField(hpack.HeaderField{Name: ":status", Value: "200"})
}

// A peer's SETTINGS must not mutate the HPACK encoder while the asynchronous
// frame writer owns it (GO-2026-6617). Preserve both the smallest and final size.
func TestKinkanHeaderTableSizeDuringWrite(t *testing.T) {
	sc := &http2serverConn{
		serveG:       http2newGoroutineLock(),
		bw:           &http2bufferedWriter{},
		wroteFrameCh: make(chan http2frameWriteResult, 1),
	}
	sc.hpackEncoder = hpack.NewEncoder(&sc.headerWriteBuf)
	release := make(chan struct{})
	defer func() {
		if release != nil {
			close(release)
		}
	}()
	w := &kinkanBlockedHeaders{started: make(chan struct{}), release: release}
	sc.startFrameWrite(http2FrameWriteRequest{write: w})
	select {
	case <-w.started:
	case <-time.After(5 * time.Second):
		t.Fatal("frame writer did not start")
	}
	for _, size := range []uint32{0, 2048} {
		if err := sc.processSetting(http2Setting{ID: http2SettingHeaderTableSize, Val: size}); err != nil {
			t.Fatal(err)
		}
	}
	if got := sc.hpackEncoder.MaxDynamicTableSize(); got != 4096 {
		t.Fatalf("encoder changed during an active frame write: got %d, want 4096", got)
	}
	close(release)
	release = nil
	select {
	case result := <-sc.wroteFrameCh:
		if result.err != nil {
			t.Fatal(result.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("frame writer did not finish")
	}
	// The serve loop has received completion; it may start the next frame.
	sc.writingFrame = false
	sc.headerWriteBuf.Reset()
	w = &kinkanBlockedHeaders{started: make(chan struct{}), release: make(chan struct{})}
	close(w.release)
	sc.startFrameWrite(http2FrameWriteRequest{write: w})
	select {
	case result := <-sc.wroteFrameCh:
		if result.err != nil {
			t.Fatal(result.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("second frame writer did not finish")
	}
	var want bytes.Buffer
	enc := hpack.NewEncoder(&want)
	enc.SetMaxDynamicTableSize(0)
	enc.SetMaxDynamicTableSize(2048)
	if err := enc.WriteField(hpack.HeaderField{Name: ":status", Value: "200"}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(sc.headerWriteBuf.Bytes(), want.Bytes()) {
		t.Fatalf("HPACK block = %x, want %x (minimum and final table sizes)", sc.headerWriteBuf.Bytes(), want.Bytes())
	}
}
