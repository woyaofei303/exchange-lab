package identity

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
)

// TokenHash 是服务端存储的唯一会话令牌表示形式。
type TokenHash [sha256.Size]byte

// TokenIssuer 创建密码学随机的不透明会话令牌。
type TokenIssuer struct {
	Random io.Reader
}

// Issue 返回一个明文令牌及其稳定的 SHA-256 存储哈希。
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

// HashToken 将不透明令牌转换为非敏感查询键。
func HashToken(token string) TokenHash {
	return sha256.Sum256([]byte(token))
}
