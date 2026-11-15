package subs

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
)

// GeoMirrorDir is the panel's mirror of the geodata Clash apps download (geoxURL): the app
// sets it to data/panel/www/geo/meta, which the panel's site serves as /geo/meta/. A cron
// job on the server fills it from MetaCubeX meta-rules-dat. From Russian networks GitHub's
// release downloads are slow or cut off, and an app without the files cannot start the
// profile (Kinkan, 15.11.2026). "" or an empty mirror: the profile keeps GitHub's links.
var GeoMirrorDir string

const geoxGitHub = "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/"

var hostName = regexp.MustCompile(`^[A-Za-z0-9.-]+(:[0-9]{1,5})?$`)

// geoMirror points a Clash profile's geodata at the panel the client fetched it from, when
// the mirror holds the files; host is the request's Host, the address the app reaches.
func geoMirror(body []byte, host string) []byte {
	if GeoMirrorDir == "" || !hostName.MatchString(host) {
		return body
	}
	for _, f := range []string{"geoip-lite.dat", "geosite.dat", "geoip.metadb", "GeoLite2-ASN.mmdb"} {
		if st, err := os.Stat(filepath.Join(GeoMirrorDir, f)); err != nil || st.Size() == 0 {
			return body
		}
	}
	return bytes.ReplaceAll(body, []byte(geoxGitHub), []byte("https://"+host+"/geo/meta/"))
}
