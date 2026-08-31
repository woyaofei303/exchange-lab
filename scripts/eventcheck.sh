#!/bin/sh
set -eu

base_url="${BASE_URL:-http://127.0.0.1:8080}"
cookie_file="$(mktemp "${TMPDIR:-/tmp}/exchange-event-cookie.XXXXXX")"
body_file="$(mktemp "${TMPDIR:-/tmp}/exchange-event-body.XXXXXX")"
email="event-check-$(date +%s)-$$@example.test"
password='local event acceptance password 2026'
place_key="event-place-$(date +%s)-$$"

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
curl --connect-timeout 2 --max-time 5 --fail --silent --cookie "$cookie_file" -H 'Content-Type: application/json' -H "Idempotency-Key: $place_key" --data '{"symbol":"BTCUSDT-PERP","side":"buy","type":"limit","price":"49900.00","quantity":"0.010","leverage":10,"client_order_id":"event-order-0001"}' "$base_url/v1/orders" >"$body_file"
order_id="$(sed -n 's/.*"id":"\([^"]*\)".*/\1/p' "$body_file")"
[ -n "$order_id" ]

postgres_user="$(docker compose exec -T postgres sh -c 'printf "%s" "$POSTGRES_USER"')"
postgres_database="$(docker compose exec -T postgres sh -c 'printf "%s" "$POSTGRES_DB"')"
event_id=""
attempt=0
while [ "$attempt" -lt 30 ]; do
	event_id="$(docker compose exec -T postgres psql -U "$postgres_user" -d "$postgres_database" -Atc "SELECT id FROM outbox_events WHERE aggregate_id='$order_id' AND subject='orders.accepted' AND status='published' LIMIT 1;")"
	receipts="$(docker compose exec -T postgres psql -U "$postgres_user" -d "$postgres_database" -Atc "SELECT count(*) FROM event_receipts WHERE consumer_name='order-audit-v1' AND aggregate_id='$order_id';")"
	if [ -n "$event_id" ] && [ "$receipts" = 1 ]; then
		break
	fi
	attempt=$((attempt + 1))
	sleep 1
done
[ -n "$event_id" ] && [ "$receipts" = 1 ]

go run ./scripts/eventcheck -nats-url "${NATS_URL:-nats://127.0.0.1:4222}" -event-id "$event_id" -order-id "$order_id" -correlation "duplicate-$place_key"
sleep 2
processed="$(docker compose exec -T postgres psql -U "$postgres_user" -d "$postgres_database" -Atc "SELECT count(*) FROM processed_events WHERE consumer_name='order-audit-v1' AND event_id='$event_id';")"
receipts="$(docker compose exec -T postgres psql -U "$postgres_user" -d "$postgres_database" -Atc "SELECT count(*) FROM event_receipts WHERE consumer_name='order-audit-v1' AND event_id='$event_id';")"
[ "$processed" = 1 ] && [ "$receipts" = 1 ]

printf '%s\n' 'event check: Outbox published, durable consumer acknowledged and duplicate delivery caused one side effect'
