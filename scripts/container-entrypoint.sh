#!/bin/sh
set -eu

read_secret() {
	name=$1
	path=$2
	if [ -z "$path" ] || [ ! -r "$path" ]; then
		printf '%s\n' "container entrypoint: unreadable secret file for $name" >&2
		exit 1
	fi
	value="$(cat "$path")"
	if [ -z "$value" ]; then
		printf '%s\n' "container entrypoint: empty secret for $name" >&2
		exit 1
	fi
	case "$name" in
		POSTGRES_DSN) POSTGRES_DSN=$value; export POSTGRES_DSN ;;
		OPENAI_API_KEY) OPENAI_API_KEY=$value; export OPENAI_API_KEY ;;
		*) printf '%s\n' 'container entrypoint: unsupported secret name' >&2; exit 1 ;;
	esac
}

if [ -n "${POSTGRES_DSN_FILE:-}" ]; then
	read_secret POSTGRES_DSN "$POSTGRES_DSN_FILE"
fi
if [ -n "${OPENAI_API_KEY_FILE:-}" ]; then
	read_secret OPENAI_API_KEY "$OPENAI_API_KEY_FILE"
fi

if [ "$#" -eq 0 ]; then
	printf '%s\n' 'container entrypoint: command is required' >&2
	exit 1
fi
exec "$@"
