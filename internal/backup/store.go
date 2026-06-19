package backup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/linyusheng/homepage/internal/bookmark"
)

// sqliteStore implements Store backed by *sql.DB.
type sqliteStore struct {
	db *sql.DB
}

// NewStore returns a Store for backup/restore operations.
func NewStore(db *sql.DB) Store {
	return &sqliteStore{db: db}
}

// =====================================================================
// Export
// =====================================================================

func (s *sqliteStore) ExportAll(ctx context.Context, userID int64) (*ExportData, error) {
	cats, err := s.exportCategories(ctx, userID)
	if err != nil {
		return nil, err
	}
	tags, err := s.exportTags(ctx, userID)
	if err != nil {
		return nil, err
	}
	bms, err := s.exportBookmarks(ctx, userID)
	if err != nil {
		return nil, err
	}
	rl, err := s.exportReadLater(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &ExportData{
		Version:    Version,
		App:        App,
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Data: ExportPayload{
			Categories:     cats,
			Tags:           tags,
			Bookmarks:      bms,
			ReadLaterItems: rl,
		},
	}, nil
}

func (s *sqliteStore) exportCategories(ctx context.Context, userID int64) ([]CategoryExport, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT name, slug, type, icon, color, sort_order, archived, show_on_home, created_at, updated_at
		 FROM categories WHERE user_id = ? ORDER BY sort_order, name`, userID)
	if err != nil {
		return nil, fmt.Errorf("backup: export categories: %w", err)
	}
	defer rows.Close()
	var out []CategoryExport
	for rows.Next() {
		var e CategoryExport
		var slug, icon, color sql.NullString
		var archived, showOnHome int
		if err := rows.Scan(&e.Name, &slug, &e.Type, &icon, &color, &e.SortOrder, &archived, &showOnHome, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		e.Slug = slug.String
		e.Icon = icon.String
		e.Color = color.String
		e.Archived = archived != 0
		b := showOnHome != 0
		e.ShowOnHome = &b // always materialised so exports stay self-describing
		e.CreatedAt = toRFC3339(e.CreatedAt)
		e.UpdatedAt = toRFC3339(e.UpdatedAt)
		out = append(out, e)
	}
	if out == nil {
		out = []CategoryExport{}
	}
	return out, rows.Err()
}

func (s *sqliteStore) exportTags(ctx context.Context, userID int64) ([]TagExport, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT name, color FROM tags WHERE user_id = ? ORDER BY name`, userID)
	if err != nil {
		return nil, fmt.Errorf("backup: export tags: %w", err)
	}
	defer rows.Close()
	var out []TagExport
	for rows.Next() {
		var e TagExport
		var color sql.NullString
		if err := rows.Scan(&e.Name, &color); err != nil {
			return nil, err
		}
		e.Color = color.String
		out = append(out, e)
	}
	if out == nil {
		out = []TagExport{}
	}
	return out, rows.Err()
}

func (s *sqliteStore) exportBookmarks(ctx context.Context, userID int64) ([]BookmarkExport, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT b.url, b.title, b.description, COALESCE(c.name,''),
		       b.pinned, b.favorite, b.sort_order, b.click_count, b.status,
		       b.created_at, b.updated_at, b.last_opened_at, b.id
		 FROM bookmarks b
		 LEFT JOIN categories c ON c.id = b.category_id AND c.user_id = b.user_id
		 WHERE b.user_id = ?
		 ORDER BY b.sort_order, b.created_at`, userID)
	if err != nil {
		return nil, fmt.Errorf("backup: export bookmarks: %w", err)
	}
	defer rows.Close()

	// Collect ids for batch tag lookups.
	var ids []int64
	var items []BookmarkExport
	for rows.Next() {
		var e BookmarkExport
		var desc, catName, lastOpenedAt sql.NullString
		var pinned, favorite int
		var id int64
		if err := rows.Scan(&e.URL, &e.Title, &desc, &catName,
			&pinned, &favorite, &e.SortOrder, &e.ClickCount, &e.Status,
			&e.CreatedAt, &e.UpdatedAt, &lastOpenedAt, &id); err != nil {
			return nil, err
		}
		e.Description = desc.String
		e.Category = catName.String
		e.Pinned = pinned != 0
		e.Favorite = favorite != 0
		e.LastOpenedAt = toRFC3339(lastOpenedAt.String)
		e.CreatedAt = toRFC3339(e.CreatedAt)
		e.UpdatedAt = toRFC3339(e.UpdatedAt)
		ids = append(ids, id)
		items = append(items, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Batch-fetch tags for all bookmarks.
	tagMap, err := s.batchBookmarkTags(ctx, userID, ids)
	if err != nil {
		return nil, err
	}

	out := make([]BookmarkExport, 0, len(items))
	for i, e := range items {
		if i < len(ids) {
			e.Tags = tagMap[ids[i]]
		}
		out = append(out, e)
	}
	if out == nil {
		out = []BookmarkExport{}
	}
	return out, nil
}

func (s *sqliteStore) batchBookmarkTags(ctx context.Context, userID int64, bmIDs []int64) (map[int64][]string, error) {
	if len(bmIDs) == 0 {
		return nil, nil
	}
	ids := make([]string, len(bmIDs))
	args := make([]any, 0, len(bmIDs)+1)
	args = append(args, userID)
	for i, id := range bmIDs {
		ids[i] = "?"
		args = append(args, id)
	}
	q := fmt.Sprintf(
		`SELECT bt.bookmark_id, t.name FROM bookmark_tags bt
		 JOIN tags t ON t.id = bt.tag_id AND t.user_id = bt.user_id
		 WHERE bt.user_id = ? AND bt.bookmark_id IN (%s)
		 ORDER BY t.name`, strings.Join(ids, ","))
	tagRows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("backup: batch bookmark tags: %w", err)
	}
	defer tagRows.Close()

	out := make(map[int64][]string)
	for tagRows.Next() {
		var bmID int64
		var name string
		if err := tagRows.Scan(&bmID, &name); err != nil {
			return nil, err
		}
		out[bmID] = append(out[bmID], name)
	}
	return out, tagRows.Err()
}

func (s *sqliteStore) exportReadLater(ctx context.Context, userID int64) ([]ReadLaterItemExport, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT url, title, excerpt, author, site_name, state, priority, favorite,
		        reading_time_minutes, source, created_at, updated_at,
		        last_opened_at, archived_at, id
		 FROM read_later_items WHERE user_id = ?
		 ORDER BY created_at`, userID)
	if err != nil {
		return nil, fmt.Errorf("backup: export read-later: %w", err)
	}
	defer rows.Close()

	var rlIDs []int64
	var items []ReadLaterItemExport
	for rows.Next() {
		var e ReadLaterItemExport
		var excerpt, author, siteName, source sql.NullString
		var favorite int
		var lastOpenedAt, archivedAt sql.NullString
		var id int64
		if err := rows.Scan(&e.URL, &e.Title, &excerpt, &author, &siteName,
			&e.State, &e.Priority, &favorite, &e.ReadingTimeMinutes, &source,
			&e.CreatedAt, &e.UpdatedAt, &lastOpenedAt, &archivedAt, &id); err != nil {
			return nil, err
		}
		e.Excerpt = excerpt.String
		e.Author = author.String
		e.SiteName = siteName.String
		e.Source = source.String
		e.Favorite = favorite != 0
		e.LastOpenedAt = toRFC3339(lastOpenedAt.String)
		e.ArchivedAt = toRFC3339(archivedAt.String)
		e.CreatedAt = toRFC3339(e.CreatedAt)
		e.UpdatedAt = toRFC3339(e.UpdatedAt)
		rlIDs = append(rlIDs, id)
		items = append(items, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	tagMap, err := s.batchReadLaterTags(ctx, userID, rlIDs)
	if err != nil {
		return nil, err
	}
	out := make([]ReadLaterItemExport, 0, len(items))
	for i, e := range items {
		if i < len(rlIDs) {
			e.Tags = tagMap[rlIDs[i]]
		}
		out = append(out, e)
	}
	if out == nil {
		out = []ReadLaterItemExport{}
	}
	return out, nil
}

func (s *sqliteStore) batchReadLaterTags(ctx context.Context, userID int64, rlIDs []int64) (map[int64][]string, error) {
	if len(rlIDs) == 0 {
		return nil, nil
	}
	ids := make([]string, len(rlIDs))
	args := make([]any, 0, len(rlIDs)+1)
	args = append(args, userID)
	for i, id := range rlIDs {
		ids[i] = "?"
		args = append(args, id)
	}
	q := fmt.Sprintf(
		`SELECT rlt.read_later_id, t.name FROM read_later_tags rlt
		 JOIN tags t ON t.id = rlt.tag_id AND t.user_id = rlt.user_id
		 WHERE rlt.user_id = ? AND rlt.read_later_id IN (%s)
		 ORDER BY t.name`, strings.Join(ids, ","))
	tagRows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("backup: batch readlater tags: %w", err)
	}
	defer tagRows.Close()

	out := make(map[int64][]string)
	for tagRows.Next() {
		var rlID int64
		var name string
		if err := tagRows.Scan(&rlID, &name); err != nil {
			return nil, err
		}
		out[rlID] = append(out[rlID], name)
	}
	return out, tagRows.Err()
}

// =====================================================================
// Clear (replace mode)
// =====================================================================

func (s *sqliteStore) ClearUserData(ctx context.Context, tx DB, userID int64) error {
	// Order respects FK constraints (children first).
	queries := []string{
		`DELETE FROM bookmark_tags WHERE user_id = ?`,
		`DELETE FROM read_later_tags WHERE user_id = ?`,
		`DELETE FROM bookmarks WHERE user_id = ?`,
		`DELETE FROM read_later_items WHERE user_id = ?`,
		`DELETE FROM categories WHERE user_id = ?`,
		`DELETE FROM tags WHERE user_id = ?`,
	}
	for _, q := range queries {
		if _, err := tx.ExecContext(ctx, q, userID); err != nil {
			return fmt.Errorf("backup: clear %q: %w", q, err)
		}
	}
	return nil
}

// =====================================================================
// Import helpers
// =====================================================================

func (s *sqliteStore) UpsertCategory(ctx context.Context, tx DB, userID int64, e CategoryExport) (int64, bool, error) {
	// Resolve the home flag, defaulting to "show" when an older backup omits it.
	showOnHome := true
	if e.ShowOnHome != nil {
		showOnHome = *e.ShowOnHome
	}
	// Try to find existing by name.
	var id int64
	err := tx.QueryRowContext(ctx,
		`SELECT id FROM categories WHERE user_id = ? AND name = ?`, userID, e.Name).Scan(&id)
	if err == nil {
		// Update existing.
		_, err := tx.ExecContext(ctx,
			`UPDATE categories SET slug=?, type=?, icon=?, color=?, sort_order=?, archived=?, show_on_home=?, updated_at=datetime('now')
			 WHERE id=? AND user_id=?`,
			nullableStr(e.Slug), e.Type, nullableStr(e.Icon), nullableStr(e.Color), e.SortOrder, boolToInt(e.Archived), boolToInt(showOnHome), id, userID)
		if err != nil {
			return 0, false, fmt.Errorf("backup: update category %q: %w", e.Name, err)
		}
		return id, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, fmt.Errorf("backup: find category %q: %w", e.Name, err)
	}
	// Create.
	cols := `INSERT INTO categories (user_id, name, slug, type, icon, color, sort_order, archived, show_on_home`
	vals := `VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?`
	baseArgs := []any{userID, e.Name, nullableStr(e.Slug), e.Type, nullableStr(e.Icon), nullableStr(e.Color), e.SortOrder, boolToInt(e.Archived), boolToInt(showOnHome)}
	if e.CreatedAt != "" {
		cols += `, created_at`
		vals += `, ?`
		baseArgs = append(baseArgs, e.CreatedAt)
	}
	if e.UpdatedAt != "" {
		cols += `, updated_at`
		vals += `, ?`
		baseArgs = append(baseArgs, e.UpdatedAt)
	}
	cols += `)`
	vals += `)`
	res, err := tx.ExecContext(ctx, cols+vals, baseArgs...)
	if err != nil {
		return 0, false, fmt.Errorf("backup: create category %q: %w", e.Name, err)
	}
	newID, _ := res.LastInsertId()
	return newID, true, nil
}

func (s *sqliteStore) UpsertTag(ctx context.Context, tx DB, userID int64, e TagExport) (int64, bool, error) {
	var id int64
	err := tx.QueryRowContext(ctx,
		`SELECT id FROM tags WHERE user_id = ? AND name = ?`, userID, e.Name).Scan(&id)
	if err == nil {
		// Update existing.
		_, err := tx.ExecContext(ctx,
			`UPDATE tags SET color=?, updated_at=datetime('now') WHERE id=? AND user_id=?`,
			nullableStr(e.Color), id, userID)
		if err != nil {
			return 0, false, fmt.Errorf("backup: update tag %q: %w", e.Name, err)
		}
		return id, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, fmt.Errorf("backup: find tag %q: %w", e.Name, err)
	}
	res, err := tx.ExecContext(ctx,
		`INSERT INTO tags (user_id, name, color) VALUES (?, ?, ?)`,
		userID, e.Name, nullableStr(e.Color))
	if err != nil {
		return 0, false, fmt.Errorf("backup: create tag %q: %w", e.Name, err)
	}
	newID, _ := res.LastInsertId()
	return newID, true, nil
}

func (s *sqliteStore) FindCategoryID(ctx context.Context, tx DB, userID int64, name string) (int64, error) {
	if name == "" {
		return 0, nil
	}
	var id int64
	err := tx.QueryRowContext(ctx,
		`SELECT id FROM categories WHERE user_id = ? AND name = ?`, userID, name).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

func (s *sqliteStore) FindTagID(ctx context.Context, tx DB, userID int64, name string) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx,
		`SELECT id FROM tags WHERE user_id = ? AND name = ?`, userID, name).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

func (s *sqliteStore) InsertBookmark(ctx context.Context, tx DB, userID int64, e BookmarkExport, categoryID int64, tagIDs []int64) error {
	normalized, domain, ok := bookmark.NormalizeURL(e.URL)
	if !ok {
		return fmt.Errorf("backup: invalid bookmark url %q", e.URL)
	}
	// Build INSERT dynamically: include created_at/updated_at only when provided
	// (replace mode preserves timestamps; merge mode lets SQLite defaults apply).
	cols := `INSERT INTO bookmarks (user_id, category_id, url, normalized_url, title, description,
		pinned, favorite, sort_order, click_count, status, last_opened_at, domain`
	vals := `VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?`
	baseArgs := []any{userID, nullableInt64(categoryID), e.URL, normalized, e.Title, orEmpty(e.Description),
		boolToInt(e.Pinned), boolToInt(e.Favorite), e.SortOrder, e.ClickCount,
		e.Status, nullableStr(e.LastOpenedAt), domain}
	if e.CreatedAt != "" {
		cols += `, created_at`
		vals += `, ?`
		baseArgs = append(baseArgs, e.CreatedAt)
	}
	if e.UpdatedAt != "" {
		cols += `, updated_at`
		vals += `, ?`
		baseArgs = append(baseArgs, e.UpdatedAt)
	}
	cols += `)`
	vals += `)`
	res, err := tx.ExecContext(ctx, cols+vals, baseArgs...)
	if err != nil {
		return fmt.Errorf("backup: insert bookmark %q: %w", e.URL, err)
	}
	bmID, _ := res.LastInsertId()
	if len(tagIDs) > 0 {
		if err := s.replaceBookmarkTags(ctx, tx, userID, bmID, tagIDs); err != nil {
			return err
		}
	}
	return nil
}

func (s *sqliteStore) UpdateBookmarkByURL(ctx context.Context, tx DB, userID int64, normalizedURL string, e BookmarkExport, categoryID int64, tagIDs []int64) (bool, error) {
	var id int64
	err := tx.QueryRowContext(ctx,
		`SELECT id FROM bookmarks WHERE user_id = ? AND normalized_url = ?`, userID, normalizedURL).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("backup: find bookmark by url: %w", err)
	}
	// Update non-empty fields.
	sets := []string{"category_id = ?", "title = ?", "description = ?",
		"pinned = ?", "favorite = ?", "sort_order = ?", "click_count = ?"}
	args := []any{nullableInt64(categoryID), e.Title, orEmpty(e.Description),
		boolToInt(e.Pinned), boolToInt(e.Favorite), e.SortOrder, e.ClickCount}
	// Only update status when non-empty (merge mode clears it to preserve existing).
	if e.Status != "" {
		sets = append(sets, "status = ?")
		args = append(args, e.Status)
	}
	if e.LastOpenedAt != "" {
		sets = append(sets, "last_opened_at = ?")
		args = append(args, e.LastOpenedAt)
	}
	args = append(args, id, userID)
	q := "UPDATE bookmarks SET " + strings.Join(sets, ", ") + ", updated_at = datetime('now') WHERE id = ? AND user_id = ?"
	if _, err := tx.ExecContext(ctx, q, args...); err != nil {
		return false, fmt.Errorf("backup: update bookmark %q: %w", e.URL, err)
	}
	if err := s.replaceBookmarkTags(ctx, tx, userID, id, tagIDs); err != nil {
		return false, err
	}
	return true, nil
}

func (s *sqliteStore) replaceBookmarkTags(ctx context.Context, tx DB, userID, bookmarkID int64, tagIDs []int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM bookmark_tags WHERE bookmark_id = ? AND user_id = ?`, bookmarkID, userID); err != nil {
		return fmt.Errorf("backup: clear bookmark tags: %w", err)
	}
	seen := make(map[int64]struct{})
	for _, tid := range tagIDs {
		if _, dup := seen[tid]; dup {
			continue
		}
		seen[tid] = struct{}{}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO bookmark_tags (bookmark_id, tag_id, user_id) VALUES (?, ?, ?)`, bookmarkID, tid, userID); err != nil {
			return fmt.Errorf("backup: insert bookmark tag: %w", err)
		}
	}
	return nil
}

func (s *sqliteStore) InsertReadLaterItem(ctx context.Context, tx DB, userID int64, e ReadLaterItemExport, tagIDs []int64) error {
	normalized, domain, ok := bookmark.NormalizeURL(e.URL)
	if !ok {
		return fmt.Errorf("backup: invalid read-later url %q", e.URL)
	}
	cols := `INSERT INTO read_later_items (user_id, url, normalized_url, title, excerpt, author, site_name,
		state, priority, favorite, reading_time_minutes, source, domain,
		last_opened_at, archived_at`
	vals := `VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?`
	baseArgs := []any{userID, e.URL, normalized, e.Title, orEmpty(e.Excerpt), orEmpty(e.Author), orEmpty(e.SiteName),
		e.State, e.Priority, boolToInt(e.Favorite), e.ReadingTimeMinutes, orEmpty(e.Source), domain,
		nullableStr(e.LastOpenedAt), nullableStr(e.ArchivedAt)}
	if e.CreatedAt != "" {
		cols += `, created_at`
		vals += `, ?`
		baseArgs = append(baseArgs, e.CreatedAt)
	}
	if e.UpdatedAt != "" {
		cols += `, updated_at`
		vals += `, ?`
		baseArgs = append(baseArgs, e.UpdatedAt)
	}
	cols += `)`
	vals += `)`
	res, err := tx.ExecContext(ctx, cols+vals, baseArgs...)
	if err != nil {
		return fmt.Errorf("backup: insert read-later %q: %w", e.URL, err)
	}
	rlID, _ := res.LastInsertId()
	if len(tagIDs) > 0 {
		if err := s.replaceReadLaterTags(ctx, tx, userID, rlID, tagIDs); err != nil {
			return err
		}
	}
	return nil
}

func (s *sqliteStore) UpdateReadLaterByURL(ctx context.Context, tx DB, userID int64, normalizedURL string, e ReadLaterItemExport, tagIDs []int64) (bool, error) {
	var id int64
	err := tx.QueryRowContext(ctx,
		`SELECT id FROM read_later_items WHERE user_id = ? AND normalized_url = ?`, userID, normalizedURL).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("backup: find read-later by url: %w", err)
	}
	sets := []string{"title = ?", "excerpt = ?", "author = ?", "site_name = ?",
		"priority = ?", "favorite = ?", "reading_time_minutes = ?", "source = ?"}
	args := []any{e.Title, orEmpty(e.Excerpt), orEmpty(e.Author), orEmpty(e.SiteName),
		e.Priority, boolToInt(e.Favorite), e.ReadingTimeMinutes, orEmpty(e.Source)}
	// Only update state when non-empty (merge mode clears it to preserve existing).
	if e.State != "" {
		sets = append(sets, "state = ?")
		args = append(args, e.State)
	}
	if e.LastOpenedAt != "" {
		sets = append(sets, "last_opened_at = ?")
		args = append(args, e.LastOpenedAt)
	}
	if e.ArchivedAt != "" {
		sets = append(sets, "archived_at = ?")
		args = append(args, e.ArchivedAt)
	}
	args = append(args, id, userID)
	q := "UPDATE read_later_items SET " + strings.Join(sets, ", ") + ", updated_at = datetime('now') WHERE id = ? AND user_id = ?"
	if _, err := tx.ExecContext(ctx, q, args...); err != nil {
		return false, fmt.Errorf("backup: update read-later %q: %w", e.URL, err)
	}
	if err := s.replaceReadLaterTags(ctx, tx, userID, id, tagIDs); err != nil {
		return false, err
	}
	return true, nil
}

func (s *sqliteStore) replaceReadLaterTags(ctx context.Context, tx DB, userID, rlID int64, tagIDs []int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM read_later_tags WHERE read_later_id = ? AND user_id = ?`, rlID, userID); err != nil {
		return fmt.Errorf("backup: clear read-later tags: %w", err)
	}
	seen := make(map[int64]struct{})
	for _, tid := range tagIDs {
		if _, dup := seen[tid]; dup {
			continue
		}
		seen[tid] = struct{}{}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO read_later_tags (read_later_id, tag_id, user_id) VALUES (?, ?, ?)`, rlID, tid, userID); err != nil {
			return fmt.Errorf("backup: insert read-later tag: %w", err)
		}
	}
	return nil
}

// ----- helpers -----

func nullableStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// orEmpty returns s as-is (never nil), for NOT NULL DEFAULT '' columns.
func orEmpty(s string) string {
	return s
}

func nullableInt64(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ----- time helpers -----

// toRFC3339 converts a SQLite datetime string to RFC3339 UTC.
// Empty strings and unparseable values are returned as-is.
func toRFC3339(s string) string {
	if s == "" {
		return ""
	}
	t, err := parseDBTime(s)
	if err != nil {
		return s
	}
	return t.UTC().Format(time.RFC3339)
}

// parseDBTime parses both SQLite datetime and RFC3339 formats.
func parseDBTime(s string) (time.Time, error) {
	if t, err := time.Parse("2006-01-02 15:04:05", s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, s)
}

// fromRFC3339 converts an RFC3339 time string to SQLite datetime format.
// Returns the original string if parsing fails.
func fromRFC3339(s string) string {
	if s == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return s
	}
	return t.UTC().Format("2006-01-02 15:04:05")
}
