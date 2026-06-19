package csrf

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestIssueWritesReadableCookie(t *testing.T) {
	manager := New(Config{CookieName: "csrf", HeaderName: "X-Test-CSRF", CookieSecure: true, CookieSameSite: "strict", TTL: time.Hour})
	rec := httptest.NewRecorder()
	token, err := manager.Issue(rec)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if token == "" {
		t.Fatal("empty token")
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies=%d, want 1", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != "csrf" || cookie.Value != token {
		t.Fatalf("cookie = %+v, token=%q", cookie, token)
	}
	if cookie.HttpOnly {
		t.Fatal("csrf cookie must be readable by frontend JavaScript")
	}
	if !cookie.Secure {
		t.Fatal("secure flag not applied")
	}
	if cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("SameSite=%v, want Strict", cookie.SameSite)
	}
}

func TestMiddlewareAllowsSafeMethodsWithoutToken(t *testing.T) {
	manager := New(Config{CookieName: "csrf", HeaderName: "X-CSRF-Token", SessionCookieName: "session", TTL: time.Hour})
	hit := false
	handler := manager.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusNoContent || !hit {
		t.Fatalf("safe request status=%d hit=%v", res.Code, hit)
	}
}

func TestMiddlewareOnlyProtectsCookieAuthenticatedUnsafeRequests(t *testing.T) {
	manager := New(Config{CookieName: "csrf", HeaderName: "X-CSRF-Token", SessionCookieName: "session", TTL: time.Hour})
	handler := manager.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/bookmarks", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusNoContent {
		t.Fatalf("unsafe without session should pass to auth layer, got %d", res.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/bookmarks", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "s"})
	res = httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("unsafe with session and no csrf = %d, want 403", res.Code)
	}
}

func TestMiddlewareRejectsMismatchAndAcceptsMatch(t *testing.T) {
	manager := New(Config{CookieName: "csrf", HeaderName: "X-CSRF-Token", SessionCookieName: "session", TTL: time.Hour})
	handler := manager.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/bookmarks/1", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "s"})
	req.AddCookie(&http.Cookie{Name: "csrf", Value: "abc"})
	req.Header.Set("X-CSRF-Token", "def")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("mismatch = %d, want 403", res.Code)
	}

	req = httptest.NewRequest(http.MethodPatch, "/api/v1/bookmarks/1", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "s"})
	req.AddCookie(&http.Cookie{Name: "csrf", Value: "abc"})
	req.Header.Set("X-CSRF-Token", "abc")
	res = httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusNoContent {
		t.Fatalf("match = %d, want 204; body=%s", res.Code, res.Body.String())
	}
}

func TestSetupAndLoginAreExempt(t *testing.T) {
	manager := New(Config{CookieName: "csrf", HeaderName: "X-CSRF-Token", SessionCookieName: "session", TTL: time.Hour})
	handler := manager.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, path := range []string{"/api/v1/auth/setup", "/api/v1/auth/login"} {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		req.AddCookie(&http.Cookie{Name: "session", Value: "s"})
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusNoContent {
			t.Fatalf("%s = %d, want 204", path, res.Code)
		}
	}
}
