-- 0002_bookmarks.sql
-- Bookmark core data model: categories, tags, bookmarks, and the
-- bookmark_tags many-to-many join table.
--
-- Scope (phase): categories/tags CRUD, bookmarks CRUD, search/filter, soft
-- delete + restore, archive, click/open tracking. HTML bookmark import and
-- metadata fetching are deferred to later migrations/tasks; the schema is
-- shaped to accept them without further changes.
--
-- Multi-user isolation: every table carries user_id and every index prefixes
-- with user_id so queries are always scoped to one user. The app is
-- single-user today, but the boundary is mandatory and not optional.
--
-- Conventions (match 0001_init.sql):
--   * Timestamps are ISO-8601 TEXT via datetime('now') (UTC).
--   * INTEGER PRIMARY KEY AUTOINCREMENT for stable, sortable ids.
--   * Soft delete uses a status of 'trash' (bookmarks) / 'archived' flag
--     (categories) rather than physical deletion; nothing is hard-deleted.

CREATE TABLE categories (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    parent_id   INTEGER REFERENCES categories(id) ON DELETE SET NULL,
    name        TEXT    NOT NULL,
    slug        TEXT,
    type        TEXT    NOT NULL DEFAULT 'bookmark',  -- bookmark | read_later | all
    icon        TEXT,
    color       TEXT,
    sort_order  INTEGER NOT NULL DEFAULT 0,
    archived    INTEGER NOT NULL DEFAULT 0,           -- 0 = active, 1 = archived
    created_at  TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE UNIQUE INDEX uq_categories_user_name ON categories(user_id, name);
CREATE INDEX idx_categories_user_sort ON categories(user_id, sort_order);

CREATE TABLE tags (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name        TEXT    NOT NULL,
    color       TEXT,
    created_at  TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT    NOT NULL DEFAULT (datetime('now'))
);

-- Tag names are unique per user.
CREATE UNIQUE INDEX uq_tags_user_name ON tags(user_id, name);
CREATE INDEX idx_tags_user ON tags(user_id);

CREATE TABLE bookmarks (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id           INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category_id       INTEGER REFERENCES categories(id) ON DELETE SET NULL,
    url               TEXT    NOT NULL,
    normalized_url    TEXT    NOT NULL,
    title             TEXT    NOT NULL DEFAULT '',
    description       TEXT    NOT NULL DEFAULT '',
    favicon_url       TEXT,
    preview_image_url TEXT,
    domain            TEXT    NOT NULL DEFAULT '',
    pinned            INTEGER NOT NULL DEFAULT 0,     -- 0/1
    favorite          INTEGER NOT NULL DEFAULT 0,     -- 0/1
    sort_order        INTEGER NOT NULL DEFAULT 0,
    click_count       INTEGER NOT NULL DEFAULT 0,
    -- status drives the default list and the soft-delete/restore flow:
    --   active  -> visible by default
    --   archived-> visible only when status=archived is requested
    --   trash   -> soft-deleted; visible only when status=trash is requested
    status            TEXT    NOT NULL DEFAULT 'active',
    -- metadata_status is reserved for a future metadata-fetching task:
    --   pending | success | failed. Default pending so the first fetch runs.
    metadata_status   TEXT    NOT NULL DEFAULT 'pending',
    last_opened_at    TEXT,
    created_at        TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at        TEXT    NOT NULL DEFAULT (datetime('now'))
);

-- A user cannot save the same normalized URL twice (dedup policy: 409).
CREATE UNIQUE INDEX uq_bookmarks_user_normurl
    ON bookmarks(user_id, normalized_url) WHERE status != 'trash';
CREATE INDEX idx_bookmarks_user_status ON bookmarks(user_id, status);
CREATE INDEX idx_bookmarks_user_category ON bookmarks(user_id, category_id);
CREATE INDEX idx_bookmarks_user_domain ON bookmarks(user_id, domain);
CREATE INDEX idx_bookmarks_user_pinned ON bookmarks(user_id, pinned);
CREATE INDEX idx_bookmarks_user_sort ON bookmarks(user_id, status, sort_order);

-- bookmark_tags is the many-to-many join between bookmarks and tags.
CREATE TABLE bookmark_tags (
    bookmark_id INTEGER NOT NULL REFERENCES bookmarks(id) ON DELETE CASCADE,
    tag_id      INTEGER NOT NULL REFERENCES tags(id)     ON DELETE CASCADE,
    user_id     INTEGER NOT NULL REFERENCES users(id)    ON DELETE CASCADE,
    PRIMARY KEY (bookmark_id, tag_id)
);

CREATE INDEX idx_bookmark_tags_tag ON bookmark_tags(tag_id);
CREATE INDEX idx_bookmark_tags_user ON bookmark_tags(user_id);
