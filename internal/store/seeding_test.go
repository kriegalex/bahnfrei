// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import (
	"context"
	"errors"
	"testing"

	"github.com/kriegalex/bahnfrei/internal/domain"
)

// seedingFixtureRound creates a meet with one event and one round (with its
// default placeholder unit) for unit-assignment storage tests.
func seedingFixtureRound(t *testing.T, s *Store) (domain.Round, UnitRecord) {
	t.Helper()
	ctx := context.Background()
	ev := entryFixtureEvent(t, s)
	round, err := CreateRound(ctx, s.DB(), domain.Round{EventID: ev.ID, Kind: domain.RoundQualification}, 0)
	if err != nil {
		t.Fatalf("CreateRound: %v", err)
	}
	unit, err := CreateUnit(ctx, s.DB(), domain.Unit{RoundID: round.ID})
	if err != nil {
		t.Fatalf("CreateUnit: %v", err)
	}
	return round, unit
}

func seedingFixtureEntry(t *testing.T, s *Store, ev EventRecord, athleteName string) EntryRecord {
	t.Helper()
	athlete := entryFixtureAthlete(t, s, athleteName)
	rec, err := CreateEntry(context.Background(), s.DB(), domain.Entry{
		EventID: ev.ID, AthleteID: athlete.ID, Status: domain.EntryConfirmed,
	})
	if err != nil {
		t.Fatalf("CreateEntry: %v", err)
	}
	return rec
}

// TestSaveAndListUnitAssignmentsSYS026 covers the storage round trip heat
// seeding needs: save several entries into one unit, list them back in
// seed-rank order.
func TestSaveAndListUnitAssignmentsSYS026(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	_, unit := seedingFixtureRound(t, s)
	evRec, err := GetEvent(ctx, s.DB(), roundEventID(t, s, unit.RoundID))
	if err != nil {
		t.Fatalf("GetEvent: %v", err)
	}

	e1 := seedingFixtureEntry(t, s, evRec, "Anna")
	e2 := seedingFixtureEntry(t, s, evRec, "Berta")

	if _, err := SaveUnitAssignment(ctx, s.DB(), domain.UnitAssignment{UnitID: unit.ID, EntryID: e2.ID, SeedRank: 2, Lane: 5}); err != nil {
		t.Fatalf("SaveUnitAssignment e2: %v", err)
	}
	if _, err := SaveUnitAssignment(ctx, s.DB(), domain.UnitAssignment{UnitID: unit.ID, EntryID: e1.ID, SeedRank: 1, Lane: 4}); err != nil {
		t.Fatalf("SaveUnitAssignment e1: %v", err)
	}

	rows, err := ListUnitAssignments(ctx, s.DB(), unit.ID)
	if err != nil {
		t.Fatalf("ListUnitAssignments: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 assignments, got %d", len(rows))
	}
	if rows[0].EntryID != e1.ID || rows[0].Lane != 4 {
		t.Errorf("expected e1 first (seed rank 1) with lane 4, got %+v", rows[0])
	}
	if rows[1].EntryID != e2.ID || rows[1].Lane != 5 {
		t.Errorf("expected e2 second (seed rank 2) with lane 5, got %+v", rows[1])
	}
}

// TestSaveUnitAssignmentUpsertBumpsVersionSYS028 covers regeneration:
// re-saving the same (unit, entry) pair updates in place and bumps version,
// never creating a duplicate row.
func TestSaveUnitAssignmentUpsertBumpsVersionSYS028(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	_, unit := seedingFixtureRound(t, s)
	evID := roundEventID(t, s, unit.RoundID)
	ev, err := GetEvent(ctx, s.DB(), evID)
	if err != nil {
		t.Fatalf("GetEvent: %v", err)
	}
	e1 := seedingFixtureEntry(t, s, ev, "Clara")

	first, err := SaveUnitAssignment(ctx, s.DB(), domain.UnitAssignment{UnitID: unit.ID, EntryID: e1.ID, SeedRank: 1, Lane: 4})
	if err != nil {
		t.Fatalf("save 1: %v", err)
	}
	if first.Version != 1 {
		t.Fatalf("expected version 1, got %d", first.Version)
	}
	second, err := SaveUnitAssignment(ctx, s.DB(), domain.UnitAssignment{UnitID: unit.ID, EntryID: e1.ID, SeedRank: 1, Lane: 6})
	if err != nil {
		t.Fatalf("save 2: %v", err)
	}
	if second.Version != 2 {
		t.Errorf("expected version 2 after upsert, got %d", second.Version)
	}
	if second.Lane != 6 {
		t.Errorf("expected lane updated to 6, got %d", second.Lane)
	}

	rows, err := ListUnitAssignments(ctx, s.DB(), unit.ID)
	if err != nil {
		t.Fatalf("ListUnitAssignments: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected exactly one row after upsert, got %d", len(rows))
	}
}

// TestUnitAssignmentDuplicateLaneRejectedSYS027 exercises the partial
// unique index (unit_id, lane) WHERE lane <> 0: two entries can never share
// a drawn lane within one unit.
func TestUnitAssignmentDuplicateLaneRejectedSYS027(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	_, unit := seedingFixtureRound(t, s)
	evID := roundEventID(t, s, unit.RoundID)
	ev, err := GetEvent(ctx, s.DB(), evID)
	if err != nil {
		t.Fatalf("GetEvent: %v", err)
	}
	e1 := seedingFixtureEntry(t, s, ev, "Dora")
	e2 := seedingFixtureEntry(t, s, ev, "Elin")

	if _, err := SaveUnitAssignment(ctx, s.DB(), domain.UnitAssignment{UnitID: unit.ID, EntryID: e1.ID, Lane: 4}); err != nil {
		t.Fatalf("save e1: %v", err)
	}
	if _, err := SaveUnitAssignment(ctx, s.DB(), domain.UnitAssignment{UnitID: unit.ID, EntryID: e2.ID, Lane: 4}); err == nil {
		t.Fatal("expected a unique-constraint error assigning the same lane twice in one unit")
	}
	// Lane 0 ("no lane") may repeat freely (non-laned events, by-lot events
	// pending a draw).
	e3 := seedingFixtureEntry(t, s, ev, "Frida")
	if _, err := SaveUnitAssignment(ctx, s.DB(), domain.UnitAssignment{UnitID: unit.ID, EntryID: e3.ID, Lane: 0}); err != nil {
		t.Fatalf("expected lane 0 to be reusable, got %v", err)
	}
}

// TestSetUnitAssignmentQualificationSYS029 covers writing a round's
// progression outcome under optimistic concurrency.
func TestSetUnitAssignmentQualificationSYS029(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	_, unit := seedingFixtureRound(t, s)
	evID := roundEventID(t, s, unit.RoundID)
	ev, err := GetEvent(ctx, s.DB(), evID)
	if err != nil {
		t.Fatalf("GetEvent: %v", err)
	}
	e1 := seedingFixtureEntry(t, s, ev, "Greta")
	rec, err := SaveUnitAssignment(ctx, s.DB(), domain.UnitAssignment{UnitID: unit.ID, EntryID: e1.ID})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	newVersion, err := SetUnitAssignmentQualification(ctx, s.DB(), rec.ID, rec.Version, domain.StatusQ)
	if err != nil {
		t.Fatalf("SetUnitAssignmentQualification: %v", err)
	}
	if newVersion != rec.Version+1 {
		t.Errorf("expected version %d, got %d", rec.Version+1, newVersion)
	}
	got, err := GetUnitAssignment(ctx, s.DB(), unit.ID, e1.ID)
	if err != nil {
		t.Fatalf("GetUnitAssignment: %v", err)
	}
	if got.Qualification != domain.StatusQ {
		t.Errorf("qualification = %q, want Q", got.Qualification)
	}
	if _, err := SetUnitAssignmentQualification(ctx, s.DB(), rec.ID, rec.Version /* stale */, domain.StatusQt); !errors.Is(err, ErrVersionConflict) {
		t.Errorf("expected ErrVersionConflict on a stale version, got %v", err)
	}
}

// TestUpdateUnitAssignmentManualAndReleaseSYS028 covers the manual-override
// lifecycle: a manual heat/lane edit sets manual_override, and release
// clears it so regeneration may recompute the row again.
func TestUpdateUnitAssignmentManualAndReleaseSYS028(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	_, unit := seedingFixtureRound(t, s)
	evID := roundEventID(t, s, unit.RoundID)
	ev, err := GetEvent(ctx, s.DB(), evID)
	if err != nil {
		t.Fatalf("GetEvent: %v", err)
	}
	e1 := seedingFixtureEntry(t, s, ev, "Hanna")
	rec, err := SaveUnitAssignment(ctx, s.DB(), domain.UnitAssignment{UnitID: unit.ID, EntryID: e1.ID, Lane: 3})
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	v, err := UpdateUnitAssignmentManual(ctx, s.DB(), rec.ID, rec.Version, unit.ID, 7)
	if err != nil {
		t.Fatalf("UpdateUnitAssignmentManual: %v", err)
	}
	got, err := GetUnitAssignment(ctx, s.DB(), unit.ID, e1.ID)
	if err != nil {
		t.Fatalf("GetUnitAssignment: %v", err)
	}
	if !got.ManualOverride || got.Lane != 7 {
		t.Fatalf("expected manual override with lane 7, got %+v", got)
	}

	v2, err := ReleaseManualOverride(ctx, s.DB(), rec.ID, v)
	if err != nil {
		t.Fatalf("ReleaseManualOverride: %v", err)
	}
	_ = v2
	got2, err := GetUnitAssignment(ctx, s.DB(), unit.ID, e1.ID)
	if err != nil {
		t.Fatalf("GetUnitAssignment: %v", err)
	}
	if got2.ManualOverride {
		t.Error("expected manual override cleared after release")
	}
}

// TestEnsureRoundUnitCountGrowsAndKeepsExtras covers heat splitting
// (SYS-026: "each round gets one schedulable unit; heat splitting is
// seeding's job"): growing from the default 1 unit to N, and never shrinking
// below whatever already exists.
func TestEnsureRoundUnitCountGrowsAndKeepsExtras(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	round, _ := seedingFixtureRound(t, s)

	units, err := EnsureRoundUnitCount(ctx, s.DB(), round.ID, 3)
	if err != nil {
		t.Fatalf("EnsureRoundUnitCount(3): %v", err)
	}
	if len(units) != 3 {
		t.Fatalf("expected 3 units, got %d", len(units))
	}

	// A request for fewer units than already exist must not delete any.
	units2, err := EnsureRoundUnitCount(ctx, s.DB(), round.ID, 1)
	if err != nil {
		t.Fatalf("EnsureRoundUnitCount(1): %v", err)
	}
	if len(units2) != 3 {
		t.Fatalf("expected shrink request to keep all 3 existing units, got %d", len(units2))
	}
}

// TestEnsureRoundUnitCountRejectsNonPositive is a denial/edge-path test.
func TestEnsureRoundUnitCountRejectsNonPositive(t *testing.T) {
	s := openTest(t)
	round, _ := seedingFixtureRound(t, s)
	if _, err := EnsureRoundUnitCount(context.Background(), s.DB(), round.ID, 0); err == nil {
		t.Error("expected an error for a non-positive unit count")
	}
}

// TestGetUnitAssignmentNotFound is a denial/edge-path test.
func TestGetUnitAssignmentNotFound(t *testing.T) {
	s := openTest(t)
	if _, err := GetUnitAssignment(context.Background(), s.DB(), "no-such-unit", "no-such-entry"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// roundEventID looks up the event id a round belongs to (test helper).
func roundEventID(t *testing.T, s *Store, roundID string) string {
	t.Helper()
	row := s.DB().QueryRowContext(context.Background(), `SELECT event_id FROM rounds WHERE id = ?`, roundID)
	var eventID string
	if err := row.Scan(&eventID); err != nil {
		t.Fatalf("lookup event for round %s: %v", roundID, err)
	}
	return eventID
}
