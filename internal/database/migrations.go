package database

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed migrations
var migrationsFS embed.FS

// RunMigrations applies all pending *.up.sql migrations in lexicographical order.
// Migrations are tracked in schema_migrations and are idempotent.
func RunMigrations(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version    TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
	)`)
	if err != nil {
		return fmt.Errorf("migration: create tracking table: %w", err)
	}

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("migration: read dir: %w", err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".up.sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for _, fname := range files {
		var count int
		if err := db.QueryRow(
			"SELECT COUNT(*) FROM schema_migrations WHERE version = ?", fname,
		).Scan(&count); err != nil {
			return fmt.Errorf("migration: check %s: %w", fname, err)
		}
		if count > 0 {
			continue
		}

		data, err := migrationsFS.ReadFile("migrations/" + fname)
		if err != nil {
			return fmt.Errorf("migration: read %s: %w", fname, err)
		}

		for _, stmt := range splitSQL(string(data)) {
			if _, err := db.Exec(stmt); err != nil {
				return fmt.Errorf("migration: exec %s: %w", fname, err)
			}
		}

		if _, err := db.Exec(
			"INSERT INTO schema_migrations (version) VALUES (?)", fname,
		); err != nil {
			return fmt.Errorf("migration: record %s: %w", fname, err)
		}
	}
	return nil
}

// splitSQL splits a SQL script into individual statements, ignoring empty lines.
func splitSQL(script string) []string {
	parts := strings.Split(script, ";")
	var stmts []string
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			stmts = append(stmts, s)
		}
	}
	return stmts
}
