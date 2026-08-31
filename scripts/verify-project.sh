#!/bin/sh
set -eu

script_dir="$(CDPATH= cd "$(dirname "$0")" && pwd)"
project_dir="$(dirname "$script_dir")"
temporary_dir="$(mktemp -d "${TMPDIR:-/tmp}/exchange-project-check.XXXXXX")"
failures=0

cleanup() {
	status=$?
	trap - EXIT HUP INT TERM
	rm -rf "$temporary_dir"
	exit "$status"
}

trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

compose_file="$temporary_dir/compose.json"
if ! docker compose --project-directory "$project_dir" -f "$project_dir/compose.yaml" config --format json >"$compose_file"; then
	printf '%s\n' 'project check failed: Compose configuration could not be rendered' >&2
	exit 1
fi

if ! (cd "$project_dir" && go run ./scripts/composeverify -config "$compose_file"); then
	printf '%s\n' 'project check failed: rendered Compose contract mismatch' >&2
	failures=$((failures + 1))
fi

override_file="$temporary_dir/compose-override.json"
if ! POSTGRES_USER=week_one_probe_user \
	POSTGRES_DB=week_one_probe_db \
	POSTGRES_PASSWORD='reserved@probe' \
	POSTGRES_DSN='postgres://week_one_probe_user:reserved%40probe@postgres:5432/week_one_probe_db?sslmode=disable' \
	docker compose --project-directory "$project_dir" -f "$project_dir/compose.yaml" config --format json >"$override_file"; then
	printf '%s\n' 'project check failed: overridden Compose configuration could not be rendered' >&2
	exit 1
fi
if ! (cd "$project_dir" && go run ./scripts/composeverify \
	-config "$override_file" \
	-postgres-user week_one_probe_user \
	-postgres-database week_one_probe_db \
	-postgres-password 'reserved@probe' \
	-postgres-dsn 'postgres://week_one_probe_user:reserved%40probe@postgres:5432/week_one_probe_db?sslmode=disable'); then
	printf '%s\n' 'project check failed: canonical PostgreSQL override contract mismatch' >&2
	failures=$((failures + 1))
fi

context_dir="$temporary_dir/context"
mkdir -p "$context_dir/cmd/api" "$context_dir/internal/app" "$context_dir/internal/testdata" \
	"$context_dir/internal/state" "$context_dir/bin" "$context_dir/scripts" "$context_dir/.git"
cp "$project_dir/.dockerignore" "$context_dir/.dockerignore"
printf '%s\n' 'module context-probe' >"$context_dir/go.mod"
: >"$context_dir/go.sum"
printf '%s\n' 'package main' >"$context_dir/cmd/api/main.go"
printf '%s\n' 'package app' >"$context_dir/internal/app/run.go"
printf '%s\n' 'probe' >"$context_dir/.env"
printf '%s\n' 'probe' >"$context_dir/bin/api"
printf '%s\n' 'probe' >"$context_dir/README.md"
printf '%s\n' 'probe' >"$context_dir/scripts/probe.sh"
printf '%s\n' 'probe' >"$context_dir/.git/config"
printf '%s\n' 'probe' >"$context_dir/unknown-credential.key"
printf '%s\n' 'probe' >"$context_dir/internal/testdata/private.key"
printf '%s\n' 'probe' >"$context_dir/cmd/api/local.pem"
printf '%s\n' 'probe' >"$context_dir/internal/state/debug.dump"

write_probe_dockerfile() {
	probe_path=$1
	{
		printf '%s\n' 'FROM scratch'
		printf 'COPY %s /probe\n' "$probe_path"
	} >"$context_dir/Dockerfile"
}

{
	printf '%s\n' 'FROM scratch'
	printf '%s\n' 'COPY go.mod /go.mod'
	printf '%s\n' 'COPY cmd/api/main.go /cmd/api/main.go'
	printf '%s\n' 'COPY internal/app/run.go /internal/app/run.go'
} >"$context_dir/Dockerfile"
if ! docker build --quiet -f "$context_dir/Dockerfile" "$context_dir" >"$temporary_dir/docker-control.log" 2>&1; then
	printf '%s\n' 'project check failed: Docker context control build failed' >&2
	exit 1
fi

for excluded_path in .env bin README.md scripts/probe.sh .git unknown-credential.key \
	internal/testdata/private.key cmd/api/local.pem internal/state/debug.dump; do
	write_probe_dockerfile "$excluded_path"
	if docker build --quiet -f "$context_dir/Dockerfile" "$context_dir" >"$temporary_dir/docker-probe.log" 2>&1; then
		printf '%s\n' "project check failed: Docker context included denied path: $excluded_path" >&2
		failures=$((failures + 1))
	fi
done

if [ "$failures" -ne 0 ]; then
	exit 1
fi
