// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/kriegalex/bahnfrei/internal/store"
)

// backupFlags holds the parsed "backup" subcommand flags.
type backupFlags struct {
	dataDir string
	out     string
}

func parseBackupFlags(args []string, out io.Writer) (backupFlags, error) {
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	fs.SetOutput(out)
	dataDir := fs.String("data-dir", ".", "directory holding the running instance's database")
	dest := fs.String("out", "", "path to write the backup artifact to (required)")
	if err := fs.Parse(args); err != nil {
		return backupFlags{}, err
	}
	if *dest == "" {
		return backupFlags{}, fmt.Errorf("--out is required")
	}
	return backupFlags{dataDir: *dataDir, out: *dest}, nil
}

// runBackup implements the "backup" subcommand (SYS-084, UC-020 #3): a
// single operator action producing one portable, self-contained snapshot
// of the whole instance database, independent of the office-UI download
// (/admin/backup) — useful for cron/scripted backups where no browser is
// involved.
func runBackup(ctx context.Context, args []string, out io.Writer) error {
	cfg, err := parseBackupFlags(args, out)
	if err != nil {
		return err
	}

	dbPath := filepath.Join(cfg.dataDir, "bahnfrei.db")
	st, err := store.Open(ctx, dbPath)
	if err != nil {
		return fmt.Errorf("open store at %s: %w", dbPath, err)
	}
	defer func() { _ = st.Close() }()

	manifest, err := st.Backup(ctx, cfg.out, version)
	if err != nil {
		return fmt.Errorf("backup: %w", err)
	}

	fmt.Fprintf(out, "bahnfrei: backup written to %s\n", cfg.out)
	fmt.Fprintf(out, "  created:  %s\n", manifest.CreatedAt.Format("2006-01-02T15:04:05Z"))
	fmt.Fprintf(out, "  meets:    %d\n", manifest.MeetCount)
	fmt.Fprintf(out, "  results:  %d\n", manifest.ResultCount)
	fmt.Fprintf(out, "  checksum: %s\n", manifest.Checksum)
	return nil
}

// restoreFlags holds the parsed "restore" subcommand flags.
type restoreFlags struct {
	dataDir string
	from    string
}

func parseRestoreFlags(args []string, out io.Writer) (restoreFlags, error) {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	fs.SetOutput(out)
	dataDir := fs.String("data-dir", ".", "directory for the restored instance's database (must not already contain one)")
	from := fs.String("from", "", "path to a backup artifact produced by \"bahnfrei backup\" (required)")
	if err := fs.Parse(args); err != nil {
		return restoreFlags{}, err
	}
	if *from == "" {
		return restoreFlags{}, fmt.Errorf("--from is required")
	}
	return restoreFlags{dataDir: *dataDir, from: *from}, nil
}

// runRestore implements the "restore" subcommand (UC-020 #3: "restored
// onto a fresh installation"). It refuses to run over an existing
// database — a backup artifact is a complete instance database (SYS-084),
// so restoring is placing it at a fresh install's data directory, never
// merging into one that already has data.
func runRestore(ctx context.Context, args []string, out io.Writer) error {
	cfg, err := parseRestoreFlags(args, out)
	if err != nil {
		return err
	}

	dbPath := filepath.Join(cfg.dataDir, "bahnfrei.db")
	if _, err := os.Stat(dbPath); err == nil {
		return fmt.Errorf("restore refused: %s already has a database — restore only supports a fresh install (use an empty --data-dir)", dbPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", dbPath, err)
	}

	if err := os.MkdirAll(cfg.dataDir, 0o755); err != nil {
		return fmt.Errorf("create data dir %s: %w", cfg.dataDir, err)
	}
	data, err := os.ReadFile(cfg.from)
	if err != nil {
		return fmt.Errorf("read backup artifact %s: %w", cfg.from, err)
	}
	if err := os.WriteFile(dbPath, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", dbPath, err)
	}

	// Opening runs migrations (a no-op for an already-current artifact) and
	// the startup consistency check (SYS-081/130) — restore is verified,
	// not just copied.
	st, err := store.Open(ctx, dbPath)
	if err != nil {
		return fmt.Errorf("open restored database: %w", err)
	}
	defer func() { _ = st.Close() }()

	meets, err := store.ListMeets(ctx, st.DB())
	if err != nil {
		return fmt.Errorf("list restored meets: %w", err)
	}
	fmt.Fprintf(out, "bahnfrei: restored %s into %s (%d meet(s), consistency check green)\n",
		cfg.from, dbPath, len(meets))
	if manifest, err := store.ReadBackupManifest(ctx, st.DB()); err == nil {
		fmt.Fprintf(out, "  backup created: %s\n", manifest.CreatedAt.Format("2006-01-02T15:04:05Z"))
		fmt.Fprintf(out, "  checksum:       %s\n", manifest.Checksum)
	}
	return nil
}
