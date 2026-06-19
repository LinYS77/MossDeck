// Package auth implements Mossdeck's personal password-lock authentication:
// first-run owner password setup, password login, logout, current-user
// retrieval, the session cookie, and the RequireAuth middleware.
//
// Mossdeck is single-owner software. There is no registration, public account
// creation, named-account login, or multi-user collaboration surface.
package auth

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

var (
	ErrUserNotFound    = errors.New("auth: owner not found")
	ErrSessionNotFound = errors.New("auth: session not found")
)

var (
	ErrAlreadyInitialized = errors.New("auth: setup has already been completed")
	ErrSetupDisabled      = errors.New("auth: setup is disabled")
	ErrInvalidSetupToken  = errors.New("auth: invalid setup token")
	ErrInvalidCredentials = errors.New("auth: invalid credentials")
	ErrSessionInvalid     = errors.New("auth: session is invalid or expired")
)

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

const dbTimeFormat = "2006-01-02 15:04:05"

type User struct {
	ID           int64
	PasswordHash string
	LastLoginAt  string
	CreatedAt    string
	UpdatedAt    string
}

type Session struct {
	ID        int64
	UserID    int64
	TokenHash string
	ExpiresAt string
	CreatedAt string
	UserAgent string
	IPAddress string
}

type CreateUserParams struct {
	PasswordHash string
}

type CreateSessionParams struct {
	UserID    int64
	TokenHash string
	ExpiresAt string
	UserAgent string
	IPAddress string
}

type SetupParams struct {
	Password        string
	ConfirmPassword string
	SetupToken      string
}

type Store interface {
	CountUsers(ctx context.Context) (int, error)
	CreateUser(ctx context.Context, p CreateUserParams) (*User, error)
	GetFirstUser(ctx context.Context) (*User, error)
	GetUserByID(ctx context.Context, id int64) (*User, error)
	CreateSession(ctx context.Context, p CreateSessionParams) error
	GetSessionByTokenHash(ctx context.Context, hash string) (*Session, error)
	RevokeSessionByTokenHash(ctx context.Context, hash string) error
	TouchLogin(ctx context.Context, userID int64, at string) error
}

type scanner interface {
	Scan(dest ...any) error
}

func scanUser(s scanner) (*User, error) {
	var u User
	var lastLogin sql.NullString
	err := s.Scan(&u.ID, &u.PasswordHash, &lastLogin, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	u.LastLoginAt = lastLogin.String
	return &u, nil
}

func nullIfEmpty(s string) any {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return s
}
