// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrations returns the production migration set embedded in the binary.
func Migrations() fs.FS {
	sub, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		panic(err) // embed layout is fixed at compile time
	}
	return sub
}

var migrationName = regexp.MustCompile(`^(\d{4})_[a-z0-9_]+\.sql$`)

// Migrate applies pending migrations from fsys in version order, each in its
// own transaction. Applied migrations are recorded with a SHA-256 checksum;
// a later run refuses to proceed if an already-applied file changed
// (migrations are immutable history, like requirement IDs).
// It returns the number of migrations applied in this run.
func Migrate(ctx context.Context, db *sql.DB, fsys fs.FS) (int, error) {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		name       TEXT NOT NULL,
		sha256     TEXT NOT NULL,
		applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
	)`); err != nil {
		return 0, fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return 0, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		m := migrationName.FindStringSubmatch(e.Name())
		if m == nil {
			return 0, fmt.Errorf("migration %q does not match NNNN_name.sql", e.Name())
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	applied := 0
	for _, name := range names {
		version, _ := strconv.Atoi(migrationName.FindStringSubmatch(name)[1])
		body, err := fs.ReadFile(fsys, name)
		if err != nil {
			return applied, err
		}
		sum := sha256.Sum256(body)
		checksum := hex.EncodeToString(sum[:])

		var prior string
		err = db.QueryRowContext(ctx,
			`SELECT sha256 FROM schema_migrations WHERE version = ?`, version).Scan(&prior)
		switch {
		case err == nil:
			if prior != checksum {
				return applied, fmt.Errorf("migration %s already applied with different content (checksum %s != %s)", name, prior, checksum)
			}
			continue
		case err != sql.ErrNoRows:
			return applied, err
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return applied, err
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			_ = tx.Rollback()
			return applied, fmt.Errorf("apply %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO schema_migrations (version, name, sha256) VALUES (?, ?, ?)`,
			version, name, checksum); err != nil {
			_ = tx.Rollback()
			return applied, fmt.Errorf("record %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return applied, fmt.Errorf("commit %s: %w", name, err)
		}
		applied++
	}
	return applied, nil
}
