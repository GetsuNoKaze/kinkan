package nodeapi

import (
	"context"
	"time"
)

// ScannerRecord contains rejected-auth metadata only, aggregated by day/source.
// An unauthenticated request may be a browser; it is not proof of a scanner.
type ScannerRecord struct {
	Day          int64  `json:"day"`
	IP           string `json:"ip"`
	Inbound      string `json:"inbound"`
	Protocol     string `json:"protocol"`
	Reason       string `json:"reason"`
	Method       string `json:"method,omitempty"`
	Client       bool   `json:"client"`
	Count        int64  `json:"count"`
	First        int64  `json:"first"`
	Last         int64  `json:"last"`
	ASN          string `json:"asn,omitempty"`
	Organization string `json:"organization,omitempty"`
	Country      string `json:"country,omitempty"`
	PTR          string `json:"ptr,omitempty"`
	Scanner      string `json:"scanner"`
	Evidence     string `json:"evidence,omitempty"`
	// Own is the panel's: the address is the panel's own, the record its own check.
	Own bool `json:"own,omitempty"`
}
type ScannerSnapshot struct {
	Epoch   string          `json:"epoch"`
	Rows    []ScannerRecord `json:"rows"`
	Dropped int64           `json:"dropped"`
	// Observer is the address the request for this snapshot came from, as the node saw
	// it: the panel's own outgoing address, which its checks of the node come from too
	// (behind Cloudflare or NAT it is not what the panel's domain resolves to).
	Observer string `json:"observer,omitempty"`
}

// ResetScanners empties the node's scanner journal under a new epoch.
func (c *Client) ResetScanners(ctx context.Context) error {
	return c.do(ctx, "DELETE", "/v1/scanners", nil, nil, 10*time.Second)
}

func (c *Client) Scanners(ctx context.Context) (ScannerSnapshot, error) {
	var out ScannerSnapshot
	err := c.do(ctx, "GET", "/v1/scanners", nil, &out, 10*time.Second)
	return out, err
}
