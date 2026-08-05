// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"errors"
	"testing"

	"github.com/kriegalex/bahnfrei/internal/domain"
)

// TestAssignedMeetsSYS090DEC025TASK042 covers the field-official panel of
// the "my assignments" dashboard (TASK-042, DEC-025): only the meets/units
// TASK-013 scoped the actor to come back — never an unassigned unit of a
// scoped meet, nor a meet the actor holds no assignment in at all.
func TestAssignedMeetsSYS090DEC025TASK042(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	meetA := createUKCMeet(t, meets)
	_ = createUKCMeet(t, meets) // meetB: fieldOfficial holds no assignment here at all

	unitID := unitOf(t, results, meets, meetA.ID, "60m") // assigns fieldOfficial to this unit only

	got, err := results.AssignedMeets(ctx, fieldOfficial)
	if err != nil {
		t.Fatalf("AssignedMeets: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("AssignedMeets = %+v, want exactly the one meet holding an assignment", got)
	}
	if got[0].MeetID != meetA.ID {
		t.Errorf("MeetID = %q, want %q", got[0].MeetID, meetA.ID)
	}
	if len(got[0].Units) != 1 || got[0].Units[0].UnitID != unitID {
		t.Errorf("Units = %+v, want exactly [%q] — SYS-090: no other unit of the scoped meet leaks", got[0].Units, unitID)
	}
}

// TestAssignedMeetsNonFieldOfficialSYS090DEC025TASK042 documents the
// deliberate scope narrowing: SYS-090 per-event scoping applies only to
// RoleFieldOfficial, so any other authorized caller (e.g. competition
// office, which holds no field_official_units rows) gets an empty list
// here rather than an unfiltered dump — it has its own dashboard panel.
func TestAssignedMeetsNonFieldOfficialSYS090DEC025TASK042(t *testing.T) {
	_, results, _ := newTestResults(t)
	got, err := results.AssignedMeets(context.Background(), office)
	if err != nil {
		t.Fatalf("AssignedMeets(office): %v", err)
	}
	if len(got) != 0 {
		t.Errorf("AssignedMeets(office) = %+v, want empty (not RoleFieldOfficial)", got)
	}
}

func TestAssignedMeetsRequiresCaptureCapabilitySYS090DEC025TASK042(t *testing.T) {
	_, results, _ := newTestResults(t)
	var forbidden ErrForbidden
	public := Session{Role: RolePublic}
	if _, err := results.AssignedMeets(context.Background(), public); !errors.As(err, &forbidden) {
		t.Errorf("AssignedMeets(public) = %v, want ErrForbidden", err)
	}
}

// TestOfficeMeetsSYS090DEC025TASK042 covers the competition-office panel:
// SYS-090 gives that role no per-meet scoping, so every meet in the
// instance comes back.
func TestOfficeMeetsSYS090DEC025TASK042(t *testing.T) {
	meets, _ := newTestMeets(t)
	ctx := context.Background()
	m1, err := meets.CreateMeet(ctx, organizer, ucMeetRequest())
	if err != nil {
		t.Fatalf("CreateMeet: %v", err)
	}
	m2, err := meets.CreateMeet(ctx, organizer, ucMeetRequest())
	if err != nil {
		t.Fatalf("CreateMeet: %v", err)
	}

	got, err := meets.OfficeMeets(ctx, office)
	if err != nil {
		t.Fatalf("OfficeMeets: %v", err)
	}
	ids := map[string]bool{}
	for _, m := range got {
		ids[m.ID] = true
	}
	if !ids[m1.ID] || !ids[m2.ID] {
		t.Errorf("OfficeMeets = %+v, want both created meets (SYS-090: office holds no per-meet scoping)", got)
	}
}

func TestOfficeMeetsRequiresOfficeCapabilitySYS090DEC025TASK042(t *testing.T) {
	meets, _ := newTestMeets(t)
	var forbidden ErrForbidden
	if _, err := meets.OfficeMeets(context.Background(), fieldOfficial); !errors.As(err, &forbidden) {
		t.Errorf("OfficeMeets(fieldOfficial) = %v, want ErrForbidden", err)
	}
}

// TestSubmitterMeetsSYS090DEC025TASK042 covers the entry-submitter panel:
// only the meets/entries submitted_by attributes to actor come back, never
// another submitter's entries at the same meet.
func TestSubmitterMeetsSYS090DEC025TASK042(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec, err := meets.CreateMeet(ctx, organizer, ucMeetRequest())
	if err != nil {
		t.Fatalf("CreateMeet: %v", err)
	}
	ev, err := meets.AddEvent(ctx, organizer, rec.ID, AddEventRequest{
		DisciplineCode: "100m", CategoryCodes: []string{"U16 W"},
	})
	if err != nil {
		t.Fatalf("AddEvent: %v", err)
	}
	if err := meets.PublishMeet(ctx, organizer, rec.ID, rec.Version); err != nil {
		t.Fatalf("PublishMeet: %v", err)
	}

	otherSubmitter := Session{AccountID: "01SUB2", Username: "submitter2", Role: RoleEntrySubmitter}
	if _, err := results.SubmitIndividualEntry(ctx, entrySubmitter, rec.ID, IndividualEntryInput{
		EventID: ev.ID, FirstName: "Anna", LastName: "Muster", BirthYear: 2011,
		Sex: domain.SexFemale, Club: "LC Test", SeedPerformance: "13.50",
	}); err != nil {
		t.Fatalf("SubmitIndividualEntry(entrySubmitter): %v", err)
	}
	if _, err := results.SubmitIndividualEntry(ctx, otherSubmitter, rec.ID, IndividualEntryInput{
		EventID: ev.ID, FirstName: "Berta", LastName: "Beispiel", BirthYear: 2011,
		Sex: domain.SexFemale, Club: "LC Test", SeedPerformance: "14.00",
	}); err != nil {
		t.Fatalf("SubmitIndividualEntry(otherSubmitter): %v", err)
	}

	got, err := results.SubmitterMeets(ctx, entrySubmitter)
	if err != nil {
		t.Fatalf("SubmitterMeets: %v", err)
	}
	if len(got) != 1 || got[0].MeetID != rec.ID {
		t.Fatalf("SubmitterMeets = %+v, want exactly the one meet", got)
	}
	if len(got[0].Entries) != 1 || got[0].Entries[0].AthleteName != "Anna Muster" {
		t.Errorf("Entries = %+v, want exactly entrySubmitter's own entry, not otherSubmitter's (SYS-090)", got[0].Entries)
	}
}

func TestSubmitterMeetsRequiresSubmitCapabilitySYS090DEC025TASK042(t *testing.T) {
	_, results, _ := newTestResults(t)
	var forbidden ErrForbidden
	public := Session{Role: RolePublic}
	if _, err := results.SubmitterMeets(context.Background(), public); !errors.As(err, &forbidden) {
		t.Errorf("SubmitterMeets(public) = %v, want ErrForbidden", err)
	}
}
