package nodeapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestScannerJournalUnsupportedErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"none", nil, false},
		{"transport", ErrUnavailable, false},
		{"explicit", ErrScannersUnsupported, true},
		{"missing", &StatusError{Method: "GET", Path: "/v1/scanners", Status: 404}, true},
		{"old reset", &StatusError{Method: "DELETE", Path: "/v1/scanners", Status: 405}, true},
		{"server error", &StatusError{Path: "/v1/scanners", Status: 500}, false},
		{"other endpoint", &StatusError{Path: "/v1/site", Status: 404}, false},
		{"no collector", &Error{Code: "invalid_state", Message: "no scanner journal"}, true},
		{"other state error", &Error{Code: "invalid_state", Message: "bad site"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsScannerUnsupported(tc.err); got != tc.want {
				t.Fatalf("unsupported=%v, want %v: %v", got, tc.want, tc.err)
			}
		})
	}
	old := httptest.NewServer(http.NotFoundHandler())
	defer old.Close()
	c := &Client{hc: old.Client(), base: old.URL}
	if _, err := c.Scanners(context.Background()); !IsScannerUnsupported(err) {
		t.Fatalf("old node collection: %v", err)
	}
	if err := c.ResetScanners(context.Background()); !IsScannerUnsupported(err) {
		t.Fatalf("old node clearing: %v", err)
	}
	old.Close()
	if err := c.ResetScanners(context.Background()); !errors.Is(err, ErrUnavailable) || IsScannerUnsupported(err) {
		t.Fatalf("offline node confused with an old node: %v", err)
	}
}
