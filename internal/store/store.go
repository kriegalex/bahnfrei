// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import (
	"context"
	"database/sql"
	"errors"
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

// readPoolSize bounds the pooled WAL read-connection set added by the
// ADR-004 read-path amendment (TASK-035/DEC-015, OQ-066): SQLite's WAL
// mode lets any number of readers run concurrently with the single writer
// (they never block on or contend with capture writes), so this pool
// exists purely to stop read-only public-surface queries from convoying
// through the single writer connection — it does not weaken the ratified
// single-WRITER invariant, which stays at one connection. Sized well above
// SYS-120's reference hardware ("commodity 4-core CPU") since it bounds
// concurrent in-flight QUERIES, not CPU cores; the per-meet render cache
// (internal/web) is what actually keeps read volume low under the SYS-122
// load profile — this pool is the "necessary but not sufficient" half of
// that fix (see OQ-066).
const readPoolSize = 16

// Store owns the one SQLite database per instance (ADR-004 §1): a
// writer-only single connection (db) for every transaction and mutating
// statement, plus a pooled set of read-only WAL connections (readDB) for
// read paths that do not need to participate in a write transaction.
type Store struct {
	db     *sql.DB
	readDB *sql.DB
	path   string
}

// sqliteDSN builds the file: DSN this package always opens with: WAL mode,
// full synchronous durability (SYS-081) and a 10s busy timeout, plus
// whatever extra query-string pragmas the caller appends (e.g. the read
// pool adds _query_only(1)).
func sqliteDSN(path, extraPragmas string) string {
	return fmt.Sprintf("file:%s?%s%s", url.PathEscape(filepath.ToSlash(path)),
		"_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(10000)",
		extraPragmas)
}

// Open opens (creating if needed) the database at path, applies the durability
// pragmas (WAL, synchronous=FULL — SYS-081), runs pending migrations, and runs
// the startup consistency check. Per ADR-004 §1 the check must report green
// before the application accepts writes, so a red check fails Open.
func Open(ctx context.Context, path string) (*Store, error) {
	db, err := sql.Open("sqlite", sqliteDSN(path, ""))
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	// Single-writer concurrency model (ADR-004 §2): all writes flow through
	// this process over this one connection — it is now writer-only, not
	// the sole connection to the database (ADR-004 read-path amendment,
	// TASK-035): read-only public-surface queries use readDB below instead.
	db.SetMaxOpenConns(1)

	// Pooled WAL read-connection set (ADR-004 read-path amendment,
	// TASK-035/OQ-066): a second connection pool to the same file,
	// query_only-pragma'd as a belt-and-braces guard against a read path
	// accidentally issuing a write (it would fail loudly rather than
	// silently racing the writer connection). WAL mode is a persistent,
	// file-level property once the writer connection above has enabled it,
	// so these connections see it immediately.
	readDB, err := sql.Open("sqlite", sqliteDSN(path, "&_pragma=query_only(1)"))
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open %s (read pool): %w", path, err)
	}
	readDB.SetMaxOpenConns(readPoolSize)

	s := &Store{db: db, readDB: readDB, path: path}
	if _, err := Migrate(ctx, db, Migrations()); err != nil {
		_ = s.Close()
		return nil, fmt.Errorf("migrate %s: %w", path, err)
	}
	report, err := s.Check(ctx)
	if err != nil {
		_ = s.Close()
		return nil, fmt.Errorf("consistency check %s: %w", path, err)
	}
	if !report.OK() {
		_ = s.Close()
		return nil, fmt.Errorf("consistency check %s: %s", path, report.Summary())
	}
	return s, nil
}

// DB exposes the writer connection to the app layer (architecture.md §3:
// web never sees this — it goes through app). Every transaction and
// mutating statement MUST go through this handle — it is the one
// connection the ADR-004 §2 single-writer invariant is defined over.
func (s *Store) DB() *sql.DB { return s.db }

// ReadDB exposes the pooled, query_only WAL read-connection set (ADR-004
// read-path amendment, TASK-035/OQ-066) to the app layer, for read-only
// public-surface queries that must not convoy behind DB()'s single writer
// connection. Never use this for a transaction or a write — the
// query_only pragma rejects them, and even without it doing so would
// bypass the single-writer invariant's ordering guarantees.
func (s *Store) ReadDB() *sql.DB { return s.readDB }

// Path returns the database file location (used by backup, TASK-014).
func (s *Store) Path() string { return s.path }

// Close closes both the writer connection and the read pool. It tolerates
// either being nil (Open closes a partially constructed Store on its own
// error paths before both are always set).
func (s *Store) Close() error {
	var errs []error
	if s.readDB != nil {
		if err := s.readDB.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if s.db != nil {
		if err := s.db.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
