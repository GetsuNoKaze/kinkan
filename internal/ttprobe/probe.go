// Package ttprobe checks the unauthenticated HTTPS surface of a TrustTunnel
// listener. It never supplies real credentials or follows redirects.
package ttprobe

import (
	"bufio"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/http2"
)

const maxBody = 2 << 20

type Finding struct {
	Name   string `json:"name"`
	Level  string `json:"level"`
	Detail string `json:"detail"`
}

type Report struct {
	Target    string    `json:"target"`
	Reference string    `json:"reference,omitempty"`
	Findings  []Finding `json:"findings"`
}

// Config deliberately keeps custom trust roots out of the public CLI: production
// probes always validate certificates using the system trust store.
type Config struct {
	Target, Reference string
	Timeout           time.Duration
	TLS               *tls.Config
}

func (r *Report) add(name, level, detail string) {
	r.Findings = append(r.Findings, Finding{name, level, detail})
}

// CheckError means an observed leak or an incomplete check, rather than a CLI error.
type CheckError struct{ Failed, Incomplete int }

func (e *CheckError) Error() string {
	return fmt.Sprintf("%d failed, %d incomplete checks", e.Failed, e.Incomplete)
}

func Run(ctx context.Context, args []string, out, stderr io.Writer) error {
	f := flag.NewFlagSet("kinkan probe", flag.ContinueOnError)
	f.SetOutput(stderr)
	ref := f.String("reference", "", "HTTPS cover site to compare (e.g. https://example.org:8444)")
	timeout := f.Duration("timeout", 5*time.Second, "deadline per request/handshake (100ms..30s)")
	jsonOut := f.Bool("json", false, "write a machine-readable report")
	f.Usage = func() {
		fmt.Fprintln(stderr, "Usage: kinkan probe [--reference HTTPS_URL] [--timeout 5s] [--json] HTTPS_URL")
		f.PrintDefaults()
	}
	if err := f.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if f.NArg() != 1 {
		f.Usage()
		return errors.New("one HTTPS target is required; flags precede the target")
	}
	r, err := Scan(ctx, Config{Target: f.Arg(0), Reference: *ref, Timeout: *timeout})
	if err != nil {
		return err
	}
	if *jsonOut {
		err = json.NewEncoder(out).Encode(r)
	} else {
		_, err = fmt.Fprintf(out, "Target: %s\nReference: %s\n", r.Target, r.Reference)
		for _, item := range r.Findings {
			if err != nil {
				break
			}
			_, err = fmt.Fprintf(out, "[%s] %s: %s\n", item.Level, item.Name, item.Detail)
		}
		if err == nil {
			_, err = fmt.Fprintln(out, "This checks observable responses, not resistance to blocking or every scanner fingerprint.")
		}
	}
	if err != nil {
		return err
	}
	failed, incomplete := 0, 0
	for _, item := range r.Findings {
		if item.Level == "FAIL" {
			failed++
		}
		if item.Level == "ERROR" {
			incomplete++
		}
	}
	if failed != 0 || incomplete != 0 {
		return &CheckError{failed, incomplete}
	}
	return nil
}

func endpoint(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u == nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || strings.HasSuffix(u.Host, ":") {
		return nil, fmt.Errorf("expected an HTTPS origin without credentials, path, query or fragment: %q", raw)
	}
	port := u.Port()
	if port == "" {
		port = "443"
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		return nil, errors.New("HTTPS port must be between 1 and 65535")
	}
	u.Path = ""
	return u, nil
}

func Scan(ctx context.Context, cfg Config) (Report, error) {
	r := Report{}
	u, err := endpoint(cfg.Target)
	if err != nil {
		return r, err
	}
	var ref *url.URL
	if cfg.Reference != "" {
		ref, err = endpoint(cfg.Reference)
		if err != nil {
			return r, err
		}
	}
	if cfg.Timeout < 100*time.Millisecond || cfg.Timeout > 30*time.Second {
		return r, errors.New("timeout must be between 100ms and 30s")
	}
	r.Target = u.String()
	if ref != nil {
		r.Reference = ref.String()
	}
	for _, alpn := range [][]string{nil, {"http/1.1"}, {"h2"}, {"h2", "http/1.1"}} {
		name := "TLS/ALPN " + strings.Join(alpn, ",")
		if len(alpn) == 0 {
			name = "TLS/no ALPN"
		}
		c, err := dial(ctx, cfg, u, alpn, u.Hostname(), false)
		if err != nil {
			r.add(name, "ERROR", err.Error())
			continue
		}
		s := c.ConnectionState()
		c.Close()
		level := "PASS"
		if len(alpn) > 0 && s.NegotiatedProtocol != alpn[0] {
			level = "WARN"
		}
		cert := s.PeerCertificates[0]
		sum := sha256.Sum256(cert.Raw)
		r.add(name, level, fmt.Sprintf("negotiated=%q; %s; issuer=%q; DNS SAN=%q; IP SAN=%v; expires=%s; certificate sha256=%x", s.NegotiatedProtocol, tls.VersionName(s.Version), cert.Issuer.String(), cert.DNSNames, cert.IPAddresses, cert.NotAfter.UTC().Format(time.RFC3339), sum))
	}
	// These handshakes intentionally use unexpected/no SNI. They are diagnostic:
	// default certificates and TLS alerts are both normal virtual-host behaviour.
	for _, sni := range []string{"", "kinkan-probe.invalid"} {
		name := "SNI " + sni
		if sni == "" {
			name = "SNI absent"
		}
		c, err := dial(ctx, cfg, u, []string{"http/1.1"}, sni, true)
		if err != nil {
			r.add(name, "INFO", "handshake rejected: "+err.Error())
			continue
		}
		cert := c.ConnectionState().PeerCertificates[0]
		c.Close()
		r.add(name, "INFO", fmt.Sprintf("diagnostic handshake only; unverified certificate DNS SAN=%q; IP SAN=%v", cert.DNSNames, cert.IPAddresses))
	}
	for _, alpn := range [][]string{nil, {"http/1.1"}} {
		name := "raw HTTP/2 preface without h2: " + strings.Join(alpn, ",")
		c, err := dial(ctx, cfg, u, alpn, u.Hostname(), false)
		if err != nil {
			r.add(name, "ERROR", err.Error())
			continue
		}
		stop := context.AfterFunc(ctx, func() { c.Close() })
		if _, err = io.WriteString(c, http2.ClientPreface); err == nil {
			err = http2.NewFramer(c, c).WriteSettings()
		}
		var prefix [9]byte
		if err == nil {
			_, err = io.ReadFull(c, prefix[:])
		}
		c.Close()
		stop()
		if err != nil {
			r.add(name, "WARN", "no complete response: "+err.Error())
			continue
		}
		if strings.HasPrefix(string(prefix[:]), "HTTP/1.") {
			r.add(name, "PASS", "HTTP/1 response")
		} else if prefix[3] == byte(http2.FrameSettings) && prefix[5] == 0 && prefix[6] == 0 && prefix[7] == 0 && prefix[8] == 0 {
			r.add(name, "FAIL", "HTTP/2 SETTINGS returned without negotiated h2")
		} else {
			r.add(name, "WARN", "unexpected response prefix: "+hex.EncodeToString(prefix[:]))
		}
	}
	for _, proto := range []string{"http/1.1", "h2"} {
		for _, p := range probes {
			if err := ctx.Err(); err != nil {
				return r, err
			}
			name := proto + " " + p.name
			a, err := request(ctx, cfg, u, u.Host, proto, p)
			if err != nil {
				r.add(name, "ERROR", err.Error())
				continue
			}
			if a.status == 407 || a.proxyAuth {
				r.add(name, "FAIL", fmt.Sprintf("proxy disclosure: status=%d Proxy-Authenticate=%t", a.status, a.proxyAuth))
				continue
			}
			if proto == "h2" && a.proto != "HTTP/2.0" {
				r.add(name, "FAIL", "HTTP/2 request did not use HTTP/2")
				continue
			}
			if ref == nil {
				r.add(name, "PASS", fmt.Sprintf("status=%d; no explicit proxy authentication response", a.status))
				continue
			}
			b, err := request(ctx, cfg, ref, u.Host, proto, p)
			if err != nil {
				r.add(name, "ERROR", "reference: "+err.Error())
				continue
			}
			if a.status != b.status || a.hash != b.hash {
				r.add(name, "WARN", fmt.Sprintf("cover differs: target status=%d sha256=%s; reference status=%d sha256=%s", a.status, a.hash, b.status, b.hash))
			} else {
				r.add(name, "PASS", fmt.Sprintf("status=%d and body match cover", a.status))
			}
		}
	}
	a, err := fingerprint(ctx, cfg, u, u.Host)
	if err != nil {
		r.add("HTTP/2 fingerprint", "ERROR", err.Error())
	} else if ref == nil {
		raw, _ := json.Marshal(a)
		r.add("HTTP/2 fingerprint", "INFO", string(raw))
	} else {
		b, err := fingerprint(ctx, cfg, ref, u.Host)
		if err != nil {
			r.add("HTTP/2 fingerprint", "ERROR", "reference: "+err.Error())
		} else {
			ra, _ := json.Marshal(a)
			rb, _ := json.Marshal(b)
			level := "PASS"
			if string(ra) != string(rb) {
				level = "WARN"
			}
			r.add("HTTP/2 fingerprint", level, fmt.Sprintf("target=%s reference=%s (SETTINGS order/values, initial windows, response header order)", ra, rb))
		}
	}
	if ref == nil {
		r.add("reference comparison", "WARN", "not performed; pass --reference https://COVER_HOST:PORT to compare responses and HTTP/2 fingerprint")
	}
	return r, nil
}

func dial(ctx context.Context, cfg Config, u *url.URL, alpn []string, sni string, diagnostic bool) (*tls.Conn, error) {
	settings := &tls.Config{MinVersion: tls.VersionTLS12}
	if cfg.TLS != nil {
		settings = cfg.TLS.Clone()
	}
	settings.NextProtos = alpn
	settings.ServerName = sni
	if !diagnostic && cfg.TLS != nil && cfg.TLS.ServerName != "" {
		settings.ServerName = cfg.TLS.ServerName
	}
	settings.InsecureSkipVerify = diagnostic
	port := u.Port()
	if port == "" {
		port = "443"
	}
	part, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	raw, err := (&net.Dialer{Timeout: cfg.Timeout}).DialContext(part, "tcp", net.JoinHostPort(u.Hostname(), port))
	if err != nil {
		return nil, err
	}
	// tls.Dialer fills an empty ServerName from the address, which would silently
	// turn the "SNI absent" probe into a normal SNI handshake.
	conn := tls.Client(raw, settings)
	if err := conn.HandshakeContext(part); err != nil {
		conn.Close()
		return nil, err
	}
	deadline, _ := part.Deadline()
	conn.SetDeadline(deadline)
	return conn, nil
}

type probe struct{ name, method, path, auth string }

var probes = []probe{
	{"GET /", "GET", "/", ""},
	{"HEAD /", "HEAD", "/", ""},
	{"OPTIONS /", "OPTIONS", "/", ""},
	{"CONNECT", "CONNECT", "", ""},
	{"wrong Basic", "GET", "/", "Basic a2lua2FuLXByb2JlOmludmFsaWQ="},
	{"malformed Basic", "GET", "/", "Basic !!!"},
	{"wrong Bearer", "GET", "/", "Bearer kinkan-probe-invalid"},
	{"CONNECT wrong Basic", "CONNECT", "", "Basic a2lua2FuLXByb2JlOmludmFsaWQ="},
	{"robots", "GET", "/robots.txt", ""},
	{"favicon", "GET", "/favicon.ico", ""},
	{"security.txt", "GET", "/.well-known/security.txt", ""},
	{"missing path", "GET", "/__kinkan_probe_missing_9e74d4__", ""},
}

type response struct {
	status      int
	proto, hash string
	proxyAuth   bool
}

func request(ctx context.Context, cfg Config, u *url.URL, host, proto string, p probe) (response, error) {
	var result response
	settings := &tls.Config{MinVersion: tls.VersionTLS12}
	if cfg.TLS != nil {
		settings = cfg.TLS.Clone()
	}
	settings.InsecureSkipVerify = false
	settings.ServerName = u.Hostname()
	if cfg.TLS != nil && cfg.TLS.ServerName != "" {
		settings.ServerName = cfg.TLS.ServerName
	}
	var transport http.RoundTripper
	var closeIdle func()
	if proto == "h2" {
		t := &http2.Transport{TLSClientConfig: settings, DisableCompression: true, MaxHeaderListSize: 64 << 10}
		transport = t
		closeIdle = t.CloseIdleConnections
	} else {
		t := &http.Transport{TLSClientConfig: settings, ForceAttemptHTTP2: false, TLSNextProto: map[string]func(string, *tls.Conn) http.RoundTripper{}, DisableCompression: true, DisableKeepAlives: true, MaxResponseHeaderBytes: 64 << 10}
		transport = t
		closeIdle = t.CloseIdleConnections
	}
	defer closeIdle()
	part, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	v := *u
	v.Path = p.path
	req, err := http.NewRequestWithContext(part, p.method, v.String(), nil)
	if err != nil {
		return result, err
	}
	req.Host = host
	req.Header.Set("User-Agent", "kinkan-probe/1")
	req.Header.Set("Accept-Encoding", "identity")
	if p.auth != "" {
		req.Header.Set("Proxy-Authorization", p.auth)
	}
	resp, err := transport.RoundTrip(req)
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()
	_, auth := resp.Header["Proxy-Authenticate"]
	if resp.StatusCode == 407 || auth {
		return response{status: resp.StatusCode, proto: resp.Proto, proxyAuth: auth}, nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return result, err
	}
	if len(body) > maxBody {
		return result, errors.New("response exceeds 2 MiB; comparison incomplete")
	}
	sum := sha256.Sum256(body)
	return response{resp.StatusCode, resp.Proto, hex.EncodeToString(sum[:]), auth}, nil
}

// Used by the raw framer to bound reads without buffering an untrusted body.
func reader(c *tls.Conn) *bufio.Reader { return bufio.NewReaderSize(c, 16<<10) }
