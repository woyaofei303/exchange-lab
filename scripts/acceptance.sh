#!/bin/sh
set -eu

base_url="${BASE_URL:-http://127.0.0.1:8080}"
response_file="$(mktemp "${TMPDIR:-/tmp}/exchange-acceptance.XXXXXX")"
completed=0

cleanup() {
	status=$?
	trap - EXIT HUP INT TERM
	rm -f "$response_file"
	if [ "$completed" -ne 1 ]; then
		docker compose start redis >/dev/null 2>&1 || true
		docker compose up -d api >/dev/null 2>&1 || true
	fi
	exit "$status"
}

trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

./scripts/smoke.sh

docker compose exec -T postgres psql -U exchange -d exchange -v ON_ERROR_STOP=1 -c \
	"CREATE TABLE IF NOT EXISTS week_one_probe (id integer PRIMARY KEY, note text NOT NULL);"
docker compose exec -T postgres psql -U exchange -d exchange -v ON_ERROR_STOP=1 -c \
	"INSERT INTO week_one_probe (id, note) VALUES (1, 'persistent') ON CONFLICT (id) DO UPDATE SET note = EXCLUDED.note;"

docker compose stop redis
health_status="$(curl --silent --output "$response_file" --write-out '%{http_code}' "$base_url/healthz")"
test "$health_status" = "200"
test "$(tr -d '\n' <"$response_file")" = '{"status":"ok"}'
printf 'redis outage healthz: HTTP %s %s\n' "$health_status" "$(tr -d '\n' <"$response_file")"

ready_status="$(curl --silent --output "$response_file" --write-out '%{http_code}' "$base_url/readyz")"
test "$ready_status" = "503"
grep -q '"status":"not_ready"' "$response_file"
grep -q '"redis":"error"' "$response_file"
grep -q '"postgres":"ok"' "$response_file"
grep -q '"nats":"ok"' "$response_file"
error_count="$(grep -o '"[^"]*":"error"' "$response_file" | wc -l | tr -d ' ')"
test "$error_count" = "1"
printf 'redis outage readyz: HTTP %s %s\n' "$ready_status" "$(tr -d '\n' <"$response_file")"

docker compose start redis
attempt=0
until curl --fail --silent "$base_url/readyz" >"$response_file"; do
	attempt=$((attempt + 1))
	test "$attempt" -lt 30
	sleep 1
done
grep -q '"status":"ready"' "$response_file"
grep -q '"postgres":"ok"' "$response_file"
grep -q '"redis":"ok"' "$response_file"
grep -q '"nats":"ok"' "$response_file"
printf 'redis recovery readyz: HTTP 200 %s\n' "$(tr -d '\n' <"$response_file")"

docker compose up -d --force-recreate postgres api
attempt=0
until curl --fail --silent "$base_url/readyz" >"$response_file"; do
	attempt=$((attempt + 1))
	test "$attempt" -lt 30
	sleep 1
done

value="$(docker compose exec -T postgres psql -U exchange -d exchange -Atc "SELECT note FROM week_one_probe WHERE id = 1;")"
test "$value" = "persistent"
printf 'postgres marker after forced postgres+api recreation: %s\n' "$value"

completed=1
