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

// testEnv wires a real in-memory SQLite store, a configurable rate limiter,
// and the auth Service/handlers for integration testing. The bcrypt cost is
// kept at the minimum (4) so password hashing is fast in tests.
type testEnv struct {
	mux    *http.ServeMux
	svc    *Service
	store  Store
	db     *sql.DB
	cookie *http.Cookie // set after a successful login helper call
}

// newTestEnv wires a real in-memory SQLite store, a configurable rate limiter
// and client-IP resolver, and the auth Service/handlers for integration
// testing. The bcrypt cost is kept at the minimum (4) so password hashing is
// fast in tests. resolver may be nil, in which case an untrusted resolver
// (no forwarded headers honoured) is used.
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

// defaultLimiter is a generous in-memory limiter for tests that do not focus
// on rate limiting.
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

// setupAdmin performs a first-run setup and returns the resulting session
// cookie for convenience.
func (e *testEnv) setupAdmin(t *testing.T, username, password string) (*httptest.ResponseRecorder, envelope, *http.Cookie) {
	t.Helper()
	rec, env := e.do(t, http.MethodPost, "/api/v1/auth/setup", map[string]string{
		"username":    username,
		"password":    password,
		"displayName": "Admin",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup: status %d, body %s", rec.Code, rec.Body.String())
	}
	return rec, env, nil
}

// login returns the response recorder so callers can read Set-Cookie.
func (e *testEnv) login(t *testing.T, username, password string) (*httptest.ResponseRecorder, envelope) {
	t.Helper()
	return e.do(t, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"username": username,
		"password": password,
	})
}

func sessionCookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	cookies := rec.Result().Cookies()
	for _, c := range cookies {
		if c.Name == "homepage_session" {
			return c
		}
	}
	t.Fatalf("no homepage_session cookie in response; got %d cookies", len(cookies))
	return nil
}

// --- setup ---

func TestSetupCreatesAdmin(t *testing.T) {
	e := newTestEnv(t, defaultLimiter(), nil)

	rec, env, _ := e.setupAdmin(t, "admin", "supersecret")
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d", rec.Code)
	}
	var u UserDTO
	env.data(t, &u)
	if u.Username != "admin" || u.Role != "admin" || u.Status != "active" {
		t.Fatalf("unexpected user DTO: %+v", u)
	}
	if u.ID == 0 || u.CreatedAt == "" {
		t.Fatalf("expected id and createdAt, got %+v", u)
	}
	if u.Email != "" {
		t.Fatalf("expected empty email, got %q", u.Email)
	}
	if c := csrfCookie(t, rec); c.Value == "" || c.HttpOnly {
		t.Fatalf("setup must issue readable csrf cookie, got %+v", c)
	}
}

func TestSetupDisabled(t *testing.T) {
	e := newTestEnv(t, defaultLimiter(), nil)
	e.svc.cfg.SetupEnabled = false

	rec, env := e.do(t, http.MethodPost, "/api/v1/auth/setup", map[string]string{
		"username": "admin", "password": "supersecret",
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("disabled setup: status = %d, want 403", rec.Code)
	}
	if env.Error == nil || env.Error.Code != "SETUP_DISABLED" {
		t.Fatalf("expected SETUP_DISABLED, got %+v", env.Error)
	}
}

func TestSetupOnlyOnce(t *testing.T) {
	e := newTestEnv(t, defaultLimiter(), nil)
	e.setupAdmin(t, "admin", "supersecret")

	// Second setup must be rejected even with valid input.
	rec, env := e.do(t, http.MethodPost, "/api/v1/auth/setup", map[string]string{
		"username": "other", "password": "anothersecret",
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
		{"short username", map[string]string{"username": "ab", "password": "supersecret"}},
		{"short password", map[string]string{"username": "admin", "password": "short"}},
		{"bad email", map[string]string{"username": "admin", "password": "supersecret", "email": "not-an-email"}},
		{"bad chars in username", map[string]string{"username": "bad name!", "password": "supersecret"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec, _ := e.do(t, http.MethodPost, "/api/v1/auth/setup", c.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body %s", rec.Code, rec.Body.String())
			}
		})
	}

	// Malformed JSON body.
	rec, _ := e.doRaw(t, http.MethodPost, "/api/v1/auth/setup", []byte("{not json"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed body: status = %d, want 400", rec.Code)
	}
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

// --- login ---

func TestLoginSuccessSetsCookie(t *testing.T) {
	e := newTestEnv(t, defaultLimiter(), nil)
	e.setupAdmin(t, "admin", "supersecret")

	rec, env := e.login(t, "admin", "supersecret")
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
	if csrfCookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("csrf SameSite = %v, want Lax", csrfCookie.SameSite)
	}
	// The response body must not echo the token.
	if bytes.Contains(rec.Body.Bytes(), []byte(c.Value)) {
		t.Fatal("response body must not contain the session token")
	}
	var u UserDTO
	env.data(t, &u)
	if u.Username != "admin" {
		t.Fatalf("login body user = %+v", u)
	}
}

func TestLoginFailureIsUniform(t *testing.T) {
	e := newTestEnv(t, defaultLimiter(), nil)
	e.setupAdmin(t, "admin", "supersecret")

	// Wrong password (existing user) and unknown user must produce the SAME
	// error code and message so the username is not leaked.
	recWrong, envWrong := e.login(t, "admin", "wrongpassword")
	recUnknown, envUnknown := e.login(t, "ghost", "whatever")

	for _, rec := range []*httptest.ResponseRecorder{recWrong, recUnknown} {
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
	}
	if envWrong.Error == nil || envUnknown.Error == nil {
		t.Fatal("expected error envelope")
	}
	if envWrong.Error.Code != envUnknown.Error.Code ||
		envWrong.Error.Message != envUnknown.Error.Message {
		t.Fatalf("login errors differ, leaking user existence:\n wrong=%+v\n unknown=%+v",
			envWrong.Error, envUnknown.Error)
	}
	if envWrong.Error.Message != "invalid username or password" {
		t.Fatalf("unexpected message: %q", envWrong.Error.Message)
	}
	// No session cookie on failure.
	if c := sessionCookieOrNil(recWrong); c != nil && c.Value != "" {
		t.Fatal("must not set a session cookie on failed login")
	}
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

// --- me ---

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
	e.setupAdmin(t, "admin", "supersecret")
	rec, _ := e.login(t, "admin", "supersecret")
	c := sessionCookie(t, rec)

	rec2, env := e.do(t, http.MethodGet, "/api/v1/auth/me", nil, c)
	if rec2.Code != http.StatusOK {
		t.Fatalf("me: status = %d, want 200; body %s", rec2.Code, rec2.Body.String())
	}
	var u UserDTO
	env.data(t, &u)
	if u.Username != "admin" || u.ID == 0 {
		t.Fatalf("me returned %+v", u)
	}
}

// --- logout ---

func TestLogoutInvalidatesSession(t *testing.T) {
	e := newTestEnv(t, defaultLimiter(), nil)
	e.setupAdmin(t, "admin", "supersecret")
	loginRec, _ := e.login(t, "admin", "supersecret")
	c := sessionCookie(t, loginRec)

	// Authenticated before logout.
	if rec, _ := e.do(t, http.MethodGet, "/api/v1/auth/me", nil, c); rec.Code != http.StatusOK {
		t.Fatalf("me before logout: status = %d, want 200", rec.Code)
	}

	// Logout.
	rec, _ := e.do(t, http.MethodPost, "/api/v1/auth/logout", nil, c)
	if rec.Code != http.StatusOK {
		t.Fatalf("logout: status = %d, want 200", rec.Code)
	}
	// The logout response clears the cookie.
	if cleared := sessionCookieOrNil(rec); cleared == nil || cleared.MaxAge != -1 {
		t.Fatalf("logout must clear cookie, got %+v", cleared)
	}
	if cleared := csrfCookieOrNil(rec); cleared == nil || cleared.MaxAge != -1 {
		t.Fatalf("logout must clear csrf cookie, got %+v", cleared)
	}

	// The same cookie must no longer authenticate (session revoked in store).
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

// --- sessions: revoked vs expired ---

func TestResolveSessionRejectsExpired(t *testing.T) {
	e := newTestEnv(t, defaultLimiter(), nil)
	e.setupAdmin(t, "admin", "supersecret")

	token, err := security.GenerateToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	// Insert a session whose expiry is in the past.
	_, err = e.db.Exec(`INSERT INTO sessions (user_id, token_hash, expires_at)
		VALUES (1, ?, ?)`, security.HashToken(token),
		time.Now().Add(-time.Hour).UTC().Format(dbTimeFormat))
	if err != nil {
		t.Fatalf("insert expired session: %v", err)
	}

	if _, err := e.svc.ResolveSession(context.Background(), token); err == nil {
		t.Fatal("expected ResolveSession to reject an expired session")
	}
}

func TestResolveSessionRejectsRevoked(t *testing.T) {
	e := newTestEnv(t, defaultLimiter(), nil)
	e.setupAdmin(t, "admin", "supersecret")
	loginRec, _ := e.login(t, "admin", "supersecret")
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

// --- rate limiting ---

func TestLoginRateLimits(t *testing.T) {
	// Tight limiter: 3 failures allowed, then throttle.
	limiter := ratelimit.New(context.Background(), 3, time.Minute)
	e := newTestEnv(t, limiter, nil)
	e.setupAdmin(t, "admin", "supersecret")

	// Three wrong-password attempts are served (401) and counted.
	for i := 0; i < 3; i++ {
		rec, _ := e.login(t, "admin", "wrong")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d, want 401", i+1, rec.Code)
		}
	}
	// The fourth is throttled before credentials are even checked.
	rec, _ := e.login(t, "admin", "wrong")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("throttled attempt: status = %d, want 429", rec.Code)
	}
	if ra := rec.Header().Get("Retry-After"); ra == "" {
		t.Fatal("expected Retry-After header on 429")
	}

	// Even correct credentials are throttled once blocked.
	rec2, _ := e.login(t, "admin", "supersecret")
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("correct creds while blocked: status = %d, want 429", rec2.Code)
	}
}

func TestSuccessfulLoginResetsRateLimit(t *testing.T) {
	limiter := ratelimit.New(context.Background(), 3, time.Minute)
	e := newTestEnv(t, limiter, nil)
	e.setupAdmin(t, "admin", "supersecret")

	// Two failures (counter = 2, not yet blocked).
	for i := 0; i < 2; i++ {
		if rec, _ := e.login(t, "admin", "wrong"); rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d", i+1, rec.Code)
		}
	}
	// A successful login resets the counter.
	if rec, _ := e.login(t, "admin", "supersecret"); rec.Code != http.StatusOK {
		t.Fatalf("success: status = %d, want 200; body %s", rec.Code, rec.Body.String())
	}
	// Now three more failures should be required to block again (proving reset).
	for i := 0; i < 3; i++ {
		if rec, _ := e.login(t, "admin", "wrong"); rec.Code != http.StatusUnauthorized {
			t.Fatalf("post-reset attempt %d: status = %d", i+1, rec.Code)
		}
	}
	// The next is throttled.
	if rec, _ := e.login(t, "admin", "wrong"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after 3 post-reset failures, got %d", rec.Code)
	}
}

// --- Trusted-proxy boundary (public-safety regression) ---

// loginWithHeaders posts a login with extra request headers, returning the
// recorder so callers can assert status. Used to test forged forwarded IPs.
func (e *testEnv) loginWithHeaders(t *testing.T, username, password string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(map[string]string{"username": username, "password": password})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, req)
	return rec
}

// TestRateLimitNotBypassedByForgedXFF is the security regression for the
// public gap: with NO trusted proxy configured, a client rotating a forged
// X-Forwarded-For must NOT escape per-IP rate limiting. httptest sets the
// request RemoteAddr to 192.0.2.1:1234, so all attempts share that peer
// regardless of the forged header.
func TestRateLimitNotBypassedByForgedXFF(t *testing.T) {
	limiter := ratelimit.New(context.Background(), 3, time.Minute)
	e := newTestEnv(t, limiter, nil) // untrusted resolver
	e.setupAdmin(t, "admin", "supersecret")

	// Three wrong attempts, each forging a DIFFERENT client IP.
	forged := []string{"203.0.113.10", "203.0.113.11", "203.0.113.12"}
	for i, ip := range forged {
		rec := e.loginWithHeaders(t, "admin", "wrong", map[string]string{"X-Forwarded-For": ip})
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("forged attempt %d: status = %d, want 401", i+1, rec.Code)
		}
	}
	// A fourth attempt — with yet another forged IP — must still be throttled,
	// proving the forged headers did not rotate the rate-limit key.
	rec := e.loginWithHeaders(t, "admin", "wrong", map[string]string{"X-Forwarded-For": "203.0.113.99"})
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("forged XFF bypassed rate limiting: status = %d, want 429", rec.Code)
	}
}

// TestTrustedProxyHonorsXFF confirms the opposite policy: when the reverse
// proxy network IS trusted and the request peer is inside it, forwarded
// headers are honoured (so distinct real clients get distinct rate-limit
// buckets). This is the intended behaviour behind a correctly configured proxy.
func TestTrustedProxyHonorsXFF(t *testing.T) {
	// httptest default peer is 192.0.2.1:1234; trust that host's /24.
	resolver, err := httpx.NewClientIPResolver([]string{"192.0.2.0/24"})
	if err != nil {
		t.Fatalf("resolver: %v", err)
	}
	limiter := ratelimit.New(context.Background(), 3, time.Minute)
	e := newTestEnv(t, limiter, resolver)
	e.setupAdmin(t, "admin", "supersecret")

	// Many wrong attempts, each from a DISTINCT forged (trusted) client IP.
	// Because the resolver now trusts XFF, each lands in its own bucket and
	// none should trigger the throttle.
	for i := 0; i < 10; i++ {
		ip := "203.0.113." + strconv.Itoa(i+1)
		rec := e.loginWithHeaders(t, "admin", "wrong", map[string]string{"X-Forwarded-For": ip})
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("distinct-ip attempt %d: status = %d, want 401 (each IP has its own bucket)", i, rec.Code)
		}
	}
}
