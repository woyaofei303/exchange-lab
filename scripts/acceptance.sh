#!/bin/sh
set -eu

base_url="${BASE_URL:-http://127.0.0.1:8080}"
curl_connect_timeout=1
curl_max_time=2
max_attempts=20
health_body='{"status":"ok"}'
ready_body='{"status":"ready","checks":{"nats":"ok","postgres":"ok","redis":"ok"}}'
not_ready_body='{"status":"not_ready","checks":{"nats":"ok","postgres":"ok","redis":"error"}}'
nats_not_ready_body='{"status":"not_ready","checks":{"nats":"error","postgres":"ok","redis":"ok"}}'
instruments_body='{"data":[{"symbol":"BTCUSDT-PERP","base_asset":"BTC","quote_asset":"USDT","price_scale":2,"quantity_scale":3,"price_step":"0.10","quantity_step":"0.001","min_notional":"5.00000000"}]}'
response_file="$(mktemp "${TMPDIR:-/tmp}/exchange-acceptance.XXXXXX")"
completed=0

request_matches() {
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

wait_for_response() {
	path=$1
	expected_status=$2
	expected_body=$3
	attempt=0
	while [ "$attempt" -lt "$max_attempts" ]; do
		if request_matches "$path" "$expected_status" "$expected_body"; then
			return 0
		fi
		attempt=$((attempt + 1))
		if [ "$attempt" -lt "$max_attempts" ]; then
			sleep 1
		fi
	done
	return 1
}

all_services_healthy() {
	for service in api web postgres redis nats market-simulator candle-worker event-worker price-worker risk-worker funding-worker; do
		container_id="$(docker compose ps -q "$service")" || return 1
		[ -n "$container_id" ] || return 1
		health="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}missing{{end}}' "$container_id")" || return 1
		[ "$health" = "healthy" ] || return 1
	done
	market_worker_id="$(docker compose ps -q market-worker)" || return 1
	[ -n "$market_worker_id" ] || return 1
	[ "$(docker inspect --format '{{.State.Running}}' "$market_worker_id")" = true ] || return 1
}

wait_for_services_healthy() {
	attempt=0
	while [ "$attempt" -lt "$max_attempts" ]; do
		if all_services_healthy; then
			return 0
		fi
		attempt=$((attempt + 1))
		if [ "$attempt" -lt "$max_attempts" ]; then
			sleep 1
		fi
	done
	return 1
}

wait_for_full_recovery() {
	attempt=0
	while [ "$attempt" -lt "$max_attempts" ]; do
		if request_matches /readyz 200 "$ready_body" && all_services_healthy; then
			return 0
		fi
		attempt=$((attempt + 1))
		if [ "$attempt" -lt "$max_attempts" ]; then
			sleep 1
		fi
	done
	return 1
}

wait_for_market_pipeline() {
	attempt=0
	while [ "$attempt" -lt "$max_attempts" ]; do
		if go run ./scripts/marketcheck; then
			return 0
		fi
		attempt=$((attempt + 1))
		if [ "$attempt" -lt "$max_attempts" ]; then
			sleep 1
		fi
	done
	return 1
}

fail() {
	printf '%s\n' "$1" >&2
	exit 1
}

cleanup() {
	status=$?
	trap - EXIT HUP INT TERM
	if [ "$completed" -ne 1 ]; then
		cleanup_failed=0
		if ! docker compose up -d postgres redis nats market-simulator candle-worker event-worker price-worker risk-worker funding-worker market-worker api web >/dev/null 2>&1; then
			cleanup_failed=1
		fi
		if ! wait_for_full_recovery; then
			cleanup_failed=1
		fi
		if [ "$cleanup_failed" -ne 0 ]; then
			printf '%s\n' 'acceptance cleanup failed to restore and verify service health' >&2
		fi
		if [ "$status" -eq 0 ]; then
			status=1
		fi
	fi
	rm -f "$response_file"
	exit "$status"
}

trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

./scripts/smoke.sh

if ! postgres_user="$(docker compose exec -T postgres sh -c 'test -n "${POSTGRES_USER:-}" && printf "%s" "$POSTGRES_USER"')"; then
	fail 'acceptance failed: running PostgreSQL user is unavailable'
fi
if ! postgres_database="$(docker compose exec -T postgres sh -c 'test -n "${POSTGRES_DB:-}" && printf "%s" "$POSTGRES_DB"')"; then
	fail 'acceptance failed: running PostgreSQL database is unavailable'
fi
if [ -z "$postgres_user" ] || [ -z "$postgres_database" ]; then
	fail 'acceptance failed: running PostgreSQL identity is unavailable'
fi

docker compose exec -T postgres psql -U "$postgres_user" -d "$postgres_database" -v ON_ERROR_STOP=1 -c \
	"CREATE TABLE IF NOT EXISTS week_one_probe (id integer PRIMARY KEY, note text NOT NULL);"
docker compose exec -T postgres psql -U "$postgres_user" -d "$postgres_database" -v ON_ERROR_STOP=1 -c \
	"INSERT INTO week_one_probe (id, note) VALUES (1, 'persistent') ON CONFLICT (id) DO UPDATE SET note = EXCLUDED.note;"

docker compose stop redis
if ! request_matches /healthz 200 "$health_body"; then
	fail 'acceptance failed: Redis-outage liveness contract mismatch'
fi
printf '%s\n' 'redis outage healthz: HTTP 200 exact contract verified'

if ! request_matches /readyz 503 "$not_ready_body"; then
	fail 'acceptance failed: Redis-outage readiness contract mismatch'
fi
printf '%s\n' 'redis outage readyz: HTTP 503 exact Redis-only failure verified'

if ! request_matches /v1/instruments 200 "$instruments_body"; then
	fail 'acceptance failed: Redis-outage authoritative instrument fallback mismatch'
fi
printf '%s\n' 'redis outage instruments: HTTP 200 PostgreSQL fallback verified'

docker compose start redis
if ! wait_for_response /readyz 200 "$ready_body"; then
	fail 'acceptance failed: Redis recovery readiness contract mismatch'
fi
printf '%s\n' 'redis recovery readyz: HTTP 200 exact contract verified'

docker compose stop nats
docker compose up -d --no-deps --force-recreate api
if ! wait_for_response /healthz 200 "$health_body"; then
	fail 'acceptance failed: NATS cold-start outage liveness contract mismatch'
fi
printf '%s\n' 'nats cold-start outage healthz: HTTP 200 exact contract verified'

if ! wait_for_response /readyz 503 "$nats_not_ready_body"; then
	fail 'acceptance failed: NATS cold-start outage readiness contract mismatch'
fi
printf '%s\n' 'nats cold-start outage readyz: HTTP 503 exact NATS-only failure verified'

docker compose start nats
if ! wait_for_full_recovery; then
	fail 'acceptance failed: NATS automatic recovery or Compose health mismatch'
fi
printf '%s\n' 'nats automatic recovery: exact HTTP 200 contract and dependent services healthy'

docker compose up -d --force-recreate postgres api
if ! wait_for_response /readyz 200 "$ready_body"; then
	fail 'acceptance failed: recreation readiness contract mismatch'
fi
printf '%s\n' 'recreation readyz: HTTP 200 exact contract verified'

if ! wait_for_services_healthy; then
	fail 'acceptance failed: Compose services did not become healthy'
fi
printf '%s\n' 'compose health: web, API, data stores, simulator, and all derivative workers healthy'

value="$(docker compose exec -T postgres psql -U "$postgres_user" -d "$postgres_database" -Atc "SELECT note FROM week_one_probe WHERE id = 1;")"
if [ "$value" != "persistent" ]; then
	fail 'acceptance failed: PostgreSQL persistence marker mismatch'
fi
printf '%s\n' 'postgres marker after forced postgres+api recreation: persistent'

if ! wait_for_market_pipeline; then
	fail 'acceptance failed: normalized market pipeline did not recover'
fi
if ! go run ./scripts/candlecheck; then
	fail 'acceptance failed: candle REST/WebSocket pipeline did not recover'
fi
if ! POSTGRES_DSN="${CHECK_POSTGRES_DSN:-postgres://exchange:exchange_dev_only@127.0.0.1:5432/exchange?sslmode=disable}" go run ./scripts/candlequality; then
	fail 'acceptance failed: candle data quality check failed'
fi
if ! ./scripts/authcheck.sh; then
	fail 'acceptance failed: identity session flow failed'
fi
if ! ./scripts/ledgercheck.sh; then
	fail 'acceptance failed: double-entry ledger flow failed'
fi
if ! ./scripts/ordercheck.sh; then
	fail 'acceptance failed: order freeze/outbox flow failed'
fi
if ! ./scripts/eventcheck.sh; then
	fail 'acceptance failed: JetStream idempotent event flow failed'
fi
if ! ./scripts/matchingcheck.sh; then
	fail 'acceptance failed: deterministic matching flow failed'
fi
if ! ./scripts/derivatives-check.sh; then
	fail 'acceptance failed: perpetual derivative flow failed'
fi

completed=1
