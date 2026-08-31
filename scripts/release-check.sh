#!/bin/sh
set -eu

script_dir="$(CDPATH= cd "$(dirname "$0")" && pwd)"
project_dir="$(dirname "$script_dir")"

sh -n "$project_dir/deploy/release.sh" "$project_dir/deploy/rollback.sh"
IMAGE_TAG=0123456789abcdef0123456789abcdef01234567 \
EXCHANGE_DOMAIN=exchange.example.test \
ACME_EMAIL=ops@example.test \
docker compose -f "$project_dir/compose.yaml" -f "$project_dir/compose.observability.yaml" -f "$project_dir/compose.prod.yaml" config --quiet
printf '%s\n' 'release check: production Compose, release and rollback syntax verified'
