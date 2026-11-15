package subs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeoMirror(t *testing.T) {
	body := []byte("geox-url:\n  geosite: " + geoxGitHub + "geosite.dat\n")
	GeoMirrorDir = ""
	if got := string(geoMirror(body, "panel.example:21973")); got != string(body) {
		t.Fatalf("no mirror dir: changed to %q", got)
	}
	dir := t.TempDir()
	GeoMirrorDir = dir
	defer func() { GeoMirrorDir = "" }()
	if got := string(geoMirror(body, "panel.example:21973")); got != string(body) {
		t.Fatalf("empty mirror: changed to %q", got)
	}
	for _, f := range []string{"geoip-lite.dat", "geosite.dat", "geoip.metadb", "GeoLite2-ASN.mmdb"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got := string(geoMirror(body, "panel.example:21973"))
	if !strings.Contains(got, "https://panel.example:21973/geo/meta/geosite.dat") || strings.Contains(got, "github.com") {
		t.Fatalf("mirror: got %q", got)
	}
	if got := string(geoMirror(body, "evil.example/x?")); got != string(body) {
		t.Fatalf("odd host must not be used: %q", got)
	}
}
