package auth

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/linyusheng/homepage/internal/csrf"
	"github.com/linyusheng/homepage/internal/db"
	"github.com/linyusheng/homepage/internal/httpx"
	"github.com/linyusheng/homepage/internal/ratelimit"
	"github.com/linyusheng/homepage/internal/security"
)

type testEnv struct {
	mux   *http.ServeMux
	svc   *Service
	store Store
	db    *sql.DB
}

func newTestEnv(t *testing.T, limiter ratelimit.Limiter, resolver ClientIPResolver) *testEnv {
	t.Helper()
	if resolver == nil {
		r, err := httpx.NewClientIPResolver(nil)
		if err != nil {
			t.Fatalf("untrusted resolver: %v", err)
		}
		resolver = r
	}
	database, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := db.Migrate(context.Background(), database); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	store := NewStore(database)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(Config{
		SessionTTL:     time.Hour,
		BcryptCost:     4,
		CookieName:     "homepage_session",
		CookieSecure:   false,
		CookieSameSite: "lax",
		SetupEnabled:   true,
	}, store, limiter, resolver, logger)
	svc.SetCSRFManager(csrf.New(csrf.Config{
		CookieName:        "homepage_csrf",
		HeaderName:        "X-CSRF-Token",
		SessionCookieName: "homepage_session",
		CookieSecure:      false,
		CookieSameSite:    "lax",
		TTL:               time.Hour,
	}))
	mux := http.NewServeMux()
	Register(mux, svc)
	return &testEnv{mux: mux, svc: svc, store: store, db: database}
}

func defaultLimiter() ratelimit.Limiter {
	return ratelimit.New(context.Background(), 5, time.Minute)
}

type envelope struct {
	Data  json.RawMessage `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	RequestID string `json:"requestId"`
}

func (e envelope) data(t *testing.T, v any) {
	t.Helper()
	if e.Data == nil {
		t.Fatalf("expected data, got nil envelope data; error=%+v", e.Error)
	}
	if err := json.Unmarshal(e.Data, v); err != nil {
		t.Fatalf("unmarshal data %q: %v", string(e.Data), err)
	}
}

func (e *testEnv) do(t *testing.T, method, path string, body any, cookies ...*http.Cookie) (*httptest.ResponseRecorder, envelope) {
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
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, req)
	var env envelope
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &env)
	}
	return rec, env
}

func (e *testEnv) setupOwner(t *testing.T, password string) (*httptest.ResponseRecorder, envelope) {
	t.Helper()
	rec, env := e.do(t, http.MethodPost, "/api/v1/auth/setup", map[string]string{
		"password":        password,
		"confirmPassword": password,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup: status %d, body %s", rec.Code, rec.Body.String())
	}
	return rec, env
}

func (e *testEnv) login(t *testing.T, password string) (*httptest.ResponseRecorder, envelope) {
	t.Helper()
	return e.do(t, http.MethodPost, "/api/v1/auth/login", map[string]string{"password": password})
}

func sessionCookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == "homepage_session" {
			return c
		}
	}
	t.Fatalf("no homepage_session cookie in response")
	return nil
}

func sessionCookieOrNil(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == "homepage_session" {
			return c
		}
	}
	return nil
}

func csrfCookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == "homepage_csrf" {
			return c
		}
	}
	t.Fatalf("no homepage_csrf cookie in response")
	return nil
}

func csrfCookieOrNil(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == "homepage_csrf" {
			return c
		}
	}
	return nil
}

func (e *testEnv) doRaw(t *testing.T, method, path string, body []byte) (*httptest.ResponseRecorder, envelope) {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, req)
	var env envelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	return rec, env
}

func TestSetupCreatesOwnerPassword(t *testing.T) {
	e := newTestEnv(t, defaultLimiter(), nil)
	rec, env := e.setupOwner(t, "StrongPass1!")

	var u UserDTO
	env.data(t, &u)
	if u.ID == 0 || u.CreatedAt == "" {
		t.Fatalf("expected id and createdAt, got %+v", u)
	}
	if c := csrfCookie(t, rec); c.Value == "" || c.HttpOnly {
		t.Fatalf("setup must issue readable csrf cookie, got %+v", c)
	}
}

func TestSetupDisabled(t *testing.T) {
	e := newTestEnv(t, defaultLimiter(), nil)
	e.svc.cfg.SetupEnabled = false
	rec, env := e.do(t, http.MethodPost, "/api/v1/auth/setup", map[string]string{
		"password": "StrongPass1!", "confirmPassword": "StrongPass1!",
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("disabled setup: status = %d, want 403", rec.Code)
	}
	if env.Error == nil || env.Error.Code != "SETUP_DISABLED" {
		t.Fatalf("expected SETUP_DISABLED, got %+v", env.Error)
	}
}

func TestSetupRequiresTokenWhenConfigured(t *testing.T) {
	e := newTestEnv(t, defaultLimiter(), nil)
	e.svc.cfg.SetupToken = "setup-token-123456"

	rec, env := e.do(t, http.MethodPost, "/api/v1/auth/setup", map[string]string{
		"password": "StrongPass1!", "confirmPassword": "StrongPass1!", "setupToken": "wrong",
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("wrong setup token: status = %d, want 403", rec.Code)
	}
	if env.Error == nil || env.Error.Code != "SETUP_TOKEN_INVALID" {
		t.Fatalf("expected SETUP_TOKEN_INVALID, got %+v", env.Error)
	}

	rec, _ = e.do(t, http.MethodPost, "/api/v1/auth/setup", map[string]string{
		"password": "StrongPass1!", "confirmPassword": "StrongPass1!", "setupToken": "setup-token-123456",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("valid setup token: status = %d, want 201; body %s", rec.Code, rec.Body.String())
	}
}

func TestAuthStatusReportsInitialization(t *testing.T) {
	e := newTestEnv(t, defaultLimiter(), nil)
	e.svc.cfg.SetupToken = "setup-token-123456"

	rec, env := e.do(t, http.MethodGet, "/api/v1/auth/status", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status before setup = %d, want 200", rec.Code)
	}
	var before statusResponse
	env.data(t, &before)
	if before.Initialized || !before.SetupEnabled || !before.SetupTokenRequired {
		t.Fatalf("unexpected auth status before setup: %+v", before)
	}

	rec, _ = e.do(t, http.MethodPost, "/api/v1/auth/setup", map[string]string{
		"password": "StrongPass1!", "confirmPassword": "StrongPass1!", "setupToken": "setup-token-123456",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup with token: status = %d, want 201; body %s", rec.Code, rec.Body.String())
	}
	rec, env = e.do(t, http.MethodGet, "/api/v1/auth/status", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status after setup = %d, want 200", rec.Code)
	}
	var after statusResponse
	env.data(t, &after)
	if !after.Initialized {
		t.Fatalf("unexpected auth status after setup: %+v", after)
	}
}

func TestSetupOnlyOnce(t *testing.T) {
	e := newTestEnv(t, defaultLimiter(), nil)
	e.setupOwner(t, "StrongPass1!")
	rec, env := e.do(t, http.MethodPost, "/api/v1/auth/setup", map[string]string{
		"password": "AnotherPass1!", "confirmPassword": "AnotherPass1!",
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("second setup: status = %d, want 409", rec.Code)
	}
	if env.Error == nil || env.Error.Code != "SETUP_DISABLED" {
		t.Fatalf("expected SETUP_DISABLED, got %+v", env.Error)
	}
}

func TestSetupValidation(t *testing.T) {
	e := newTestEnv(t, defaultLimiter(), nil)
	cases := []struct {
		name string
		body any
	}{
		{"short password", map[string]string{"password": "short", "confirmPassword": "short"}},
		{"weak password", map[string]string{"password": "alllowercasepassword", "confirmPassword": "alllowercasepassword"}},
		{"missing confirm", map[string]string{"password": "StrongPass1!"}},
		{"mismatched confirm", map[string]string{"password": "StrongPass1!", "confirmPassword": "StrongPass2!"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec, _ := e.do(t, http.MethodPost, "/api/v1/auth/setup", c.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body %s", rec.Code, rec.Body.String())
			}
		})
	}

	rec, _ := e.doRaw(t, http.MethodPost, "/api/v1/auth/setup", []byte("{not json"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed body: status = %d, want 400", rec.Code)
	}
}

func TestLoginSuccessSetsCookie(t *testing.T) {
	e := newTestEnv(t, defaultLimiter(), nil)
	e.setupOwner(t, "StrongPass1!")

	rec, env := e.login(t, "StrongPass1!")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	c := sessionCookie(t, rec)
	if c.Value == "" {
		t.Fatal("empty session cookie value")
	}
	if !c.HttpOnly {
		t.Error("cookie must be HttpOnly")
	}
	if c.Path != "/" {
		t.Errorf("cookie Path = %q, want /", c.Path)
	}
	if c.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite = %v, want Lax", c.SameSite)
	}
	csrfCookie := csrfCookie(t, rec)
	if csrfCookie.Value == "" {
		t.Fatal("empty csrf cookie value")
	}
	if csrfCookie.HttpOnly {
		t.Fatal("csrf cookie must not be HttpOnly")
	}
	if bytes.Contains(rec.Body.Bytes(), []byte(c.Value)) {
		t.Fatal("response body must not contain the session token")
	}
	var u UserDTO
	env.data(t, &u)
	if u.ID == 0 {
		t.Fatalf("login body user = %+v", u)
	}
}

func TestLoginFailureIsUniform(t *testing.T) {
	e := newTestEnv(t, defaultLimiter(), nil)
	e.setupOwner(t, "StrongPass1!")
	recWrong, envWrong := e.login(t, "WrongPass1!")
	recMissingOwner, envMissingOwner := newTestEnv(t, defaultLimiter(), nil).login(t, "WhateverPass1!")

	for _, rec := range []*httptest.ResponseRecorder{recWrong, recMissingOwner} {
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
	}
	if envWrong.Error == nil || envMissingOwner.Error == nil {
		t.Fatal("expected error envelope")
	}
	if envWrong.Error.Code != envMissingOwner.Error.Code ||
		envWrong.Error.Message != envMissingOwner.Error.Message {
		t.Fatalf("login errors differ:\n wrong=%+v\n missing=%+v", envWrong.Error, envMissingOwner.Error)
	}
	if envWrong.Error.Message != "invalid password" {
		t.Fatalf("unexpected message: %q", envWrong.Error.Message)
	}
	if c := sessionCookieOrNil(recWrong); c != nil && c.Value != "" {
		t.Fatal("must not set a session cookie on failed login")
	}
}

func TestMeUnauthenticated(t *testing.T) {
	e := newTestEnv(t, defaultLimiter(), nil)
	rec, env := e.do(t, http.MethodGet, "/api/v1/auth/me", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if env.Error == nil || env.Error.Code != "UNAUTHORIZED" {
		t.Fatalf("expected UNAUTHORIZED, got %+v", env.Error)
	}
}

func TestMeAuthenticated(t *testing.T) {
	e := newTestEnv(t, defaultLimiter(), nil)
	e.setupOwner(t, "StrongPass1!")
	rec, _ := e.login(t, "StrongPass1!")
	c := sessionCookie(t, rec)

	rec2, env := e.do(t, http.MethodGet, "/api/v1/auth/me", nil, c)
	if rec2.Code != http.StatusOK {
		t.Fatalf("me: status = %d, want 200; body %s", rec2.Code, rec2.Body.String())
	}
	var u UserDTO
	env.data(t, &u)
	if u.ID == 0 {
		t.Fatalf("me returned %+v", u)
	}
}

func TestLogoutInvalidatesSession(t *testing.T) {
	e := newTestEnv(t, defaultLimiter(), nil)
	e.setupOwner(t, "StrongPass1!")
	loginRec, _ := e.login(t, "StrongPass1!")
	c := sessionCookie(t, loginRec)

	if rec, _ := e.do(t, http.MethodGet, "/api/v1/auth/me", nil, c); rec.Code != http.StatusOK {
		t.Fatalf("me before logout: status = %d, want 200", rec.Code)
	}

	rec, _ := e.do(t, http.MethodPost, "/api/v1/auth/logout", nil, c)
	if rec.Code != http.StatusOK {
		t.Fatalf("logout: status = %d, want 200", rec.Code)
	}
	if cleared := sessionCookieOrNil(rec); cleared == nil || cleared.MaxAge != -1 {
		t.Fatalf("logout must clear cookie, got %+v", cleared)
	}
	if cleared := csrfCookieOrNil(rec); cleared == nil || cleared.MaxAge != -1 {
		t.Fatalf("logout must clear csrf cookie, got %+v", cleared)
	}
	if rec, _ := e.do(t, http.MethodGet, "/api/v1/auth/me", nil, c); rec.Code != http.StatusUnauthorized {
		t.Fatalf("me after logout: status = %d, want 401", rec.Code)
	}
}

func TestLogoutIdempotentWithoutCookie(t *testing.T) {
	e := newTestEnv(t, defaultLimiter(), nil)
	rec, _ := e.do(t, http.MethodPost, "/api/v1/auth/logout", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("logout without cookie: status = %d, want 200", rec.Code)
	}
}

func TestResolveSessionRejectsExpired(t *testing.T) {
	e := newTestEnv(t, defaultLimiter(), nil)
	e.setupOwner(t, "StrongPass1!")
	token, err := security.GenerateToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	_, err = e.db.Exec(`INSERT INTO sessions (user_id, token_hash, expires_at)
		VALUES (1, ?, ?)`, security.HashToken(token), time.Now().Add(-time.Hour).UTC().Format(dbTimeFormat))
	if err != nil {
		t.Fatalf("insert expired session: %v", err)
	}
	if _, err := e.svc.ResolveSession(context.Background(), token); err == nil {
		t.Fatal("expected ResolveSession to reject an expired session")
	}
}

func TestResolveSessionRejectsRevoked(t *testing.T) {
	e := newTestEnv(t, defaultLimiter(), nil)
	e.setupOwner(t, "StrongPass1!")
	loginRec, _ := e.login(t, "StrongPass1!")
	c := sessionCookie(t, loginRec)
	if err := e.svc.Logout(context.Background(), c.Value); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, err := e.svc.ResolveSession(context.Background(), c.Value); err == nil {
		t.Fatal("expected ResolveSession to reject a revoked session")
	}
}

func TestResolveSessionRejectsGarbage(t *testing.T) {
	e := newTestEnv(t, defaultLimiter(), nil)
	if _, err := e.svc.ResolveSession(context.Background(), "not-a-real-token"); err == nil {
		t.Fatal("expected ResolveSession to reject a bogus token")
	}
}

func TestLoginRateLimits(t *testing.T) {
	limiter := ratelimit.New(context.Background(), 3, time.Minute)
	e := newTestEnv(t, limiter, nil)
	e.setupOwner(t, "StrongPass1!")

	for i := 0; i < 3; i++ {
		rec, _ := e.login(t, "wrong")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d, want 401", i+1, rec.Code)
		}
	}
	rec, _ := e.login(t, "wrong")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("throttled attempt: status = %d, want 429", rec.Code)
	}
	if ra := rec.Header().Get("Retry-After"); ra == "" {
		t.Fatal("expected Retry-After header on 429")
	}
	rec2, _ := e.login(t, "StrongPass1!")
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("correct password while blocked: status = %d, want 429", rec2.Code)
	}
}

func TestSuccessfulLoginResetsRateLimit(t *testing.T) {
	limiter := ratelimit.New(context.Background(), 3, time.Minute)
	e := newTestEnv(t, limiter, nil)
	e.setupOwner(t, "StrongPass1!")

	for i := 0; i < 2; i++ {
		if rec, _ := e.login(t, "wrong"); rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d", i+1, rec.Code)
		}
	}
	if rec, _ := e.login(t, "StrongPass1!"); rec.Code != http.StatusOK {
		t.Fatalf("success: status = %d, want 200; body %s", rec.Code, rec.Body.String())
	}
	for i := 0; i < 3; i++ {
		if rec, _ := e.login(t, "wrong"); rec.Code != http.StatusUnauthorized {
			t.Fatalf("post-reset attempt %d: status = %d", i+1, rec.Code)
		}
	}
	if rec, _ := e.login(t, "wrong"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after 3 post-reset failures, got %d", rec.Code)
	}
}

func (e *testEnv) loginWithHeaders(t *testing.T, password string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(map[string]string{"password": password})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, req)
	return rec
}

func TestRateLimitNotBypassedByForgedXFF(t *testing.T) {
	limiter := ratelimit.New(context.Background(), 3, time.Minute)
	e := newTestEnv(t, limiter, nil)
	e.setupOwner(t, "StrongPass1!")

	forged := []string{"203.0.113.10", "203.0.113.11", "203.0.113.12"}
	for i, ip := range forged {
		rec := e.loginWithHeaders(t, "wrong", map[string]string{"X-Forwarded-For": ip})
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("forged attempt %d: status = %d, want 401", i+1, rec.Code)
		}
	}
	rec := e.loginWithHeaders(t, "wrong", map[string]string{"X-Forwarded-For": "203.0.113.99"})
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("forged XFF bypassed rate limiting: status = %d, want 429", rec.Code)
	}
}

func TestTrustedProxyHonorsXFF(t *testing.T) {
	resolver, err := httpx.NewClientIPResolver([]string{"192.0.2.0/24"})
	if err != nil {
		t.Fatalf("resolver: %v", err)
	}
	limiter := ratelimit.New(context.Background(), 3, time.Minute)
	e := newTestEnv(t, limiter, resolver)
	e.setupOwner(t, "StrongPass1!")

	for i := 0; i < 10; i++ {
		ip := "203.0.113." + strconv.Itoa(i+1)
		rec := e.loginWithHeaders(t, "wrong", map[string]string{"X-Forwarded-For": ip})
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("distinct-ip attempt %d: status = %d, want 401", i, rec.Code)
		}
	}
}
