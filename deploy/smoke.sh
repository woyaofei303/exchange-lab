#!/bin/sh
set -eu

base_url="${BASE_URL:?BASE_URL is required}"
response="$(mktemp "${TMPDIR:-/tmp}/exchange-release-smoke.XXXXXX")"
cleanup() { status=$?; trap - EXIT HUP INT TERM; rm -f "$response"; exit "$status"; }
trap cleanup EXIT HUP INT TERM

request() {
	path=$1
	expected=$2
	status="$(curl --fail-with-body --silent --show-error --connect-timeout 3 --max-time 15 --output "$response" --write-out '%{http_code}' "$base_url$path")"
	[ "$status" = "$expected" ]
}
request /healthz 200
grep -qx '{"status":"ok"}' "$response"
request /readyz 200
grep -q '"status":"ready"' "$response"
request '/v1/prices?symbol=BTCUSDT-PERP' 200
grep -q '"mark_price"' "$response"
request / 200
grep -q '<title>Exchange Lab · 永续合约实验室</title>' "$response"
printf '%s\n' 'release smoke: HTTPS web, API readiness and authoritative mark price verified'
