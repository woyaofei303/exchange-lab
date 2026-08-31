#!/bin/sh
set -eu

base_url="${BASE_URL:-http://127.0.0.1:8080}"
simulator_base_url="${SIMULATOR_BASE_URL:-http://127.0.0.1:8090}"
web_base_url="${WEB_BASE_URL:-http://127.0.0.1:3000}"
event_base_url="${EVENT_BASE_URL:-http://127.0.0.1:8092}"
price_base_url="${PRICE_BASE_URL:-http://127.0.0.1:8093}"
risk_base_url="${RISK_BASE_URL:-http://127.0.0.1:8094}"
funding_base_url="${FUNDING_BASE_URL:-http://127.0.0.1:8095}"
curl_connect_timeout=1
curl_max_time=2
health_body='{"status":"ok"}'
ready_body='{"status":"ready","checks":{"nats":"ok","postgres":"ok","redis":"ok"}}'
instruments_body='{"data":[{"symbol":"BTCUSDT-PERP","base_asset":"BTC","quote_asset":"USDT","price_scale":2,"quantity_scale":3,"price_step":"0.10","quantity_step":"0.001","min_notional":"5.00000000"}]}'
response_file="$(mktemp "${TMPDIR:-/tmp}/exchange-smoke.XXXXXX")"
headers_file="$(mktemp "${TMPDIR:-/tmp}/exchange-smoke-headers.XXXXXX")"

cleanup() {
	status=$?
	trap - EXIT HUP INT TERM
	rm -f "$response_file" "$headers_file"
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
		--silent --dump-header "$headers_file" --output "$response_file" --write-out '%{http_code}' "$base_url$path")"; then
		actual_body="$(cat "$response_file")"
		[ "$actual_status" = "$expected_status" ] && [ "$actual_body" = "$expected_body" ] &&
			grep -Eiq '^X-Request-ID: [A-Za-z0-9][A-Za-z0-9._-]*' "$headers_file"
		return
	fi
	return 1
}

request_ticker() {
	if actual_status="$(curl --connect-timeout "$curl_connect_timeout" --max-time "$curl_max_time" \
		--silent --dump-header "$headers_file" --output "$response_file" --write-out '%{http_code}' "$base_url/v1/tickers")"; then
		actual_body="$(cat "$response_file")"
		[ "$actual_status" = 200 ] &&
			grep -Eiq '^X-Request-ID: [A-Za-z0-9][A-Za-z0-9._-]*' "$headers_file" &&
			printf '%s' "$actual_body" | grep -Eq '"symbol":"BTCUSDT-PERP"' &&
			printf '%s' "$actual_body" | grep -Eq '"last_price":"[0-9]+\.[0-9]{2}"' &&
			printf '%s' "$actual_body" | grep -Eq '"updated_at":"[^"]+"'
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
if ! request_exact /v1/instruments 200 "$instruments_body"; then
	printf '%s\n' 'smoke failed: instruments contract mismatch' >&2
	exit 1
fi
if ! request_ticker; then
	printf '%s\n' 'smoke failed: tickers contract mismatch' >&2
	exit 1
fi
if ! curl --connect-timeout "$curl_connect_timeout" --max-time "$curl_max_time" --fail --silent "$simulator_base_url/healthz" |
	grep -qx '{"status":"ok"}'; then
	printf '%s\n' 'smoke failed: market simulator health mismatch' >&2
	exit 1
fi
if ! curl --connect-timeout "$curl_connect_timeout" --max-time "$curl_max_time" --fail --silent "$web_base_url/healthz" |
	grep -qx '{"status":"ok"}'; then
	printf '%s\n' 'smoke failed: web health mismatch' >&2
	exit 1
fi
if ! curl --connect-timeout "$curl_connect_timeout" --max-time "$curl_max_time" --fail --silent "$event_base_url/healthz" |
	grep -qx '{"status":"ready"}'; then
	printf '%s\n' 'smoke failed: event worker health mismatch' >&2
	exit 1
fi
for name_and_url in "price:$price_base_url" "risk:$risk_base_url" "funding:$funding_base_url"; do
	name=${name_and_url%%:*}
	url=${name_and_url#*:}
	if ! curl --connect-timeout "$curl_connect_timeout" --max-time "$curl_max_time" --fail --silent "$url/healthz" |
		grep -qx '{"status":"ready"}'; then
		printf '%s\n' "smoke failed: $name worker health mismatch" >&2
		exit 1
	fi
done
if ! curl --connect-timeout "$curl_connect_timeout" --max-time "$curl_max_time" --fail --silent "$base_url/v1/prices?symbol=BTCUSDT-PERP" |
	grep -q '"mark_price"'; then
	printf '%s\n' 'smoke failed: authoritative risk price mismatch' >&2
	exit 1
fi
if ! curl --connect-timeout "$curl_connect_timeout" --max-time "$curl_max_time" --fail --silent "$web_base_url/" |
	grep -q '<title>Exchange Lab · 永续合约实验室</title>'; then
	printf '%s\n' 'smoke failed: trading terminal shell mismatch' >&2
	exit 1
fi
