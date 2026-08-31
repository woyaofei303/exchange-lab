#!/bin/sh
set -eu

script_dir="$(CDPATH= cd "$(dirname "$0")" && pwd)"
project_dir="$(dirname "$script_dir")"
artifact_dir="$project_dir/output-tdd/postgres/backup-restore-check"
backup_file="$artifact_dir/exchange.dump"
restore_database="exchange_restore_check_$$"
restore_created=0

case "$restore_database" in
	exchange_restore_check_[0-9]*) ;;
	*)
		printf '%s\n' 'backup restore check failed: unsafe restore database name' >&2
		exit 1
		;;
esac

cleanup() {
	status=$?
	trap - EXIT HUP INT TERM
	if [ "$restore_created" -eq 1 ]; then
		docker compose exec -T postgres dropdb --if-exists --force -U "$postgres_user" "$restore_database" >/dev/null 2>&1 || true
	fi
	rm -f "$backup_file"
	exit "$status"
}

trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

mkdir -p "$artifact_dir"

postgres_user="$(docker compose exec -T postgres sh -c 'test -n "${POSTGRES_USER:-}" && printf "%s" "$POSTGRES_USER"')"
postgres_database="$(docker compose exec -T postgres sh -c 'test -n "${POSTGRES_DB:-}" && printf "%s" "$POSTGRES_DB"')"
if [ -z "$postgres_user" ] || [ -z "$postgres_database" ]; then
	printf '%s\n' 'backup restore check failed: PostgreSQL identity is unavailable' >&2
	exit 1
fi

docker compose exec -T postgres pg_dump \
	--format=custom \
	--no-owner \
	--no-privileges \
	-U "$postgres_user" \
	-d "$postgres_database" >"$backup_file"

docker compose exec -T postgres createdb -U "$postgres_user" "$restore_database"
restore_created=1
docker compose exec -T postgres pg_restore \
	--exit-on-error \
	--no-owner \
	--no-privileges \
	-U "$postgres_user" \
	-d "$restore_database" <"$backup_file"

schema_version="$(docker compose exec -T postgres psql -U "$postgres_user" -d "$restore_database" -Atc \
	"SELECT COALESCE(MAX(version_id) FILTER (WHERE is_applied), 0) FROM schema_migrations;")"
if [ "$schema_version" != 16 ]; then
	printf '%s\n' 'backup restore check failed: restored migration version mismatch' >&2
	exit 1
fi

instrument_count="$(docker compose exec -T postgres psql -U "$postgres_user" -d "$restore_database" -Atc \
	"SELECT count(*) FROM instruments WHERE symbol = 'BTCUSDT-PERP';")"
if [ "$instrument_count" != 1 ]; then
	printf '%s\n' 'backup restore check failed: restored instrument mismatch' >&2
	exit 1
fi

table_count="$(docker compose exec -T postgres psql -U "$postgres_user" -d "$restore_database" -Atc \
	"SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_name IN ('users', 'user_sessions', 'instruments', 'candles', 'market_trades', 'audit_logs', 'ledger_accounts', 'balances', 'ledger_transactions', 'ledger_entries', 'faucet_requests', 'orders', 'order_events', 'idempotency_keys', 'outbox_events', 'processed_events', 'event_receipts', 'parked_events', 'matching_book_orders', 'matching_offsets', 'contract_trades', 'matching_events', 'contracts', 'leverage_tiers', 'price_sources', 'index_prices', 'mark_prices', 'positions', 'position_events', 'position_snapshots', 'trade_settlements', 'margin_accounts', 'leverage_settings', 'position_margins', 'risk_limits', 'margin_adjustments', 'cross_risk_snapshots', 'liquidation_jobs', 'liquidation_orders', 'liquidation_events', 'premium_indexes', 'funding_rates', 'funding_settlements', 'insurance_fund_entries', 'adl_candidates', 'adl_events', 'conditional_orders', 'ai_runs', 'tool_audit_logs', 'schema_migrations');")"
if [ "$table_count" != 50 ]; then
	printf '%s\n' 'backup restore check failed: restored table set mismatch' >&2
	exit 1
fi

printf '%s\n' 'backup restore check: migration v16 and all 50 schema tables verified in an independent database'
