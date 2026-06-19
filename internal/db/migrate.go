package db

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"

	"github.com/linyusheng/homepage/migrations"
)

// Migrate applies all embedded SQL migrations that have not run yet.
//
// Migrations are plain .sql files under the migrations/ directory, applied in
// lexical filename order. Each migration runs inside its own transaction and
// is recorded in the schema_migrations table, which makes the process safe to
// re-run (idempotent) and avoids any external migration tool dependency.
//
// Down/rollback migrations are intentionally not supported: the project ships
// forward-only migrations, which keeps operational reasoning simple for a
// single-user private deployment.
func Migrate(ctx context.Context, db *sql.DB) error {
	if err := ensureTable(ctx, db); err != nil {
		return fmt.Errorf("init schema_migrations: %w", err)
	}

	names, err := fs.Glob(migrations.FS, "*.sql")
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}
	sort.Strings(names)

	for _, name := range names {
		applied, err := isApplied(ctx, db, name)
		if err != nil {
			return fmt.Errorf("check %s: %w", name, err)
		}
		if applied {
			continue
		}
		content, err := migrations.FS.ReadFile(name)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		if err := apply(ctx, db, name, content); err != nil {
			return err
		}
		slog.Info("migration applied", "name", name)
	}
	return nil
}

func ensureTable(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
	version    TEXT PRIMARY KEY,
	applied_at TEXT NOT NULL DEFAULT (datetime('now'))
);`)
	return err
}

func isApplied(ctx context.Context, db *sql.DB, name string) (bool, error) {
	var v string
	err := db.QueryRowContext(ctx, `SELECT version FROM schema_migrations WHERE version = ?`, name).Scan(&v)
	switch {
	case err == nil:
		return true, nil
	case err == sql.ErrNoRows:
		return false, nil
	default:
		return false, err
	}
}

func apply(ctx context.Context, db *sql.DB, name string, content []byte) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin %s: %w", name, err)
	}
	defer func() { _ = tx.Rollback() }() // safe to roll back after Commit

	if _, err := tx.ExecContext(ctx, string(content)); err != nil {
		return fmt.Errorf("apply %s: %w", name, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES (?)`, name); err != nil {
		return fmt.Errorf("record %s: %w", name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit %s: %w", name, err)
	}
	return nil
}
