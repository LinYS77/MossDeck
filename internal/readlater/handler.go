package readlater

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

const maxBodyBytes = 64 * 1024

func Register(mux *http.ServeMux, svc *Service) {
	require := func(h http.HandlerFunc) http.Handler {
		return auth.RequireAuth(svc.auth)(h)
	}
	mux.Handle("GET /api/v1/read-later", require(svc.handleList()))
	mux.Handle("POST /api/v1/read-later", require(svc.handleCreate()))
	mux.Handle("GET /api/v1/read-later/{id}", require(svc.handleGet()))
	mux.Handle("PATCH /api/v1/read-later/{id}", require(svc.handleUpdate()))
	mux.Handle("DELETE /api/v1/read-later/{id}", require(svc.handleDelete()))
	mux.Handle("POST /api/v1/read-later/{id}/open", require(svc.handleOpen()))
	mux.Handle("POST /api/v1/read-later/{id}/archive", require(svc.handleArchive()))
	mux.Handle("POST /api/v1/read-later/{id}/restore", require(svc.handleRestore()))
	mux.Handle("DELETE /api/v1/read-later/{id}/purge", require(svc.handlePurge()))
}

type tagDTO struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Color     string `json:"color,omitempty"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

func toTagDTO(t Tag) tagDTO {
	return tagDTO{ID: t.ID, Name: t.Name, Color: t.Color, CreatedAt: toRFC3339(t.CreatedAt), UpdatedAt: toRFC3339(t.UpdatedAt)}
}

type itemDTO struct {
	ID                 int64    `json:"id"`
	URL                string   `json:"url"`
	Title              string   `json:"title"`
	Excerpt            string   `json:"excerpt,omitempty"`
	Author             string   `json:"author,omitempty"`
	SiteName           string   `json:"siteName,omitempty"`
	FaviconURL         string   `json:"faviconUrl,omitempty"`
	CoverImageURL      string   `json:"coverImageUrl,omitempty"`
	Domain             string   `json:"domain"`
	ReadingTimeMinutes int      `json:"readingTimeMinutes"`
	State              string   `json:"state"`
	Priority           int      `json:"priority"`
	Favorite           bool     `json:"favorite"`
	Source             string   `json:"source,omitempty"`
	LastOpenedAt       string   `json:"lastOpenedAt,omitempty"`
	ArchivedAt         string   `json:"archivedAt,omitempty"`
	MetadataStatus     string   `json:"metadataStatus"`
	Tags               []tagDTO `json:"tags,omitempty"`
	CreatedAt          string   `json:"createdAt"`
	UpdatedAt          string   `json:"updatedAt"`
	DeletedAt          string   `json:"deletedAt,omitempty"`
}

func toItemDTO(item *Item) itemDTO {
	tags := make([]tagDTO, 0, len(item.Tags))
	for _, t := range item.Tags {
		tags = append(tags, toTagDTO(t))
	}
	return itemDTO{
		ID: item.ID, URL: item.URL, Title: item.Title, Excerpt: item.Excerpt,
		Author: item.Author, SiteName: item.SiteName, FaviconURL: item.FaviconURL,
		CoverImageURL: item.CoverImageURL, Domain: item.Domain,
		ReadingTimeMinutes: item.ReadingTimeMinutes, State: item.State,
		Priority: item.Priority, Favorite: item.Favorite, Source: item.Source,
		LastOpenedAt: toRFC3339(item.LastOpenedAt), ArchivedAt: toRFC3339(item.ArchivedAt),
		MetadataStatus: item.MetadataStatus, Tags: tags,
		CreatedAt: toRFC3339(item.CreatedAt), UpdatedAt: toRFC3339(item.UpdatedAt), DeletedAt: toRFC3339(item.DeletedAt),
	}
}

type listDTO struct {
	Items    []itemDTO `json:"items"`
	Page     int       `json:"page"`
	PageSize int       `json:"pageSize"`
	Total    int       `json:"total"`
}

func (s *Service) handleList() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := ListQuery{UserID: currentUserID(r)}
		q.Q = strings.TrimSpace(r.URL.Query().Get("q"))
		q.State = strings.TrimSpace(r.URL.Query().Get("state"))
		q.Tag = strings.TrimSpace(r.URL.Query().Get("tag"))
		q.Domain = strings.TrimSpace(r.URL.Query().Get("domain"))
		q.TagID = queryInt64(r, "tagId")
		q.Page = queryInt(r, "page")
		q.PageSize = queryInt(r, "pageSize")
		q.Sort = strings.TrimSpace(r.URL.Query().Get("sort"))
		q.Favorite = queryBool(r, "favorite")
		if priority := r.URL.Query().Get("priority"); priority != "" {
			v, err := strconv.Atoi(priority)
			if err != nil {
				api.WriteError(w, r, api.BadRequest("invalid priority", err))
				return
			}
			q.Priority = &v
		}
		res, err := s.List(r.Context(), q)
		if err != nil {
			s.writeErr(w, r, err)
			return
		}
		items := make([]itemDTO, 0, len(res.Items))
		for i := range res.Items {
			items = append(items, toItemDTO(&res.Items[i]))
		}
		api.WriteOK(w, r, listDTO{Items: items, Page: res.Page, PageSize: res.PageSize, Total: res.Total})
	}
}

func (s *Service) handleCreate() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createRequest
		if err := decodeJSON(r, &req); err != nil {
			api.WriteError(w, r, api.BadRequest("invalid request body", err))
			return
		}
		item, err := s.Create(r.Context(), currentUserID(r), req.toParams())
		if err != nil {
			s.writeErr(w, r, err)
			return
		}
		api.WriteCreated(w, r, toItemDTO(item))
	}
}

func (s *Service) handleGet() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseID(w, r, "id")
		if !ok {
			return
		}
		item, err := s.store.GetItem(r.Context(), currentUserID(r), id)
		if err != nil {
			s.writeErr(w, r, err)
			return
		}
		api.WriteOK(w, r, toItemDTO(item))
	}
}

func (s *Service) handleUpdate() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseID(w, r, "id")
		if !ok {
			return
		}
		var req updateRequest
		if err := decodeJSON(r, &req); err != nil {
			api.WriteError(w, r, api.BadRequest("invalid request body", err))
			return
		}
		item, err := s.Update(r.Context(), currentUserID(r), id, req.toParams())
		if err != nil {
			s.writeErr(w, r, err)
			return
		}
		api.WriteOK(w, r, toItemDTO(item))
	}
}

func (s *Service) handleDelete() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseID(w, r, "id")
		if !ok {
			return
		}
		item, err := s.Trash(r.Context(), currentUserID(r), id)
		if err != nil {
			s.writeErr(w, r, err)
			return
		}
		api.WriteOK(w, r, toItemDTO(item))
	}
}

func (s *Service) handleOpen() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseID(w, r, "id")
		if !ok {
			return
		}
		item, err := s.Open(r.Context(), currentUserID(r), id)
		if err != nil {
			s.writeErr(w, r, err)
			return
		}
		api.WriteOK(w, r, toItemDTO(item))
	}
}

func (s *Service) handleArchive() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseID(w, r, "id")
		if !ok {
			return
		}
		item, err := s.Archive(r.Context(), currentUserID(r), id)
		if err != nil {
			s.writeErr(w, r, err)
			return
		}
		api.WriteOK(w, r, toItemDTO(item))
	}
}

func (s *Service) handleRestore() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseID(w, r, "id")
		if !ok {
			return
		}
		item, err := s.Restore(r.Context(), currentUserID(r), id)
		if err != nil {
			s.writeErr(w, r, err)
			return
		}
		api.WriteOK(w, r, toItemDTO(item))
	}
}

func (s *Service) handlePurge() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseID(w, r, "id")
		if !ok {
			return
		}
		if err := s.Purge(r.Context(), currentUserID(r), id); err != nil {
			s.writeErr(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Service) writeErr(w http.ResponseWriter, r *http.Request, err error) {
	var ve ValidationError
	switch {
	case errors.As(err, &ve):
		api.WriteError(w, r, api.BadRequest(ve.Error(), nil))
	case errors.Is(err, ErrURLTaken):
		api.WriteError(w, r, api.Conflict("read-later url already exists"))
	case errors.Is(err, ErrNotFound):
		api.WriteError(w, r, api.NotFound("read-later item not found"))
	case errors.Is(err, ErrInvalidState):
		api.WriteError(w, r, api.BadRequest("invalid state", nil))
	default:
		s.logger.Error("read-later operation", "request_id", reqid.From(r.Context()), "error", err)
		api.WriteError(w, r, api.Internal("internal server error", err))
	}
}

type createRequest struct {
	URL                string  `json:"url"`
	Title              string  `json:"title"`
	Excerpt            string  `json:"excerpt"`
	Author             string  `json:"author"`
	SiteName           string  `json:"siteName"`
	FaviconURL         string  `json:"faviconUrl"`
	CoverImageURL      string  `json:"coverImageUrl"`
	ReadingTimeMinutes int     `json:"readingTimeMinutes"`
	Priority           int     `json:"priority"`
	Favorite           bool    `json:"favorite"`
	Source             string  `json:"source"`
	TagIDs             []int64 `json:"tagIds"`
}

func (r createRequest) toParams() CreateParams {
	return CreateParams{
		URL: r.URL, Title: r.Title, Excerpt: r.Excerpt, Author: r.Author, SiteName: r.SiteName,
		FaviconURL: r.FaviconURL, CoverImageURL: r.CoverImageURL,
		ReadingTimeMinutes: r.ReadingTimeMinutes, Priority: r.Priority,
		Favorite: r.Favorite, Source: r.Source, TagIDs: r.TagIDs,
	}
}

type updateRequest struct {
	URL                *string  `json:"url,omitempty"`
	Title              *string  `json:"title,omitempty"`
	Excerpt            *string  `json:"excerpt,omitempty"`
	Author             *string  `json:"author,omitempty"`
	SiteName           *string  `json:"siteName,omitempty"`
	FaviconURL         *string  `json:"faviconUrl,omitempty"`
	CoverImageURL      *string  `json:"coverImageUrl,omitempty"`
	ReadingTimeMinutes *int     `json:"readingTimeMinutes,omitempty"`
	State              *string  `json:"state,omitempty"`
	Priority           *int     `json:"priority,omitempty"`
	Favorite           *bool    `json:"favorite,omitempty"`
	Source             *string  `json:"source,omitempty"`
	TagIDs             *[]int64 `json:"tagIds,omitempty"`
}

func (r updateRequest) toParams() UpdateParams {
	return UpdateParams{
		URL: r.URL, Title: r.Title, Excerpt: r.Excerpt, Author: r.Author, SiteName: r.SiteName,
		FaviconURL: r.FaviconURL, CoverImageURL: r.CoverImageURL,
		ReadingTimeMinutes: r.ReadingTimeMinutes, State: r.State, Priority: r.Priority,
		Favorite: r.Favorite, Source: r.Source, TagIDs: r.TagIDs,
	}
}

func currentUserID(r *http.Request) int64 {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		return 0
	}
	return u.ID
}

func parseID(w http.ResponseWriter, r *http.Request, key string) (int64, bool) {
	raw := r.PathValue(key)
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		api.WriteError(w, r, api.BadRequest("invalid "+key, err))
		return 0, false
	}
	return id, true
}

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
