-- 0001_init.sql
-- Foundational schema for the authentication feature (users, sessions).
--
-- This migration lays only the auth data model so the next phase can add
-- login/session logic on top. Bookmark, read-later, category, tag, settings,
-- jobs, and audit tables are deliberately deferred to their own migrations in
-- later tasks to keep each change focused and reviewable.
--
-- Conventions:
--   * Timestamps are stored as ISO-8601 TEXT via datetime('now') (UTC).
--   * Soft-delete columns (deleted_at) are added per-table as needed later.
--   * INTEGER PRIMARY KEY AUTOINCREMENT is used for stable, sortable ids.

CREATE TABLE users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    username      TEXT    NOT NULL UNIQUE,
    email         TEXT,
    password_hash TEXT    NOT NULL,
    display_name  TEXT,
    avatar_url    TEXT,
    -- role is reserved for future granularity; MVP has a single admin.
    role          TEXT    NOT NULL DEFAULT 'admin',
    status        TEXT    NOT NULL DEFAULT 'active',
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
