package readlater

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/linyusheng/homepage/internal/auth"
	"github.com/linyusheng/homepage/internal/bookmark"
	"github.com/linyusheng/homepage/internal/csrf"
	"github.com/linyusheng/homepage/internal/db"
	"github.com/linyusheng/homepage/internal/httpx"
	"github.com/linyusheng/homepage/internal/ratelimit"
)

type testEnv struct {
	mux        *http.ServeMux
	handler    http.Handler
	db         *sql.DB
	svc        *Service
	cookie     *http.Cookie
	csrfCookie *http.Cookie
	userID     int64
	logger     *slog.Logger
	authSvc    *auth.Service
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	database, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := db.Migrate(context.Background(), database); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	resolver, _ := httpx.NewClientIPResolver(nil)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	limiter := ratelimit.New(ctx, 5, time.Minute)
	authSvc := auth.NewService(auth.Config{
		SessionTTL: time.Hour, BcryptCost: 4, CookieName: "homepage_session",
		CookieSecure: false, CookieSameSite: "lax", SetupEnabled: true,
	}, auth.NewStore(database), limiter, resolver, logger)
	csrfManager := csrf.New(csrf.Config{
		CookieName:        "homepage_csrf",
		HeaderName:        "X-CSRF-Token",
		SessionCookieName: "homepage_session",
		CookieSecure:      false,
		CookieSameSite:    "lax",
		TTL:               time.Hour,
	})
	authSvc.SetCSRFManager(csrfManager)

	mux := http.NewServeMux()
	auth.Register(mux, authSvc)
	mux.Handle("GET /api/v1/auth/csrf", csrfManager.HandleToken())
	bmSvc := bookmark.NewService(bookmark.NewStore(database), authSvc, logger)
	bookmark.Register(mux, bmSvc)
	svc := NewService(NewStore(database), authSvc, logger)
	Register(mux, svc)
	return &testEnv{mux: mux, handler: csrfManager.Middleware(mux), db: database, svc: svc, logger: logger, authSvc: authSvc}
}

func (e *testEnv) loginAdmin(t *testing.T) {
	t.Helper()
	rec := e.post(t, "/api/v1/auth/setup", map[string]string{"username": "admin", "password": "supersecret"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup: %d %s", rec.Code, rec.Body.String())
	}
	rec = e.post(t, "/api/v1/auth/login", map[string]string{"username": "admin", "password": "supersecret"})
	if rec.Code != http.StatusOK {
		t.Fatalf("login: %d %s", rec.Code, rec.Body.String())
	}
	for _, c := range rec.Result().Cookies() {
		switch c.Name {
		case "homepage_session":
			e.cookie = c
		case "homepage_csrf":
			e.csrfCookie = c
		}
	}
	if e.cookie == nil {
		t.Fatal("missing session cookie")
	}
	if e.csrfCookie == nil || e.csrfCookie.Value == "" {
		t.Fatal("missing csrf cookie")
	}
	e.userID = 1
}

func (e *testEnv) send(t *testing.T, method, path string, body any, authed bool) *httptest.ResponseRecorder {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		r = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if authed && e.cookie != nil {
		req.AddCookie(e.cookie)
		if e.csrfCookie != nil {
			req.AddCookie(e.csrfCookie)
			if method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions {
				req.Header.Set("X-CSRF-Token", e.csrfCookie.Value)
			}
		}
	}
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	return rec
}

func (e *testEnv) post(t *testing.T, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	return e.send(t, http.MethodPost, path, body, true)
}

func (e *testEnv) get(t *testing.T, path string) *httptest.ResponseRecorder {
	t.Helper()
	return e.send(t, http.MethodGet, path, nil, true)
}

func (e *testEnv) patch(t *testing.T, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	return e.send(t, http.MethodPatch, path, body, true)
}

func (e *testEnv) del(t *testing.T, path string) *httptest.ResponseRecorder {
	t.Helper()
	return e.send(t, http.MethodDelete, path, nil, true)
}

type envelope struct {
	Data  json.RawMessage `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	RequestID string `json:"requestId"`
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) envelope {
	t.Helper()
	var env envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return env
}

func data[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	env := decode(t, rec)
	if env.Error != nil {
		t.Fatalf("unexpected error: %+v", env.Error)
	}
	var out T
	if err := json.Unmarshal(env.Data, &out); err != nil {
		t.Fatalf("decode data %q: %v", string(env.Data), err)
	}
	return out
}

func createItem(t *testing.T, e *testEnv, url string, extras map[string]any) itemDTO {
	t.Helper()
	body := map[string]any{"url": url}
	for k, v := range extras {
		body[k] = v
	}
	rec := e.post(t, "/api/v1/read-later", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create %s: %d %s", url, rec.Code, rec.Body.String())
	}
	return data[itemDTO](t, rec)
}

func createTag(t *testing.T, e *testEnv, name string) int64 {
	t.Helper()
	rec := e.post(t, "/api/v1/tags", map[string]string{"name": name})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create tag: %d %s", rec.Code, rec.Body.String())
	}
	var tag struct {
		ID int64 `json:"id"`
	}
	return data[struct {
		ID int64 `json:"id"`
	}](t, rec).ID + tag.ID
}

func TestReadLaterRequiresAuth(t *testing.T) {
	e := newTestEnv(t)
	rec := e.send(t, http.MethodGet, "/api/v1/read-later", nil, false)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET without auth = %d, want 401", rec.Code)
	}
	rec = e.send(t, http.MethodPost, "/api/v1/read-later", map[string]string{"url": "https://example.com"}, false)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("POST without auth = %d, want 401", rec.Code)
	}
}

func TestReadLaterMutationRequiresCSRF(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t)
	body, _ := json.Marshal(map[string]string{"url": "https://example.com"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/read-later", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(e.cookie)
	req.AddCookie(e.csrfCookie)
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("POST without csrf header = %d, want 403; body %s", rec.Code, rec.Body.String())
	}
	env := decode(t, rec)
	if env.Error == nil || env.Error.Code != "CSRF_INVALID" {
		t.Fatalf("expected CSRF_INVALID, got %+v", env.Error)
	}
}

func TestReadLaterCRUDDeleteRestore(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t)
	tagID := createTag(t, e, "reading")
	item := createItem(t, e, "https://example.com/article?utm_source=x#frag", map[string]any{
		"title": "Article", "excerpt": "hello", "tagIds": []int64{tagID}, "favorite": true, "priority": 3,
	})
	if item.Domain != "example.com" || item.Title != "Article" || item.State != StateUnread || !item.Favorite || item.Priority != 3 {
		t.Fatalf("created item unexpected: %+v", item)
	}
	if len(item.Tags) != 1 || item.Tags[0].ID != tagID {
		t.Fatalf("tags = %+v, want tag %d", item.Tags, tagID)
	}

	got := data[itemDTO](t, e.get(t, "/api/v1/read-later/"+itoa(item.ID)))
	if got.ID != item.ID || got.URL == "" {
		t.Fatalf("get item mismatch: %+v", got)
	}

	updated := data[itemDTO](t, e.patch(t, "/api/v1/read-later/"+itoa(item.ID), map[string]any{
		"title": "Updated", "favorite": false, "priority": 1, "tagIds": []int64{},
	}))
	if updated.Title != "Updated" || updated.Favorite || updated.Priority != 1 || len(updated.Tags) != 0 {
		t.Fatalf("updated item unexpected: %+v", updated)
	}

	trashed := data[itemDTO](t, e.del(t, "/api/v1/read-later/"+itoa(item.ID)))
	if trashed.State != StateTrash || trashed.DeletedAt == "" {
		t.Fatalf("trash unexpected: %+v", trashed)
	}
	list := data[listDTO](t, e.get(t, "/api/v1/read-later"))
	if list.Total != 0 {
		t.Fatalf("default list should hide trash, got total=%d", list.Total)
	}
	trashList := data[listDTO](t, e.get(t, "/api/v1/read-later?state=trash"))
	if trashList.Total != 1 {
		t.Fatalf("trash list total=%d, want 1", trashList.Total)
	}
	restored := data[itemDTO](t, e.post(t, "/api/v1/read-later/"+itoa(item.ID)+"/restore", nil))
	if restored.State != StateUnread || restored.DeletedAt != "" {
		t.Fatalf("restore unexpected: %+v", restored)
	}
}

func TestReadLaterArchiveOpenStateAndTimestamps(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t)
	item := createItem(t, e, "https://example.com/open", nil)
	opened := data[itemDTO](t, e.post(t, "/api/v1/read-later/"+itoa(item.ID)+"/open", nil))
	if opened.State != StateReading || opened.LastOpenedAt == "" {
		t.Fatalf("open should move unread -> reading and stamp time: %+v", opened)
	}
	archived := data[itemDTO](t, e.post(t, "/api/v1/read-later/"+itoa(item.ID)+"/archive", nil))
	if archived.State != StateArchived || archived.ArchivedAt == "" {
		t.Fatalf("archive unexpected: %+v", archived)
	}
	list := data[listDTO](t, e.get(t, "/api/v1/read-later"))
	if list.Total != 0 {
		t.Fatalf("default list should hide archived, got total=%d", list.Total)
	}
	archivedList := data[listDTO](t, e.get(t, "/api/v1/read-later?state=archived"))
	if archivedList.Total != 1 {
		t.Fatalf("archived list total=%d, want 1", archivedList.Total)
	}
}

func TestReadLaterDuplicateCreateUpdateRestoreConflicts(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t)
	first := createItem(t, e, "https://example.com/a?b=2&a=1#frag", nil)
	second := createItem(t, e, "https://example.com/b", nil)

	rec := e.post(t, "/api/v1/read-later", map[string]string{"url": "https://example.com/a?a=1&b=2"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate create = %d, want 409; %s", rec.Code, rec.Body.String())
	}
	rec = e.patch(t, "/api/v1/read-later/"+itoa(second.ID), map[string]string{"url": "https://example.com/a?a=1&b=2"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate update = %d, want 409; %s", rec.Code, rec.Body.String())
	}

	_ = data[itemDTO](t, e.del(t, "/api/v1/read-later/"+itoa(first.ID)))
	recreated := createItem(t, e, "https://example.com/a?a=1&b=2", nil)
	if recreated.ID == first.ID {
		t.Fatal("expected recreated item to be a new row")
	}
	rec = e.post(t, "/api/v1/read-later/"+itoa(first.ID)+"/restore", nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("restore conflict = %d, want 409; %s", rec.Code, rec.Body.String())
	}
}

func TestReadLaterSearchFilterPagination(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t)
	goTag := createTag(t, e, "go")
	jsTag := createTag(t, e, "js")
	createItem(t, e, "https://go.dev/doc", map[string]any{"title": "Go Docs", "siteName": "Go", "tagIds": []int64{goTag}, "favorite": true, "priority": 5})
	createItem(t, e, "https://example.com/js", map[string]any{"title": "JavaScript", "excerpt": "frontend", "tagIds": []int64{jsTag}, "priority": 1})
	createItem(t, e, "https://go.dev/blog", map[string]any{"title": "Go Blog", "tagIds": []int64{goTag}, "priority": 1})

	cases := []struct {
		path string
		want int
	}{
		{"/api/v1/read-later?q=frontend", 1},
		{"/api/v1/read-later?domain=go.dev", 2},
		{"/api/v1/read-later?tag=go", 2},
		{"/api/v1/read-later?tagId=" + itoa(jsTag), 1},
		{"/api/v1/read-later?favorite=true", 1},
		{"/api/v1/read-later?priority=1", 2},
		{"/api/v1/read-later?page=2&pageSize=2", 3},
	}
	for _, tc := range cases {
		res := data[listDTO](t, e.get(t, tc.path))
		if res.Total != tc.want {
			t.Fatalf("%s total=%d, want %d; %+v", tc.path, res.Total, tc.want, res)
		}
		if strings.Contains(tc.path, "page=2") && len(res.Items) != 1 {
			t.Fatalf("%s page items=%d, want 1; %+v", tc.path, len(res.Items), res)
		}
	}
	res := data[listDTO](t, e.get(t, "/api/v1/read-later?sort=priorityDesc&pageSize=1"))
	if len(res.Items) != 1 || res.Items[0].Priority != 5 {
		t.Fatalf("priorityDesc first = %+v", res.Items)
	}
}

func TestReadLaterTagOwnershipAndMultiUserIsolation(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t)
	ownTag := createTag(t, e, "mine")
	otherUserID := insertUser(t, e.db, "other")
	otherTagID := insertTag(t, e.db, otherUserID, "other-tag")
	otherItem, err := e.svc.Create(context.Background(), otherUserID, CreateParams{URL: "https://other.example/item", Title: "Other", TagIDs: []int64{otherTagID}})
	if err != nil {
		t.Fatalf("create other item: %v", err)
	}

	rec := e.post(t, "/api/v1/read-later", map[string]any{"url": "https://example.com/badtag", "tagIds": []int64{otherTagID}})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("foreign tag create = %d, want 400; %s", rec.Code, rec.Body.String())
	}
	item := createItem(t, e, "https://example.com/owned", map[string]any{"tagIds": []int64{ownTag}})
	if _, err := e.svc.store.GetItem(context.Background(), e.userID, otherItem.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("current user should not see other item; err=%v", err)
	}
	if _, err := e.svc.store.GetItem(context.Background(), otherUserID, item.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other user should not see current item; err=%v", err)
	}
}

func TestReadLaterPurgeRequiresAuth(t *testing.T) {
	e := newTestEnv(t)
	// Without login, DELETE /purge must return 401.
	rec := e.send(t, http.MethodDelete, "/api/v1/read-later/1/purge", nil, false)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("purge without auth = %d, want 401; %s", rec.Code, rec.Body.String())
	}
}

func TestReadLaterPurgeRequiresCSRF(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t)
	item := createItem(t, e, "https://example.com/csrf-purge", nil)
	_ = data[itemDTO](t, e.del(t, "/api/v1/read-later/"+itoa(item.ID)))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/read-later/"+itoa(item.ID)+"/purge", nil)
	req.AddCookie(e.cookie)
	req.AddCookie(e.csrfCookie)
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("purge without csrf header = %d, want 403; body %s", rec.Code, rec.Body.String())
	}
	env := decode(t, rec)
	if env.Error == nil || env.Error.Code != "CSRF_INVALID" {
		t.Fatalf("expected CSRF_INVALID, got %+v", env.Error)
	}
}

func TestReadLaterPurgePermanently(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t)

	// Create and trash an item.
	item := createItem(t, e, "https://example.com/purge-me", nil)
	trashed := data[itemDTO](t, e.del(t, "/api/v1/read-later/"+itoa(item.ID)))
	if trashed.State != StateTrash {
		t.Fatalf("expected trash state, got %s", trashed.State)
	}

	// Purge the trashed item via DELETE.
	rec := e.send(t, http.MethodDelete, "/api/v1/read-later/"+itoa(item.ID)+"/purge", nil, true)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("purge = %d, want 204; %s", rec.Code, rec.Body.String())
	}

	// Verify the item is gone (GET now returns 404).
	rec = e.get(t, "/api/v1/read-later/"+itoa(item.ID))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET after purge = %d, want 404; %s", rec.Code, rec.Body.String())
	}

	// Verify trash list is empty.
	trashList := data[listDTO](t, e.get(t, "/api/v1/read-later?state=trash"))
	if trashList.Total != 0 {
		t.Fatalf("trash list after purge total=%d, want 0", trashList.Total)
	}
}

func TestReadLaterPurgeNonTrashReturnsError(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t)

	// Try to purge an active item.
	item := createItem(t, e, "https://example.com/active", nil)
	rec := e.send(t, http.MethodDelete, "/api/v1/read-later/"+itoa(item.ID)+"/purge", nil, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("purge active = %d, want 400; %s", rec.Code, rec.Body.String())
	}

	// Archive then try to purge an archived item.
	archived := data[itemDTO](t, e.post(t, "/api/v1/read-later/"+itoa(item.ID)+"/archive", nil))
	rec = e.send(t, http.MethodDelete, "/api/v1/read-later/"+itoa(archived.ID)+"/purge", nil, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("purge archived = %d, want 400; %s", rec.Code, rec.Body.String())
	}
}

func TestReadLaterPurgeNotFound(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t)

	rec := e.send(t, http.MethodDelete, "/api/v1/read-later/99999/purge", nil, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("purge missing = %d, want 404; %s", rec.Code, rec.Body.String())
	}
}

func TestReadLaterPurgeCrossUserIsolation(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t)

	// Create an item for another user, trash it.
	otherUserID := insertUser(t, e.db, "other")
	otherItem, err := e.svc.Create(context.Background(), otherUserID, CreateParams{URL: "https://other.example/trash-me", Title: "Other"})
	if err != nil {
		t.Fatalf("create other item: %v", err)
	}
	if _, err := e.svc.Trash(context.Background(), otherUserID, otherItem.ID); err != nil {
		t.Fatalf("trash other item: %v", err)
	}

	// Current user must NOT be able to purge another user's item.
	rec := e.send(t, http.MethodDelete, "/api/v1/read-later/"+itoa(otherItem.ID)+"/purge", nil, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-user purge = %d, want 404; %s", rec.Code, rec.Body.String())
	}

	// Verify the other user's item still exists.
	item, err := e.svc.store.GetItem(context.Background(), otherUserID, otherItem.ID)
	if err != nil {
		t.Fatalf("other item should still exist after denied purge: %v", err)
	}
	if item.State != StateTrash {
		t.Fatalf("other item state should still be trash, got %s", item.State)
	}
}

func TestReadLaterPurgeAllowsRecreate(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t)

	// Create, trash, then purge.
	item := createItem(t, e, "https://example.com/recreate", nil)
	_ = data[itemDTO](t, e.del(t, "/api/v1/read-later/"+itoa(item.ID)))
	rec := e.send(t, http.MethodDelete, "/api/v1/read-later/"+itoa(item.ID)+"/purge", nil, true)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("purge = %d, want 204", rec.Code)
	}

	// Now recreate with the same URL — should succeed since the old row is gone.
	newItem := createItem(t, e, "https://example.com/recreate", nil)
	if newItem.ID == item.ID {
		t.Fatalf("expected different id after recreate, got same %d", newItem.ID)
	}
}

func insertUser(t *testing.T, database *sql.DB, username string) int64 {
	t.Helper()
	res, err := database.Exec(`INSERT INTO users (username, password_hash) VALUES (?, ?)`, username, "hash")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func insertTag(t *testing.T, database *sql.DB, userID int64, name string) int64 {
	t.Helper()
	res, err := database.Exec(`INSERT INTO tags (user_id, name) VALUES (?, ?)`, userID, name)
	if err != nil {
		t.Fatalf("insert tag: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func itoa(id int64) string { return strconv.FormatInt(id, 10) }
