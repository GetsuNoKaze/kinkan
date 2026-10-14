package authevent

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

// maxHello bounds what a HelloConn keeps: a ClientHello with a post-quantum key share is
// under 2 KB, and nothing after the first handshake message is kept.
const maxHello = 8 << 10

// HelloConn records the client's first TLS handshake message as it is read, so that a
// listener which rejects the client can report the client's TLS fingerprint (JA4). Once
// the ClientHello is complete it stops recording and lets copies reach the connection
// underneath, so splice and Vision keep their fast path.
type HelloConn struct {
	net.Conn
	mu   sync.Mutex
	buf  []byte
	done atomic.Bool
}

func NewHelloConn(c net.Conn) *HelloConn { return &HelloConn{Conn: c} }

func (c *HelloConn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	if n > 0 && !c.done.Load() {
		c.mu.Lock()
		if !c.done.Load() {
			take := n
			if room := maxHello - len(c.buf); take > room {
				take = room
			}
			c.buf = append(c.buf, b[:take]...)
			c.done.Store(len(c.buf) >= maxHello || helloComplete(c.buf))
		}
		c.mu.Unlock()
	}
	return n, err
}

func (c *HelloConn) Upstream() any { return c.Conn }

func (c *HelloConn) ReaderReplaceable() bool { return c.done.Load() }

func (c *HelloConn) WriterReplaceable() bool { return true }

// JA4 is the fingerprint of the ClientHello read so far, or "" without a whole one.
func (c *HelloConn) JA4() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return JA4(c.buf)
}

// handshake joins the payloads of the TLS handshake records at the start of raw.
func handshake(raw []byte) []byte {
	var hs []byte
	for len(raw) >= 5 && raw[0] == 0x16 {
		n := int(binary.BigEndian.Uint16(raw[3:5]))
		if len(raw) < 5+n {
			hs = append(hs, raw[5:]...)
			break
		}
		hs = append(hs, raw[5:5+n]...)
		raw = raw[5+n:]
	}
	return hs
}

func helloComplete(raw []byte) bool {
	hs := handshake(raw)
	if len(raw) >= 1 && raw[0] != 0x16 {
		return true // not TLS: nothing more to wait for
	}
	return len(hs) >= 4 && len(hs) >= 4+int(hs[1])<<16|int(hs[2])<<8|int(hs[3])
}

func grease(v uint16) bool { return v&0x0f0f == 0x0a0a && v>>8 == v&0xff }

type reader struct {
	b  []byte
	ok bool
}

func (r *reader) take(n int) []byte {
	if !r.ok || n < 0 || len(r.b) < n {
		r.ok = false
		return nil
	}
	v := r.b[:n]
	r.b = r.b[n:]
	return v
}
func (r *reader) u8() int {
	if v := r.take(1); v != nil {
		return int(v[0])
	}
	return 0
}

func (r *reader) u16() int {
	if v := r.take(2); v != nil {
		return int(binary.BigEndian.Uint16(v))
	}
	return 0
}

func (r *reader) u24() int {
	if v := r.take(3); v != nil {
		return int(v[0])<<16 | int(v[1])<<8 | int(v[2])
	}
	return 0
}

// JA4 computes the JA4 fingerprint (FoxIO, BSD-3-Clause spec) of the ClientHello at the
// start of raw TLS bytes, or "" when there is no whole ClientHello there.
func JA4(raw []byte) string {
	hs := handshake(raw)
	r := &reader{b: hs, ok: true}
	if r.u8() != 1 {
		return ""
	}
	body := &reader{b: r.take(r.u24()), ok: r.ok}
	legacy := body.u16()
	body.take(32)
	body.take(body.u8())
	var ciphers []string
	cs := &reader{b: body.take(body.u16()), ok: body.ok}
	for len(cs.b) >= 2 {
		if v := uint16(cs.u16()); !grease(v) {
			ciphers = append(ciphers, fmt.Sprintf("%04x", v))
		}
	}
	body.take(body.u8())
	var exts, sigs []string
	sni, alpn, version, count := "i", "00", legacy, 0
	if body.ok && len(body.b) >= 2 {
		es := &reader{b: body.take(body.u16()), ok: body.ok}
		for es.ok && len(es.b) >= 4 {
			t := uint16(es.u16())
			data := &reader{b: es.take(es.u16()), ok: es.ok}
			if grease(t) {
				continue
			}
			count++
			switch t {
			case 0x0000:
				sni = "d"
			case 0x0010:
				data.take(2)
				if v := data.take(data.u8()); len(v) > 0 {
					alpn = alpnChars(v)
				}
			case 0x002b:
				vs := &reader{b: data.take(data.u8()), ok: data.ok}
				best := 0
				for len(vs.b) >= 2 {
					if v := vs.u16(); !grease(uint16(v)) && v > best {
						best = v
					}
				}
				if best > 0 {
					version = best
				}
			case 0x000d:
				ss := &reader{b: data.take(data.u16()), ok: data.ok}
				for len(ss.b) >= 2 {
					if v := uint16(ss.u16()); !grease(v) {
						sigs = append(sigs, fmt.Sprintf("%04x", v))
					}
				}
			}
			if t != 0x0000 && t != 0x0010 {
				exts = append(exts, fmt.Sprintf("%04x", t))
			}
		}
	}
	if !body.ok {
		return ""
	}
	v := map[int]string{0x0304: "13", 0x0303: "12", 0x0302: "11", 0x0301: "10", 0x0300: "s3"}[version]
	if v == "" {
		v = "00"
	}
	a := fmt.Sprintf("t%s%s%02d%02d%s", v, sni, upTo99(len(ciphers)), upTo99(count), alpn)
	sort.Strings(ciphers)
	sort.Strings(exts)
	c := strings.Join(exts, ",")
	if len(sigs) > 0 {
		c += "_" + strings.Join(sigs, ",")
	}
	return a + "_" + hash12(strings.Join(ciphers, ","), len(ciphers)) + "_" + hash12(c, len(exts))
}

func alpnChars(v []byte) string {
	f, l := v[0], v[len(v)-1]
	if alnum(f) && alnum(l) {
		return string([]byte{f, l})
	}
	h := hex.EncodeToString(v)
	return string([]byte{h[0], h[len(h)-1]})
}

func alnum(b byte) bool { return b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' }

func hash12(s string, n int) string {
	if n == 0 {
		return "000000000000"
	}
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])[:12]
}

func upTo99(n int) int {
	if n > 99 {
		return 99
	}
	return n
}
