package authevent

import (
	"encoding/binary"
	"net"
	"testing"
)

func u16(v int) []byte { return []byte{byte(v >> 8), byte(v)} }

func ext(t int, data []byte) []byte { return append(append(u16(t), u16(len(data))...), data...) }

// chromeHello is a ClientHello with Chrome's ciphers, extensions and signature algorithms,
// GREASE included, whose JA4 is the one in the JA4 specification.
func chromeHello() []byte {
	var ciphers []byte
	for _, c := range []int{0x1a1a, 0x1301, 0x1302, 0x1303, 0xc02b, 0xc02f, 0xc02c, 0xc030, 0xcca9, 0xcca8, 0xc013, 0xc014, 0x009c, 0x009d, 0x002f, 0x0035} {
		ciphers = append(ciphers, u16(c)...)
	}
	host := []byte("example.org")
	sni := append(append(u16(len(host)+3), 0), append(u16(len(host)), host...)...)
	alpn := append(u16(12), append([]byte{2, 'h', '2'}, append([]byte{8}, "http/1.1"...)...)...)
	var sigs []byte
	for _, s := range []int{0x0403, 0x0804, 0x0401, 0x0503, 0x0805, 0x0501, 0x0806, 0x0601} {
		sigs = append(sigs, u16(s)...)
	}
	exts := ext(0x2a2a, nil)
	exts = append(exts, ext(0x0000, sni)...)
	exts = append(exts, ext(0x0017, nil)...)
	exts = append(exts, ext(0xff01, []byte{0})...)
	exts = append(exts, ext(0x000a, append(u16(4), 0x00, 0x1d, 0x00, 0x17))...)
	exts = append(exts, ext(0x000b, []byte{1, 0})...)
	exts = append(exts, ext(0x0023, nil)...)
	exts = append(exts, ext(0x0010, alpn)...)
	exts = append(exts, ext(0x0005, []byte{1, 0, 0, 0, 0})...)
	exts = append(exts, ext(0x000d, append(u16(len(sigs)), sigs...))...)
	exts = append(exts, ext(0x0012, nil)...)
	exts = append(exts, ext(0x0033, append(u16(36), append(u16(0x001d), append(u16(32), make([]byte, 32)...)...)...))...)
	exts = append(exts, ext(0x002d, []byte{1, 1})...)
	exts = append(exts, ext(0x002b, []byte{4, 0x3a, 0x3a, 0x03, 0x04})...)
	exts = append(exts, ext(0x001b, []byte{2, 0, 2})...)
	exts = append(exts, ext(0x4469, []byte{0, 3, 2, 'h', '2'})...)
	exts = append(exts, ext(0x4a4a, []byte{0})...)
	exts = append(exts, ext(0x0015, make([]byte, 10))...)
	body := append(u16(0x0303), make([]byte, 32)...)
	body = append(body, 32)
	body = append(body, make([]byte, 32)...)
	body = append(body, u16(len(ciphers))...)
	body = append(body, ciphers...)
	body = append(body, 1, 0)
	body = append(body, u16(len(exts))...)
	body = append(body, exts...)
	hs := append([]byte{1, byte(len(body) >> 16), byte(len(body) >> 8), byte(len(body))}, body...)
	return append(append([]byte{0x16, 0x03, 0x01}, u16(len(hs))...), hs...)
}

func TestJA4OfChrome(t *testing.T) {
	if got := JA4(chromeHello()); got != "t13d1516h2_8daaf6152771_e5627efa2ab1" {
		t.Fatalf("JA4 = %q", got)
	}
}

func TestJA4OfSplitAndBadInput(t *testing.T) {
	h := chromeHello()
	// The same ClientHello in two records.
	hs := h[5:]
	split := append(append([]byte{0x16, 0x03, 0x01}, u16(100)...), hs[:100]...)
	split = append(split, append(append([]byte{0x16, 0x03, 0x01}, u16(len(hs)-100)...), hs[100:]...)...)
	if JA4(split) != JA4(h) {
		t.Fatal("a ClientHello across two records")
	}
	for _, bad := range [][]byte{nil, []byte("GET / HTTP/1.1\r\n"), h[:60], {0x16, 3, 1, 0, 4, 1, 0xff, 0xff, 0xff}} {
		if got := JA4(bad); got != "" {
			t.Fatalf("JA4(%q) = %q", bad, got)
		}
	}
}

func TestHelloConnRecordsOnlyTheHello(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	h := chromeHello()
	go func() {
		b.Write(h[:50])
		b.Write(append(h[50:], []byte("after the hello")...))
		b.Close()
	}()
	c := NewHelloConn(a)
	buf := make([]byte, 4096)
	for {
		if _, err := c.Read(buf); err != nil {
			break
		}
	}
	if !c.ReaderReplaceable() || len(c.buf) > len(h)+len("after the hello") {
		t.Fatalf("kept %d bytes, done %v", len(c.buf), c.done.Load())
	}
	if c.JA4() != "t13d1516h2_8daaf6152771_e5627efa2ab1" {
		t.Fatalf("JA4 = %q", c.JA4())
	}
	if binary.BigEndian.Uint16(h[3:5]) == 0 {
		t.Fatal("empty record")
	}
}
