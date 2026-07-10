// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/kriegalex/bahnfrei/internal/domain"
)

// testBackupMeet builds a minimal valid meet for the backup tests; day() is
// meet_test.go's existing date helper (2027-06-12-style calls), shared
// within the package.
func testBackupMeet(name string) domain.Meet {
	return domain.Meet{
		Name: name, Venue: "Teststadion",
		StartDate: day(2027, 5, 1), EndDate: day(2027, 5, 1),
		Organizer: "tester", Tier: "C-Meeting", CategorySchemeID: "swiss-athletics",
	}
}

// TestBackupProducesConsistentSnapshotSYS084 exercises the one-action
// backup at the storage layer (SYS-084): the artifact is a standalone,
// openable SQLite database containing the meet metadata, and it carries a
// manifest (row counts + checksum) that a fresh, independent computation
// over the same artifact reproduces exactly.
func TestBackupProducesConsistentSnapshotSYS084(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s := openTest(t)

	meet, err := CreateMeet(ctx, s.DB(), testBackupMeet("Backup Test Meeting"))
	if err != nil {
		t.Fatalf("CreateMeet: %v", err)
	}

	dest := filepath.Join(dir, "backup-1.db")
	manifest, err := s.Backup(ctx, dest, "test-version")
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	if manifest.MeetCount != 1 {
		t.Errorf("manifest.MeetCount = %d, want 1", manifest.MeetCount)
	}
	if manifest.AppVersion != "test-version" {
		t.Errorf("manifest.AppVersion = %q", manifest.AppVersion)
	}
	if manifest.Checksum == "" {
		t.Error("manifest.Checksum is empty")
	}
	if manifest.CreatedAt.IsZero() {
		t.Error("manifest.CreatedAt is zero")
	}

	// The artifact is a standalone, self-contained database: openable with
	// no reference back to the live database or its WAL/SHM files, and it
	// already contains the meet's metadata (SYS-084 "with meet metadata").
	dest2, err := sql.Open("sqlite", "file:"+dest)
	if err != nil {
		t.Fatalf("open artifact: %v", err)
	}
	defer func() { _ = dest2.Close() }()
	var name, venue string
	if err := dest2.QueryRowContext(ctx, `SELECT name, venue FROM meets WHERE id = ?`, meet.ID).
		Scan(&name, &venue); err != nil {
		t.Fatalf("read meet from artifact: %v", err)
	}
	if name != "Backup Test Meeting" || venue != "Teststadion" {
		t.Errorf("artifact meet = %q/%q, want the original meet's metadata", name, venue)
	}

	// The manifest is embedded in the artifact, not a sidecar.
	readBack, err := ReadBackupManifest(ctx, dest2)
	if err != nil {
		t.Fatalf("ReadBackupManifest: %v", err)
	}
	if readBack.Checksum != manifest.Checksum || readBack.MeetCount != manifest.MeetCount {
		t.Errorf("stamped manifest = %+v, want %+v", readBack, manifest)
	}

	// Recomputing the manifest from the artifact independently reproduces
	// the same checksum (UC-020 #3: "checksums equal" is a recomputation,
	// not just re-reading the stored claim).
	recomputed, err := ComputeManifest(ctx, dest2, "test-version")
	if err != nil {
		t.Fatalf("ComputeManifest: %v", err)
	}
	if recomputed.Checksum != manifest.Checksum {
		t.Errorf("recomputed checksum %q != original %q", recomputed.Checksum, manifest.Checksum)
	}
}

// TestBackupChecksumDetectsDivergenceSYS084 proves the checksum is not a
// tautology: two databases with different business data get different
// checksums.
func TestBackupChecksumDetectsDivergenceSYS084(t *testing.T) {
	ctx := context.Background()
	a := openTest(t)
	b := openTest(t)

	if _, err := CreateMeet(ctx, a.DB(), testBackupMeet("Meet A")); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateMeet(ctx, b.DB(), testBackupMeet("Meet B")); err != nil {
		t.Fatal(err)
	}

	ma, err := ComputeManifest(ctx, a.DB(), "v")
	if err != nil {
		t.Fatal(err)
	}
	mb, err := ComputeManifest(ctx, b.DB(), "v")
	if err != nil {
		t.Fatal(err)
	}
	if ma.Checksum == mb.Checksum {
		t.Error("distinct meet data produced the same checksum")
	}
}

// TestBackupArtifactOpensAsFreshStoreSYS084 documents the "fresh install"
// restore path this task assumes: a backup artifact IS a valid
// bahnfrei.db, so restoring is placing the artifact at the target data
// directory's database path — proven here by opening a copy of the
// artifact directly through Store.Open (migrations + the startup
// consistency check both pass with no special restore code required).
func TestBackupArtifactOpensAsFreshStoreSYS084(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s := openTest(t)
	if _, err := CreateMeet(ctx, s.DB(), testBackupMeet("Restorable Meet")); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(dir, "artifact.db")
	if _, err := s.Backup(ctx, dest, "v"); err != nil {
		t.Fatalf("Backup: %v", err)
	}

	// "Restore onto a fresh installation" = the artifact becomes the fresh
	// install's database file.
	freshInstallDB := filepath.Join(t.TempDir(), "bahnfrei.db")
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(freshInstallDB, data, 0o644); err != nil {
		t.Fatal(err)
	}

	restored, err := Open(ctx, freshInstallDB)
	if err != nil {
		t.Fatalf("Open restored artifact as fresh install: %v", err)
	}
	defer func() { _ = restored.Close() }()

	meets, err := ListMeets(ctx, restored.DB())
	if err != nil {
		t.Fatal(err)
	}
	if len(meets) != 1 || meets[0].Name != "Restorable Meet" {
		t.Errorf("restored meets = %+v, want one Restorable Meet", meets)
	}
}

// TestBackupRemovesExistingDestination ensures a stale leftover file at
// destPath (e.g. a retry after a prior failed backup) does not make
// VACUUM INTO fail — it refuses to overwrite an existing file, so Backup
// must clear it first.
func TestBackupRemovesExistingDestination(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s := openTest(t)
	if _, err := CreateMeet(ctx, s.DB(), testBackupMeet("Retry Meet")); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "artifact.db")
	if err := os.WriteFile(dest, []byte("stale leftover, not a database"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Backup(ctx, dest, "v"); err != nil {
		t.Fatalf("Backup over stale destination: %v", err)
	}
}
