package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/woyaofei303/exchange-lab/internal/eventing"
	"github.com/woyaofei303/exchange-lab/internal/fixedmath"
	"github.com/woyaofei303/exchange-lab/internal/position"
	"github.com/woyaofei303/exchange-lab/internal/trading"
)

// PositionConsumer is the stable processed-event identity for trade settlement.
const PositionConsumer = "position-v1"
const takerFeeBPS int64 = 4

// PositionRepository atomically settles trades into positions, margin and ledger facts.
type PositionRepository struct {
	pool    *pgxpool.Pool
	timeout time.Duration
}

// NewPositionRepository constructs a context-bounded position repository.
func NewPositionRepository(pool *pgxpool.Pool, timeout time.Duration) *PositionRepository {
	return &PositionRepository{pool: pool, timeout: timeout}
}

type executedTrade struct {
	Version      int          `json:"version"`
	TradeID      string       `json:"trade_id"`
	Symbol       string       `json:"symbol"`
	MakerOrderID string       `json:"maker_order_id"`
	TakerOrderID string       `json:"taker_order_id"`
	MakerUserID  int64        `json:"maker_user_id"`
	TakerUserID  int64        `json:"taker_user_id"`
	TakerSide    trading.Side `json:"taker_side"`
	Price        string       `json:"price"`
	Quantity     string       `json:"quantity"`
	OccurredAt   time.Time    `json:"occurred_at"`
}

func decodeExecutedTrade(envelope eventing.Envelope) (executedTrade, error) {
	if envelope.Type != "trades.executed" {
		return executedTrade{}, fmt.Errorf("decode executed trade: wrong subject")
	}
	decoder := json.NewDecoder(bytes.NewReader(envelope.Data))
	decoder.DisallowUnknownFields()
	var trade executedTrade
	if err := decoder.Decode(&trade); err != nil {
		return executedTrade{}, fmt.Errorf("decode executed trade")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF || trade.Version != 1 || trade.TradeID != envelope.AggregateID || trade.Symbol == "" || trade.MakerUserID <= 0 || trade.TakerUserID <= 0 || trade.MakerUserID == trade.TakerUserID || trade.OccurredAt.IsZero() || (trade.TakerSide != trading.SideBuy && trade.TakerSide != trading.SideSell) {
		return executedTrade{}, fmt.Errorf("decode executed trade: invalid")
	}
	return trade, nil
}

// Process settles both trade legs exactly once in one Serializable transaction.
func (r *PositionRepository) Process(ctx context.Context, envelope eventing.Envelope, receivedAt time.Time) (bool, error) {
	if r == nil || r.pool == nil || r.timeout <= 0 {
		return false, fmt.Errorf("settle position: invalid repository")
	}
	trade, err := decodeExecutedTrade(envelope)
	if err != nil {
		return false, err
	}
	op, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	tx, err := r.pool.BeginTx(op, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return false, fmt.Errorf("settle position: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	inserted, err := insertProcessedEvent(op, tx, PositionConsumer, envelope, receivedAt)
	if err != nil {
		return false, err
	}
	if !inserted {
		if err := tx.Commit(op); err != nil {
			return false, err
		}
		return false, nil
	}
	var priceScale, quantityScale, quoteScale uint8
	if err := tx.QueryRow(op, `SELECT price_scale,quantity_scale,quote_scale FROM instruments WHERE symbol=$1`, trade.Symbol).Scan(&priceScale, &quantityScale, &quoteScale); err != nil {
		return false, fmt.Errorf("settle position: instrument: %w", err)
	}
	priceValue, err := trading.ParsePrice(trade.Price, priceScale)
	if err != nil {
		return false, fmt.Errorf("settle position: price")
	}
	quantityValue, err := trading.ParseQuantity(trade.Quantity, quantityScale)
	if err != nil {
		return false, fmt.Errorf("settle position: quantity")
	}
	makerSide := trading.SideBuy
	if trade.TakerSide == trading.SideBuy {
		makerSide = trading.SideSell
	}
	legs := []struct {
		userID  int64
		side    trading.Side
		orderID string
	}{{trade.MakerUserID, makerSide, trade.MakerOrderID}, {trade.TakerUserID, trade.TakerSide, trade.TakerOrderID}}
	for _, leg := range legs {
		if err := r.settleLeg(op, tx, envelope, trade, leg.userID, leg.side, leg.orderID, priceValue.Atoms(), quantityValue.Atoms(), priceScale, quantityScale, quoteScale, receivedAt); err != nil {
			return false, err
		}
	}
	if _, err := tx.Exec(op, `INSERT INTO trade_settlements(trade_id,source_event_id,settled_at) VALUES($1,$2,$3)`, trade.TradeID, envelope.ID, receivedAt.UTC()); err != nil {
		return false, fmt.Errorf("settle position: record settlement: %w", err)
	}
	if _, err := tx.Exec(op, `INSERT INTO event_receipts(consumer_name,event_id,aggregate_id,correlation_id,received_at) VALUES($1,$2,$3,$4,$5)`, PositionConsumer, envelope.ID, envelope.AggregateID, envelope.CorrelationID, receivedAt.UTC()); err != nil {
		return false, fmt.Errorf("settle position: receipt: %w", err)
	}
	if err := tx.Commit(op); err != nil {
		return false, fmt.Errorf("settle position: commit: %w", err)
	}
	return true, nil
}

func (r *PositionRepository) settleLeg(ctx context.Context, tx pgx.Tx, envelope eventing.Envelope, trade executedTrade, userID int64, side trading.Side, orderID string, priceAtoms, quantityAtoms int64, priceScale, quantityScale, quoteScale uint8, at time.Time) error {
	current := position.State{Symbol: trade.Symbol, PriceScale: priceScale, QuantityScale: quantityScale}
	var cumulativePnL, fees, version int64
	err := tx.QueryRow(ctx, `SELECT size_atoms,entry_price_atoms,realized_pnl_atoms,fee_atoms,version FROM positions WHERE user_id=$1 AND symbol=$2 FOR UPDATE`, userID, trade.Symbol).Scan(&current.SizeAtoms, &current.EntryPriceAtoms, &cumulativePnL, &fees, &version)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("settle position: lock: %w", err)
	}
	var reduceOnly bool
	if err := tx.QueryRow(ctx, `SELECT reduce_only FROM orders WHERE id=$1 AND user_id=$2`, orderID, userID).Scan(&reduceOnly); err != nil {
		return fmt.Errorf("settle position: order policy: %w", err)
	}
	before := current.SizeAtoms
	updated, pnlPriceScale, err := position.ApplyFill(current, position.Fill{Side: position.Side(side), PriceAtoms: priceAtoms, QuantityAtoms: quantityAtoms, ReduceOnly: reduceOnly})
	if err != nil {
		return err
	}
	pnlLedger, err := rescaleExact(pnlPriceScale, priceScale, quoteScale)
	if err != nil {
		return fmt.Errorf("settle position: pnl scale")
	}
	notional, err := notionalAtQuoteScale(priceAtoms, quantityAtoms, priceScale, quantityScale, quoteScale)
	if err != nil {
		return fmt.Errorf("settle position: notional scale")
	}
	fee, err := fixedmath.MulDivCeil(notional, takerFeeBPS, 10_000)
	if err != nil {
		return fmt.Errorf("settle position: fee")
	}
	if _, err := tx.Exec(ctx, `INSERT INTO positions(user_id,symbol,size_atoms,entry_price_atoms,price_scale,quantity_scale,realized_pnl_atoms,fee_atoms,version,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,1,$9) ON CONFLICT(user_id,symbol) DO UPDATE SET size_atoms=EXCLUDED.size_atoms,entry_price_atoms=EXCLUDED.entry_price_atoms,realized_pnl_atoms=positions.realized_pnl_atoms+$7,fee_atoms=positions.fee_atoms+$8,version=positions.version+1,updated_at=$9`, userID, trade.Symbol, updated.SizeAtoms, updated.EntryPriceAtoms, priceScale, quantityScale, pnlLedger, fee, at.UTC()); err != nil {
		return fmt.Errorf("settle position: update: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO position_events(event_id,trade_id,user_id,symbol,side,fill_price_atoms,fill_quantity_atoms,size_before_atoms,size_after_atoms,realized_pnl_atoms,fee_atoms,occurred_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, envelope.ID, trade.TradeID, userID, trade.Symbol, side, priceAtoms, quantityAtoms, before, updated.SizeAtoms, pnlLedger, fee, at.UTC()); err != nil {
		return fmt.Errorf("settle position: event: %w", err)
	}
	if err := settleLedgerAndMargin(ctx, tx, userID, trade.Symbol, trade.TradeID, orderID, updated, pnlLedger, fee, quoteScale, at); err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]any{"version": 1, "user_id": userID, "symbol": trade.Symbol, "size_atoms": updated.SizeAtoms, "entry_price_atoms": updated.EntryPriceAtoms, "position_version": version + 1, "occurred_at": at.UTC().Format(time.RFC3339Nano)})
	id, _ := randomBusinessID("out_")
	_, err = tx.Exec(ctx, `INSERT INTO outbox_events(id,aggregate_type,aggregate_id,subject,correlation_id,payload,available_at,created_at) VALUES($1,'position',$2,'positions.updated',$3,$4,$5,$5)`, id, fmt.Sprintf("%d:%s", userID, trade.Symbol), envelope.CorrelationID, payload, at.UTC())
	if err != nil {
		return fmt.Errorf("settle position: outbox: %w", err)
	}
	return nil
}

func settleLedgerAndMargin(ctx context.Context, tx pgx.Tx, userID int64, symbol, tradeID, orderID string, state position.State, pnl, fee int64, quoteScale uint8, at time.Time) error {
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO balances(user_id,asset) VALUES($1,'USDT') ON CONFLICT DO NOTHING`, []any{userID}},
		{`INSERT INTO margin_accounts(user_id) VALUES($1) ON CONFLICT DO NOTHING`, []any{userID}},
		{`INSERT INTO position_margins(user_id,symbol) VALUES($1,$2) ON CONFLICT DO NOTHING`, []any{userID, symbol}},
		{`INSERT INTO ledger_accounts(owner_user_id,asset,account_type) VALUES($1,'USDT','available') ON CONFLICT DO NOTHING`, []any{userID}},
	}
	for _, statement := range statements {
		if _, err := tx.Exec(ctx, statement.query, statement.args...); err != nil {
			return fmt.Errorf("settle trade accounts: %w", err)
		}
	}
	var available, locked int64
	if err := tx.QueryRow(ctx, `SELECT available_atoms,locked_atoms FROM balances WHERE user_id=$1 AND asset='USDT' FOR UPDATE`, userID).Scan(&available, &locked); err != nil {
		return fmt.Errorf("settle trade balance: %w", err)
	}
	var leverage, reserved int64
	var status string
	if err := tx.QueryRow(ctx, `SELECT leverage,reserved_margin_atoms,status FROM orders WHERE id=$1 AND user_id=$2 FOR UPDATE`, orderID, userID).Scan(&leverage, &reserved, &status); err != nil {
		return fmt.Errorf("settle trade order: %w", err)
	}
	release := int64(0)
	if status == "filled" {
		release = reserved
		reserved = 0
	}
	available += release
	locked -= release
	target := int64(0)
	if state.SizeAtoms != 0 {
		positionQuantity, absErr := fixedmath.Abs(state.SizeAtoms)
		if absErr != nil {
			return fmt.Errorf("settle trade: invalid position size")
		}
		positionNotional, notionalErr := notionalAtQuoteScale(state.EntryPriceAtoms, positionQuantity, state.PriceScale, state.QuantityScale, quoteScale)
		if notionalErr != nil {
			return fmt.Errorf("settle trade: invalid position notional")
		}
		target = (positionNotional + leverage - 1) / leverage
	}
	var currentMargin int64
	if err := tx.QueryRow(ctx, `SELECT isolated_margin_atoms FROM position_margins WHERE user_id=$1 AND symbol=$2 FOR UPDATE`, userID, symbol).Scan(&currentMargin); err != nil {
		return fmt.Errorf("settle trade margin: %w", err)
	}
	delta := target - currentMargin
	available -= delta
	locked += delta
	net := pnl - fee
	available += net
	if available < 0 || locked < 0 {
		return fmt.Errorf("settle trade: insufficient collateral")
	}
	if _, err := tx.Exec(ctx, `UPDATE balances SET available_atoms=$2,locked_atoms=$3,version=version+1,updated_at=$4 WHERE user_id=$1 AND asset='USDT'`, userID, available, locked, at.UTC()); err != nil {
		return fmt.Errorf("settle trade balances: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE orders SET reserved_margin_atoms=$2,updated_at=$3 WHERE id=$1`, orderID, reserved, at.UTC()); err != nil {
		return fmt.Errorf("settle trade order reserve: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE position_margins SET isolated_margin_atoms=$3,updated_at=$4 WHERE user_id=$1 AND symbol=$2`, userID, symbol, target, at.UTC()); err != nil {
		return fmt.Errorf("settle trade position margin: %w", err)
	}
	var userAccount, feeAccount, insuranceAccount int64
	if err := tx.QueryRow(ctx, `SELECT id FROM ledger_accounts WHERE owner_user_id=$1 AND asset='USDT' AND account_type='available'`, userID).Scan(&userAccount); err != nil {
		return err
	}
	if err := tx.QueryRow(ctx, `SELECT id FROM ledger_accounts WHERE owner_user_id IS NULL AND asset='USDT' AND account_type='fees'`).Scan(&feeAccount); err != nil {
		return err
	}
	if err := tx.QueryRow(ctx, `SELECT id FROM ledger_accounts WHERE owner_user_id IS NULL AND asset='USDT' AND account_type='insurance'`).Scan(&insuranceAccount); err != nil {
		return err
	}
	txID := "trade:" + tradeID + ":" + fmt.Sprint(userID)
	key := "trade-settle:" + tradeID + ":" + fmt.Sprint(userID)
	if _, err := tx.Exec(ctx, `INSERT INTO ledger_transactions(id,user_id,kind,idempotency_key,created_at) VALUES($1,$2,'trade_settlement',$3,$4)`, txID, userID, key, at.UTC()); err != nil {
		return fmt.Errorf("settle trade ledger transaction: %w", err)
	}
	entries := [][2]int64{{userAccount, pnl}, {insuranceAccount, -pnl}, {userAccount, -fee}, {feeAccount, fee}}
	for _, entry := range entries {
		if entry[1] == 0 {
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO ledger_entries(transaction_id,account_id,asset,amount_atoms,created_at) VALUES($1,$2,'USDT',$3,$4)`, txID, entry[0], entry[1], at.UTC()); err != nil {
			return fmt.Errorf("settle trade ledger entry: %w", err)
		}
	}
	return nil
}

// List returns user positions valued against the latest persisted mark price.
func (r *PositionRepository) List(ctx context.Context, userID int64) ([]position.View, error) {
	if userID <= 0 {
		return nil, fmt.Errorf("list positions: invalid user")
	}
	op, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	rows, err := r.pool.Query(op, `SELECT p.user_id,p.symbol,p.size_atoms,p.entry_price_atoms,m.mark_price_atoms,p.realized_pnl_atoms,p.fee_atoms,p.price_scale,p.quantity_scale,i.quote_scale,p.version,p.updated_at,m.as_of FROM positions p JOIN instruments i ON i.symbol=p.symbol JOIN LATERAL(SELECT mark_price_atoms,as_of FROM mark_prices WHERE symbol=p.symbol ORDER BY created_at DESC,id DESC LIMIT 1)m ON true WHERE p.user_id=$1 ORDER BY p.symbol`, userID)
	if err != nil {
		return nil, fmt.Errorf("list positions: %w", err)
	}
	defer rows.Close()
	result := []position.View{}
	for rows.Next() {
		var v position.View
		var markAsOf time.Time
		if err := rows.Scan(&v.UserID, &v.Symbol, &v.SizeAtoms, &v.EntryPriceAtoms, &v.MarkPriceAtoms, &v.RealizedPnLAtoms, &v.FeeAtoms, &v.PriceScale, &v.QuantityScale, &v.QuoteScale, &v.Version, &v.UpdatedAt, &markAsOf); err != nil {
			return nil, err
		}
		upnl, err := position.UnrealizedPnL(position.State{SizeAtoms: v.SizeAtoms, EntryPriceAtoms: v.EntryPriceAtoms, QuantityScale: v.QuantityScale}, v.MarkPriceAtoms)
		if err != nil {
			return nil, err
		}
		v.UnrealizedPnLAtoms, err = rescaleExact(upnl, v.PriceScale, v.QuoteScale)
		if err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

func notionalAtQuoteScale(price, qty int64, priceScale, qtyScale, quoteScale uint8) (int64, error) {
	n := new(big.Int).Mul(big.NewInt(price), big.NewInt(qty))
	source := int(priceScale) + int(qtyScale)
	target := int(quoteScale)
	if target >= source {
		n.Mul(n, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(target-source)), nil))
	} else {
		d := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(source-target)), nil)
		q, r := new(big.Int), new(big.Int)
		q.QuoRem(n, d, r)
		if r.Sign() != 0 {
			q.Add(q, big.NewInt(1))
		}
		n = q
	}
	if !n.IsInt64() {
		return 0, fixedmath.ErrArithmetic
	}
	return n.Int64(), nil
}
func rescaleExact(value int64, source, target uint8) (int64, error) {
	if source == target {
		return value, nil
	}
	n := big.NewInt(value)
	if target > source {
		n.Mul(n, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(target-source)), nil))
	} else {
		d := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(source-target)), nil)
		q, r := new(big.Int), new(big.Int)
		q.QuoRem(n, d, r)
		if r.Sign() != 0 {
			return 0, fixedmath.ErrArithmetic
		}
		n = q
	}
	if !n.IsInt64() {
		return 0, fixedmath.ErrArithmetic
	}
	return n.Int64(), nil
}
