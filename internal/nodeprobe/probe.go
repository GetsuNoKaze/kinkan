// Package nodeprobe measures unauthenticated inbound responses. It does not log in,
// forward user traffic or change the node's configuration.
package nodeprobe

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"mikan/internal/ttprobe"
)

type Config struct {
	Protocol  string `json:"protocol"`
	Target    string `json:"target"`
	Reference string `json:"reference,omitempty"`
	// Address pins BOTH target and reference to the node, retaining their TLS names.
	Address    string        `json:"address,omitempty"`
	Obfuscated bool          `json:"obfuscated,omitempty"`
	ALPN       []string      `json:"alpn,omitempty"`
	Timeout    time.Duration `json:"-"`
	TLS        *tls.Config   `json:"-"` // custom roots for isolated tests only
}

type Report struct {
	Protocol   string            `json:"protocol"`
	Verdict    string            `json:"verdict" enum:"quiet,noticeable,exposed,inconclusive"`
	Incomplete bool              `json:"incomplete"`
	Target     string            `json:"target"`
	Reference  string            `json:"reference,omitempty"`
	Findings   []ttprobe.Finding `json:"findings"`
}

func (r *Report) add(name, level, detail string) {
	r.Findings = append(r.Findings, ttprobe.Finding{Name: name, Level: level, Detail: detail})
}

// Summarize never turns a timeout, an obfuscated port or an unsupported transport
// into a quiet verdict. A positive leak still counts when other checks failed.
func (r *Report) Summarize() {
	r.Verdict = "inconclusive"
	fail, warn, pass := false, false, false
	for _, f := range r.Findings {
		switch f.Level {
		case "FAIL":
			fail = true
		case "WARN":
			if f.Name != "reference comparison" {
				warn = true
			}
		case "ERROR":
			r.Incomplete = true
		case "PASS":
			pass = true
		}
	}
	switch {
	case fail:
		r.Verdict = "exposed"
	case warn:
		r.Verdict = "noticeable"
	case !r.Incomplete && pass && r.Reference != "":
		r.Verdict = "quiet"
	}
}

func origin(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u == nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Opaque != "" || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.HasSuffix(u.Host, ":") {
		return nil, errors.New("expected HTTPS origin without credentials, path, query or fragment")
	}
	if u.Port() != "" {
		n, err := strconv.Atoi(u.Port())
		if err != nil || n < 1 || n > 65535 {
			return nil, errors.New("invalid port")
		}
	}
	u.Path = ""
	return u, nil
}

func endpoint(u *url.URL, address string) string {
	host := u.Hostname()
	if address != "" {
		host = address
	}
	port := u.Port()
	if port == "" {
		port = "443"
	}
	return net.JoinHostPort(host, port)
}

func Scan(ctx context.Context, cfg Config) (r Report, err error) {
	r = Report{Protocol: cfg.Protocol, Target: cfg.Target, Reference: cfg.Reference, Findings: []ttprobe.Finding{}}
	u, err := origin(cfg.Target)
	if err != nil {
		return r, err
	}
	if cfg.Reference != "" {
		if _, err := origin(cfg.Reference); err != nil {
			return r, err
		}
	}
	if cfg.Address != "" {
		ip, err := netip.ParseAddr(cfg.Address)
		if err != nil || ip.Zone() != "" {
			return r, errors.New("address must be an IP without a zone")
		}
	}
	if cfg.Timeout < 100*time.Millisecond || cfg.Timeout > 30*time.Second {
		return r, errors.New("timeout must be between 100ms and 30s")
	}
	defer r.Summarize()
	if err := ctx.Err(); err != nil {
		r.add("scan", "ERROR", err.Error())
		return r, nil
	}
	switch cfg.Protocol {
	case "trusttunnel", "vless", "vmess", "trojan", "anytls":
		tc := ttprobe.Config{Target: cfg.Target, Reference: cfg.Reference, Timeout: cfg.Timeout, TLS: cfg.TLS}
		if cfg.Address != "" {
			tc.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
				_, port, err := net.SplitHostPort(addr)
				if err != nil {
					return nil, err
				}
				return (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort(cfg.Address, port))
			}
		}
		report, err := ttprobe.Scan(ctx, tc)
		r.Findings = append(r.Findings, report.Findings...)
		if err != nil {
			r.add("HTTPS scan", "ERROR", err.Error())
		}
		r.scanBytes(ctx, cfg, u, true)
	case "hysteria2", "tuic", "shadowquic":
		r.scanQUIC(ctx, cfg, u)
	case "shadowsocks", "snell", "sudoku", "mieru":
		r.Incomplete = true // timing alone cannot establish a scanner fingerprint
		r.scanBytes(ctx, cfg, u, false)
	default:
		r.Incomplete = true
		r.add("transport", "ERROR", "unsupported protocol/transport: "+cfg.Protocol)
	}
	return r, nil
}

type observation struct {
	Kind    string
	Bytes   int
	Elapsed time.Duration
	Detail  string
}

func bytesProbe(ctx context.Context, cfg Config, u *url.URL, payload []byte, encrypted bool) observation {
	start := time.Now()
	part, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	c, err := (&net.Dialer{}).DialContext(part, "tcp", endpoint(u, cfg.Address))
	if err != nil {
		return observation{Kind: "error", Detail: err.Error(), Elapsed: time.Since(start)}
	}
	defer c.Close()
	deadline, _ := part.Deadline()
	_ = c.SetDeadline(deadline)
	// Cancellation must interrupt a blocked read before its per-probe deadline.
	stop := context.AfterFunc(part, func() { _ = c.Close() })
	defer stop()
	if encrypted {
		tc := &tls.Config{MinVersion: tls.VersionTLS12}
		if cfg.TLS != nil {
			tc = cfg.TLS.Clone()
		}
		tc.ServerName = u.Hostname()
		tc.NextProtos = []string{"http/1.1"}
		tc.InsecureSkipVerify = false
		t := tls.Client(c, tc)
		if err := t.HandshakeContext(part); err != nil {
			return observation{Kind: "error", Detail: err.Error(), Elapsed: time.Since(start)}
		}
		c = t
	}
	if _, err := c.Write(payload); err != nil {
		return observation{Kind: "error", Detail: err.Error(), Elapsed: time.Since(start)}
	}
	b := make([]byte, 512)
	n, err := c.Read(b)
	o := observation{Bytes: n, Elapsed: time.Since(start)}
	switch {
	case n > 0:
		o.Kind = "response"
		prefix := string(b[:n])
		if strings.HasPrefix(prefix, "HTTP/") {
			prefix = strings.SplitN(prefix, "\r\n", 2)[0]
		}
		o.Detail = fmt.Sprintf("prefix=%q", prefix)
	case errors.Is(err, io.EOF):
		o.Kind = "closed"
	case part.Err() != nil:
		o.Kind = "timeout"
		o.Detail = part.Err().Error()
	default:
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			o.Kind = "timeout"
		} else {
			o.Kind = "error"
			o.Detail = fmt.Sprint(err)
		}
	}
	return o
}

func (o observation) String() string {
	return fmt.Sprintf("%s; bytes=%d; elapsed=%s; %s", o.Kind, o.Bytes, o.Elapsed.Round(time.Millisecond), o.Detail)
}

func (r *Report) scanBytes(ctx context.Context, cfg Config, u *url.URL, encrypted bool) {
	for _, size := range []int{32, 256, 1024} {
		if ctx.Err() != nil {
			r.add("random bytes", "ERROR", ctx.Err().Error())
			return
		}
		payload := make([]byte, size)
		if _, err := rand.Read(payload); err != nil {
			r.add("random bytes", "ERROR", err.Error())
			return
		}
		// A complete invalid HTTP request lets a cover site answer without waiting
		// for a newline that random bytes alone often lack.
		if encrypted {
			payload = append(payload, []byte("\r\n\r\n")...)
		}
		a := bytesProbe(ctx, cfg, u, payload, encrypted)
		level, detail := "INFO", a.String()
		if a.Kind == "error" || a.Kind == "timeout" {
			level = "ERROR"
		}
		if encrypted && cfg.Reference != "" {
			ref, _ := origin(cfg.Reference)
			b := bytesProbe(ctx, cfg, ref, payload, true)
			detail += "; cover: " + b.String()
			if b.Kind == "error" || b.Kind == "timeout" {
				level = "ERROR"
			} else if level != "ERROR" {
				level = "PASS"
				// Duration is reported, never used as a binary fingerprint threshold.
				if a.Kind != b.Kind || a.Detail != b.Detail {
					level = "WARN"
				}
			}
		}
		r.add(fmt.Sprintf("random bytes/%d", size), level, detail)
	}
}
