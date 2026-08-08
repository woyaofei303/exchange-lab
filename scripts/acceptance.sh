#!/bin/sh
set -eu

base_url="${BASE_URL:-http://127.0.0.1:8080}"
curl_connect_timeout="${CURL_CONNECT_TIMEOUT:-1}"
curl_max_time="${CURL_MAX_TIME:-2}"
max_attempts=20
health_body='{"status":"ok"}'
ready_body='{"status":"ready","checks":{"nats":"ok","postgres":"ok","redis":"ok"}}'
not_ready_body='{"status":"not_ready","checks":{"nats":"ok","postgres":"ok","redis":"error"}}'
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
	for service in api postgres redis nats; do
		container_id="$(docker compose ps -q "$service")" || return 1
		[ -n "$container_id" ] || return 1
		health="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}missing{{end}}' "$container_id")" || return 1
		[ "$health" = "healthy" ] || return 1
	done
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

fail() {
	printf '%s\n' "$1" >&2
	exit 1
}

cleanup() {
	status=$?
	trap - EXIT HUP INT TERM
	if [ "$completed" -ne 1 ]; then
		cleanup_failed=0
		if ! docker compose up -d postgres redis nats api >/dev/null 2>&1; then
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

CURL_CONNECT_TIMEOUT="$curl_connect_timeout" CURL_MAX_TIME="$curl_max_time" ./scripts/smoke.sh

docker compose exec -T postgres psql -U exchange -d exchange -v ON_ERROR_STOP=1 -c \
	"CREATE TABLE IF NOT EXISTS week_one_probe (id integer PRIMARY KEY, note text NOT NULL);"
docker compose exec -T postgres psql -U exchange -d exchange -v ON_ERROR_STOP=1 -c \
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

docker compose start redis
if ! wait_for_response /readyz 200 "$ready_body"; then
	fail 'acceptance failed: Redis recovery readiness contract mismatch'
fi
printf '%s\n' 'redis recovery readyz: HTTP 200 exact contract verified'

docker compose up -d --force-recreate postgres api
if ! wait_for_response /readyz 200 "$ready_body"; then
	fail 'acceptance failed: recreation readiness contract mismatch'
fi
printf '%s\n' 'recreation readyz: HTTP 200 exact contract verified'

if ! wait_for_services_healthy; then
	fail 'acceptance failed: Compose services did not become healthy'
fi
printf '%s\n' 'compose health: api, postgres, redis, and nats healthy'

value="$(docker compose exec -T postgres psql -U exchange -d exchange -Atc "SELECT note FROM week_one_probe WHERE id = 1;")"
if [ "$value" != "persistent" ]; then
	fail 'acceptance failed: PostgreSQL persistence marker mismatch'
fi
printf '%s\n' 'postgres marker after forced postgres+api recreation: persistent'

completed=1
