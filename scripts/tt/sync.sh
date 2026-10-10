#!/usr/bin/env bash
# Merges upstream mikan's main into the sync/upstream branch and regenerates
# third_party/mihomo (and third_party/metacubex-http, which mihomo's version decides)
# if the merge moved mikan to another mihomo version.
#
# Writes to $GITHUB_OUTPUT (when set):
#   up_to_date=true    upstream main is already merged, nothing to do
#   branch=<name>      the branch holding the merge
#   what=<text>        a name for the upstream state, e.g. "mikan v0.5.1 (abc1234)"
set -euo pipefail
cd "$(dirname "$0")/../.."

upstream="${MIKAN_REPO:-https://github.com/Miroshka000/mikan}"
out="${GITHUB_OUTPUT:-/dev/null}"

fail() {
	echo "::error::$*" >&2
	exit 1
}

# Upstream tags have their own namespace: fetching upstream must never replace a
# fork release tag with the same version number.
git fetch --quiet --no-tags "$upstream" "+refs/heads/main:refs/remotes/upstream/main" "+refs/tags/*:refs/tags/upstream/*" ||
	fail "cannot fetch upstream mikan"
up="$(git rev-parse upstream/main)"
short="$(git rev-parse --short=7 "$up")"
tag="$(git describe --tags --match 'upstream/*' --abbrev=0 "$up" 2>/dev/null || true)"
tag="${tag#upstream/}"
what="mikan ${tag:-main} ($short)"
echo "what=$what" >>"$out"

if git merge-base --is-ancestor "$up" HEAD; then
	echo "up_to_date=true" >>"$out"
	echo "already up to date with $what"
	exit 0
fi
echo "up_to_date=false" >>"$out"

# One branch for every sync: each run rebuilds it from main and force-pushes it, so the
# open pull request follows upstream instead of a new one opening per upstream commit.
branch="sync/upstream"
echo "branch=$branch" >>"$out"
git checkout --quiet -B "$branch"

if ! git merge --no-edit -m "Merge upstream $what" "$up"; then
	conflicts="$(git diff --name-only --diff-filter=U | tr '\n' ' ')"
	git merge --abort || true
	fail "merging $what conflicts in: $conflicts"
fi

before="$(cat third_party/mihomo/.mihomo-version 2>/dev/null || true)"
scripts/tt/mihomo.sh
scripts/tt/http.sh
if [ -n "$(git status --porcelain -- third_party go.mod go.sum)" ]; then
	after="$(cat third_party/mihomo/.mihomo-version)"
	http="$(cat third_party/metacubex-http/.http-version)"
	git add -A third_party go.mod go.sum
	git commit --quiet -m "third_party: mihomo $after (was ${before:-none}), metacubex/http $http, with their patches"
fi

echo "merged $what into $branch"
