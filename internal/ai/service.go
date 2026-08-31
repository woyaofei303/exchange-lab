package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"exchange-lab/internal/risk"
	"fmt"
	"io"
	"time"
)

// ErrInvalidOutput 表示模型输出违反结构或证据约束。
var ErrInvalidOutput = errors.New("invalid AI structured output")

// Summary 是系统接受的唯一 AI 模型结构化解释格式。
type Summary struct {
	Trend                  string   `json:"trend"`
	Position               string   `json:"position"`
	MarginRatioBPS         int64    `json:"margin_ratio_bps"`
	LiquidationDistanceBPS int64    `json:"liquidation_distance_bps"`
	FundingRateBPS         int64    `json:"funding_rate_bps"`
	Risks                  []string `json:"risks"`
	EvidenceIDs            []string `json:"evidence_ids"`
	Disclaimer             string   `json:"disclaimer"`
}

// Usage 记录模型令牌数量，作为审计和成本证据。
type Usage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
}

// Model 根据提示词和 JSON Schema 生成一条结构化解释。
type Model interface {
	Generate(context.Context, string, map[string]any) ([]byte, Usage, error)
}

// Source 提供解释可以引用的服务端计算事实。
type Source interface {
	Snapshot(context.Context, int64, string) (risk.Snapshot, error)
	FundingHistory(context.Context, string) ([]risk.FundingRecord, error)
	Liquidations(context.Context, int64) ([]risk.LiquidationRecord, error)
}

// RunRecord 是一次解释尝试的可审计元数据。
type RunRecord struct {
	UserID                       int64
	Symbol, PromptVersion, Model string
	Usage                        Usage
	Latency                      time.Duration
	EvidenceIDs                  []string
	Status                       string
	CreatedAt                    time.Time
}

// AuditStore 持久化 AI 运行元数据，但不将其作为交易事实。
type AuditStore interface {
	Record(context.Context, RunRecord) error
}

// Service 将模型输出绑定到经过身份验证的服务端计算证据。
type Service struct {
	source                   Source
	model                    Model
	audit                    AuditStore
	modelName, promptVersion string
	now                      func() time.Time
}
type modelInput struct {
	Instruction        string                   `json:"instruction"`
	Snapshot           risk.Snapshot            `json:"snapshot"`
	Funding            []risk.FundingRecord     `json:"funding"`
	Liquidations       []risk.LiquidationRecord `json:"liquidations"`
	AllowedEvidenceIDs []string                 `json:"allowed_evidence_ids"`
}

// NewService 校验所有证据、模型和审计依赖项。
func NewService(source Source, model Model, audit AuditStore, modelName, promptVersion string, now func() time.Time) (*Service, error) {
	if source == nil || model == nil || audit == nil || modelName == "" || promptVersion == "" || now == nil {
		return nil, fmt.Errorf("create AI service: invalid configuration")
	}
	return &Service{source: source, model: model, audit: audit, modelName: modelName, promptVersion: promptVersion, now: now}, nil
}

// Summarize 生成并审计风险解释，拒绝虚构的证据或数字。
func (s *Service) Summarize(ctx context.Context, userID int64, symbol string) (Summary, Usage, error) {
	started := s.now()
	snapshot, err := s.source.Snapshot(ctx, userID, symbol)
	if err != nil {
		return Summary{}, Usage{}, err
	}
	fundingHistory, err := s.source.FundingHistory(ctx, symbol)
	if err != nil {
		return Summary{}, Usage{}, err
	}
	liquidations, err := s.source.Liquidations(ctx, userID)
	if err != nil {
		return Summary{}, Usage{}, err
	}
	ids := []string{fmt.Sprintf("risk:%d:%s:%d", userID, symbol, snapshot.RiskVersion), fmt.Sprintf("mark:%s:%d", symbol, snapshot.AsOf.UnixNano())}
	if len(fundingHistory) > 0 {
		ids = append(ids, fmt.Sprintf("funding:%d", fundingHistory[0].ID))
	}
	for _, item := range liquidations {
		ids = append(ids, "liquidation:"+item.ID)
	}
	input := modelInput{Instruction: "Explain only the supplied server facts. Never promise returns, predict a liquidation trigger, or invent evidence. Preserve all numeric facts exactly.", Snapshot: snapshot, Funding: fundingHistory, Liquidations: liquidations, AllowedEvidenceIDs: ids}
	prompt, _ := json.Marshal(input)
	payload, usage, modelErr := s.model.Generate(ctx, string(prompt), SummarySchema())
	status := "completed"
	if modelErr != nil {
		status = "failed"
	}
	record := RunRecord{UserID: userID, Symbol: symbol, PromptVersion: s.promptVersion, Model: s.modelName, Usage: usage, Latency: s.now().Sub(started), EvidenceIDs: ids, Status: status, CreatedAt: started.UTC()}
	auditErr := s.audit.Record(ctx, record)
	if modelErr != nil || auditErr != nil {
		return Summary{}, usage, errors.Join(modelErr, auditErr)
	}
	summary, err := decodeSummary(payload)
	if err != nil {
		return Summary{}, usage, err
	}
	allowed := map[string]bool{}
	for _, id := range ids {
		allowed[id] = true
	}
	if len(summary.EvidenceIDs) == 0 {
		return Summary{}, usage, ErrInvalidOutput
	}
	for _, id := range summary.EvidenceIDs {
		if !allowed[id] {
			return Summary{}, usage, ErrInvalidOutput
		}
	}
	if summary.MarginRatioBPS != snapshot.MarginRatioBPS || summary.LiquidationDistanceBPS != snapshot.LiquidationDistanceBPS {
		return Summary{}, usage, ErrInvalidOutput
	}
	return summary, usage, nil
}
func decodeSummary(payload []byte) (Summary, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var summary Summary
	if err := decoder.Decode(&summary); err != nil {
		return Summary{}, ErrInvalidOutput
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF || summary.Trend == "" || summary.Position == "" || summary.Disclaimer == "" || len(summary.Risks) > 20 {
		return Summary{}, ErrInvalidOutput
	}
	return summary, nil
}

// SummarySchema 返回与结构化输出模型共享的严格 JSON Schema。
func SummarySchema() map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false, "required": []string{"trend", "position", "margin_ratio_bps", "liquidation_distance_bps", "funding_rate_bps", "risks", "evidence_ids", "disclaimer"}, "properties": map[string]any{"trend": map[string]any{"type": "string"}, "position": map[string]any{"type": "string"}, "margin_ratio_bps": map[string]any{"type": "integer"}, "liquidation_distance_bps": map[string]any{"type": "integer"}, "funding_rate_bps": map[string]any{"type": "integer"}, "risks": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "evidence_ids": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "disclaimer": map[string]any{"type": "string"}}}
}
