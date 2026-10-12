package api

import "testing"

func TestScannerSpikeNeedsHistory(t *testing.T) {
	const day = 86400
	today := int64(1760227200) // 12.10.2026 00:00 UTC
	cases := []struct {
		name                  string
		current, prior, since int64
		want                  bool
	}{
		{"never collected", 500, 0, 0, false},
		{"moved to Kinkan today", 500, 0, today + 3600, false},
		{"two days of history", 500, 0, today - 2*day, false},
		{"three quiet days, then a burst", 500, 30, today - 3*day, true},
		{"a week at the same level", 500, 7 * 400, today - 30*day, false},
		{"a week of little, then a burst", 500, 7 * 20, today - 30*day, true},
		{"under a hundred is never a spike", 90, 0, today - 30*day, false},
	}
	for _, c := range cases {
		if got := scannerSpike(c.current, c.prior, c.since, today); got != c.want {
			t.Errorf("%s: spike = %v", c.name, got)
		}
	}
}
