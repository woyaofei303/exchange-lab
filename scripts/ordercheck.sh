#!/bin/sh
set -eu

base_url="${BASE_URL:-http://127.0.0.1:8080}"
cookie_file="$(mktemp "${TMPDIR:-/tmp}/exchange-order-cookie.XXXXXX")"
body_file="$(mktemp "${TMPDIR:-/tmp}/exchange-order-body.XXXXXX")"
email="order-check-$(date +%s)-$$@example.test"
password='local order acceptance password 2026'
place_key="order-place-$(date +%s)-$$"
cancel_key="order-cancel-$(date +%s)-$$"

cleanup() {
	status=$?
	trap - EXIT HUP INT TERM
	rm -f "$cookie_file" "$body_file"
	exit "$status"
}
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

curl --connect-timeout 2 --max-time 5 --fail --silent -H 'Content-Type: application/json' --data "{\"email\":\"$email\",\"password\":\"$password\"}" "$base_url/v1/auth/register" >/dev/null
curl --connect-timeout 2 --max-time 5 --fail --silent --cookie-jar "$cookie_file" -H 'Content-Type: application/json' --data "{\"email\":\"$email\",\"password\":\"$password\"}" "$base_url/v1/auth/login" >/dev/null
curl --connect-timeout 2 --max-time 5 --fail --silent --cookie "$cookie_file" -H 'Content-Type: application/json' -H "Idempotency-Key: faucet-$place_key" --data '{"amount":"1000.00000000"}' "$base_url/v1/faucet" >/dev/null

place() {
	curl --connect-timeout 2 --max-time 5 --fail --silent --cookie "$cookie_file" -H 'Content-Type: application/json' -H "Idempotency-Key: $place_key" \
		--data '{"symbol":"BTCUSDT-PERP","side":"buy","type":"limit","price":"50000.00","quantity":"0.010","leverage":10,"client_order_id":"acceptance-order-0001"}' "$base_url/v1/orders"
}

first="$(place)"
second="$(place)"
printf '%s' "$first" | grep -q '"reserved_margin":"50.00000000"'
printf '%s' "$first" | grep -q '"replayed":false'
printf '%s' "$second" | grep -q '"replayed":true'
order_id="$(printf '%s' "$first" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')"
[ -n "$order_id" ]

curl --connect-timeout 2 --max-time 5 --fail --silent --cookie "$cookie_file" "$base_url/v1/balances" >"$body_file"
grep -q '"available":"950.00000000"' "$body_file"
grep -q '"locked":"50.00000000"' "$body_file"

cancel() {
	curl --connect-timeout 2 --max-time 5 --fail --silent --cookie "$cookie_file" -X POST -H "Idempotency-Key: $cancel_key" "$base_url/v1/orders/$order_id/cancel"
}
cancel_first="$(cancel)"
cancel_second="$(cancel)"
printf '%s' "$cancel_first" | grep -q '"status":"canceled"'
printf '%s' "$cancel_second" | grep -q '"replayed":true'
curl --connect-timeout 2 --max-time 5 --fail --silent --cookie "$cookie_file" "$base_url/v1/balances" >"$body_file"
grep -q '"available":"1000.00000000"' "$body_file"
grep -q '"locked":"0.00000000"' "$body_file"

postgres_user="$(docker compose exec -T postgres sh -c 'printf "%s" "$POSTGRES_USER"')"
postgres_database="$(docker compose exec -T postgres sh -c 'printf "%s" "$POSTGRES_DB"')"
facts="$(docker compose exec -T postgres psql -U "$postgres_user" -d "$postgres_database" -Atc "SELECT count(*) FROM outbox_events WHERE aggregate_id='$order_id' AND subject IN ('orders.accepted','orders.cancel_requested');")"
[ "$facts" = 2 ]

printf '%s\n' 'order check: atomic reserve, idempotent place/cancel, exact release and transactional outbox verified'
