package presets

import (
	"strings"
	"testing"
)

func TestKinkanQuietDefaults(t *testing.T) {
	for _, p := range All {
		if p.ID == "tuic_v5" && p.Default {
			t.Fatal("TUIC v5 is on by default; one probe tells such a node apart")
		}
		if p.ID == "tuic_v5" && !strings.Contains(p.Summary, "одной пробой") {
			t.Fatalf("TUIC v5 does not warn: %q", p.Summary)
		}
	}
	var n int
	for _, p := range All {
		if p.Default {
			n++
		}
	}
	if n == 0 {
		t.Fatal("no preset left on by default")
	}
}
