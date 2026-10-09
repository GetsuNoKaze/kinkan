package release

import "regexp"

// forkSuffix is the pre-release part of a Kinkan release such as 0.5.0.5-tt.3. The fork
// numbers its releases after the upstream version they are built on, and these are its
// stable releases: updaters on the stable channel take them and GitHub marks them latest.
// Anything more after it (0.5.0.5-tt.4-rc.1) is still a pre-release.
var forkSuffix = regexp.MustCompile(`^tt\.[0-9]+$`)

// forkStable reports whether a version's pre-release part names a Kinkan release.
func forkStable(pre string) bool { return forkSuffix.MatchString(pre) }
