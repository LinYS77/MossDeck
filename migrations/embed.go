// Package migrations embeds the SQL migration files so the binary is fully
// self-contained at deploy time. Add new migrations as numbered .sql files
// (e.g. 0002_sessions_indexes.sql) and they will be picked up automatically
// by db.Migrate.
package migrations

import "embed"

// FS holds all *.sql migration files embedded at build time.
//
//go:embed *.sql
var FS embed.FS
