package backup

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/linyusheng/homepage/internal/auth"
	"github.com/linyusheng/homepage/internal/bookmark"
)

// Service implements backup business rules.
type Service struct {
	store  Store
	db     *sql.DB
	auth   *auth.Service
	logger *slog.Logger
}

// NewService creates a Service.
func NewService(store Store, db *sql.DB, authSvc *auth.Service, logger *slog.Logger) *Service {
	return &Service{store: store, db: db, auth: authSvc, logger: logger}
}

// Export returns a JSON snapshot of all user data.
func (s *Service) Export(ctx context.Context, userID int64) (*ExportData, error) {
	return s.store.ExportAll(ctx, userID)
}

// Import merges or replaces the current user's data with the provided backup.
// The entire operation runs in a single transaction; on any error the database
// is rolled back to its pre-import state.
func (s *Service) Import(ctx context.Context, userID int64, req ImportRequest) (*ImportSummary, error) {
	// Validate mode.
	mode := strings.TrimSpace(strings.ToLower(req.Mode))
	if mode != "merge" && mode != "replace" {
		return &ImportSummary{
			Mode:   mode,
			Errors: []string{ErrInvalidMode.Error()},
		}, ErrInvalidMode
	}

	// Validate backup structure.
	summary := &ImportSummary{Mode: mode}
	b := req.Backup
	if b.Version != Version {
		summary.Errors = append(summary.Errors, fmt.Sprintf("unsupported version %d (expected %d)", b.Version, Version))
		return summary, ErrInvalidBackup
	}
	if b.App != "" && b.App != App {
		summary.Errors = append(summary.Errors, fmt.Sprintf("unknown app %q", b.App))
		return summary, ErrInvalidBackup
	}

	// Validate URLs in the backup data so we fail fast before touching the DB.
	for i, bm := range b.Data.Bookmarks {
		if _, _, ok := bookmark.NormalizeURL(bm.URL); !ok {
			summary.Errors = append(summary.Errors, fmt.Sprintf("bookmark[%d]: invalid url %q", i, bm.URL))
		}
	}
	for i, rl := range b.Data.ReadLaterItems {
		if _, _, ok := bookmark.NormalizeURL(rl.URL); !ok {
			summary.Errors = append(summary.Errors, fmt.Sprintf("readLater[%d]: invalid url %q", i, rl.URL))
		}
	}

	// Validate field lengths (defensive caps).
	const maxLen = 2000
	for i, bm := range b.Data.Bookmarks {
		if len(bm.Title) > maxLen {
			summary.Errors = append(summary.Errors, fmt.Sprintf("bookmark[%d]: title too long (%d)", i, len(bm.Title)))
		}
	}
	for i, rl := range b.Data.ReadLaterItems {
		if len(rl.Title) > maxLen {
			summary.Errors = append(summary.Errors, fmt.Sprintf("readLater[%d]: title too long (%d)", i, len(rl.Title)))
		}
	}

	// Cap entity counts.
	const maxEntities = 10000
	if len(b.Data.Bookmarks) > maxEntities {
		summary.Errors = append(summary.Errors, fmt.Sprintf("too many bookmarks: %d (max %d)", len(b.Data.Bookmarks), maxEntities))
	}
	if len(b.Data.ReadLaterItems) > maxEntities {
		summary.Errors = append(summary.Errors, fmt.Sprintf("too many read-later items: %d (max %d)", len(b.Data.ReadLaterItems), maxEntities))
	}
	if len(b.Data.Categories) > maxEntities {
		summary.Errors = append(summary.Errors, fmt.Sprintf("too many categories: %d (max %d)", len(b.Data.Categories), maxEntities))
	}
	if len(b.Data.Tags) > maxEntities {
		summary.Errors = append(summary.Errors, fmt.Sprintf("too many tags: %d (max %d)", len(b.Data.Tags), maxEntities))
	}

	// --- Enum validation + normalization ---
	for i := range b.Data.Categories {
		t := strings.TrimSpace(b.Data.Categories[i].Type)
		if t == "" {
			t = "bookmark"
		}
		if t != "bookmark" && t != "read_later" && t != "all" {
			summary.Errors = append(summary.Errors, fmt.Sprintf("category[%d]: invalid type %q", i, b.Data.Categories[i].Type))
		} else {
			b.Data.Categories[i].Type = t // normalize in-place
		}
	}
	for i := range b.Data.Bookmarks {
		st := strings.TrimSpace(b.Data.Bookmarks[i].Status)
		if st == "" {
			st = "active"
		}
		if st != "active" && st != "archived" && st != "trash" {
			summary.Errors = append(summary.Errors, fmt.Sprintf("bookmark[%d]: invalid status %q", i, b.Data.Bookmarks[i].Status))
		} else {
			b.Data.Bookmarks[i].Status = st
		}
	}
	for i := range b.Data.ReadLaterItems {
		st := strings.TrimSpace(b.Data.ReadLaterItems[i].State)
		if st == "" {
			st = "unread"
		}
		if st != "unread" && st != "reading" && st != "archived" && st != "trash" {
			summary.Errors = append(summary.Errors, fmt.Sprintf("readLater[%d]: invalid state %q", i, b.Data.ReadLaterItems[i].State))
		} else {
			b.Data.ReadLaterItems[i].State = st
		}
	}

	// --- Time validation ---
	for i, bm := range b.Data.Bookmarks {
		if bm.CreatedAt != "" {
			if _, err := parseTime(bm.CreatedAt); err != nil {
				summary.Errors = append(summary.Errors, fmt.Sprintf("bookmark[%d]: invalid createdAt %q", i, bm.CreatedAt))
			}
		}
		if bm.UpdatedAt != "" {
			if _, err := parseTime(bm.UpdatedAt); err != nil {
				summary.Errors = append(summary.Errors, fmt.Sprintf("bookmark[%d]: invalid updatedAt %q", i, bm.UpdatedAt))
			}
		}
		if bm.LastOpenedAt != "" {
			if _, err := parseTime(bm.LastOpenedAt); err != nil {
				summary.Errors = append(summary.Errors, fmt.Sprintf("bookmark[%d]: invalid lastOpenedAt %q", i, bm.LastOpenedAt))
			}
		}
	}
	for i, rl := range b.Data.ReadLaterItems {
		for _, field := range []struct{ name, val string }{
			{"createdAt", rl.CreatedAt}, {"updatedAt", rl.UpdatedAt},
			{"lastOpenedAt", rl.LastOpenedAt}, {"archivedAt", rl.ArchivedAt},
		} {
			if field.val != "" {
				if _, err := parseTime(field.val); err != nil {
					summary.Errors = append(summary.Errors, fmt.Sprintf("readLater[%d]: invalid %s %q", i, field.name, field.val))
				}
			}
		}
	}

	// --- Category time validation ---
	for i, cat := range b.Data.Categories {
		if cat.CreatedAt != "" {
			if _, err := parseTime(cat.CreatedAt); err != nil {
				summary.Errors = append(summary.Errors, fmt.Sprintf("category[%d]: invalid createdAt %q", i, cat.CreatedAt))
			}
		}
		if cat.UpdatedAt != "" {
			if _, err := parseTime(cat.UpdatedAt); err != nil {
				summary.Errors = append(summary.Errors, fmt.Sprintf("category[%d]: invalid updatedAt %q", i, cat.UpdatedAt))
			}
		}
	}

	// --- Duplicate URL check within backup ---
	bmURLs := make(map[string]int) // normalized URL -> first index
	for i, bm := range b.Data.Bookmarks {
		norm, _, ok := bookmark.NormalizeURL(bm.URL)
		if !ok {
			continue // already reported above
		}
		if bm.Status == "trash" {
			continue // trashed items don't conflict with unique constraint
		}
		if first, exists := bmURLs[norm]; exists {
			summary.Errors = append(summary.Errors, fmt.Sprintf("duplicate bookmark url %q at indices %d and %d", bm.URL, first, i))
		} else {
			bmURLs[norm] = i
		}
	}
	rlURLs := make(map[string]int)
	for i, rl := range b.Data.ReadLaterItems {
		norm, _, ok := bookmark.NormalizeURL(rl.URL)
		if !ok {
			continue
		}
		if rl.State == "trash" {
			continue
		}
		if first, exists := rlURLs[norm]; exists {
			summary.Errors = append(summary.Errors, fmt.Sprintf("duplicate read-later url %q at indices %d and %d", rl.URL, first, i))
		} else {
			rlURLs[norm] = i
		}
	}

	// --- Duplicate category/tag name check within backup ---
	catNames := make(map[string]int)
	for i, cat := range b.Data.Categories {
		n := strings.ToLower(strings.TrimSpace(cat.Name))
		if n == "" {
			continue
		}
		if first, exists := catNames[n]; exists {
			summary.Errors = append(summary.Errors, fmt.Sprintf("duplicate category name %q at indices %d and %d", cat.Name, first, i))
		} else {
			catNames[n] = i
		}
	}
	tagNames := make(map[string]int)
	for i, tag := range b.Data.Tags {
		n := strings.ToLower(strings.TrimSpace(tag.Name))
		if n == "" {
			continue
		}
		if first, exists := tagNames[n]; exists {
			summary.Errors = append(summary.Errors, fmt.Sprintf("duplicate tag name %q at indices %d and %d", tag.Name, first, i))
		} else {
			tagNames[n] = i
		}
	}

	if len(summary.Errors) > 0 {
		return summary, ErrInvalidBackup
	}

	// Execute import in a transaction.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("backup: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	switch mode {
	case "replace":
		if err := s.store.ClearUserData(ctx, tx, userID); err != nil {
			return nil, err
		}
		if err := s.importAll(ctx, tx, userID, b.Data, summary, true); err != nil {
			return nil, err
		}
	case "merge":
		if err := s.importAll(ctx, tx, userID, b.Data, summary, false); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("backup: commit: %w", err)
	}
	return summary, nil
}

// importAll processes all four entity groups. The clear flag controls whether
// we treat every entity as a create (replace mode) or do upserts (merge mode).
func (s *Service) importAll(ctx context.Context, tx DB, userID int64, data ExportPayload, summary *ImportSummary, clear bool) error {
	// --- Categories ---
	catIDs := make(map[string]int64) // name -> id after upsert
	for _, e := range data.Categories {
		if e.Name == "" {
			summary.Warnings = append(summary.Warnings, "skipped category with empty name")
			summary.Categories.Skipped++
			continue
		}
		// Replace mode: convert times to SQLite format.
		if clear {
			e.CreatedAt = normTime(e.CreatedAt)
			e.UpdatedAt = normTime(e.UpdatedAt)
		}
		id, created, err := s.store.UpsertCategory(ctx, tx, userID, e)
		if err != nil {
			return err
		}
		catIDs[e.Name] = id
		if created {
			summary.Categories.Created++
		} else {
			summary.Categories.Updated++
		}
	}

	// --- Tags ---
	tagIDs := make(map[string]int64) // name -> id after upsert
	for _, e := range data.Tags {
		if e.Name == "" {
			summary.Warnings = append(summary.Warnings, "skipped tag with empty name")
			summary.Tags.Skipped++
			continue
		}
		id, created, err := s.store.UpsertTag(ctx, tx, userID, e)
		if err != nil {
			return err
		}
		tagIDs[e.Name] = id
		if created {
			summary.Tags.Created++
		} else {
			summary.Tags.Updated++
		}
	}

	// --- Bookmarks ---
	for _, e := range data.Bookmarks {
		// Normalize enums.
		if e.Status == "" {
			e.Status = "active"
		}
		// Replace mode: convert times to SQLite format.
		if clear {
			e.CreatedAt = normTime(e.CreatedAt)
			e.UpdatedAt = normTime(e.UpdatedAt)
			e.LastOpenedAt = normTime(e.LastOpenedAt)
		}

		// Resolve category.
		var catID int64
		if e.Category != "" {
			if id, ok := catIDs[e.Category]; ok {
				catID = id
			} else if id, err := s.store.FindCategoryID(ctx, tx, userID, e.Category); err != nil {
				return err
			} else if id != 0 {
				catID = id
			} else {
				summary.Warnings = append(summary.Warnings, fmt.Sprintf("bookmark %q: category %q not found, leaving uncategorized", e.URL, e.Category))
			}
		}

		// Resolve tags.
		var tIDs []int64
		for _, tName := range e.Tags {
			if id, ok := tagIDs[tName]; ok {
				tIDs = append(tIDs, id)
			} else if id, err := s.store.FindTagID(ctx, tx, userID, tName); err != nil {
				return err
			} else if id != 0 {
				tIDs = append(tIDs, id)
			} else {
				summary.Warnings = append(summary.Warnings, fmt.Sprintf("bookmark %q: tag %q not found, skipped", e.URL, tName))
			}
		}

		normalized, _, ok := bookmark.NormalizeURL(e.URL)
		if !ok {
			summary.Errors = append(summary.Errors, fmt.Sprintf("bookmark %q: invalid url", e.URL))
			continue
		}

		if !clear {
			// Merge: don't overwrite existing status/state with backup values.
			e.Status = ""
			updated, err := s.store.UpdateBookmarkByURL(ctx, tx, userID, normalized, e, catID, tIDs)
			if err != nil {
				return err
			}
			if updated {
				summary.Bookmarks.Updated++
				continue
			}
		}
		// Create new.
		if err := s.store.InsertBookmark(ctx, tx, userID, e, catID, tIDs); err != nil {
			return err
		}
		summary.Bookmarks.Created++
	}

	// --- Read-later items ---
	for _, e := range data.ReadLaterItems {
		// Normalize enums.
		if e.State == "" {
			e.State = "unread"
		}
		// Replace mode: convert times to SQLite format.
		if clear {
			e.CreatedAt = normTime(e.CreatedAt)
			e.UpdatedAt = normTime(e.UpdatedAt)
			e.LastOpenedAt = normTime(e.LastOpenedAt)
			e.ArchivedAt = normTime(e.ArchivedAt)
		}

		var tIDs []int64
		for _, tName := range e.Tags {
			if id, ok := tagIDs[tName]; ok {
				tIDs = append(tIDs, id)
			} else if id, err := s.store.FindTagID(ctx, tx, userID, tName); err != nil {
				return err
			} else if id != 0 {
				tIDs = append(tIDs, id)
			} else {
				summary.Warnings = append(summary.Warnings, fmt.Sprintf("read-later %q: tag %q not found, skipped", e.URL, tName))
			}
		}

		normalized, _, ok := bookmark.NormalizeURL(e.URL)
		if !ok {
			summary.Errors = append(summary.Errors, fmt.Sprintf("read-later %q: invalid url", e.URL))
			continue
		}

		if !clear {
			// Merge: don't overwrite existing state with backup values.
			e.State = ""
			updated, err := s.store.UpdateReadLaterByURL(ctx, tx, userID, normalized, e, tIDs)
			if err != nil {
				return err
			}
			if updated {
				summary.ReadLaterItems.Updated++
				continue
			}
		}
		if err := s.store.InsertReadLaterItem(ctx, tx, userID, e, tIDs); err != nil {
			return err
		}
		summary.ReadLaterItems.Created++
	}

	// Squash errors into summary but don't return them (errors at this point
	// are per-item and we've already recorded them).
	return nil
}

// currentUserID extracts the authenticated user from the context set by
// auth.RequireAuth.
func currentUserID(ctx context.Context) int64 {
	u, ok := auth.UserFromContext(ctx)
	if !ok {
		return 0
	}
	return u.ID
}

// parseTime parses a time string in RFC3339 or SQLite datetime format.
func parseTime(s string) (bool, error) {
	if s == "" {
		return true, nil
	}
	if _, err := time.Parse(time.RFC3339, s); err == nil {
		return true, nil
	}
	if _, err := time.Parse("2006-01-02 15:04:05", s); err == nil {
		return true, nil
	}
	return false, fmt.Errorf("invalid time format: %q", s)
}

// normTime converts an RFC3339 time string to SQLite datetime format.
func normTime(s string) string {
	if s == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return s
	}
	return t.UTC().Format("2006-01-02 15:04:05")
}
