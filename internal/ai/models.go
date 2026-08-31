// Package ai produces evidence-bound risk explanations without trading writes.
package ai

import (
	"context"
	"encoding/json"
	"fmt"
	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"strings"
)

// DeterministicModel provides a local, repeatable fallback with no network dependency.
type DeterministicModel struct{}

// Generate returns a conservative summary derived only from the supplied facts.
func (DeterministicModel) Generate(_ context.Context, prompt string, _ map[string]any) ([]byte, Usage, error) {
	var input modelInput
	if err := json.Unmarshal([]byte(prompt), &input); err != nil {
		return nil, Usage{}, err
	}
	funding := int64(0)
	if len(input.Funding) > 0 {
		funding = input.Funding[0].FinalRateBPS
	}
	positionName := "flat"
	if input.Snapshot.PositionSizeAtoms > 0 {
		positionName = "long"
	} else if input.Snapshot.PositionSizeAtoms < 0 {
		positionName = "short"
	}
	risks := []string{"mark price can change quickly"}
	if !input.Snapshot.Fresh {
		risks = append(risks, "risk price is stale")
	}
	summary := Summary{Trend: "neutral", Position: positionName, MarginRatioBPS: input.Snapshot.MarginRatioBPS, LiquidationDistanceBPS: input.Snapshot.LiquidationDistanceBPS, FundingRateBPS: funding, Risks: risks, EvidenceIDs: input.AllowedEvidenceIDs, Disclaimer: "教学模拟说明，不构成收益承诺或投资建议。"}
	payload, err := json.Marshal(summary)
	return payload, Usage{}, err
}

// OpenAIModel adapts structured Responses output to the Model contract.
type OpenAIModel struct {
	client openai.Client
	model  string
}

// NewOpenAIModel validates credentials and model name without logging either value.
func NewOpenAIModel(apiKey, model string) (*OpenAIModel, error) {
	if strings.TrimSpace(apiKey) == "" || strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("create OpenAI model: missing configuration")
	}
	return &OpenAIModel{client: openai.NewClient(option.WithAPIKey(apiKey)), model: model}, nil
}

// Generate requests JSON-schema-constrained output and returns token usage.
func (m *OpenAIModel) Generate(ctx context.Context, prompt string, schema map[string]any) ([]byte, Usage, error) {
	response, err := m.client.Responses.New(ctx, responses.ResponseNewParams{Model: responses.ResponsesModel(m.model), Input: responses.ResponseNewParamsInputUnion{OfString: openai.String(prompt)}, Text: responses.ResponseTextConfigParam{Format: responses.ResponseFormatTextConfigParamOfJSONSchema("contract_risk_summary", schema)}})
	if err != nil {
		return nil, Usage{}, fmt.Errorf("OpenAI response failed")
	}
	return []byte(response.OutputText()), Usage{InputTokens: response.Usage.InputTokens, OutputTokens: response.Usage.OutputTokens}, nil
}
