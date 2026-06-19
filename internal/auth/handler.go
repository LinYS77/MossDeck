package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/linyusheng/homepage/internal/api"
	"github.com/linyusheng/homepage/internal/reqid"
)

// maxAuthBodyBytes caps request bodies for auth endpoints to keep parsing
// cheap and bounded under abuse.
const maxAuthBodyBytes = 8 * 1024

// UserDTO is the public representation of a user returned by auth endpoints.
// It omits password_hash and other sensitive fields.
type UserDTO struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	Email       string `json:"email,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	Role        string `json:"role"`
	Status      string `json:"status"`
	LastLoginAt string `json:"lastLoginAt,omitempty"`
	CreatedAt   string `json:"createdAt"`
}

func toUserDTO(u *User) UserDTO {
	return UserDTO{
		ID:          u.ID,
		Username:    u.Username,
		Email:       u.Email,
		DisplayName: u.DisplayName,
		Role:        u.Role,
		Status:      u.Status,
		LastLoginAt: toRFC3339(u.LastLoginAt),
		CreatedAt:   toRFC3339(u.CreatedAt),
	}
}

// Register mounts the auth endpoints onto mux. Public endpoints (setup,
// login, logout) are mounted directly; me is wrapped in RequireAuth.
func Register(mux *http.ServeMux, svc *Service) {
	mux.HandleFunc("POST /api/v1/auth/setup", svc.handleSetup())
	mux.HandleFunc("POST /api/v1/auth/login", svc.handleLogin())
	mux.HandleFunc("POST /api/v1/auth/logout", svc.handleLogout())
	mux.Handle("GET /api/v1/auth/me", RequireAuth(svc)(http.HandlerFunc(svc.handleMe())))
}

type setupRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
}

func (s *Service) handleSetup() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req setupRequest
		if err := decodeJSON(r, &req); err != nil {
			api.WriteError(w, r, api.BadRequest("invalid request body", err))
			return
		}
		user, err := s.Setup(r.Context(), SetupParams{
			Username:    req.Username,
			Password:    req.Password,
			Email:       req.Email,
			DisplayName: req.DisplayName,
		})
		if err != nil {
			switch {
			case errors.Is(err, ErrSetupDisabled):
				api.WriteError(w, r, api.New(http.StatusForbidden, "SETUP_DISABLED",
					"setup is disabled on this server", nil))
			case errors.Is(err, ErrAlreadyInitialized):
				api.WriteError(w, r, api.New(http.StatusConflict, "SETUP_DISABLED",
					"setup has already been completed", nil))
			case errors.Is(err, ErrUsernameTaken):
				api.WriteError(w, r, api.Conflict("username already taken"))
			default:
				s.writeDomainOrInternal(w, r, err, "setup failed")
			}
			return
		}
		if err := s.issueCSRF(w); err != nil {
			s.logger.Error("issue csrf token failed", "request_id", reqid.From(r.Context()), "error", err)
			api.WriteError(w, r, api.Internal("issue csrf token failed", err))
			return
		}
		api.WriteCreated(w, r, toUserDTO(user))
	}
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Service) handleLogin() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req loginRequest
		if err := decodeJSON(r, &req); err != nil {
			api.WriteError(w, r, api.BadRequest("invalid request body", err))
			return
		}
		username := strings.TrimSpace(req.Username)
		if username == "" || req.Password == "" {
			api.WriteError(w, r, api.BadRequest("username and password are required", nil))
			return
		}

		ip := s.resolver.ClientIP(r)
		key := loginKey(ip, username)

		// Throttle before touching credentials so a flooded key never reaches
		// the (expensive) bcrypt comparison.
		if blocked, retry := s.limiter.TooMany(key); blocked {
			w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
			api.WriteError(w, r, api.TooManyRequests("too many failed login attempts, try again later"))
			return
		}

		user, token, err := s.Login(r.Context(), username, req.Password, r.UserAgent(), ip)
		if err != nil {
			// Account for the failure (rate-limit) and respond with a uniform
			// message that never reveals whether the username exists.
			if errors.Is(err, ErrInvalidCredentials) || errors.Is(err, ErrUserInactive) {
				s.limiter.RecordFailure(key)
			}
			if errors.Is(err, ErrInvalidCredentials) || errors.Is(err, ErrUserInactive) {
				s.logger.Info("login failed",
					"request_id", reqid.From(r.Context()),
					"username", username, "ip", ip)
				api.WriteError(w, r, api.Unauthorized("invalid username or password"))
				return
			}
			s.logger.Error("login error",
				"request_id", reqid.From(r.Context()), "error", err)
			api.WriteError(w, r, api.Internal("login failed", err))
			return
		}

		s.limiter.Reset(key)
		s.cookie.Set(w, token)
		if err := s.issueCSRF(w); err != nil {
			s.logger.Error("issue csrf token failed", "request_id", reqid.From(r.Context()), "error", err)
			api.WriteError(w, r, api.Internal("issue csrf token failed", err))
			return
		}
		s.logger.Info("login success",
			"request_id", reqid.From(r.Context()),
			"user_id", user.ID, "username", username, "ip", ip)
		api.WriteOK(w, r, toUserDTO(user))
	}
}

func (s *Service) handleLogout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cookie, err := r.Cookie(s.cookie.Name); err == nil && cookie.Value != "" {
			if err := s.Logout(r.Context(), cookie.Value); err != nil {
				// Revoke is best-effort; the cookie is cleared regardless.
				s.logger.Warn("revoke session failed",
					"request_id", reqid.From(r.Context()), "error", err)
			}
		}
		s.cookie.Clear(w)
		if s.csrf != nil {
			s.csrf.Clear(w)
		}
		api.WriteOK(w, r, map[string]any{"loggedOut": true})
	}
}

func (s *Service) issueCSRF(w http.ResponseWriter) error {
	if s.csrf == nil {
		return nil
	}
	_, err := s.csrf.Issue(w)
	return err
}

func (s *Service) handleMe() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := UserFromContext(r.Context())
		if !ok {
			api.WriteError(w, r, api.Unauthorized("authentication required"))
			return
		}
		api.WriteOK(w, r, toUserDTO(user))
	}
}

// writeDomainOrInternal maps ValidationError to a 400 and everything else to
// a 500 (logged), used by handlers that only expect validation vs. unexpected
// errors.
func (s *Service) writeDomainOrInternal(w http.ResponseWriter, r *http.Request, err error, op string) {
	var ve ValidationError
	if errors.As(err, &ve) {
		api.WriteError(w, r, api.BadRequest(ve.Error(), nil))
		return
	}
	s.logger.Error(op, "request_id", reqid.From(r.Context()), "error", err)
	api.WriteError(w, r, api.Internal(op, err))
}

// loginKey builds the rate-limit key from the client IP and a normalized
// username. Normalizing the username to lower case means case-only variations
// share a bucket.
func loginKey(ip, username string) string {
	return ip + "|" + strings.ToLower(strings.TrimSpace(username))
}

// decodeJSON reads a bounded JSON body into dst. It rejects unknown fields and
// trailing content so clients get clear feedback on malformed payloads.
func decodeJSON(r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, maxAuthBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if dec.More() {
		return errors.New("unexpected trailing content in request body")
	}
	return nil
}

// toRFC3339 converts a SQLite datetime('now')-style timestamp into RFC3339 for
// API output. Values that do not parse (e.g. empty) are returned unchanged.
func toRFC3339(s string) string {
	if s == "" {
		return ""
	}
	t, err := time.Parse(dbTimeFormat, s)
	if err != nil {
		return s
	}
	return t.UTC().Format(time.RFC3339)
}
