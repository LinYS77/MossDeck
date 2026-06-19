// Package handler holds the HTTP handlers for the skeleton. Feature modules
// (auth, bookmarks, import) will live in their own packages later; this
// package currently owns only the system health check.
package handler

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/linyusheng/homepage/internal/api"
)

// serviceName is the stable identifier reported by the health endpoint.
const serviceName = "homepage"

// Register mounts the system endpoints onto mux.
//
// Future feature routes (auth, bookmarks, import, ...) will be mounted by
// their own packages; the skeleton registers only the system endpoints here.
func Register(mux *http.ServeMux, db *sql.DB) {
	mux.HandleFunc("GET /api/v1/system/health", health(db))
	// /healthz is a short, load-balancer-friendly alias to the same handler.
	mux.HandleFunc("GET /healthz", health(db))
}

type healthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
	Time    string `json:"time"`
}

// health reports liveness and database reachability. It is intentionally
// cheap and free of external network calls so it can be probed frequently.
func health(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		if err := db.PingContext(ctx); err != nil {
			api.WriteError(w, r, api.New(
				http.StatusServiceUnavailable,
				"DB_UNAVAILABLE",
				"database unavailable",
				err,
			))
			return
		}
		api.WriteOK(w, r, healthResponse{
			Status:  "ok",
			Service: serviceName,
			Time:    time.Now().UTC().Format(time.RFC3339),
		})
	}
}
