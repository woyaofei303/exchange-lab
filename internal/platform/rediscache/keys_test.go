package rediscache

import (
	"strings"
	"testing"
)

func TestPrivateKeyNeverContainsRawIdentityOrToken(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"User@Example.Test", "raw-session-token-secret"} {
		key, err := privateKey("security:test", value)
		if err != nil {
			t.Fatalf("privateKey() error = %v", err)
		}
		if strings.Contains(strings.ToLower(key), strings.ToLower(value)) {
			t.Fatalf("private key leaked raw value: %s", key)
		}
		if !strings.HasPrefix(key, "exchange-lab:v1:security:test:") {
			t.Fatalf("private key prefix = %q", key)
		}
	}
}

func TestPrivateKeyRejectsEmptyOrUnboundedInput(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"", "   ", strings.Repeat("x", 513)} {
		if _, err := privateKey("security:test", value); err == nil {
			t.Fatalf("privateKey(%q) error = nil", value)
		}
	}
}
