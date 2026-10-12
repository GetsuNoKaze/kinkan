package nodeprobe

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"net"
	"net/url"
	"time"
)

// These are noise probes, not a substitute for encrypted protocol authentication.
// Even equal silence on the target and cover never yields a PASS.
func udpNoise(ctx context.Context, cfg Config, u *url.URL, payload []byte) (o AuthObservation) {
	start := time.Now()
	defer func() { o.ElapsedMS = time.Since(start).Milliseconds() }()
	o.Kind, o.Close = "error", "not_observed"
	part, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	c, err := (&net.Dialer{}).DialContext(part, "udp", endpoint(u, cfg.Address))
	if err != nil {
		o.Detail = err.Error()
		return
	}
	defer c.Close()
	dl, _ := part.Deadline()
	_ = c.SetDeadline(dl)
	stop := context.AfterFunc(part, func() { _ = c.Close() })
	defer stop()
	if _, err = c.Write(payload); err != nil {
		o.Detail = err.Error()
		return
	}
	b := make([]byte, 2048)
	n, err := c.Read(b)
	if n > 0 {
		o.Kind, o.Bytes, o.Digest = "response", n, fmt.Sprintf("%x", sha256.Sum256(b[:n]))
		return
	}
	if ctx.Err() != nil {
		o.Detail = ctx.Err().Error()
		return
	}
	varTimeout := false
	if ne, ok := err.(net.Error); ok {
		varTimeout = ne.Timeout()
	}
	if varTimeout || part.Err() != nil {
		o.Kind, o.Detail = "timeout", "no datagram within observation window"
		return
	}
	o.Detail = fmt.Sprint(err)
	return
}

func (r *Report) scanUDPNoise(ctx context.Context, cfg Config, u *url.URL) {
	if !cfg.UDPNoise {
		return
	}
	switch cfg.Protocol {
	case "shadowsocks", "snell", "sudoku", "mieru":
	default:
		return
	}
	r.Incomplete = true
	for _, size := range []int{32, 256, 1024} {
		if ctx.Err() != nil {
			r.add("UDP noise", "ERROR", ctx.Err().Error())
			return
		}
		payload := make([]byte, size)
		if _, err := rand.Read(payload); err != nil {
			r.add("UDP noise", "ERROR", err.Error())
			return
		}
		a := udpNoise(ctx, cfg, u, payload)
		level, detail := "INFO", fmt.Sprintf("target=%+v", a)
		if a.Kind == "error" {
			level = "ERROR"
		}
		if cfg.Reference != "" {
			ref, _ := origin(cfg.Reference)
			b := udpNoise(ctx, cfg, ref, payload)
			detail += fmt.Sprintf("; cover UDP=%+v", b)
			if b.Kind == "error" {
				level = "ERROR"
			} else if a.Kind == "response" && b.Kind == "response" && a.Digest != b.Digest {
				level = "WARN"
			}
		}
		r.add(fmt.Sprintf("UDP noise/%d", size), level, detail+"; silence or timing cannot prove concealment")
	}
}
