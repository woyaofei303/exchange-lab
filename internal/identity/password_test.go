package identity

import (
	"bytes"
	"strings"
	"testing"
)

func TestPasswordHasherHashAndVerify(t *testing.T) {
	hasher := testHasher()
	hash, err := hasher.Hash("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(hash, "correct horse") || !strings.HasPrefix(hash, "$argon2id$v=19$") {
		t.Fatalf("Hash() = %q", hash)
	}
	if ok := hasher.Verify(hash, "correct horse battery staple"); !ok {
		t.Fatal("Verify() = false")
	}
	if ok := hasher.Verify(hash, "wrong password"); ok {
		t.Fatal("Verify(wrong) = true")
	}
}

func TestPasswordHasherRejectsInvalidInputsAndHashes(t *testing.T) {
	hasher := testHasher()
	for _, password := range []string{"short", strings.Repeat("x", 1025)} {
		if _, err := hasher.Hash(password); err == nil {
			t.Fatalf("Hash(%d bytes) error = nil", len(password))
		}
	}
	for _, encoded := range []string{"", "$argon2id$bad", "$argon2id$v=18$m=8,t=1,p=1$YQ$Yg"} {
		if hasher.Verify(encoded, "correct horse battery staple") {
			t.Fatalf("Verify(%q) = true", encoded)
		}
	}
}

func testHasher() PasswordHasher {
	return PasswordHasher{MemoryKiB: 8, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32, Random: bytes.NewReader(bytes.Repeat([]byte{7}, 1024))}
}
