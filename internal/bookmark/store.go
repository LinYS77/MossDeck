package bookmark

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// sqliteStore is the production Store backed by *sql.DB. It is concurrency-safe
// via the DB pool. Every method scopes queries by user_id.
type sqliteStore struct {
	db *sql.DB
}

// NewStore returns a Store backed by db. The bookmark tables must exist
// (see migrations/0001_init.sql and 0002_bookmarks.sql).
func NewStore(db *sql.DB) Store {
	return &sqliteStore{db: db}
}

// =====================================================================
// Categories
// =====================================================================

func (s *sqliteStore) CreateCategory(ctx context.Context, userID int64, p CreateCategoryParams) (*Category, error) {
	res, err := s.db.ExecContext(ctx, `
INSERT INTO categories (user_id, parent_id, name, slug, type, icon, color, sort_order)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, nullableInt64(p.ParentID), p.Name, nullableStr(p.Slug),
		defaultStr(p.Type, "bookmark"), nullableStr(p.Icon), nullableStr(p.Color), p.SortOrder,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrCategoryNameTaken
		}
		return nil, fmt.Errorf("bookmark: create category: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.GetCategory(ctx, userID, id)
}

func (s *sqliteStore) GetCategory(ctx context.Context, userID, id int64) (*Category, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT c.id, c.user_id, COALESCE(c.parent_id,0), c.name, c.slug, c.type,
       c.icon, c.color, c.sort_order, c.archived, c.show_on_home,
       c.created_at, c.updated_at,
       (SELECT COUNT(*) FROM bookmarks b WHERE b.user_id = c.user_id AND b.category_id = c.id AND b.status != 'trash')
FROM categories c WHERE c.id = ? AND c.user_id = ?`, id, userID)
	c, err := scanCategory(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("bookmark: get category: %w", err)
	}
	return c, nil
}

func (s *sqliteStore) ListCategories(ctx context.Context, userID int64) ([]*Category, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT c.id, c.user_id, COALESCE(c.parent_id,0), c.name, c.slug, c.type,
       c.icon, c.color, c.sort_order, c.archived, c.show_on_home,
       c.created_at, c.updated_at,
       (SELECT COUNT(*) FROM bookmarks b WHERE b.user_id = c.user_id AND b.category_id = c.id AND b.status != 'trash')
FROM categories c WHERE c.user_id = ?
ORDER BY c.sort_order, c.name`, userID)
	if err != nil {
		return nil, fmt.Errorf("bookmark: list categories: %w", err)
	}
	defer rows.Close()
	var out []*Category
	for rows.Next() {
		c, err := scanCategory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *sqliteStore) UpdateCategory(ctx context.Context, userID, id int64, p UpdateCategoryParams) (*Category, error) {
	sets, args := []string{}, []any{}
	add := func(col, expr string, val any) {
		sets = append(sets, col+" = "+expr)
		args = append(args, val)
	}
	if p.Name != nil {
		add("name", "?", *p.Name)
	}
	if p.Slug != nil {
		add("slug", "?", nullableStr(*p.Slug))
	}
	if p.Type != nil {
		add("type", "?", *p.Type)
	}
	if p.Icon != nil {
		add("icon", "?", nullableStr(*p.Icon))
	}
	if p.Color != nil {
		add("color", "?", nullableStr(*p.Color))
	}
	if p.ParentID != nil {
		add("parent_id", "?", nullableInt64(*p.ParentID))
	}
	if p.SortOrder != nil {
		add("sort_order", "?", *p.SortOrder)
	}
	if p.Archived != nil {
		add("archived", "?", boolToInt(*p.Archived))
	}
	if p.ShowOnHome != nil {
		add("show_on_home", "?", boolToInt(*p.ShowOnHome))
	}
	if len(sets) == 0 {
		// Nothing to change; return current row.
		return s.GetCategory(ctx, userID, id)
	}
	args = append(args, id, userID)
	q := "UPDATE categories SET " + strings.Join(sets, ", ") +
		", updated_at = datetime('now') WHERE id = ? AND user_id = ?"
	res, err := s.db.ExecContext(ctx, q, args...)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrCategoryNameTaken
		}
		return nil, fmt.Errorf("bookmark: update category: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return s.GetCategory(ctx, userID, id)
}

func (s *sqliteStore) DeleteCategory(ctx context.Context, userID, id int64) error {
	res, err := s.db.ExecContext(ctx, `
UPDATE categories SET archived = 1, updated_at = datetime('now')
WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return fmt.Errorf("bookmark: delete category: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// =====================================================================
// Tags
// =====================================================================

func (s *sqliteStore) CreateTag(ctx context.Context, userID int64, p CreateTagParams) (*Tag, error) {
	res, err := s.db.ExecContext(ctx, `
INSERT INTO tags (user_id, name, color) VALUES (?, ?, ?)`,
		userID, p.Name, nullableStr(p.Color))
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrTagNameTaken
		}
		return nil, fmt.Errorf("bookmark: create tag: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.GetTag(ctx, userID, id)
}

func (s *sqliteStore) GetTag(ctx context.Context, userID, id int64) (*Tag, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT t.id, t.user_id, t.name, t.color, t.created_at, t.updated_at,
       (SELECT COUNT(*) FROM bookmark_tags bt WHERE bt.tag_id = t.id AND bt.user_id = t.user_id)
FROM tags t WHERE t.id = ? AND t.user_id = ?`, id, userID)
	tg, err := scanTag(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("bookmark: get tag: %w", err)
	}
	return tg, nil
}

func (s *sqliteStore) GetTagByName(ctx context.Context, userID int64, name string) (*Tag, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT t.id, t.user_id, t.name, t.color, t.created_at, t.updated_at,
       (SELECT COUNT(*) FROM bookmark_tags bt WHERE bt.tag_id = t.id AND bt.user_id = t.user_id)
FROM tags t WHERE t.name = ? AND t.user_id = ?`, name, userID)
	tg, err := scanTag(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("bookmark: get tag by name: %w", err)
	}
	return tg, nil
}

func (s *sqliteStore) ListTags(ctx context.Context, userID int64) ([]*Tag, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT t.id, t.user_id, t.name, t.color, t.created_at, t.updated_at,
       (SELECT COUNT(*) FROM bookmark_tags bt WHERE bt.tag_id = t.id AND bt.user_id = t.user_id)
FROM tags t WHERE t.user_id = ?
ORDER BY t.name`, userID)
	if err != nil {
		return nil, fmt.Errorf("bookmark: list tags: %w", err)
	}
	defer rows.Close()
	var out []*Tag
	for rows.Next() {
		tg, err := scanTag(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, tg)
	}
	return out, rows.Err()
}

func (s *sqliteStore) UpdateTag(ctx context.Context, userID, id int64, p UpdateTagParams) (*Tag, error) {
	sets, args := []string{}, []any{}
	if p.Name != nil {
		sets = append(sets, "name = ?")
		args = append(args, *p.Name)
	}
	if p.Color != nil {
		sets = append(sets, "color = ?")
		args = append(args, nullableStr(*p.Color))
	}
	if len(sets) == 0 {
		return s.GetTag(ctx, userID, id)
	}
	args = append(args, id, userID)
	q := "UPDATE tags SET " + strings.Join(sets, ", ") +
		", updated_at = datetime('now') WHERE id = ? AND user_id = ?"
	res, err := s.db.ExecContext(ctx, q, args...)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrTagNameTaken
		}
		return nil, fmt.Errorf("bookmark: update tag: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return s.GetTag(ctx, userID, id)
}

func (s *sqliteStore) DeleteTag(ctx context.Context, userID, id int64) error {
	// bookmark_tags FK is ON DELETE CASCADE, so join rows are removed first.
	res, err := s.db.ExecContext(ctx, `DELETE FROM tags WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return fmt.Errorf("bookmark: delete tag: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// --- bookmark_tags link management ---

// replaceBookmarkTags atomically sets the full tag set for a bookmark.
func (s *sqliteStore) replaceBookmarkTags(ctx context.Context, userID, bookmarkID int64, tagIDs []int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("bookmark: begin replace tags: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM bookmark_tags WHERE bookmark_id = ? AND user_id = ?`, bookmarkID, userID); err != nil {
		return fmt.Errorf("bookmark: clear bookmark_tags: %w", err)
	}
	// De-duplicate tagIDs before inserting to avoid a partial-apply surprise.
	seen := make(map[int64]struct{}, len(tagIDs))
	for _, tid := range tagIDs {
		if _, dup := seen[tid]; dup {
			continue
		}
		seen[tid] = struct{}{}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO bookmark_tags (bookmark_id, tag_id, user_id) VALUES (?, ?, ?)`,
			bookmarkID, tid, userID); err != nil {
			return fmt.Errorf("bookmark: insert bookmark_tags: %w", err)
		}
	}
	return tx.Commit()
}

func (s *sqliteStore) TagIDsForBookmark(ctx context.Context, userID, bookmarkID int64) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT tag_id FROM bookmark_tags WHERE bookmark_id = ? AND user_id = ?`, bookmarkID, userID)
	if err != nil {
		return nil, fmt.Errorf("bookmark: list bookmark_tags: %w", err)
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// =====================================================================
// Shared scan / sql helpers
// =====================================================================

// scanner is satisfied by *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanCategory(s scanner) (*Category, error) {
	var c Category
	var parentID sql.NullInt64
	var slug, icon, color sql.NullString
	var archived int
	var showOnHome int
	err := s.Scan(&c.ID, &c.UserID, &parentID, &c.Name, &slug, &c.Type,
		&icon, &color, &c.SortOrder, &archived, &showOnHome,
		&c.CreatedAt, &c.UpdatedAt,
		&c.BookmarkCount)
	if err != nil {
		return nil, err
	}
	c.ParentID = parentID.Int64
	c.Slug = slug.String
	c.Icon = icon.String
	c.Color = color.String
	c.Archived = archived != 0
	c.ShowOnHome = showOnHome != 0
	return &c, nil
}

func scanTag(s scanner) (*Tag, error) {
	var t Tag
	var color sql.NullString
	err := s.Scan(&t.ID, &t.UserID, &t.Name, &color, &t.CreatedAt, &t.UpdatedAt,
		&t.BookmarkCount)
	if err != nil {
		return nil, err
	}
	t.Color = color.String
	return &t, nil
}

func nullableStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullableInt64(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

func defaultStr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// isUniqueViolation reports whether err is a SQLite UNIQUE constraint failure.
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
