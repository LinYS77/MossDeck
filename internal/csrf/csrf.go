// Package csrf implements double-submit CSRF protection for cookie-authenticated
// browser clients.
package csrf

import (
	"crypto/subtle"
	"net/http"
	"time"

	"github.com/linyusheng/homepage/internal/api"
	"github.com/linyusheng/homepage/internal/security"
)

const (
	DefaultCookieName = "homepage_csrf"
	DefaultHeaderName = "X-CSRF-Token"
)

type Config struct {
	CookieName        string
	HeaderName        string
	SessionCookieName string
	CookieSecure      bool
	CookieSameSite    string
	TTL               time.Duration
}

type Manager struct {
	cookieName        string
	headerName        string
	sessionCookieName string
	secure            bool
	sameSite          string
	ttl               time.Duration
}

type Response struct {
	Token      string `json:"token"`
	CookieName string `json:"cookieName"`
	HeaderName string `json:"headerName"`
}

func New(cfg Config) *Manager {
	cookieName := cfg.CookieName
	if cookieName == "" {
		cookieName = DefaultCookieName
	}
	headerName := cfg.HeaderName
	if headerName == "" {
		headerName = DefaultHeaderName
	}
	ttl := cfg.TTL
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &Manager{
		cookieName:        cookieName,
		headerName:        headerName,
		sessionCookieName: cfg.SessionCookieName,
		secure:            cfg.CookieSecure,
		sameSite:          cfg.CookieSameSite,
		ttl:               ttl,
	}
}

func (m *Manager) CookieName() string { return m.cookieName }

func (m *Manager) HeaderName() string { return m.headerName }

func (m *Manager) Issue(w http.ResponseWriter) (string, error) {
	token, err := security.GenerateToken()
	if err != nil {
		return "", err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     m.cookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(m.ttl.Seconds()),
		Expires:  time.Now().Add(m.ttl),
		HttpOnly: false,
		Secure:   m.secure,
		SameSite: m.sameSiteMode(),
	})
	return token, nil
}

func (m *Manager) Clear(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     m.cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(1, 0),
		HttpOnly: false,
		Secure:   m.secure,
		SameSite: m.sameSiteMode(),
	})
}

func (m *Manager) HandleToken() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, err := m.Issue(w)
		if err != nil {
			api.WriteError(w, r, api.Internal("issue csrf token failed", err))
			return
		}
		api.WriteOK(w, r, Response{Token: token, CookieName: m.cookieName, HeaderName: m.headerName})
	}
}

func (m *Manager) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m.requiresCheck(r) && !m.valid(r) {
			api.WriteError(w, r, InvalidError())
			return
		}
		next.ServeHTTP(w, r)
	})
}

func InvalidError() *api.StatusError {
	return api.New(http.StatusForbidden, "CSRF_INVALID", "invalid csrf token", nil)
}

func (m *Manager) requiresCheck(r *http.Request) bool {
	if isSafeMethod(r.Method) || m.isExempt(r) {
		return false
	}
	if m.sessionCookieName == "" {
		return true
	}
	cookie, err := r.Cookie(m.sessionCookieName)
	return err == nil && cookie.Value != ""
}

func (m *Manager) valid(r *http.Request) bool {
	cookie, err := r.Cookie(m.cookieName)
	if err != nil || cookie.Value == "" {
		return false
	}
	header := r.Header.Get(m.headerName)
	if header == "" || len(header) != len(cookie.Value) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(header), []byte(cookie.Value)) == 1
}

func (m *Manager) isExempt(r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}
	switch r.URL.Path {
	case "/api/v1/auth/setup", "/api/v1/auth/login":
		return true
	default:
		return false
	}
}

func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

func (m *Manager) sameSiteMode() http.SameSite {
	switch m.sameSite {
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteLaxMode
	}
}
