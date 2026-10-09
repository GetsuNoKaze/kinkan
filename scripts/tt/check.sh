#!/usr/bin/env bash
# The checks for this fork's changes: third_party/mihomo matches its patches, mikan
# builds with it, go vet passes, and the tests that cover the changes pass.
# mikan's own full suite (it needs PostgreSQL) runs in .github/workflows/ci.yml.
set -euo pipefail
cd "$(dirname "$0")/../.."

fail() {
	echo "::error::$*" >&2
	exit 1
}
step() { echo "==> $*"; }

step "third_party/mihomo"
scripts/tt/mihomo.sh --check

# The panel embeds the web bundle; these checks do not need the real one.
if [ ! -d web/dist ]; then
	mkdir -p web/dist
	echo '<!doctype html><title>check build</title>' >web/dist/index.html
fi

step "build"
CGO_ENABLED=0 go build -o "${TMPDIR:-/tmp}/kinkan-bin/" ./cmd/mikan ./cmd/mikan-node ||
	fail "mikan does not build with third_party/mihomo"

step "vet"
go vet ./... || fail "go vet fails"

step "test mikan"
go run ./cmd/mikan-release fork-check || fail "release repository differs"
go test ./internal/proto/... ./internal/release/... ./cmd/mikan-release/... ||
	fail "protocol or release tests fail"
go test -run TestTrustTunnelFallbackOnNode ./internal/node ||
	fail "embedded TrustTunnel fallback test fails"

step "test mihomo"
(cd third_party/mihomo && go test ./transport/trusttunnel/ && go test -run TrustTunnel ./listener/inbound/) ||
	fail "mihomo TrustTunnel tests fail"

echo "checks passed"
