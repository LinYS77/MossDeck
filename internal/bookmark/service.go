package bookmark

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/linyusheng/homepage/internal/auth"
)

// Service implements the bookmark business logic: input validation, URL
// normalization, dedup policy, status transitions, and tag-name resolution.
// It depends on a Store (injected) so tests can substitute a fake. The auth
// Service is held only so Register can wrap handlers in auth.RequireAuth.
type Service struct {
	store  Store
	auth   *auth.Service
	logger *slog.Logger
	// itemLimit is the per-import cap on processed bookmarks. It defaults to
	// maxImportItems; kept as a field so tests can exercise truncation cheaply
	// without creating tens of thousands of rows.
	itemLimit int
}

// NewService wires the bookmark service. authSvc is the authentication service
// whose RequireAuth middleware guards every bookmark endpoint.
func NewService(store Store, authSvc *auth.Service, logger *slog.Logger) *Service {
	return &Service{store: store, auth: authSvc, logger: logger, itemLimit: maxImportItems}
}

// =====================================================================
// Categories
// =====================================================================

// CreateCategory validates input and creates a category.
func (s *Service) CreateCategory(ctx context.Context, userID int64, p CreateCategoryParams) (*Category, error) {
	if err := validateCategory(p); err != nil {
		return nil, err
	}
	return s.store.CreateCategory(ctx, userID, p)
}

// UpdateCategory validates and applies an update.
func (s *Service) UpdateCategory(ctx context.Context, userID, id int64, p UpdateCategoryParams) (*Category, error) {
	if p.Name != nil {
		if err := validateCategoryName(*p.Name); err != nil {
			return nil, err
		}
	}
	if p.Type != nil && !isValidCategoryType(*p.Type) {
		return nil, ValidationError{Field: "type", Reason: "must be bookmark, read_later, or all"}
	}
	return s.store.UpdateCategory(ctx, userID, id, p)
}

// DeleteCategory soft-deletes (archives) a category.
func (s *Service) DeleteCategory(ctx context.Context, userID, id int64) error {
	return s.store.DeleteCategory(ctx, userID, id)
}

// =====================================================================
// Tags
// =====================================================================

// CreateTag validates and creates a tag.
func (s *Service) CreateTag(ctx context.Context, userID int64, p CreateTagParams) (*Tag, error) {
	if err := validateTag(p); err != nil {
		return nil, err
	}
	return s.store.CreateTag(ctx, userID, p)
}

// UpdateTag validates and applies a tag update.
func (s *Service) UpdateTag(ctx context.Context, userID, id int64, p UpdateTagParams) (*Tag, error) {
	if p.Name != nil {
		name := strings.TrimSpace(*p.Name)
		if name == "" {
			return nil, ValidationError{Field: "name", Reason: "must not be empty"}
		}
		if len(name) > 64 {
			return nil, ValidationError{Field: "name", Reason: "must be at most 64 characters"}
		}
		p.Name = &name
	}
	return s.store.UpdateTag(ctx, userID, id, p)
}

// =====================================================================
// Bookmarks
// =====================================================================

// validateCategoryOwnership ensures categoryID (when non-zero) exists and
// belongs to userID. Because GetCategory scopes by user_id, a category that
// belongs to another user returns ErrNotFound just like a missing one, which
// we map to a consistent 400 validation error. This prevents a DB foreign-key
// error (or, worse, a cross-user association) from surfacing as a 500.
func (s *Service) validateCategoryOwnership(ctx context.Context, userID, categoryID int64) error {
	if categoryID == 0 {
		return nil // uncategorized is always valid
	}
	if _, err := s.store.GetCategory(ctx, userID, categoryID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return ValidationError{Field: "categoryId", Reason: "category does not exist"}
		}
		return err
	}
	return nil
}

// CreateBookmark validates input, normalizes the URL, enforces the per-user
// dedup policy (409 on duplicate normalized URL), and creates the bookmark.
//
// Tag ids are validated for ownership before insert; unknown tag ids are
// dropped silently to keep the create resilient (the caller asked for them,
// but a missing one shouldn't abort the whole save). categoryId is validated
// strictly: a non-existent or foreign categoryId is a 400.
func (s *Service) CreateBookmark(ctx context.Context, userID int64, p CreateBookmarkParams) (*Bookmark, error) {
	normalized, domain, ok := validateURL(p.URL)
	if !ok {
		return nil, ValidationError{Field: "url", Reason: "must be a valid http(s) URL"}
	}
	title := strings.TrimSpace(p.Title)
	if title == "" {
		// Default the title to the host so the UI always has something to show;
		// the metadata-fetch task will later overwrite it.
		title = domain
	}
	description := strings.TrimSpace(p.Description)

	// Strictly validate the categoryId references one of the user's own
	// categories (cross-user or missing -> 400, never a FK 500).
	if err := s.validateCategoryOwnership(ctx, userID, p.CategoryID); err != nil {
		return nil, err
	}

	// Validate tag ownership up front; keep only ids the user actually owns.
	tagIDs, err := s.filterOwnedTagIDs(ctx, userID, p.TagIDs)
	if err != nil {
		return nil, err
	}

	b := &Bookmark{
		URL:           p.URL,
		NormalizedURL: normalized,
		Title:         title,
		Description:   description,
		Domain:        domain,
		CategoryID:    p.CategoryID,
		Pinned:        p.Pinned,
		Favorite:      p.Favorite,
		SortOrder:     p.SortOrder,
		Status:        StatusActive,
	}
	created, err := s.store.CreateBookmark(ctx, userID, b, tagIDs)
	if err != nil {
		if errors.Is(err, ErrBookmarkURLTaken) {
			return nil, ErrBookmarkURLTaken
		}
		return nil, err
	}
	return created, nil
}

// UpdateBookmark validates and applies a partial update. When the URL changes,
// it is re-normalized and re-checked for dedup (the row's own id is excluded).
func (s *Service) UpdateBookmark(ctx context.Context, userID, id int64, p UpdateBookmarkParams) (*Bookmark, error) {
	if p.URL != nil {
		normalized, domain, ok := validateURL(*p.URL)
		if !ok {
			return nil, ValidationError{Field: "url", Reason: "must be a valid http(s) URL"}
		}
		// If the normalized URL is changing, enforce dedup against other rows.
		existing, err := s.store.GetBookmark(ctx, userID, id)
		if err != nil {
			return nil, err
		}
		if normalized != existing.NormalizedURL {
			other, err := s.store.GetBookmarkByNormalizedURL(ctx, userID, normalized)
			if err == nil && other.ID != id {
				return nil, ErrBookmarkURLTaken
			} else if err != nil && !errors.Is(err, ErrNotFound) {
				return nil, err
			}
		}
		// Carry the derived normalized URL + domain so the UPDATE keeps them in
		// sync with the raw URL and domain filters stay accurate.
		p.NormalizedURL = &normalized
		p.Domain = &domain
	}
	if p.Title != nil {
		t := strings.TrimSpace(*p.Title)
		p.Title = &t
	}
	if p.Description != nil {
		d := strings.TrimSpace(*p.Description)
		p.Description = &d
	}
	if p.Status != nil && !isValidStatus(*p.Status) {
		return nil, ErrInvalidStatus
	}
	if p.CategoryID != nil {
		// A category change must still reference one of the user's own
		// categories; 0 means "uncategorized" and is allowed.
		if err := s.validateCategoryOwnership(ctx, userID, *p.CategoryID); err != nil {
			return nil, err
		}
	}
	if p.TagIDs != nil {
		owned, err := s.filterOwnedTagIDs(ctx, userID, *p.TagIDs)
		if err != nil {
			return nil, err
		}
		p.TagIDs = &owned
	}
	return s.store.UpdateBookmark(ctx, userID, id, p)
}

// Archive moves a bookmark to the archived status. Archive on an already-
// archived or trashed bookmark is allowed (idempotent-ish) but trashed items
// must be restored first — we surface that as NotFound to avoid leaking state.
func (s *Service) Archive(ctx context.Context, userID, id int64) (*Bookmark, error) {
	return s.store.SetBookmarkStatus(ctx, userID, id, StatusArchived)
}

// Trash soft-deletes a bookmark (status=trash). It is idempotent.
func (s *Service) Trash(ctx context.Context, userID, id int64) (*Bookmark, error) {
	return s.store.SetBookmarkStatus(ctx, userID, id, StatusTrash)
}

// Restore brings a soft-deleted/archived bookmark back to active.
func (s *Service) Restore(ctx context.Context, userID, id int64) (*Bookmark, error) {
	return s.store.SetBookmarkStatus(ctx, userID, id, StatusActive)
}

// Open records a click/open on a bookmark (increments count, stamps
// last_opened_at). It does not return the bookmark body; the response is just
// an acknowledgement.
func (s *Service) Open(ctx context.Context, userID, id int64) error {
	return s.store.IncrementClickCount(ctx, userID, id)
}

// List resolves the tag-name filter (if any) and returns a page of bookmarks.
func (s *Service) List(ctx context.Context, q ListQuery) (*ListResult, error) {
	// If a tag name was supplied, resolve it to a tag id (scoped to the user).
	// An unknown tag name yields an empty result rather than an error.
	if q.Tag != "" && q.TagID == 0 {
		t, err := s.store.GetTagByName(ctx, q.UserID, q.Tag)
		if err == nil {
			q.TagID = t.ID
		} else if errors.Is(err, ErrNotFound) {
			return &ListResult{Items: []Bookmark{}, Page: max1(q.Page), PageSize: pageSizeOrDefault(q.PageSize), Total: 0}, nil
		} else {
			return nil, err
		}
	}
	return s.store.ListBookmarks(ctx, q)
}

// filterOwnedTagIDs returns the subset of tagIDs that belong to userID. It
// keeps the create/update resilient: a stale or wrong tag id is dropped rather
// than failing the whole operation.
func (s *Service) filterOwnedTagIDs(ctx context.Context, userID int64, tagIDs []int64) ([]int64, error) {
	if len(tagIDs) == 0 {
		return nil, nil
	}
	owned, err := s.store.ListTags(ctx, userID)
	if err != nil {
		return nil, err
	}
	allowed := make(map[int64]struct{}, len(owned))
	for _, t := range owned {
		allowed[t.ID] = struct{}{}
	}
	out := make([]int64, 0, len(tagIDs))
	seen := make(map[int64]struct{}, len(tagIDs))
	for _, id := range tagIDs {
		if id == 0 {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		if _, ok := allowed[id]; ok {
			out = append(out, id)
		}
	}
	return out, nil
}

// =====================================================================
// Validation helpers
// =====================================================================

func validateCategory(p CreateCategoryParams) error {
	if err := validateCategoryName(p.Name); err != nil {
		return err
	}
	if !isValidCategoryType(p.Type) {
		// Treat empty as the default rather than an error so callers can omit it.
		if p.Type != "" {
			return ValidationError{Field: "type", Reason: "must be bookmark, read_later, or all"}
		}
	}
	return nil
}

func validateCategoryName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ValidationError{Field: "name", Reason: "must not be empty"}
	}
	if len(name) > 64 {
		return ValidationError{Field: "name", Reason: "must be at most 64 characters"}
	}
	return nil
}

func isValidCategoryType(t string) bool {
	switch t {
	case "", "bookmark", "read_later", "all":
		return true
	}
	return false
}

func validateTag(p CreateTagParams) error {
	name := strings.TrimSpace(p.Name)
	if name == "" {
		return ValidationError{Field: "name", Reason: "must not be empty"}
	}
	if len(name) > 64 {
		return ValidationError{Field: "name", Reason: "must be at most 64 characters"}
	}
	return nil
}

func max1(v int) int {
	if v < 1 {
		return 1
	}
	return v
}

func pageSizeOrDefault(v int) int {
	if v <= 0 {
		return defaultPageSize
	}
	if v > maxPageSize {
		return maxPageSize
	}
	return v
}
