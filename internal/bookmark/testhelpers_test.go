package bookmark

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/linyusheng/homepage/internal/security"
)

// createUser inserts an internal owner row directly so isolation tests can
// verify every query remains scoped by user_id. The product has no public
// multi-user account surface.
func createUser(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	hash, err := security.HashPassword("StrongPass1!", 4)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	res, err := db.Exec(`INSERT INTO users (password_hash) VALUES (?)`, hash)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func sessionForUser(t *testing.T, db *sql.DB, userID int64) *http.Cookie {
	t.Helper()
	token, err := security.GenerateToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	expiresAt := time.Now().Add(time.Hour).UTC().Format("2006-01-02 15:04:05")
	_, err = db.Exec(`INSERT INTO sessions (user_id, token_hash, expires_at) VALUES (?, ?, ?)`,
		userID, security.HashToken(token), expiresAt)
	if err != nil {
		t.Fatalf("insert session: %v", err)
	}
	return &http.Cookie{Name: "homepage_session", Value: token, Path: "/"}
}

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
