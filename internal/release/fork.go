package release

import "regexp"

// forkSuffix is the pre-release part of a Kinkan release such as 0.5.0.5-tt.3. The fork
// numbers its releases after the upstream version they are built on, and these are its
// stable releases: updaters on the stable channel take them and GitHub marks them latest.
// Anything more after it (0.5.0.5-tt.4-rc.1) is still a pre-release.
var forkSuffix = regexp.MustCompile(`^tt\.[0-9]+$`)

// forkStable reports whether a version's pre-release part names a Kinkan release.
func forkStable(pre string) bool { return forkSuffix.MatchString(pre) }

// forkPre splits a Kinkan pre-release part: 4 and "" for tt.4, 4 and "rc.1" for
// tt.4-rc.1; ok is false for anything else (a Mikan rc.1).
var forkPre = regexp.MustCompile(`^tt\.([0-9]+)(?:-(.+))?$`)

// compareForkPrerelease orders pre-release parts with Kinkan's in mind: a release
// (tt.4) comes after its own candidates (tt.4-rc.1), which plain pre-release order puts
// the other way round, since "4-rc" is not a number. Anything else is plain order.
func compareForkPrerelease(a, b string) int {
	ma, mb := forkPre.FindStringSubmatch(a), forkPre.FindStringSubmatch(b)
	if ma == nil || mb == nil {
		return comparePrerelease(a, b)
	}
	if c := comparePrerelease(ma[1], mb[1]); c != 0 {
		return c
	}
	switch {
	case ma[2] == mb[2]:
		return 0
	case ma[2] == "":
		return 1
	case mb[2] == "":
		return -1
	}
	return comparePrerelease(ma[2], mb[2])
}
