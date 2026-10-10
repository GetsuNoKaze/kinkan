package proto

import (
	"strings"
	"testing"
)

// REALITY may aim at the node's own website on loopback only when the node has one.
func TestRealityDestOwnSite(t *testing.T) {
	priv, _ := realityKey(t)
	tpl := func(dest string) Template {
		yaml := "type: vless\nreality-config:\n  dest: " + dest + "\n  private-key: " + priv + "\n  short-id: [a1b2]\n  server-names: [node.example]\n"
		parsed, err := Parse(yaml)
		if err != nil {
			t.Fatal(err)
		}
		return parsed
	}
	if err := Validate(tpl("127.0.0.1:17443"), Options{SitePort: 17443}); err != nil {
		t.Errorf("own site refused: %v", err)
	}
	for name, o := range map[string]Options{"no site": {}, "another port": {SitePort: 17444}, "panel only": {SelfStealPort: 21973}} {
		if err := Validate(tpl("127.0.0.1:17443"), o); err == nil || !strings.Contains(err.Error(), "reality_dest_private") {
			t.Errorf("%s: %v", name, err)
		}
	}
	if err := Validate(tpl("127.0.0.1:21973"), Options{SelfStealPort: 21973, SitePort: 17443}); err != nil {
		t.Errorf("the panel's self-steal refused next to a site: %v", err)
	}
}
