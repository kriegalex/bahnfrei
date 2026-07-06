// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"

	_ "modernc.org/sqlite" // database/sql driver
)

// DBTX is the common surface of *sql.DB and *sql.Tx, so helpers work both
// standalone and inside an application transaction.
type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Store owns the one SQLite database per instance (ADR-004 §1).
type Store struct {
	db   *sql.DB
	path string
}

// Open opens (creating if needed) the database at path, applies the durability
// pragmas (WAL, synchronous=FULL — SYS-081), runs pending migrations, and runs
// the startup consistency check. Per ADR-004 §1 the check must report green
// before the application accepts writes, so a red check fails Open.
func Open(ctx context.Context, path string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?%s", url.PathEscape(filepath.ToSlash(path)),
		"_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(10000)")
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	// Single-writer concurrency model (ADR-004 §2): all writes flow through
	// this process over one connection; WAL readers can be pooled separately
	// once profiling demands it (SYS-120 sits far below SQLite's ceiling).
	db.SetMaxOpenConns(1)

	s := &Store{db: db, path: path}
	if _, err := Migrate(ctx, db, Migrations()); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate %s: %w", path, err)
	}
	report, err := s.Check(ctx)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("consistency check %s: %w", path, err)
	}
	if !report.OK() {
		_ = db.Close()
		return nil, fmt.Errorf("consistency check %s: %s", path, report.Summary())
	}
	return s, nil
}

// DB exposes the underlying handle to the app layer (architecture.md §3:
// web never sees this — it goes through app).
func (s *Store) DB() *sql.DB { return s.db }

// Path returns the database file location (used by backup, TASK-014).
func (s *Store) Path() string { return s.path }

func (s *Store) Close() error { return s.db.Close() }
