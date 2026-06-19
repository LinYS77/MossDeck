package auth

import (
	"net/http"
	"time"
)

// cookieSettings describes how the session cookie is written. It is derived
// once from configuration and reused for every set/clear.
type cookieSettings struct {
	Name     string
	Secure   bool
	SameSite string // "lax" | "strict" | "none"
	TTL      time.Duration
}

func (c cookieSettings) sameSiteMode() http.SameSite {
	switch c.SameSite {
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteLaxMode
	}
}

// Set writes the session cookie with value (the raw session token) using the
// secure attributes configured at startup.
func (c cookieSettings) Set(w http.ResponseWriter, value string) {
	http.SetCookie(w, &http.Cookie{
		Name:     c.Name,
		Value:    value,
		Path:     "/",
		MaxAge:   int(c.TTL.Seconds()),
		Expires:  time.Now().Add(c.TTL),
		HttpOnly: true,
		Secure:   c.Secure,
		SameSite: c.sameSiteMode(),
	})
}

// Clear expires the session cookie so the browser drops it. Used on logout
// and when an incoming cookie is no longer valid.
func (c cookieSettings) Clear(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     c.Name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(1, 0), // firmly in the past
		HttpOnly: true,
		Secure:   c.Secure,
		SameSite: c.sameSiteMode(),
	})
}
