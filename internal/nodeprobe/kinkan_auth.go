package nodeprobe

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
)

type KinkanConfig struct {
	// Only the raw TCP transport is supported; HTTP transports need their own framing.
	Transport string `json:"transport,omitempty"`
	PlainTCP  bool   `json:"plain_tcp,omitempty"`
	UDPNoise  bool   `json:"udp_noise,omitempty"`
	wrongAuth string
}

type KinkanReport struct {
	WrongKey *WrongKeyComparison `json:"wrong_key,omitempty"`
}

type AuthObservation struct {
	Kind      string `json:"kind"`
	Bytes     int    `json:"bytes"`
	Status    int    `json:"status,omitempty"`
	Digest    string `json:"sha256,omitempty"`
	ElapsedMS int64  `json:"elapsed_ms"`
	Close     string `json:"close"`
	Detail    string `json:"detail,omitempty"`
}

type WrongKeyComparison struct {
	Verdict   string           `json:"verdict" enum:"site,different,exposed,inconclusive"`
	Reason    string           `json:"reason"`
	Target    AuthObservation  `json:"target"`
	Reference *AuthObservation `json:"reference,omitempty"`
}

func wrongKeyPayload(protocol string) ([]byte, error) {
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	switch protocol {
	case "vless":
		// v0, UUID, no addons, TCP, port 80, IPv4 TEST-NET-1. No real destination.
		b := append([]byte{0}, key...)
		return append(b, 0, 1, 0, 80, 1, 192, 0, 2, 1), nil
	case "trojan":
		h := sha256.Sum224(key)
		return append([]byte(hex.EncodeToString(h[:])+"\r\n"), 1, 1, 192, 0, 2, 1, 0, 80, '\r', '\n'), nil
	case "anytls":
		h := sha256.Sum256(key)
		return append(h[:], 0, 0), nil // password digest, zero padding
	default:
		return nil, fmt.Errorf("wrong-key framing is not implemented for %s", protocol)
	}
}

func authTCP(ctx context.Context, cfg Config, u *url.URL, payload []byte) (o AuthObservation) {
	o.Kind, o.Close = "error", "not_connected"
	start := time.Now()
	defer func() { o.ElapsedMS = time.Since(start).Milliseconds() }()
	part, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	c, err := (&net.Dialer{}).DialContext(part, "tcp", endpoint(u, cfg.Address))
	if err != nil {
		o.Detail = err.Error()
		return
	}
	defer c.Close()
	deadline, _ := part.Deadline()
	_ = c.SetDeadline(deadline)
	stop := context.AfterFunc(part, func() { _ = c.Close() })
	defer stop()
	if !cfg.PlainTCP {
		tc := &tls.Config{MinVersion: tls.VersionTLS12}
		if cfg.TLS != nil {
			tc = cfg.TLS.Clone()
		}
		tc.ServerName, tc.NextProtos, tc.InsecureSkipVerify = u.Hostname(), []string{"http/1.1"}, false
		t := tls.Client(c, tc)
		if err = t.HandshakeContext(part); err != nil {
			o.Detail = err.Error()
			return
		}
		c = t
	}
	if _, err = c.Write(payload); err != nil {
		o.Detail = err.Error()
		return
	}
	o.Close = "observation_window"
	data, err := io.ReadAll(io.LimitReader(c, (64<<10)+1))
	o.Bytes = len(data)
	switch {
	case len(data) > 64<<10:
		o.Detail = "response exceeds 64 KiB"
		return
	case ctx.Err() != nil:
		o.Detail = ctx.Err().Error()
		return
	case err == nil:
		o.Close = "eof"
	case errors.Is(err, syscall.ECONNRESET):
		o.Close = "reset"
	default:
		var ne net.Error
		if !errors.As(err, &ne) || !ne.Timeout() {
			if part.Err() == nil {
				o.Detail = err.Error()
				return
			}
		}
	}
	if len(data) == 0 {
		if o.Close == "observation_window" {
			o.Kind = "timeout"
		} else {
			o.Kind = "closed"
		}
		return
	}
	o.Kind = "response"
	if strings.HasPrefix(string(data), "HTTP/") {
		resp, parseErr := http.ReadResponse(bufio.NewReader(bytes.NewReader(data)), nil)
		if parseErr != nil {
			o.Kind, o.Detail = "error", "incomplete HTTP response"
			return
		}
		defer resp.Body.Close()
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			o.Kind, o.Detail = "error", "incomplete HTTP body"
			return
		}
		o.Status = resp.StatusCode
		_, proxyAuth := resp.Header["Proxy-Authenticate"]
		if o.Status == 407 || proxyAuth {
			o.Detail = "proxy authentication response"
		}
		data = body // Date and other dynamic headers are not fingerprints.
	}
	h := sha256.Sum256(data)
	o.Digest = hex.EncodeToString(h[:])
	return
}

func compareAuth(a AuthObservation, b *AuthObservation) (string, string) {
	if a.Detail == "proxy authentication response" {
		return "exposed", "proxy authentication response to an invalid key"
	}
	if a.Kind == "error" || a.Kind == "timeout" {
		return "inconclusive", "target did not complete the wrong-key observation: " + a.Detail
	}
	if b == nil {
		return "inconclusive", "no independent cover-site reference"
	}
	if b.Kind == "error" || b.Kind == "timeout" {
		return "inconclusive", "cover-site observation did not complete: " + b.Detail
	}
	if a.Kind != b.Kind || a.Close != b.Close || a.Status != b.Status || a.Digest != b.Digest {
		return "different", "wrong-key response or close differs from the cover site; this alone does not identify the protocol"
	}
	return "site", "wrong-key response and close match the cover site within the observation window; timing is reported separately"
}

func (r *Report) scanWrongKey(ctx context.Context, cfg Config, u *url.URL) {
	if ctx.Err() != nil {
		return
	}
	defer r.scanUDPNoise(ctx, cfg, u)
	if cfg.Protocol == "tuic" || cfg.Protocol == "hysteria2" || cfg.Protocol == "shadowquic" {
		r.scanWrongQUIC(ctx, cfg, u)
		return
	}
	comparison := &WrongKeyComparison{Verdict: "inconclusive", Reason: "wrong-key framing is not implemented for this transport"}
	r.WrongKey = comparison
	if cfg.Transport != "" && cfg.Transport != "tcp" {
		comparison.Reason = "wrong-key framing is not implemented for transport " + cfg.Transport
		r.Incomplete = true
		r.add("wrong key", "INFO", comparison.Reason)
		return
	}
	var payload []byte
	var err error
	if cfg.Protocol == "trusttunnel" {
		key := make([]byte, 32)
		if _, err = rand.Read(key); err != nil {
			comparison.Reason = err.Error()
			r.authFinding()
			return
		}
		auth := base64.StdEncoding.EncodeToString([]byte(hex.EncodeToString(key[:16]) + ":" + hex.EncodeToString(key[16:])))
		payload = []byte("GET / HTTP/1.1\r\nHost: " + u.Host + "\r\nProxy-Authorization: Basic " + auth + "\r\nConnection: close\r\n\r\n")
	} else {
		payload, err = wrongKeyPayload(cfg.Protocol)
	}
	if err != nil {
		comparison.Reason = err.Error()
		r.Incomplete = true
		r.add("wrong key", "INFO", comparison.Reason)
		return
	}
	// Finish a malformed HTTP header too, so the cover can reject the same
	// bytes without waiting for a newline. Protocol servers reject the key first.
	if cfg.Protocol != "trusttunnel" {
		payload = append(payload, []byte("\r\n\r\n")...)
	}
	comparison.Target = authTCP(ctx, cfg, u, payload)
	if cfg.Reference != "" {
		ref, _ := origin(cfg.Reference)
		if strings.EqualFold(u.Hostname(), ref.Hostname()) && endpoint(u, cfg.Address) == endpoint(ref, cfg.Address) {
			comparison.Verdict, comparison.Reason = compareAuth(comparison.Target, nil)
			if comparison.Verdict != "exposed" {
				comparison.Reason = "reference is this inbound itself, not an independent cover site"
			}
			r.Incomplete = true
			r.authFinding()
			return
		}
		refCfg := cfg
		refCfg.PlainTCP = false // reference is always HTTPS, including plain TCP targets.
		if cfg.Protocol == "trusttunnel" {
			payload = bytes.Replace(payload, []byte("Host: "+u.Host+"\r\n"), []byte("Host: "+ref.Host+"\r\n"), 1)
		}
		b := authTCP(ctx, refCfg, ref, payload)
		comparison.Reference = &b
	}
	comparison.Verdict, comparison.Reason = compareAuth(comparison.Target, comparison.Reference)
	r.authFinding()
}

func (r *Report) authFinding() {
	c := r.WrongKey
	level := map[string]string{"site": "PASS", "different": "WARN", "exposed": "FAIL", "inconclusive": "ERROR"}[c.Verdict]
	r.add("wrong key", level, fmt.Sprintf("%s; target=%+v; cover=%+v", c.Reason, c.Target, c.Reference))
}
