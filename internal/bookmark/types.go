// Package bookmark implements the bookmark core backend: categories, tags,
// bookmarks CRUD, search/filter, and soft-delete/restore.
//
// It mirrors the established internal/auth layering:
//
//   - Store    : persistence behind an interface (tests can substitute a fake).
//   - Service  : business rules (validation, dedup, status transitions).
//   - handlers : HTTP decoding, response shaping, status mapping.
//
// Every query is scoped by user_id (auth.UserFromContext). The app is
// single-user today, but the isolation boundary is mandatory, not optional.
//
// Persistence conventions follow 0001_init.sql / 0002_bookmarks.sql:
//   - Timestamps are ISO-8601 TEXT (UTC) via datetime('now').
//   - Booleans are INTEGER 0/1 in SQLite and bool in Go.
//   - Soft delete uses bookmark.status='trash' (no hard deletes).
package bookmark

import (
	"context"
	"errors"
)

// Sentinel domain errors. Handlers map these to HTTP responses; callers must
// use errors.Is to test for them.
var (
	// ErrNotFound is returned for a missing category, tag, or bookmark.
	ErrNotFound = errors.New("bookmark: not found")
	// ErrCategoryNameTaken is returned when creating/updating a category to a
	// name that already exists for the user.
	ErrCategoryNameTaken = errors.New("bookmark: category name already taken")
	// ErrTagNameTaken is returned when creating/updating a tag to a name that
	// already exists for the user.
	ErrTagNameTaken = errors.New("bookmark: tag name already taken")
	// ErrBookmarkURLTaken is returned when creating/updating a bookmark to a
	// normalized URL that already exists (and is not trashed) for the user.
	ErrBookmarkURLTaken = errors.New("bookmark: bookmark url already exists")
	// ErrInvalidStatus is returned for an unknown bookmark status value.
	ErrInvalidStatus = errors.New("bookmark: invalid status")
)

// Bookmark status constants. These match the CHECK-ish domain enforced in the
// service layer (SQLite TEXT has no CHECK in our schema).
const (
	StatusActive   = "active"
	StatusArchived = "archived"
	StatusTrash    = "trash"
)

// isValidStatus reports whether s is a known bookmark status.
func isValidStatus(s string) bool {
	switch s {
	case StatusActive, StatusArchived, StatusTrash:
		return true
	}
	return false
}

// ValidationError describes an invalid request field. Handlers map it to 400.
type ValidationError struct {
	Field  string
	Reason string
}

func (e ValidationError) Error() string {
	if e.Field == "" {
		return e.Reason
	}
	return e.Field + ": " + e.Reason
}

// Category is the in-memory representation of a categories row.
type Category struct {
	ID            int64
	UserID        int64
	ParentID      int64 // 0 = none
	Name          string
	Slug          string
	Type          string
	Icon          string
	Color         string
	SortOrder     int
	Archived      bool
	ShowOnHome    bool // surface this category's bookmarks on the homepage
	CreatedAt     string
	UpdatedAt     string
	BookmarkCount int // optional aggregate, 0 when not computed
}

// Tag is the in-memory representation of a tags row.
type Tag struct {
	ID            int64
	UserID        int64
	Name          string
	Color         string
	CreatedAt     string
	UpdatedAt     string
	BookmarkCount int // optional aggregate, 0 when not computed
}

// Bookmark is the in-memory representation of a bookmarks row.
type Bookmark struct {
	ID              int64
	UserID          int64
	CategoryID      int64 // 0 = uncategorized
	URL             string
	NormalizedURL   string
	Title           string
	Description     string
	FaviconURL      string
	PreviewImageURL string
	Domain          string
	Pinned          bool
	Favorite        bool
	SortOrder       int
	ClickCount      int
	Status          string
	MetadataStatus  string
	LastOpenedAt    string
	CreatedAt       string
	UpdatedAt       string
	Tags            []Tag // populated for detail/list when requested
}

// CreateCategoryParams is the input for creating a category.
type CreateCategoryParams struct {
	Name      string
	Slug      string
	Type      string
	Icon      string
	Color     string
	ParentID  int64
	SortOrder int
}

// UpdateCategoryParams carries optional category fields. Pointers are nil for
// "leave unchanged"; the service decides which to apply.
type UpdateCategoryParams struct {
	Name       *string
	Slug       *string
	Type       *string
	Icon       *string
	Color      *string
	ParentID   *int64
	SortOrder  *int
	Archived   *bool
	ShowOnHome *bool
}

// CreateTagParams is the input for creating a tag.
type CreateTagParams struct {
	Name  string
	Color string
}

// UpdateTagParams carries optional tag fields.
type UpdateTagParams struct {
	Name  *string
	Color *string
}

// CreateBookmarkParams is the input for creating a bookmark.
type CreateBookmarkParams struct {
	URL         string
	Title       string
	Description string
	CategoryID  int64
	TagIDs      []int64
	Pinned      bool
	Favorite    bool
	SortOrder   int
}

// UpdateBookmarkParams carries optional bookmark fields. TagIDs replaces the
// full tag set when non-nil.
type UpdateBookmarkParams struct {
	URL           *string
	NormalizedURL *string // set by the service when URL changes
	Domain        *string // set by the service when URL changes
	Title         *string
	Description   *string
	CategoryID    *int64
	TagIDs        *[]int64
	Pinned        *bool
	Favorite      *bool
	SortOrder     *int
	Status        *string
}

// ListQuery describes a bookmark list/filter/sort/pagination request.
type ListQuery struct {
	UserID     int64
	Q          string // free text over title/url/description/domain
	CategoryID int64  // 0 = any
	TagID      int64  // 0 = any
	Tag        string // tag name (resolved to TagID by the service when set)
	Status     string // "" = active (default visible set)
	Domain     string // "" = any
	Favorite   *bool  // nil = any
	Pinned     *bool  // nil = any
	Page       int    // 1-based
	PageSize   int
	Sort       string
}

// ListResult is a page of bookmarks plus pagination metadata.
type ListResult struct {
	Items    []Bookmark `json:"items"`
	Page     int        `json:"page"`
	PageSize int        `json:"pageSize"`
	Total    int        `json:"total"`
}

// Result bounds for pagination, applied by the service.
const (
	defaultPageSize = 20
	maxPageSize     = 100
)

// Store is the persistence boundary for bookmark data. The concrete
// implementation is a thin SQLite wrapper; tests may substitute a fake.
type Store interface {
	// --- categories ---
	CreateCategory(ctx context.Context, userID int64, p CreateCategoryParams) (*Category, error)
	GetCategory(ctx context.Context, userID, id int64) (*Category, error)
	ListCategories(ctx context.Context, userID int64) ([]*Category, error)
	UpdateCategory(ctx context.Context, userID, id int64, p UpdateCategoryParams) (*Category, error)
	DeleteCategory(ctx context.Context, userID, id int64) error // sets archived=1

	// --- tags ---
	CreateTag(ctx context.Context, userID int64, p CreateTagParams) (*Tag, error)
	GetTag(ctx context.Context, userID, id int64) (*Tag, error)
	ListTags(ctx context.Context, userID int64) ([]*Tag, error)
	UpdateTag(ctx context.Context, userID, id int64, p UpdateTagParams) (*Tag, error)
	DeleteTag(ctx context.Context, userID, id int64) error
	GetTagByName(ctx context.Context, userID int64, name string) (*Tag, error)

	// --- bookmarks ---
	CreateBookmark(ctx context.Context, userID int64, b *Bookmark, tagIDs []int64) (*Bookmark, error)
	GetBookmark(ctx context.Context, userID, id int64) (*Bookmark, error)
	GetBookmarkByNormalizedURL(ctx context.Context, userID int64, normalizedURL string) (*Bookmark, error)
	ListBookmarks(ctx context.Context, q ListQuery) (*ListResult, error)
	UpdateBookmark(ctx context.Context, userID, id int64, p UpdateBookmarkParams) (*Bookmark, error)
	SetBookmarkStatus(ctx context.Context, userID, id int64, status string) (*Bookmark, error)
	IncrementClickCount(ctx context.Context, userID, id int64) error

	// TagIDsForBookmark returns the tag ids attached to a bookmark.
	TagIDsForBookmark(ctx context.Context, userID, bookmarkID int64) ([]int64, error)
}
