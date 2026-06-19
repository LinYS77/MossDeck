package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/linyusheng/homepage/internal/config"
	"github.com/linyusheng/homepage/internal/db"
	"github.com/linyusheng/homepage/internal/server/middleware"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	database, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := db.Migrate(context.Background(), database); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	cfg := config.Config{
		App:              "development",
		HTTPAddr:         ":0",
		ReadTimeout:      5 * time.Second,
		WriteTimeout:     5 * time.Second,
		SessionTTL:       time.Hour,
		BcryptCost:       4, // fast for tests
		CookieName:       "homepage_session",
		CookieSecure:     false,
		CookieSameSite:   "lax",
		CSRFCookieName:   "homepage_csrf",
		CSRFHeaderName:   "X-CSRF-Token",
		SetupEnabled:     true,
		LoginMaxFailures: 5,
		LoginWindow:      time.Minute,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(context.Background(), cfg, database, logger)
}

func do(t *testing.T, s *Server, target string) (*httptest.ResponseRecorder, envelope) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)

	var env envelope
	if rec.Body.Len() > 0 {
		// Some responses (e.g. ServeMux's plain-text 404) are not JSON; that is
		// fine, callers assert on rec.Code in those cases.
		_ = json.Unmarshal(rec.Body.Bytes(), &env)
	}
	return rec, env
}

type envelope struct {
	Data  map[string]any `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	RequestID string `json:"requestId"`
}

func TestHealthEndpoint(t *testing.T) {
	s := newTestServer(t)

	for _, path := range []string{"/api/v1/system/health", "/healthz"} {
		rec, env := do(t, s, path)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200", path, rec.Code)
		}
		if env.Error != nil {
			t.Fatalf("%s: unexpected error %+v", path, env.Error)
		}
		if env.Data["status"] != "ok" {
			t.Fatalf("%s: status field = %v", path, env.Data["status"])
		}
		if env.Data["service"] != "homepage" {
			t.Fatalf("%s: service field = %v", path, env.Data["service"])
		}
		if env.RequestID == "" {
			t.Fatalf("%s: empty requestId in body", path)
		}
		if got := rec.Header().Get("X-Request-ID"); got != env.RequestID {
			t.Fatalf("%s: header request id %q != body %q", path, got, env.RequestID)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
			t.Fatalf("%s: content-type = %q", path, ct)
		}
	}
}

func TestRequestIDHeaderEchoedWhenProvided(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/health", nil)
	req.Header.Set("X-Request-ID", "req_aaaa1111bbbb")
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Request-ID"); got != "req_aaaa1111bbbb" {
		t.Fatalf("expected trusted request id echoed, got %q", got)
	}
}

func TestRequestIDRejectedWhenMalformed(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/health", nil)
	req.Header.Set("X-Request-ID", "bad value with spaces")
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)

	// A malformed inbound id must be replaced, not echoed.
	if got := rec.Header().Get("X-Request-ID"); got == "bad value with spaces" || got == "" {
		t.Fatalf("expected a regenerated id, got %q", got)
	}
}

func TestUnknownRouteReturns404(t *testing.T) {
	s := newTestServer(t)
	rec, _ := do(t, s, "/api/v1/nope")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestRecoverFromPanic(t *testing.T) {
	database, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	mux := http.NewServeMux()
	mux.HandleFunc("GET /boom", func(w http.ResponseWriter, r *http.Request) {
		panic("kaboom")
	})

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	var h http.Handler = mux
	h = middleware.RequestID(h)
	h = middleware.Recover(logger)(h)

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

func doJSON(t *testing.T, s *Server, method, target string, body any, cookies ...*http.Cookie) (*httptest.ResponseRecorder, envelope) {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		r = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, target, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	var env envelope
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &env)
	}
	return rec, env
}

func doJSONWithCSRF(t *testing.T, s *Server, method, target string, body any, csrfToken string, cookies ...*http.Cookie) (*httptest.ResponseRecorder, envelope) {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		r = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, target, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if csrfToken != "" {
		req.Header.Set("X-CSRF-Token", csrfToken)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, req)
	var env envelope
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &env)
	}
	return rec, env
}

func cookieNamed(t *testing.T, rec *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("missing cookie %s", name)
	return nil
}

func setupAndLogin(t *testing.T, s *Server) (*http.Cookie, *http.Cookie) {
	t.Helper()
	rec, _ := doJSON(t, s, http.MethodPost, "/api/v1/auth/setup", map[string]string{"password": "StrongPass1!", "confirmPassword": "StrongPass1!"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup: %d %s", rec.Code, rec.Body.String())
	}
	setupCSRF := cookieNamed(t, rec, "homepage_csrf")
	if setupCSRF.Value == "" || setupCSRF.HttpOnly {
		t.Fatalf("setup csrf cookie invalid: %+v", setupCSRF)
	}
	rec, _ = doJSON(t, s, http.MethodPost, "/api/v1/auth/login", map[string]string{"password": "StrongPass1!"})
	if rec.Code != http.StatusOK {
		t.Fatalf("login: %d %s", rec.Code, rec.Body.String())
	}
	return cookieNamed(t, rec, "homepage_session"), cookieNamed(t, rec, "homepage_csrf")
}

func TestCSRFEndpointIssuesReadableCookie(t *testing.T) {
	s := newTestServer(t)
	rec, env := do(t, s, "/api/v1/auth/csrf")
	if rec.Code != http.StatusOK {
		t.Fatalf("csrf endpoint status=%d body=%s", rec.Code, rec.Body.String())
	}
	cookie := cookieNamed(t, rec, "homepage_csrf")
	if cookie.Value == "" || cookie.HttpOnly {
		t.Fatalf("csrf cookie invalid: %+v", cookie)
	}
	if env.Data["token"] != cookie.Value {
		t.Fatalf("body token %q != cookie %q", env.Data["token"], cookie.Value)
	}
	if env.Data["headerName"] != "X-CSRF-Token" || env.Data["cookieName"] != "homepage_csrf" {
		t.Fatalf("unexpected csrf metadata: %+v", env.Data)
	}
}

func TestCSRFDoesNotBlockSafeMethods(t *testing.T) {
	s := newTestServer(t)
	session, _ := setupAndLogin(t, s)
	rec, env := doJSON(t, s, http.MethodGet, "/api/v1/auth/me", nil, session)
	if rec.Code != http.StatusOK {
		t.Fatalf("me without csrf: status=%d error=%+v body=%s", rec.Code, env.Error, rec.Body.String())
	}
	rec, _ = do(t, s, "/api/v1/system/health")
	if rec.Code != http.StatusOK {
		t.Fatalf("health without csrf: status=%d", rec.Code)
	}
}

func TestCSRFMissingOrMismatchedTokenRejectsLoggedInMutation(t *testing.T) {
	s := newTestServer(t)
	session, csrfCookie := setupAndLogin(t, s)

	rec, env := doJSON(t, s, http.MethodPost, "/api/v1/categories", map[string]string{"name": "Dev"}, session, csrfCookie)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("missing csrf header: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if env.Error == nil || env.Error.Code != "CSRF_INVALID" {
		t.Fatalf("missing csrf error=%+v", env.Error)
	}

	rec, env = doJSONWithCSRF(t, s, http.MethodPost, "/api/v1/categories", map[string]string{"name": "Dev"}, "wrong", session, csrfCookie)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("mismatched csrf: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if env.Error == nil || env.Error.Code != "CSRF_INVALID" {
		t.Fatalf("mismatched csrf error=%+v", env.Error)
	}
}

func TestCSRFCorrectTokenAllowsMutationAndLogout(t *testing.T) {
	s := newTestServer(t)
	session, csrfCookie := setupAndLogin(t, s)

	rec, _ := doJSONWithCSRF(t, s, http.MethodPost, "/api/v1/categories", map[string]string{"name": "Dev"}, csrfCookie.Value, session, csrfCookie)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create category with csrf: status=%d body=%s", rec.Code, rec.Body.String())
	}

	rec, env := doJSON(t, s, http.MethodPost, "/api/v1/auth/logout", nil, session, csrfCookie)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("logout without csrf: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if env.Error == nil || env.Error.Code != "CSRF_INVALID" {
		t.Fatalf("logout csrf error=%+v", env.Error)
	}

	rec, _ = doJSONWithCSRF(t, s, http.MethodPost, "/api/v1/auth/logout", nil, csrfCookie.Value, session, csrfCookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("logout with csrf: status=%d body=%s", rec.Code, rec.Body.String())
	}
	clearedSession := cookieNamed(t, rec, "homepage_session")
	clearedCSRF := cookieNamed(t, rec, "homepage_csrf")
	if clearedSession.MaxAge != -1 || clearedCSRF.MaxAge != -1 {
		t.Fatalf("logout should clear cookies: session=%+v csrf=%+v", clearedSession, clearedCSRF)
	}
}

func TestUnauthenticatedMutationStillReturns401(t *testing.T) {
	s := newTestServer(t)
	rec, env := doJSON(t, s, http.MethodPost, "/api/v1/categories", map[string]string{"name": "Dev"})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401 body=%s", rec.Code, rec.Body.String())
	}
	if env.Error == nil || env.Error.Code != "UNAUTHORIZED" {
		t.Fatalf("error=%+v", env.Error)
	}
}
