package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/woyaofei303/exchange-lab/internal/fixedmath"
	"github.com/woyaofei303/exchange-lab/internal/insurance"
	"github.com/woyaofei303/exchange-lab/internal/position"
)

// ProcessLiquidations settles a bounded batch of pending jobs transactionally.
func (r *RiskRepository) ProcessLiquidations(ctx context.Context, at time.Time) (int, error) {
	op, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	rows, err := r.pool.Query(op, `SELECT id FROM liquidation_jobs WHERE status='pending' ORDER BY created_at,id LIMIT 25`)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	completed := 0
	for _, id := range ids {
		tx, err := r.pool.BeginTx(op, pgx.TxOptions{IsoLevel: pgx.Serializable})
		if err != nil {
			return completed, err
		}
		if err := r.processLiquidation(op, tx, id, at.UTC()); err != nil {
			_ = tx.Rollback(context.Background())
			return completed, err
		}
		if err := tx.Commit(op); err != nil {
			return completed, err
		}
		completed++
	}
	return completed, nil
}

func (r *RiskRepository) processLiquidation(ctx context.Context, tx pgx.Tx, id string, at time.Time) error {
	var user, mark int64
	var symbol, mode, status string
	if err := tx.QueryRow(ctx, `SELECT user_id,symbol,margin_mode,status,mark_price_atoms FROM liquidation_jobs WHERE id=$1 FOR UPDATE`, id).Scan(&user, &symbol, &mode, &status, &mark); err != nil {
		return err
	}
	if status != "pending" {
		return nil
	}
	state := position.State{Symbol: symbol}
	var quoteScale uint8
	if err := tx.QueryRow(ctx, `SELECT p.size_atoms,p.entry_price_atoms,p.price_scale,p.quantity_scale,i.quote_scale FROM positions p JOIN instruments i ON i.symbol=p.symbol WHERE p.user_id=$1 AND p.symbol=$2 FOR UPDATE`, user, symbol).Scan(&state.SizeAtoms, &state.EntryPriceAtoms, &state.PriceScale, &state.QuantityScale, &quoteScale); err != nil {
		return err
	}
	abs, err := fixedmath.Abs(state.SizeAtoms)
	if err != nil {
		return err
	}
	closeQty := abs
	if mode == "cross" && abs > 1 {
		closeQty = (abs + 1) / 2
	}
	side := position.Sell
	if state.SizeAtoms < 0 {
		side = position.Buy
	}
	updated, pnlPriceScale, err := position.ApplyFill(state, position.Fill{Side: side, PriceAtoms: mark, QuantityAtoms: closeQty, ReduceOnly: true})
	if err != nil {
		return err
	}
	pnl, err := rescaleExact(pnlPriceScale, state.PriceScale, quoteScale)
	if err != nil {
		return err
	}
	var marginAtoms int64
	if err := tx.QueryRow(ctx, `SELECT isolated_margin_atoms FROM position_margins WHERE user_id=$1 AND symbol=$2 FOR UPDATE`, user, symbol).Scan(&marginAtoms); err != nil {
		return err
	}
	releasedMargin, err := fixedmath.MulDiv(marginAtoms, closeQty, abs)
	if err != nil {
		return err
	}
	equity := releasedMargin + pnl
	loss := int64(0)
	credit := equity
	if credit < 0 {
		loss = -credit
		credit = 0
	}
	var available, locked int64
	if err := tx.QueryRow(ctx, `SELECT available_atoms,locked_atoms FROM balances WHERE user_id=$1 AND asset='USDT' FOR UPDATE`, user).Scan(&available, &locked); err != nil {
		return err
	}
	if locked < releasedMargin {
		return fmt.Errorf("liquidation: locked margin invariant")
	}
	available += credit
	locked -= releasedMargin
	if _, err := tx.Exec(ctx, `UPDATE balances SET available_atoms=$2,locked_atoms=$3,version=version+1,updated_at=$4 WHERE user_id=$1 AND asset='USDT'`, user, available, locked, at); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE positions SET size_atoms=$3,entry_price_atoms=$4,realized_pnl_atoms=realized_pnl_atoms+$5,version=version+1,updated_at=$6 WHERE user_id=$1 AND symbol=$2`, user, symbol, updated.SizeAtoms, updated.EntryPriceAtoms, pnl, at); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE position_margins SET isolated_margin_atoms=isolated_margin_atoms-$3,updated_at=$4 WHERE user_id=$1 AND symbol=$2`, user, symbol, releasedMargin, at); err != nil {
		return err
	}
	if err := recordLiquidationLedger(ctx, tx, id, user, pnl, at); err != nil {
		return err
	}
	if err := r.coverLiquidationLoss(ctx, tx, id, user, symbol, loss, at); err != nil {
		return err
	}
	liqOrderID, _ := randomBusinessID("liqord_")
	if _, err := tx.Exec(ctx, `INSERT INTO liquidation_orders(id,job_id,quantity_atoms,status,created_at) VALUES($1,$2,$3,'filled',$4)`, liqOrderID, id, closeQty, at); err != nil {
		return err
	}
	evidence, _ := json.Marshal(map[string]any{"mark_price_atoms": mark, "realized_pnl_atoms": pnl, "loss_atoms": loss, "mode": mode})
	if _, err := tx.Exec(ctx, `INSERT INTO liquidation_events(job_id,event_type,evidence,occurred_at) VALUES($1,'completed',$2,$3)`, id, evidence, at); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE liquidation_jobs SET status='completed',updated_at=$2 WHERE id=$1`, id, at); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE margin_accounts SET risk_version=risk_version+1,updated_at=$2 WHERE user_id=$1`, user, at); err != nil {
		return err
	}
	return nil
}

func recordLiquidationLedger(ctx context.Context, tx pgx.Tx, id string, user, pnl int64, at time.Time) error {
	if pnl == 0 {
		return nil
	}
	var userAccount, insuranceAccount int64
	if err := tx.QueryRow(ctx, `SELECT id FROM ledger_accounts WHERE owner_user_id=$1 AND asset='USDT' AND account_type='available'`, user).Scan(&userAccount); err != nil {
		return err
	}
	if err := tx.QueryRow(ctx, `SELECT id FROM ledger_accounts WHERE owner_user_id IS NULL AND asset='USDT' AND account_type='insurance'`).Scan(&insuranceAccount); err != nil {
		return err
	}
	txID := "liquidation:" + id
	if _, err := tx.Exec(ctx, `INSERT INTO ledger_transactions(id,user_id,kind,idempotency_key,created_at) VALUES($1,$2,'liquidation',$1,$3)`, txID, user, at); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO ledger_entries(transaction_id,account_id,asset,amount_atoms,created_at) VALUES($1,$2,'USDT',$3,$4),($1,$5,'USDT',$6,$4)`, txID, userAccount, pnl, at, insuranceAccount, -pnl)
	return err
}

func (r *RiskRepository) coverLiquidationLoss(ctx context.Context, tx pgx.Tx, jobID string, user int64, symbol string, loss int64, at time.Time) error {
	if loss == 0 {
		return nil
	}
	var fundBalance int64
	if err := tx.QueryRow(ctx, `SELECT balance_after_atoms FROM insurance_fund_entries ORDER BY occurred_at DESC,id DESC LIMIT 1 FOR UPDATE`).Scan(&fundBalance); err != nil {
		return err
	}
	coverage, err := insurance.CoverLoss(fundBalance, loss)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO insurance_fund_entries(liquidation_job_id,amount_atoms,balance_after_atoms,reason,occurred_at) VALUES($1,$2,$3,'bankruptcy_loss',$4)`, jobID, -coverage.InsuranceDebitAtoms, coverage.RemainingFundAtoms, at); err != nil {
		return err
	}
	if coverage.ADLShortfallAtoms == 0 {
		return nil
	}
	rows, err := tx.Query(ctx, `SELECT p.user_id,p.size_atoms,p.entry_price_atoms,p.quantity_scale,m.mark_price_atoms,COALESCE(l.leverage,10) FROM positions p JOIN LATERAL(SELECT mark_price_atoms FROM mark_prices WHERE symbol=p.symbol ORDER BY created_at DESC,id DESC LIMIT 1)m ON true LEFT JOIN leverage_settings l ON l.user_id=p.user_id AND l.symbol=p.symbol WHERE p.symbol=$1 AND p.user_id<>$2 AND p.size_atoms<>0`, symbol, user)
	if err != nil {
		return err
	}
	var candidates []insurance.Candidate
	for rows.Next() {
		var candidateUser, size, entry, price, leverage int64
		var quantityScale uint8
		if err := rows.Scan(&candidateUser, &size, &entry, &quantityScale, &price, &leverage); err != nil {
			rows.Close()
			return err
		}
		profit, _ := position.UnrealizedPnL(position.State{SizeAtoms: size, EntryPriceAtoms: entry, QuantityScale: quantityScale}, price)
		if profit > 0 {
			candidates = append(candidates, insurance.Candidate{UserID: candidateUser, ProfitAtoms: profit, Leverage: leverage})
		}
	}
	rows.Close()
	ranked, _ := insurance.RankADL(candidates)
	for rank, candidate := range ranked {
		evidence, _ := json.Marshal(map[string]any{"profit_atoms": candidate.ProfitAtoms, "leverage": candidate.Leverage, "shortfall_atoms": coverage.ADLShortfallAtoms})
		if _, err := tx.Exec(ctx, `INSERT INTO adl_candidates(liquidation_job_id,user_id,symbol,profit_atoms,leverage,rank,evidence) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING`, jobID, candidate.UserID, symbol, candidate.ProfitAtoms, candidate.Leverage, rank+1, evidence); err != nil {
			return err
		}
	}
	return nil
}
