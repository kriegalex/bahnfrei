// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func rawDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(t.TempDir(), "m.db")))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestMigrateAppliesInOrderOnce(t *testing.T) {
	ctx := context.Background()
	db := rawDB(t)
	fsys := fstest.MapFS{
		"0002_second.sql": {Data: []byte(`ALTER TABLE things ADD COLUMN color TEXT;`)},
		"0001_first.sql":  {Data: []byte(`CREATE TABLE things (id TEXT PRIMARY KEY);`)},
	}
	n, err := Migrate(ctx, db, fsys)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if n != 2 {
		t.Errorf("applied = %d, want 2", n)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO things (id, color) VALUES ('x', 'red')`); err != nil {
		t.Errorf("schema not as expected after ordered apply: %v", err)
	}

	// Second run: nothing to do.
	n, err = Migrate(ctx, db, fsys)
	if err != nil {
		t.Fatalf("re-Migrate: %v", err)
	}
	if n != 0 {
		t.Errorf("re-applied = %d, want 0", n)
	}

	// A new migration joins: only it runs.
	fsys["0003_third.sql"] = &fstest.MapFile{Data: []byte(`CREATE INDEX things_color ON things (color);`)}
	n, err = Migrate(ctx, db, fsys)
	if err != nil {
		t.Fatalf("incremental Migrate: %v", err)
	}
	if n != 1 {
		t.Errorf("applied = %d, want 1", n)
	}
}

func TestMigrateRejectsChangedHistory(t *testing.T) {
	ctx := context.Background()
	db := rawDB(t)
	if _, err := Migrate(ctx, db, fstest.MapFS{
		"0001_first.sql": {Data: []byte(`CREATE TABLE a (id TEXT);`)},
	}); err != nil {
		t.Fatal(err)
	}
	_, err := Migrate(ctx, db, fstest.MapFS{
		"0001_first.sql": {Data: []byte(`CREATE TABLE a (id TEXT, sneaky TEXT);`)},
	})
	if err == nil || !strings.Contains(err.Error(), "different content") {
		t.Errorf("changed applied migration not rejected, err = %v", err)
	}
}

func TestMigrateRejectsBadName(t *testing.T) {
	_, err := Migrate(context.Background(), rawDB(t), fstest.MapFS{
		"01_short.sql": {Data: []byte(`SELECT 1;`)},
	})
	if err == nil || !strings.Contains(err.Error(), "NNNN_name.sql") {
		t.Errorf("bad migration name not rejected, err = %v", err)
	}
}

func TestMigrateRollsBackFailedMigration(t *testing.T) {
	ctx := context.Background()
	db := rawDB(t)
	_, err := Migrate(ctx, db, fstest.MapFS{
		"0001_broken.sql": {Data: []byte(`CREATE TABLE ok (id TEXT); THIS IS NOT SQL;`)},
	})
	if err == nil {
		t.Fatal("broken migration did not error")
	}
	// The half-applied DDL must have been rolled back and not recorded.
	var cnt int
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM sqlite_master WHERE name = 'ok'`).Scan(&cnt); err != nil {
		t.Fatal(err)
	}
	if cnt != 0 {
		t.Error("failed migration left partial DDL behind")
	}
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM schema_migrations`).Scan(&cnt); err != nil {
		t.Fatal(err)
	}
	if cnt != 0 {
		t.Error("failed migration was recorded as applied")
	}
}
