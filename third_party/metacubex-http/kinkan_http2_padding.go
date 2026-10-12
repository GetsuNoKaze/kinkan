package http

import (
	"crypto/rand"
	"os"
)

// Only the TrustTunnel front implements this marker. Other users of this
// HTTP library retain upstream DATA framing, including its HTTP/2 client.
type kinkanTrustTunnelPadding interface {
	KinkanTrustTunnelHTTP2Padding() bool
}

type kinkanHTTP2DataPadding struct{ kinkanPad []byte }

var kinkanHTTP2ZeroPadding [255]byte

func (w *http2writeData) kinkanDataSize() int {
	n := len(w.p)
	if w.kinkanPad != nil {
		n += 1 + len(w.kinkanPad)
	}
	return n
}

func (w *http2writeData) kinkanWriteData(f *http2Framer) error {
	return f.WriteDataPadded(w.streamID, w.endStream, w.p, w.kinkanPad)
}

func (sc *http2serverConn) kinkanStartWriteData(w *http2writeData) {
	// The existing async path flushes this buffer in writeFrameAsync.
	_ = sc.framer.startWriteDataPadded(w.streamID, w.endStream, w.p, w.kinkanPad)
}

// kinkanPreparePadding reserves padding before upstream Consume splits DATA.
// Consume then deducts data bytes itself, from the same two windows.
func (wr http2FrameWriteRequest) kinkanPreparePadding(n int32) int32 {
	wd, ok := wr.write.(*http2writeData)
	if !ok || wr.stream == nil || wr.stream.sc == nil || wr.stream.sc.hs == nil {
		return n
	}
	marker, ok := wr.stream.sc.hs.Handler.(kinkanTrustTunnelPadding)
	if !ok || !marker.KinkanTrustTunnelHTTP2Padding() || os.Getenv("KINKAN_TT_PADDING") == "0" {
		return n
	}
	limit := kinkanMinPadding(n, kinkanMinPadding(wr.stream.flow.available(), wr.stream.sc.maxFrameSize))
	if limit <= 0 {
		return n
	}
	var random [1]byte
	if _, err := rand.Read(random[:]); err != nil {
		return n
	}
	overhead := int32(random[0]) + 1
	size := limit
	if len(wd.p) < int(limit) {
		size = int32(len(wd.p))
	}
	// Full-size DATA must leave room for padding within MAX_FRAME_SIZE.
	// Window-limited frames keep their data size and go unpadded if needed.
	if limit == wr.stream.sc.maxFrameSize && size == limit {
		size -= overhead
	}
	if size+overhead > limit {
		return n
	}
	wd.kinkanPad = kinkanHTTP2ZeroPadding[:int(random[0])]
	// A non-nil empty slice encodes PADDED with Pad Length=0. take charges
	// both stream and connection; RFC 9113 counts the whole DATA payload.
	wr.stream.flow.take(overhead)
	return kinkanMinPadding(n, wr.stream.sc.maxFrameSize-overhead)
}

func (w *http2writeData) kinkanPadding() kinkanHTTP2DataPadding {
	return w.kinkanHTTP2DataPadding
}

func kinkanMinPadding(a, b int32) int32 {
	if a < b {
		return a
	}
	return b
}

func kinkanNewWriteData(streamID uint32, data []byte, endStream bool) *http2writeData {
	return &http2writeData{streamID: streamID, p: data, endStream: endStream}
}
