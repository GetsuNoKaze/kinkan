package nodeapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"
)

// Kinkan: the website a node shows (ROADMAP item 2). The panel keeps sites; the desired
// state names one by the hash of its content, and the node, when it does not have it yet,
// says so in ApplyResult.SiteMissing and gets it by PutSite.

// SiteState is the site a node shows and where it serves it. Both addresses are on
// 127.0.0.1: the plain one is TrustTunnel's fallback, the TLS one REALITY's own target.
type SiteState struct {
	Hash      string `json:"hash"`
	HTTPPort  int    `json:"http_port"`
	HTTPSPort int    `json:"https_port,omitempty"` // 0: no TLS (the node has no certificate)
}

// SiteStatus is what the node serves: the site's hash, the addresses it listens on and
// why one is missing.
type SiteStatus struct {
	Hash  string `json:"hash"`
	HTTP  string `json:"http,omitempty"`
	HTTPS string `json:"https,omitempty"`
	Error string `json:"error,omitempty"`
}

// SiteHash is the form of a site's hash (internal/site): lower-case sha256 hex.
var SiteHash = regexp.MustCompile(`^[0-9a-f]{64}$`)

// PutSite gives the node a site's archive (internal/site Archive) under its hash; the node
// checks the archive and that it hashes so. A node without sites answers 404
// (StatusError).
func (c *Client) PutSite(ctx context.Context, hash string, archive []byte) error {
	if !SiteHash.MatchString(hash) {
		return fmt.Errorf("site hash %q", hash)
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	path := "/v1/site/" + hash
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.base+path, bytes.NewReader(archive))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/zip")
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var e Error
		if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&e) == nil && e.Code != "" {
			return &e
		}
		return &StatusError{Method: http.MethodPut, Path: path, Status: resp.StatusCode}
	}
	return nil
}
