package postgres

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/woyaofei303/exchange-lab/internal/ai"
	"time"
)

// AIRepository persists explanation metadata and evidence, never model credentials.
type AIRepository struct {
	pool    *pgxpool.Pool
	timeout time.Duration
}

// NewAIRepository constructs the bounded AI audit repository.
func NewAIRepository(pool *pgxpool.Pool, timeout time.Duration) *AIRepository {
	return &AIRepository{pool: pool, timeout: timeout}
}

// Record appends one AI run audit fact.
func (r *AIRepository) Record(ctx context.Context, record ai.RunRecord) error {
	op, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	id, err := randomBusinessID("airun_")
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(op, `INSERT INTO ai_runs(id,user_id,symbol,prompt_version,model,input_tokens,output_tokens,latency_ms,evidence_ids,status,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, id, record.UserID, record.Symbol, record.PromptVersion, record.Model, record.Usage.InputTokens, record.Usage.OutputTokens, record.Latency.Milliseconds(), record.EvidenceIDs, record.Status, record.CreatedAt.UTC())
	if err != nil {
		return fmt.Errorf("record AI run: %w", err)
	}
	return nil
}
