package security

import (
	"strings"
	"testing"
)

func TestHashAndCheckPassword(t *testing.T) {
	hash, err := HashPassword("correct horse", 4) // min cost for speed in tests
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if hash == "" || strings.Contains(hash, "correct horse") {
		t.Fatal("hash must be non-empty and must not contain the plaintext")
	}
	if err := CheckPassword(hash, "correct horse"); err != nil {
		t.Fatalf("CheckPassword correct: %v", err)
	}
	if err := CheckPassword(hash, "battery staple"); err == nil {
		t.Fatal("expected mismatch for wrong password")
	}
}

func TestHashPasswordUnique(t *testing.T) {
	a, _ := HashPassword("same", 4)
	b, _ := HashPassword("same", 4)
	if a == b {
		t.Fatal("bcrypt hashes for the same password must differ (salt)")
	}
}

func TestHashPasswordRejectsBadCost(t *testing.T) {
	if _, err := HashPassword("x", 3); err == nil {
		t.Fatal("expected error for cost below minimum")
	}
	if _, err := HashPassword("x", 32); err == nil {
		t.Fatal("expected error for cost above maximum")
	}
}

func TestGenerateTokenUniqueAndLength(t *testing.T) {
	seen := make(map[string]struct{}, 256)
	for i := 0; i < 256; i++ {
		tok, err := GenerateToken()
		if err != nil {
			t.Fatalf("GenerateToken: %v", err)
		}
		if tok == "" {
			t.Fatal("empty token")
		}
		if _, dup := seen[tok]; dup {
			t.Fatalf("duplicate token generated: %q", tok)
		}
		seen[tok] = struct{}{}
		// 32 random bytes -> 52 base32 chars (no padding).
		if len(tok) != 52 {
			t.Fatalf("token length = %d, want 52", len(tok))
		}
	}
}

func TestHashTokenDeterministic(t *testing.T) {
	if HashToken("abc") != HashToken("abc") {
		t.Fatal("HashToken must be deterministic")
	}
	if HashToken("abc") == HashToken("abd") {
		t.Fatal("distinct inputs must hash distinctly")
	}
}

func TestHashEqual(t *testing.T) {
	h := HashToken("abc")
	if !HashEqual(h, HashToken("abc")) {
		t.Fatal("expected equal hashes to compare equal")
	}
	if HashEqual(h, HashToken("abd")) {
		t.Fatal("expected unequal hashes to compare unequal")
	}
}
