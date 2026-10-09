#!/usr/bin/env bash
# Reports a check run in this repository's issues, and in Telegram when
# TELEGRAM_BOT_TOKEN and TELEGRAM_CHAT_ID are set.
#
#   scripts/tt/report.sh fail <what> <log file>   open an issue, or comment on the open one
#   scripts/tt/report.sh ok <what>                close the open issue, if any
#
# <what> names the upstream state that was tried, for example "mikan v0.5.1 (abc1234)".
#
# Needs gh with GH_TOKEN, and GITHUB_REPOSITORY, GITHUB_SERVER_URL and GITHUB_RUN_ID
# as GitHub Actions sets them.
set -euo pipefail

status="${1:?usage: report.sh fail|ok <what> [log file]}"
ref="${2:?what was tried}"
log="${3:-}"
label="sync-failure"
run_url="$GITHUB_SERVER_URL/$GITHUB_REPOSITORY/actions/runs/$GITHUB_RUN_ID"

telegram() {
	[ -n "${TELEGRAM_BOT_TOKEN:-}" ] && [ -n "${TELEGRAM_CHAT_ID:-}" ] || return 0
	curl -fsS -o /dev/null "https://api.telegram.org/bot$TELEGRAM_BOT_TOKEN/sendMessage" \
		--data-urlencode "chat_id=$TELEGRAM_CHAT_ID" \
		--data-urlencode "text=$1" ||
		echo "::warning::Telegram message not sent"
}

open_issue="$(gh issue list --label "$label" --state open --json number --jq '.[0].number // empty')"

case "$status" in
fail)
	reason="$(grep -m1 '::error::' "$log" | sed 's/^.*::error:://' || true)"
	[ -n "$reason" ] || reason="the check failed; see the log"
	body="$(
		printf 'Syncing with %s failed: %s\n\nRun: %s\n\nLast lines of the log:\n\n```\n' "$ref" "$reason" "$run_url"
		tail -n 60 "$log"
		printf '```\n'
	)"
	if [ -n "$open_issue" ]; then
		gh issue comment "$open_issue" --body "$body"
	else
		gh label create "$label" --color d73a4a --description "Merging upstream mikan fails to merge, build or pass tests" --force >/dev/null
		gh issue create --title "Sync fails: $ref" --label "$label" --body "$body"
	fi
	telegram "kinkan: sync with $ref failed. $reason $run_url"
	;;
ok)
	if [ -n "$open_issue" ]; then
		gh issue close "$open_issue" --comment "Sync works again with $ref: $run_url"
		telegram "kinkan: sync with $ref works again. $run_url"
	fi
	;;
*)
	echo "unknown status $status" >&2
	exit 2
	;;
esac
