// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"hash"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// backupManifestTable is the artifact-local provenance table Backup stamps
// into every snapshot it produces. It is deliberately not a tracked
// migration (schema_migrations): it exists only in backup artifacts, never
// in the live instance database, so restoring an artifact over a fresh
// install (which already has this table) does not collide with anything
// Migrate expects.
const backupManifestTable = "backup_manifest"

// systemTables are excluded from the manifest's structural checksum: they
// are either SQLite-internal, migration bookkeeping (immutable and
// re-derivable, not "meet data"), or the manifest table itself (which does
// not exist yet at the point the checksum it stores is computed).
var systemTables = map[string]bool{
	"schema_migrations": true,
	backupManifestTable: true,
}

// BackupManifest describes one backup artifact (SYS-084, UC-020 #3): when
// it was produced, by which build, and a content fingerprint (row counts
// per key table plus a whole-database checksum) that restore verification
// can recompute independently and compare, rather than trusting the
// artifact's own say-so.
type BackupManifest struct {
	CreatedAt   time.Time
	AppVersion  string
	MeetCount   int64
	ResultCount int64
	AuditRows   int64
	// Checksum is a SHA-256 over every business table's rows (every table
	// except schema_migrations/backup_manifest), each row canonicalized by
	// sorting on its own column values — not rowid, which VACUUM is free to
	// renumber (SQLite docs: rowids may change across VACUUM for tables
	// without an INTEGER PRIMARY KEY alias). Two databases with identical
	// business data produce the same checksum regardless of physical
	// layout.
	Checksum string
}

// Backup produces a single, self-contained, point-in-time-consistent
// snapshot of the entire instance database at destPath (SYS-084: "one
// portable artifact containing all meet data" — architecture.md §1 has
// one SQLite database per instance, so a full-database snapshot already
// contains every meet's data plus its metadata (meets, sessions, events,
// accounts, audit trail, …), not a separate sidecar).
//
// It uses SQLite's VACUUM INTO, not a raw file copy: VACUUM INTO takes its
// snapshot through the engine inside an implicit read transaction, so it
// can never observe a torn WAL checkpoint the way copying the live
// database/WAL/SHM files off disk could. The resulting file needs no WAL
// replay to open — it is a complete, standalone database from the moment
// VACUUM INTO returns.
//
// The manifest is stamped into the artifact itself (a backup_manifest
// table) so the single file the operator downloads carries its own
// provenance and verification data; there is no sidecar to lose.
func (s *Store) Backup(ctx context.Context, destPath, appVersion string) (BackupManifest, error) {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return BackupManifest{}, fmt.Errorf("backup: create destination dir: %w", err)
	}
	// VACUUM INTO refuses to write over an existing file; destPath is
	// expected to be a fresh path per backup (callers use a timestamped or
	// random name), but clear any stale leftover defensively.
	if err := os.Remove(destPath); err != nil && !os.IsNotExist(err) {
		return BackupManifest{}, fmt.Errorf("backup: clear destination: %w", err)
	}

	if _, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, destPath); err != nil {
		return BackupManifest{}, fmt.Errorf("backup: vacuum into %s: %w", destPath, err)
	}

	dest, err := sql.Open("sqlite", fmt.Sprintf("file:%s?%s",
		url.PathEscape(filepath.ToSlash(destPath)), "_pragma=busy_timeout(5000)"))
	if err != nil {
		_ = os.Remove(destPath)
		return BackupManifest{}, fmt.Errorf("backup: open snapshot: %w", err)
	}
	defer func() { _ = dest.Close() }()

	manifest, err := ComputeManifest(ctx, dest, appVersion)
	if err != nil {
		_ = os.Remove(destPath)
		return BackupManifest{}, fmt.Errorf("backup: compute manifest: %w", err)
	}
	manifest.CreatedAt = time.Now().UTC()

	if err := stampManifest(ctx, dest, manifest); err != nil {
		_ = os.Remove(destPath)
		return BackupManifest{}, fmt.Errorf("backup: stamp manifest: %w", err)
	}
	return manifest, nil
}

// ComputeManifest independently recomputes a BackupManifest's counts and
// checksum from db — the same computation Backup runs against the fresh
// snapshot, exposed so restore verification (UC-020 #3: "record counts,
// checksums equal") can recompute it against the *restored* database and
// compare, rather than only re-reading the value the artifact already
// carries.
func ComputeManifest(ctx context.Context, db *sql.DB, appVersion string) (BackupManifest, error) {
	tables, err := businessTables(ctx, db)
	if err != nil {
		return BackupManifest{}, err
	}

	h := sha256.New()
	for _, table := range tables {
		if err := hashTable(ctx, db, h, table); err != nil {
			return BackupManifest{}, fmt.Errorf("checksum table %s: %w", table, err)
		}
	}

	m := BackupManifest{AppVersion: appVersion, Checksum: fmt.Sprintf("%x", h.Sum(nil))}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM meets`).Scan(&m.MeetCount); err != nil {
		return BackupManifest{}, fmt.Errorf("count meets: %w", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM results`).Scan(&m.ResultCount); err != nil {
		return BackupManifest{}, fmt.Errorf("count results: %w", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM audit_log`).Scan(&m.AuditRows); err != nil {
		return BackupManifest{}, fmt.Errorf("count audit_log: %w", err)
	}
	return m, nil
}

// businessTables lists every user table in db except the excluded
// bookkeeping/manifest tables, sorted for a stable checksum order.
func businessTables(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		if systemTables[name] {
			continue
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

// hashTable writes a canonical representation of every row of table into h:
// the table name, then every row's column values in the table's own column
// order, rows sorted by their full value tuple (not rowid — see
// BackupManifest.Checksum) so the result is independent of physical
// storage order.
func hashTable(ctx context.Context, db *sql.DB, h hash.Hash, table string) error {
	// table is always one of our own migration-created names (never
	// user input), so interpolating it into the query is safe.
	colRows, err := db.QueryContext(ctx, fmt.Sprintf(`SELECT * FROM %q LIMIT 0`, table))
	if err != nil {
		return err
	}
	cols, err := colRows.Columns()
	colRows.Close()
	if err != nil {
		return err
	}

	orderBy := make([]string, len(cols))
	for i := range cols {
		orderBy[i] = strconv.Itoa(i + 1)
	}
	query := fmt.Sprintf(`SELECT * FROM %q ORDER BY %s`, table, strings.Join(orderBy, ","))
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()

	fmt.Fprintf(h, "TABLE %s\x1d", table)
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			return err
		}
		for _, v := range vals {
			if b, ok := v.([]byte); ok {
				v = string(b)
			}
			fmt.Fprintf(h, "%v\x1f", v)
		}
		h.Write([]byte("\x1e"))
	}
	return rows.Err()
}

// stampManifest creates (if needed) the backup_manifest table in db and
// inserts m as its single row.
func stampManifest(ctx context.Context, db *sql.DB, m BackupManifest) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS `+backupManifestTable+` (
		created_at   TEXT NOT NULL,
		app_version  TEXT NOT NULL,
		meet_count   INTEGER NOT NULL,
		result_count INTEGER NOT NULL,
		audit_rows   INTEGER NOT NULL,
		checksum     TEXT NOT NULL
	)`); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, `INSERT INTO `+backupManifestTable+`
		(created_at, app_version, meet_count, result_count, audit_rows, checksum)
		VALUES (?, ?, ?, ?, ?, ?)`,
		m.CreatedAt.Format(time.RFC3339Nano), m.AppVersion, m.MeetCount, m.ResultCount, m.AuditRows, m.Checksum)
	return err
}

// ReadBackupManifest reads the manifest an artifact was stamped with at
// backup time (as opposed to ComputeManifest, which recomputes one fresh).
// It is how the CLI and restore verification display/compare "what this
// artifact claims" against "what the data actually is".
func ReadBackupManifest(ctx context.Context, db *sql.DB) (BackupManifest, error) {
	var m BackupManifest
	var created string
	err := db.QueryRowContext(ctx, `SELECT created_at, app_version, meet_count, result_count, audit_rows, checksum
		FROM `+backupManifestTable).
		Scan(&created, &m.AppVersion, &m.MeetCount, &m.ResultCount, &m.AuditRows, &m.Checksum)
	if err != nil {
		return BackupManifest{}, fmt.Errorf("read backup manifest: %w", err)
	}
	if m.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return BackupManifest{}, fmt.Errorf("backup manifest: bad created_at %q: %w", created, err)
	}
	return m, nil
}
