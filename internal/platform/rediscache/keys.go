// Package rediscache owns rebuildable caches and fail-closed security state.
package rediscache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

const keyNamespace = "exchange-lab:v1"

func privateKey(kind, value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" || len(normalized) > 512 {
		return "", fmt.Errorf("create private Redis key: invalid input")
	}
	digest := sha256.Sum256([]byte(normalized))
	return keyNamespace + ":" + kind + ":" + hex.EncodeToString(digest[:]), nil
}
