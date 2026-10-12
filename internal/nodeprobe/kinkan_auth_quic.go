package nodeprobe

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	http "github.com/metacubex/http"
	quic "github.com/metacubex/quic-go"
)

func remoteAuthClose(err error) AuthObservation {
	o := AuthObservation{Kind: "error", Close: "not_observed", Detail: fmt.Sprint(err)}
	var app *quic.ApplicationError
	if errors.As(err, &app) && app.Remote {
		o.Kind, o.Close = "closed", fmt.Sprintf("application:0x%x", uint64(app.ErrorCode))
		o.Detail = "remote application close"
		if app.ErrorCode == 0xfffffff1 || app.ErrorCode == 0xfffffff2 {
			o.Detail = "TUIC authentication close"
		}
	}
	var transport *quic.TransportError
	if errors.As(err, &transport) && transport.Remote {
		o.Kind, o.Close, o.Detail = "closed", fmt.Sprintf("transport:0x%x", uint64(transport.ErrorCode)), "remote transport close"
	}
	return o
}

func wrongQUIC(ctx context.Context, cfg Config, u *url.URL, payload []byte) (o AuthObservation) {
	start := time.Now()
	defer func() { o.ElapsedMS = time.Since(start).Milliseconds() }()
	part, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	alpn := cfg.ALPN
	if len(alpn) == 0 {
		alpn = []string{"h3"}
	}
	c, cleanup, err := dialQUIC(part, cfg, u, alpn)
	if err != nil {
		return remoteAuthClose(err)
	}
	defer cleanup()
	s, err := c.OpenUniStreamSync(part)
	if err == nil {
		deadline, _ := part.Deadline()
		_ = s.SetWriteDeadline(deadline)
		_, err = s.Write(payload)
		_ = s.Close()
	}
	if err != nil {
		return remoteAuthClose(err)
	}
	select {
	case <-c.Context().Done():
		return remoteAuthClose(context.Cause(c.Context()))
	case <-part.Done():
		if ctx.Err() != nil {
			return AuthObservation{Kind: "error", Close: "cancelled", Detail: ctx.Err().Error()}
		}
		return AuthObservation{Kind: "open", Close: "observation_window", Detail: "no remote close within observation window"}
	}
}

func wrongH3(ctx context.Context, cfg Config, u *url.URL) (o AuthObservation) {
	start := time.Now()
	defer func() { o.ElapsedMS = time.Since(start).Milliseconds() }()
	a, err := h3Request(ctx, cfg, u, u.Host, "POST", "/auth", true)
	if err != nil {
		return remoteAuthClose(err)
	}
	o = AuthObservation{Kind: "response", Status: a.status, Digest: a.hash, Close: "not_observed"}
	if a.status == 407 || a.status == 233 || a.proxyAuth {
		o.Detail = "proxy authentication response"
	}
	return
}

func (r *Report) scanWrongQUIC(ctx context.Context, cfg Config, u *url.URL) {
	c := &WrongKeyComparison{Verdict: "inconclusive", Reason: "wrong-key obfuscation framing is not implemented; plain QUIC silence does not prove concealment"}
	r.WrongKey = c
	if cfg.Obfuscated || cfg.Protocol == "shadowquic" {
		r.Incomplete = true
		r.add("wrong key", "INFO", c.Reason)
		return
	}
	var probe func(*url.URL, Config) AuthObservation
	if cfg.Protocol == "hysteria2" {
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			c.Reason = err.Error()
			r.authFinding()
			return
		}
		cfg.wrongAuth = hex.EncodeToString(key)
		probe = func(v *url.URL, opts Config) AuthObservation { return wrongH3(ctx, opts, v) }
	} else {
		payload := make([]byte, 50)
		if _, err := rand.Read(payload); err != nil {
			c.Reason = err.Error()
			r.add("wrong key", "ERROR", c.Reason)
			return
		}
		payload[0], payload[1] = 5, 0
		probe = func(v *url.URL, opts Config) AuthObservation { return wrongQUIC(ctx, opts, v, payload) }
	}
	c.Target = probe(u, cfg)
	if cfg.Reference != "" {
		ref, _ := origin(cfg.Reference)
		if strings.EqualFold(u.Hostname(), ref.Hostname()) && endpoint(u, cfg.Address) == endpoint(ref, cfg.Address) {
			r.Incomplete = true
		} else {
			refCfg := cfg
			refCfg.ALPN = []string{"h3"}
			b := probe(ref, refCfg)
			c.Reference = &b
		}
	}
	c.Verdict, c.Reason = compareAuth(c.Target, c.Reference)
	if c.Target.Detail == "TUIC authentication close" {
		c.Verdict, c.Reason = "exposed", "remote TUIC authentication close identifies the protocol"
	}
	if cfg.Protocol == "hysteria2" {
		c.Reason += "; HTTP/3 response time measured, connection close not observed"
	}
	r.authFinding()
}

func kinkanAuthRequest(req *http.Request, cfg Config) {
	if cfg.wrongAuth != "" && req.Header.Get("Hysteria-Auth") != "" {
		req.Header.Set("Hysteria-Auth", cfg.wrongAuth)
	}
}
