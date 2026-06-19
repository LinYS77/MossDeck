package auth

import (
	"context"
	"errors"
	"net/http"

	"github.com/linyusheng/homepage/internal/api"
	"github.com/linyusheng/homepage/internal/reqid"
)

// userCtxKey is an unexported key type so other packages cannot forge a user
// in context.
type userCtxKey struct{}

// WithUser stores the authenticated user in ctx.
func WithUser(ctx context.Context, u *User) context.Context {
	return context.WithValue(ctx, userCtxKey{}, u)
}

// UserFromContext returns the authenticated user placed there by RequireAuth.
// The bool is false when there is no authenticated user (e.g. on a public
// route). Protected handlers should be mounted behind RequireAuth so the
// user is always present.
func UserFromContext(ctx context.Context) (*User, bool) {
	u, ok := ctx.Value(userCtxKey{}).(*User)
	return u, ok
}

// RequireAuth wraps a handler so it only runs for requests with a valid
// session cookie; otherwise it responds 401. On success the resolved user is
// available downstream via UserFromContext.
//
// Future protected routes wrap their handlers the same way, e.g.:
//
//	mux.Handle("GET /api/v1/bookmarks",
//	    auth.RequireAuth(authSvc)(bookmarks.ListHandler))
//
// Role/permission checks can later be layered as an additional middleware
// that reads UserFromContext; this keeps the CSRF/authorization boundary
// separate from authentication.
func RequireAuth(svc *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(svc.cookie.Name)
			if err != nil || cookie.Value == "" {
				api.WriteError(w, r, api.Unauthorized("authentication required"))
				return
			}
			user, err := svc.ResolveSession(r.Context(), cookie.Value)
			if err != nil {
				// Drop the now-invalid cookie so the browser stops sending it,
				// then deny. Any non-domain error is logged for ops.
				svc.cookie.Clear(w)
				if !isDomainErr(err) {
					svc.logger.Error("resolve session failed",
						"request_id", reqid.From(r.Context()),
						"error", err)
				}
				api.WriteError(w, r, api.Unauthorized("authentication required"))
				return
			}
			next.ServeHTTP(w, r.WithContext(WithUser(r.Context(), user)))
		})
	}
}

// isDomainErr reports whether err is one of the expected, non-alarming auth
// errors (used to avoid logging routine 401s at error level).
func isDomainErr(err error) bool {
	return errors.Is(err, ErrSessionInvalid)
}
