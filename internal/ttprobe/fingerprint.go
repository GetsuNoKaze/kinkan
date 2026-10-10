package ttprobe

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

type setting struct {
	ID    uint16 `json:"id"`
	Value uint32 `json:"value"`
}
type window struct {
	Stream    uint32 `json:"stream"`
	Increment uint32 `json:"increment"`
}
type h2Fingerprint struct {
	Settings []setting `json:"settings"`
	Windows  []window  `json:"initial_window_updates"`
	Headers  []string  `json:"header_order"`
}

func fingerprint(ctx context.Context, cfg Config, u *url.URL, host string) (h2Fingerprint, error) {
	r := h2Fingerprint{Settings: []setting{}, Windows: []window{}, Headers: []string{}}
	c, err := dial(ctx, cfg, u, []string{"h2"}, u.Hostname(), false)
	if err != nil {
		return r, err
	}
	defer c.Close()
	stop := context.AfterFunc(ctx, func() { c.Close() })
	defer stop()
	if c.ConnectionState().NegotiatedProtocol != "h2" {
		return r, errors.New("h2 was not negotiated")
	}
	if _, err := c.Write([]byte(http2.ClientPreface)); err != nil {
		return r, err
	}
	f := http2.NewFramer(c, reader(c))
	f.SetMaxReadFrameSize(64 << 10)
	if err := f.WriteSettings(); err != nil {
		return r, err
	}
	var block bytes.Buffer
	encoder := hpack.NewEncoder(&block)
	for _, field := range []hpack.HeaderField{{Name: ":method", Value: "GET"}, {Name: ":scheme", Value: "https"}, {Name: ":authority", Value: host}, {Name: ":path", Value: "/"}, {Name: "user-agent", Value: "kinkan-probe/1"}, {Name: "accept-encoding", Value: "identity"}} {
		if err := encoder.WriteField(field); err != nil {
			return r, err
		}
	}
	if err := f.WriteHeaders(http2.HeadersFrameParam{StreamID: 1, BlockFragment: block.Bytes(), EndStream: true, EndHeaders: true}); err != nil {
		return r, err
	}
	decoder := hpack.NewDecoder(4096, func(field hpack.HeaderField) { r.Headers = append(r.Headers, field.Name) })
	decoder.SetMaxStringLength(64 << 10)
	var headerBlock []byte
	continuation := false
	for count := 0; count < 128; count++ {
		if err := ctx.Err(); err != nil {
			return r, err
		}
		frame, err := f.ReadFrame()
		if err != nil {
			return r, err
		}
		if count == 0 {
			if first, ok := frame.(*http2.SettingsFrame); !ok || first.IsAck() {
				return r, errors.New("first server frame is not non-ACK SETTINGS")
			}
		}
		if continuation {
			if _, ok := frame.(*http2.ContinuationFrame); !ok {
				return r, errors.New("interleaved HTTP/2 header block")
			}
		}
		endHeaders := false
		switch frame := frame.(type) {
		case *http2.SettingsFrame:
			if !frame.IsAck() {
				if err := frame.ForeachSetting(func(s http2.Setting) error {
					if len(r.Settings) >= 128 {
						return errors.New("too many SETTINGS")
					}
					r.Settings = append(r.Settings, setting{uint16(s.ID), s.Val})
					return nil
				}); err != nil {
					return r, err
				}
				if err := f.WriteSettingsAck(); err != nil {
					return r, err
				}
			}
		case *http2.WindowUpdateFrame:
			r.Windows = append(r.Windows, window{frame.StreamID, frame.Increment})
		case *http2.PingFrame:
			if !frame.IsAck() {
				if err := f.WritePing(true, frame.Data); err != nil {
					return r, err
				}
			}
		case *http2.HeadersFrame:
			if frame.StreamID != 1 {
				return r, errors.New("unexpected HEADERS stream")
			}
			headerBlock = append(headerBlock, frame.HeaderBlockFragment()...)
			endHeaders = frame.HeadersEnded()
			continuation = !endHeaders
		case *http2.ContinuationFrame:
			if !continuation || frame.StreamID != 1 {
				return r, errors.New("unexpected CONTINUATION")
			}
			headerBlock = append(headerBlock, frame.HeaderBlockFragment()...)
			endHeaders = frame.HeadersEnded()
			continuation = !endHeaders
		case *http2.GoAwayFrame:
			return r, fmt.Errorf("GOAWAY before response headers: %s", frame.ErrCode)
		case *http2.RSTStreamFrame:
			return r, fmt.Errorf("stream reset before response headers: %s", frame.ErrCode)
		}
		if len(headerBlock) > 64<<10 {
			return r, errors.New("HTTP/2 headers exceed 64 KiB")
		}
		if endHeaders {
			if _, err := decoder.Write(headerBlock); err != nil {
				return r, err
			}
			if err := decoder.Close(); err != nil {
				return r, err
			}
			return r, nil
		}
	}
	return r, errors.New("too many frames before response headers")
}
