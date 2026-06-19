package bookmark

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/linyusheng/homepage/internal/api"
	"github.com/linyusheng/homepage/internal/auth"
	"github.com/linyusheng/homepage/internal/reqid"
)

// maxBodyBytes caps request bodies for bookmark endpoints to keep parsing
// bounded. Bookmark payloads are small (text fields + small tag arrays).
const maxBodyBytes = 64 * 1024

// Register mounts every bookmark endpoint onto mux, each wrapped in
// auth.RequireAuth so all access requires a valid session. The authenticated
// user (auth.UserFromContext) provides the user_id isolation key.
func Register(mux *http.ServeMux, svc *Service) {
	require := func(h http.HandlerFunc) http.Handler {
		return auth.RequireAuth(svc.auth)(h)
	}

	// --- categories ---
	mux.Handle("GET /api/v1/categories", require(svc.handleListCategories()))
	mux.Handle("POST /api/v1/categories", require(svc.handleCreateCategory()))
	mux.Handle("GET /api/v1/categories/{id}", require(svc.handleGetCategory()))
	mux.Handle("PATCH /api/v1/categories/{id}", require(svc.handleUpdateCategory()))
	mux.Handle("DELETE /api/v1/categories/{id}", require(svc.handleDeleteCategory()))

	// --- tags ---
	mux.Handle("GET /api/v1/tags", require(svc.handleListTags()))
	mux.Handle("POST /api/v1/tags", require(svc.handleCreateTag()))
	mux.Handle("GET /api/v1/tags/{id}", require(svc.handleGetTag()))
	mux.Handle("PATCH /api/v1/tags/{id}", require(svc.handleUpdateTag()))
	mux.Handle("DELETE /api/v1/tags/{id}", require(svc.handleDeleteTag()))

	// --- bookmarks ---
	mux.Handle("GET /api/v1/bookmarks", require(svc.handleListBookmarks()))
	mux.Handle("POST /api/v1/bookmarks", require(svc.handleCreateBookmark()))
	mux.Handle("GET /api/v1/bookmarks/{id}", require(svc.handleGetBookmark()))
	mux.Handle("PATCH /api/v1/bookmarks/{id}", require(svc.handleUpdateBookmark()))
	mux.Handle("DELETE /api/v1/bookmarks/{id}", require(svc.handleDeleteBookmark()))
	mux.Handle("POST /api/v1/bookmarks/{id}/restore", require(svc.handleRestoreBookmark()))
	mux.Handle("POST /api/v1/bookmarks/{id}/archive", require(svc.handleArchiveBookmark()))
	mux.Handle("POST /api/v1/bookmarks/{id}/open", require(svc.handleOpenBookmark()))

	// --- bookmark import (Netscape HTML) ---
	mux.Handle("POST /api/v1/bookmarks/import/html", require(svc.handleImportHTML()))
}

// =====================================================================
// DTOs
// =====================================================================

type categoryDTO struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	Slug          string `json:"slug,omitempty"`
	Type          string `json:"type"`
	Icon          string `json:"icon,omitempty"`
	Color         string `json:"color,omitempty"`
	ParentID      int64  `json:"parentId,omitempty"`
	SortOrder     int    `json:"sortOrder"`
	Archived      bool   `json:"archived"`
	ShowOnHome    bool   `json:"showOnHome"`
	BookmarkCount int    `json:"bookmarkCount"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

func toCategoryDTO(c *Category) categoryDTO {
	return categoryDTO{
		ID: c.ID, Name: c.Name, Slug: c.Slug, Type: c.Type,
		Icon: c.Icon, Color: c.Color, ParentID: c.ParentID, SortOrder: c.SortOrder,
		Archived: c.Archived, ShowOnHome: c.ShowOnHome, BookmarkCount: c.BookmarkCount,
		CreatedAt: toRFC3339(c.CreatedAt), UpdatedAt: toRFC3339(c.UpdatedAt),
	}
}

type tagDTO struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	Color         string `json:"color,omitempty"`
	BookmarkCount int    `json:"bookmarkCount"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

func toTagDTO(t *Tag) tagDTO {
	return tagDTO{
		ID: t.ID, Name: t.Name, Color: t.Color, BookmarkCount: t.BookmarkCount,
		CreatedAt: toRFC3339(t.CreatedAt), UpdatedAt: toRFC3339(t.UpdatedAt),
	}
}

type bookmarkDTO struct {
	ID           int64    `json:"id"`
	URL          string   `json:"url"`
	Title        string   `json:"title"`
	Description  string   `json:"description,omitempty"`
	Domain       string   `json:"domain"`
	CategoryID   int64    `json:"categoryId,omitempty"`
	Tags         []tagDTO `json:"tags,omitempty"`
	Pinned       bool     `json:"pinned"`
	Favorite     bool     `json:"favorite"`
	SortOrder    int      `json:"sortOrder"`
	ClickCount   int      `json:"clickCount"`
	Status       string   `json:"status"`
	LastOpenedAt string   `json:"lastOpenedAt,omitempty"`
	CreatedAt    string   `json:"createdAt"`
	UpdatedAt    string   `json:"updatedAt"`
}

func toBookmarkDTO(b *Bookmark) bookmarkDTO {
	tags := make([]tagDTO, 0, len(b.Tags))
	for _, t := range b.Tags {
		tags = append(tags, toTagDTO(&t))
	}
	return bookmarkDTO{
		ID: b.ID, URL: b.URL, Title: b.Title, Description: b.Description,
		Domain: b.Domain, CategoryID: b.CategoryID, Tags: tags,
		Pinned: b.Pinned, Favorite: b.Favorite, SortOrder: b.SortOrder,
		ClickCount: b.ClickCount, Status: b.Status,
		LastOpenedAt: toRFC3339(b.LastOpenedAt),
		CreatedAt:    toRFC3339(b.CreatedAt), UpdatedAt: toRFC3339(b.UpdatedAt),
	}
}

// =====================================================================
// Categories handlers
// =====================================================================

func (s *Service) handleListCategories() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := currentUserID(r)
		cats, err := s.store.ListCategories(r.Context(), userID)
		if err != nil {
			s.writeInternal(w, r, err, "list categories")
			return
		}
		out := make([]categoryDTO, 0, len(cats))
		for _, c := range cats {
			out = append(out, toCategoryDTO(c))
		}
		api.WriteOK(w, r, out)
	}
}

func (s *Service) handleGetCategory() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseID(w, r, "id")
		if !ok {
			return
		}
		c, err := s.store.GetCategory(r.Context(), currentUserID(r), id)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				api.WriteError(w, r, api.NotFound("category not found"))
				return
			}
			s.writeInternal(w, r, err, "get category")
			return
		}
		api.WriteOK(w, r, toCategoryDTO(c))
	}
}

func (s *Service) handleCreateCategory() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name      string `json:"name"`
			Slug      string `json:"slug"`
			Type      string `json:"type"`
			Icon      string `json:"icon"`
			Color     string `json:"color"`
			ParentID  int64  `json:"parentId"`
			SortOrder int    `json:"sortOrder"`
		}
		if err := decodeJSON(r, &req); err != nil {
			api.WriteError(w, r, api.BadRequest("invalid request body", err))
			return
		}
		cat, err := s.CreateCategory(r.Context(), currentUserID(r), CreateCategoryParams{
			Name: req.Name, Slug: req.Slug, Type: req.Type, Icon: req.Icon,
			Color: req.Color, ParentID: req.ParentID, SortOrder: req.SortOrder,
		})
		if err != nil {
			s.writeCategoryErr(w, r, err)
			return
		}
		api.WriteCreated(w, r, toCategoryDTO(cat))
	}
}

func (s *Service) handleUpdateCategory() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseID(w, r, "id")
		if !ok {
			return
		}
		var req updateCategoryRequest
		if err := decodeJSON(r, &req); err != nil {
			api.WriteError(w, r, api.BadRequest("invalid request body", err))
			return
		}
		cat, err := s.UpdateCategory(r.Context(), currentUserID(r), id, req.toParams())
		if err != nil {
			s.writeCategoryErr(w, r, err)
			return
		}
		api.WriteOK(w, r, toCategoryDTO(cat))
	}
}

func (s *Service) handleDeleteCategory() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseID(w, r, "id")
		if !ok {
			return
		}
		if err := s.DeleteCategory(r.Context(), currentUserID(r), id); err != nil {
			if errors.Is(err, ErrNotFound) {
				api.WriteError(w, r, api.NotFound("category not found"))
				return
			}
			s.writeInternal(w, r, err, "delete category")
			return
		}
		api.WriteOK(w, r, map[string]any{"deleted": true})
	}
}

// =====================================================================
// Tags handlers
// =====================================================================

func (s *Service) handleListTags() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := currentUserID(r)
		tags, err := s.store.ListTags(r.Context(), userID)
		if err != nil {
			s.writeInternal(w, r, err, "list tags")
			return
		}
		out := make([]tagDTO, 0, len(tags))
		for _, t := range tags {
			out = append(out, toTagDTO(t))
		}
		api.WriteOK(w, r, out)
	}
}

func (s *Service) handleGetTag() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseID(w, r, "id")
		if !ok {
			return
		}
		t, err := s.store.GetTag(r.Context(), currentUserID(r), id)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				api.WriteError(w, r, api.NotFound("tag not found"))
				return
			}
			s.writeInternal(w, r, err, "get tag")
			return
		}
		api.WriteOK(w, r, toTagDTO(t))
	}
}

func (s *Service) handleCreateTag() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name  string `json:"name"`
			Color string `json:"color"`
		}
		if err := decodeJSON(r, &req); err != nil {
			api.WriteError(w, r, api.BadRequest("invalid request body", err))
			return
		}
		t, err := s.CreateTag(r.Context(), currentUserID(r), CreateTagParams{Name: req.Name, Color: req.Color})
		if err != nil {
			s.writeTagErr(w, r, err)
			return
		}
		api.WriteCreated(w, r, toTagDTO(t))
	}
}

func (s *Service) handleUpdateTag() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseID(w, r, "id")
		if !ok {
			return
		}
		var req updateTagRequest
		if err := decodeJSON(r, &req); err != nil {
			api.WriteError(w, r, api.BadRequest("invalid request body", err))
			return
		}
		t, err := s.UpdateTag(r.Context(), currentUserID(r), id, req.toParams())
		if err != nil {
			s.writeTagErr(w, r, err)
			return
		}
		api.WriteOK(w, r, toTagDTO(t))
	}
}

func (s *Service) handleDeleteTag() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseID(w, r, "id")
		if !ok {
			return
		}
		if err := s.store.DeleteTag(r.Context(), currentUserID(r), id); err != nil {
			if errors.Is(err, ErrNotFound) {
				api.WriteError(w, r, api.NotFound("tag not found"))
				return
			}
			s.writeInternal(w, r, err, "delete tag")
			return
		}
		api.WriteOK(w, r, map[string]any{"deleted": true})
	}
}

// =====================================================================
// Bookmarks handlers
// =====================================================================

func (s *Service) handleListBookmarks() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := ListQuery{UserID: currentUserID(r)}
		q.Q = strings.TrimSpace(r.URL.Query().Get("q"))
		q.Status = r.URL.Query().Get("status")
		q.Domain = strings.TrimSpace(r.URL.Query().Get("domain"))
		q.Tag = strings.TrimSpace(r.URL.Query().Get("tag"))
		q.CategoryID = queryInt64(r, "categoryId")
		q.TagID = queryInt64(r, "tagId")
		q.Page = queryInt(r, "page")
		q.PageSize = queryInt(r, "pageSize")
		q.Sort = r.URL.Query().Get("sort")
		if v := queryBool(r, "favorite"); v != nil {
			q.Favorite = v
		}
		if v := queryBool(r, "pinned"); v != nil {
			q.Pinned = v
		}

		res, err := s.List(r.Context(), q)
		if err != nil {
			s.writeInternal(w, r, err, "list bookmarks")
			return
		}
		items := make([]bookmarkDTO, 0, len(res.Items))
		for i := range res.Items {
			items = append(items, toBookmarkDTO(&res.Items[i]))
		}
		api.WriteOK(w, r, ListResultDTO{
			Items: items, Page: res.Page, PageSize: res.PageSize, Total: res.Total,
		})
	}
}

func (s *Service) handleCreateBookmark() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createBookmarkRequest
		if err := decodeJSON(r, &req); err != nil {
			api.WriteError(w, r, api.BadRequest("invalid request body", err))
			return
		}
		b, err := s.CreateBookmark(r.Context(), currentUserID(r), CreateBookmarkParams{
			URL: req.URL, Title: req.Title, Description: req.Description,
			CategoryID: req.CategoryID, TagIDs: req.TagIDs,
			Pinned: req.Pinned, Favorite: req.Favorite, SortOrder: req.SortOrder,
		})
		if err != nil {
			s.writeBookmarkErr(w, r, err)
			return
		}
		api.WriteCreated(w, r, toBookmarkDTO(b))
	}
}

func (s *Service) handleGetBookmark() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseID(w, r, "id")
		if !ok {
			return
		}
		b, err := s.store.GetBookmark(r.Context(), currentUserID(r), id)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				api.WriteError(w, r, api.NotFound("bookmark not found"))
				return
			}
			s.writeInternal(w, r, err, "get bookmark")
			return
		}
		api.WriteOK(w, r, toBookmarkDTO(b))
	}
}

func (s *Service) handleUpdateBookmark() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseID(w, r, "id")
		if !ok {
			return
		}
		var req updateBookmarkRequest
		if err := decodeJSON(r, &req); err != nil {
			api.WriteError(w, r, api.BadRequest("invalid request body", err))
			return
		}
		b, err := s.UpdateBookmark(r.Context(), currentUserID(r), id, req.toParams())
		if err != nil {
			s.writeBookmarkErr(w, r, err)
			return
		}
		api.WriteOK(w, r, toBookmarkDTO(b))
	}
}

// handleDeleteBookmark soft-deletes (trashes) a bookmark. Physical deletion is
// intentionally not offered; use restore to bring it back.
func (s *Service) handleDeleteBookmark() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseID(w, r, "id")
		if !ok {
			return
		}
		b, err := s.Trash(r.Context(), currentUserID(r), id)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				api.WriteError(w, r, api.NotFound("bookmark not found"))
				return
			}
			s.writeInternal(w, r, err, "delete bookmark")
			return
		}
		api.WriteOK(w, r, toBookmarkDTO(b))
	}
}

func (s *Service) handleRestoreBookmark() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseID(w, r, "id")
		if !ok {
			return
		}
		b, err := s.Restore(r.Context(), currentUserID(r), id)
		if err != nil {
			// Restore can collide on the partial unique index when another
			// non-trash row already holds the same normalized_url; that maps to
			// 409 (ErrBookmarkURLTaken) via writeBookmarkErr, not 500.
			s.writeBookmarkErr(w, r, err)
			return
		}
		api.WriteOK(w, r, toBookmarkDTO(b))
	}
}

func (s *Service) handleArchiveBookmark() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseID(w, r, "id")
		if !ok {
			return
		}
		b, err := s.Archive(r.Context(), currentUserID(r), id)
		if err != nil {
			s.writeBookmarkErr(w, r, err)
			return
		}
		api.WriteOK(w, r, toBookmarkDTO(b))
	}
}

func (s *Service) handleOpenBookmark() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseID(w, r, "id")
		if !ok {
			return
		}
		if err := s.Open(r.Context(), currentUserID(r), id); err != nil {
			if errors.Is(err, ErrNotFound) {
				api.WriteError(w, r, api.NotFound("bookmark not found"))
				return
			}
			s.writeInternal(w, r, err, "open bookmark")
			return
		}
		api.WriteOK(w, r, map[string]any{"opened": true})
	}
}

// =====================================================================
// Error mapping
// =====================================================================

func (s *Service) writeCategoryErr(w http.ResponseWriter, r *http.Request, err error) {
	var ve ValidationError
	switch {
	case errors.As(err, &ve):
		api.WriteError(w, r, api.BadRequest(ve.Error(), nil))
	case errors.Is(err, ErrCategoryNameTaken):
		api.WriteError(w, r, api.Conflict("category name already taken"))
	case errors.Is(err, ErrNotFound):
		api.WriteError(w, r, api.NotFound("category not found"))
	default:
		s.writeInternal(w, r, err, "category operation")
	}
}

func (s *Service) writeTagErr(w http.ResponseWriter, r *http.Request, err error) {
	var ve ValidationError
	switch {
	case errors.As(err, &ve):
		api.WriteError(w, r, api.BadRequest(ve.Error(), nil))
	case errors.Is(err, ErrTagNameTaken):
		api.WriteError(w, r, api.Conflict("tag name already taken"))
	case errors.Is(err, ErrNotFound):
		api.WriteError(w, r, api.NotFound("tag not found"))
	default:
		s.writeInternal(w, r, err, "tag operation")
	}
}

func (s *Service) writeBookmarkErr(w http.ResponseWriter, r *http.Request, err error) {
	var ve ValidationError
	switch {
	case errors.As(err, &ve):
		api.WriteError(w, r, api.BadRequest(ve.Error(), nil))
	case errors.Is(err, ErrBookmarkURLTaken):
		api.WriteError(w, r, api.Conflict("bookmark url already exists"))
	case errors.Is(err, ErrNotFound):
		api.WriteError(w, r, api.NotFound("bookmark not found"))
	case errors.Is(err, ErrInvalidStatus):
		api.WriteError(w, r, api.BadRequest("invalid status", nil))
	default:
		s.writeInternal(w, r, err, "bookmark operation")
	}
}

func (s *Service) writeInternal(w http.ResponseWriter, r *http.Request, err error, op string) {
	s.logger.Error(op, "request_id", reqid.From(r.Context()), "error", err)
	api.WriteError(w, r, api.Internal("internal server error", err))
}

// =====================================================================
// Request types (with optional-pointer toParams)
// =====================================================================

type updateCategoryRequest struct {
	Name       *string `json:"name,omitempty"`
	Slug       *string `json:"slug,omitempty"`
	Type       *string `json:"type,omitempty"`
	Icon       *string `json:"icon,omitempty"`
	Color      *string `json:"color,omitempty"`
	ParentID   *int64  `json:"parentId,omitempty"`
	SortOrder  *int    `json:"sortOrder,omitempty"`
	Archived   *bool   `json:"archived,omitempty"`
	ShowOnHome *bool   `json:"showOnHome,omitempty"`
}

func (r updateCategoryRequest) toParams() UpdateCategoryParams {
	return UpdateCategoryParams{
		Name: r.Name, Slug: r.Slug, Type: r.Type, Icon: r.Icon, Color: r.Color,
		ParentID: r.ParentID, SortOrder: r.SortOrder, Archived: r.Archived,
		ShowOnHome: r.ShowOnHome,
	}
}

type updateTagRequest struct {
	Name  *string `json:"name,omitempty"`
	Color *string `json:"color,omitempty"`
}

func (r updateTagRequest) toParams() UpdateTagParams {
	return UpdateTagParams{Name: r.Name, Color: r.Color}
}

type createBookmarkRequest struct {
	URL         string  `json:"url"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	CategoryID  int64   `json:"categoryId"`
	TagIDs      []int64 `json:"tagIds"`
	Pinned      bool    `json:"pinned"`
	Favorite    bool    `json:"favorite"`
	SortOrder   int     `json:"sortOrder"`
}

type updateBookmarkRequest struct {
	URL         *string  `json:"url,omitempty"`
	Title       *string  `json:"title,omitempty"`
	Description *string  `json:"description,omitempty"`
	CategoryID  *int64   `json:"categoryId,omitempty"`
	TagIDs      *[]int64 `json:"tagIds,omitempty"`
	Pinned      *bool    `json:"pinned,omitempty"`
	Favorite    *bool    `json:"favorite,omitempty"`
	SortOrder   *int     `json:"sortOrder,omitempty"`
	Status      *string  `json:"status,omitempty"`
}

func (r updateBookmarkRequest) toParams() UpdateBookmarkParams {
	return UpdateBookmarkParams{
		URL: r.URL, Title: r.Title, Description: r.Description,
		CategoryID: r.CategoryID, TagIDs: r.TagIDs, Pinned: r.Pinned,
		Favorite: r.Favorite, SortOrder: r.SortOrder, Status: r.Status,
	}
}

// ListResultDTO is the JSON shape of a bookmark list page.
type ListResultDTO struct {
	Items    []bookmarkDTO `json:"items"`
	Page     int           `json:"page"`
	PageSize int           `json:"pageSize"`
	Total    int           `json:"total"`
}

// =====================================================================
// HTTP helpers
// =====================================================================

// currentUserID returns the authenticated user's id from context. It panics
// only if Register's wiring was bypassed; RequireAuth guarantees presence.
func currentUserID(r *http.Request) int64 {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		// RequireAuth should have already rejected the request. Treat the
		// impossible case as "no user" (id 0), which no row matches.
		return 0
	}
	return u.ID
}

// parseID extracts and validates an int64 path parameter. On error it writes a
// 400 and returns ok=false so the caller can return early.
func parseID(w http.ResponseWriter, r *http.Request, key string) (int64, bool) {
	raw := r.PathValue(key)
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		api.WriteError(w, r, api.BadRequest("invalid "+key, err))
		return 0, false
	}
	return id, true
}

// queryInt64 parses an int64 query parameter; 0 means unset/invalid.
func queryInt64(r *http.Request, key string) int64 {
	v := r.URL.Query().Get(key)
	if v == "" {
		return 0
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

// queryInt parses an int query parameter; 0 means unset.
func queryInt(r *http.Request, key string) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0
	}
	return n
}

// queryBool parses a bool query parameter ("1"/"true"/etc.); nil means unset.
func queryBool(r *http.Request, key string) *bool {
	v := r.URL.Query().Get(key)
	if v == "" {
		return nil
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		b := true
		return &b
	case "0", "false", "no", "off":
		b := false
		return &b
	}
	return nil
}

// decodeJSON reads a bounded JSON body into dst, rejecting unknown fields and
// trailing content for clear error feedback.
func decodeJSON(r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if dec.More() {
		return errors.New("unexpected trailing content in request body")
	}
	return nil
}

// toRFC3339 converts a SQLite datetime('now')-style timestamp into RFC3339 for
// API output. Values that do not parse (e.g. empty) are returned unchanged.
func toRFC3339(s string) string {
	if s == "" {
		return ""
	}
	t, err := parseDBTime(s)
	if err != nil {
		return s
	}
	return t.UTC().Format(rfc3339)
}
