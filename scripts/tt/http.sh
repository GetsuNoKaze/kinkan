#!/usr/bin/env bash
# Rebuilds third_party/metacubex-http: the github.com/metacubex/http version that
# third_party/mihomo requires, with patches/metacubex-http applied. go.mod replaces the
# module with this copy, so mihomo's HTTP servers (the TrustTunnel front among them)
# are built from it.
#
#   scripts/tt/http.sh           regenerate third_party/metacubex-http
#   scripts/tt/http.sh --check   fail if third_party/metacubex-http differs from a regeneration
#
# Run it after scripts/tt/mihomo.sh: a new mihomo may require another version.
set -euo pipefail

root="$(cd "$(dirname "$0")/../.." && pwd)"
module="github.com/metacubex/http"
dest="$root/third_party/metacubex-http"
check=false
[ "${1:-}" = "--check" ] && check=true

fail() {
	echo "::error::$*" >&2
	exit 1
}

version="$(awk -v m="$module" '$1 == m && $2 ~ /^v/ { print $2; exit }' "$root/third_party/mihomo/go.mod")"
[ -n "$version" ] || fail "no $module version in third_party/mihomo/go.mod"

# The module proxy's copy, verified against the checksum database like any download.
dir="$(cd "$root" && GOFLAGS=-mod=mod go mod download -json "$module@$version" | awk -F'"' '$2 == "Dir" { print $4 }')"
[ -n "$dir" ] || fail "cannot download $module $version"
dir="${dir//\\\\//}"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
src="$tmp/http"
mkdir -p "$src"
cp -R "$dir/." "$src/"
chmod -R u+w "$src"
git init --quiet "$src"
git -C "$src" config core.autocrlf false
for p in "$root"/patches/metacubex-http/*.patch; do
	git -C "$src" apply --check "$p" || fail "$(basename "$p") does not apply to $module $version"
	git -C "$src" apply "$p"
done
rm -rf "$src/.git"
echo "$version" >"$src/.http-version"

if $check; then
	diff -r -q "$src" "$dest" >/dev/null ||
		fail "third_party/metacubex-http is not $module $version with patches/metacubex-http; run scripts/tt/http.sh"
	echo "third_party/metacubex-http matches $module $version with patches/metacubex-http"
	exit 0
fi

rm -rf "$dest"
mkdir -p "$root/third_party"
mv "$src" "$dest"
(cd "$root" && go mod edit -replace "$module=./third_party/metacubex-http")
echo "third_party/metacubex-http: $module $version with patches/metacubex-http"
