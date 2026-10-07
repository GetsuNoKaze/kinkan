package proto

import (
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"testing"
)

func xhttpTemplate(t *testing.T, xhttp string) Template {
	t.Helper()
	priv, _ := realityKey(t)
	return mustParse(t, "type: vless\nxhttp-config: {path: /x"+xhttp+"}\n"+
		"reality-config:\n  dest: www.microsoft.com:443\n  private-key: "+priv+"\n  short-id: [a1b2]\n  server-names: [www.microsoft.com]\n")
}

// The form writes what it shows: every field, an empty one back to the core's default, and
// what it read from YAML (numbers too) comes back unchanged.
func TestSetXHTTPRoundTrip(t *testing.T) {
	tpl := xhttpTemplate(t, ", x-padding-bytes: 1000, sc-max-buffered-posts: 30")
	x, ok := XHTTPOf(tpl)
	if !ok || x.PaddingBytes != "1000" {
		t.Fatalf("read: %+v %v", x, ok)
	}
	x.Mode, x.PaddingObfs, x.PaddingPlacement, x.PaddingKey, x.PaddingMethod = "packet-up", true, "cookie", "sid", "tokenish"
	x.UplinkMethod, x.MaxEachPostBytes, x.MaxConnections, x.MinPostsIntervalMs = "PUT", "500000-1000000", "2-4", "10-50"
	if err := SetXHTTP(tpl, x); err != nil {
		t.Fatal(err)
	}
	if err := Validate(tpl, Options{AnyDest: true}); err != nil {
		t.Fatalf("written template: %v", err)
	}
	got, _ := XHTTPOf(tpl)
	if got != x {
		t.Fatalf("read back:\n%+v\nwant\n%+v", got, x)
	}
	if sec := tpl.section("xhttp-config"); sec["sc-max-buffered-posts"] != 30 || sec["path"] != "/x" {
		t.Fatalf("keys the form does not show stay: %v", sec)
	}

	if err := SetXHTTP(tpl, XHTTPTuning{}); err != nil {
		t.Fatal(err)
	}
	sec := tpl.section("xhttp-config")
	if _, has := sec["x-padding-bytes"]; has || sec["x-padding-obfs-mode"] != nil || tpl[extKey] != nil {
		t.Fatalf("defaults again: %v %v", sec, tpl[extKey])
	}
}

func TestXHTTPTuningCheck(t *testing.T) {
	for name, x := range map[string]XHTTPTuning{
		"auto mode":        {Mode: "auto"},
		"bad range":        {PaddingBytes: "1000-100"},
		"not a number":     {PaddingBytes: "lots"},
		"zero chunk":       {UplinkChunkSize: "0"},
		"bad method":       {UplinkMethod: "GET"},
		"bad placement":    {PaddingPlacement: "body"},
		"bad key":          {PaddingKey: "a b"},
		"streams and conn": {MaxConcurrency: "8", MaxConnections: "2"},
	} {
		var pe *Error
		if err := SetXHTTP(xhttpTemplate(t, ""), x); !errors.As(err, &pe) {
			t.Errorf("%s: %v", name, err)
		}
	}
	priv, _ := realityKey(t)
	ws := mustParse(t, "type: vless\nws-path: /ws\nreality-config:\n  dest: www.microsoft.com:443\n  private-key: "+priv+"\n  short-id: [a1b2]\n  server-names: [www.microsoft.com]\n")
	if err := SetXHTTP(ws, XHTTPTuning{}); err == nil {
		t.Fatal("not XHTTP")
	}
	// The same rules hold for YAML typed by hand.
	if err := Validate(xhttpTemplate(t, ", uplink-http-method: GET"), Options{AnyDest: true}); err == nil {
		t.Fatal("YAML with a bad method passed")
	}
}

// The client's own reuse settings replace the panel's, in mihomo's profile and in the
// Xray link's extra.
func TestXHTTPClientReuse(t *testing.T) {
	tpl := xhttpTemplate(t, "")
	plain, err := ClientConfig(tpl, ClientInput{Name: "X", Host: "203.0.113.7", Port: 443, Slot: slots[0]})
	if err != nil {
		t.Fatal(err)
	}
	if r := plain.Mihomo["xhttp-opts"].(map[string]any)["reuse-settings"].(map[string]any); r["max-concurrency"] != "16-32" {
		t.Fatalf("the panel's default: %v", r)
	}
	if err := SetXHTTP(tpl, XHTTPTuning{MaxConnections: "2-4", MaxReusableSecs: "600", MinPostsIntervalMs: "20"}); err != nil {
		t.Fatal(err)
	}
	c, err := ClientConfig(tpl, ClientInput{Name: "X", Host: "203.0.113.7", Port: 443, Slot: slots[0]})
	if err != nil {
		t.Fatal(err)
	}
	opts := c.Mihomo["xhttp-opts"].(map[string]any)
	r := opts["reuse-settings"].(map[string]any)
	if r["max-connections"] != "2-4" || r["h-max-reusable-secs"] != "600" || r["max-concurrency"] != nil || opts["sc-min-posts-interval-ms"] != "20" {
		t.Fatalf("mihomo: %v", opts)
	}
	u, _ := url.Parse(c.URI)
	var extra map[string]any
	if err := json.Unmarshal([]byte(u.Query().Get("extra")), &extra); err != nil {
		t.Fatalf("extra %q: %v", u.Query().Get("extra"), err)
	}
	xm, _ := extra["xmux"].(map[string]any)
	if xm["maxConnections"] != "2-4" || xm["hMaxReusableSecs"] != "600" || extra["scMinPostsIntervalMs"] != "20" || strings.Contains(c.URI, "maxConcurrency") {
		t.Fatalf("link extra: %v", extra)
	}
}
