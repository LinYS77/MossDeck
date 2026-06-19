// Package server assembles the HTTP router, applies the middleware chain,
// and owns the graceful lifecycle of the *http.Server.
//
// Routing uses the standard library net/http ServeMux (Go 1.22+ method and
// path patterns) to avoid pulling in a third-party router for the skeleton.
// When a feature outgrows ServeMux, swapping it in is localized to this file.
package server

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/linyusheng/homepage/internal/auth"
	"github.com/linyusheng/homepage/internal/backup"
	"github.com/linyusheng/homepage/internal/bookmark"
	"github.com/linyusheng/homepage/internal/config"
	"github.com/linyusheng/homepage/internal/csrf"
	"github.com/linyusheng/homepage/internal/httpx"
	"github.com/linyusheng/homepage/internal/ratelimit"
	"github.com/linyusheng/homepage/internal/readlater"
	"github.com/linyusheng/homepage/internal/server/handler"
	"github.com/linyusheng/homepage/internal/server/middleware"
)

// Server wraps an *http.Server and remembers its logger so shutdown can be
// logged through the same structured logger as the rest of the app.
type Server struct {
	httpServer *http.Server
	logger     *slog.Logger
}

// New builds the router, applies middleware, and returns a ready-to-serve
// Server. The database is passed through so handlers can reach it.
//
// ctx is the application lifetime context: it scopes the in-memory login
// rate limiter's background sweeper, which stops when ctx is cancelled.
func New(ctx context.Context, cfg config.Config, db *sql.DB, logger *slog.Logger) *Server {
	mux := http.NewServeMux()
	handler.Register(mux, db)

	// --- Authentication ---
	// The rate limiter is process-local today; swapping in a persistent/
	// distributed limiter later only changes the construction below.
	limiter := ratelimit.New(ctx, cfg.LoginMaxFailures, cfg.LoginWindow)

	// Client-IP resolver trusts forwarded headers only from the configured
	// reverse-proxy CIDRs; empty config trusts nothing, so forged headers
	// cannot rotate apparent IPs to bypass per-IP login limiting. CIDRs are
	// validated by config.Load, so construction should not fail; on the
	// defensive off-chance it does, fall back to an untrusted resolver and
	// log loudly rather than silently weakening security.
	resolver, err := httpx.NewClientIPResolver(cfg.TrustedProxyCIDRs)
	if err != nil {
		logger.Error("invalid trusted proxy CIDRs; forwarding headers will NOT be trusted",
			"error", err)
		resolver, _ = httpx.NewClientIPResolver(nil)
	}

	csrfManager := csrf.New(csrf.Config{
		CookieName:        cfg.CSRFCookieName,
		HeaderName:        cfg.CSRFHeaderName,
		SessionCookieName: cfg.CookieName,
		CookieSecure:      cfg.CookieSecure,
		CookieSameSite:    cfg.CookieSameSite,
		TTL:               cfg.SessionTTL,
	})

	authSvc := auth.NewService(auth.Config{
		SessionTTL:     cfg.SessionTTL,
		BcryptCost:     cfg.BcryptCost,
		CookieName:     cfg.CookieName,
		CookieSecure:   cfg.CookieSecure,
		CookieSameSite: cfg.CookieSameSite,
		SetupEnabled:   cfg.SetupEnabled,
		SetupToken:     cfg.SetupToken,
	}, auth.NewStore(db), limiter, resolver, logger)
	authSvc.SetCSRFManager(csrfManager)
	auth.Register(mux, authSvc)
	mux.Handle("GET /api/v1/auth/csrf", csrfManager.HandleToken())

	// --- Bookmark core ---
	// Every bookmark endpoint is wrapped in auth.RequireAuth so access always
	// requires a valid session; the user id (auth.UserFromContext) scopes every
	// query. The bookmark Service holds a reference to authSvc for the middleware.
	bookmarkSvc := bookmark.NewService(bookmark.NewStore(db), authSvc, logger)
	bookmark.Register(mux, bookmarkSvc)

	// --- Read-later core ---
	// Every read-later endpoint is also wrapped in auth.RequireAuth and every
	// query is scoped by user id inside the service/store layer.
	readLaterSvc := readlater.NewService(readlater.NewStore(db), authSvc, logger)
	readlater.Register(mux, readLaterSvc)

	// --- Backup / restore ---
	backupSvc := backup.NewService(backup.NewStore(db), db, authSvc, logger)
	backup.Register(mux, backupSvc)

	// Mount points reserved for upcoming feature modules:
	//   /api/v1/import/...      -> internal/backup / import

	// --- Static SPA (production) ---
	// When APP_STATIC_DIR is set, serve the built frontend for all non-API
	// GET requests. The catch-all pattern gives way to more specific API
	// routes registered above. In development (StaticDir="") this is skipped.
	if cfg.StaticDir != "" {
		logger.Info("serving static frontend", "dir", cfg.StaticDir)
		mux.HandleFunc("GET /{pathname...}", spaFileServer(cfg.StaticDir))
	}

	var h http.Handler = mux
	// Order matters: Recover is outermost, then RequestID so the access log
	// and panic log can both reference the id, then Logger innermost.
	h = csrfManager.Middleware(h)
	h = middleware.RequestID(h)
	h = middleware.Logger(logger)(h)
	h = middleware.Recover(logger)(h)

	return &Server{
		logger: logger,
		httpServer: &http.Server{
			Addr:              cfg.HTTPAddr,
			Handler:           h,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       cfg.ReadTimeout,
			WriteTimeout:      cfg.WriteTimeout,
			IdleTimeout:       60 * time.Second,
			// Route http.Server's own error log through slog as JSON lines.
			ErrorLog: slog.NewLogLogger(logger.Handler(), slog.LevelError),
		},
	}
}

// ListenAndServe starts serving HTTP. It blocks until Shutdown is called or
// the listener fails.
func (s *Server) ListenAndServe() error {
	if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Shutdown gracefully drains in-flight requests, waiting up to ctx for them
// to finish before forcibly closing.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("http server shutting down")
	return s.httpServer.Shutdown(ctx)
}

// spaFileServer returns a handler that serves files from root, falling back to
// index.html for SPA client-side routing. Directories and missing files both
// serve index.html so React Router can handle the path.
func spaFileServer(root string) http.HandlerFunc {
	fs := http.FileServer(http.Dir(root))
	return func(w http.ResponseWriter, r *http.Request) {
		// Clean the path to prevent directory traversal.
		upath := filepath.Clean(r.URL.Path)
		fpath := filepath.Join(root, upath)

		// Ensure the resolved path stays inside root.
		if !strings.HasPrefix(fpath, filepath.Clean(root)+string(filepath.Separator)) &&
			fpath != filepath.Clean(root) {
			http.NotFound(w, r)
			return
		}

		info, err := os.Stat(fpath)
		if err != nil || info.IsDir() {
			// SPA fallback: serve index.html for any path React Router handles.
			http.ServeFile(w, r, filepath.Join(root, "index.html"))
			return
		}
		fs.ServeHTTP(w, r)
	}
}
