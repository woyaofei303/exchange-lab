package ai

import (
	"encoding/json"
	"testing"
)

func TestUsageJSONUsesPublicSnakeCaseContract(t *testing.T) {
	payload, err := json.Marshal(Usage{InputTokens: 10, OutputTokens: 20})
	if err != nil {
		t.Fatalf("marshal usage: %v", err)
	}
	if got, want := string(payload), `{"input_tokens":10,"output_tokens":20}`; got != want {
		t.Fatalf("usage JSON = %s, want %s", got, want)
	}
}
