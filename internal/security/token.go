package security

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/hex"
)

// tokenBytes is the amount of randomness in a session token. 32 bytes gives
// 256 bits of entropy, far beyond any practical brute-force.
const tokenBytes = 32

// encoding is base32 without padding: URL/cookie safe and compact.
var encoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// GenerateToken returns a cryptographically random, cookie-safe opaque token.
// The raw token is handed to the client (via cookie) only; the server stores
// HashToken(token) so that a database leak cannot be replayed as credentials.
func GenerateToken() (string, error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return encoding.EncodeToString(b), nil
}

// HashToken returns the lowercase SHA-256 hex digest of a token. SHA-256 is
// sufficient for an opaque high-entropy token (unlike passwords, tokens do
// not need a slow KDF because their entropy makes brute force infeasible).
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// HashEqual reports whether two hex token hashes are equal in constant time.
// It avoids leaking length/timing information when comparing stored hashes.
func HashEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
