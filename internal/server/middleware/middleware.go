// Package middleware contains the HTTP middleware shared by all routes.
//
// The chain runs in registration order (outermost first):
//
//	Recover -> RequestID -> Logger -> handler
//
// Recover must wrap everything so panics anywhere downstream are caught;
// RequestID must run before Logger so the access log can include the id.
package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/linyusheng/homepage/internal/api"
	"github.com/linyusheng/homepage/internal/reqid"
)

// Recover catches panics from downstream handlers, logs them with a stack
// trace, and returns a clean 500 instead of crashing the process.
func Recover(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Error("panic recovered",
						"request_id", reqid.From(r.Context()),
						"method", r.Method,
						"path", r.URL.Path,
						"panic", rec,
						"stack", string(debug.Stack()),
					)
					api.WriteError(w, r, api.Internal("internal server error", nil))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// RequestID ensures every request carries a request id. If the inbound
// request has a trusted "X-Request-ID" header it is honoured (useful behind
// a gateway), otherwise a fresh id is generated. The id is stored in context
// and echoed back via the response header.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if !isValidID(id) {
			id = reqid.New()
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(reqid.With(r.Context(), id)))
	})
}

// Logger emits one structured access line per request with method, path,
// status, size, latency, and the request id. It never fails the request.
func Logger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			logger.Info("http request",
				"request_id", reqid.From(r.Context()),
				"method", r.Method,
				"path", r.URL.RequestURI(),
				"status", rec.status,
				"bytes", rec.bytes,
				"duration_ms", time.Since(start).Milliseconds(),
				"remote_addr", r.RemoteAddr,
			)
		})
	}
}

// statusRecorder wraps http.ResponseWriter to capture the status code and
// bytes written for logging. It intentionally implements Unwrap for
// http.ResponseController compatibility.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	bytes       int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if r.wroteHeader {
		return
	}
	r.status = code
	r.wroteHeader = true
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(p []byte) (int, error) {
	n, err := r.ResponseWriter.Write(p)
	r.bytes += n
	return n, err
}

// Unwrap allows http.ResponseController to reach the underlying writer.
func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// isValidID accepts the common request-id shapes we generate or trust from
// upstream. Empty strings or anything with whitespace/quotes is rejected so
// we always emit a controlled header value.
func isValidID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, c := range id {
		switch {
		case c >= 'a' && c <= 'z',
			c >= 'A' && c <= 'Z',
			c >= '0' && c <= '9',
			c == '_', c == '-', c == '.':
		default:
			return false
		}
	}
	return true
}
