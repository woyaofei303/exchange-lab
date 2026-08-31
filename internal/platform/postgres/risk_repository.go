package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/woyaofei303/exchange-lab/internal/fixedmath"
	"github.com/woyaofei303/exchange-lab/internal/funding"
	"github.com/woyaofei303/exchange-lab/internal/liquidation"
	"github.com/woyaofei303/exchange-lab/internal/position"
	"github.com/woyaofei303/exchange-lab/internal/risk"
	"github.com/woyaofei303/exchange-lab/internal/trading"
)

// RiskRepository owns authoritative risk, funding and conditional-order persistence.
type RiskRepository struct {
	pool    *pgxpool.Pool
	timeout time.Duration
}

// NewRiskRepository constructs a context-bounded derivatives repository.
func NewRiskRepository(pool *pgxpool.Pool, timeout time.Duration) *RiskRepository {
	return &RiskRepository{pool: pool, timeout: timeout}
}

// Snapshot calculates one user's current mark-price margin and liquidation projection.
func (r *RiskRepository) Snapshot(ctx context.Context, userID int64, symbol string) (risk.Snapshot, error) {
	if r == nil || r.pool == nil || userID <= 0 || symbol == "" {
		return risk.Snapshot{}, fmt.Errorf("risk snapshot: invalid input")
	}
	op, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	var s risk.Snapshot
	s.UserID = userID
	s.Symbol = symbol
	var mmBPS int64
	err := r.pool.QueryRow(op, `SELECT COALESCE(a.mode,'isolated'),COALESCE(l.leverage,10),COALESCE(p.size_atoms,0),COALESCE(p.entry_price_atoms,0),m.mark_price_atoms,i.price_scale,i.quantity_scale,i.quote_scale,COALESCE(pm.isolated_margin_atoms,0),COALESCE(pm.order_margin_atoms,0),COALESCE(b.available_atoms,0),COALESCE(a.risk_version,1),m.fresh,m.as_of,COALESCE(t.maintenance_margin_bps,50)
		FROM instruments i JOIN LATERAL(SELECT mark_price_atoms,fresh,as_of FROM mark_prices WHERE symbol=i.symbol ORDER BY created_at DESC,id DESC LIMIT 1)m ON true
		LEFT JOIN positions p ON p.symbol=i.symbol AND p.user_id=$1 LEFT JOIN margin_accounts a ON a.user_id=$1 LEFT JOIN leverage_settings l ON l.user_id=$1 AND l.symbol=i.symbol LEFT JOIN position_margins pm ON pm.user_id=$1 AND pm.symbol=i.symbol LEFT JOIN balances b ON b.user_id=$1 AND b.asset='USDT' LEFT JOIN leverage_tiers t ON t.symbol=i.symbol AND t.tier=1 WHERE i.symbol=$2`, userID, symbol).Scan(&s.Mode, &s.Leverage, &s.PositionSizeAtoms, &s.EntryPriceAtoms, &s.MarkPriceAtoms, &s.PriceScale, &s.QuantityScale, &s.QuoteScale, &s.PositionMarginAtoms, &s.OrderMarginAtoms, &s.WalletAvailableAtoms, &s.RiskVersion, &s.Fresh, &s.AsOf, &mmBPS)
	if err != nil {
		return risk.Snapshot{}, fmt.Errorf("risk snapshot: %w", err)
	}
	if s.PositionSizeAtoms != 0 {
		upnl, err := position.UnrealizedPnL(position.State{SizeAtoms: s.PositionSizeAtoms, EntryPriceAtoms: s.EntryPriceAtoms, QuantityScale: s.QuantityScale}, s.MarkPriceAtoms)
		if err != nil {
			return risk.Snapshot{}, err
		}
		s.UnrealizedPnLAtoms, err = rescaleExact(upnl, s.PriceScale, s.QuoteScale)
		if err != nil {
			return risk.Snapshot{}, err
		}
		abs, _ := fixedmath.Abs(s.PositionSizeAtoms)
		notional, err := notionalAtQuoteScale(s.MarkPriceAtoms, abs, s.PriceScale, s.QuantityScale, s.QuoteScale)
		if err != nil {
			return risk.Snapshot{}, err
		}
		s.MaintenanceMarginAtoms, err = fixedmath.MulDivCeil(notional, mmBPS, 10_000)
		if err != nil {
			return risk.Snapshot{}, err
		}
	}
	s.MarginBalanceAtoms = s.PositionMarginAtoms + s.UnrealizedPnLAtoms
	if s.MarginBalanceAtoms > 0 {
		s.MarginRatioBPS, _ = fixedmath.MulDiv(s.MaintenanceMarginAtoms, 10_000, s.MarginBalanceAtoms)
	} else if s.MaintenanceMarginAtoms > 0 {
		s.MarginRatioBPS = 10_000
	}
	if s.PositionSizeAtoms != 0 {
		priceBalance, _ := rescaleRounded(maxInt64(s.MarginBalanceAtoms, 0), s.QuoteScale, s.PriceScale)
		priceMM, _ := rescaleRounded(s.MaintenanceMarginAtoms, s.QuoteScale, s.PriceScale)
		decision, decisionErr := liquidation.Evaluate(liquidation.Snapshot{UserID: userID, Symbol: s.Symbol, PositionSizeAtoms: s.PositionSizeAtoms, EntryPriceAtoms: s.EntryPriceAtoms, MarkPriceAtoms: s.MarkPriceAtoms, MarginBalanceAtoms: priceBalance, MaintenanceMarginAtoms: priceMM, QuantityScale: s.QuantityScale, RiskVersion: s.RiskVersion})
		if decisionErr == nil {
			distance := s.MarkPriceAtoms - decision.BankruptcyPriceAtoms
			if distance < 0 {
				distance = -distance
			}
			s.LiquidationDistanceBPS, _ = fixedmath.MulDiv(distance, 10_000, s.MarkPriceAtoms)
		}
	}
	return s, nil
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// SetLeverage validates the active tier and advances the user's risk version.
func (r *RiskRepository) SetLeverage(ctx context.Context, userID int64, symbol string, leverage int64) error {
	if userID <= 0 || symbol == "" || leverage < 1 || leverage > 125 {
		return fmt.Errorf("set leverage: invalid input")
	}
	op, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	tx, err := r.pool.BeginTx(op, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var max int64
	if err := tx.QueryRow(op, `SELECT max_leverage FROM leverage_tiers WHERE symbol=$1 ORDER BY tier LIMIT 1`, symbol).Scan(&max); err != nil {
		return err
	}
	if leverage > max {
		return fmt.Errorf("set leverage: exceeds risk tier")
	}
	if _, err := tx.Exec(op, `INSERT INTO margin_accounts(user_id) VALUES($1) ON CONFLICT DO NOTHING`, userID); err != nil {
		return fmt.Errorf("set leverage: %w", err)
	}
	if _, err := tx.Exec(op, `INSERT INTO leverage_settings(user_id,symbol,leverage) VALUES($1,$2,$3) ON CONFLICT(user_id,symbol) DO UPDATE SET leverage=EXCLUDED.leverage,updated_at=now()`, userID, symbol, leverage); err != nil {
		return fmt.Errorf("set leverage: %w", err)
	}
	if _, err := tx.Exec(op, `UPDATE margin_accounts SET risk_version=risk_version+1,updated_at=now() WHERE user_id=$1`, userID); err != nil {
		return fmt.Errorf("set leverage: %w", err)
	}
	return tx.Commit(op)
}

// SetMode switches isolated/cross mode and advances the user's risk version.
func (r *RiskRepository) SetMode(ctx context.Context, userID int64, mode string) error {
	if userID <= 0 || (mode != "isolated" && mode != "cross") {
		return fmt.Errorf("set margin mode: invalid input")
	}
	op, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	_, err := r.pool.Exec(op, `INSERT INTO margin_accounts(user_id,mode) VALUES($1,$2) ON CONFLICT(user_id) DO UPDATE SET mode=EXCLUDED.mode,risk_version=margin_accounts.risk_version+1,updated_at=now()`, userID, mode)
	if err != nil {
		return fmt.Errorf("set margin mode: %w", err)
	}
	return nil
}

// AdjustMargin transfers collateral idempotently between balance and position margin.
func (r *RiskRepository) AdjustMargin(ctx context.Context, userID int64, symbol string, amount int64, key string) (int64, bool, error) {
	if userID <= 0 || symbol == "" || amount == 0 || len(key) < 8 || len(key) > 128 {
		return 0, false, fmt.Errorf("adjust margin: invalid input")
	}
	op, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	tx, err := r.pool.BeginTx(op, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return 0, false, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(op, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, fmt.Sprintf("%d:%s", userID, key)); err != nil {
		return 0, false, err
	}
	var storedAmount, storedAfter int64
	err = tx.QueryRow(op, `SELECT amount_atoms,margin_after_atoms FROM margin_adjustments WHERE user_id=$1 AND idempotency_key=$2`, userID, key).Scan(&storedAmount, &storedAfter)
	if err == nil {
		if storedAmount != amount {
			return 0, false, fmt.Errorf("adjust margin: idempotency conflict")
		}
		if err := tx.Commit(op); err != nil {
			return 0, false, fmt.Errorf("commit replayed margin adjustment: %w", err)
		}
		return storedAfter, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, false, err
	}
	if _, err := tx.Exec(op, `INSERT INTO position_margins(user_id,symbol) VALUES($1,$2) ON CONFLICT DO NOTHING`, userID, symbol); err != nil {
		return 0, false, err
	}
	var available, locked, current int64
	if err := tx.QueryRow(op, `SELECT available_atoms,locked_atoms FROM balances WHERE user_id=$1 AND asset='USDT' FOR UPDATE`, userID).Scan(&available, &locked); err != nil {
		return 0, false, err
	}
	if err := tx.QueryRow(op, `SELECT isolated_margin_atoms FROM position_margins WHERE user_id=$1 AND symbol=$2 FOR UPDATE`, userID, symbol).Scan(&current); err != nil {
		return 0, false, err
	}
	after := current + amount
	if after < 0 || available < amount || locked+amount < 0 {
		return 0, false, fmt.Errorf("adjust margin: insufficient balance")
	}
	available -= amount
	locked += amount
	id, _ := randomBusinessID("madj_")
	if _, err := tx.Exec(op, `UPDATE balances SET available_atoms=$2,locked_atoms=$3,version=version+1,updated_at=now() WHERE user_id=$1 AND asset='USDT'`, userID, available, locked); err != nil {
		return 0, false, fmt.Errorf("adjust margin balance: %w", err)
	}
	if _, err := tx.Exec(op, `UPDATE position_margins SET isolated_margin_atoms=$3,updated_at=now() WHERE user_id=$1 AND symbol=$2`, userID, symbol, after); err != nil {
		return 0, false, fmt.Errorf("adjust position margin: %w", err)
	}
	if _, err := tx.Exec(op, `INSERT INTO margin_adjustments(id,user_id,symbol,amount_atoms,idempotency_key,margin_after_atoms,created_at) VALUES($1,$2,$3,$4,$5,$6,now())`, id, userID, symbol, amount, key, after); err != nil {
		return 0, false, fmt.Errorf("record margin adjustment: %w", err)
	}
	if _, err := tx.Exec(op, `INSERT INTO margin_accounts(user_id) VALUES($1) ON CONFLICT DO NOTHING`, userID); err != nil {
		return 0, false, fmt.Errorf("adjust margin: %w", err)
	}
	if _, err := tx.Exec(op, `UPDATE margin_accounts SET risk_version=risk_version+1,updated_at=now() WHERE user_id=$1`, userID); err != nil {
		return 0, false, fmt.Errorf("adjust margin: %w", err)
	}
	if err := tx.Commit(op); err != nil {
		return 0, false, err
	}
	return after, false, nil
}

// ScanLiquidations creates one idempotent pending job per breached risk version.
func (r *RiskRepository) ScanLiquidations(ctx context.Context, at time.Time) (int, error) {
	op, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	rows, err := r.pool.Query(op, `SELECT p.user_id,p.symbol,p.size_atoms,p.entry_price_atoms,p.price_scale,p.quantity_scale,i.quote_scale,m.mark_price_atoms,COALESCE(pm.isolated_margin_atoms,0),COALESCE(a.risk_version,1),COALESCE(a.mode,'isolated'),COALESCE(t.maintenance_margin_bps,50) FROM positions p JOIN instruments i ON i.symbol=p.symbol JOIN LATERAL(SELECT mark_price_atoms FROM mark_prices WHERE symbol=p.symbol ORDER BY created_at DESC,id DESC LIMIT 1)m ON true LEFT JOIN position_margins pm ON pm.user_id=p.user_id AND pm.symbol=p.symbol LEFT JOIN margin_accounts a ON a.user_id=p.user_id LEFT JOIN leverage_tiers t ON t.symbol=p.symbol AND t.tier=1 WHERE p.size_atoms<>0`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	created := 0
	for rows.Next() {
		var user, size, entry, mark, positionMargin, version, mmBPS int64
		var symbol, mode string
		var ps, qs, quote uint8
		if err := rows.Scan(&user, &symbol, &size, &entry, &ps, &qs, &quote, &mark, &positionMargin, &version, &mode, &mmBPS); err != nil {
			return created, err
		}
		upnlPS, _ := position.UnrealizedPnL(position.State{SizeAtoms: size, EntryPriceAtoms: entry, QuantityScale: qs}, mark)
		upnl, _ := rescaleExact(upnlPS, ps, quote)
		balance := positionMargin + upnl
		abs, _ := fixedmath.Abs(size)
		notional, _ := notionalAtQuoteScale(mark, abs, ps, qs, quote)
		mm, _ := fixedmath.MulDivCeil(notional, mmBPS, 10_000)
		decisionBalance := balance
		if decisionBalance < 0 {
			decisionBalance = 0
		}
		priceBalance, _ := rescaleRounded(decisionBalance, quote, ps)
		priceMM, _ := rescaleRounded(mm, quote, ps)
		d, err := liquidation.Evaluate(liquidation.Snapshot{UserID: user, Symbol: symbol, PositionSizeAtoms: size, EntryPriceAtoms: entry, MarkPriceAtoms: mark, MarginBalanceAtoms: priceBalance, MaintenanceMarginAtoms: priceMM, QuantityScale: qs, RiskVersion: version})
		if err != nil || !d.Required {
			continue
		}
		id := "liq_" + fmt.Sprintf("%x", sha256.Sum256([]byte(d.JobKey)))[:24]
		tag, err := r.pool.Exec(op, `INSERT INTO liquidation_jobs(id,idempotency_key,user_id,symbol,margin_mode,risk_version,mark_price_atoms,bankruptcy_price_atoms,status,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'pending',$9,$9) ON CONFLICT(idempotency_key) DO NOTHING`, id, d.JobKey, user, symbol, mode, version, mark, d.BankruptcyPriceAtoms, at.UTC())
		if err != nil {
			return created, err
		}
		created += int(tag.RowsAffected())
	}
	return created, rows.Err()
}

// Liquidations returns a bounded user-scoped liquidation history.
func (r *RiskRepository) Liquidations(ctx context.Context, userID int64) ([]risk.LiquidationRecord, error) {
	op, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	rows, err := r.pool.Query(op, `SELECT id,symbol,margin_mode,status,mark_price_atoms,bankruptcy_price_atoms,risk_version,created_at,updated_at FROM liquidation_jobs WHERE user_id=$1 ORDER BY created_at DESC,id DESC LIMIT 100`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []risk.LiquidationRecord
	for rows.Next() {
		var v risk.LiquidationRecord
		if err := rows.Scan(&v.ID, &v.Symbol, &v.Mode, &v.Status, &v.MarkPriceAtoms, &v.BankruptcyPriceAtoms, &v.RiskVersion, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

// RunFunding settles one symbol/time window exactly once with zero-sum entries.
func (r *RiskRepository) RunFunding(ctx context.Context, symbol string, fundingTime time.Time) (int64, int, error) {
	if symbol == "" || fundingTime.IsZero() {
		return 0, 0, fmt.Errorf("funding: invalid input")
	}
	op, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	tx, err := r.pool.BeginTx(op, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var mark, index int64
	var ps, qs, quote uint8
	if err := tx.QueryRow(op, `SELECT m.mark_price_atoms,m.index_price_atoms,m.price_scale,i.quantity_scale,i.quote_scale FROM mark_prices m JOIN instruments i ON i.symbol=m.symbol WHERE m.symbol=$1 ORDER BY m.created_at DESC,m.id DESC LIMIT 1`, symbol).Scan(&mark, &index, &ps, &qs, &quote); err != nil {
		return 0, 0, err
	}
	premium, _ := fixedmath.MulDiv(mark-index, 10_000, index)
	rate, _ := funding.ComputeRate(premium, 0, 100)
	var rateID int64
	err = tx.QueryRow(op, `INSERT INTO funding_rates(symbol,funding_time,predicted_rate_bps,final_rate_bps,mark_price_atoms,status) VALUES($1,$2,$3,$3,$4,'final') ON CONFLICT(symbol,funding_time) DO UPDATE SET symbol=EXCLUDED.symbol RETURNING id`, symbol, fundingTime.UTC(), rate, mark).Scan(&rateID)
	if err != nil {
		return 0, 0, err
	}
	var existing int
	if err := tx.QueryRow(op, `SELECT count(*) FROM funding_settlements WHERE funding_rate_id=$1`, rateID).Scan(&existing); err != nil {
		return 0, 0, err
	}
	if existing > 0 {
		if err := tx.Commit(op); err != nil {
			return 0, 0, fmt.Errorf("commit replayed funding settlement: %w", err)
		}
		return rateID, existing, nil
	}
	rows, err := tx.Query(op, `SELECT user_id,size_atoms,version FROM positions WHERE symbol=$1 AND size_atoms<>0 ORDER BY user_id FOR UPDATE`, symbol)
	if err != nil {
		return 0, 0, err
	}
	type payment struct{ user, size, version, amount int64 }
	var payments []payment
	amounts := make([]int64, 0)
	for rows.Next() {
		var p payment
		if err := rows.Scan(&p.user, &p.size, &p.version); err != nil {
			rows.Close()
			return 0, 0, err
		}
		amountPS, _ := funding.Payment(p.size, mark, qs, rate)
		p.amount, _ = rescaleExact(amountPS, ps, quote)
		payments = append(payments, p)
		amounts = append(amounts, p.amount)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}
	systemOffset, err := funding.SystemOffset(amounts)
	if err != nil {
		return 0, 0, fmt.Errorf("funding: settlement overflow")
	}
	nonZero := systemOffset != 0
	for _, p := range payments {
		if p.amount != 0 {
			nonZero = true
			break
		}
	}
	if nonZero {
		txID := fmt.Sprintf("funding:%d", rateID)
		if _, err := tx.Exec(op, `INSERT INTO ledger_transactions(id,user_id,kind,idempotency_key,created_at) VALUES($1,NULL,'funding',$1,$2)`, txID, fundingTime.UTC()); err != nil {
			return 0, 0, err
		}
		for _, p := range payments {
			if p.amount == 0 {
				continue
			}
			var account int64
			if err := tx.QueryRow(op, `SELECT id FROM ledger_accounts WHERE owner_user_id=$1 AND asset='USDT' AND account_type='available'`, p.user).Scan(&account); err != nil {
				return 0, 0, err
			}
			tag, err := tx.Exec(op, `UPDATE balances SET available_atoms=available_atoms+$2,version=version+1,updated_at=$3 WHERE user_id=$1 AND asset='USDT' AND available_atoms+$2>=0`, p.user, p.amount, fundingTime.UTC())
			if err != nil || tag.RowsAffected() != 1 {
				return 0, 0, fmt.Errorf("funding: insufficient user balance")
			}
			if _, err := tx.Exec(op, `INSERT INTO ledger_entries(transaction_id,account_id,asset,amount_atoms,created_at) VALUES($1,$2,'USDT',$3,$4)`, txID, account, p.amount, fundingTime.UTC()); err != nil {
				return 0, 0, err
			}
		}
		if systemOffset != 0 {
			var insuranceAccount, fundBalance int64
			if err := tx.QueryRow(op, `SELECT id FROM ledger_accounts WHERE owner_user_id IS NULL AND asset='USDT' AND account_type='insurance'`).Scan(&insuranceAccount); err != nil {
				return 0, 0, err
			}
			if err := tx.QueryRow(op, `SELECT balance_after_atoms FROM insurance_fund_entries ORDER BY occurred_at DESC,id DESC LIMIT 1 FOR UPDATE`).Scan(&fundBalance); err != nil {
				return 0, 0, err
			}
			after := fundBalance + systemOffset
			if after < 0 {
				return 0, 0, fmt.Errorf("funding: insurance fund depleted")
			}
			if _, err := tx.Exec(op, `INSERT INTO ledger_entries(transaction_id,account_id,asset,amount_atoms,created_at) VALUES($1,$2,'USDT',$3,$4)`, txID, insuranceAccount, systemOffset, fundingTime.UTC()); err != nil {
				return 0, 0, err
			}
			if _, err := tx.Exec(op, `INSERT INTO insurance_fund_entries(liquidation_job_id,amount_atoms,balance_after_atoms,reason,occurred_at) VALUES(NULL,$1,$2,'funding_imbalance',$3)`, systemOffset, after, fundingTime.UTC()); err != nil {
				return 0, 0, err
			}
		}
	}
	for _, p := range payments {
		if _, err := tx.Exec(op, `INSERT INTO funding_settlements(funding_rate_id,user_id,symbol,position_size_atoms,payment_atoms,position_version,settled_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, rateID, p.user, symbol, p.size, p.amount, p.version, fundingTime.UTC()); err != nil {
			return 0, 0, err
		}
	}
	if _, err := tx.Exec(op, `UPDATE funding_rates SET status='settled' WHERE id=$1`, rateID); err != nil {
		return 0, 0, err
	}
	if err := tx.Commit(op); err != nil {
		return 0, 0, err
	}
	return rateID, len(payments), nil
}

// FundingHistory returns the latest bounded rate history for a symbol.
func (r *RiskRepository) FundingHistory(ctx context.Context, symbol string) ([]risk.FundingRecord, error) {
	op, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	rows, err := r.pool.Query(op, `SELECT id,symbol,funding_time,predicted_rate_bps,final_rate_bps,mark_price_atoms,status FROM funding_rates WHERE symbol=$1 ORDER BY funding_time DESC LIMIT 100`, symbol)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []risk.FundingRecord
	for rows.Next() {
		var v risk.FundingRecord
		if err := rows.Scan(&v.ID, &v.Symbol, &v.FundingTime, &v.PredictedRateBPS, &v.FinalRateBPS, &v.MarkPriceAtoms, &v.Status); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

// FundingPayments returns the latest bounded settlements for one user.
func (r *RiskRepository) FundingPayments(ctx context.Context, userID int64) ([]risk.FundingPayment, error) {
	op, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	rows, err := r.pool.Query(op, `SELECT funding_rate_id,symbol,position_size_atoms,payment_atoms,settled_at FROM funding_settlements WHERE user_id=$1 ORDER BY settled_at DESC LIMIT 100`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []risk.FundingPayment
	for rows.Next() {
		var v risk.FundingPayment
		if err := rows.Scan(&v.FundingRateID, &v.Symbol, &v.PositionSizeAtoms, &v.PaymentAtoms, &v.SettledAt); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

// CreateConditional stores a reduce-only trigger idempotently.
func (r *RiskRepository) CreateConditional(ctx context.Context, userID int64, item risk.ConditionalOrder, key string) (risk.ConditionalOrder, bool, error) {
	if userID <= 0 || item.Symbol == "" || (item.Type != "take_profit" && item.Type != "stop_loss") || (item.TriggerSource != "mark" && item.TriggerSource != "last") || (item.TriggerDirection != "lte" && item.TriggerDirection != "gte") || item.TriggerPriceAtoms <= 0 || item.QuantityAtoms <= 0 || len(key) < 8 {
		return risk.ConditionalOrder{}, false, fmt.Errorf("conditional: invalid input")
	}
	op, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	id, _ := randomBusinessID("cond_")
	now := time.Now().UTC()
	tag, err := r.pool.Exec(op, `INSERT INTO conditional_orders(id,user_id,symbol,condition_type,trigger_source,trigger_direction,trigger_price_atoms,quantity_atoms,status,idempotency_key,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'pending',$9,$10,$10) ON CONFLICT(user_id,idempotency_key) DO NOTHING`, id, userID, item.Symbol, item.Type, item.TriggerSource, item.TriggerDirection, item.TriggerPriceAtoms, item.QuantityAtoms, key, now)
	if err != nil {
		return risk.ConditionalOrder{}, false, err
	}
	replayed := tag.RowsAffected() == 0
	err = r.pool.QueryRow(op, `SELECT id,user_id,symbol,condition_type,trigger_source,trigger_direction,trigger_price_atoms,quantity_atoms,status,created_at,updated_at FROM conditional_orders WHERE user_id=$1 AND idempotency_key=$2`, userID, key).Scan(&item.ID, &item.UserID, &item.Symbol, &item.Type, &item.TriggerSource, &item.TriggerDirection, &item.TriggerPriceAtoms, &item.QuantityAtoms, &item.Status, &item.CreatedAt, &item.UpdatedAt)
	return item, replayed, err
}

// Conditionals returns a bounded user-scoped trigger history.
func (r *RiskRepository) Conditionals(ctx context.Context, userID int64) ([]risk.ConditionalOrder, error) {
	op, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	rows, err := r.pool.Query(op, `SELECT id,user_id,symbol,condition_type,trigger_source,trigger_direction,trigger_price_atoms,quantity_atoms,status,created_at,updated_at FROM conditional_orders WHERE user_id=$1 ORDER BY created_at DESC LIMIT 100`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []risk.ConditionalOrder
	for rows.Next() {
		var v risk.ConditionalOrder
		if err := rows.Scan(&v.ID, &v.UserID, &v.Symbol, &v.Type, &v.TriggerSource, &v.TriggerDirection, &v.TriggerPriceAtoms, &v.QuantityAtoms, &v.Status, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

// Insurance returns the latest fund balance and pending ADL evidence count.
func (r *RiskRepository) Insurance(ctx context.Context) (risk.InsuranceStatus, error) {
	op, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	var s risk.InsuranceStatus
	err := r.pool.QueryRow(op, `SELECT balance_after_atoms,occurred_at,(SELECT count(*) FROM adl_candidates) FROM insurance_fund_entries ORDER BY occurred_at DESC,id DESC LIMIT 1`).Scan(&s.BalanceAtoms, &s.UpdatedAt, &s.ADLPending)
	return s, err
}

// TriggerConditionals creates reduce-only orders for eligible mark-price triggers.
func (r *RiskRepository) TriggerConditionals(ctx context.Context, at time.Time) (int, error) {
	op, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	rows, err := r.pool.Query(op, `SELECT c.id,c.user_id,c.symbol,c.trigger_price_atoms,c.quantity_atoms,c.trigger_direction,p.size_atoms,m.mark_price_atoms,i.price_scale,i.quantity_scale,i.quote_scale,COALESCE(l.leverage,10)
		FROM conditional_orders c JOIN positions p ON p.user_id=c.user_id AND p.symbol=c.symbol
		JOIN instruments i ON i.symbol=c.symbol JOIN LATERAL(SELECT mark_price_atoms FROM mark_prices WHERE symbol=c.symbol ORDER BY created_at DESC,id DESC LIMIT 1)m ON true
		LEFT JOIN leverage_settings l ON l.user_id=c.user_id AND l.symbol=c.symbol
		WHERE c.status='pending' AND c.trigger_source='mark' AND p.size_atoms<>0
		AND ((c.trigger_direction='lte' AND m.mark_price_atoms<=c.trigger_price_atoms) OR (c.trigger_direction='gte' AND m.mark_price_atoms>=c.trigger_price_atoms))`)
	if err != nil {
		return 0, err
	}
	type candidate struct {
		id                                       string
		user, trigger, qty, size, mark, leverage int64
		symbol, direction                        string
		ps, qs, quote                            uint8
	}
	var items []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.id, &c.user, &c.symbol, &c.trigger, &c.qty, &c.direction, &c.size, &c.mark, &c.ps, &c.qs, &c.quote, &c.leverage); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, c)
	}
	rows.Close()
	triggered := 0
	for _, c := range items {
		tx, err := r.pool.BeginTx(op, pgx.TxOptions{IsoLevel: pgx.Serializable})
		if err != nil {
			return triggered, err
		}
		func() {
			defer func() { _ = tx.Rollback(context.Background()) }()
			var status string
			if err = tx.QueryRow(op, `SELECT status FROM conditional_orders WHERE id=$1 FOR UPDATE`, c.id).Scan(&status); err != nil || status != "pending" {
				return
			}
			abs, _ := fixedmath.Abs(c.size)
			qty := c.qty
			if qty > abs {
				qty = abs
			}
			side := "sell"
			if c.size < 0 {
				side = "buy"
			}
			orderID, _ := randomBusinessID("ord_")
			price, _ := trading.FormatDecimal(c.mark, c.ps)
			quantity, _ := trading.FormatDecimal(qty, c.qs)
			reserved, _ := trading.FormatDecimal(0, c.quote)
			if _, err = tx.Exec(op, `INSERT INTO orders(id,user_id,client_order_id,symbol,side,order_type,status,price_atoms,price_scale,quantity_atoms,remaining_atoms,quantity_scale,quote_scale,leverage,reserved_margin_atoms,reduce_only,created_at,updated_at) VALUES($1,$2,$3,$4,$5,'limit','open',$6,$7,$8,$8,$9,$10,$11,0,true,$12,$12)`, orderID, c.user, "conditional:"+c.id, c.symbol, side, c.mark, c.ps, qty, c.qs, c.quote, c.leverage, at.UTC()); err != nil {
				return
			}
			payload, _ := json.Marshal(map[string]any{"version": 1, "order_id": orderID, "user_id": c.user, "symbol": c.symbol, "side": side, "type": "limit", "status": "open", "price": price, "quantity": quantity, "remaining": quantity, "leverage": c.leverage, "reserved_margin": reserved, "occurred_at": at.UTC().Format(time.RFC3339Nano)})
			eventID, _ := randomBusinessID("oev_")
			outboxID, _ := randomBusinessID("out_")
			if _, err = tx.Exec(op, `INSERT INTO order_events(id,order_id,event_type,payload,occurred_at) VALUES($1,$2,'accepted',$3,$4)`, eventID, orderID, payload, at.UTC()); err != nil {
				return
			}
			if _, err = tx.Exec(op, `INSERT INTO outbox_events(id,aggregate_type,aggregate_id,subject,correlation_id,payload,available_at,created_at) VALUES($1,'order',$2,'orders.accepted',$3,$4,$5,$5)`, outboxID, orderID, "conditional:"+c.id, payload, at.UTC()); err != nil {
				return
			}
			if _, err = tx.Exec(op, `UPDATE conditional_orders SET status='triggered',updated_at=$2 WHERE id=$1`, c.id, at.UTC()); err != nil {
				return
			}
			if err = tx.Commit(op); err == nil {
				triggered++
			}
		}()
		if err != nil {
			return triggered, err
		}
	}
	return triggered, nil
}

func rescaleRounded(value int64, source, target uint8) (int64, error) {
	if source <= target {
		return rescaleExact(value, source, target)
	}
	divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(source-target)), nil)
	n := big.NewInt(value)
	q, r := new(big.Int), new(big.Int)
	q.QuoRem(n, divisor, r)
	if new(big.Int).Abs(r).Mul(new(big.Int).Abs(r), big.NewInt(2)).Cmp(divisor) >= 0 {
		if value >= 0 {
			q.Add(q, big.NewInt(1))
		} else {
			q.Sub(q, big.NewInt(1))
		}
	}
	if !q.IsInt64() {
		return 0, fixedmath.ErrArithmetic
	}
	return q.Int64(), nil
}
