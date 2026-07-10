// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"errors"
	"testing"

	"github.com/kriegalex/bahnfrei/internal/domain"
	"github.com/kriegalex/bahnfrei/internal/store"
)

// TestPermissionMatrixSweepSYS090UC022_3 sweeps every role × capability
// pair against an independently written expectation (each role's rank in
// SYS-090's least-privilege ordering vs. each capability's documented
// minimum role) rather than re-deriving it from capabilityMinRole, so a
// regression in either the map or Authorize's logic is caught (UC-022 #3:
// "outcomes match the documented matrix exactly").
func TestPermissionMatrixSweepSYS090UC022_3(t *testing.T) {
	roleRank := map[Role]int{
		RolePublic: 0, RoleEntrySubmitter: 1, RoleFieldOfficial: 2,
		RoleCompetitionOffice: 3, RoleMeetOrganizer: 4, RoleInstanceAdmin: 5,
	}
	// Documented minimum rank per capability (role.go doc comments, SYS-090).
	capMinRank := map[Capability]int{
		CapViewPublicResult: 0,
		CapSubmitEntries:    1,
		CapCaptureResults:   2,
		CapOfficeActions:    3,
		CapAssignUnits:      3,
		CapViewAudit:        3,
		CapOrganizeMeet:     4,
		CapManageAccounts:   5,
	}
	for role, rRank := range roleRank {
		for cap, cRank := range capMinRank {
			want := rRank >= cRank
			got := Authorize(role, cap) == nil
			if got != want {
				t.Errorf("Authorize(%q, %q) allowed=%v, want %v (role rank %d, capability min rank %d)",
					role, cap, got, want, rRank, cRank)
			}
		}
	}
}

// TestFieldOfficialEventScopeDeniedSYS090UC022_1 is the denial half of
// SYS-090's per-event scoping: an account with exactly RoleFieldOfficial
// that has never been granted this unit is refused on every capture entry
// point, and — UC-022 #1's "the attempt logged" — each denial itself lands
// in the audit trail.
func TestFieldOfficialEventScopeDeniedSYS090UC022_1(t *testing.T) {
	meets, results, st := newTestResults(t)
	ctx := context.Background()
	rec := createUKCMeet(t, meets)
	unitID := unitOf(t, results, meets, rec.ID, "ZoneLJ") // grants the shared fieldOfficial, not this one
	anna := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2014, Sex: domain.SexFemale, Bib: "101",
	})

	unassigned := Session{AccountID: "01UNASSIGNED", Username: "unassigned", Role: RoleFieldOfficial}

	if _, err := results.SaveFieldAttempt(ctx, unassigned, rec.ID, unitID, FieldAttemptInput{
		AthleteID: anna.AthleteID, Seq: 1, Kind: domain.AttemptValid, Mark: "3.00",
	}); !errors.Is(err, ErrUnitNotAssigned) {
		t.Errorf("SaveFieldAttempt by an unassigned field official = %v, want ErrUnitNotAssigned", err)
	}
	if _, err := results.CheckoutUnit(ctx, unassigned, rec.ID, unitID, "tablet-X"); !errors.Is(err, ErrUnitNotAssigned) {
		t.Errorf("CheckoutUnit by an unassigned field official = %v, want ErrUnitNotAssigned", err)
	}
	if err := results.CheckUnitAccess(ctx, unassigned, rec.ID, unitID); !errors.Is(err, ErrUnitNotAssigned) {
		t.Errorf("CheckUnitAccess for an unassigned field official = %v, want ErrUnitNotAssigned", err)
	}

	trail, err := store.AuditTrail(ctx, st.DB(), "unit", unitID)
	if err != nil {
		t.Fatal(err)
	}
	var denied int
	for _, e := range trail {
		if e.Action == "capture.access_denied" && e.Actor == unassigned.AccountID {
			denied++
		}
	}
	if denied < 2 {
		t.Errorf("audit trail for unit %s has %d capture.access_denied entries for %s, want at least 2",
			unitID, denied, unassigned.AccountID)
	}
}

// TestFieldOfficialEventScopeAllowedSYS090UC022_1 is the allow half: a
// field official granted this unit captures normally.
func TestFieldOfficialEventScopeAllowedSYS090UC022_1(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec := createUKCMeet(t, meets)
	unitID := unitOf(t, results, meets, rec.ID, "ZoneLJ") // grants fieldOfficial
	anna := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2014, Sex: domain.SexFemale, Bib: "101",
	})

	if _, err := results.SaveFieldAttempt(ctx, fieldOfficial, rec.ID, unitID, FieldAttemptInput{
		AthleteID: anna.AthleteID, Seq: 1, Kind: domain.AttemptValid, Mark: "3.10",
	}); err != nil {
		t.Errorf("assigned field official SaveFieldAttempt = %v, want success", err)
	}
	if err := results.CheckUnitAccess(ctx, fieldOfficial, rec.ID, unitID); err != nil {
		t.Errorf("CheckUnitAccess for an assigned unit = %v, want nil", err)
	}
}

// TestOfficeAndAboveNotScopedByEventSYS090 proves SYS-090's scoping applies
// only to "field/event official" — competition office and above capture
// normally on a unit they were never explicitly granted.
func TestOfficeAndAboveNotScopedByEventSYS090(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec := createUKCMeet(t, meets)
	unitID := unitOf(t, results, meets, rec.ID, "60m")
	anna := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2014, Sex: domain.SexFemale, Bib: "101",
	})

	if _, err := results.SaveTrackResult(ctx, office, rec.ID, unitID, TrackResultInput{
		AthleteID: anna.AthleteID, Time: "9.50", Timing: domain.TimingManual,
	}); err != nil {
		t.Errorf("office capture on an unassigned unit = %v, want success (not unit-scoped)", err)
	}
}

// TestFieldOfficialAssignmentRequiresFieldOfficialRoleSYS090 exercises the
// grant/revoke lifecycle itself: only office level and above may grant
// (CapAssignUnits), and only onto an account that actually carries
// RoleFieldOfficial.
func TestFieldOfficialAssignmentRequiresFieldOfficialRoleSYS090(t *testing.T) {
	meets, results, st := newTestResults(t)
	ctx := context.Background()
	rec := createUKCMeet(t, meets)
	unitID := unitOf(t, results, meets, rec.ID, "ZoneLJ")

	fo, err := store.CreateAccount(ctx, st.DB(), store.Account{
		Username: "fo2", DisplayName: "Field Official Two", PasswordHash: "x", Role: string(RoleFieldOfficial),
	})
	if err != nil {
		t.Fatal(err)
	}
	notFO, err := store.CreateAccount(ctx, st.DB(), store.Account{
		Username: "org2", DisplayName: "Organizer Two", PasswordHash: "x", Role: string(RoleMeetOrganizer),
	})
	if err != nil {
		t.Fatal(err)
	}

	var forbidden ErrForbidden
	if err := results.AssignFieldOfficialUnit(ctx, fieldOfficial, rec.ID, unitID, fo.ID); !errors.As(err, &forbidden) {
		t.Errorf("AssignFieldOfficialUnit by a field official = %v, want ErrForbidden", err)
	}
	if err := results.AssignFieldOfficialUnit(ctx, office, rec.ID, unitID, notFO.ID); !errors.Is(err, ErrNotFieldOfficial) {
		t.Errorf("assigning a non-field-official account = %v, want ErrNotFieldOfficial", err)
	}
	if err := results.AssignFieldOfficialUnit(ctx, office, rec.ID, unitID, fo.ID); err != nil {
		t.Fatalf("office AssignFieldOfficialUnit: %v", err)
	}

	foSession := Session{AccountID: fo.ID, Role: RoleFieldOfficial}
	if err := results.CheckUnitAccess(ctx, foSession, rec.ID, unitID); err != nil {
		t.Errorf("newly assigned official CheckUnitAccess = %v, want nil", err)
	}

	rows, err := results.FieldOfficialAssignments(ctx, office, rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	var listed bool
	for _, row := range rows {
		if row.AccountID == fo.ID {
			listed = true
			if !row.AssignedUnitIDs[unitID] {
				t.Errorf("FieldOfficialAssignments row for %s missing unit %s", fo.ID, unitID)
			}
		}
		if row.AccountID == notFO.ID {
			t.Errorf("FieldOfficialAssignments must only list RoleFieldOfficial accounts, found %s", notFO.ID)
		}
	}
	if !listed {
		t.Error("FieldOfficialAssignments must list the newly assigned official")
	}

	if err := results.UnassignFieldOfficialUnit(ctx, office, rec.ID, unitID, fo.ID); err != nil {
		t.Fatalf("UnassignFieldOfficialUnit: %v", err)
	}
	if err := results.CheckUnitAccess(ctx, foSession, rec.ID, unitID); !errors.Is(err, ErrUnitNotAssigned) {
		t.Errorf("after unassign, CheckUnitAccess = %v, want ErrUnitNotAssigned", err)
	}
}

// TestAccountLifecycleCreateDisableEnableRoleChangeSYS090UC022_2 walks the
// PoC account admin flow (create, disable, re-enable, role change) and
// proves every privileged action is written to the audit trail (SYS-046,
// UC-022 #2), plus that disabling revokes a live session and blocks login
// immediately (SYS-091).
func TestAccountLifecycleCreateDisableEnableRoleChangeSYS090UC022_2(t *testing.T) {
	auth := newTestAuth(t)
	ctx := context.Background()
	admin, err := auth.Bootstrap(ctx, "admin", "Administrator", "s3cret-passphrase")
	if err != nil {
		t.Fatal(err)
	}
	adminSession := Session{AccountID: admin.ID, Role: RoleInstanceAdmin}

	created, err := auth.CreateAccount(ctx, adminSession, CreateAccountRequest{
		Username: "office1", DisplayName: "Office One", Password: "p4ssword-here", Role: RoleCompetitionOffice,
	})
	if err != nil {
		t.Fatal(err)
	}

	sess, err := auth.Login(ctx, "office1", "p4ssword-here")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auth.SetAccountEnabled(ctx, adminSession, created.ID, false, "left the venue"); err != nil {
		t.Fatalf("SetAccountEnabled(false): %v", err)
	}
	if _, err := auth.CurrentSession(sess.Token); !errors.Is(err, ErrSessionNotFound) {
		t.Error("disabling an account must revoke its live sessions immediately (SYS-091)")
	}
	if _, err := auth.Login(ctx, "office1", "p4ssword-here"); !errors.Is(err, ErrAccountDisabled) {
		t.Errorf("login on a disabled account = %v, want ErrAccountDisabled", err)
	}

	if _, err := auth.SetAccountEnabled(ctx, adminSession, created.ID, true, ""); err != nil {
		t.Fatalf("SetAccountEnabled(true): %v", err)
	}
	if _, err := auth.Login(ctx, "office1", "p4ssword-here"); err != nil {
		t.Errorf("login after re-enable: %v", err)
	}

	updated, err := auth.ChangeAccountRole(ctx, adminSession, created.ID, RoleMeetOrganizer, "promoted")
	if err != nil {
		t.Fatalf("ChangeAccountRole: %v", err)
	}
	if updated.Role != string(RoleMeetOrganizer) {
		t.Errorf("role after change = %q, want %q", updated.Role, RoleMeetOrganizer)
	}

	trail, err := store.AuditTrail(ctx, auth.db, "account", created.ID)
	if err != nil {
		t.Fatal(err)
	}
	wantActions := map[string]bool{
		"account.create": false, "account.disable": false, "account.enable": false, "account.role_change": false,
	}
	for _, e := range trail {
		if _, ok := wantActions[e.Action]; ok {
			wantActions[e.Action] = true
		}
	}
	for action, seen := range wantActions {
		if !seen {
			t.Errorf("audit trail for account %s missing %q", created.ID, action)
		}
	}

	accounts, err := auth.ListAccounts(ctx, adminSession)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, a := range accounts {
		if a.ID == created.ID {
			found = true
			if !a.Enabled || a.Role != string(RoleMeetOrganizer) {
				t.Errorf("ListAccounts row = %+v, want enabled meet_organizer", a)
			}
		}
	}
	if !found {
		t.Error("ListAccounts must include the created account")
	}

	nonAdmin := Session{AccountID: "01X", Role: RoleFieldOfficial}
	if _, err := auth.ListAccounts(ctx, nonAdmin); err == nil {
		t.Error("ListAccounts by a non-admin should be forbidden")
	}
}

// TestAccountChangeRoleAndDisableRefuseLastAdminSYS090 guards SYS-090's
// least-privilege model against a self-inflicted lockout: the only enabled
// instance-admin account cannot be demoted or disabled, but the same
// action succeeds once a second enabled admin exists.
func TestAccountChangeRoleAndDisableRefuseLastAdminSYS090(t *testing.T) {
	auth := newTestAuth(t)
	ctx := context.Background()
	admin, err := auth.Bootstrap(ctx, "admin", "Administrator", "s3cret-passphrase")
	if err != nil {
		t.Fatal(err)
	}
	adminSession := Session{AccountID: admin.ID, Role: RoleInstanceAdmin}

	if _, err := auth.ChangeAccountRole(ctx, adminSession, admin.ID, RoleMeetOrganizer, "oops"); !errors.Is(err, ErrLastEnabledAdmin) {
		t.Errorf("demoting the last enabled admin = %v, want ErrLastEnabledAdmin", err)
	}
	if _, err := auth.SetAccountEnabled(ctx, adminSession, admin.ID, false, "oops"); !errors.Is(err, ErrLastEnabledAdmin) {
		t.Errorf("disabling the last enabled admin = %v, want ErrLastEnabledAdmin", err)
	}

	if _, err := auth.CreateAccount(ctx, adminSession, CreateAccountRequest{
		Username: "admin2", DisplayName: "Administrator Two", Password: "p4ssword-here", Role: RoleInstanceAdmin,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.ChangeAccountRole(ctx, adminSession, admin.ID, RoleMeetOrganizer, "handing off"); err != nil {
		t.Errorf("demoting the first admin once a second enabled admin exists: %v", err)
	}
}

// TestPrivilegedAuditLogSurfacesPrivilegedActionsOnlySYS091UC022_2 proves
// the office/organizer-visible audit surface (UC-022 #2): privileged
// account actions appear, the view is gated by CapViewAudit, and it is not
// meant to (nor does it) require the caller to reach into the raw table.
func TestPrivilegedAuditLogSurfacesPrivilegedActionsOnlySYS091UC022_2(t *testing.T) {
	auth := newTestAuth(t)
	ctx := context.Background()
	admin, err := auth.Bootstrap(ctx, "admin", "Administrator", "s3cret-passphrase")
	if err != nil {
		t.Fatal(err)
	}
	adminSession := Session{AccountID: admin.ID, Role: RoleInstanceAdmin}

	created, err := auth.CreateAccount(ctx, adminSession, CreateAccountRequest{
		Username: "office1", DisplayName: "Office One", Password: "p4ssword-here", Role: RoleCompetitionOffice,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auth.SetAccountEnabled(ctx, adminSession, created.ID, false, "test"); err != nil {
		t.Fatal(err)
	}

	events, err := auth.PrivilegedAuditLog(ctx, adminSession, 50)
	if err != nil {
		t.Fatal(err)
	}
	var sawCreate, sawDisable bool
	for _, e := range events {
		switch {
		case e.Action == "account.create" && e.EntityID == created.ID:
			sawCreate = true
			if e.ActorName != "Administrator" {
				t.Errorf("event actor name = %q, want the actor's resolved display name", e.ActorName)
			}
		case e.Action == "account.disable" && e.EntityID == created.ID:
			sawDisable = true
		}
	}
	if !sawCreate || !sawDisable {
		t.Errorf("privileged audit log missing account.create/disable for %s (sawCreate=%v sawDisable=%v)",
			created.ID, sawCreate, sawDisable)
	}

	officeSession := Session{AccountID: created.ID, Role: RoleCompetitionOffice}
	if _, err := auth.PrivilegedAuditLog(ctx, officeSession, 50); err != nil {
		t.Errorf("office level should be able to view the audit log: %v", err)
	}
	fieldSession := Session{AccountID: "01FLD2", Role: RoleFieldOfficial}
	if _, err := auth.PrivilegedAuditLog(ctx, fieldSession, 50); err == nil {
		t.Error("PrivilegedAuditLog by a field official should be forbidden (CapViewAudit)")
	}
}
