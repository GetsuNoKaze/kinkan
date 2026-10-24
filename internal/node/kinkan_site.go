package node

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/pem"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"mikan/internal/fsutil"
	"mikan/internal/nodeapi"
	"mikan/internal/site"
)

// Kinkan: the website a node shows (ROADMAP item 2). The panel sends a site's archive
// once (PUT /v1/site/{hash}); the node keeps it under its data directory, checks it again
// and serves it on 127.0.0.1 — over plain HTTP for TrustTunnel's fallback and over TLS,
// with the node's certificate, as REALITY's own target. It answers the way Caddy's
// file_server does, the server a cover site usually stands behind.

// siteMaxHeaderBytes is Caddy's request header limit (see listen).
const siteMaxHeaderBytes = 16 << 10

// siteDir holds the archives, one per hash: the current one and none else once it serves.
const siteDir = "site"

func (e *Engine) sitePath(hash string) string {
	return filepath.Join(e.dataDir, siteDir, hash+".zip")
}

// PutSite keeps a site's archive under its hash, once it checks and hashes so.
func (e *Engine) PutSite(hash string, archive []byte) error {
	if !nodeapi.SiteHash.MatchString(hash) {
		return &nodeapi.Error{Code: "invalid_state", Message: "site hash"}
	}
	s, err := site.Unpack(archive)
	if err != nil {
		return &nodeapi.Error{Code: "invalid_state", Message: "site: " + err.Error()}
	}
	if s.Hash != hash {
		return &nodeapi.Error{Code: "invalid_state", Message: "site: the archive hashes to " + s.Hash}
	}
	if err := os.MkdirAll(filepath.Join(e.dataDir, siteDir), 0o700); err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(e.sitePath(hash), archive, 0o600)
}

// siteHandler is the Node API's PUT /v1/site/{hash}.
func siteHandler(e *Engine, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		archive, err := io.ReadAll(io.LimitReader(r.Body, site.MaxArchive+1))
		if err != nil {
			fail(w, log, err)
			return
		}
		if len(archive) > site.MaxArchive {
			writeJSON(w, http.StatusRequestEntityTooLarge, nodeapi.Error{Code: "invalid_state", Message: "site archive too large"})
			return
		}
		if err := e.PutSite(r.PathValue("hash"), archive); err != nil {
			fail(w, log, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// siteServer serves one site on the addresses of one state.
type siteServer struct {
	state  nodeapi.SiteState
	cert   string            // the certificate it serves TLS with, to notice a new one
	tls    *nodeapi.TLSFiles // the same, to listen again
	tried  time.Time         // the last attempt to listen
	files  map[string]siteFile
	notFnd siteFile
	hosts  map[string]bool // the names the site answers for: its certificate's; none, any
	srv    []*http.Server
	status nodeapi.SiteStatus
}

type siteFile struct {
	name string
	data []byte
	mod  time.Time
	etag string
}

// applySite brings the served site in line with the state, under e.mu. It reports whether
// the state names a site the node does not have; the one served so far stays until it does.
func (e *Engine) applySite(st nodeapi.DesiredState) (missing bool) {
	want := st.Site
	cert := ""
	if st.TLS != nil {
		cert = st.TLS.CertPEM
	}
	if want == nil || !nodeapi.SiteHash.MatchString(want.Hash) {
		e.stopSite()
		e.pruneSites("")
		return false
	}
	if e.site != nil && e.site.state == *want && e.site.cert == cert && e.site.status.Error == "" {
		return false
	}
	archive, err := os.ReadFile(e.sitePath(want.Hash))
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	next := &siteServer{state: *want, cert: cert, tls: st.TLS, hosts: certHosts(st.TLS), status: nodeapi.SiteStatus{Hash: want.Hash}}
	if err == nil {
		err = next.load(archive, e.siteTime(want.Hash))
	}
	if err != nil {
		// A kept archive that does not check any more is sent again.
		e.log.Error("site", "hash", want.Hash, "err", err)
		_ = os.Remove(e.sitePath(want.Hash))
		return true
	}
	e.stopSite()
	next.listen(st.TLS, e.log)
	e.site = next
	e.pruneSites(want.Hash)
	return false
}

// siteTime is when the node got the site: what Last-Modified and ETag say, as a file
// server's would say when the files were copied there.
func (e *Engine) siteTime(hash string) time.Time {
	if fi, err := os.Stat(e.sitePath(hash)); err == nil {
		return fi.ModTime().UTC().Truncate(time.Second)
	}
	return time.Now().UTC().Truncate(time.Second)
}

func (e *Engine) stopSite() {
	if e.site == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for _, s := range e.site.srv {
		_ = s.Shutdown(ctx)
	}
	e.site = nil
}

// pruneSites removes every kept archive but keep's.
func (e *Engine) pruneSites(keep string) {
	entries, _ := os.ReadDir(filepath.Join(e.dataDir, siteDir))
	for _, d := range entries {
		if d.Name() != keep+".zip" {
			_ = os.Remove(filepath.Join(e.dataDir, siteDir, d.Name()))
		}
	}
}

// sitePort is the node's website over TLS that REALITY may aim at, 0 when the state names
// no site.
func sitePort(st nodeapi.DesiredState) int {
	if st.Site == nil {
		return 0
	}
	return st.Site.HTTPSPort
}

func (e *Engine) siteStatus() *nodeapi.SiteStatus {
	if e.site == nil {
		return nil
	}
	s := e.site.status
	return &s
}

func (s *siteServer) load(archive []byte, mod time.Time) error {
	st, err := site.Unpack(archive)
	if err != nil {
		return err
	}
	if st.Hash != s.state.Hash {
		return errors.New("the kept archive hashes to " + st.Hash)
	}
	s.files = make(map[string]siteFile, len(st.Files))
	for p, data := range st.Files {
		// A file system keeps nanoseconds; an archive keeps seconds. A part of a second of
		// the file's own makes the ETag look like Caddy's on a disk.
		sum := sha256.Sum256([]byte(st.Hash + "/" + p))
		fmod := mod.Truncate(time.Second).Add(time.Duration(binary.BigEndian.Uint32(sum[:4]) % uint32(time.Second)))
		s.files[p] = siteFile{name: path.Base(p), data: data, mod: fmod, etag: caddyETag(fmod, len(data))}
	}
	s.notFnd = s.files["404.html"]
	return nil
}

// caddyETag is the ETag Caddy's file_server (2.11) gives a file: its modification time in
// nanoseconds and its size, in base 36, quoted.
func caddyETag(mod time.Time, size int) string {
	return `"` + strconv.FormatInt(mod.UnixNano(), 36) + "-" + strconv.FormatInt(int64(size), 36) + `"`
}

// certHosts are the DNS names of a certificate, which Caddy would have as its site address.
func certHosts(certs *nodeapi.TLSFiles) map[string]bool {
	if certs == nil {
		return nil
	}
	block, _ := pem.Decode([]byte(certs.CertPEM))
	if block == nil {
		return nil
	}
	c, err := x509.ParseCertificate(block.Bytes)
	if err != nil || len(c.DNSNames) == 0 {
		return nil
	}
	hosts := make(map[string]bool, len(c.DNSNames))
	for _, n := range c.DNSNames {
		hosts[strings.ToLower(n)] = true
	}
	return hosts
}

// ownHost: Caddy serves a site only for the names in its address; a request for another
// Host, or none (a bare HTTP/2 preface), meets no site at all.
func (s *siteServer) ownHost(host string) bool {
	if s.hosts == nil {
		return true
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return s.hosts[strings.ToLower(strings.TrimSuffix(host, "."))]
}

// ownName is one of the names the site answers for (the first in order), "" when it
// answers any.
func (s *siteServer) ownName() string {
	names := make([]string, 0, len(s.hosts))
	for h := range s.hosts {
		names = append(names, h)
	}
	slices.Sort(names)
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

// siteRetryEvery is how often a site that could not listen (a port another program held)
// tries again: the panel sends the same state only when it changes.
const siteRetryEvery = 15 * time.Second

// retrySite listens again, under e.mu, when the site could not and siteRetryEvery has
// passed; the node's health, which the panel reads every few seconds, calls it.
func (e *Engine) retrySite(now time.Time) {
	s := e.site
	if s == nil || s.status.Error == "" || now.Sub(s.tried) < siteRetryEvery {
		return
	}
	// The address that listens keeps serving; only the one that failed is tried again.
	s.status.Error = ""
	s.listen(s.tls, e.log)
	if s.status.Error == "" {
		e.log.Info("site listens again", "hash", s.state.Hash)
	}
}

func (s *siteServer) listen(certs *nodeapi.TLSFiles, log *slog.Logger) {
	s.tried = time.Now()
	var errs []string
	serve := func(port int, conf *tls.Config) string {
		addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			errs = append(errs, err.Error())
			return ""
		}
		var handler http.Handler = s
		if conf == nil {
			// The plain port is TrustTunnel's fallback alone, and Citrus binds the Host of
			// what it forwards to its target, this address (a stranger must not pick another
			// vhost). Here that Host is the site's own name; elsewhere it stays a stranger's,
			// which Caddy answers with an empty 200 (the site was blank behind TT, 24.10).
			handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Host == addr {
					r = r.Clone(r.Context())
					r.Host = s.ownName()
				}
				s.ServeHTTP(w, r)
			})
		}
		srv := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 2 * time.Minute, ErrorLog: slog.NewLogLogger(log.Handler(), slog.LevelDebug),
			// Caddy's request header limit, as TrustTunnel's fallback front has it: HTTP/2
			// announces it (MAX_HEADER_LIST_SIZE 16704), and Go's 1 MB would set REALITY's
			// own site apart from the TrustTunnel port on the same node.
			MaxHeaderBytes: siteMaxHeaderBytes}
		s.srv = append(s.srv, srv)
		if conf != nil {
			srv.TLSConfig = conf
			ln = tls.NewListener(ln, conf)
		}
		go func() { _ = srv.Serve(ln) }()
		return addr
	}
	if s.state.HTTPPort > 0 && s.status.HTTP == "" {
		s.status.HTTP = serve(s.state.HTTPPort, nil)
	}
	if s.state.HTTPSPort > 0 && s.status.HTTPS == "" && certs != nil && certs.CertPEM != "" {
		pair, err := tls.X509KeyPair([]byte(certs.CertPEM), []byte(certs.KeyPEM))
		if err != nil {
			errs = append(errs, "certificate: "+err.Error())
		} else {
			s.status.HTTPS = serve(s.state.HTTPSPort, &tls.Config{Certificates: []tls.Certificate{pair}, NextProtos: []string{"h2", "http/1.1"}, MinVersion: tls.VersionTLS12})
		}
	}
	s.status.Error = strings.Join(errs, "; ")
}

// ServeHTTP answers like Caddy 2.11's file_server with the site's own page for errors
// (handle_errors rewriting to /404.html): GET and HEAD, index.html for a directory (a
// directory asked for without its slash is redirected to it), conditional and range
// requests, everything else not allowed. Checked against Caddy on the nodes, 12.10.2026.
func (s *siteServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Server", "Caddy")
	if !s.ownHost(r.Host) {
		// No site for this name: Caddy answers an empty 200.
		h.Set("Content-Length", "0")
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method == http.MethodConnect {
		// CONNECT has no path; Caddy cleans it to "/" with a redirect.
		h.Set("Location", "/")
		h.Set("Content-Length", "0")
		w.WriteHeader(http.StatusPermanentRedirect)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		h.Set("Allow", "GET, HEAD")
		s.errorPage(w, r, http.StatusMethodNotAllowed)
		return
	}
	clean := path.Clean("/" + r.URL.Path)
	rel := strings.TrimPrefix(clean, "/")
	switch f, ok := s.files[rel]; {
	case rel == "":
		s.file(w, r, s.files["index.html"])
	case ok:
		s.file(w, r, f)
	default:
		index, ok := s.files[rel+"/index.html"]
		switch {
		case ok && strings.HasSuffix(r.URL.Path, "/"):
			s.file(w, r, index)
		case ok:
			http.Redirect(w, r, clean+"/", http.StatusPermanentRedirect)
		default:
			s.errorPage(w, r, http.StatusNotFound)
		}
	}
}

func (s *siteServer) file(w http.ResponseWriter, r *http.Request, f siteFile) {
	w.Header().Set("Etag", f.etag)
	w.Header().Set("Vary", "Accept-Encoding")
	http.ServeContent(w, r, f.name, f.mod, bytes.NewReader(f.data))
}

// errorPage is Caddy's handle_errors serving /404.html: the page with its file headers and
// the error's status. A 405 carries Vary twice, as Caddy's does (the method's error and the
// file server each add it).
func (s *siteServer) errorPage(w http.ResponseWriter, r *http.Request, code int) {
	f, h := s.notFnd, w.Header()
	h.Set("Accept-Ranges", "bytes")
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Etag", f.etag)
	h.Set("Last-Modified", f.mod.UTC().Format(http.TimeFormat))
	h.Set("Vary", "Accept-Encoding")
	if code == http.StatusMethodNotAllowed {
		h.Add("Vary", "Accept-Encoding")
	}
	h.Set("Content-Length", strconv.Itoa(len(f.data)))
	w.WriteHeader(code)
	if r.Method != http.MethodHead {
		_, _ = w.Write(f.data)
	}
}
