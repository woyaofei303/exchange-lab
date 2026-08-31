#!/bin/sh
set -eu

project_dir="${PROJECT_DIR:-/opt/exchange-lab}"
environment="${DEPLOY_ENV:-}"
case "$environment" in staging|production) ;; *) printf '%s\n' 'DEPLOY_ENV must be staging or production' >&2; exit 2 ;; esac
if [ "${CONFIRM_ENV:-}" != "$environment" ]; then
	printf '%s\n' 'CONFIRM_ENV must exactly match DEPLOY_ENV' >&2
	exit 2
fi
cd "$project_dir"
previous_tag="${ROLLBACK_IMAGE_TAG:-$(cat .release-previous 2>/dev/null || true)}"
case "$previous_tag" in *[!0-9a-f]*|'') printf '%s\n' 'a hexadecimal ROLLBACK_IMAGE_TAG is required' >&2; exit 2 ;; esac
if [ "$environment" = production ] && [ "${APPROVE_ROLLBACK:-}" != "$previous_tag" ]; then
	printf '%s\n' 'APPROVE_ROLLBACK must equal ROLLBACK_IMAGE_TAG for production' >&2
	exit 2
fi
export IMAGE_TAG="$previous_tag"
compose="docker compose --env-file .env.prod -f compose.yaml -f compose.observability.yaml -f compose.prod.yaml"
$compose pull
$compose up -d --no-deps --wait --wait-timeout 180 api web market-simulator market-worker candle-worker event-worker price-worker risk-worker funding-worker caddy
BASE_URL="https://${EXCHANGE_DOMAIN}" ./deploy/smoke.sh
printf '%s\n' "IMAGE_TAG=$previous_tag" >.release-current
printf '%s\n' "rollback complete: $environment $previous_tag"
