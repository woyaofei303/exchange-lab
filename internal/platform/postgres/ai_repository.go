package postgres

import (
	"context"
	"exchange-lab/internal/ai"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AIRepository 持久化解释元数据和证据，绝不存储模型凭据。
type AIRepository struct {
	pool    *pgxpool.Pool
	timeout time.Duration
}

// NewAIRepository 创建有界的 AI 审计仓储。
func NewAIRepository(pool *pgxpool.Pool, timeout time.Duration) *AIRepository {
	return &AIRepository{pool: pool, timeout: timeout}
}

// Record 追加一条 AI 运行审计事实。
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
