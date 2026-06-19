package backup

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/linyusheng/homepage/internal/api"
	"github.com/linyusheng/homepage/internal/auth"
	"github.com/linyusheng/homepage/internal/reqid"
)

// Register mounts backup endpoints onto mux. Both are wrapped in
// auth.RequireAuth so every access requires a valid session.
func Register(mux *http.ServeMux, svc *Service) {
	require := func(h http.HandlerFunc) http.Handler {
		return auth.RequireAuth(svc.auth)(h)
	}
	mux.Handle("GET /api/v1/backup/export", require(svc.handleExport()))
	mux.Handle("POST /api/v1/backup/import", require(svc.handleImport()))
}

func (s *Service) handleExport() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := currentUserID(r.Context())
		data, err := s.Export(r.Context(), userID)
		if err != nil {
			s.writeErr(w, r, err)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Disposition",
			`attachment; filename="homepage-backup-`+time.Now().UTC().Format("20060102")+`.json"`)
		api.WriteOK(w, r, data)
	}
}

func (s *Service) handleImport() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, MaxImportBytes)
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()

		var req ImportRequest
		if err := dec.Decode(&req); err != nil {
			var maxBytesErr *http.MaxBytesError
			if errors.As(err, &maxBytesErr) {
				api.WriteError(w, r, api.BadRequest("request body too large (max 10 MiB)", err))
				return
			}
			api.WriteError(w, r, api.BadRequest("invalid JSON", err))
			return
		}
		if dec.More() {
			api.WriteError(w, r, api.BadRequest("unexpected trailing content in request body", nil))
			return
		}

		userID := currentUserID(r.Context())
		summary, err := s.Import(r.Context(), userID, req)
		if err != nil {
			// Validation errors: return 200 with the summary (which has inline
			// errors/warnings) so the frontend can display them directly.
			if errors.Is(err, ErrInvalidBackup) || errors.Is(err, ErrInvalidMode) {
				api.WriteOK(w, r, summary)
				return
			}
			// System errors (tx failure etc): 500.
			s.writeErr(w, r, err)
			return
		}
		api.WriteOK(w, r, summary)
	}
}

func (s *Service) writeErr(w http.ResponseWriter, r *http.Request, err error) {
	se := api.Internal("backup operation failed", err)
	if errors.Is(err, ErrInvalidBackup) || errors.Is(err, ErrInvalidMode) {
		se = api.BadRequest(err.Error(), err)
	}
	s.logger.Error("backup operation", "request_id", reqid.From(r.Context()), "error", err)
	api.WriteError(w, r, se)
}

// ensure time import is used (in export filename)
var _ = time.Now
