#!/usr/bin/env bash
# Merges upstream mikan's main into the sync/upstream branch and regenerates
# third_party/mihomo (and third_party/metacubex-http, which mihomo's version decides)
# if the merge moved mikan to another mihomo version or source (Mikan's own core,
# Citrus: see scripts/tt/mihomo.sh), and the API schema (openapi.json and
# its TypeScript types) from the merged code: a conflict only in those is resolved by
# making them again, since both sides' endpoints are in the code.
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

# Where upstream takes mihomo from: its go.mod's replace for the module (Mikan's own core,
# Citrus, or a copy in its repository), or nothing for MetaCubeX's at the required version.
# scripts/tt/mihomo.sh builds third_party/mihomo from there; this fork's go.mod replaces
# the module with that copy either way.
upstream_mihomo="$(git show "$up:go.mod" | awk '
	{ sub(/^replace[ \t]+/, "") }
	$1 == "github.com/metacubex/mihomo" {
		for (i = 2; i <= NF; i++) if ($i == "=>") { print $(i + 1), (i + 2 <= NF ? $(i + 2) : ""); exit }
	}')"

generated=(web/src/api/openapi.json web/src/api/schema.d.ts)
# go.mod and go.sum are made again too: upstream's, with this fork's replaces put back by
# the scripts below and go mod tidy.
regenerated=("${generated[@]}" go.mod go.sum)
if ! git merge --no-edit -m "Merge upstream $what" "$up"; then
	conflicts="$(git diff --name-only --diff-filter=U)"
	rest="$(grep -vxF "${regenerated[@]/#/-e}" <<<"$conflicts" || true)"
	if [ -n "$rest" ]; then
		git merge --abort || true
		fail "merging $what conflicts in: $(tr '\n' ' ' <<<"$conflicts")"
	fi
	# Upstream's version for now; regenerated below from the merged code.
	git checkout --theirs -- $conflicts
	git add -- $conflicts
	git commit --quiet --no-edit
	echo "resolved by regeneration: $(tr '\n' ' ' <<<"$conflicts")"
fi

# The Go build embeds web/dist, which is not in git; a placeholder lets go mod tidy and
# the schema generator below build, as scripts/tt/check.sh does.
if [ ! -d web/dist ]; then
	mkdir -p web/dist
	echo '<!doctype html><title>sync build</title>' >web/dist/index.html
fi

if [ -n "$upstream_mihomo" ]; then
	echo "$upstream_mihomo" >patches/mihomo/UPSTREAM
	echo "upstream takes mihomo from: $upstream_mihomo"
else
	rm -f patches/mihomo/UPSTREAM
fi
before="$(cat third_party/mihomo/.mihomo-version 2>/dev/null || true)"
scripts/tt/mihomo.sh
scripts/tt/http.sh
go mod tidy
if [ -n "$(git status --porcelain -- third_party go.mod go.sum patches/mihomo/UPSTREAM)" ]; then
	after="$(cat third_party/mihomo/.mihomo-version)"
	http="$(cat third_party/metacubex-http/.http-version)"
	git add -A third_party go.mod go.sum patches/mihomo
	git commit --quiet -m "third_party: mihomo $after (was ${before:-none}), metacubex/http $http, with their patches"
fi

go run ./cmd/mikan openapi >web/src/api/openapi.json.new
mv web/src/api/openapi.json.new web/src/api/openapi.json
(cd web && pnpm install --frozen-lockfile --silent && pnpm run --silent api)
if [ -n "$(git status --porcelain -- "${generated[@]}")" ]; then
	git add -- "${generated[@]}"
	git commit --quiet -m "web: the API schema made again from the merged code"
fi

echo "merged $what into $branch"
