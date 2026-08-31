#!/bin/sh
set -eu

docker_host="${DOCKER_HOST:-}"
if [ -z "$docker_host" ]; then
	active_context="$(docker context show)"
	docker_host="$(docker context inspect --format '{{.Endpoints.docker.Host}}' "$active_context")"
fi
if [ -z "$docker_host" ]; then
	printf '%s\n' 'integration failed: Docker host is unavailable' >&2
	exit 1
fi
export DOCKER_HOST="$docker_host"

case "$docker_host" in
	*'/.colima/'*)
		if [ -z "${TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE:-}" ]; then
			export TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock
		fi
		;;
esac

go test -count=1 -race -tags=integration \
	./internal/health \
	./internal/platform/postgres \
	./internal/platform/rediscache
