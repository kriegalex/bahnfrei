// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/kriegalex/bahnfrei/internal/store"
)

// TestBackupServiceAuthorizationSYS084 gates the one-action backup at
// instance-admin level (UC-020's actor is "operator (admin)") — anything
// below is refused, never silently downgraded to a partial export.
func TestBackupServiceAuthorizationSYS084(t *testing.T) {
	_, st := newTestMeets(t)
	backup := NewBackupService(st)
	dest := filepath.Join(t.TempDir(), "out.db")

	for _, r := range []Role{RolePublic, RoleEntrySubmitter, RoleFieldOfficial, RoleCompetitionOffice, RoleMeetOrganizer} {
		actor := Session{AccountID: "01X", Username: "x", Role: r}
		if _, err := backup.Backup(context.Background(), actor, dest, "v"); !errors.As(err, &ErrForbidden{}) {
			t.Errorf("role %q: Backup err = %v, want ErrForbidden", r, err)
		}
	}
}

// TestBackupServiceProducesAuditedArtifactSYS084 covers the "single
// operator action" half of SYS-084: an instance admin triggers one backup,
// gets back a manifest, and the action is recorded to the audit trail
// (SYS-046 — any privileged action).
func TestBackupServiceProducesAuditedArtifactSYS084(t *testing.T) {
	meets, st := newTestMeets(t)
	ctx := context.Background()
	if _, err := meets.CreateMeet(ctx, organizer, MeetRequest{
		Name: "Admin Backup Meet", Venue: "V",
		StartDate: meetDay(0), EndDate: meetDay(0), Tier: "C-Meeting",
		CategorySchemeID: "swiss-athletics",
	}); err != nil {
		t.Fatalf("CreateMeet: %v", err)
	}

	backup := NewBackupService(st)
	admin := Session{AccountID: "01ADM", Username: "admin", Role: RoleInstanceAdmin}
	dest := filepath.Join(t.TempDir(), "out.db")

	manifest, err := backup.Backup(ctx, admin, dest, "test-build")
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	if manifest.MeetCount != 1 {
		t.Errorf("manifest.MeetCount = %d, want 1", manifest.MeetCount)
	}
	if manifest.AppVersion != "test-build" {
		t.Errorf("manifest.AppVersion = %q", manifest.AppVersion)
	}

	trail, err := store.AuditTrail(ctx, st.DB(), "instance", "backup")
	if err != nil {
		t.Fatal(err)
	}
	if len(trail) != 1 || trail[0].Action != "backup.create" || trail[0].Actor != admin.AccountID {
		t.Errorf("audit trail = %+v, want one backup.create by %s", trail, admin.AccountID)
	}
}
