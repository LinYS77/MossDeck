package backup

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/linyusheng/homepage/internal/auth"
	"github.com/linyusheng/homepage/internal/bookmark"
	"github.com/linyusheng/homepage/internal/csrf"
	"github.com/linyusheng/homepage/internal/db"
	"github.com/linyusheng/homepage/internal/httpx"
	"github.com/linyusheng/homepage/internal/ratelimit"
	"github.com/linyusheng/homepage/internal/readlater"
)

type testEnv struct {
	mux        *http.ServeMux
	handler    http.Handler
	db         *sql.DB
	svc        *Service
	authSvc    *auth.Service
	cookie     *http.Cookie
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
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	limiter := ratelimit.New(ctx, 5, time.Minute)
	authSvc := auth.NewService(auth.Config{
		SessionTTL: time.Hour, BcryptCost: 4, CookieName: "homepage_session",
		CookieSecure: false, CookieSameSite: "lax", SetupEnabled: true,
	}, auth.NewStore(database), limiter, resolver, logger)
	csrfManager := csrf.New(csrf.Config{
		CookieName: "homepage_csrf", HeaderName: "X-CSRF-Token",
		SessionCookieName: "homepage_session",
		CookieSecure:      false, CookieSameSite: "lax", TTL: time.Hour,
	})
	authSvc.SetCSRFManager(csrfManager)

	mux := http.NewServeMux()
	auth.Register(mux, authSvc)
	mux.Handle("GET /api/v1/auth/csrf", csrfManager.HandleToken())

	bmSvc := bookmark.NewService(bookmark.NewStore(database), authSvc, logger)
	bookmark.Register(mux, bmSvc)
	rlSvc := readlater.NewService(readlater.NewStore(database), authSvc, logger)
	readlater.Register(mux, rlSvc)

	svc := NewService(NewStore(database), database, authSvc, logger)
	Register(mux, svc)

	return &testEnv{
		mux: mux, handler: csrfManager.Middleware(mux), db: database,
		svc: svc, authSvc: authSvc,
	}
}

func (e *testEnv) loginOwner(t *testing.T) {
	t.Helper()
	rec := e.post(t, "/api/v1/auth/setup", map[string]string{"password": "StrongPass1!", "confirmPassword": "StrongPass1!"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup: %d %s", rec.Code, rec.Body.String())
	}
	rec = e.post(t, "/api/v1/auth/login", map[string]string{"password": "StrongPass1!"})
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
	return e.send(t, http.MethodPost, path, body, true)
}

func (e *testEnv) get(t *testing.T, path string) *httptest.ResponseRecorder {
	return e.send(t, http.MethodGet, path, nil, true)
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

// seedData creates some categories, tags, bookmarks, and read-later items
// for testing export/import.
func (e *testEnv) seedData(t *testing.T) {
	t.Helper()
	e.post(t, "/api/v1/categories", map[string]any{"name": "Dev", "type": "bookmark"})
	e.post(t, "/api/v1/tags", map[string]string{"name": "go"})
	e.post(t, "/api/v1/bookmarks", map[string]any{
		"url": "https://go.dev", "title": "Go", "categoryId": 1, "tagIds": []int{1},
	})
	e.post(t, "/api/v1/read-later", map[string]any{
		"url": "https://example.com/article", "title": "Article",
	})
}

// =====================================================================
// Tests
// =====================================================================

func TestExportRequiresAuth(t *testing.T) {
	e := newTestEnv(t)
	rec := e.send(t, http.MethodGet, "/api/v1/backup/export", nil, false)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("export without auth = %d, want 401", rec.Code)
	}
}

func TestImportRequiresAuth(t *testing.T) {
	e := newTestEnv(t)
	rec := e.send(t, http.MethodPost, "/api/v1/backup/import",
		map[string]any{"mode": "merge", "backup": map[string]any{}}, false)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("import without auth = %d, want 401", rec.Code)
	}
}

func TestImportRequiresCSRF(t *testing.T) {
	e := newTestEnv(t)
	e.loginOwner(t)
	body, _ := json.Marshal(map[string]string{"mode": "merge"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backup/import", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(e.cookie)
	req.AddCookie(e.csrfCookie) // cookie present but header missing
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("import without csrf header = %d, want 403", rec.Code)
	}
}

func TestExportEmpty(t *testing.T) {
	e := newTestEnv(t)
	e.loginOwner(t)

	rec := e.get(t, "/api/v1/backup/export")
	if rec.Code != http.StatusOK {
		t.Fatalf("export empty = %d, want 200; %s", rec.Code, rec.Body.String())
	}
	export := data[ExportData](t, rec)
	if export.Version != Version {
		t.Fatalf("version = %d, want %d", export.Version, Version)
	}
	if export.App != App {
		t.Fatalf("app = %q, want %q", export.App, App)
	}
	if export.ExportedAt == "" {
		t.Fatal("exportedAt is empty")
	}
	// Even empty data should have non-nil slices.
	if export.Data.Categories == nil {
		t.Fatal("categories should be [] not null")
	}
	if export.Data.Tags == nil {
		t.Fatal("tags should be [] not null")
	}
}

func TestExportIncludesAllData(t *testing.T) {
	e := newTestEnv(t)
	e.loginOwner(t)

	// Create categories, tags, bookmarks, read-later.
	e.post(t, "/api/v1/categories", map[string]any{"name": "Dev", "type": "bookmark"})
	e.post(t, "/api/v1/tags", map[string]string{"name": "go"})

	e.post(t, "/api/v1/bookmarks", map[string]any{
		"url": "https://go.dev", "title": "Go", "categoryId": 1, "tagIds": []int{1},
	})
	// Create a second bookmark and trash it.
	e.post(t, "/api/v1/bookmarks", map[string]any{
		"url": "https://trash.example", "title": "TrashBM",
	})
	e.post(t, "/api/v1/read-later", map[string]any{
		"url": "https://example.com/article", "title": "Article",
	})
	// Trash the second bookmark (id autoincrement → 2).
	rec := e.send(t, http.MethodDelete, "/api/v1/bookmarks/2", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("trash bookmark 2 = %d, want 200", rec.Code)
	}

	rec = e.get(t, "/api/v1/backup/export")
	if rec.Code != http.StatusOK {
		t.Fatalf("export = %d; %s", rec.Code, rec.Body.String())
	}
	export := data[ExportData](t, rec)

	// Categories.
	if len(export.Data.Categories) != 1 {
		t.Fatalf("categories = %d, want 1", len(export.Data.Categories))
	}
	if export.Data.Categories[0].Name != "Dev" {
		t.Fatalf("category name = %q", export.Data.Categories[0].Name)
	}

	// Tags.
	if len(export.Data.Tags) != 1 {
		t.Fatalf("tags = %d, want 1", len(export.Data.Tags))
	}

	// Bookmarks (including trashed).
	if len(export.Data.Bookmarks) != 2 {
		t.Fatalf("bookmarks = %d, want 2 (includes trashed)", len(export.Data.Bookmarks))
	}
	hasTrash := false
	for _, bm := range export.Data.Bookmarks {
		if bm.Status == "trash" {
			hasTrash = true
		}
	}
	if !hasTrash {
		t.Fatal("export should include trashed bookmarks")
	}

	// Read-later.
	if len(export.Data.ReadLaterItems) != 1 {
		t.Fatalf("readLaterItems = %d, want 1", len(export.Data.ReadLaterItems))
	}
}

func TestImportMergeDedup(t *testing.T) {
	e := newTestEnv(t)
	e.loginOwner(t)

	// Create initial data.
	e.post(t, "/api/v1/categories", map[string]any{"name": "Dev", "type": "bookmark"})
	e.post(t, "/api/v1/tags", map[string]string{"name": "go"})
	e.post(t, "/api/v1/bookmarks", map[string]any{
		"url": "https://go.dev", "title": "Go Site", "tagIds": []int{1}, "categoryId": 1,
	})

	// Now import a backup that has the same bookmark with updated title.
	backup := ExportData{
		Version: Version,
		Data: ExportPayload{
			Categories: []CategoryExport{{Name: "Dev", Type: "bookmark"}},
			Tags:       []TagExport{{Name: "go"}, {Name: "rust"}},
			Bookmarks: []BookmarkExport{
				{URL: "https://go.dev", Title: "Go Language", Status: "active", Category: "Dev", Tags: []string{"go", "rust"}},
				{URL: "https://new.example", Title: "New", Status: "active"},
			},
		},
	}

	rec := e.send(t, http.MethodPost, "/api/v1/backup/import",
		ImportRequest{Mode: "merge", Backup: backup}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("import merge = %d; %s", rec.Code, rec.Body.String())
	}
	summary := data[ImportSummary](t, rec)
	if summary.Mode != "merge" {
		t.Fatalf("mode = %q", summary.Mode)
	}
	// Category: 1 updated (already existed).
	if summary.Categories.Updated != 1 {
		t.Fatalf("categories updated = %d, want 1", summary.Categories.Updated)
	}
	// Tags: 1 updated (go), 1 created (rust).
	if summary.Tags.Updated != 1 || summary.Tags.Created != 1 {
		t.Fatalf("tags updated=%d created=%d, want 1/1", summary.Tags.Updated, summary.Tags.Created)
	}
	// Bookmarks: 1 updated (go.dev), 1 created (new).
	if summary.Bookmarks.Updated != 1 || summary.Bookmarks.Created != 1 {
		t.Fatalf("bookmarks updated=%d created=%d, want 1/1", summary.Bookmarks.Updated, summary.Bookmarks.Created)
	}

	// Verify the updated title.
	var bms struct {
		Items []struct {
			Title string `json:"title"`
		} `json:"items"`
	}
	rec2 := e.get(t, "/api/v1/bookmarks")
	bms = data[struct {
		Items []struct {
			Title string `json:"title"`
		} `json:"items"`
	}](t, rec2)
	found := false
	for _, bm := range bms.Items {
		if bm.Title == "Go Language" {
			found = true
		}
	}
	if !found {
		t.Fatal("title should have been updated to 'Go Language'")
	}
}

func TestImportReplaceClearsAndInserts(t *testing.T) {
	e := newTestEnv(t)
	e.loginOwner(t)

	// Create initial data.
	e.post(t, "/api/v1/categories", map[string]any{"name": "Old", "type": "bookmark"})
	e.post(t, "/api/v1/bookmarks", map[string]any{"url": "https://old.example", "title": "Old"})

	// Now replace with completely different data.
	backup := ExportData{
		Version: Version,
		Data: ExportPayload{
			Categories: []CategoryExport{{Name: "New", Type: "bookmark"}},
			Bookmarks:  []BookmarkExport{{URL: "https://new.example", Title: "New", Status: "active"}},
		},
	}

	rec := e.send(t, http.MethodPost, "/api/v1/backup/import",
		ImportRequest{Mode: "replace", Backup: backup}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("import replace = %d; %s", rec.Code, rec.Body.String())
	}
	summary := data[ImportSummary](t, rec)
	if summary.Categories.Created != 1 {
		t.Fatalf("categories created = %d, want 1", summary.Categories.Created)
	}
	if summary.Bookmarks.Created != 1 {
		t.Fatalf("bookmarks created = %d, want 1", summary.Bookmarks.Created)
	}

	// Verify old data is gone.
	var bms struct {
		Items []struct {
			Title string `json:"title"`
		} `json:"items"`
		Total int `json:"total"`
	}
	rec2 := e.get(t, "/api/v1/bookmarks")
	bms = data[struct {
		Items []struct {
			Title string `json:"title"`
		} `json:"items"`
		Total int `json:"total"`
	}](t, rec2)
	if bms.Total != 1 || bms.Items[0].Title != "New" {
		t.Fatalf("after replace: total=%d title=%q, want 1/New", bms.Total, bms.Items[0].Title)
	}
}

func TestImportReplaceDoesNotAffectOtherUser(t *testing.T) {
	e := newTestEnv(t)
	e.loginOwner(t)

	// Create data for another user.
	otherUserID := insertUser(t, e.db)
	// Insert bookmark directly for the other user.
	e.db.ExecContext(context.Background(),
		`INSERT INTO bookmarks (user_id, url, normalized_url, title, domain, status)
		 VALUES (?, 'https://other.example', 'https://other.example', 'Other', 'other.example', 'active')`,
		otherUserID)

	// Replace owner data.
	backup := ExportData{
		Version: Version,
		Data: ExportPayload{
			Bookmarks: []BookmarkExport{{URL: "https://owner.example", Title: "Owner", Status: "active"}},
		},
	}

	rec := e.send(t, http.MethodPost, "/api/v1/backup/import",
		ImportRequest{Mode: "replace", Backup: backup}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("import replace = %d; %s", rec.Code, rec.Body.String())
	}

	// Verify owner has only the new bookmark.
	var bms struct{ Total int }
	rec2 := e.get(t, "/api/v1/bookmarks")
	bms = data[struct{ Total int }](t, rec2)
	if bms.Total != 1 {
		t.Fatalf("owner bookmarks total = %d, want 1", bms.Total)
	}

	// Verify other user's bookmark still exists.
	var count int
	if err := e.db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM bookmarks WHERE user_id = ?`, otherUserID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("other user bookmarks = %d, want 1", count)
	}
}

func TestImportInvalidVersionRejected(t *testing.T) {
	e := newTestEnv(t)
	e.loginOwner(t)

	backup := ExportData{Version: 99}
	rec := e.send(t, http.MethodPost, "/api/v1/backup/import",
		ImportRequest{Mode: "merge", Backup: backup}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status %d", rec.Code)
	}
	summary := data[ImportSummary](t, rec)
	if len(summary.Errors) == 0 {
		t.Fatal("expected errors for invalid version")
	}
}

func TestImportInvalidURLRejected(t *testing.T) {
	e := newTestEnv(t)
	e.loginOwner(t)

	backup := ExportData{
		Version: Version,
		Data: ExportPayload{
			Bookmarks: []BookmarkExport{{URL: "not-a-valid-url", Title: "Bad", Status: "active"}},
		},
	}
	rec := e.send(t, http.MethodPost, "/api/v1/backup/import",
		ImportRequest{Mode: "merge", Backup: backup}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status %d", rec.Code)
	}
	summary := data[ImportSummary](t, rec)
	if len(summary.Errors) == 0 {
		t.Fatal("expected errors for invalid URL")
	}
}

func TestImportInvalidModeRejected(t *testing.T) {
	e := newTestEnv(t)
	e.loginOwner(t)

	backup := ExportData{Version: Version}
	rec := e.send(t, http.MethodPost, "/api/v1/backup/import",
		ImportRequest{Mode: "invalid", Backup: backup}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status %d", rec.Code)
	}
	summary := data[ImportSummary](t, rec)
	if len(summary.Errors) == 0 || summary.Errors[0] == "" {
		t.Fatalf("expected errors for invalid mode, got %v", summary.Errors)
	}
}

func TestExportThenImportRoundtrip(t *testing.T) {
	e := newTestEnv(t)
	e.loginOwner(t)

	// Seed data.
	e.post(t, "/api/v1/categories", map[string]any{"name": "Dev", "type": "bookmark"})
	e.post(t, "/api/v1/tags", map[string]string{"name": "go"})
	e.post(t, "/api/v1/bookmarks", map[string]any{
		"url": "https://go.dev", "title": "Go", "categoryId": 1, "tagIds": []int{1},
	})
	e.post(t, "/api/v1/read-later", map[string]any{
		"url": "https://example.com/article", "title": "Article",
	})

	// Export.
	rec := e.get(t, "/api/v1/backup/export")
	export := data[ExportData](t, rec)

	// Replace with the exported data (roundtrip).
	rec2 := e.send(t, http.MethodPost, "/api/v1/backup/import",
		ImportRequest{Mode: "replace", Backup: export}, true)
	if rec2.Code != http.StatusOK {
		t.Fatalf("roundtrip import = %d; %s", rec2.Code, rec2.Body.String())
	}

	// Verify data is restored.
	var bms struct{ Total int }
	rec3 := e.get(t, "/api/v1/bookmarks")
	bms = data[struct{ Total int }](t, rec3)
	if bms.Total != 1 {
		t.Fatalf("after roundtrip: bookmarks total = %d, want 1", bms.Total)
	}
	var rl struct{ Total int }
	rec4 := e.get(t, "/api/v1/read-later")
	rl = data[struct{ Total int }](t, rec4)
	if rl.Total != 1 {
		t.Fatalf("after roundtrip: readlater total = %d, want 1", rl.Total)
	}
}

func insertUser(t *testing.T, database *sql.DB) int64 {
	t.Helper()
	res, err := database.Exec(`INSERT INTO users (password_hash) VALUES (?)`, "hash")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// =====================================================================
// New security/validation tests
// =====================================================================

func TestImportInvalidEnumRejected(t *testing.T) {
	e := newTestEnv(t)
	e.loginOwner(t)

	backup := ExportData{
		Version: Version,
		Data: ExportPayload{
			Categories:     []CategoryExport{{Name: "Bad", Type: "invalid"}},
			Bookmarks:      []BookmarkExport{{URL: "https://x.com", Title: "X", Status: "badstatus"}},
			ReadLaterItems: []ReadLaterItemExport{{URL: "https://y.com", Title: "Y", State: "badstate"}},
		},
	}
	rec := e.send(t, http.MethodPost, "/api/v1/backup/import",
		ImportRequest{Mode: "merge", Backup: backup}, true)

	summary := data[ImportSummary](t, rec)
	// Should have at least 3 enum errors.
	errCount := 0
	for _, err := range summary.Errors {
		t.Logf("error: %s", err)
		errCount++
	}
	if errCount < 3 {
		t.Fatalf("expected at least 3 enum errors, got %d", errCount)
	}
}

func TestImportInvalidTimeRejected(t *testing.T) {
	e := newTestEnv(t)
	e.loginOwner(t)

	backup := ExportData{
		Version: Version,
		Data: ExportPayload{
			Bookmarks: []BookmarkExport{{
				URL: "https://x.com", Title: "X", Status: "active",
				CreatedAt: "not-a-date",
			}},
		},
	}
	rec := e.send(t, http.MethodPost, "/api/v1/backup/import",
		ImportRequest{Mode: "merge", Backup: backup}, true)

	summary := data[ImportSummary](t, rec)
	found := false
	for _, err := range summary.Errors {
		if strings.Contains(err, "invalid createdAt") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected invalid createdAt error, got %v", summary.Errors)
	}
}

func TestMergeDoesNotChangeStatus(t *testing.T) {
	e := newTestEnv(t)
	e.loginOwner(t)

	// Create an active bookmark.
	e.post(t, "/api/v1/bookmarks", map[string]any{
		"url": "https://keep-active.example", "title": "Keep",
	})

	// Merge a backup that wants to set the same bookmark to trash.
	backup := ExportData{
		Version: Version,
		Data: ExportPayload{
			Bookmarks: []BookmarkExport{{
				URL: "https://keep-active.example", Title: "Keep", Status: "trash",
			}},
		},
	}
	rec := e.send(t, http.MethodPost, "/api/v1/backup/import",
		ImportRequest{Mode: "merge", Backup: backup}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("merge = %d; %s", rec.Code, rec.Body.String())
	}

	// Verify the bookmark is still active (not trashed).
	var bms struct {
		Items []struct {
			Status string `json:"status"`
		} `json:"items"`
	}
	rec2 := e.get(t, "/api/v1/bookmarks")
	bms = data[struct {
		Items []struct {
			Status string `json:"status"`
		} `json:"items"`
	}](t, rec2)
	if len(bms.Items) != 1 || bms.Items[0].Status != "active" {
		t.Fatalf("merge should not trash existing bookmark: status=%q", bms.Items[0].Status)
	}
}

func TestMergeDoesNotChangeReadLaterState(t *testing.T) {
	e := newTestEnv(t)
	e.loginOwner(t)

	e.post(t, "/api/v1/read-later", map[string]any{
		"url": "https://keep-unread.example", "title": "Keep",
	})

	backup := ExportData{
		Version: Version,
		Data: ExportPayload{
			ReadLaterItems: []ReadLaterItemExport{{
				URL: "https://keep-unread.example", Title: "Keep", State: "archived",
			}},
		},
	}
	rec := e.send(t, http.MethodPost, "/api/v1/backup/import",
		ImportRequest{Mode: "merge", Backup: backup}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("merge = %d", rec.Code)
	}

	var rl struct {
		Items []struct {
			State string `json:"state"`
		} `json:"items"`
	}
	rec2 := e.get(t, "/api/v1/read-later")
	rl = data[struct {
		Items []struct {
			State string `json:"state"`
		} `json:"items"`
	}](t, rec2)
	if len(rl.Items) != 1 || rl.Items[0].State != "unread" {
		t.Fatalf("merge should not archive existing read-later: state=%q", rl.Items[0].State)
	}
}

func TestDuplicateURLInBackupRejected(t *testing.T) {
	e := newTestEnv(t)
	e.loginOwner(t)

	backup := ExportData{
		Version: Version,
		Data: ExportPayload{
			Bookmarks: []BookmarkExport{
				{URL: "https://dup.example", Title: "First", Status: "active"},
				{URL: "https://dup.example", Title: "Second", Status: "active"},
			},
		},
	}
	rec := e.send(t, http.MethodPost, "/api/v1/backup/import",
		ImportRequest{Mode: "merge", Backup: backup}, true)

	summary := data[ImportSummary](t, rec)
	found := false
	for _, err := range summary.Errors {
		if strings.Contains(err, "duplicate bookmark url") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected duplicate url error, got %v", summary.Errors)
	}
	// Verify nothing was written.
	var bms struct{ Total int }
	rec2 := e.get(t, "/api/v1/bookmarks")
	bms = data[struct{ Total int }](t, rec2)
	if bms.Total != 0 {
		t.Fatalf("nothing should be written on duplicate error, got %d", bms.Total)
	}
}

func TestReplacePreservesTrashAndTimestamps(t *testing.T) {
	e := newTestEnv(t)
	e.loginOwner(t)

	// Create an item, trash it, export.
	e.post(t, "/api/v1/bookmarks", map[string]any{
		"url": "https://trash-restore.example", "title": "Trashed",
	})
	rec := e.send(t, http.MethodDelete, "/api/v1/bookmarks/1", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("trash = %d", rec.Code)
	}

	// Export.
	export := data[ExportData](t, e.get(t, "/api/v1/backup/export"))
	if len(export.Data.Bookmarks) != 1 {
		t.Fatalf("export bookmarks = %d", len(export.Data.Bookmarks))
	}
	trashBM := export.Data.Bookmarks[0]
	if trashBM.Status != "trash" {
		t.Fatalf("exported status = %q, want trash", trashBM.Status)
	}
	// Verify createdAt is RFC3339.
	if trashBM.CreatedAt != "" && !strings.Contains(trashBM.CreatedAt, "T") {
		t.Fatalf("createdAt should be RFC3339, got %q", trashBM.CreatedAt)
	}

	// Replace with the exported backup.
	rec2 := e.send(t, http.MethodPost, "/api/v1/backup/import",
		ImportRequest{Mode: "replace", Backup: export}, true)
	if rec2.Code != http.StatusOK {
		t.Fatalf("replace = %d; %s", rec2.Code, rec2.Body.String())
	}

	// Verify the trashed bookmark is restored as trash.
	var bms struct {
		Items []struct {
			Status string `json:"status"`
		} `json:"items"`
		Total int `json:"total"`
	}
	rec3 := e.get(t, "/api/v1/bookmarks?status=trash")
	bms = data[struct {
		Items []struct {
			Status string `json:"status"`
		} `json:"items"`
		Total int `json:"total"`
	}](t, rec3)
	if bms.Total != 1 || bms.Items[0].Status != "trash" {
		t.Fatalf("replace should preserve trash status: total=%d status=%q", bms.Total, bms.Items[0].Status)
	}
}

func TestDuplicateCategoryNameInBackupRejected(t *testing.T) {
	e := newTestEnv(t)
	e.loginOwner(t)

	backup := ExportData{
		Version: Version,
		Data: ExportPayload{
			Categories: []CategoryExport{
				{Name: "Same", Type: "bookmark"},
				{Name: "Same", Type: "read_later"},
			},
		},
	}
	rec := e.send(t, http.MethodPost, "/api/v1/backup/import",
		ImportRequest{Mode: "merge", Backup: backup}, true)

	summary := data[ImportSummary](t, rec)
	found := false
	for _, err := range summary.Errors {
		if strings.Contains(err, "duplicate category name") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected duplicate category name error, got %v", summary.Errors)
	}
}

func TestEmptyCategoryTypeDefaultsToBookmark(t *testing.T) {
	e := newTestEnv(t)
	e.loginOwner(t)

	backup := ExportData{
		Version: Version,
		Data: ExportPayload{
			Categories: []CategoryExport{{Name: "Inbox", Type: ""}},
		},
	}
	rec := e.send(t, http.MethodPost, "/api/v1/backup/import",
		ImportRequest{Mode: "replace", Backup: backup}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("import = %d; %s", rec.Code, rec.Body.String())
	}
	var cats []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	rec2 := e.get(t, "/api/v1/categories")
	cats = data[[]struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}](t, rec2)
	if len(cats) != 1 || cats[0].Type != "bookmark" {
		t.Fatalf("expected type=bookmark, got type=%q", cats[0].Type)
	}
}

func TestEnumWithSpacesIsNormalized(t *testing.T) {
	e := newTestEnv(t)
	e.loginOwner(t)

	backup := ExportData{
		Version: Version,
		Data: ExportPayload{
			Bookmarks: []BookmarkExport{{
				URL: "https://space.example", Title: "Space", Status: " active ",
			}},
			ReadLaterItems: []ReadLaterItemExport{{
				URL: "https://space-rl.example", Title: "SpaceRL", State: " reading ",
			}},
		},
	}
	rec := e.send(t, http.MethodPost, "/api/v1/backup/import",
		ImportRequest{Mode: "replace", Backup: backup}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("import = %d; %s", rec.Code, rec.Body.String())
	}

	var bms struct {
		Items []struct {
			Status string `json:"status"`
		} `json:"items"`
	}
	rec2 := e.get(t, "/api/v1/bookmarks")
	bms = data[struct {
		Items []struct {
			Status string `json:"status"`
		} `json:"items"`
	}](t, rec2)
	if len(bms.Items) != 1 || bms.Items[0].Status != "active" {
		t.Fatalf("expected status=active, got %q", bms.Items[0].Status)
	}

	var rl struct {
		Items []struct {
			State string `json:"state"`
		} `json:"items"`
	}
	rec3 := e.get(t, "/api/v1/read-later")
	rl = data[struct {
		Items []struct {
			State string `json:"state"`
		} `json:"items"`
	}](t, rec3)
	if len(rl.Items) != 1 || rl.Items[0].State != "reading" {
		t.Fatalf("expected state=reading, got %q", rl.Items[0].State)
	}
}
