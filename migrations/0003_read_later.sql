-- 0003_read_later.sql
-- Read-later core data model.
--
-- Scope: read-later CRUD, state transitions, soft-delete/restore, open
-- tracking, favorite/priority, search/filter, and tags via the existing tags
-- table. Every table/index carries user_id for mandatory user isolation.

CREATE TABLE read_later_items (
    id                   INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id              INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    url                  TEXT    NOT NULL,
    normalized_url       TEXT    NOT NULL,
    title                TEXT    NOT NULL DEFAULT '',
    excerpt              TEXT    NOT NULL DEFAULT '',
    author               TEXT    NOT NULL DEFAULT '',
    site_name            TEXT    NOT NULL DEFAULT '',
    favicon_url          TEXT,
    cover_image_url      TEXT,
    domain               TEXT    NOT NULL DEFAULT '',
    reading_time_minutes INTEGER NOT NULL DEFAULT 0,
    -- state drives the list/soft-delete flow:
    --   unread   -> default newly saved item
    --   reading  -> user has opened/started it
    --   archived -> hidden unless requested
    --   trash    -> soft-deleted; URL is freed for re-create
    state                TEXT    NOT NULL DEFAULT 'unread',
    priority             INTEGER NOT NULL DEFAULT 0,
    favorite             INTEGER NOT NULL DEFAULT 0,
    source               TEXT    NOT NULL DEFAULT '',
    last_opened_at       TEXT,
    archived_at          TEXT,
    metadata_status      TEXT    NOT NULL DEFAULT 'pending',
    created_at           TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at           TEXT    NOT NULL DEFAULT (datetime('now')),
    deleted_at           TEXT
);

-- A user cannot have two non-trash read-later items for the same normalized URL.
CREATE UNIQUE INDEX uq_read_later_user_normurl
    ON read_later_items(user_id, normalized_url) WHERE state != 'trash';
CREATE INDEX idx_read_later_user_state_created ON read_later_items(user_id, state, created_at);
CREATE INDEX idx_read_later_user_domain ON read_later_items(user_id, domain);
CREATE INDEX idx_read_later_user_favorite ON read_later_items(user_id, favorite);
CREATE INDEX idx_read_later_user_priority ON read_later_items(user_id, priority);

CREATE TABLE read_later_tags (
    read_later_id INTEGER NOT NULL REFERENCES read_later_items(id) ON DELETE CASCADE,
    tag_id        INTEGER NOT NULL REFERENCES tags(id)             ON DELETE CASCADE,
    user_id       INTEGER NOT NULL REFERENCES users(id)            ON DELETE CASCADE,
    PRIMARY KEY (read_later_id, tag_id)
);

CREATE INDEX idx_read_later_tags_tag ON read_later_tags(tag_id);
CREATE INDEX idx_read_later_tags_user ON read_later_tags(user_id);
