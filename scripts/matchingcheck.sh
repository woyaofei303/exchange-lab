#!/bin/sh
set -eu

base_url="${BASE_URL:-http://127.0.0.1:8080}"
seller_cookie="$(mktemp "${TMPDIR:-/tmp}/exchange-match-seller.XXXXXX")"
buyer_cookie="$(mktemp "${TMPDIR:-/tmp}/exchange-match-buyer.XXXXXX")"
body_file="$(mktemp "${TMPDIR:-/tmp}/exchange-match-body.XXXXXX")"
suffix="$(date +%s)-$$"
password='local matching acceptance password 2026'

cleanup() {
	status=$?
	trap - EXIT HUP INT TERM
	rm -f "$seller_cookie" "$buyer_cookie" "$body_file"
	exit "$status"
}
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

create_trader() {
	email=$1
	cookie=$2
	key=$3
	curl --connect-timeout 2 --max-time 5 --fail --silent -H 'Content-Type: application/json' --data "{\"email\":\"$email\",\"password\":\"$password\"}" "$base_url/v1/auth/register" >/dev/null
	curl --connect-timeout 2 --max-time 5 --fail --silent --cookie-jar "$cookie" -H 'Content-Type: application/json' --data "{\"email\":\"$email\",\"password\":\"$password\"}" "$base_url/v1/auth/login" >/dev/null
	curl --connect-timeout 2 --max-time 5 --fail --silent --cookie "$cookie" -H 'Content-Type: application/json' -H "Idempotency-Key: faucet-$key" --data '{"amount":"1000.00000000"}' "$base_url/v1/faucet" >/dev/null
}

create_trader "match-seller-$suffix@example.test" "$seller_cookie" "seller-$suffix"
create_trader "match-buyer-$suffix@example.test" "$buyer_cookie" "buyer-$suffix"

postgres_user="$(docker compose exec -T postgres sh -c 'printf "%s" "$POSTGRES_USER"')"
postgres_database="$(docker compose exec -T postgres sh -c 'printf "%s" "$POSTGRES_DB"')"
mark_atoms="$(docker compose exec -T postgres psql -U "$postgres_user" -d "$postgres_database" -Atc "SELECT mark_price_atoms FROM mark_prices WHERE symbol='BTCUSDT-PERP' ORDER BY created_at DESC,id DESC LIMIT 1;")"
max_bid="$(docker compose exec -T postgres psql -U "$postgres_user" -d "$postgres_database" -Atc "SELECT COALESCE(max(price_atoms),0) FROM matching_book_orders WHERE side='buy';")"
min_ask="$(docker compose exec -T postgres psql -U "$postgres_user" -d "$postgres_database" -Atc "SELECT COALESCE(min(price_atoms),0) FROM matching_book_orders WHERE side='sell';")"
price_atoms=$mark_atoms
if [ "$max_bid" -gt 0 ]; then
	price_atoms=$((max_bid + 10))
fi
if [ "$min_ask" -gt 0 ] && [ "$price_atoms" -ge "$min_ask" ]; then
	printf '%s\n' 'matching check failed: no isolated price gap in existing order book' >&2
	exit 1
fi
price="$(printf '%d.%02d' "$((price_atoms / 100))" "$((price_atoms % 100))")"

curl --connect-timeout 2 --max-time 5 --fail --silent --cookie "$seller_cookie" -H 'Content-Type: application/json' -H "Idempotency-Key: sell-$suffix" --data "{\"symbol\":\"BTCUSDT-PERP\",\"side\":\"sell\",\"type\":\"limit\",\"price\":\"$price\",\"quantity\":\"0.010\",\"leverage\":10,\"client_order_id\":\"sell-$suffix\"}" "$base_url/v1/orders" >"$body_file"
sell_id="$(sed -n 's/.*"id":"\([^"]*\)".*/\1/p' "$body_file")"
[ -n "$sell_id" ]

attempt=0
while [ "$attempt" -lt 30 ]; do
	resting="$(docker compose exec -T postgres psql -U "$postgres_user" -d "$postgres_database" -Atc "SELECT count(*) FROM matching_book_orders WHERE order_id='$sell_id';")"
	[ "$resting" = 1 ] && break
	attempt=$((attempt + 1))
	sleep 1
done
[ "$resting" = 1 ]

curl --connect-timeout 2 --max-time 5 --fail --silent --cookie "$buyer_cookie" -H 'Content-Type: application/json' -H "Idempotency-Key: buy-$suffix" --data "{\"symbol\":\"BTCUSDT-PERP\",\"side\":\"buy\",\"type\":\"limit\",\"price\":\"$price\",\"quantity\":\"0.010\",\"leverage\":10,\"client_order_id\":\"buy-$suffix\"}" "$base_url/v1/orders" >"$body_file"
buy_id="$(sed -n 's/.*"id":"\([^"]*\)".*/\1/p' "$body_file")"
[ -n "$buy_id" ]

attempt=0
trade=""
while [ "$attempt" -lt 30 ]; do
	trade="$(docker compose exec -T postgres psql -U "$postgres_user" -d "$postgres_database" -AtF '|' -c "SELECT price_atoms,quantity_atoms FROM contract_trades WHERE maker_order_id='$sell_id' AND taker_order_id='$buy_id';")"
	[ "$trade" = "$price_atoms|10" ] && break
	attempt=$((attempt + 1))
	sleep 1
done
[ "$trade" = "$price_atoms|10" ]
states="$(docker compose exec -T postgres psql -U "$postgres_user" -d "$postgres_database" -Atc "SELECT string_agg(status,',' ORDER BY id) FROM orders WHERE id IN ('$sell_id','$buy_id');")"
[ "$states" = 'filled,filled' ]
resting="$(docker compose exec -T postgres psql -U "$postgres_user" -d "$postgres_database" -Atc "SELECT count(*) FROM matching_book_orders WHERE order_id IN ('$sell_id','$buy_id');")"
[ "$resting" = 0 ]

printf '%s\n' 'matching check: maker price, price-time match, exact quantity, filled states and empty matched book verified'
