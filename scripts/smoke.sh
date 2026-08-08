#!/bin/sh
set -eu

base_url="${BASE_URL:-http://127.0.0.1:8080}"
curl_connect_timeout="${CURL_CONNECT_TIMEOUT:-1}"
curl_max_time="${CURL_MAX_TIME:-2}"
health_body='{"status":"ok"}'
ready_body='{"status":"ready","checks":{"nats":"ok","postgres":"ok","redis":"ok"}}'
response_file="$(mktemp "${TMPDIR:-/tmp}/exchange-smoke.XXXXXX")"

cleanup() {
	status=$?
	trap - EXIT HUP INT TERM
	rm -f "$response_file"
	exit "$status"
}

trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

request_exact() {
	path=$1
	expected_status=$2
	expected_body=$3
	if actual_status="$(curl --connect-timeout "$curl_connect_timeout" --max-time "$curl_max_time" \
		--silent --output "$response_file" --write-out '%{http_code}' "$base_url$path")"; then
		actual_body="$(cat "$response_file")"
		[ "$actual_status" = "$expected_status" ] && [ "$actual_body" = "$expected_body" ]
		return
	fi
	return 1
}

if ! request_exact /healthz 200 "$health_body"; then
	printf '%s\n' 'smoke failed: health contract mismatch' >&2
	exit 1
fi
if ! request_exact /readyz 200 "$ready_body"; then
	printf '%s\n' 'smoke failed: readiness contract mismatch' >&2
	exit 1
fi
