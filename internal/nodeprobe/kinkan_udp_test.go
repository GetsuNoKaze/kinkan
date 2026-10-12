package nodeprobe

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestUDPNoiseResponseTimeoutAndCancellation(t *testing.T) {
	c, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	go func() {
		b := make([]byte, 2048)
		for {
			n, addr, err := c.ReadFrom(b)
			if err != nil {
				return
			}
			if n > 0 && b[0] == 1 {
				_, _ = c.WriteTo([]byte("response"), addr)
			}
		}
	}()
	u, _ := origin("https://" + c.LocalAddr().String())
	cfg := Config{Protocol: "shadowsocks", Timeout: 100 * time.Millisecond, KinkanConfig: KinkanConfig{UDPNoise: true}}
	a := udpNoise(context.Background(), cfg, u, []byte{1})
	if a.Kind != "response" || a.Bytes != 8 || a.Digest == "" {
		t.Fatal(a)
	}
	a = udpNoise(context.Background(), cfg, u, []byte{0})
	if a.Kind != "timeout" {
		t.Fatal(a)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if a := udpNoise(ctx, cfg, u, []byte{0}); a.Kind != "error" {
		t.Fatal(a)
	}
	r := Report{Protocol: "shadowsocks", Reference: u.String()}
	r.scanUDPNoise(context.Background(), cfg, u)
	r.Summarize()
	if !r.Incomplete || r.Verdict == "quiet" {
		t.Fatal("UDP silence claimed concealment", r)
	}
}
