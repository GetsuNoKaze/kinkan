#!/usr/bin/env bash
# Rebuilds third_party/mihomo: the mihomo version mikan's go.mod requires, with
# patches/mihomo applied. go.mod replaces github.com/metacubex/mihomo with this copy.
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
case "$version" in
*-*-*) rev="${version##*-}" ;; # pseudo-version: fetch the commit
*) rev="refs/tags/$version" ;;
esac

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
src="$tmp/mihomo"
git init --quiet "$src"
# Byte for byte as upstream has it, whatever core.autocrlf says on this machine.
git -C "$src" config core.autocrlf false
git -C "$src" config core.eol lf
git -C "$src" fetch --quiet --depth 1 "$repo" "$rev" || fail "cannot fetch mihomo $version"
git -C "$src" checkout --quiet FETCH_HEAD
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
