package identity

import (
	"bytes"
	"testing"
)

func TestTokenIssuerProducesOpaqueTokenAndStableHash(t *testing.T) {
	issuer := TokenIssuer{Random: bytes.NewReader(bytes.Repeat([]byte{9}, 64))}
	token, hash, err := issuer.Issue()
	if err != nil {
		t.Fatal(err)
	}
	if len(token) < 40 || token == string(hash[:]) {
		t.Fatalf("Issue() returned an invalid opaque token")
	}
	if HashToken(token) != hash {
		t.Fatal("HashToken() mismatch")
	}
}

func TestRoleAuthorization(t *testing.T) {
	if !RoleAdmin.Allows(RoleUser) || !RoleAdmin.Allows(RoleAdmin) || RoleUser.Allows(RoleAdmin) {
		t.Fatal("role hierarchy mismatch")
	}
	if Role("owner").Valid() {
		t.Fatal("unknown role accepted")
	}
}
