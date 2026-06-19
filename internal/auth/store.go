package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// sqliteStore is the production Store, backed by *sql.DB (modernc.org/sqlite).
// It is concurrency-safe because *sql.DB manages a connection pool.
type sqliteStore struct {
	db *sql.DB
}

// NewStore returns a Store backed by the given database. The users and
// sessions tables must already exist (see migrations/0001_init.sql).
func NewStore(db *sql.DB) Store {
	return &sqliteStore{db: db}
}

func (s *sqliteStore) CountUsers(ctx context.Context) (int, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		return 0, fmt.Errorf("auth: count users: %w", err)
	}
	return n, nil
}

func (s *sqliteStore) CreateUser(ctx context.Context, p CreateUserParams) (*User, error) {
	res, err := s.db.ExecContext(ctx, `
INSERT INTO users (username, email, password_hash, display_name, role, status)
VALUES (?, ?, ?, ?, ?, ?)`,
		p.Username, nullIfEmpty(p.Email), p.PasswordHash,
		nullIfEmpty(p.DisplayName), defaultStr(p.Role, "admin"), defaultStr(p.Status, "active"),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrUsernameTaken
		}
		return nil, fmt.Errorf("auth: create user: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("auth: create user last id: %w", err)
	}
	return s.GetUserByID(ctx, id)
}

func (s *sqliteStore) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, username, email, password_hash, display_name, avatar_url, role, status, last_login_at, created_at, updated_at
FROM users WHERE username = ?`, username)
	u, err := scanUser(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("auth: get user by username: %w", err)
	}
	return u, nil
}

func (s *sqliteStore) GetUserByID(ctx context.Context, id int64) (*User, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, username, email, password_hash, display_name, avatar_url, role, status, last_login_at, created_at, updated_at
FROM users WHERE id = ?`, id)
	u, err := scanUser(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("auth: get user by id: %w", err)
	}
	return u, nil
}

func (s *sqliteStore) CreateSession(ctx context.Context, p CreateSessionParams) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO sessions (user_id, token_hash, expires_at, user_agent, ip_address)
VALUES (?, ?, ?, ?, ?)`,
		p.UserID, p.TokenHash, p.ExpiresAt, nullIfEmpty(p.UserAgent), nullIfEmpty(p.IPAddress),
	)
	if err != nil {
		return fmt.Errorf("auth: create session: %w", err)
	}
	return nil
}

func (s *sqliteStore) GetSessionByTokenHash(ctx context.Context, hash string) (*Session, error) {
	var sess Session
	err := s.db.QueryRowContext(ctx, `
SELECT id, user_id, token_hash, expires_at, created_at,
       COALESCE(user_agent, ''), COALESCE(ip_address, '')
FROM sessions
WHERE token_hash = ? AND revoked_at IS NULL AND expires_at > datetime('now')`, hash).
		Scan(&sess.ID, &sess.UserID, &sess.TokenHash, &sess.ExpiresAt, &sess.CreatedAt,
			&sess.UserAgent, &sess.IPAddress)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrSessionNotFound
		}
		return nil, fmt.Errorf("auth: get session: %w", err)
	}
	return &sess, nil
}

func (s *sqliteStore) RevokeSessionByTokenHash(ctx context.Context, hash string) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE sessions SET revoked_at = datetime('now')
WHERE token_hash = ? AND revoked_at IS NULL`, hash)
	if err != nil {
		return fmt.Errorf("auth: revoke session: %w", err)
	}
	return nil
}

func (s *sqliteStore) TouchLogin(ctx context.Context, userID int64, at string) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE users SET last_login_at = ?, updated_at = datetime('now') WHERE id = ?`, at, userID)
	if err != nil {
		return fmt.Errorf("auth: touch login: %w", err)
	}
	return nil
}

// defaultStr returns v when non-empty, otherwise def.
func defaultStr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
