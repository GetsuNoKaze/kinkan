#!/usr/bin/env bash
# Run on a test node after enabling the fork's TrustTunnel inbound and HTTP cover.
# Requires curl with HTTP/2 support. No client password or panel token is needed.
set -euo pipefail
domain="${1:?usage: smoke.sh DOMAIN TRUSTTUNNEL_PORT [COVER_PORT]}"
port="${2:?usage: smoke.sh DOMAIN TRUSTTUNNEL_PORT [COVER_PORT]}"
cover="${3:-8080}"
[[ "$domain" =~ ^[A-Za-z0-9.-]+$ ]] || { echo 'Invalid domain' >&2; exit 1; }
for p in "$port" "$cover"; do
	[[ "$p" =~ ^[0-9]{1,5}$ ]] && (( 10#$p >= 1 && 10#$p <= 65535 )) ||
		{ echo 'Invalid port' >&2; exit 1; }
done
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
url="https://$domain:$port/"
origin="http://127.0.0.1:$cover/"
request() {
	local name="$1" target="$2"; shift 2
	curl --silent --show-error --noproxy '*' --connect-timeout 5 --max-time 15 \
		--max-filesize 2097152 -D "$tmp/$name.headers" -o "$tmp/$name.body" \
		-w '%{http_code} %{http_version}' "$@" "$target" >"$tmp/$name.status"
}
request cover "$origin" -H "Host: $domain:$port"
[[ "$(cut -d' ' -f1 "$tmp/cover.status")" == 200 ]] ||
	{ echo 'Local cover does not return 200' >&2; exit 1; }
for name in http1 http2 wrong-auth; do
	case "$name" in
		http1) options=(--http1.1) ;;
		http2) options=(--http2) ;;
		wrong-auth) options=(--http1.1 -H 'Proxy-Authorization: Basic aW52YWxpZDppbnZhbGlk') ;;
	esac
	request "$name" "$url" "${options[@]}"
	[[ "$(cut -d' ' -f1 "$tmp/$name.status")" == 200 ]] &&
		cmp -s "$tmp/cover.body" "$tmp/$name.body" ||
		{ echo "$name differs from the cover site" >&2; exit 1; }
done
[[ "$(cut -d' ' -f2 "$tmp/http2.status")" == 2 ]] ||
	{ echo 'HTTP/2 was not negotiated' >&2; exit 1; }
request cover-connect "$origin" --http1.1 --request CONNECT -H "Host: $domain:$port"
request connect "$url" --http1.1 --request CONNECT
cmp -s "$tmp/cover-connect.status" "$tmp/connect.status" &&
	cmp -s "$tmp/cover-connect.body" "$tmp/connect.body" ||
	{ echo 'CONNECT differs from the cover response' >&2; exit 1; }
for name in http1 http2 wrong-auth connect; do
	if grep -qi '^Proxy-Authenticate:' "$tmp/$name.headers" ||
		[[ "$(cut -d' ' -f1 "$tmp/$name.status")" == 407 ]]; then
		echo "$name exposes proxy authentication" >&2; exit 1
	fi
done
echo 'PASS: valid TLS certificate, HTTP/1.1, HTTP/2, unauthenticated GET, wrong credentials and CONNECT cover responses'
echo 'Also verify an authenticated client, subscription, restart and rollback before deployment.'
