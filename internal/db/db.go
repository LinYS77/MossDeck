// Package db manages the SQLite connection and runs schema migrations.
//
// The driver is modernc.org/sqlite, a pure-Go (CGO-free) implementation,
// which keeps builds and cross-compilation simple. Connection defaults are
// tuned for single-process reliability:
//   - WAL journaling for reader/writer concurrency,
//   - foreign keys enabled,
//   - a busy timeout so transient lock contention waits instead of failing,
//   - a single pooled connection to keep write ordering simple and avoid
//     "database is locked" surprises in the early skeleton.
//
// The pool size is a conservative starting point and can be revisited once
// auth/bookmark handlers add real concurrency requirements.
package db

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // register the "sqlite" driver
)

// Open returns a configured *sql.DB for the SQLite file at path. An in-memory
// database may be requested with path == ":memory:" (used by tests).
func Open(ctx context.Context, path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", buildDSN(path))
	if err != nil {
		return nil, fmt.Errorf("open sqlite %q: %w", path, err)
	}
	// A single connection serializes access. For SQLite this is the most
	// robust default and removes a whole class of locking bugs early on.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite %q: %w", path, err)
	}
	return db, nil
}

func buildDSN(path string) string {
	const pragmas = "_pragma=journal_mode(WAL)" +
		"&_pragma=foreign_keys(ON)" +
		"&_pragma=busy_timeout(5000)" +
		"&_pragma=synchronous(NORMAL)"
	if path == ":memory:" {
		return "file::memory:?" + pragmas
	}
	return "file:" + path + "?" + pragmas
}
