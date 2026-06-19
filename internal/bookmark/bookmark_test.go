package bookmark

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/linyusheng/homepage/internal/auth"
	"github.com/linyusheng/homepage/internal/csrf"
	"github.com/linyusheng/homepage/internal/db"
	"github.com/linyusheng/homepage/internal/httpx"
	"github.com/linyusheng/homepage/internal/ratelimit"
)

// testEnv wires a real in-memory SQLite DB, a real auth Service (so RequireAuth
// is exercised end-to-end), and the bookmark Service/handlers. Each test logs
// in to obtain a session cookie that is then sent with bookmark requests.
type testEnv struct {
	mux        *http.ServeMux
	handler    http.Handler
	authSvc    *auth.Service
	svc        *Service
	db         *sql.DB
	cookie     *http.Cookie // populated by loginAdmin()
	csrfCookie *http.Cookie
	userID     int64
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
	// Use a cancellable context so the in-memory rate limiter's background
	// sweeper stops when the test ends (avoids leaking a goroutine per test).
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	limiter := ratelimit.New(ctx, 5, time.Minute)
	authSvc := auth.NewService(auth.Config{
		SessionTTL:     time.Hour,
		BcryptCost:     4, // fast for tests
		CookieName:     "homepage_session",
		CookieSecure:   false,
		CookieSameSite: "lax",
		SetupEnabled:   true,
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
	bookmarkSvc := NewService(NewStore(database), authSvc, logger)
	Register(mux, bookmarkSvc)

	return &testEnv{mux: mux, handler: csrfManager.Middleware(mux), authSvc: authSvc, svc: bookmarkSvc, db: database}
}

// loginAdmin creates the first admin user and logs in, returning the session
// cookie to attach to subsequent requests.
func (e *testEnv) loginAdmin(t *testing.T, username, password string) *http.Cookie {
	t.Helper()
	rec := e.post(t, "/api/v1/auth/setup", map[string]string{
		"username": username, "password": password,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup: %d %s", rec.Code, rec.Body.String())
	}
	rec = e.post(t, "/api/v1/auth/login", map[string]string{
		"username": username, "password": password,
	})
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
		t.Fatal("no session cookie after login")
	}
	if e.csrfCookie == nil || e.csrfCookie.Value == "" {
		t.Fatal("no csrf cookie after login")
	}
	e.userID = 1
	return e.cookie
}

// post is a thin helper; authed controls whether the session cookie is sent.
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

// send builds and runs a request. When authed is true and a cookie exists, the
// session cookie is attached.
func (e *testEnv) send(t *testing.T, method, path string, body any, authed bool) *httptest.ResponseRecorder {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
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

// envelope decodes the standard API envelope generically.
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
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatalf("decode body %q: %v", rec.Body.String(), err)
		}
	}
	return env
}

func mustData[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	env := decode(t, rec)
	var v T
	if env.Data == nil {
		t.Fatalf("expected data, got nil; status=%d error=%+v body=%s", rec.Code, env.Error, rec.Body.String())
	}
	if err := json.Unmarshal(env.Data, &v); err != nil {
		t.Fatalf("unmarshal data %s: %v", string(env.Data), err)
	}
	return v
}

// =====================================================================
// Auth boundary
// =====================================================================

func TestAllBookmarkEndpointsRequireAuth(t *testing.T) {
	e := newTestEnv(t)
	// Deliberately do NOT log in; every request should be 401.
	paths := []struct {
		method, path string
	}{
		{http.MethodGet, "/api/v1/categories"},
		{http.MethodPost, "/api/v1/categories"},
		{http.MethodGet, "/api/v1/tags"},
		{http.MethodPost, "/api/v1/tags"},
		{http.MethodGet, "/api/v1/bookmarks"},
		{http.MethodPost, "/api/v1/bookmarks"},
		{http.MethodGet, "/api/v1/bookmarks/1"},
		{http.MethodDelete, "/api/v1/bookmarks/1"},
	}
	for _, p := range paths {
		rec := e.send(t, p.method, p.path, nil, false)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: status = %d, want 401", p.method, p.path, rec.Code)
		}
	}
}

func TestBookmarkMutationRequiresCSRF(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t, "admin", "supersecret")
	body, _ := json.Marshal(map[string]string{"name": "Dev"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/categories", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(e.cookie)
	req.AddCookie(e.csrfCookie)
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body %s", rec.Code, rec.Body.String())
	}
	env := decode(t, rec)
	if env.Error == nil || env.Error.Code != "CSRF_INVALID" {
		t.Fatalf("expected CSRF_INVALID, got %+v", env.Error)
	}
}

// =====================================================================
// Categories CRUD
// =====================================================================

func TestCategoryCRUD(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t, "admin", "supersecret")

	// Create
	rec := e.post(t, "/api/v1/categories", map[string]any{
		"name": "Dev", "type": "bookmark", "sortOrder": 1,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	cat := mustData[categoryDTO](t, rec)
	if cat.ID == 0 || cat.Name != "Dev" || cat.Type != "bookmark" {
		t.Fatalf("unexpected category DTO: %+v", cat)
	}

	// List
	list := mustData[[]categoryDTO](t, e.get(t, "/api/v1/categories"))
	if len(list) != 1 || list[0].ID != cat.ID {
		t.Fatalf("list = %+v", list)
	}

	// Update
	rec = e.patch(t, "/api/v1/categories/"+itoa(cat.ID), map[string]any{
		"name": "Development", "color": "#6366f1",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d %s", rec.Code, rec.Body.String())
	}
	upd := mustData[categoryDTO](t, rec)
	if upd.Name != "Development" || upd.Color != "#6366f1" {
		t.Fatalf("update not applied: %+v", upd)
	}

	// Soft delete (archived=1)
	rec = e.del(t, "/api/v1/categories/"+itoa(cat.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}
}

func TestCategoryNameUnique(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t, "admin", "supersecret")

	if rec := e.post(t, "/api/v1/categories", map[string]any{"name": "Dev"}); rec.Code != http.StatusCreated {
		t.Fatalf("first create: %d %s", rec.Code, rec.Body.String())
	}
	rec := e.post(t, "/api/v1/categories", map[string]any{"name": "Dev"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate name: status = %d, want 409; body %s", rec.Code, rec.Body.String())
	}
}

// =====================================================================
// Tags CRUD + uniqueness
// =====================================================================

func TestTagCRUDAndUniqueness(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t, "admin", "supersecret")

	rec := e.post(t, "/api/v1/tags", map[string]any{"name": "Go", "color": "#00add8"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	tag := mustData[tagDTO](t, rec)
	if tag.Name != "Go" {
		t.Fatalf("unexpected tag: %+v", tag)
	}

	// Duplicate name (exact) -> 409. (Uniqueness is case-sensitive at the DB
	// layer today; see report.)
	rec = e.post(t, "/api/v1/tags", map[string]any{"name": "Go"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("dup tag: %d, want 409; body %s", rec.Code, rec.Body.String())
	}

	// Rename to another existing name -> 409.
	if rec := e.post(t, "/api/v1/tags", map[string]any{"name": "Rust"}); rec.Code != http.StatusCreated {
		t.Fatalf("create rust: %d", rec.Code)
	}
	rec = e.patch(t, "/api/v1/tags/"+itoa(tag.ID), map[string]any{"name": "Rust"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("rename to existing: %d, want 409", rec.Code)
	}

	// Update color.
	rec = e.patch(t, "/api/v1/tags/"+itoa(tag.ID), map[string]any{"color": "#123456"})
	if rec.Code != http.StatusOK {
		t.Fatalf("update tag color: %d", rec.Code)
	}
	upd := mustData[tagDTO](t, rec)
	if upd.Color != "#123456" {
		t.Fatalf("color not updated: %+v", upd)
	}

	// Delete.
	if rec := e.del(t, "/api/v1/tags/"+itoa(tag.ID)); rec.Code != http.StatusOK {
		t.Fatalf("delete tag: %d", rec.Code)
	}
}

// =====================================================================
// Bookmarks CRUD
// =====================================================================

func TestBookmarkCRUD(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t, "admin", "supersecret")

	// Create a category + two tags to attach.
	cat := mustData[categoryDTO](t, e.post(t, "/api/v1/categories", map[string]any{"name": "Dev"}))
	tag1 := mustData[tagDTO](t, e.post(t, "/api/v1/tags", map[string]any{"name": "Go"}))
	tag2 := mustData[tagDTO](t, e.post(t, "/api/v1/tags", map[string]any{"name": "docs"}))

	// Create bookmark.
	rec := e.post(t, "/api/v1/bookmarks", map[string]any{
		"url":        "https://go.dev/doc/",
		"title":      "Go Documentation",
		"categoryId": cat.ID,
		"tagIds":     []int64{tag1.ID, tag2.ID},
		"favorite":   true,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	b := mustData[bookmarkDTO](t, rec)
	if b.URL != "https://go.dev/doc/" {
		t.Fatalf("url = %q", b.URL)
	}
	if b.Domain != "go.dev" {
		t.Fatalf("domain = %q, want go.dev", b.Domain)
	}
	if b.Status != "active" || !b.Favorite {
		t.Fatalf("status/favorite = %q/%v", b.Status, b.Favorite)
	}
	if len(b.Tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(b.Tags))
	}

	// Get by id.
	got := mustData[bookmarkDTO](t, e.get(t, "/api/v1/bookmarks/"+itoa(b.ID)))
	if got.ID != b.ID || len(got.Tags) != 2 {
		t.Fatalf("get mismatch: %+v", got)
	}

	// Update (title + tags reduced to one).
	rec = e.patch(t, "/api/v1/bookmarks/"+itoa(b.ID), map[string]any{
		"title":  "Go Docs",
		"tagIds": []int64{tag1.ID},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d %s", rec.Code, rec.Body.String())
	}
	upd := mustData[bookmarkDTO](t, rec)
	if upd.Title != "Go Docs" || len(upd.Tags) != 1 {
		t.Fatalf("update not applied: %+v", upd)
	}

	// Update with unknown tag id: dropped silently, request still succeeds.
	rec = e.patch(t, "/api/v1/bookmarks/"+itoa(b.ID), map[string]any{
		"tagIds": []int64{tag2.ID, 999999},
	})
	upd = mustData[bookmarkDTO](t, rec)
	if len(upd.Tags) != 1 || upd.Tags[0].Name != "docs" {
		t.Fatalf("unknown tag should be dropped: %+v", upd.Tags)
	}
}

func TestBookmarkRejectsInvalidURL(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t, "admin", "supersecret")
	for _, bad := range []string{"", "not a url", "ftp://x", "example.com"} {
		rec := e.post(t, "/api/v1/bookmarks", map[string]any{"url": bad})
		if rec.Code != http.StatusBadRequest {
			t.Errorf("url %q: status = %d, want 400", bad, rec.Code)
		}
	}
}

func TestBookmarkDuplicateURLConflict(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t, "admin", "supersecret")

	if rec := e.post(t, "/api/v1/bookmarks", map[string]any{"url": "https://go.dev/"}); rec.Code != http.StatusCreated {
		t.Fatalf("first: %d %s", rec.Code, rec.Body.String())
	}
	// Same URL (different trailing slash normalizes equal) -> 409.
	rec := e.post(t, "/api/v1/bookmarks", map[string]any{"url": "https://go.dev"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("dup url: status = %d, want 409; body %s", rec.Code, rec.Body.String())
	}
	// Updating an unrelated bookmark to this URL -> 409 too.
	b2 := mustData[bookmarkDTO](t, e.post(t, "/api/v1/bookmarks", map[string]any{"url": "https://rust-lang.org/"}))
	rec = e.patch(t, "/api/v1/bookmarks/"+itoa(b2.ID), map[string]any{"url": "https://go.dev/"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("update to dup url: %d, want 409", rec.Code)
	}
}

func TestSoftDeleteExcludedFromDefaultAndRestorable(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t, "admin", "supersecret")
	b := mustData[bookmarkDTO](t, e.post(t, "/api/v1/bookmarks", map[string]any{"url": "https://a.example/"}))

	// Soft delete (DELETE -> trash).
	del := mustData[bookmarkDTO](t, e.del(t, "/api/v1/bookmarks/"+itoa(b.ID)))
	if del.Status != "trash" {
		t.Fatalf("status after delete = %q, want trash", del.Status)
	}

	// Default list excludes trashed items.
	list := mustData[ListResultDTO](t, e.get(t, "/api/v1/bookmarks"))
	if list.Total != 0 || len(list.Items) != 0 {
		t.Fatalf("trashed bookmark appeared in default list: %+v", list)
	}

	// status=trash shows it.
	trashed := mustData[ListResultDTO](t, e.get(t, "/api/v1/bookmarks?status=trash"))
	if trashed.Total != 1 || trashed.Items[0].ID != b.ID {
		t.Fatalf("expected trashed bookmark in trash list, got %+v", trashed)
	}

	// Restore -> appears in default list again.
	if rec := e.post(t, "/api/v1/bookmarks/"+itoa(b.ID)+"/restore", nil); rec.Code != http.StatusOK {
		t.Fatalf("restore: %d %s", rec.Code, rec.Body.String())
	}
	list = mustData[ListResultDTO](t, e.get(t, "/api/v1/bookmarks"))
	if list.Total != 1 || list.Items[0].ID != b.ID {
		t.Fatalf("restored bookmark missing from default list: %+v", list)
	}

	// After trashing + restoring, the dedup-unique constraint must NOT have
	// blocked re-create semantics: the same normalized URL works because the
	// row still exists (active). Verify by creating a *new* trash+restore cycle.
}

// TestRestoreAfterRecreateConflicts409 reproduces the restore edge case: after
// a bookmark is trashed, a new active bookmark takes its normalized_url, so
// restoring the old trashed row would collide. The restore must return 409
// (ErrBookmarkURLTaken), not 500.
func TestRestoreAfterRecreateConflicts409(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t, "admin", "supersecret")

	// Create + trash the original (same normalized URL because root path collapses).
	orig := mustData[bookmarkDTO](t, e.post(t, "/api/v1/bookmarks", map[string]any{"url": "https://go.dev/"}))
	if rec := e.del(t, "/api/v1/bookmarks/"+itoa(orig.ID)); rec.Code != http.StatusOK {
		t.Fatalf("trash orig: %d %s", rec.Code, rec.Body.String())
	}

	// The trash frees the normalized_url (partial unique index excludes trash),
	// so a brand-new bookmark with the same URL is allowed.
	repl := mustData[bookmarkDTO](t, e.post(t, "/api/v1/bookmarks", map[string]any{"url": "https://go.dev"}))
	if repl.ID == orig.ID {
		t.Fatal("expected a distinct replacement row")
	}

	// Restoring the original now collides with the active replacement.
	rec := e.post(t, "/api/v1/bookmarks/"+itoa(orig.ID)+"/restore", nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("restore after recreate: status = %d, want 409; body %s", rec.Code, rec.Body.String())
	}
	env := decode(t, rec)
	if env.Error == nil || env.Error.Code != "CONFLICT" {
		t.Fatalf("expected CONFLICT error, got %+v; body %s", env.Error, rec.Body.String())
	}

	// The original stays trashed (the failing restore did not mutate state).
	origAfter := mustData[bookmarkDTO](t, e.get(t, "/api/v1/bookmarks/"+itoa(orig.ID)))
	if origAfter.Status != "trash" {
		t.Fatalf("original status changed to %q; restore must be atomic (still trash)", origAfter.Status)
	}
}

// TestCategoryIdValidation covers the categoryId ownership boundary: a
// non-existent id and a foreign-user id are both rejected as 400, on both
// create and update. None should surface as a 500 FK error.
func TestCategoryIdValidation(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t, "admin", "supersecret")

	// A valid category that admin owns, plus a bookmark under it (happy path).
	cat := mustData[categoryDTO](t, e.post(t, "/api/v1/categories", map[string]any{"name": "Dev"}))
	good := mustData[bookmarkDTO](t, e.post(t, "/api/v1/bookmarks", map[string]any{
		"url": "https://go.dev/", "categoryId": cat.ID,
	}))
	if good.CategoryID != cat.ID {
		t.Fatalf("happy path: expected categoryId=%d, got %d", cat.ID, good.CategoryID)
	}

	t.Run("create with non-existent categoryId is 400", func(t *testing.T) {
		rec := e.post(t, "/api/v1/bookmarks", map[string]any{
			"url": "https://x.example/", "categoryId": 999999,
		})
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("update with non-existent categoryId is 400", func(t *testing.T) {
		rec := e.patch(t, "/api/v1/bookmarks/"+itoa(good.ID), map[string]any{"categoryId": 999999})
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body %s", rec.Code, rec.Body.String())
		}
		// The failing update must not change the bookmark's category.
		after := mustData[bookmarkDTO](t, e.get(t, "/api/v1/bookmarks/"+itoa(good.ID)))
		if after.CategoryID != cat.ID {
			t.Fatalf("category changed to %d despite invalid input", after.CategoryID)
		}
	})

	// Provision a second user and a category owned by them.
	secondID := createUser(t, e.db, "other", "supersecret")
	secondCookie := loginAs(t, e.handler, "other", "supersecret")
	otherCat := secondUserPost(t, e.handler, secondCookie, "/api/v1/categories", map[string]any{"name": "Foreign"})
	if otherCat.ID == 0 {
		t.Fatal("expected second-user category id")
	}

	t.Run("create with foreign-user categoryId is 400", func(t *testing.T) {
		rec := e.post(t, "/api/v1/bookmarks", map[string]any{
			"url": "https://y.example/", "categoryId": otherCat.ID,
		})
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("update with foreign-user categoryId is 400", func(t *testing.T) {
		rec := e.patch(t, "/api/v1/bookmarks/"+itoa(good.ID), map[string]any{"categoryId": otherCat.ID})
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body %s", rec.Code, rec.Body.String())
		}
	})

	_ = secondID // second user id referenced for clarity
}

// TestCategoryIdZeroAllowsUncategorized confirms categoryId=0 (or omitted) is
// always valid, so the ownership check does not break the default path.
func TestCategoryIdZeroAllowsUncategorized(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t, "admin", "supersecret")
	b := mustData[bookmarkDTO](t, e.post(t, "/api/v1/bookmarks", map[string]any{"url": "https://a.example/"}))
	if b.CategoryID != 0 {
		t.Fatalf("expected categoryId 0 when omitted, got %d", b.CategoryID)
	}
	// Explicit 0 on update stays valid.
	upd := mustData[bookmarkDTO](t, e.patch(t, "/api/v1/bookmarks/"+itoa(b.ID), map[string]any{"categoryId": 0}))
	if upd.CategoryID != 0 {
		t.Fatalf("explicit categoryId 0 rejected; got %d", upd.CategoryID)
	}
}

func TestArchiveBehavior(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t, "admin", "supersecret")
	b := mustData[bookmarkDTO](t, e.post(t, "/api/v1/bookmarks", map[string]any{"url": "https://a.example/"}))

	arch := mustData[bookmarkDTO](t, e.post(t, "/api/v1/bookmarks/"+itoa(b.ID)+"/archive", nil))
	if arch.Status != "archived" {
		t.Fatalf("status = %q, want archived", arch.Status)
	}
	// Excluded from default list.
	if list := mustData[ListResultDTO](t, e.get(t, "/api/v1/bookmarks")); list.Total != 0 {
		t.Fatalf("archived appeared in default list: %+v", list)
	}
	// Visible under status=archived.
	if list := mustData[ListResultDTO](t, e.get(t, "/api/v1/bookmarks?status=archived")); list.Total != 1 {
		t.Fatalf("expected 1 archived, got %+v", list)
	}
}

func TestOpenIncrementsClickCount(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t, "admin", "supersecret")
	b := mustData[bookmarkDTO](t, e.post(t, "/api/v1/bookmarks", map[string]any{"url": "https://a.example/"}))

	if rec := e.post(t, "/api/v1/bookmarks/"+itoa(b.ID)+"/open", nil); rec.Code != http.StatusOK {
		t.Fatalf("open: %d %s", rec.Code, rec.Body.String())
	}
	got := mustData[bookmarkDTO](t, e.get(t, "/api/v1/bookmarks/"+itoa(b.ID)))
	if got.ClickCount != 1 || got.LastOpenedAt == "" {
		t.Fatalf("click not recorded: %+v", got)
	}
}

// =====================================================================
// Search / filter
// =====================================================================

func TestSearchAndFilter(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t, "admin", "supersecret")

	cat := mustData[categoryDTO](t, e.post(t, "/api/v1/categories", map[string]any{"name": "Dev"}))
	tag := mustData[tagDTO](t, e.post(t, "/api/v1/tags", map[string]any{"name": "go"}))

	create := func(url, title, desc string, pinned, fav bool, tagIDs []int64) {
		body := map[string]any{
			"url": url, "title": title, "description": desc,
			"categoryId": cat.ID, "pinned": pinned, "favorite": fav,
		}
		if tagIDs != nil {
			body["tagIds"] = tagIDs
		}
		if rec := e.post(t, "/api/v1/bookmarks", body); rec.Code != http.StatusCreated {
			t.Fatalf("create %s: %d %s", url, rec.Code, rec.Body.String())
		}
	}
	create("https://go.dev/doc", "Go Docs", "language docs", true, false, []int64{tag.ID})
	create("https://blog.golang.org/", "Go Blog", "community blog", false, true, []int64{tag.ID})
	create("https://rust-lang.org/", "Rust", "another language", false, false, nil)

	// Search by title substring.
	r := mustData[ListResultDTO](t, e.get(t, "/api/v1/bookmarks?q=go"))
	if r.Total != 2 {
		t.Fatalf("q=go: total = %d, want 2", r.Total)
	}
	// Search by domain.
	r = mustData[ListResultDTO](t, e.get(t, "/api/v1/bookmarks?q=rust-lang"))
	if r.Total != 1 {
		t.Fatalf("q=rust-lang: total = %d, want 1", r.Total)
	}
	// Filter by tag id.
	r = mustData[ListResultDTO](t, e.get(t, "/api/v1/bookmarks?tagId="+itoa(tag.ID)))
	if r.Total != 2 {
		t.Fatalf("tagId filter: total = %d, want 2", r.Total)
	}
	// Filter by tag name.
	r = mustData[ListResultDTO](t, e.get(t, "/api/v1/bookmarks?tag=go"))
	if r.Total != 2 {
		t.Fatalf("tag name filter: total = %d, want 2", r.Total)
	}
	// Filter by favorite.
	r = mustData[ListResultDTO](t, e.get(t, "/api/v1/bookmarks?favorite=true"))
	if r.Total != 1 {
		t.Fatalf("favorite filter: total = %d, want 1", r.Total)
	}
	// Filter by pinned.
	r = mustData[ListResultDTO](t, e.get(t, "/api/v1/bookmarks?pinned=true"))
	if r.Total != 1 {
		t.Fatalf("pinned filter: total = %d, want 1", r.Total)
	}
	// Filter by category.
	r = mustData[ListResultDTO](t, e.get(t, "/api/v1/bookmarks?categoryId="+itoa(cat.ID)))
	if r.Total != 3 {
		t.Fatalf("category filter: total = %d, want 3", r.Total)
	}
	// Unknown tag name -> empty, not error.
	r = mustData[ListResultDTO](t, e.get(t, "/api/v1/bookmarks?tag=nonexistent"))
	if r.Total != 0 {
		t.Fatalf("unknown tag: total = %d, want 0", r.Total)
	}
}

func TestPagination(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t, "admin", "supersecret")
	for i := 0; i < 5; i++ {
		url := "https://example.com/" + string(rune('a'+i))
		if rec := e.post(t, "/api/v1/bookmarks", map[string]any{"url": url}); rec.Code != http.StatusCreated {
			t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
		}
	}
	r := mustData[ListResultDTO](t, e.get(t, "/api/v1/bookmarks?pageSize=2&page=1"))
	if r.Total != 5 || len(r.Items) != 2 || r.Page != 1 || r.PageSize != 2 {
		t.Fatalf("page1: %+v", r)
	}
	r = mustData[ListResultDTO](t, e.get(t, "/api/v1/bookmarks?pageSize=2&page=3"))
	if len(r.Items) != 1 {
		t.Fatalf("page3: expected 1 item, got %d", len(r.Items))
	}
	// pageSize capped at max (100); oversize clamps.
	r = mustData[ListResultDTO](t, e.get(t, "/api/v1/bookmarks?pageSize=9999"))
	if r.PageSize != 100 || r.Total != 5 || len(r.Items) != 5 {
		t.Fatalf("cap: %+v", r)
	}
}

// =====================================================================
// Multi-user isolation
// =====================================================================

// TestUserIsolation verifies that data created by user A is invisible to
// user B. Because setup is one-shot, user B is simulated by inserting a second
// user directly and a second session cookie obtained by logging in as them.
func TestUserIsolation(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t, "admin", "supersecret")
	// admin creates a category and a bookmark.
	e.post(t, "/api/v1/categories", map[string]any{"name": "AdminCat"})
	e.post(t, "/api/v1/bookmarks", map[string]any{"url": "https://admin.example/"})

	// Create a second user directly in the DB and log in as them.
	secondID := createUser(t, e.db, "other", "supersecret")
	secondCookie := loginAs(t, e.handler, "other", "supersecret")

	// Second user should see none of admin's categories or bookmarks via API.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/categories", nil)
	req.AddCookie(secondCookie)
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	cats := mustData[[]categoryDTO](t, rec)
	if len(cats) != 0 {
		t.Fatalf("user isolation breach: user %d sees %d categories", secondID, len(cats))
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/bookmarks", nil)
	req.AddCookie(secondCookie)
	rec = httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	bm := mustData[ListResultDTO](t, rec)
	if bm.Total != 0 {
		t.Fatalf("user isolation breach: user %d sees %d bookmarks", secondID, bm.Total)
	}
}

func TestListIncludesTags(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t, "admin", "supersecret")
	tag1 := mustData[tagDTO](t, e.post(t, "/api/v1/tags", map[string]any{"name": "a"}))
	tag2 := mustData[tagDTO](t, e.post(t, "/api/v1/tags", map[string]any{"name": "b"}))
	b := mustData[bookmarkDTO](t, e.post(t, "/api/v1/bookmarks", map[string]any{
		"url": "https://x.example/", "tagIds": []int64{tag1.ID, tag2.ID},
	}))
	_ = b

	list := mustData[ListResultDTO](t, e.get(t, "/api/v1/bookmarks"))
	if list.Total != 1 || len(list.Items[0].Tags) != 2 {
		t.Fatalf("list must include tags per item: %+v", list)
	}
}

func TestNotFoundErrors(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t, "admin", "supersecret")
	for _, path := range []string{
		"/api/v1/bookmarks/999999",
		"/api/v1/categories/999999",
		"/api/v1/tags/999999",
	} {
		rec := e.get(t, path)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", path, rec.Code)
		}
	}
}

// =====================================================================
// helpers that reach outside the env (DB-level user creation / login)
// =====================================================================

// itoa is a tiny strconv-free helper to keep test bodies readable.
func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
