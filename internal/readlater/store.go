package readlater

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type sqliteStore struct {
	db *sql.DB
}

func NewStore(db *sql.DB) Store {
	return &sqliteStore{db: db}
}

func (s *sqliteStore) CreateItem(ctx context.Context, userID int64, item *Item, tagIDs []int64) (*Item, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("readlater: begin create: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `
INSERT INTO read_later_items (
    user_id, url, normalized_url, title, excerpt, author, site_name,
    favicon_url, cover_image_url, domain, reading_time_minutes, state,
    priority, favorite, source, metadata_status
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending')`,
		userID, item.URL, item.NormalizedURL, item.Title, item.Excerpt,
		item.Author, item.SiteName, nullableStr(item.FaviconURL), nullableStr(item.CoverImageURL),
		item.Domain, item.ReadingTimeMinutes, item.State, item.Priority, boolToInt(item.Favorite), item.Source)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrURLTaken
		}
		return nil, fmt.Errorf("readlater: create item: %w", err)
	}
	id, _ := res.LastInsertId()
	if err := replaceReadLaterTagsTx(ctx, tx, userID, id, tagIDs); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("readlater: commit create: %w", err)
	}
	return s.GetItem(ctx, userID, id)
}

func (s *sqliteStore) GetItem(ctx context.Context, userID, id int64) (*Item, error) {
	row := s.db.QueryRowContext(ctx, baseItemSelect()+` WHERE ri.id = ? AND ri.user_id = ?`, id, userID)
	item, err := scanItem(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("readlater: get item: %w", err)
	}
	tags, err := s.tagsForItem(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	item.Tags = tags
	return item, nil
}

func (s *sqliteStore) GetItemByNormalizedURL(ctx context.Context, userID int64, normalizedURL string) (*Item, error) {
	row := s.db.QueryRowContext(ctx, baseItemSelect()+`
WHERE ri.user_id = ? AND ri.normalized_url = ? AND ri.state != 'trash'`, userID, normalizedURL)
	item, err := scanItem(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("readlater: get item by normalized url: %w", err)
	}
	tags, err := s.tagsForItem(ctx, userID, item.ID)
	if err != nil {
		return nil, err
	}
	item.Tags = tags
	return item, nil
}

func (s *sqliteStore) UpdateItem(ctx context.Context, userID, id int64, p UpdateParams) (*Item, error) {
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
	if p.Excerpt != nil {
		add("excerpt", "?", *p.Excerpt)
	}
	if p.Author != nil {
		add("author", "?", *p.Author)
	}
	if p.SiteName != nil {
		add("site_name", "?", *p.SiteName)
	}
	if p.FaviconURL != nil {
		add("favicon_url", "?", nullableStr(*p.FaviconURL))
	}
	if p.CoverImageURL != nil {
		add("cover_image_url", "?", nullableStr(*p.CoverImageURL))
	}
	if p.ReadingTimeMinutes != nil {
		add("reading_time_minutes", "?", *p.ReadingTimeMinutes)
	}
	if p.Priority != nil {
		add("priority", "?", *p.Priority)
	}
	if p.Favorite != nil {
		add("favorite", "?", boolToInt(*p.Favorite))
	}
	if p.Source != nil {
		add("source", "?", *p.Source)
	}
	if p.State != nil {
		add("state", "?", *p.State)
		switch *p.State {
		case StateTrash:
			sets = append(sets, "deleted_at = datetime('now')")
		case StateArchived:
			sets = append(sets, "archived_at = datetime('now')", "deleted_at = NULL")
		default:
			sets = append(sets, "archived_at = NULL", "deleted_at = NULL")
		}
	}

	if len(sets) == 0 && p.TagIDs == nil {
		return s.GetItem(ctx, userID, id)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("readlater: begin update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if len(sets) > 0 {
		args = append(args, id, userID)
		q := "UPDATE read_later_items SET " + strings.Join(sets, ", ") +
			", updated_at = datetime('now') WHERE id = ? AND user_id = ?"
		res, err := tx.ExecContext(ctx, q, args...)
		if err != nil {
			if isUniqueViolation(err) {
				return nil, ErrURLTaken
			}
			return nil, fmt.Errorf("readlater: update item: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil, ErrNotFound
		}
	}
	if p.TagIDs != nil {
		// When only tags are being changed there is no UPDATE rows-affected check,
		// so explicitly verify item ownership before touching the join table. This
		// keeps missing/foreign ids as ErrNotFound instead of surfacing FK errors.
		if len(sets) == 0 {
			var itemID int64
			if err := tx.QueryRowContext(ctx,
				`SELECT id FROM read_later_items WHERE id = ? AND user_id = ?`, id, userID).Scan(&itemID); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return nil, ErrNotFound
				}
				return nil, fmt.Errorf("readlater: check item before tag update: %w", err)
			}
		}
		if err := replaceReadLaterTagsTx(ctx, tx, userID, id, *p.TagIDs); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("readlater: commit update: %w", err)
	}
	return s.GetItem(ctx, userID, id)
}

func (s *sqliteStore) SetItemState(ctx context.Context, userID, id int64, state string) (*Item, error) {
	sets := []string{"state = ?", "updated_at = datetime('now')"}
	args := []any{state}
	switch state {
	case StateTrash:
		sets = append(sets, "deleted_at = datetime('now')")
	case StateArchived:
		sets = append(sets, "archived_at = datetime('now')", "deleted_at = NULL")
	default:
		sets = append(sets, "archived_at = NULL", "deleted_at = NULL")
	}
	args = append(args, id, userID)
	q := "UPDATE read_later_items SET " + strings.Join(sets, ", ") + " WHERE id = ? AND user_id = ?"
	res, err := s.db.ExecContext(ctx, q, args...)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrURLTaken
		}
		return nil, fmt.Errorf("readlater: set state: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return s.GetItem(ctx, userID, id)
}

func (s *sqliteStore) OpenItem(ctx context.Context, userID, id int64) (*Item, error) {
	res, err := s.db.ExecContext(ctx, `
UPDATE read_later_items
SET last_opened_at = datetime('now'),
    state = CASE WHEN state = 'unread' THEN 'reading' ELSE state END,
    updated_at = datetime('now')
WHERE id = ? AND user_id = ? AND state != 'trash'`, id, userID)
	if err != nil {
		return nil, fmt.Errorf("readlater: open item: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return s.GetItem(ctx, userID, id)
}

// DeletePermanently hard-deletes an item. It only succeeds when the item is in
// trash state; otherwise it returns ErrInvalidState (non-trash) or ErrNotFound
// (missing). ON DELETE CASCADE cleans up tag links automatically.
func (s *sqliteStore) DeletePermanently(ctx context.Context, userID, id int64) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM read_later_items WHERE id = ? AND user_id = ? AND state = 'trash'`, id, userID)
	if err != nil {
		return fmt.Errorf("readlater: purge: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		var exists int
		if err := s.db.QueryRowContext(ctx,
			`SELECT 1 FROM read_later_items WHERE id = ? AND user_id = ?`, id, userID).Scan(&exists); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return fmt.Errorf("readlater: purge check: %w", err)
		}
		return ErrInvalidState
	}
	return nil
}

func (s *sqliteStore) ListItems(ctx context.Context, q ListQuery) (*ListResult, error) {
	page := max1(q.Page)
	pageSize := pageSizeOrDefault(q.PageSize)
	where, args := buildListWhere(q)

	var total int
	countQ := "SELECT COUNT(*) FROM read_later_items ri " + where
	if err := s.db.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("readlater: count items: %w", err)
	}

	order := orderBy(q.Sort)
	limitArgs := append(append([]any{}, args...), pageSize, (page-1)*pageSize)
	rows, err := s.db.QueryContext(ctx, baseItemSelect()+" "+where+" "+order+" LIMIT ? OFFSET ?", limitArgs...)
	if err != nil {
		return nil, fmt.Errorf("readlater: list items: %w", err)
	}
	defer rows.Close()

	items := []Item{}
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.fillTags(ctx, q.UserID, items); err != nil {
		return nil, err
	}
	return &ListResult{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

func (s *sqliteStore) ListTags(ctx context.Context, userID int64) ([]*Tag, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, user_id, name, color, created_at, updated_at
FROM tags WHERE user_id = ? ORDER BY name`, userID)
	if err != nil {
		return nil, fmt.Errorf("readlater: list tags: %w", err)
	}
	defer rows.Close()
	out := []*Tag{}
	for rows.Next() {
		tag, err := scanTag(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, tag)
	}
	return out, rows.Err()
}

func (s *sqliteStore) GetTag(ctx context.Context, userID, id int64) (*Tag, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, user_id, name, color, created_at, updated_at
FROM tags WHERE id = ? AND user_id = ?`, id, userID)
	tag, err := scanTag(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("readlater: get tag: %w", err)
	}
	return tag, nil
}

func (s *sqliteStore) GetTagByName(ctx context.Context, userID int64, name string) (*Tag, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, user_id, name, color, created_at, updated_at
FROM tags WHERE name = ? AND user_id = ?`, name, userID)
	tag, err := scanTag(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("readlater: get tag by name: %w", err)
	}
	return tag, nil
}

func replaceReadLaterTagsTx(ctx context.Context, tx *sql.Tx, userID, itemID int64, tagIDs []int64) error {
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM read_later_tags WHERE read_later_id = ? AND user_id = ?`, itemID, userID); err != nil {
		return fmt.Errorf("readlater: clear tags: %w", err)
	}
	seen := make(map[int64]struct{}, len(tagIDs))
	for _, tagID := range tagIDs {
		if _, dup := seen[tagID]; dup {
			continue
		}
		seen[tagID] = struct{}{}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO read_later_tags (read_later_id, tag_id, user_id) VALUES (?, ?, ?)`,
			itemID, tagID, userID); err != nil {
			return fmt.Errorf("readlater: insert tag link: %w", err)
		}
	}
	return nil
}

func buildListWhere(q ListQuery) (string, []any) {
	where := []string{"ri.user_id = ?"}
	args := []any{q.UserID}
	if q.State != "" {
		where = append(where, "ri.state = ?")
		args = append(args, q.State)
	} else {
		// Default visible set: active reading queue only. Archived/trash are
		// opt-in via state=archived or state=trash.
		where = append(where, "ri.state IN ('unread', 'reading')")
	}
	if q.Q != "" {
		like := "%" + q.Q + "%"
		where = append(where, `(ri.title LIKE ? OR ri.url LIKE ? OR ri.excerpt LIKE ? OR ri.domain LIKE ? OR ri.site_name LIKE ?)`)
		args = append(args, like, like, like, like, like)
	}
	if q.TagID > 0 {
		where = append(where, `EXISTS (SELECT 1 FROM read_later_tags rlt WHERE rlt.read_later_id = ri.id AND rlt.user_id = ri.user_id AND rlt.tag_id = ?)`)
		args = append(args, q.TagID)
	}
	if q.Domain != "" {
		where = append(where, "ri.domain = ?")
		args = append(args, q.Domain)
	}
	if q.Favorite != nil {
		where = append(where, "ri.favorite = ?")
		args = append(args, boolToInt(*q.Favorite))
	}
	if q.Priority != nil {
		where = append(where, "ri.priority = ?")
		args = append(args, *q.Priority)
	}
	return "WHERE " + strings.Join(where, " AND "), args
}

func orderBy(sort string) string {
	switch sort {
	case "createdAtAsc", "created_at_asc":
		return "ORDER BY ri.created_at ASC, ri.id ASC"
	case "updatedAtAsc", "updated_at_asc":
		return "ORDER BY ri.updated_at ASC, ri.id ASC"
	case "updatedAtDesc", "updated_at_desc":
		return "ORDER BY ri.updated_at DESC, ri.id DESC"
	case "priorityAsc", "priority_asc":
		return "ORDER BY ri.priority ASC, ri.created_at DESC, ri.id DESC"
	case "priorityDesc", "priority_desc":
		return "ORDER BY ri.priority DESC, ri.created_at DESC, ri.id DESC"
	case "titleAsc", "title_asc":
		return "ORDER BY ri.title ASC, ri.id ASC"
	default:
		return "ORDER BY ri.created_at DESC, ri.id DESC"
	}
}

func baseItemSelect() string {
	return `
SELECT ri.id, ri.user_id, ri.url, ri.normalized_url, ri.title, ri.excerpt,
       ri.author, ri.site_name, ri.favicon_url, ri.cover_image_url, ri.domain,
       ri.reading_time_minutes, ri.state, ri.priority, ri.favorite, ri.source,
       ri.last_opened_at, ri.archived_at, ri.metadata_status, ri.created_at,
       ri.updated_at, ri.deleted_at
FROM read_later_items ri`
}

type scanner interface {
	Scan(dest ...any) error
}

func scanItem(s scanner) (*Item, error) {
	var item Item
	var faviconURL, coverImageURL, lastOpenedAt, archivedAt, deletedAt sql.NullString
	var favorite int
	err := s.Scan(
		&item.ID, &item.UserID, &item.URL, &item.NormalizedURL, &item.Title, &item.Excerpt,
		&item.Author, &item.SiteName, &faviconURL, &coverImageURL, &item.Domain,
		&item.ReadingTimeMinutes, &item.State, &item.Priority, &favorite, &item.Source,
		&lastOpenedAt, &archivedAt, &item.MetadataStatus, &item.CreatedAt,
		&item.UpdatedAt, &deletedAt,
	)
	if err != nil {
		return nil, err
	}
	item.FaviconURL = faviconURL.String
	item.CoverImageURL = coverImageURL.String
	item.Favorite = favorite != 0
	item.LastOpenedAt = lastOpenedAt.String
	item.ArchivedAt = archivedAt.String
	item.DeletedAt = deletedAt.String
	return &item, nil
}

func scanTag(s scanner) (*Tag, error) {
	var tag Tag
	var color sql.NullString
	if err := s.Scan(&tag.ID, &tag.UserID, &tag.Name, &color, &tag.CreatedAt, &tag.UpdatedAt); err != nil {
		return nil, err
	}
	tag.Color = color.String
	return &tag, nil
}

func (s *sqliteStore) tagsForItem(ctx context.Context, userID, itemID int64) ([]Tag, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT t.id, t.user_id, t.name, t.color, t.created_at, t.updated_at
FROM tags t
JOIN read_later_tags rlt ON rlt.tag_id = t.id AND rlt.user_id = t.user_id
WHERE rlt.read_later_id = ? AND rlt.user_id = ?
ORDER BY t.name`, itemID, userID)
	if err != nil {
		return nil, fmt.Errorf("readlater: item tags: %w", err)
	}
	defer rows.Close()
	out := []Tag{}
	for rows.Next() {
		tag, err := scanTag(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *tag)
	}
	return out, rows.Err()
}

func (s *sqliteStore) fillTags(ctx context.Context, userID int64, items []Item) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]string, 0, len(items))
	args := make([]any, 0, len(items)+1)
	args = append(args, userID)
	index := make(map[int64]int, len(items))
	for i := range items {
		ids = append(ids, "?")
		args = append(args, items[i].ID)
		index[items[i].ID] = i
	}
	q := `
SELECT rlt.read_later_id, t.id, t.user_id, t.name, t.color, t.created_at, t.updated_at
FROM read_later_tags rlt
JOIN tags t ON t.id = rlt.tag_id AND t.user_id = rlt.user_id
WHERE rlt.user_id = ? AND rlt.read_later_id IN (` + strings.Join(ids, ",") + `)
ORDER BY t.name`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("readlater: fill tags: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var itemID int64
		var tag Tag
		var color sql.NullString
		if err := rows.Scan(&itemID, &tag.ID, &tag.UserID, &tag.Name, &color, &tag.CreatedAt, &tag.UpdatedAt); err != nil {
			return err
		}
		tag.Color = color.String
		if i, ok := index[itemID]; ok {
			items[i].Tags = append(items[i].Tags, tag)
		}
	}
	return rows.Err()
}

func nullableStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
