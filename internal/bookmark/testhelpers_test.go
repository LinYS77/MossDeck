package bookmark

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/linyusheng/homepage/internal/security"
)

// createUser inserts a user directly into the DB (bypassing the one-shot setup
// endpoint) so multi-user isolation tests can provision a second identity.
func createUser(t *testing.T, db *sql.DB, username, password string) int64 {
	t.Helper()
	hash, err := security.HashPassword(password, 4)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	res, err := db.Exec(`INSERT INTO users (username, password_hash, role, status) VALUES (?, ?, 'admin', 'active')`,
		username, hash)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// loginAs posts to the login endpoint and returns the resulting session cookie.
func loginAs(t *testing.T, handler http.Handler, username, password string) *http.Cookie {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login as %s: %d %s", username, rec.Code, rec.Body.String())
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == "homepage_session" {
			return c
		}
	}
	t.Fatal("no session cookie after loginAs")
	return nil
}

// secondUserPost performs an authenticated POST as a different user (via their
// session cookie), returning the decoded data field. Used by cross-user tests
// that need to provision resources owned by another identity.
func secondUserPost(t *testing.T, handler http.Handler, cookie *http.Cookie, path string, body any) categoryDTO {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	csrfCookie := csrfCookieFor(t, handler)
	req.AddCookie(csrfCookie)
	req.Header.Set("X-CSRF-Token", csrfCookie.Value)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("second-user POST %s: %d %s", path, rec.Code, rec.Body.String())
	}
	var env envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	var c categoryDTO
	if err := json.Unmarshal(env.Data, &c); err != nil {
		t.Fatalf("unmarshal category %s: %v", string(env.Data), err)
	}
	return c
}

func csrfCookieFor(t *testing.T, handler http.Handler) *http.Cookie {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/csrf", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("csrf endpoint: %d %s", rec.Code, rec.Body.String())
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == "homepage_csrf" {
			return c
		}
	}
	t.Fatal("no csrf cookie from endpoint")
	return nil
}
