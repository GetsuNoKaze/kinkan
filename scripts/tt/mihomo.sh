#!/usr/bin/env bash
# Rebuilds third_party/mihomo: the mihomo mikan builds with, with patches/mihomo applied.
# go.mod replaces github.com/metacubex/mihomo with this copy.
#
# Where that mihomo comes from is in patches/mihomo/UPSTREAM, which scripts/tt/sync.sh
# writes from upstream's go.mod: a module and version ("github.com/getmikan/Citrus
# v0.0.0-...", for Mikan's own core), or a directory of the repository ("./citrus") when
# upstream keeps a copy. Without the file it is MetaCubeX's mihomo at the version go.mod
# requires.
#
#   scripts/tt/mihomo.sh           regenerate third_party/mihomo
#   scripts/tt/mihomo.sh --check   fail if third_party/mihomo differs from a regeneration
#
# Run it after go.mod moves to a new mihomo version (for example after merging upstream).
set -euo pipefail

root="$(cd "$(dirname "$0")/../.." && pwd)"
repo="${MIHOMO_REPO:-https://github.com/MetaCubeX/mihomo}"
check=false
[ "${1:-}" = "--check" ] && check=true

fail() {
	echo "::error::$*" >&2
	exit 1
}

# The required version, not the replacement: "github.com/metacubex/mihomo vX" in require.
version="$(awk '$1 == "github.com/metacubex/mihomo" && $2 ~ /^v/ { print $2; exit }' "$root/go.mod")"
[ -n "$version" ] || fail "no github.com/metacubex/mihomo version in go.mod"
dir=""
if [ -f "$root/patches/mihomo/UPSTREAM" ]; then
	read -r where at <"$root/patches/mihomo/UPSTREAM" || true
	case "$where" in
	./* | ../*) dir="$where" ;;
	*/*)
		repo="https://$where"
		version="${at:?patches/mihomo/UPSTREAM: no version after $where}"
		;;
	*) fail "patches/mihomo/UPSTREAM: cannot read \"$where $at\"" ;;
	esac
fi
case "$version" in
*-*-*) rev="${version##*-}" ;; # pseudo-version: fetch the commit
*) rev="refs/tags/$version" ;;
esac

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
src="$tmp/mihomo"
if [ -n "$dir" ]; then
	# Upstream's own copy in the repository: taken as it is in the merged tree.
	[ -d "$root/$dir" ] || fail "patches/mihomo/UPSTREAM names $dir, which is not in the repository"
	mkdir -p "$src"
	cp -R "$root/$dir/." "$src/"
	rm -rf "$src/.git"
	version="$dir"
	git init --quiet "$src"
	git -C "$src" config core.autocrlf false
elif [ "${rev#refs/}" = "$rev" ]; then
	# A pseudo-version names a commit by 12 characters, and a server hands out commits by
	# their full name only: clone the history without files (commits only), find the
	# commit, and take the files of that one. Byte for byte, whatever core.autocrlf says.
	git -c core.autocrlf=false -c core.eol=lf clone --quiet --filter=tree:0 --no-checkout "$repo" "$src" ||
		fail "cannot fetch mihomo $version from $repo"
	full="$(git -C "$src" rev-parse --verify --quiet "$rev^{commit}")" || fail "no commit $rev in $repo for mihomo $version"
	git -C "$src" -c core.autocrlf=false -c core.eol=lf checkout --quiet "$full"
else
	git init --quiet "$src"
	# Byte for byte as upstream has it, whatever core.autocrlf says on this machine.
	git -C "$src" config core.autocrlf false
	git -C "$src" config core.eol lf
	git -C "$src" fetch --quiet --depth 1 "$repo" "$rev" || fail "cannot fetch mihomo $version from $repo"
	git -C "$src" checkout --quiet FETCH_HEAD
fi
for p in "$root"/patches/mihomo/*.patch; do
	git -C "$src" apply --check "$p" || fail "$(basename "$p") does not apply to mihomo $version"
	git -C "$src" apply "$p"
done
rm -rf "$src/.git"
echo "$version" >"$src/.mihomo-version"

if $check; then
	diff -r -q "$src" "$root/third_party/mihomo" >/dev/null ||
		fail "third_party/mihomo is not mihomo $version with patches/mihomo; run scripts/tt/mihomo.sh"
	echo "third_party/mihomo matches mihomo $version with patches/mihomo"
	exit 0
fi

rm -rf "$root/third_party/mihomo"
mkdir -p "$root/third_party"
mv "$src" "$root/third_party/mihomo"
(cd "$root" && go mod edit -replace github.com/metacubex/mihomo=./third_party/mihomo)
echo "third_party/mihomo: mihomo $version with patches/mihomo"
