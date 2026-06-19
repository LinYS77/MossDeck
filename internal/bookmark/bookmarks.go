package bookmark

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// =====================================================================
// Bookmarks
// =====================================================================

// bookmarkSelectColumns is the canonical column list for bookmark reads, in
// the order scanBookmark expects. Centralizing it keeps list/get/update in
// sync and avoids drift.
const bookmarkSelectColumns = `
b.id, b.user_id, COALESCE(b.category_id,0), b.url, b.normalized_url,
b.title, b.description, COALESCE(b.favicon_url,''), COALESCE(b.preview_image_url,''),
b.domain, b.pinned, b.favorite, b.sort_order, b.click_count,
b.status, b.metadata_status, COALESCE(b.last_opened_at,''),
b.created_at, b.updated_at`

func (s *sqliteStore) CreateBookmark(ctx context.Context, userID int64, b *Bookmark, tagIDs []int64) (*Bookmark, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("bookmark: begin create: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `
INSERT INTO bookmarks
(user_id, category_id, url, normalized_url, title, description, domain,
 pinned, favorite, sort_order, status)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, nullableInt64(b.CategoryID), b.URL, b.NormalizedURL,
		b.Title, b.Description, b.Domain,
		boolToInt(b.Pinned), boolToInt(b.Favorite), b.SortOrder, b.Status)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrBookmarkURLTaken
		}
		return nil, fmt.Errorf("bookmark: insert: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("bookmark: last insert id: %w", err)
	}

	// Attach tags within the same transaction so a partial failure rolls back.
	seen := make(map[int64]struct{}, len(tagIDs))
	for _, tid := range tagIDs {
		if _, dup := seen[tid]; dup {
			continue
		}
		seen[tid] = struct{}{}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO bookmark_tags (bookmark_id, tag_id, user_id) VALUES (?, ?, ?)`,
			id, tid, userID); err != nil {
			return nil, fmt.Errorf("bookmark: insert tag link: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("bookmark: commit create: %w", err)
	}
	return s.GetBookmark(ctx, userID, id)
}

func (s *sqliteStore) GetBookmark(ctx context.Context, userID, id int64) (*Bookmark, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+bookmarkSelectColumns+` FROM bookmarks b WHERE b.id = ? AND b.user_id = ?`,
		id, userID)
	b, err := scanBookmark(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("bookmark: get: %w", err)
	}
	tags, err := s.tagsForBookmark(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	b.Tags = tags
	return b, nil
}

func (s *sqliteStore) GetBookmarkByNormalizedURL(ctx context.Context, userID int64, normalizedURL string) (*Bookmark, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+bookmarkSelectColumns+`
FROM bookmarks b
WHERE b.user_id = ? AND b.normalized_url = ? AND b.status != 'trash'`,
		userID, normalizedURL)
	b, err := scanBookmark(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("bookmark: get by url: %w", err)
	}
	return b, nil
}

func (s *sqliteStore) ListBookmarks(ctx context.Context, q ListQuery) (*ListResult, error) {
	q = normalizeQuery(q)

	var where []string
	var args []any
	add := func(cond string, val any) {
		where = append(where, cond)
		args = append(args, val)
	}
	add("b.user_id = ?", q.UserID)
	if q.Status != "" {
		add("b.status = ?", q.Status)
	} else {
		// Default visible set: active bookmarks.
		add("b.status = ?", StatusActive)
	}
	if q.CategoryID != 0 {
		add("b.category_id = ?", q.CategoryID)
	}
	if q.Domain != "" {
		add("b.domain = ?", q.Domain)
	}
	if q.Favorite != nil {
		add("b.favorite = ?", boolToInt(*q.Favorite))
	}
	if q.Pinned != nil {
		add("b.pinned = ?", boolToInt(*q.Pinned))
	}
	if q.TagID != 0 {
		add("EXISTS (SELECT 1 FROM bookmark_tags bt WHERE bt.bookmark_id = b.id AND bt.tag_id = ? AND bt.user_id = b.user_id)", q.TagID)
	}
	if q.Q != "" {
		like := "%" + q.Q + "%"
		where = append(where, "(b.title LIKE ? OR b.url LIKE ? OR b.description LIKE ? OR b.domain LIKE ?)")
		args = append(args, like, like, like, like)
	}

	orderBy := orderClause(q.Sort)
	whereSQL := strings.Join(where, " AND ")

	// Count first (no tags join, cheap) for pagination metadata.
	var total int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM bookmarks b WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("bookmark: count: %w", err)
	}

	offset := (q.Page - 1) * q.PageSize
	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, q.PageSize, offset)
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+bookmarkSelectColumns+`
FROM bookmarks b
WHERE `+whereSQL+`
ORDER BY `+orderBy+`
LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return nil, fmt.Errorf("bookmark: list: %w", err)
	}

	// PHASE 1: scan all rows first and close the result set BEFORE running any
	// other query. The connection pool is a single connection (SetMaxOpenConns(1)),
	// so a nested query while `rows` is still open would deadlock. Closing rows
	// here returns the connection so phase 2 can reuse it.
	out := make([]Bookmark, 0)
	for rows.Next() {
		b, err := scanBookmark(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, *b)
	}
	if cerr := rows.Close(); cerr != nil {
		return nil, fmt.Errorf("bookmark: close list rows: %w", cerr)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// PHASE 2: populate tags per item now that the connection is free. This is
	// an N+1 (one cheap indexed query per item), acceptable for a single-user app
	// with page sizes <= 100; revisit with a single batched query if profiling
	// ever demands it.
	for i := range out {
		tags, err := s.tagsForBookmark(ctx, q.UserID, out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Tags = tags
	}

	return &ListResult{
		Items:    out,
		Page:     q.Page,
		PageSize: q.PageSize,
		Total:    total,
	}, nil
}

func (s *sqliteStore) UpdateBookmark(ctx context.Context, userID, id int64, p UpdateBookmarkParams) (*Bookmark, error) {
	sets, args := []string{}, []any{}
	add := func(col, expr string, val any) {
		sets = append(sets, col+" = "+expr)
		args = append(args, val)
	}
	if p.URL != nil {
		add("url", "?", *p.URL)
	}
	if p.NormalizedURL != nil {
		add("normalized_url", "?", *p.NormalizedURL)
	}
	if p.Domain != nil {
		add("domain", "?", *p.Domain)
	}
	if p.Title != nil {
		add("title", "?", *p.Title)
	}
	if p.Description != nil {
		add("description", "?", *p.Description)
	}
	if p.CategoryID != nil {
		add("category_id", "?", nullableInt64(*p.CategoryID))
	}
	if p.Pinned != nil {
		add("pinned", "?", boolToInt(*p.Pinned))
	}
	if p.Favorite != nil {
		add("favorite", "?", boolToInt(*p.Favorite))
	}
	if p.SortOrder != nil {
		add("sort_order", "?", *p.SortOrder)
	}
	if p.Status != nil {
		add("status", "?", *p.Status)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("bookmark: begin update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if len(sets) > 0 {
		args = append(args, id, userID)
		q := "UPDATE bookmarks SET " + strings.Join(sets, ", ") +
			", updated_at = datetime('now') WHERE id = ? AND user_id = ?"
		res, err := tx.ExecContext(ctx, q, args...)
		if err != nil {
			if isUniqueViolation(err) {
				return nil, ErrBookmarkURLTaken
			}
			return nil, fmt.Errorf("bookmark: update: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil, ErrNotFound
		}
	}

	if p.TagIDs != nil {
		// Replace tags within the same transaction.
		if err := replaceBookmarkTagsTx(ctx, tx, userID, id, *p.TagIDs); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("bookmark: commit update: %w", err)
	}
	return s.GetBookmark(ctx, userID, id)
}

func (s *sqliteStore) SetBookmarkStatus(ctx context.Context, userID, id int64, status string) (*Bookmark, error) {
	res, err := s.db.ExecContext(ctx, `
UPDATE bookmarks SET status = ?, updated_at = datetime('now')
WHERE id = ? AND user_id = ?`, status, id, userID)
	if err != nil {
		// Restoring (status -> active) can trip the partial unique index
		// uq_bookmarks_user_normurl (WHERE status != 'trash') when another
		// non-trash row already holds the same normalized_url. Surface that as
		// a domain conflict (409) instead of a 500.
		if isUniqueViolation(err) {
			return nil, ErrBookmarkURLTaken
		}
		return nil, fmt.Errorf("bookmark: set status: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return s.GetBookmark(ctx, userID, id)
}

func (s *sqliteStore) IncrementClickCount(ctx context.Context, userID, id int64) error {
	res, err := s.db.ExecContext(ctx, `
UPDATE bookmarks
SET click_count = click_count + 1,
    last_opened_at = datetime('now'),
    updated_at = datetime('now')
WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return fmt.Errorf("bookmark: increment click: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// --- tags-for-bookmark lookup ---

func (s *sqliteStore) tagsForBookmark(ctx context.Context, userID, bookmarkID int64) ([]Tag, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT t.id, t.user_id, t.name, COALESCE(t.color,''), t.created_at, t.updated_at
FROM tags t
JOIN bookmark_tags bt ON bt.tag_id = t.id
WHERE bt.bookmark_id = ? AND bt.user_id = ?
ORDER BY t.name`, bookmarkID, userID)
	if err != nil {
		return nil, fmt.Errorf("bookmark: tags for bookmark: %w", err)
	}
	defer rows.Close()
	out := make([]Tag, 0)
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &t.Color, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// scanBookmark maps a bookmarks row (in bookmarkSelectColumns order) into a
// *Bookmark.
func scanBookmark(s scanner) (*Bookmark, error) {
	var b Bookmark
	var pinned, favorite int
	var catID sql.NullInt64
	err := s.Scan(
		&b.ID, &b.UserID, &catID, &b.URL, &b.NormalizedURL,
		&b.Title, &b.Description, &b.FaviconURL, &b.PreviewImageURL,
		&b.Domain, &pinned, &favorite, &b.SortOrder, &b.ClickCount,
		&b.Status, &b.MetadataStatus, &b.LastOpenedAt,
		&b.CreatedAt, &b.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	b.CategoryID = catID.Int64
	b.Pinned = pinned != 0
	b.Favorite = favorite != 0
	return &b, nil
}

// =====================================================================
// Query/pagination normalization
// =====================================================================

// normalizeQuery applies pagination bounds and resolves a free-text tag name
// to a tag id. It does not run any query for the tag name itself — resolution
// by name is the caller's (Service) responsibility; here it only enforces
// bounds and defaults for page/pageSize/sort.
func normalizeQuery(q ListQuery) ListQuery {
	if q.PageSize <= 0 {
		q.PageSize = defaultPageSize
	}
	if q.PageSize > maxPageSize {
		q.PageSize = maxPageSize
	}
	if q.Page < 1 {
		q.Page = 1
	}
	if q.Status != "" && !isValidStatus(q.Status) {
		q.Status = ""
	}
	return q
}

// orderClause maps a sort token to an ORDER BY clause. Unknown/empty sorts
// default to pinned-first, then created-desc, which is the most useful default
// for a homepage.
func orderClause(sort string) string {
	switch sort {
	case "title":
		return "b.title COLLATE NOCASE ASC"
	case "title_desc":
		return "b.title COLLATE NOCASE DESC"
	case "created":
		return "b.created_at ASC, b.id ASC"
	case "created_desc":
		return "b.created_at DESC, b.id DESC"
	case "clicks":
		return "b.click_count DESC, b.id DESC"
	case "opened":
		return "b.last_opened_at DESC, b.id DESC"
	case "title_asc":
		return "b.title COLLATE NOCASE ASC"
	default:
		// homepage default: pinned first, newest first.
		return "b.pinned DESC, b.created_at DESC, b.id DESC"
	}
}

// replaceBookmarkTagsTx is the transaction-scoped variant used by UpdateBookmark.
func replaceBookmarkTagsTx(ctx context.Context, tx *sql.Tx, userID, bookmarkID int64, tagIDs []int64) error {
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM bookmark_tags WHERE bookmark_id = ? AND user_id = ?`, bookmarkID, userID); err != nil {
		return fmt.Errorf("bookmark: clear bookmark_tags: %w", err)
	}
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
	return nil
}
