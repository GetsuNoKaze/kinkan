package http

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/metacubex/tls"
)

type kinkanPaddingTestHandler struct{ Handler }

func (kinkanPaddingTestHandler) KinkanTrustTunnelHTTP2Padding() bool { return true }

// Capture plaintext between the same-library HTTP/2 client and the server.
// The parser sees actual wire payload lengths, including Pad Length=0.
type kinkanPaddingCapture struct {
	sync.Mutex
	pending          []byte
	padded, unpadded int
	lengths          map[byte]bool
	err              error
}

func (c *kinkanPaddingCapture) add(p []byte) {
	c.Lock()
	defer c.Unlock()
	c.pending = append(c.pending, p...)
	for len(c.pending) >= 9 {
		h := c.pending
		size := int(h[0])<<16 | int(h[1])<<8 | int(h[2])
		if len(h) < 9+size {
			return
		}
		if h[3] == 0 {
			if h[4]&8 != 0 {
				c.padded++
				if size == 0 || int(h[9])+1 > size {
					c.err = fmt.Errorf("invalid padding")
				} else {
					c.lengths[h[9]] = true
					for _, b := range h[9+size-int(h[9]) : 9+size] {
						if b != 0 {
							c.err = fmt.Errorf("non-zero padding")
						}
					}
				}
			} else {
				c.unpadded++
			}
			if size > 16384 {
				c.err = fmt.Errorf("oversized DATA: %d", size)
			}
		}
		// FLOW_CONTROL_ERROR may arrive as GOAWAY or RST_STREAM.
		if (h[3] == 7 && size >= 8 && bytes.Equal(h[13:17], []byte{0, 0, 0, 3})) ||
			(h[3] == 3 && size == 4 && bytes.Equal(h[9:13], []byte{0, 0, 0, 3})) {
			c.err = fmt.Errorf("FLOW_CONTROL_ERROR on wire")
		}
		c.pending = h[9+size:]
	}
}

type kinkanPaddingCaptureConn struct {
	net.Conn
	capture *kinkanPaddingCapture
}

func (c *kinkanPaddingCaptureConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	c.capture.add(p[:n])
	return n, err
}

func TestKinkanHTTP2PaddingTransfer(t *testing.T) {
	for _, test := range []struct {
		name, env string
		marked    bool
		window    int
		wantPad   bool
	}{
		{"default", "", true, 4 << 20, true},
		{"16KiB", "", true, 16 << 10, true},
		{"disabled", "0", true, 16 << 10, false},
		{"other-front", "", false, 16 << 10, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("KINKAN_TT_PADDING", test.env)
			payload := bytes.Repeat([]byte("TrustTunnel padding experiment 0123456789\x00"), 80000)
			handler := Handler(HandlerFunc(func(w ResponseWriter, r *Request) {
				input, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("incoming request: %v", err)
					return
				}
				if !bytes.Equal(input, payload) {
					t.Error("client data changed")
				}
				w.Header().Set("Content-Length", fmt.Sprint(len(input)))
				if _, err := w.Write(input); err != nil {
					t.Errorf("server write: %v", err)
				}
			}))
			if test.marked {
				handler = kinkanPaddingTestHandler{handler}
			}
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			protocols := new(Protocols)
			protocols.SetUnencryptedHTTP2(true)
			server := &Server{Handler: handler, Protocols: protocols}
			done := make(chan error, 1)
			go func() { done <- server.Serve(listener) }()
			defer func() {
				_ = server.Close()
				if err := <-done; err != ErrServerClosed {
					t.Error(err)
				}
			}()
			capture := &kinkanPaddingCapture{lengths: make(map[byte]bool)}
			transport := &http2Transport{
				AllowHTTP: true,
				t1:        &Transport{HTTP2: &HTTP2Config{MaxReceiveBufferPerStream: test.window, MaxReceiveBufferPerConnection: 65535}},
				DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
					conn, err := (&net.Dialer{}).DialContext(ctx, network, addr)
					if err != nil {
						return nil, err
					}
					return &kinkanPaddingCaptureConn{conn, capture}, nil
				},
			}
			defer transport.CloseIdleConnections()
			// Concurrent streams also share a connection window.
			var wg sync.WaitGroup
			for i := 0; i < 3; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()
					req, err := NewRequestWithContext(ctx, "POST", "http://"+listener.Addr().String()+"/file", bytes.NewReader(payload))
					if err != nil {
						t.Error(err)
						return
					}
					res, err := transport.RoundTrip(req)
					if err != nil {
						t.Error(err)
						return
					}
					defer res.Body.Close()
					data, err := io.ReadAll(res.Body)
					if err != nil {
						t.Error(err)
						return
					}
					if res.ProtoMajor != 2 || !bytes.Equal(data, payload) {
						t.Error("HTTP/2 download differs")
					}
				}()
			}
			wg.Wait()
			capture.Lock()
			defer capture.Unlock()
			if capture.err != nil {
				t.Fatal(capture.err)
			}
			if test.wantPad {
				if capture.padded < 10 || len(capture.lengths) < 2 {
					t.Fatalf("padded=%d lengths=%v", capture.padded, capture.lengths)
				}
			} else if capture.padded != 0 || capture.unpadded == 0 {
				t.Fatalf("padded=%d unpadded=%d", capture.padded, capture.unpadded)
			}
			t.Logf("%d bytes x3, padded=%d, unpadded=%d, distinct lengths=%d", len(payload), capture.padded, capture.unpadded, len(capture.lengths))
		})
	}
}

func TestKinkanHTTP2PaddingWindows(t *testing.T) {
	t.Setenv("KINKAN_TT_PADDING", "")
	for _, test := range []struct {
		name                       string
		data, stream, conn, budget int32
		wantPad                    bool
		count                      int
	}{
		{"room", 100, 1000, 1000, 1000, true, 1},
		{"stream-exact", 100, 100, 1000, 1000, false, 1},
		{"connection-exact", 100, 1000, 100, 1000, false, 1},
		{"stream-boundary", 1000, 100, 1000, 1000, false, 2},
		{"connection-boundary", 1000, 1000, 100, 1000, false, 2},
		{"scheduler-budget", 1000, 1000, 1000, 100, false, 2},
		{"blocked", 100, 0, 1000, 1000, false, 0},
		{"empty-no-window", 0, 0, 0, 1000, false, 1},
		{"empty-with-window", 0, 1000, 1000, 1000, true, 1},
		{"full-frame", 20000, 65535, 65535, 65535, true, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			sc := &http2serverConn{hs: &Server{Handler: kinkanPaddingTestHandler{}}, maxFrameSize: 16384}
			sc.flow.n = test.conn
			stream := &http2stream{id: 1, sc: sc}
			stream.flow.n = test.stream
			stream.flow.setConnFlow(&sc.flow)
			done := make(chan error, 1)
			wr := http2FrameWriteRequest{stream: stream, done: done, write: &http2writeData{streamID: 1, p: make([]byte, test.data), endStream: true}}
			first, rest, count := wr.Consume(test.budget)
			if count != test.count {
				t.Fatalf("count=%d", count)
			}
			size := int32(0)
			if count > 0 {
				wd := first.write.(*http2writeData)
				if (wd.kinkanPad != nil) != test.wantPad {
					t.Fatalf("pad length=%d present=%v", len(wd.kinkanPad), wd.kinkanPad != nil)
				}
				size = int32(first.DataSize())
				if size > 16384 || size > test.budget {
					t.Fatalf("payload=%d", size)
				}
				if count == 2 {
					tail := rest.write.(*http2writeData)
					if wd.endStream || first.done != nil || !tail.endStream || rest.done != done || tail.kinkanPad != nil {
						t.Fatal("split flags/padding/completion")
					}
					if len(wd.p)+len(tail.p) != int(test.data) {
						t.Fatal("split data")
					}
				} else if first.done != done || !wd.endStream {
					t.Fatal("final frame completion")
				}
			}
			if stream.flow.n != test.stream-size || sc.flow.n != test.conn-size {
				t.Fatalf("windows stream=%d conn=%d wire=%d", stream.flow.n, sc.flow.n, size)
			}
		})
	}
}

func TestKinkanHTTP2PaddingEncoding(t *testing.T) {
	for _, length := range []int{0, 255} {
		t.Run(fmt.Sprint(length), func(t *testing.T) {
			data := []byte("server DATA")
			wd := &http2writeData{
				kinkanHTTP2DataPadding: kinkanHTTP2DataPadding{kinkanPad: kinkanHTTP2ZeroPadding[:length]},
				streamID:               1,
				p:                      data,
				endStream:              true,
			}
			wireSize := 9 + len(data) + 1 + length
			if !wd.staysWithinBuffer(wireSize) || wd.staysWithinBuffer(wireSize-1) {
				t.Fatal("buffer accounting excludes padding")
			}
			var direct, async bytes.Buffer
			if err := wd.kinkanWriteData(http2NewFramer(&direct, nil)); err != nil {
				t.Fatal(err)
			}
			framer := http2NewFramer(&async, nil)
			(&http2serverConn{framer: framer}).kinkanStartWriteData(wd)
			if err := framer.endWrite(); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(direct.Bytes(), async.Bytes()) || direct.Len() != wireSize {
				t.Fatal("buffered and async encoding differ")
			}
			frame, err := http2NewFramer(nil, &direct).ReadFrame()
			if err != nil {
				t.Fatal(err)
			}
			df := frame.(*http2DataFrame)
			if !df.Header().Flags.Has(http2FlagDataPadded) || !df.StreamEnded() || !bytes.Equal(df.Data(), data) {
				t.Fatal("padding or END_STREAM changed data")
			}
		})
	}
}
