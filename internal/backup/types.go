// Package backup implements JSON export/import of the current user's data
// (categories, tags, bookmarks, read-later items and their tag associations).
// Auth/session/password data is deliberately excluded.
package backup

import (
	"context"
	"database/sql"
	"errors"
)

// Version is the current backup format version. The importer rejects unknown versions.
const Version = 1

// App identifier embedded in every backup for cross-verification.
const App = "homepage"

// Sentinel errors.
var (
	ErrInvalidBackup  = errors.New("backup: invalid backup format or version")
	ErrImportFailed   = errors.New("backup: import failed")
	ErrInvalidMode    = errors.New("backup: invalid import mode, must be merge or replace")
)

// MaxImportBytes caps the request body for the import endpoint.
const MaxImportBytes = 10 << 20 // 10 MiB

// ----- JSON schema types (exposed to the API consumer) -----

// ExportData is the top-level JSON structure for both export and import.
type ExportData struct {
	Version    int           `json:"version"`
	App        string        `json:"app"`
	ExportedAt string        `json:"exportedAt"`
	Data       ExportPayload `json:"data"`
}

// ExportPayload holds the four entity groups. Every field is a slice (never
// null) so consumers can rely on a stable JSON shape.
type ExportPayload struct {
	Categories     []CategoryExport     `json:"categories"`
	Tags           []TagExport          `json:"tags"`
	Bookmarks      []BookmarkExport     `json:"bookmarks"`
	ReadLaterItems []ReadLaterItemExport `json:"readLaterItems"`
}

// CategoryExport mirrors a categories row without internal ids.
//
// ShowOnHome is a pointer so importing an older backup that predates the
// flag (field absent -> nil) can fall back to the "show" default rather than
// silently hiding every category.
type CategoryExport struct {
	Name       string `json:"name"`
	Slug       string `json:"slug,omitempty"`
	Type       string `json:"type"`
	Icon       string `json:"icon,omitempty"`
	Color      string `json:"color,omitempty"`
	SortOrder  int    `json:"sortOrder"`
	Archived   bool   `json:"archived"`
	ShowOnHome *bool  `json:"showOnHome,omitempty"`
	CreatedAt  string `json:"createdAt,omitempty"`
	UpdatedAt  string `json:"updatedAt,omitempty"`
}

// TagExport mirrors a tags row without internal ids. Tags are matched by name
// during import.
type TagExport struct {
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
}

// BookmarkExport mirrors a bookmarks row. Tags and category are referenced by
// name so the backup is human-readable and portable.
type BookmarkExport struct {
	URL          string   `json:"url"`
	Title        string   `json:"title"`
	Description  string   `json:"description,omitempty"`
	Category     string   `json:"category,omitempty"` // category name, not id
	Tags         []string `json:"tags,omitempty"`      // tag names, not ids
	Pinned       bool     `json:"pinned"`
	Favorite     bool     `json:"favorite"`
	SortOrder    int      `json:"sortOrder"`
	ClickCount   int      `json:"clickCount"`
	Status       string   `json:"status"`
	CreatedAt    string   `json:"createdAt,omitempty"`
	UpdatedAt    string   `json:"updatedAt,omitempty"`
	LastOpenedAt string   `json:"lastOpenedAt,omitempty"`
}

// ReadLaterItemExport mirrors read_later_items. Tags are referenced by name.
type ReadLaterItemExport struct {
	URL                string   `json:"url"`
	Title              string   `json:"title"`
	Excerpt            string   `json:"excerpt,omitempty"`
	Author             string   `json:"author,omitempty"`
	SiteName           string   `json:"siteName,omitempty"`
	Tags               []string `json:"tags,omitempty"`
	State              string   `json:"state"`
	Priority           int      `json:"priority"`
	Favorite           bool     `json:"favorite"`
	ReadingTimeMinutes int      `json:"readingTimeMinutes"`
	Source             string   `json:"source,omitempty"`
	CreatedAt          string   `json:"createdAt,omitempty"`
	UpdatedAt          string   `json:"updatedAt,omitempty"`
	LastOpenedAt       string   `json:"lastOpenedAt,omitempty"`
	ArchivedAt         string   `json:"archivedAt,omitempty"`
}

// ----- Import -----

// ImportRequest is the JSON body for POST /api/v1/backup/import.
type ImportRequest struct {
	Mode   string     `json:"mode"`
	Backup ExportData `json:"backup"`
}

// ImportSummary is returned after a successful import so the caller can
// inspect what happened.
type ImportSummary struct {
	Mode           string       `json:"mode"`
	Categories     CountSummary `json:"categories"`
	Tags           CountSummary `json:"tags"`
	Bookmarks      CountSummary `json:"bookmarks"`
	ReadLaterItems CountSummary `json:"readLaterItems"`
	Errors         []string     `json:"errors,omitempty"`
	Warnings       []string     `json:"warnings,omitempty"`
}

// CountSummary reports creates/updates/skips for one entity group.
type CountSummary struct {
	Created int `json:"created"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
}

// ----- Store interface -----

// Store abstracts persistence so the service can be tested with a real
// database. All writes happen inside a caller-managed transaction (tx).
type Store interface {
	// ExportAll returns a complete snapshot of the user's data.
	ExportAll(ctx context.Context, userID int64) (*ExportData, error)

	// ClearUserData removes all categories, tags, bookmarks, read-later
	// items (and their join-table rows) for a user. Must run inside tx.
	ClearUserData(ctx context.Context, tx DB, userID int64) error

	// Import helpers — each runs inside a caller-provided tx.

	UpsertCategory(ctx context.Context, tx DB, userID int64, e CategoryExport) (id int64, created bool, err error)
	UpsertTag(ctx context.Context, tx DB, userID int64, e TagExport) (id int64, created bool, err error)
	InsertBookmark(ctx context.Context, tx DB, userID int64, e BookmarkExport, categoryID int64, tagIDs []int64) error
	InsertReadLaterItem(ctx context.Context, tx DB, userID int64, e ReadLaterItemExport, tagIDs []int64) error

	// UpdateBookmarkByURL updates an existing bookmark (matched by normalized
	// URL) with the non-zero fields from e. Returns false if no match.
	UpdateBookmarkByURL(ctx context.Context, tx DB, userID int64, normalizedURL string, e BookmarkExport, categoryID int64, tagIDs []int64) (bool, error)
	// UpdateReadLaterByURL updates an existing read-later item (matched by
	// normalized URL) with the non-zero fields from e. Returns false if no match.
	UpdateReadLaterByURL(ctx context.Context, tx DB, userID int64, normalizedURL string, e ReadLaterItemExport, tagIDs []int64) (bool, error)

	// FindCategoryID returns the category id for a given name, or 0 if not found.
	FindCategoryID(ctx context.Context, tx DB, userID int64, name string) (int64, error)
	// FindTagID returns the tag id for a given name, or 0 if not found.
	FindTagID(ctx context.Context, tx DB, userID int64, name string) (int64, error)
}

// DB is the minimal transaction/db interface needed by Store methods.
// Both *sql.DB and *sql.Tx satisfy it.
type DB interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}
