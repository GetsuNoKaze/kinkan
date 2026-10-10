// Package site checks and unpacks a node's site: the static website a Kinkan node shows to
// anyone who is not a client — TrustTunnel's fallback, REALITY's own target, a browser
// opening the node's address. The admin uploads it as a zip; this package decides whether
// it is one a node may serve. It runs nothing and writes nothing.
package site

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"html"
	"io"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// Limits on an uploaded site. A cover site is a few pages and pictures; anything larger
// is more likely a mistake (a video, a whole project folder) than a website.
const (
	MaxArchive  = 16 << 20 // the zip itself
	MaxUnpacked = 64 << 20 // all files together
	MaxFile     = 16 << 20 // one file
	MaxFiles    = 2000
)

// Required are the files every site needs at its root, besides a favicon (see favicon):
// a front page, a not-found page of its own (a server's built-in one would differ from
// the site's look) and robots.txt, which crawlers and scanners ask for first.
var Required = []string{"index.html", "404.html", "robots.txt"}

// Error says what is wrong with an upload: Code for the panel's translations, Path the
// file it is about, if any.
type Error struct {
	Code   string
	Path   string
	Detail string
}

func (e *Error) Error() string {
	s := e.Code
	if e.Path != "" {
		s += " (" + e.Path + ")"
	}
	if e.Detail != "" {
		s += ": " + e.Detail
	}
	return s
}

func fail(code, p, detail string) error { return &Error{Code: code, Path: p, Detail: detail} }

// Site is a checked upload.
type Site struct {
	// Files by path relative to the site's root, with forward slashes.
	Files map[string][]byte
	// Hash names the site's content: the same files give the same hash, whatever the zip
	// they came in looked like.
	Hash string
	// Title is the front page's <title>, to tell sites apart and to notice one site on
	// several nodes.
	Title string
	Size  int64
}

// Paths are the site's files in order.
func (s *Site) Paths() []string {
	out := make([]string, 0, len(s.Files))
	for p := range s.Files {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// static are the file types a static site is made of. Anything that would need a server
// to run it (php, cgi, an .htaccess) is refused rather than shown as text.
var static = map[string]bool{
	".html": true, ".htm": true, ".css": true, ".js": true, ".mjs": true, ".json": true,
	".map": true, ".txt": true, ".xml": true, ".webmanifest": true, ".csv": true, ".md": true,
	".svg": true, ".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true,
	".avif": true, ".ico": true, ".bmp": true,
	".woff": true, ".woff2": true, ".ttf": true, ".otf": true, ".eot": true,
	".mp3": true, ".ogg": true, ".wav": true, ".mp4": true, ".webm": true,
	".pdf": true,
}

// junk is what archivers and file managers leave in a folder; it is dropped, not refused.
func junk(p string) bool {
	base := path.Base(p)
	return strings.HasPrefix(p, "__MACOSX/") || base == ".DS_Store" || base == "Thumbs.db" || base == "desktop.ini"
}

// Unpack checks a zip and returns the site in it. A zip of one folder (what "compress"
// in a file manager makes) is taken as that folder's content.
func Unpack(archive []byte) (*Site, error) {
	if len(archive) > MaxArchive {
		return nil, fail("site_too_big", "", fmt.Sprintf("archive is %d bytes, at most %d", len(archive), MaxArchive))
	}
	r, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, fail("site_zip", "", err.Error())
	}
	files := map[string][]byte{}
	var total int64
	for _, f := range r.File {
		name := strings.TrimPrefix(f.Name, "./")
		if junk(name) || f.FileInfo().IsDir() {
			continue
		}
		p, ok := clean(name)
		if !ok {
			return nil, fail("site_path", name, "")
		}
		if f.Mode().Type() != 0 { // a symlink or another non-regular file
			return nil, fail("site_link", p, "")
		}
		if !static[strings.ToLower(path.Ext(p))] {
			return nil, fail("site_type", p, "")
		}
		if len(files) >= MaxFiles {
			return nil, fail("site_too_many_files", "", fmt.Sprintf("at most %d", MaxFiles))
		}
		if _, dup := files[p]; dup {
			return nil, fail("site_path", p, "the archive has it twice")
		}
		// The sizes in a zip's directory are the archiver's word: read no more than the
		// limits allow, whatever they say (a zip bomb says little and holds much).
		rc, err := f.Open()
		if err != nil {
			return nil, fail("site_zip", p, err.Error())
		}
		data, err := io.ReadAll(io.LimitReader(rc, MaxFile+1))
		rc.Close()
		if err != nil {
			return nil, fail("site_zip", p, err.Error())
		}
		if len(data) > MaxFile {
			return nil, fail("site_too_big", p, fmt.Sprintf("a file is at most %d bytes", MaxFile))
		}
		if total += int64(len(data)); total > MaxUnpacked {
			return nil, fail("site_too_big", "", fmt.Sprintf("all files together are at most %d bytes", MaxUnpacked))
		}
		files[p] = data
	}
	if len(files) == 0 {
		return nil, fail("site_empty", "", "")
	}
	files = unwrap(files)
	for _, req := range Required {
		if _, ok := files[req]; !ok {
			return nil, fail("site_missing", req, "")
		}
	}
	if !favicon(files) {
		return nil, fail("site_missing", "favicon", "favicon.ico, favicon.svg or favicon.png at the root, or a <link rel=\"icon\"> in index.html to a file of the site")
	}
	s := &Site{Files: files, Size: total, Title: title(files["index.html"])}
	s.Hash = hash(files)
	return s, nil
}

// clean turns a path in a zip into one relative to the site's root, or refuses it: an
// absolute path, a parent directory, a backslash (a Windows separator would slip past the
// checks on a Linux server), a hidden file or one that is not UTF-8.
func clean(name string) (string, bool) {
	if name == "" || !utf8.ValidString(name) || strings.ContainsAny(name, "\\\x00:") || strings.HasPrefix(name, "/") {
		return "", false
	}
	for _, part := range strings.Split(strings.TrimSuffix(name, "/"), "/") {
		if part == "" || part == "." || part == ".." || strings.HasPrefix(part, ".") {
			return "", false
		}
	}
	p := path.Clean(name)
	return p, p != "." && !strings.HasPrefix(p, "../")
}

// unwrap takes a site zipped as one folder out of that folder.
func unwrap(files map[string][]byte) map[string][]byte {
	var top string
	for p := range files {
		dir, _, found := strings.Cut(p, "/")
		if !found || (top != "" && dir != top) {
			return files
		}
		top = dir
	}
	out := make(map[string][]byte, len(files))
	for p, data := range files {
		out[strings.TrimPrefix(p, top+"/")] = data
	}
	return out
}

var iconLink = regexp.MustCompile(`(?is)<link\b[^>]*\brel\s*=\s*["']?(?:shortcut\s+)?icon["']?[^>]*>`)
var hrefAttr = regexp.MustCompile(`(?is)\bhref\s*=\s*["']([^"']+)["']`)

// favicon reports whether the site has an icon a browser finds: one of the usual names at
// the root, or a file of the site that index.html links as its icon.
func favicon(files map[string][]byte) bool {
	for _, name := range []string{"favicon.ico", "favicon.svg", "favicon.png"} {
		if _, ok := files[name]; ok {
			return true
		}
	}
	for _, link := range iconLink.FindAll(files["index.html"], -1) {
		m := hrefAttr.FindSubmatch(link)
		if m == nil {
			continue
		}
		href := strings.SplitN(html.UnescapeString(string(m[1])), "?", 2)[0]
		if strings.Contains(href, "://") || strings.HasPrefix(href, "//") || strings.HasPrefix(href, "data:") {
			continue
		}
		if _, ok := files[strings.TrimPrefix(path.Clean("/"+href), "/")]; ok {
			return true
		}
	}
	return false
}

var titleTag = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)

func title(page []byte) string {
	m := titleTag.FindSubmatch(page)
	if m == nil {
		return ""
	}
	t := strings.Join(strings.Fields(html.UnescapeString(string(m[1]))), " ")
	if len(t) > 200 {
		t = t[:200]
	}
	return t
}

// hash is over the paths and contents in order, each length-prefixed, so no two different
// sites hash alike by moving bytes between a name and a file.
func hash(files map[string][]byte) string {
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	h := sha256.New()
	var n [8]byte
	for _, p := range paths {
		binary.BigEndian.PutUint64(n[:], uint64(len(p)))
		h.Write(n[:])
		h.Write([]byte(p))
		binary.BigEndian.PutUint64(n[:], uint64(len(files[p])))
		h.Write(n[:])
		h.Write(files[p])
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Archive packs the site again from its checked files: sorted, with fixed times, so the same
// site always gives the same bytes. The panel keeps and sends this one, not the upload, and
// a node checks it with Unpack all the same.
func (s *Site) Archive() ([]byte, error) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, p := range s.Paths() {
		f, err := w.CreateHeader(&zip.FileHeader{Name: p, Method: zip.Deflate, Modified: archiveTime})
		if err != nil {
			return nil, err
		}
		if _, err := f.Write(s.Files[p]); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// archiveTime is the time every file of a packed site carries (zip's earliest).
var archiveTime = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)
