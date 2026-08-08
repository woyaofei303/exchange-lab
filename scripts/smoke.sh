#!/bin/sh
set -eu

base_url="${BASE_URL:-http://127.0.0.1:8080}"
health="$(curl --fail --silent "$base_url/healthz")"
ready="$(curl --fail --silent "$base_url/readyz")"

test "$health" = '{"status":"ok"}'
printf '%s' "$ready" | grep -q '"status":"ready"'
printf '%s' "$ready" | grep -q '"postgres":"ok"'
printf '%s' "$ready" | grep -q '"redis":"ok"'
printf '%s' "$ready" | grep -q '"nats":"ok"'
