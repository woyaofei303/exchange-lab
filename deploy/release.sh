#!/bin/sh
set -eu

project_dir="${PROJECT_DIR:-/opt/exchange-lab}"
environment="${DEPLOY_ENV:-}"
case "$environment" in staging|production) ;; *) printf '%s\n' 'DEPLOY_ENV must be staging or production' >&2; exit 2 ;; esac
if [ "${CONFIRM_ENV:-}" != "$environment" ]; then
	printf '%s\n' 'CONFIRM_ENV must exactly match DEPLOY_ENV' >&2
	exit 2
fi
case "${IMAGE_TAG:-}" in
	*[!0-9a-f]*|'') printf '%s\n' 'IMAGE_TAG must be a lowercase hexadecimal commit SHA' >&2; exit 2 ;;
esac
if [ "${#IMAGE_TAG}" -lt 12 ]; then
	printf '%s\n' 'IMAGE_TAG must contain at least 12 commit SHA characters' >&2
	exit 2
fi
if [ "$environment" = production ] && [ "${APPROVE_RELEASE:-}" != "$IMAGE_TAG" ]; then
	printf '%s\n' 'APPROVE_RELEASE must equal IMAGE_TAG for production' >&2
	exit 2
fi

cd "$project_dir"
compose="docker compose --env-file .env.prod -f compose.yaml -f compose.observability.yaml -f compose.prod.yaml"
previous_tag="$(sed -n 's/^IMAGE_TAG=//p' .release-current 2>/dev/null || true)"
BACKUP_DIR="${BACKUP_DIR:-/var/backups/exchange-lab}" ./scripts/backup.sh
$compose pull
$compose run --rm migrate
$compose up -d --wait --wait-timeout 180
BASE_URL="https://${EXCHANGE_DOMAIN}" ./deploy/smoke.sh
printf '%s\n' "$previous_tag" >.release-previous
printf '%s\n' "IMAGE_TAG=$IMAGE_TAG" >.release-current
printf '%s\n' "release complete: $environment $IMAGE_TAG"
