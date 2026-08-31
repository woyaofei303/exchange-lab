#!/bin/sh
set -eu

base_url="${BASE_URL:-http://127.0.0.1:8080}"
funding_url="${FUNDING_BASE_URL:-http://127.0.0.1:8095}"
seller_cookie="$(mktemp "${TMPDIR:-/tmp}/exchange-derivative-seller.XXXXXX")"
buyer_cookie="$(mktemp "${TMPDIR:-/tmp}/exchange-derivative-buyer.XXXXXX")"
liq_seller_cookie="$(mktemp "${TMPDIR:-/tmp}/exchange-derivative-liq-seller.XXXXXX")"
liq_buyer_cookie="$(mktemp "${TMPDIR:-/tmp}/exchange-derivative-liq-buyer.XXXXXX")"
body_file="$(mktemp "${TMPDIR:-/tmp}/exchange-derivative-body.XXXXXX")"
suffix="$(date +%s)-$$"
password='local derivative acceptance password 2026'
price_worker_stopped=0

cleanup() {
	status=$?
	trap - EXIT HUP INT TERM
	if [ "$price_worker_stopped" -eq 1 ]; then
		docker compose start price-worker >/dev/null 2>&1 || true
	fi
	rm -f "$seller_cookie" "$buyer_cookie" "$liq_seller_cookie" "$liq_buyer_cookie" "$body_file"
	exit "$status"
}
trap cleanup EXIT HUP INT TERM

request_post() {
	cookie=$1
	path=$2
	key=$3
	payload=$4
	curl --connect-timeout 2 --max-time 10 --fail --silent --cookie "$cookie" \
		-H 'Content-Type: application/json' -H "Idempotency-Key: $key" --data "$payload" "$base_url$path"
}

create_trader() {
	email=$1
	cookie=$2
	key=$3
	curl --connect-timeout 2 --max-time 10 --fail --silent -H 'Content-Type: application/json' \
		--data "{\"email\":\"$email\",\"password\":\"$password\"}" "$base_url/v1/auth/register" >/dev/null
	curl --connect-timeout 2 --max-time 10 --fail --silent --cookie-jar "$cookie" -H 'Content-Type: application/json' \
		--data "{\"email\":\"$email\",\"password\":\"$password\"}" "$base_url/v1/auth/login" >/dev/null
	request_post "$cookie" /v1/faucet "faucet-$key" '{"amount":"10000.00000000"}' >/dev/null
}

place_order() {
	cookie=$1
	side=$2
	price=$3
	key=$4
	request_post "$cookie" /v1/orders "$key" "{\"symbol\":\"BTCUSDT-PERP\",\"side\":\"$side\",\"type\":\"limit\",\"price\":\"$price\",\"quantity\":\"0.010\",\"leverage\":10,\"client_order_id\":\"$key\"}" >"$body_file"
	sed -n 's/.*"id":"\([^"]*\)".*/\1/p' "$body_file"
}

wait_for_sql() {
	query=$1
	want=$2
	attempt=0
	while [ "$attempt" -lt 40 ]; do
		got="$(docker compose exec -T postgres psql -U "$postgres_user" -d "$postgres_database" -Atc "$query")"
		if [ "$got" = "$want" ]; then
			return 0
		fi
		attempt=$((attempt + 1))
		sleep 1
	done
	printf '%s\n' "derivatives check timed out: got '$got', want '$want'" >&2
	return 1
}

postgres_user="$(docker compose exec -T postgres sh -c 'printf "%s" "$POSTGRES_USER"')"
postgres_database="$(docker compose exec -T postgres sh -c 'printf "%s" "$POSTGRES_DB"')"
parked_before="$(docker compose exec -T postgres psql -U "$postgres_user" -d "$postgres_database" -Atc "SELECT count(*) FROM parked_events;")"
mark_atoms="$(docker compose exec -T postgres psql -U "$postgres_user" -d "$postgres_database" -Atc "SELECT mark_price_atoms FROM mark_prices WHERE symbol='BTCUSDT-PERP' ORDER BY created_at DESC,id DESC LIMIT 1;")"
max_bid="$(docker compose exec -T postgres psql -U "$postgres_user" -d "$postgres_database" -Atc "SELECT COALESCE(max(price_atoms),0) FROM matching_book_orders WHERE side='buy';")"
min_ask="$(docker compose exec -T postgres psql -U "$postgres_user" -d "$postgres_database" -Atc "SELECT COALESCE(min(price_atoms),0) FROM matching_book_orders WHERE side='sell';")"
price_atoms=$mark_atoms
if [ "$max_bid" -gt 0 ]; then price_atoms=$((max_bid + 10)); fi
if [ "$min_ask" -gt 0 ] && [ "$price_atoms" -ge "$min_ask" ]; then
	printf '%s\n' 'derivatives check failed: no isolated price gap in existing order book' >&2
	exit 1
fi
price="$(printf '%d.%02d' "$((price_atoms / 100))" "$((price_atoms % 100))")"

seller_email="derivative-seller-$suffix@example.test"
buyer_email="derivative-buyer-$suffix@example.test"
create_trader "$seller_email" "$seller_cookie" "seller-$suffix"
create_trader "$buyer_email" "$buyer_cookie" "buyer-$suffix"
seller_id="$(docker compose exec -T postgres psql -U "$postgres_user" -d "$postgres_database" -Atc "SELECT id FROM users WHERE email='$seller_email';")"
buyer_id="$(docker compose exec -T postgres psql -U "$postgres_user" -d "$postgres_database" -Atc "SELECT id FROM users WHERE email='$buyer_email';")"

request_post "$seller_cookie" /v1/leverage "leverage-seller-$suffix" '{"symbol":"BTCUSDT-PERP","leverage":10}' >/dev/null
request_post "$buyer_cookie" /v1/leverage "leverage-buyer-$suffix" '{"symbol":"BTCUSDT-PERP","leverage":10}' >/dev/null
request_post "$seller_cookie" /v1/margin-mode "mode-seller-$suffix" '{"mode":"isolated"}' >/dev/null
request_post "$buyer_cookie" /v1/margin-mode "mode-buyer-$suffix" '{"mode":"cross"}' >/dev/null

open_sell_id="$(place_order "$seller_cookie" sell "$price" "open-sell-$suffix")"
test -n "$open_sell_id"
wait_for_sql "SELECT count(*) FROM matching_book_orders WHERE order_id='$open_sell_id';" 1
open_buy_id="$(place_order "$buyer_cookie" buy "$price" "open-buy-$suffix")"
test -n "$open_buy_id"
wait_for_sql "SELECT concat_ws('|',(SELECT size_atoms FROM positions WHERE user_id=$seller_id AND symbol='BTCUSDT-PERP'),(SELECT size_atoms FROM positions WHERE user_id=$buyer_id AND symbol='BTCUSDT-PERP'));" '-10|10'

curl --fail --silent --cookie "$seller_cookie" "$base_url/v1/positions" >"$body_file"
grep -q '"size":"-0.010"' "$body_file"
curl --fail --silent --cookie "$buyer_cookie" "$base_url/v1/risk?symbol=BTCUSDT-PERP" >"$body_file"
grep -q '"mode":"cross"' "$body_file"
grep -q '"liquidation_distance_bps":' "$body_file"
grep -q '"fresh":true' "$body_file"

adjust_key="margin-$suffix"
first_adjust="$(request_post "$seller_cookie" /v1/margin/adjust "$adjust_key" '{"symbol":"BTCUSDT-PERP","amount":"10.00000000"}')"
second_adjust="$(request_post "$seller_cookie" /v1/margin/adjust "$adjust_key" '{"symbol":"BTCUSDT-PERP","amount":"10.00000000"}')"
printf '%s' "$first_adjust" | grep -q '"replayed":false'
printf '%s' "$second_adjust" | grep -q '"replayed":true'

request_post "$buyer_cookie" /v1/conditional-orders "conditional-$suffix" "{\"symbol\":\"BTCUSDT-PERP\",\"type\":\"stop_loss\",\"trigger_source\":\"mark\",\"trigger_direction\":\"lte\",\"trigger_price\":\"1000.00\",\"quantity\":\"0.010\"}" >"$body_file"
grep -q '"status":"pending"' "$body_file"
wait_for_sql "SELECT reduce_only::int FROM conditional_orders WHERE user_id=$buyer_id AND idempotency_key='conditional-$suffix';" 1

docker compose stop price-worker >/dev/null
price_worker_stopped=1
index_atoms="$(docker compose exec -T postgres psql -U "$postgres_user" -d "$postgres_database" -Atc "SELECT price_atoms FROM index_prices WHERE symbol='BTCUSDT-PERP' ORDER BY created_at DESC,id DESC LIMIT 1;")"
funding_mark_atoms=$((index_atoms + index_atoms / 1000))
docker compose exec -T postgres psql -U "$postgres_user" -d "$postgres_database" -v ON_ERROR_STOP=1 -c "INSERT INTO mark_prices(symbol,index_price_atoms,mark_price_atoms,price_scale,premium_bps,fresh,as_of) VALUES('BTCUSDT-PERP',$index_atoms,$funding_mark_atoms,2,10,true,now());" >/dev/null
funding_at="$(docker compose exec -T postgres psql -U "$postgres_user" -d "$postgres_database" -Atc "SELECT to_char((COALESCE(min(funding_time)-interval '8 hours',date_trunc('hour',now())-interval '8 hours') AT TIME ZONE 'UTC'),'YYYY-MM-DD\"T\"HH24:MI:SS\"Z\"') FROM funding_rates;")"
funding_first="$(curl --connect-timeout 2 --max-time 15 --fail --silent -X POST "$funding_url/run?at=$funding_at")"
funding_second="$(curl --connect-timeout 2 --max-time 15 --fail --silent -X POST "$funding_url/run?at=$funding_at")"
first_funding_id="$(printf '%s' "$funding_first" | sed -n 's/.*"funding_rate_id":\([0-9][0-9]*\).*/\1/p')"
second_funding_id="$(printf '%s' "$funding_second" | sed -n 's/.*"funding_rate_id":\([0-9][0-9]*\).*/\1/p')"
test -n "$first_funding_id" && [ "$first_funding_id" = "$second_funding_id" ]
wait_for_sql "SELECT count(*) FROM funding_settlements WHERE funding_rate_id=$first_funding_id AND user_id IN ($seller_id,$buyer_id);" 2
docker compose start price-worker >/dev/null
price_worker_stopped=0

ai_runs_before="$(docker compose exec -T postgres psql -U "$postgres_user" -d "$postgres_database" -Atc "SELECT count(*) FROM ai_runs WHERE user_id=$buyer_id;")"
curl --connect-timeout 2 --max-time 15 --fail --silent --cookie "$buyer_cookie" "$base_url/v1/ai/contract-risk-summary?symbol=BTCUSDT-PERP" >"$body_file"
grep -q '"evidence_ids"' "$body_file"
grep -q '不构成收益承诺或投资建议' "$body_file"
wait_for_sql "SELECT count(*) FROM ai_runs WHERE user_id=$buyer_id;" "$((ai_runs_before + 1))"

close_sell_id="$(place_order "$buyer_cookie" sell "$price" "close-sell-$suffix")"
test -n "$close_sell_id"
wait_for_sql "SELECT count(*) FROM matching_book_orders WHERE order_id='$close_sell_id';" 1
close_buy_id="$(place_order "$seller_cookie" buy "$price" "close-buy-$suffix")"
test -n "$close_buy_id"
wait_for_sql "SELECT concat_ws('|',(SELECT size_atoms FROM positions WHERE user_id=$seller_id AND symbol='BTCUSDT-PERP'),(SELECT size_atoms FROM positions WHERE user_id=$buyer_id AND symbol='BTCUSDT-PERP'));" '0|0'

liq_seller_email="derivative-liq-seller-$suffix@example.test"
liq_buyer_email="derivative-liq-buyer-$suffix@example.test"
create_trader "$liq_seller_email" "$liq_seller_cookie" "liq-seller-$suffix"
create_trader "$liq_buyer_email" "$liq_buyer_cookie" "liq-buyer-$suffix"
liq_buyer_id="$(docker compose exec -T postgres psql -U "$postgres_user" -d "$postgres_database" -Atc "SELECT id FROM users WHERE email='$liq_buyer_email';")"
liq_sell_id="$(place_order "$liq_seller_cookie" sell 60000.00 "liq-sell-$suffix")"
wait_for_sql "SELECT count(*) FROM matching_book_orders WHERE order_id='$liq_sell_id';" 1
liq_buy_id="$(place_order "$liq_buyer_cookie" buy 60000.00 "liq-buy-$suffix")"
test -n "$liq_buy_id"
wait_for_sql "SELECT count(*) FROM liquidation_jobs WHERE user_id=$liq_buyer_id AND status='completed';" 1
curl --fail --silent --cookie "$liq_buyer_cookie" "$base_url/v1/liquidations" >"$body_file"
grep -q '"status":"completed"' "$body_file"
curl --fail --silent "$base_url/v1/insurance-fund" >"$body_file"
grep -q '"balance"' "$body_file"

broken="$(docker compose exec -T postgres psql -U "$postgres_user" -d "$postgres_database" -Atc "SELECT count(*) FROM (SELECT transaction_id FROM ledger_entries GROUP BY transaction_id HAVING sum(amount_atoms)<>0) invalid;")"
[ "$broken" = 0 ]
parked_after="$(docker compose exec -T postgres psql -U "$postgres_user" -d "$postgres_database" -Atc "SELECT count(*) FROM parked_events;")"
[ "$parked_after" = "$parked_before" ]

printf '%s\n' 'derivatives check: isolated/cross positions, PnL, margin replay, reduce-only SL, funding replay, AI evidence, close, liquidation, insurance and ledger conservation verified'
