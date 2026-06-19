package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/linyusheng/homepage/internal/api"
	"github.com/linyusheng/homepage/internal/reqid"
)

const maxAuthBodyBytes = 8 * 1024

type UserDTO struct {
	ID          int64  `json:"id"`
	LastLoginAt string `json:"lastLoginAt,omitempty"`
	CreatedAt   string `json:"createdAt"`
}

func toUserDTO(u *User) UserDTO {
	return UserDTO{
		ID:          u.ID,
		LastLoginAt: toRFC3339(u.LastLoginAt),
		CreatedAt:   toRFC3339(u.CreatedAt),
	}
}

func Register(mux *http.ServeMux, svc *Service) {
	mux.HandleFunc("GET /api/v1/auth/status", svc.handleStatus())
	mux.HandleFunc("POST /api/v1/auth/setup", svc.handleSetup())
	mux.HandleFunc("POST /api/v1/auth/login", svc.handleLogin())
	mux.HandleFunc("POST /api/v1/auth/logout", svc.handleLogout())
	mux.Handle("GET /api/v1/auth/me", RequireAuth(svc)(http.HandlerFunc(svc.handleMe())))
}

type statusResponse struct {
	Initialized        bool `json:"initialized"`
	SetupEnabled       bool `json:"setupEnabled"`
	SetupTokenRequired bool `json:"setupTokenRequired"`
}

type setupRequest struct {
	Password        string `json:"password"`
	ConfirmPassword string `json:"confirmPassword"`
	SetupToken      string `json:"setupToken"`
}

func (s *Service) handleStatus() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		initialized, setupEnabled, setupTokenRequired, err := s.Status(r.Context())
		if err != nil {
			s.logger.Error("auth status failed", "request_id", reqid.From(r.Context()), "error", err)
			api.WriteError(w, r, api.Internal("auth status failed", err))
			return
		}
		api.WriteOK(w, r, statusResponse{
			Initialized:        initialized,
			SetupEnabled:       setupEnabled,
			SetupTokenRequired: setupTokenRequired,
		})
	}
}

func (s *Service) handleSetup() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req setupRequest
		if err := decodeJSON(r, &req); err != nil {
			api.WriteError(w, r, api.BadRequest("invalid request body", err))
			return
		}
		user, err := s.Setup(r.Context(), SetupParams{
			Password:        req.Password,
			ConfirmPassword: req.ConfirmPassword,
			SetupToken:      req.SetupToken,
		})
		if err != nil {
			switch {
			case errors.Is(err, ErrSetupDisabled):
				api.WriteError(w, r, api.New(http.StatusForbidden, "SETUP_DISABLED",
					"setup is disabled on this server", nil))
			case errors.Is(err, ErrAlreadyInitialized):
				api.WriteError(w, r, api.New(http.StatusConflict, "SETUP_DISABLED",
					"setup has already been completed", nil))
			case errors.Is(err, ErrInvalidSetupToken):
				api.WriteError(w, r, api.New(http.StatusForbidden, "SETUP_TOKEN_INVALID",
					"invalid setup token", nil))
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
	Password string `json:"password"`
}

func (s *Service) handleLogin() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req loginRequest
		if err := decodeJSON(r, &req); err != nil {
			api.WriteError(w, r, api.BadRequest("invalid request body", err))
			return
		}
		if req.Password == "" {
			api.WriteError(w, r, api.BadRequest("password is required", nil))
			return
		}

		ip := s.resolver.ClientIP(r)
		key := loginKey(ip)
		if blocked, retry := s.limiter.TooMany(key); blocked {
			w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
			api.WriteError(w, r, api.TooManyRequests("too many failed login attempts, try again later"))
			return
		}

		user, token, err := s.Login(r.Context(), req.Password, r.UserAgent(), ip)
		if err != nil {
			if errors.Is(err, ErrInvalidCredentials) {
				s.limiter.RecordFailure(key)
				s.logger.Info("login failed", "request_id", reqid.From(r.Context()), "ip", ip)
				api.WriteError(w, r, api.Unauthorized("invalid password"))
				return
			}
			s.logger.Error("login error", "request_id", reqid.From(r.Context()), "error", err)
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
		s.logger.Info("login success", "request_id", reqid.From(r.Context()), "user_id", user.ID, "ip", ip)
		api.WriteOK(w, r, toUserDTO(user))
	}
}

func (s *Service) handleLogout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cookie, err := r.Cookie(s.cookie.Name); err == nil && cookie.Value != "" {
			if err := s.Logout(r.Context(), cookie.Value); err != nil {
				s.logger.Warn("revoke session failed", "request_id", reqid.From(r.Context()), "error", err)
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

func (s *Service) writeDomainOrInternal(w http.ResponseWriter, r *http.Request, err error, op string) {
	var ve ValidationError
	if errors.As(err, &ve) {
		api.WriteError(w, r, api.BadRequest(ve.Error(), nil))
		return
	}
	s.logger.Error(op, "request_id", reqid.From(r.Context()), "error", err)
	api.WriteError(w, r, api.Internal(op, err))
}

func loginKey(ip string) string {
	return ip
}

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
