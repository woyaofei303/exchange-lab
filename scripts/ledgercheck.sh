#!/bin/sh
set -eu

base_url="${BASE_URL:-http://127.0.0.1:8080}"
cookie_file="$(mktemp "${TMPDIR:-/tmp}/exchange-ledger-cookie.XXXXXX")"
body_file="$(mktemp "${TMPDIR:-/tmp}/exchange-ledger-body.XXXXXX")"
email="ledger-check-$(date +%s)-$$@example.test"
password='local ledger acceptance password 2026'
key="ledger-check-$(date +%s)-$$"

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

curl --fail --silent -H 'Content-Type: application/json' \
	--data "{\"email\":\"$email\",\"password\":\"$password\"}" \
	"$base_url/v1/auth/register" >/dev/null
curl --fail --silent --cookie-jar "$cookie_file" -H 'Content-Type: application/json' \
	--data "{\"email\":\"$email\",\"password\":\"$password\"}" \
	"$base_url/v1/auth/login" >/dev/null

credit() {
	curl --fail --silent --cookie "$cookie_file" -H 'Content-Type: application/json' \
		-H "Idempotency-Key: $key" --data '{"amount":"1000.00000000"}' \
		"$base_url/v1/faucet"
}

first="$(credit)"
second="$(credit)"
printf '%s' "$first" | grep -q '"available":"1000.00000000"'
printf '%s' "$first" | grep -q '"replayed":false'
printf '%s' "$second" | grep -q '"replayed":true'
first_id="$(printf '%s' "$first" | sed -n 's/.*"transaction_id":"\([^"]*\)".*/\1/p')"
second_id="$(printf '%s' "$second" | sed -n 's/.*"transaction_id":"\([^"]*\)".*/\1/p')"
[ -n "$first_id" ] && [ "$first_id" = "$second_id" ]

curl --fail --silent --cookie "$cookie_file" "$base_url/v1/balances" >"$body_file"
grep -q '"available":"1000.00000000"' "$body_file"

postgres_user="$(docker compose exec -T postgres sh -c 'printf "%s" "$POSTGRES_USER"')"
postgres_database="$(docker compose exec -T postgres sh -c 'printf "%s" "$POSTGRES_DB"')"
broken="$(docker compose exec -T postgres psql -U "$postgres_user" -d "$postgres_database" -Atc \
	"SELECT count(*) FROM (SELECT transaction_id FROM ledger_entries GROUP BY transaction_id HAVING count(*) < 2 OR sum(amount_atoms) <> 0) invalid;")"
[ "$broken" = 0 ]

printf '%s\n' 'ledger check: authenticated credit, decimal transport, idempotent replay and zero-sum reconciliation verified'
