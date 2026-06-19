-- 0001_init.sql
-- Foundational schema for the personal password-lock auth feature.
--
-- Mossdeck is single-owner software. The users table stores the one local
-- owner credential and keeps user_id as a stable ownership boundary for
-- bookmarks, read-later items, sessions, and backups.

CREATE TABLE users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    password_hash TEXT    NOT NULL,
    last_login_at TEXT,
    created_at    TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at    TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE sessions (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT    NOT NULL UNIQUE,
    user_agent  TEXT,
    ip_address  TEXT,
    expires_at  TEXT    NOT NULL,
    created_at  TEXT    NOT NULL DEFAULT (datetime('now')),
    revoked_at  TEXT
);

CREATE INDEX idx_sessions_user  ON sessions(user_id);
CREATE INDEX idx_sessions_token ON sessions(token_hash);
