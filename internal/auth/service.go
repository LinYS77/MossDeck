package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/linyusheng/homepage/internal/ratelimit"
	"github.com/linyusheng/homepage/internal/security"
)

// Config holds the auth-relevant configuration. It is a focused subset of the
// app config so the auth package stays independent of unrelated settings and
// is cheap to construct in tests.
type Config struct {
	SessionTTL     time.Duration
	BcryptCost     int
	CookieName     string
	CookieSecure   bool
	CookieSameSite string
	SetupEnabled   bool
}

// CSRFManager is the narrow auth-side contract for issuing and clearing the
// non-HttpOnly CSRF cookie alongside session lifecycle changes. The concrete
// implementation lives outside auth so this package stays focused on identity.
type CSRFManager interface {
	Issue(w http.ResponseWriter) (string, error)
	Clear(w http.ResponseWriter)
}

// ClientIPResolver resolves the real client IP of a request, honouring
// forwarded headers only when the sender is a trusted reverse proxy. It is
// a consumer-side interface: the concrete *httpx.ClientIPResolver supplied
// by the wiring layer satisfies it, and tests may pass a fake.
type ClientIPResolver interface {
	ClientIP(r *http.Request) string
}

// dummyHash is a precomputed, valid bcrypt hash of a throwaway value. It is
// used to keep login timing roughly constant whether or not the username
// exists: comparing the supplied password against it burns the same bcrypt
// work as a real check without revealing anything about stored users. It is
// never accepted as a real credential because no user row stores it.
var dummyHash = "$2a$12$GT2EAuTzpgFysd0zmDORC.6RfmeLplGXgjQXUnVPYtiVSW72J8fgC"

// Service implements the auth business logic. It depends on a Store (persistence)
// and a Limiter (rate limiting), both injected so tests can substitute fakes.
type Service struct {
	store    Store
	cfg      Config
	cookie   cookieSettings
	limiter  ratelimit.Limiter
	resolver ClientIPResolver
	csrf     CSRFManager
	logger   *slog.Logger
}

// NewService wires the service together. The limiter and resolver are injected
// (rather than created here) so the wiring layer owns their lifecycle and
// tests can pass deterministic fakes.
func NewService(cfg Config, store Store, limiter ratelimit.Limiter, resolver ClientIPResolver, logger *slog.Logger) *Service {
	return &Service{
		store:    store,
		cfg:      cfg,
		cookie:   cookieSettings{Name: cfg.CookieName, Secure: cfg.CookieSecure, SameSite: cfg.CookieSameSite, TTL: cfg.SessionTTL},
		limiter:  limiter,
		resolver: resolver,
		logger:   logger,
	}
}

// SetCSRFManager attaches the CSRF cookie manager used by HTTP handlers to
// issue/clear the readable double-submit cookie after auth state changes.
func (s *Service) SetCSRFManager(manager CSRFManager) {
	s.csrf = manager
}

// Setup creates the first admin user. It fails when setup is disabled or once
// any user already exists, making initialization an explicit, one-time action.
func (s *Service) Setup(ctx context.Context, p SetupParams) (*User, error) {
	if !s.cfg.SetupEnabled {
		return nil, ErrSetupDisabled
	}
	if err := validateSetup(p); err != nil {
		return nil, err
	}
	n, err := s.store.CountUsers(ctx)
	if err != nil {
		return nil, err
	}
	if n > 0 {
		return nil, ErrAlreadyInitialized
	}
	hash, err := security.HashPassword(p.Password, s.cfg.BcryptCost)
	if err != nil {
		return nil, err
	}
	user, err := s.store.CreateUser(ctx, CreateUserParams{
		Username:     strings.TrimSpace(p.Username),
		Email:        strings.TrimSpace(p.Email),
		PasswordHash: hash,
		DisplayName:  strings.TrimSpace(p.DisplayName),
		Role:         "admin",
		Status:       "active",
	})
	if err != nil {
		return nil, err
	}
	s.logger.Info("admin initialized", "user_id", user.ID, "username", user.Username)
	return user, nil
}

// Login authenticates credentials and, on success, creates a session. It
// returns the user and the raw session token (to be placed in the cookie).
//
// To avoid revealing whether a username exists, the not-found path performs a
// dummy bcrypt comparison so its timing resembles a real check. All
// credential failures are reported as the single ErrInvalidCredentials
// (inactive users map to ErrUserInactive but handlers render both as the same
// generic 401).
func (s *Service) Login(ctx context.Context, username, password, userAgent, ip string) (*User, string, error) {
	user, err := s.store.GetUserByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			_ = security.CheckPassword(dummyHash, password) // flatten timing
			return nil, "", ErrInvalidCredentials
		}
		return nil, "", err
	}
	if err := security.CheckPassword(user.PasswordHash, password); err != nil {
		return nil, "", ErrInvalidCredentials
	}
	if user.Status != "active" {
		return nil, "", ErrUserInactive
	}

	token, err := security.GenerateToken()
	if err != nil {
		return nil, "", err
	}
	expiresAt := time.Now().Add(s.cfg.SessionTTL).UTC().Format(dbTimeFormat)
	if err := s.store.CreateSession(ctx, CreateSessionParams{
		UserID:    user.ID,
		TokenHash: security.HashToken(token),
		ExpiresAt: expiresAt,
		UserAgent: truncate(userAgent, 255),
		IPAddress: ip,
	}); err != nil {
		return nil, "", err
	}
	// A failed last-login update must not invalidate an otherwise-successful
	// login; log and continue.
	if err := s.store.TouchLogin(ctx, user.ID, expiresAt); err != nil {
		s.logger.Warn("update last_login_at failed", "user_id", user.ID, "error", err)
	}
	return user, token, nil
}

// ResolveSession validates a raw session token (as received from the cookie)
// and returns the associated active user, or ErrSessionInvalid. It is the
// single entry point used by the auth middleware.
func (s *Service) ResolveSession(ctx context.Context, token string) (*User, error) {
	if token == "" {
		return nil, ErrSessionInvalid
	}
	sess, err := s.store.GetSessionByTokenHash(ctx, security.HashToken(token))
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			return nil, ErrSessionInvalid
		}
		return nil, err
	}
	user, err := s.store.GetUserByID(ctx, sess.UserID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return nil, ErrSessionInvalid
		}
		return nil, err
	}
	if user.Status != "active" {
		return nil, ErrSessionInvalid
	}
	return user, nil
}

// Logout revokes the session backing token. It is idempotent: a missing or
// already-invalid token is a no-op rather than an error, so the endpoint is
// safe to call repeatedly.
func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.store.RevokeSessionByTokenHash(ctx, security.HashToken(token))
}

// validateSetup checks setup input and returns the first problem as a
// ValidationError, or nil.
func validateSetup(p SetupParams) error {
	username := strings.TrimSpace(p.Username)
	switch {
	case len(username) < 3 || len(username) > 32:
		return ValidationError{Field: "username", Reason: "must be 3-32 characters"}
	case !validUsername(username):
		return ValidationError{Field: "username", Reason: "may only contain letters, digits, '.', '-' and '_'"}
	}
	if len(p.Password) < security.MinPasswordLength {
		return ValidationError{
			Field:  "password",
			Reason: fmt.Sprintf("must be at least %d characters", security.MinPasswordLength),
		}
	}
	if email := strings.TrimSpace(p.Email); email != "" && !validEmail(email) {
		return ValidationError{Field: "email", Reason: "invalid email format"}
	}
	return nil
}

// validUsername checks the allowed character set for usernames.
func validUsername(s string) bool {
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '.', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

// validEmail is a deliberately lightweight sanity check; it does not attempt
// full RFC validation. Empty emails are allowed by the caller.
func validEmail(s string) bool {
	at := strings.IndexByte(s, '@')
	if at <= 0 || at == len(s)-1 {
		return false
	}
	rest := s[at+1:]
	return strings.Contains(rest, ".")
}

// truncate clips s to at most n UTF-8 runes (for storing bounded metadata).
func truncate(s string, n int) string {
	if n <= 0 || len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
