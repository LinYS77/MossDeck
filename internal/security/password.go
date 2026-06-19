// Package security holds small, dependency-light primitives for credentials
// and tokens that more than one feature may need. Keeping them here (rather
// than inside internal/auth) makes the boundaries reusable: password hashing
// is the same whether it is used by auth setup, a future password-change
// endpoint, or import flows.
package security

import (
	"errors"
	"fmt"
	"unicode"

	"golang.org/x/crypto/bcrypt"
)

// MinPasswordLength is the minimum length enforced for new passwords.
const MinPasswordLength = 12

var ErrWeakPassword = errors.New("security: weak password")

// ValidatePasswordStrength requires a personal-deployment password to be long
// and varied enough for public internet exposure.
func ValidatePasswordStrength(password string) error {
	if len([]rune(password)) < MinPasswordLength {
		return fmt.Errorf("%w: must be at least %d characters", ErrWeakPassword, MinPasswordLength)
	}
	var classes int
	var hasLower, hasUpper, hasDigit, hasSymbol bool
	for _, r := range password {
		switch {
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsDigit(r):
			hasDigit = true
		case unicode.IsPunct(r) || unicode.IsSymbol(r) || unicode.IsSpace(r):
			hasSymbol = true
		}
	}
	for _, ok := range []bool{hasLower, hasUpper, hasDigit, hasSymbol} {
		if ok {
			classes++
		}
	}
	if classes < 3 {
		return fmt.Errorf("%w: use at least three of lowercase, uppercase, digits, and symbols", ErrWeakPassword)
	}
	return nil
}

// HashPassword returns a bcrypt hash of the plaintext password at the given
// cost. The cost is configurable so tests can use bcrypt.MinCost for speed
// while production uses a strong default.
func HashPassword(password string, cost int) (string, error) {
	if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
		return "", fmt.Errorf("security: invalid bcrypt cost %d", cost)
	}
	// bcrypt.GenerateFromPassword truncates passwords longer than 72 bytes; we
	// accept that trade-off for an MVP single-user app and document it rather
	// than pre-hashing, which would weaken the scheme.
	h, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return "", fmt.Errorf("security: hash password: %w", err)
	}
	return string(h), nil
}

// CheckPassword reports whether password matches the stored bcrypt hash.
// It returns a non-nil error when the hash is malformed or the password does
// not match; callers should treat any non-nil error as "incorrect".
func CheckPassword(hashedPassword, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
}
