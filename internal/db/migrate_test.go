package db

import (
	"context"
	"database/sql"
	"io/fs"
	"testing"

	"github.com/linyusheng/homepage/migrations"
)

func openMemory(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// tableExists reports whether a table of the given name is present.
func tableExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var n int
	err := db.QueryRow(
		`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?`,
		name,
	).Scan(&n)
	if err != nil {
		t.Fatalf("query sqlite_master for %q: %v", name, err)
	}
	return n == 1
}

// countApplied returns the number of rows in schema_migrations.
func countApplied(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	return n
}

// countEmbeddedMigrations returns how many *.sql files are embedded, so the
// tests stay correct as new migrations are added.
func countEmbeddedMigrations(t *testing.T) int {
	t.Helper()
	names, err := fs.Glob(migrations.FS, "*.sql")
	if err != nil {
		t.Fatalf("glob embedded migrations: %v", err)
	}
	return len(names)
}

func TestMigrateCreatesSchema(t *testing.T) {
	ctx := context.Background()
	db := openMemory(t)

	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("first migrate: %v", err)
	}

	for _, table := range []string{
		"schema_migrations", "users", "sessions",
		// 0002_bookmarks.sql tables:
		"categories", "tags", "bookmarks", "bookmark_tags",
	} {
		if !tableExists(t, db, table) {
			t.Fatalf("expected table %q to exist after migrate", table)
		}
	}

	applied, expected := countApplied(t, db), countEmbeddedMigrations(t)
	if applied != expected {
		t.Fatalf("expected %d applied migration(s), got %d", expected, applied)
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	ctx := context.Background()
	db := openMemory(t)

	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	// Running again must not error and must not re-apply anything.
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("second migrate: %v", err)
	}

	applied, expected := countApplied(t, db), countEmbeddedMigrations(t)
	if applied != expected {
		t.Fatalf("expected still %d applied migration(s), got %d", expected, applied)
	}
}

func TestForeignKeysEnabled(t *testing.T) {
	ctx := context.Background()
	db := openMemory(t)
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	var fk int
	if err := db.QueryRow(`PRAGMA foreign_keys`).Scan(&fk); err != nil {
		t.Fatalf("pragma foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Fatalf("expected foreign_keys ON (1), got %d", fk)
	}
}
