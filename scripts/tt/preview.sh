#!/usr/bin/env bash
# Tries a merge of upstream mikan's dev (its betas) into this fork's main, without
# committing, and prints the files that would conflict. The API schema (openapi.json,
# schema.d.ts) is left out: scripts/tt/sync.sh makes it again after a merge.
#
# Writes to $GITHUB_OUTPUT (when set):
#   what=<text>        the upstream state tried, e.g. "mikan v0.5.1.0-rc.4 (abc1234)"
#   conflicts=<list>   the conflicting files, space separated; empty when it merges
set -euo pipefail
cd "$(dirname "$0")/../.."

upstream="${MIKAN_REPO:-https://github.com/Miroshka000/mikan}"
out="${GITHUB_OUTPUT:-/dev/null}"

git fetch --quiet --no-tags "$upstream" "+refs/heads/dev:refs/remotes/upstream/dev" "+refs/tags/*:refs/tags/upstream/*"
up="$(git rev-parse upstream/dev)"
tag="$(git describe --tags --match 'upstream/*' --abbrev=0 "$up" 2>/dev/null || true)"
what="mikan ${tag#upstream/} ($(git rev-parse --short=7 "$up"))"
echo "what=$what" >>"$out"

tree="$(mktemp -d)"
trap 'git worktree remove --force "$tree" >/dev/null 2>&1 || true; git worktree prune' EXIT
git worktree add --quiet --detach "$tree" HEAD
conflicts=""
if ! git -C "$tree" -c user.name=preview -c user.email=preview@localhost merge --no-commit --no-ff --quiet "$up" >/dev/null 2>&1; then
	conflicts="$(git -C "$tree" diff --name-only --diff-filter=U | grep -vxF -e web/src/api/openapi.json -e web/src/api/schema.d.ts | tr '\n' ' ' | sed 's/ $//' || true)"
fi
echo "conflicts=$conflicts" >>"$out"
if [ -n "$conflicts" ]; then
	echo "merging $what would conflict in: $conflicts"
else
	echo "$what merges into main without conflicts (the API schema aside)"
fi
