package site

import (
	"archive/zip"
	"bytes"
	"errors"
	"io/fs"
	"strconv"
	"strings"
	"testing"
)

type entry struct {
	name string
	data string
	mode fs.FileMode
}

func archive(t *testing.T, entries ...entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, e := range entries {
		h := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		if e.mode != 0 {
			h.SetMode(e.mode)
		}
		f, err := w.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(e.data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func minimal(prefix string) []entry {
	return []entry{
		{name: prefix + "index.html", data: "<html><head><title> Night\n Archive &amp; Co </title></head></html>"},
		{name: prefix + "404.html", data: "not here"},
		{name: prefix + "robots.txt", data: "User-agent: *\n"},
		{name: prefix + "favicon.svg", data: "<svg/>"},
		{name: prefix + "css/style.css", data: "body{}"},
	}
}

func code(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code + ":" + e.Path
	}
	if err != nil {
		return err.Error()
	}
	return ""
}

func TestUnpackMinimal(t *testing.T) {
	s, err := Unpack(archive(t, minimal("")...))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(s.Paths(), " "); got != "404.html css/style.css favicon.svg index.html robots.txt" {
		t.Errorf("paths = %s", got)
	}
	if s.Title != "Night Archive & Co" {
		t.Errorf("title = %q", s.Title)
	}
	if len(s.Hash) != 64 {
		t.Errorf("hash = %q", s.Hash)
	}
}

// A zip of one folder, with an archiver's junk and ./ paths, is the same site.
func TestUnpackFolderZip(t *testing.T) {
	plain, err := Unpack(archive(t, minimal("")...))
	if err != nil {
		t.Fatal(err)
	}
	entries := append(minimal("my-site/"), entry{name: "__MACOSX/my-site/._index.html", data: "x"}, entry{name: "my-site/.DS_Store", data: "x"})
	wrapped, err := Unpack(archive(t, entries...))
	if err != nil {
		t.Fatal(err)
	}
	dotted, err := Unpack(archive(t, minimal("./")...))
	if err != nil {
		t.Fatal(err)
	}
	if wrapped.Hash != plain.Hash || dotted.Hash != plain.Hash {
		t.Errorf("hashes differ: %s %s %s", plain.Hash, wrapped.Hash, dotted.Hash)
	}
}

func TestUnpackRefuses(t *testing.T) {
	with := func(extra ...entry) []entry { return append(minimal(""), extra...) }
	without := func(name string) []entry {
		var out []entry
		for _, e := range minimal("") {
			if e.name != name {
				out = append(out, e)
			}
		}
		return out
	}
	cases := map[string]struct {
		entries []entry
		want    string
	}{
		"parent directory": {with(entry{name: "../evil.html", data: "x"}), "site_path:../evil.html"},
		"nested parent":    {with(entry{name: "a/../../evil.html", data: "x"}), "site_path:a/../../evil.html"},
		"absolute":         {with(entry{name: "/etc/passwd.txt", data: "x"}), "site_path:/etc/passwd.txt"},
		"backslash":        {with(entry{name: "a\\..\\evil.html", data: "x"}), "site_path:a\\..\\evil.html"},
		"hidden file":      {with(entry{name: ".htaccess", data: "x"}), "site_path:.htaccess"},
		"php":              {with(entry{name: "shell.php", data: "<?php"}), "site_type:shell.php"},
		"symlink":          {with(entry{name: "link.html", data: "/etc/passwd", mode: fs.ModeSymlink | 0o777}), "site_link:link.html"},
		"no index":         {without("index.html"), "site_missing:index.html"},
		"no 404":           {without("404.html"), "site_missing:404.html"},
		"no robots":        {without("robots.txt"), "site_missing:robots.txt"},
		"no favicon":       {without("favicon.svg"), "site_missing:favicon"},
		"twice":            {with(entry{name: "index.html", data: "again"}), "site_path:index.html"},
		"nothing but junk": {[]entry{{name: ".DS_Store", data: "x"}}, "site_empty:"},
	}
	for name, c := range cases {
		_, err := Unpack(archive(t, c.entries...))
		if got := code(err); got != c.want {
			t.Errorf("%s: got %q, want %q", name, got, c.want)
		}
	}
	if _, err := Unpack([]byte("not a zip")); code(err) != "site_zip:" {
		t.Errorf("not a zip: %v", err)
	}
}

// The favicon may be anywhere when index.html links it; a link to elsewhere does not count.
func TestUnpackLinkedFavicon(t *testing.T) {
	entries := []entry{
		{name: "index.html", data: `<link rel="shortcut icon" href="/img/icon.png?v=2">`},
		{name: "404.html", data: "x"}, {name: "robots.txt", data: "x"}, {name: "img/icon.png", data: "png"},
	}
	if _, err := Unpack(archive(t, entries...)); err != nil {
		t.Errorf("linked favicon: %v", err)
	}
	entries[0].data = `<link rel="icon" href="https://cdn.example.org/icon.png">`
	if _, err := Unpack(archive(t, entries...)); code(err) != "site_missing:favicon" {
		t.Errorf("external favicon accepted: %v", err)
	}
}

// Limits hold whatever sizes the zip's directory claims.
func TestUnpackLimits(t *testing.T) {
	big := strings.Repeat("a", MaxFile+1)
	if _, err := Unpack(archive(t, append(minimal(""), entry{name: "big.txt", data: big})...)); code(err) != "site_too_big:big.txt" {
		t.Errorf("big file: %v", err)
	}
	var many []entry
	for i := 0; i <= MaxFiles; i++ {
		many = append(many, entry{name: "p/" + strings.Repeat("x", 3) + string(rune('a'+i%26)) + "-" + strconv.Itoa(i) + ".txt"})
	}
	if _, err := Unpack(archive(t, many...)); code(err) != "site_too_many_files:" {
		t.Errorf("many files: %v", err)
	}
	if _, err := Unpack(make([]byte, MaxArchive+1)); code(err) != "site_too_big:" {
		t.Errorf("big archive: %v", err)
	}
}
