package nodeprobe

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"time"

	http "github.com/metacubex/http"
	quic "github.com/metacubex/quic-go"
	"github.com/metacubex/quic-go/http3"
	tls "github.com/metacubex/tls"
)

func quicTLS(cfg Config, u *url.URL, alpn []string) *tls.Config {
	c := &tls.Config{ServerName: u.Hostname(), NextProtos: alpn, MinVersion: tls.VersionTLS13}
	if cfg.TLS != nil {
		c.RootCAs = cfg.TLS.RootCAs
	}
	return c
}

func dialQUIC(ctx context.Context, cfg Config, u *url.URL, alpn []string) (*quic.Conn, func(), error) {
	addr, err := net.ResolveUDPAddr("udp", endpoint(u, cfg.Address))
	if err != nil {
		return nil, func() {}, err
	}
	pc, err := net.ListenUDP("udp", nil)
	if err != nil {
		return nil, func() {}, err
	}
	t := &quic.Transport{Conn: pc}
	cleanup := func() { _ = t.Close(); _ = pc.Close() }
	c, err := t.Dial(ctx, addr, quicTLS(cfg, u, alpn), &quic.Config{HandshakeIdleTimeout: cfg.Timeout, MaxIdleTimeout: cfg.Timeout + time.Second})
	if err != nil {
		cleanup()
		return nil, func() {}, err
	}
	return c, func() { _ = c.CloseWithError(0, ""); cleanup() }, nil
}

func quicError(err error) (level, detail string) {
	level, detail = "ERROR", fmt.Sprint(err)
	var app *quic.ApplicationError
	if errors.As(err, &app) && app.Remote {
		level = "WARN"
		detail = fmt.Sprintf("remote application close: code=0x%x reason=%q", uint64(app.ErrorCode), app.ErrorMessage)
		// These codes identify TUIC's unauthenticated server (v4 and v5).
		if app.ErrorCode == 0xfffffff1 || app.ErrorCode == 0xfffffff2 {
			level = "FAIL"
		}
	}
	return
}

func (r *Report) scanQUIC(ctx context.Context, cfg Config, u *url.URL) {
	if cfg.Obfuscated || cfg.Protocol == "shadowquic" {
		r.Incomplete = true
		r.add("obfuscation", "INFO", "plain QUIC probes do not authenticate or decode obfuscation; silence is inconclusive")
	}
	alpns := [][]string{{"h3"}}
	if len(cfg.ALPN) > 0 && !(len(cfg.ALPN) == 1 && cfg.ALPN[0] == "h3") {
		alpns = append(alpns, cfg.ALPN)
	}
	for _, alpn := range alpns {
		part, cancel := context.WithTimeout(ctx, cfg.Timeout)
		start := time.Now()
		c, closeConn, err := dialQUIC(part, cfg, u, alpn)
		if err != nil {
			level, detail := quicError(err)
			r.add("QUIC handshake", level, detail)
			cancel()
			continue
		}
		state := c.ConnectionState().TLS
		cert := state.PeerCertificates[0]
		r.add("QUIC TLS", "INFO", fmt.Sprintf("ALPN=%q; issuer=%q; DNS SAN=%q; expires=%s; certificate sha256=%x", state.NegotiatedProtocol, cert.Issuer.String(), cert.DNSNames, cert.NotAfter.UTC().Format(time.RFC3339), sha256.Sum256(cert.Raw)))
		select {
		case <-c.Context().Done():
			level, detail := quicError(context.Cause(c.Context()))
			r.add("QUIC without credentials", level, fmt.Sprintf("%s; elapsed=%s", detail, time.Since(start).Round(time.Millisecond)))
		case <-part.Done():
			if ctx.Err() != nil {
				r.add("QUIC without credentials", "ERROR", ctx.Err().Error())
			} else {
				r.add("QUIC without credentials", "INFO", "no remote close within observation window")
			}
		}
		closeConn()
		cancel()
	}
	for _, p := range []struct {
		method, path string
		wrong        bool
	}{{"GET", "/", false}, {"POST", "/auth", true}} {
		if ctx.Err() != nil {
			r.add("HTTP/3", "ERROR", ctx.Err().Error())
			return
		}
		a, err := h3Request(ctx, cfg, u, u.Host, p.method, p.path, p.wrong)
		name := "HTTP/3 " + p.method + " " + p.path
		if err != nil {
			level, detail := quicError(err)
			r.add(name, level, detail)
			continue
		}
		level, detail := "INFO", a.String()
		if a.status == 407 || a.proxyAuth || a.status == 233 {
			r.add(name, "FAIL", "protocol-specific authentication response: "+detail)
			continue
		}
		if cfg.Reference != "" {
			ref, _ := origin(cfg.Reference)
			b, err := h3Request(ctx, cfg, ref, u.Host, p.method, p.path, p.wrong)
			if err != nil {
				level = "ERROR"
				detail += "; cover: " + err.Error()
			} else {
				level = "PASS"
				detail += "; cover: " + b.String()
				if a.status != b.status || a.hash != b.hash {
					level = "WARN"
				}
			}
		}
		r.add(name, level, detail)
	}
	if cfg.Protocol == "tuic" {
		r.tuicWrongAuth(ctx, cfg, u)
	}
}

type h3Response struct {
	status    int
	hash      string
	proxyAuth bool
}

func (r h3Response) String() string { return fmt.Sprintf("status=%d sha256=%s", r.status, r.hash) }

func h3Request(ctx context.Context, cfg Config, u *url.URL, host, method, path string, wrong bool) (h3Response, error) {
	part, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	c, closeConn, err := dialQUIC(part, cfg, u, []string{"h3"})
	if err != nil {
		return h3Response{}, err
	}
	defer closeConn()
	t := &http3.Transport{MaxResponseHeaderBytes: 64 << 10}
	cc := t.NewClientConn(c)
	v := *u
	v.Path = path
	req, err := http.NewRequestWithContext(part, method, v.String(), nil)
	if err != nil {
		return h3Response{}, err
	}
	req.Host = host
	req.Header.Set("User-Agent", "kinkan-probe/1")
	req.Header.Set("Accept-Encoding", "identity")
	if wrong {
		req.Host = "hysteria"
		req.Header.Set("Hysteria-Auth", "kinkan-probe-invalid")
		req.Header.Set("Hysteria-CC-RX", "0")
	}
	resp, err := cc.RoundTrip(req)
	if err != nil {
		return h3Response{}, err
	}
	defer resp.Body.Close()
	_, auth := resp.Header["Proxy-Authenticate"]
	result := h3Response{status: resp.StatusCode, proxyAuth: auth}
	if auth || resp.StatusCode == 407 || resp.StatusCode == 233 {
		return result, nil
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, (2<<20)+1))
	if err != nil {
		return result, err
	}
	if len(b) > 2<<20 {
		return result, errors.New("HTTP/3 body exceeds 2 MiB")
	}
	result.hash = fmt.Sprintf("%x", sha256.Sum256(b))
	return result, nil
}

func (r *Report) tuicWrongAuth(ctx context.Context, cfg Config, u *url.URL) {
	part, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	alpn := cfg.ALPN
	if len(alpn) == 0 {
		alpn = []string{"h3"}
	}
	c, closeConn, err := dialQUIC(part, cfg, u, alpn)
	if err != nil {
		level, detail := quicError(err)
		r.add("TUIC wrong authentication", level, detail)
		return
	}
	defer closeConn()
	s, err := c.OpenUniStreamSync(part)
	if err == nil {
		b := make([]byte, 50)
		_, err = rand.Read(b)
		b[0], b[1] = 5, 0 // v5 AUTH: version, command, UUID, token
		if err == nil {
			_ = s.SetWriteDeadline(time.Now().Add(cfg.Timeout))
			_, err = s.Write(b)
			_ = s.Close()
		}
	}
	if err != nil {
		level, detail := quicError(err)
		r.add("TUIC wrong authentication", level, detail)
		return
	}
	select {
	case <-c.Context().Done():
		level, detail := quicError(context.Cause(c.Context()))
		r.add("TUIC wrong authentication", level, detail)
	case <-part.Done():
		r.add("TUIC wrong authentication", "ERROR", "no remote close within observation window")
	}
}
