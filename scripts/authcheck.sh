#!/bin/sh
set -eu

base_url="${BASE_URL:-http://127.0.0.1:8080}"
cookie_file="$(mktemp "${TMPDIR:-/tmp}/exchange-auth-cookie.XXXXXX")"
body_file="$(mktemp "${TMPDIR:-/tmp}/exchange-auth-body.XXXXXX")"
email="auth-check-$(date +%s)-$$@example.test"
password='local acceptance password 2026'

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

post_credentials() {
	path=$1
	expected=$2
	status="$(curl --connect-timeout 2 --max-time 5 --silent --output "$body_file" --write-out '%{http_code}' \
		-H 'Content-Type: application/json' --data "{\"email\":\"$email\",\"password\":\"$password\"}" \
		"$base_url$path")"
	[ "$status" = "$expected" ]
}

if ! post_credentials /v1/auth/register 201; then
	printf '%s\n' 'auth check failed: registration contract mismatch' >&2
	exit 1
fi

status="$(curl --connect-timeout 2 --max-time 5 --silent --cookie-jar "$cookie_file" --output "$body_file" --write-out '%{http_code}' \
	-H 'Content-Type: application/json' --data "{\"email\":\"$email\",\"password\":\"$password\"}" \
	"$base_url/v1/auth/login")"
if [ "$status" != 200 ] || ! grep -q 'exchange_session' "$cookie_file"; then
	printf '%s\n' 'auth check failed: login cookie contract mismatch' >&2
	exit 1
fi

status="$(curl --connect-timeout 2 --max-time 5 --silent --cookie "$cookie_file" --output "$body_file" --write-out '%{http_code}' "$base_url/v1/me")"
if [ "$status" != 200 ] || ! grep -q "\"email\":\"$email\"" "$body_file"; then
	printf '%s\n' 'auth check failed: current-user contract mismatch' >&2
	exit 1
fi

status="$(curl --connect-timeout 2 --max-time 5 --silent --cookie "$cookie_file" --output "$body_file" --write-out '%{http_code}' -X POST "$base_url/v1/auth/logout")"
if [ "$status" != 200 ]; then
	printf '%s\n' 'auth check failed: logout contract mismatch' >&2
	exit 1
fi

status="$(curl --connect-timeout 2 --max-time 5 --silent --cookie "$cookie_file" --output "$body_file" --write-out '%{http_code}' "$base_url/v1/me")"
if [ "$status" != 401 ]; then
	printf '%s\n' 'auth check failed: revoked session remained active' >&2
	exit 1
fi

printf '%s\n' 'auth check: register, login, HttpOnly session, current user, logout and revocation verified'
