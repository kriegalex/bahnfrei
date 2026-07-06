// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Kill-9 durability harness (UC-020 #1–#2, SYS-081). A child copy of this
// test binary writes rows in a tight loop — each in one transaction pairing a
// data row with its audit entry — and prints "confirmed <id>" only after
// Commit returns (synchronous=FULL: durable). The parent SIGKILLs it
// mid-burst, reopens the database (which runs the consistency check), and
// verifies every confirmed write survived with no partially applied records.
//
// Scope: SIGKILL proves confirmed writes are not trapped in process buffers
// and that recovery sees no partial transactions. It does NOT prove media
// durability — the OS page cache survives a kill, and on tmpfs (dev /tmp)
// fsync is a no-op, so synchronous=FULL's disk sync is exercised only where
// TMPDIR is real disk (the CI runners). The genuine power-loss drill on the
// assembled application is TASK-014 (UC-020 #3 scope).

const (
	crashChildEnv = "BAHNFREI_CRASH_CHILD"
	crashDBEnv    = "BAHNFREI_CRASH_DB"
	confirmations = 50
)

// crashSetup prepares the paired data table the harness writes to.
func crashSetup(s *Store) error {
	_, err := s.DB().Exec(`CREATE TABLE IF NOT EXISTS crash_rows (
		id TEXT PRIMARY KEY, n INTEGER NOT NULL
	)`)
	return err
}

// crashWrite performs one confirmed write: a data row and its audit entry in
// the same transaction. It returns the id only after Commit — which, with
// synchronous=FULL, means durable on disk (SYS-081).
func crashWrite(ctx context.Context, s *Store, n int) (string, error) {
	id := NewID()
	tx, err := s.DB().BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO crash_rows (id, n) VALUES (?, ?)`, id, n); err != nil {
		_ = tx.Rollback()
		return "", fmt.Errorf("insert: %w", err)
	}
	if _, err := AppendAudit(ctx, tx, AuditEntry{
		Actor: "crash-harness", Action: "crash.write",
		EntityType: "crash_row", EntityID: id,
	}); err != nil {
		_ = tx.Rollback()
		return "", fmt.Errorf("audit: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit: %w", err)
	}
	return id, nil
}

// TestCrashWriteUnit exercises the harness write path in-process (the child
// subprocess below runs outside coverage collection).
func TestCrashWriteUnit(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	if err := crashSetup(s); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := crashWrite(ctx, s, i); err != nil {
			t.Fatalf("crashWrite #%d: %v", i, err)
		}
	}
	var rows int
	if err := s.DB().QueryRow(`SELECT count(*) FROM crash_rows`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 3 {
		t.Errorf("rows = %d, want 3", rows)
	}
}

func TestCrashChildProcess(t *testing.T) {
	if os.Getenv(crashChildEnv) != "1" {
		t.Skip("harness child — driven by TestKill9Durability")
	}
	ctx := context.Background()
	s, err := Open(ctx, os.Getenv(crashDBEnv))
	if err != nil {
		fmt.Printf("child-error open: %v\n", err)
		os.Exit(2)
	}
	if err := crashSetup(s); err != nil {
		fmt.Printf("child-error ddl: %v\n", err)
		os.Exit(2)
	}
	for n := 0; ; n++ {
		id, err := crashWrite(ctx, s, n)
		if err != nil {
			fmt.Printf("child-error %v\n", err)
			os.Exit(2)
		}
		fmt.Printf("confirmed %s\n", id) // only after durable commit
	}
}

func TestKill9Durability(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crash.db")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=TestCrashChildProcess$")
	cmd.Env = append(os.Environ(), crashChildEnv+"=1", crashDBEnv+"="+dbPath)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	confirmed := make([]string, 0, confirmations)
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "child-error") {
			t.Fatalf("harness child failed: %s", line)
		}
		if id, ok := strings.CutPrefix(line, "confirmed "); ok {
			confirmed = append(confirmed, id)
			if len(confirmed) >= confirmations {
				break // kill mid-burst, child is still writing
			}
		}
	}
	if len(confirmed) < confirmations {
		t.Fatalf("child produced only %d confirmations (scanner err: %v)",
			len(confirmed), scanner.Err())
	}
	if err := cmd.Process.Kill(); err != nil { // SIGKILL / TerminateProcess
		t.Fatal(err)
	}
	_ = cmd.Wait() // expected to report the kill

	// UC-020 #1: recover and be operational — Open replays the WAL and runs
	// the consistency check; a red check would fail it.
	start := time.Now()
	ctx := context.Background()
	s, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("reopen after kill -9: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if elapsed := time.Since(start); elapsed > 2*time.Minute {
		t.Errorf("recovery took %v, budget is 2 minutes (UC-020 #1)", elapsed)
	}

	// Every confirmed write is present…
	for _, id := range confirmed {
		var n int
		if err := s.DB().QueryRow(
			`SELECT count(*) FROM crash_rows WHERE id = ?`, id).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Errorf("confirmed write %s lost after kill -9 (SYS-081)", id)
		}
	}
	// …and no partially applied records: each data row has exactly its
	// paired audit entry (UC-020 #2).
	var rows, audits int
	if err := s.DB().QueryRow(`SELECT count(*) FROM crash_rows`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if err := s.DB().QueryRow(
		`SELECT count(*) FROM audit_log WHERE action = 'crash.write'`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if rows != audits {
		t.Errorf("partial transaction visible: %d data rows vs %d audit rows", rows, audits)
	}
	if rows < len(confirmed) {
		t.Errorf("total rows %d < confirmed %d", rows, len(confirmed))
	}
}
