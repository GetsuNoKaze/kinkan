package release

import "testing"

// Kinkan's releases (-tt.N) are its stable releases; anything more after them is beta.
func TestForkChannel(t *testing.T) {
	for v, want := range map[string]string{
		"0.5.0.5-tt.3":      Stable,
		"0.5.0.5-tt.12":     Stable,
		"0.5.0.5":           Stable,
		"0.5.0.5-tt.4-rc.1": Beta,
		"0.5.0.5-rc.1":      Beta,
		"0.5.0.5-tt":        Beta,
		"0.5.0.5-tt.x":      Beta,
	} {
		if got := ChannelOf(v); got != want {
			t.Errorf("ChannelOf(%q) = %s, want %s", v, got, want)
		}
	}
	e := Entry{Version: "0.5.0.5-tt.3", Channel: Stable, Manifest: "https://example.com/m.json", From: "0.5.0.4"}
	if !e.valid() {
		t.Error("a stable index entry for a Kinkan release is skipped")
	}
}

// A Kinkan release comes after its own candidates, and the candidates keep their order.
func TestForkReleaseAfterItsCandidates(t *testing.T) {
	for _, c := range []struct{ newer, older string }{
		{"0.5.0.5-tt.4", "0.5.0.5-tt.4-rc.1"},
		{"0.5.0.5-tt.4-rc.2", "0.5.0.5-tt.4-rc.1"},
		{"0.5.0.5-tt.5-rc.1", "0.5.0.5-tt.4"},
		{"0.5.0.5-tt.10", "0.5.0.5-tt.9"},
		{"0.5.0.5", "0.5.0.5-tt.9"},
		{"0.5.0.5-tt.1", "0.5.0.5-rc.4"},
	} {
		if !Newer(c.newer, c.older) || Newer(c.older, c.newer) {
			t.Errorf("%s should be newer than %s", c.newer, c.older)
		}
	}
}
