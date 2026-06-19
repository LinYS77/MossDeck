package auth

import (
	"context"
	"crypto/subtle"
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
	SetupToken     string
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

// dummyHash is a precomputed, valid bcrypt hash of a throwaway value. It keeps
// login timing roughly constant when the owner password has not been created.
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

// Setup creates the personal owner account. It fails when setup is disabled or
// once any user already exists, making initialization an explicit one-time action.
func (s *Service) Setup(ctx context.Context, p SetupParams) (*User, error) {
	if !s.cfg.SetupEnabled {
		return nil, ErrSetupDisabled
	}
	if err := s.validateSetupToken(p.SetupToken); err != nil {
		return nil, err
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
	user, err := s.store.CreateUser(ctx, CreateUserParams{PasswordHash: hash})
	if err != nil {
		return nil, err
	}
	s.logger.Info("owner initialized", "user_id", user.ID)
	return user, nil
}

// Status reports whether this instance can be initialized from the browser.
func (s *Service) Status(ctx context.Context) (initialized bool, setupEnabled bool, setupTokenRequired bool, err error) {
	n, err := s.store.CountUsers(ctx)
	if err != nil {
		return false, false, false, err
	}
	return n > 0, s.cfg.SetupEnabled, s.cfg.SetupToken != "", nil
}

// Login authenticates the personal owner password and, on success, creates a session.
func (s *Service) Login(ctx context.Context, password, userAgent, ip string) (*User, string, error) {
	user, err := s.store.GetFirstUser(ctx)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			_ = security.CheckPassword(dummyHash, password)
			return nil, "", ErrInvalidCredentials
		}
		return nil, "", err
	}
	return s.loginUser(ctx, user, password, userAgent, ip)
}

func (s *Service) loginUser(ctx context.Context, user *User, password, userAgent, ip string) (*User, string, error) {
	if err := security.CheckPassword(user.PasswordHash, password); err != nil {
		return nil, "", ErrInvalidCredentials
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

func (s *Service) validateSetupToken(token string) error {
	if s.cfg.SetupToken == "" {
		return nil
	}
	provided := strings.TrimSpace(token)
	if len(provided) != len(s.cfg.SetupToken) || subtle.ConstantTimeCompare([]byte(provided), []byte(s.cfg.SetupToken)) != 1 {
		return ErrInvalidSetupToken
	}
	return nil
}

// validateSetup checks setup input and returns the first problem as a
// ValidationError, or nil.
func validateSetup(p SetupParams) error {
	if p.Password == "" {
		return ValidationError{Field: "password", Reason: "is required"}
	}
	if p.ConfirmPassword == "" {
		return ValidationError{Field: "confirmPassword", Reason: "is required"}
	}
	if p.Password != p.ConfirmPassword {
		return ValidationError{Field: "confirmPassword", Reason: "does not match password"}
	}
	if err := security.ValidatePasswordStrength(p.Password); err != nil {
		return ValidationError{
			Field:  "password",
			Reason: fmt.Sprintf("must be at least %d characters and include at least three of lowercase, uppercase, digits, and symbols", security.MinPasswordLength),
		}
	}
	return nil
}

// truncate clips s to at most n UTF-8 runes (for storing bounded metadata).
func truncate(s string, n int) string {
	if n <= 0 || len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
