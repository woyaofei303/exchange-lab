#!/bin/sh
set -eu

script_dir="$(CDPATH= cd "$(dirname "$0")" && pwd)"
project_dir="$(dirname "$script_dir")"
backup_dir="${BACKUP_DIR:-$project_dir/output-tdd/postgres/backups}"
timestamp="$(date -u '+%Y%m%dT%H%M%SZ')"
target="$backup_dir/exchange-$timestamp.dump"
temporary="$target.partial"

cleanup() {
	status=$?
	trap - EXIT HUP INT TERM
	rm -f "$temporary"
	exit "$status"
}
trap cleanup EXIT HUP INT TERM

mkdir -p "$backup_dir"
postgres_user="$(docker compose exec -T postgres sh -c 'test -n "${POSTGRES_USER:-}" && printf "%s" "$POSTGRES_USER"')"
postgres_database="$(docker compose exec -T postgres sh -c 'test -n "${POSTGRES_DB:-}" && printf "%s" "$POSTGRES_DB"')"
docker compose exec -T postgres pg_dump --format=custom --no-owner --no-privileges \
	-U "$postgres_user" -d "$postgres_database" >"$temporary"
test -s "$temporary"
mv "$temporary" "$target"
printf '%s\n' "$target"
