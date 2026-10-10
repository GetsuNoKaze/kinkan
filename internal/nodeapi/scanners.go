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
}
type ScannerSnapshot struct {
	Epoch   string          `json:"epoch"`
	Rows    []ScannerRecord `json:"rows"`
	Dropped int64           `json:"dropped"`
}

func (c *Client) Scanners(ctx context.Context) (ScannerSnapshot, error) {
	var out ScannerSnapshot
	err := c.do(ctx, "GET", "/v1/scanners", nil, &out, 10*time.Second)
	return out, err
}
