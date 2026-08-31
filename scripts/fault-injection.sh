#!/bin/sh
set -eu
api_url="${BASE_URL:-http://127.0.0.1:18080}"
cleanup() {
  status=$?
  trap - EXIT HUP INT TERM
  docker compose start nats >/dev/null 2>&1 || true
  exit "$status"
}
trap cleanup EXIT HUP INT TERM

docker compose stop nats
if curl --fail --silent "$api_url/readyz" >/dev/null 2>&1; then
  printf '%s\n' 'fault check failed: API remained ready without NATS' >&2
  exit 1
fi
docker compose start nats
attempt=0
while [ "$attempt" -lt 30 ]; do
  if curl --connect-timeout 2 --max-time 3 --fail --silent "$api_url/readyz" >/dev/null 2>&1; then
    printf '%s\n' 'fault injection check: NATS outage was detected and the existing API connection recovered'
    exit 0
  fi
  attempt=$((attempt + 1))
  sleep 1
done
printf '%s\n' 'fault check failed: API did not recover after NATS restarted' >&2
exit 1
