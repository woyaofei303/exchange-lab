package identity

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	minimumPasswordBytes = 12
	maximumPasswordBytes = 1024
)

// PasswordHasher 管理有界的 Argon2id 参数及其熵源。
type PasswordHasher struct {
	MemoryKiB   uint32
	Iterations  uint32
	Parallelism uint8
	SaltLength  uint32
	KeyLength   uint32
	Random      io.Reader
}

// DefaultPasswordHasher 返回经过审查的本地 Argon2id 配置。
func DefaultPasswordHasher() PasswordHasher {
	return PasswordHasher{MemoryKiB: 19 * 1024, Iterations: 2, Parallelism: 1, SaltLength: 16, KeyLength: 32, Random: rand.Reader}
}

// Hash 创建加盐编码后的 Argon2id 值，不保留明文。
func (h PasswordHasher) Hash(password string) (string, error) {
	if len(password) < minimumPasswordBytes || len(password) > maximumPasswordBytes || !h.valid() {
		return "", fmt.Errorf("hash password: invalid input")
	}
	salt := make([]byte, h.SaltLength)
	if _, err := io.ReadFull(h.Random, salt); err != nil {
		return "", fmt.Errorf("hash password: entropy unavailable")
	}
	key := argon2.IDKey([]byte(password), salt, h.Iterations, h.MemoryKiB, h.Parallelism, h.KeyLength)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, h.MemoryKiB, h.Iterations, h.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

// Verify 在常量时间比较前限制不可信编码参数。
func (h PasswordHasher) Verify(encoded, password string) bool {
	if len(password) < minimumPasswordBytes || len(password) > maximumPasswordBytes {
		return false
	}
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return false
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false
	}
	var memory, iterations uint32
	var parallelism uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil ||
		memory < 8 || memory > 64*1024 || iterations < 1 || iterations > 10 || parallelism < 1 || parallelism > 8 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) < 8 || len(salt) > 64 {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(want) < 16 || len(want) > 64 {
		return false
	}
	actual := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, uint32(len(want)))
	return subtle.ConstantTimeCompare(actual, want) == 1
}

func (h PasswordHasher) valid() bool {
	return h.MemoryKiB >= 8 && h.MemoryKiB <= 64*1024 && h.Iterations >= 1 && h.Iterations <= 10 &&
		h.Parallelism >= 1 && h.Parallelism <= 8 && h.SaltLength >= 8 && h.SaltLength <= 64 &&
		h.KeyLength >= 16 && h.KeyLength <= 64 && h.Random != nil
}
