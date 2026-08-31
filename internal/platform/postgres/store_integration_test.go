//go:build integration

package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/woyaofei303/exchange-lab/internal/candle"
	"github.com/woyaofei303/exchange-lab/internal/eventing"
	"github.com/woyaofei303/exchange-lab/internal/identity"
	"github.com/woyaofei303/exchange-lab/internal/ledger"
	domainorder "github.com/woyaofei303/exchange-lab/internal/order"
	store "github.com/woyaofei303/exchange-lab/internal/platform/postgres"
	"github.com/woyaofei303/exchange-lab/internal/trading"
	"github.com/woyaofei303/exchange-lab/migrations"
)

func TestPostgresMigrationsRepositoriesAndRollback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	container, err := tcpostgres.Run(ctx,
		"postgres:18.4-alpine",
		tcpostgres.WithDatabase("exchange_test"),
		tcpostgres.WithUsername("exchange_test"),
		tcpostgres.WithPassword("test-only-password"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start PostgreSQL container: %v", err)
	}
	testcontainers.CleanupContainer(t, container)

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("PostgreSQL connection string: %v", err)
	}
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open migration database: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	migrator, err := store.NewMigrator(sqlDB)
	if err != nil {
		t.Fatalf("NewMigrator() error = %v", err)
	}
	if err := migrator.Up(ctx); err != nil {
		t.Fatalf("first migration up: %v", err)
	}
	if err := migrator.Up(ctx); err != nil {
		t.Fatalf("idempotent migration up: %v", err)
	}
	assertTables(t, ctx, sqlDB, true)
	for _, table := range []string{"contracts", "positions", "margin_accounts", "cross_risk_snapshots", "liquidation_jobs", "funding_rates", "insurance_fund_entries", "ai_runs", "tool_audit_logs"} {
		assertTable(t, ctx, sqlDB, table, true)
	}

	introducedByVersion := []string{
		"",
		"users",
		"market_trades",
		"user_sessions",
		"ledger_accounts",
		"orders",
		"processed_events",
		"matching_book_orders",
		"contracts",
		"positions",
		"margin_accounts",
		"cross_risk_snapshots",
		"liquidation_jobs",
		"funding_rates",
		"insurance_fund_entries",
		"ai_runs",
		"tool_audit_logs",
	}
	for version := int(migrations.CurrentVersion); version >= 1; version-- {
		if err := migrator.Down(ctx); err != nil {
			t.Fatalf("migration v%d down: %v", version, err)
		}
		assertTable(t, ctx, sqlDB, introducedByVersion[version], false)
		gotVersion, err := migrator.Version(ctx)
		if err != nil || gotVersion != int64(version-1) {
			t.Fatalf("version after v%d down = %d, %v", version, gotVersion, err)
		}
	}
	assertTables(t, ctx, sqlDB, false)
	if err := migrator.Up(ctx); err != nil {
		t.Fatalf("migration up after down: %v", err)
	}

	assertTables(t, ctx, sqlDB, true)

	pool, err := store.OpenPool(ctx, dsn, store.PoolConfig{
		MinConns:       1,
		MaxConns:       4,
		ConnectTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("OpenPool() error = %v", err)
	}
	t.Cleanup(pool.Close)
	if got, want := pool.Config().MaxConns, int32(4); got != want {
		t.Fatalf("pool MaxConns = %d, want %d", got, want)
	}
	schemaReadiness := store.NewSchemaChecker(pool, migrations.CurrentVersion)
	if err := schemaReadiness.Check(ctx); err != nil {
		t.Fatalf("schema readiness after up: %v", err)
	}
	if err := migrator.Down(ctx); err != nil {
		t.Fatalf("migration down before readiness failure check: %v", err)
	}
	if err := schemaReadiness.Check(ctx); err == nil {
		t.Fatal("schema readiness error = nil after migration down")
	}
	if err := migrator.Up(ctx); err != nil {
		t.Fatalf("migration up after readiness failure check: %v", err)
	}

	identityRepository := store.NewIdentityRepository(pool, 2*time.Second)
	identityService, err := identity.NewService(identityRepository, identity.DefaultPasswordHasher(), identity.TokenIssuer{}, time.Hour, time.Now)
	if err != nil {
		t.Fatalf("NewService(identity) error = %v", err)
	}
	registered, err := identityService.Register(ctx, "integration@example.test", "correct horse battery staple")
	if err != nil || registered.Role != identity.RoleUser {
		t.Fatalf("Register(identity) = %#v, %v", registered, err)
	}
	token, _, err := identityService.Login(ctx, registered.Email, "correct horse battery staple")
	if err != nil {
		t.Fatalf("Login(identity) error = %v", err)
	}
	rotatedToken, _, err := identityService.Refresh(ctx, token)
	if err != nil {
		t.Fatalf("Refresh(identity) error = %v", err)
	}
	if _, err := identityService.Authenticate(ctx, token); !errors.Is(err, identity.ErrUnauthenticated) {
		t.Fatalf("Authenticate(old identity token) error = %v", err)
	}
	if err := identityService.Logout(ctx, rotatedToken); err != nil {
		t.Fatalf("Logout(identity) error = %v", err)
	}
	if _, err := identityService.Authenticate(ctx, rotatedToken); !errors.Is(err, identity.ErrUnauthenticated) {
		t.Fatalf("Authenticate(revoked identity token) error = %v", err)
	}
	ledgerRepository := store.NewLedgerRepository(pool, 2*time.Second)
	faucet, err := ledgerRepository.Faucet(ctx, registered.ID, 5_000_000_000, "integration-faucet-0001")
	if err != nil || faucet.Replayed || faucet.Balance.AvailableAtoms != 5_000_000_000 {
		t.Fatalf("Faucet() = %#v, %v", faucet, err)
	}
	replayed, err := ledgerRepository.Faucet(ctx, registered.ID, 5_000_000_000, "integration-faucet-0001")
	if err != nil || !replayed.Replayed || replayed.TransactionID != faucet.TransactionID || replayed.Balance.AvailableAtoms != faucet.Balance.AvailableAtoms {
		t.Fatalf("Faucet(replay) = %#v, %v", replayed, err)
	}
	if _, err := ledgerRepository.Faucet(ctx, registered.ID, 1, "integration-faucet-0001"); !errors.Is(err, ledger.ErrConflict) {
		t.Fatalf("Faucet(conflicting replay) error = %v", err)
	}
	if err := ledgerRepository.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile(ledger) error = %v", err)
	}
	orderRepository := store.NewOrderRepository(pool, 2*time.Second)
	orderService, err := domainorder.NewService(store.NewInstrumentRepository(pool, 2*time.Second), orderRepository, time.Now)
	if err != nil {
		t.Fatalf("NewService(order) error = %v", err)
	}
	command := domainorder.PlaceCommand{Symbol: "BTCUSDT-PERP", Side: "buy", Price: "50000.00", Quantity: "0.001", Leverage: 10, ClientOrderID: "integration-order-0001", IdempotencyKey: "integration-place-0001"}
	placed, err := orderService.Place(ctx, registered.ID, command)
	if err != nil || placed.Replayed || placed.Order.Status != trading.OrderStatusOpen || placed.Order.ReservedMarginAtoms != 500_000_000 {
		t.Fatalf("Place(order) = %#v, %v", placed, err)
	}
	replayedOrder, err := orderService.Place(ctx, registered.ID, command)
	if err != nil || !replayedOrder.Replayed || replayedOrder.Order.ID != placed.Order.ID {
		t.Fatalf("Place(order replay) = %#v, %v", replayedOrder, err)
	}
	conflicting := command
	conflicting.Price = "50000.10"
	if _, err := orderService.Place(ctx, registered.ID, conflicting); !errors.Is(err, domainorder.ErrConflict) {
		t.Fatalf("Place(order conflict) error = %v", err)
	}
	reservedBalance, err := ledgerRepository.Balance(ctx, registered.ID)
	if err != nil || reservedBalance.AvailableAtoms != 4_500_000_000 || reservedBalance.LockedAtoms != 500_000_000 {
		t.Fatalf("Balance(after order) = %#v, %v", reservedBalance, err)
	}
	canceledOrder, err := orderService.Cancel(ctx, registered.ID, placed.Order.ID, "integration-cancel-0001")
	if err != nil || canceledOrder.Order.Status != trading.OrderStatusCanceled || canceledOrder.Order.ReservedMarginAtoms != 0 {
		t.Fatalf("Cancel(order) = %#v, %v", canceledOrder, err)
	}
	cancelReplay, err := orderService.Cancel(ctx, registered.ID, placed.Order.ID, "integration-cancel-0001")
	if err != nil || !cancelReplay.Replayed || cancelReplay.Order.ID != placed.Order.ID {
		t.Fatalf("Cancel(order replay) = %#v, %v", cancelReplay, err)
	}
	releasedBalance, err := ledgerRepository.Balance(ctx, registered.ID)
	if err != nil || releasedBalance.AvailableAtoms != 5_000_000_000 || releasedBalance.LockedAtoms != 0 {
		t.Fatalf("Balance(after cancel) = %#v, %v", releasedBalance, err)
	}
	if err := ledgerRepository.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile(after order) error = %v", err)
	}
	eventRepository := store.NewEventRepository(pool, 2*time.Second)
	outbox, err := eventRepository.Claim(ctx, 10, time.Minute)
	if err != nil || len(outbox) != 2 {
		t.Fatalf("Claim(outbox) = %#v, %v", outbox, err)
	}
	if outbox[0].Subject != "orders.accepted" || outbox[1].Subject != "orders.cancel_requested" {
		t.Fatalf("outbox order = %#v", outbox)
	}
	payload, err := eventing.Encode(outbox[0])
	if err != nil {
		t.Fatalf("Encode(outbox) error = %v", err)
	}
	envelope, err := eventing.Decode(payload)
	if err != nil {
		t.Fatalf("Decode(outbox) error = %v", err)
	}
	matchingRepository := store.NewMatchingRepository(pool, 2*time.Second)
	tradeCount, processed, err := matchingRepository.Process(ctx, envelope, 1, time.Now())
	if err != nil || !processed || tradeCount != 0 {
		t.Fatalf("Process(matching accepted) = trades=%d processed=%t error=%v", tradeCount, processed, err)
	}
	if _, processed, err := matchingRepository.Process(ctx, envelope, 1, time.Now()); err != nil || processed {
		t.Fatalf("Process(matching duplicate) = processed=%t error=%v", processed, err)
	}
	cancelPayload, err := eventing.Encode(outbox[1])
	if err != nil {
		t.Fatalf("Encode(cancel outbox) error = %v", err)
	}
	cancelEnvelope, err := eventing.Decode(cancelPayload)
	if err != nil {
		t.Fatalf("Decode(cancel outbox) error = %v", err)
	}
	if _, processed, err := matchingRepository.Process(ctx, cancelEnvelope, 2, time.Now()); err != nil || !processed {
		t.Fatalf("Process(matching cancel) = processed=%t error=%v", processed, err)
	}
	var restingOrders int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM matching_book_orders`).Scan(&restingOrders); err != nil || restingOrders != 0 {
		t.Fatalf("matching book after cancel = %d, %v", restingOrders, err)
	}
	firstReceipt, err := eventRepository.RecordReceipt(ctx, eventing.AuditConsumer, envelope, time.Now())
	if err != nil || !firstReceipt {
		t.Fatalf("RecordReceipt(first) = %t, %v", firstReceipt, err)
	}
	duplicateReceipt, err := eventRepository.RecordReceipt(ctx, eventing.AuditConsumer, envelope, time.Now())
	if err != nil || duplicateReceipt {
		t.Fatalf("RecordReceipt(duplicate) = %t, %v", duplicateReceipt, err)
	}
	if err := eventRepository.MarkPublished(ctx, outbox[0].ID, time.Now()); err != nil {
		t.Fatalf("MarkPublished(outbox) error = %v", err)
	}
	if err := eventRepository.MarkFailed(ctx, outbox[1], "integration_failure", time.Now()); err != nil {
		t.Fatalf("MarkFailed(outbox) error = %v", err)
	}

	candleRepository := store.NewCandleRepository(pool, 2*time.Second)
	minute := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	tradeEvents := []candle.Trade{
		{StreamID: "integration-stream", TradeID: "middle", Symbol: "BTCUSDT-PERP", Sequence: 2, EventTime: minute.Add(30 * time.Second), PriceAtoms: 5_001_000, PriceScale: 2, QuantityAtoms: 2, QuantityScale: 3},
		{StreamID: "integration-stream", TradeID: "last", Symbol: "BTCUSDT-PERP", Sequence: 3, EventTime: minute.Add(50 * time.Second), PriceAtoms: 4_999_000, PriceScale: 2, QuantityAtoms: 3, QuantityScale: 3},
		{StreamID: "integration-stream", TradeID: "late-first", Symbol: "BTCUSDT-PERP", Sequence: 1, EventTime: minute.Add(10 * time.Second), PriceAtoms: 5_000_000, PriceScale: 2, QuantityAtoms: 4, QuantityScale: 3},
	}
	var stored candle.Candle
	for _, tradeEvent := range tradeEvents {
		var accepted bool
		stored, accepted, err = candleRepository.ApplyTrade(ctx, tradeEvent)
		if err != nil || !accepted {
			t.Fatalf("ApplyTrade(%s) = %#v, accepted=%t, error=%v", tradeEvent.TradeID, stored, accepted, err)
		}
	}
	duplicate, accepted, err := candleRepository.ApplyTrade(ctx, tradeEvents[0])
	if err != nil || accepted || duplicate.VolumeAtoms != 9 || duplicate.TradeCount != 3 {
		t.Fatalf("duplicate ApplyTrade() = %#v, accepted=%t, error=%v", duplicate, accepted, err)
	}
	if duplicate.OpenAtoms != 5_000_000 || duplicate.HighAtoms != 5_001_000 || duplicate.LowAtoms != 4_999_000 || duplicate.CloseAtoms != 4_999_000 {
		t.Fatalf("stored OHLC = %#v", duplicate)
	}
	listed, err := candleRepository.List(ctx, store.CandleQuery{Symbol: "BTCUSDT-PERP", Interval: candle.OneMinute, Start: minute, End: minute.Add(time.Minute), Limit: 100})
	if err != nil || len(listed) != 1 || listed[0].VolumeAtoms != 9 {
		t.Fatalf("List(candles) = %#v, %v", listed, err)
	}
	finalized, err := candleRepository.FinalizeBefore(ctx, "BTCUSDT-PERP", minute.Add(time.Minute))
	if err != nil || len(finalized) != 1 || !finalized[0].Final {
		t.Fatalf("FinalizeBefore() = %#v, %v", finalized, err)
	}
	rollup, err := candle.Rollup([]candle.Candle{finalized[0]}, candle.FiveMinutes)
	if err != nil {
		t.Fatal(err)
	}
	if err := candleRepository.Upsert(ctx, rollup); err != nil {
		t.Fatalf("Upsert(rollup) error = %v", err)
	}
	higher, err := candleRepository.List(ctx, store.CandleQuery{Symbol: "BTCUSDT-PERP", Interval: candle.FiveMinutes, Start: minute, End: minute.Add(5 * time.Minute), Limit: 100})
	if err != nil || len(higher) != 1 || higher[0].Interval != candle.FiveMinutes {
		t.Fatalf("List(5m) = %#v, %v", higher, err)
	}

	repository := store.NewInstrumentRepository(pool, 2*time.Second)
	instruments, err := repository.List(ctx, "")
	if err != nil {
		t.Fatalf("List() seeded instruments error = %v", err)
	}
	if got, want := len(instruments), 1; got != want || instruments[0].Symbol() != "BTCUSDT-PERP" {
		t.Fatalf("seeded instruments = %#v", instruments)
	}

	eth := mustInstrument(t, "ETH")
	if err := repository.Save(ctx, eth); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	filtered, err := repository.List(ctx, "ETHUSDT-PERP")
	if err != nil {
		t.Fatalf("List() filtered error = %v", err)
	}
	if got, want := len(filtered), 1; got != want || filtered[0].Symbol() != "ETHUSDT-PERP" {
		t.Fatalf("filtered instruments = %#v", filtered)
	}
	if got := filtered[0]; got.PriceStep().String() != "0.10" || got.QuantityStep().String() != "0.001" || got.MinNotional().Decimal() != "5.00000000" {
		t.Fatalf("round-trip filters = price %s quantity %s minimum %s", got.PriceStep(), got.QuantityStep(), got.MinNotional().Decimal())
	}

	canceled, cancelQuery := context.WithCancel(ctx)
	cancelQuery()
	if _, err := repository.List(canceled, ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("List() canceled error = %v, want context.Canceled", err)
	}

	transactor := store.NewTransactor(pool, 2*time.Second)
	rollbackCause := errors.New("force rollback")
	err = transactor.WithinTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable}, func(txCtx context.Context, tx pgx.Tx) error {
		_, execErr := tx.Exec(txCtx, `
			INSERT INTO users (email, password_hash)
			VALUES ('rollback@example.test', 'test-hash')
		`)
		if execErr != nil {
			return execErr
		}
		return rollbackCause
	})
	if !errors.Is(err, rollbackCause) {
		t.Fatalf("WithinTx() error = %v, want rollback cause", err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE email = 'rollback@example.test'`).Scan(&count); err != nil {
		t.Fatalf("count rolled-back user: %v", err)
	}
	if count != 0 {
		t.Fatalf("rolled-back user count = %d, want 0", count)
	}
}

func assertTables(t *testing.T, ctx context.Context, db *sql.DB, wantApplicationTables bool) {
	t.Helper()

	for _, table := range []string{"users", "instruments", "candles", "audit_logs"} {
		var exists bool
		if err := db.QueryRowContext(ctx, `SELECT to_regclass('public.' || $1) IS NOT NULL`, table).Scan(&exists); err != nil {
			t.Fatalf("check table %s: %v", table, err)
		}
		if exists != wantApplicationTables {
			t.Fatalf("table %s exists = %t, want %t", table, exists, wantApplicationTables)
		}
	}
	var migrationsTable bool
	if err := db.QueryRowContext(ctx, `SELECT to_regclass('public.schema_migrations') IS NOT NULL`).Scan(&migrationsTable); err != nil {
		t.Fatalf("check schema_migrations: %v", err)
	}
	if !migrationsTable {
		t.Fatal("schema_migrations table does not exist")
	}
}

func assertTable(t *testing.T, ctx context.Context, db *sql.DB, table string, want bool) {
	t.Helper()
	var exists bool
	if err := db.QueryRowContext(ctx, `SELECT to_regclass('public.' || $1) IS NOT NULL`, table).Scan(&exists); err != nil {
		t.Fatalf("check table %s: %v", table, err)
	}
	if exists != want {
		t.Fatalf("table %s exists = %t, want %t", table, exists, want)
	}
}

func mustInstrument(t *testing.T, baseCode string) trading.Instrument {
	t.Helper()

	base, err := trading.NewAsset(baseCode, 8)
	if err != nil {
		t.Fatalf("NewAsset() base error = %v", err)
	}
	quote, err := trading.NewAsset("USDT", 8)
	if err != nil {
		t.Fatalf("NewAsset() quote error = %v", err)
	}
	instrument, err := trading.NewInstrument(trading.InstrumentSpec{
		Base:          base,
		Quote:         quote,
		PriceScale:    2,
		QuantityScale: 3,
		PriceStep:     "0.10",
		QuantityStep:  "0.001",
		MinNotional:   "5",
	})
	if err != nil {
		t.Fatalf("NewInstrument() error = %v", err)
	}
	return instrument
}
