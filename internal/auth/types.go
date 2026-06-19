// Package auth implements authentication and session management for the
// homepage backend: first-run admin setup, login, logout, current-user
// retrieval, the session cookie, and the RequireAuth middleware that future
// protected routes wrap.
//
// The package is intentionally layered so each concern is testable in
// isolation:
//
//   - Store    : persistence (users/sessions) behind an interface, so the
//     business logic never talks to SQL directly.
//   - Service  : business rules (setup, login, session resolution, logout).
//   - handlers : HTTP decoding, response shaping, and rate-limit accounting.
//
// Security posture:
//   - Passwords are bcrypt-hashed; plaintext is never stored or logged.
//   - Session tokens are opaque, high-entropy, and stored only as a SHA-256
//     hash; the raw token lives only in the client cookie.
//   - Login never reveals whether a username exists (uniform 401 + a dummy
//     hash comparison to flatten timing).
//   - Cookies are HttpOnly, SameSite, and Secure (in production).
//   - Failed logins are rate-limited per (ip, username).
package auth

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// Sentinel domain errors returned by the Store and Service. Handlers map these
// to HTTP responses; callers must use errors.Is to test for them.
var (
	// ErrUserNotFound is returned by the store when no user matches a query.
	ErrUserNotFound = errors.New("auth: user not found")
	// ErrSessionNotFound is returned by the store when no valid (non-revoked,
	// non-expired) session matches a token hash.
	ErrSessionNotFound = errors.New("auth: session not found")
	// ErrUsernameTaken is returned when creating a user with an existing username.
	ErrUsernameTaken = errors.New("auth: username already taken")
)

// Service-level domain errors.
var (
	ErrAlreadyInitialized = errors.New("auth: setup has already been completed")
	ErrSetupDisabled      = errors.New("auth: setup is disabled")
	ErrInvalidCredentials = errors.New("auth: invalid credentials")
	ErrUserInactive       = errors.New("auth: user is not active")
	ErrSessionInvalid     = errors.New("auth: session is invalid or expired")
)

// ValidationError describes an invalid request field. Handlers map it to a
// 400 with a human-readable message.
type ValidationError struct {
	Field  string
	Reason string
}

func (e ValidationError) Error() string {
	if e.Field == "" {
		return e.Reason
	}
	return e.Field + ": " + e.Reason
}

// dbTimeFormat is the SQLite TEXT timestamp format produced by datetime('now').
// Storing Go-computed timestamps (e.g. expires_at) in the same format keeps
// lexical comparisons in SQL correct.
const dbTimeFormat = "2006-01-02 15:04:05"

// User is the in-memory representation of a users row. Nullable columns are
// surfaced as empty strings for simplicity.
type User struct {
	ID           int64
	Username     string
	Email        string
	PasswordHash string
	DisplayName  string
	AvatarURL    string
	Role         string
	Status       string
	LastLoginAt  string
	CreatedAt    string
	UpdatedAt    string
}

// Session is the in-memory representation of a valid sessions row.
type Session struct {
	ID        int64
	UserID    int64
	TokenHash string
	ExpiresAt string
	CreatedAt string
	UserAgent string
	IPAddress string
}

// CreateUserParams holds the fields needed to insert a new user.
type CreateUserParams struct {
	Username     string
	Email        string
	PasswordHash string
	DisplayName  string
	Role         string
	Status       string
}

// CreateSessionParams holds the fields needed to insert a new session row.
type CreateSessionParams struct {
	UserID    int64
	TokenHash string
	ExpiresAt string // dbTimeFormat, UTC
	UserAgent string
	IPAddress string
}

// SetupParams is the input to Service.Setup.
type SetupParams struct {
	Username    string
	Password    string
	Email       string
	DisplayName string
}

// Store is the persistence boundary for auth. The concrete implementation is a
// thin SQLite wrapper; tests may substitute a fake to exercise the Service
// without a database.
//
//go:generate mockery --name Store --with-expecter  (optional; not required by the build)
type Store interface {
	// CountUsers returns the total number of registered users.
	CountUsers(ctx context.Context) (int, error)
	// CreateUser inserts a user and returns the full row.
	CreateUser(ctx context.Context, p CreateUserParams) (*User, error)
	// GetUserByUsername returns the user or ErrUserNotFound.
	GetUserByUsername(ctx context.Context, username string) (*User, error)
	// GetUserByID returns the user or ErrUserNotFound.
	GetUserByID(ctx context.Context, id int64) (*User, error)
	// CreateSession inserts a session row.
	CreateSession(ctx context.Context, p CreateSessionParams) error
	// GetSessionByTokenHash returns the active (non-revoked, non-expired)
	// session for a hash, or ErrSessionNotFound.
	GetSessionByTokenHash(ctx context.Context, hash string) (*Session, error)
	// RevokeSessionByTokenHash marks a session revoked; it is idempotent.
	RevokeSessionByTokenHash(ctx context.Context, hash string) error
	// TouchLogin updates last_login_at (and updated_at) for a user.
	TouchLogin(ctx context.Context, userID int64, at string) error
}

// scanner is satisfied by *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

// scanUser maps a users row into a *User.
func scanUser(s scanner) (*User, error) {
	var u User
	var email, displayName, avatar, lastLogin sql.NullString
	err := s.Scan(
		&u.ID, &u.Username, &email, &u.PasswordHash, &displayName, &avatar,
		&u.Role, &u.Status, &lastLogin, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	u.Email = email.String
	u.DisplayName = displayName.String
	u.AvatarURL = avatar.String
	u.LastLoginAt = lastLogin.String
	return &u, nil
}

// nullIfEmpty returns nil for an empty string so optional TEXT columns are
// stored as NULL rather than "".
func nullIfEmpty(s string) any {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return s
}

// isUniqueViolation reports whether err is a SQLite UNIQUE constraint failure.
// The modernc driver surfaces these as "... UNIQUE constraint failed: ...".
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
