package identity

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
)

// TokenHash is the only session-token representation stored by the server.
type TokenHash [sha256.Size]byte

// TokenIssuer creates cryptographically random opaque session tokens.
type TokenIssuer struct {
	Random io.Reader
}

// Issue returns one plaintext token and its stable SHA-256 storage hash.
func (i TokenIssuer) Issue() (string, TokenHash, error) {
	reader := i.Random
	if reader == nil {
		reader = rand.Reader
	}
	payload := make([]byte, 32)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return "", TokenHash{}, fmt.Errorf("issue session token: entropy unavailable")
	}
	token := base64.RawURLEncoding.EncodeToString(payload)
	return token, HashToken(token), nil
}

// HashToken converts an opaque token to its non-secret lookup key.
func HashToken(token string) TokenHash {
	return sha256.Sum256([]byte(token))
}
