package migrations

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"
)

//go:embed *.up.sql
var migrationsFS embed.FS

func Run(db *sql.DB) error {
	if err := ensureSchemaMigrations(db); err != nil {
		return err
	}

	entries, err := fs.Glob(migrationsFS, "*.up.sql")
	if err != nil {
		return fmt.Errorf("migration glob failed: %w", err)
	}
	sort.Strings(entries)

	applied, err := loadAppliedVersions(db)
	if err != nil {
		return err
	}

	for _, path := range entries {
		version := extractVersion(path)
		if applied[version] {
			continue
		}

		sqlBytes, err := migrationsFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", path, err)
		}

		if _, err := db.Exec(string(sqlBytes)); err != nil {
			return fmt.Errorf("apply migration %s: %w", path, err)
		}

		if _, err := db.Exec(
			"INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)",
			version,
			time.Now().UTC(),
		); err != nil {
			return fmt.Errorf("record migration %s: %w", path, err)
		}
	}

	return nil
}

func ensureSchemaMigrations(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMP NOT NULL
		)
	`)
	return err
}

func loadAppliedVersions(db *sql.DB) (map[string]bool, error) {
	rows, err := db.Query("SELECT version FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := map[string]bool{}
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		applied[version] = true
	}
	return applied, rows.Err()
}

func extractVersion(path string) string {
	return strings.TrimSuffix(path, ".up.sql")
}
