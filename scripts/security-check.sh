#!/bin/sh
set -eu

script_dir="$(CDPATH= cd "$(dirname "$0")" && pwd)"
project_dir="$(dirname "$script_dir")"
artifact_dir="$project_dir/output-tdd/security"
rendered="$artifact_dir/compose.prod.json"
mkdir -p "$artifact_dir"

IMAGE_TAG=0123456789abcdef0123456789abcdef01234567 \
EXCHANGE_DOMAIN=exchange.example.test \
ACME_EMAIL=ops@example.test \
docker compose -f "$project_dir/compose.yaml" -f "$project_dir/compose.observability.yaml" -f "$project_dir/compose.prod.yaml" \
	config --format json >"$rendered"

if jq -e '[.services | to_entries[] | select(.key != "caddy") | select((.value.ports // []) | length > 0)] | length != 0' "$rendered" >/dev/null; then
	printf '%s\n' 'security check failed: a non-Caddy service publishes a host port' >&2
	exit 1
fi
if [ "$(jq -r '[.services.caddy.ports[].published] | sort | join(",")' "$rendered")" != "443,443,80" ]; then
	printf '%s\n' 'security check failed: Caddy public port contract mismatch' >&2
	exit 1
fi
if ! jq -e '.networks.exchange.internal == true' "$rendered" >/dev/null; then
	printf '%s\n' 'security check failed: backend network is not internal' >&2
	exit 1
fi
if ! jq -e '.services.api.environment.POSTGRES_DSN == "" and .services.api.environment.OPENAI_API_KEY == "" and .services.api.environment.POSTGRES_DSN_FILE == "/run/secrets/postgres_dsn" and .services.api.environment.OPENAI_API_KEY_FILE == "/run/secrets/openai_api_key"' "$rendered" >/dev/null; then
	printf '%s\n' 'security check failed: API secret-file contract mismatch' >&2
	exit 1
fi
if ! grep -qx 'USER 10001:10001' "$project_dir/Dockerfile"; then
	printf '%s\n' 'security check failed: application runtime image is not pinned to a non-root UID' >&2
	exit 1
fi
if docker image inspect exchange-lab:local >/dev/null 2>&1; then
	image_user="$(docker image inspect exchange-lab:local --format '{{.Config.User}}')"
	if [ "$image_user" != '10001:10001' ]; then
		printf '%s\n' 'security check failed: built image user mismatch' >&2
		exit 1
	fi
fi

(cd "$project_dir" && go tool govulncheck ./...)
printf '%s\n' 'security check: secret files, public ports, internal network, non-root image and Go vulnerabilities verified'
