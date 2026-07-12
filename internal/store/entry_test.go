// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import (
	"context"
	"errors"
	"testing"

	"github.com/kriegalex/bahnfrei/internal/domain"
)

// entryFixtureEvent creates a meet with one open event for entry-storage
// tests.
func entryFixtureEvent(t *testing.T, s *Store) EventRecord {
	t.Helper()
	ctx := context.Background()
	meet := testMeet(t, s)
	ev, err := CreateEvent(ctx, s.DB(), domain.Event{
		MeetID: meet.ID, DisciplineCode: "100m", CategoryCodes: []string{"U18 W"},
	})
	if err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}
	return ev
}

func entryFixtureAthlete(t *testing.T, s *Store, name string) AthleteRecord {
	t.Helper()
	a, err := CreateAthlete(context.Background(), s.DB(), domain.Athlete{
		FirstName: name, LastName: "Muster", BirthYear: 2010, Sex: domain.SexFemale,
	})
	if err != nil {
		t.Fatalf("CreateAthlete: %v", err)
	}
	return a
}

// TestEntryStoreCreateAndGetSYS011 covers the storage round trip an online
// entry needs: status defaults to entered, source to online.
func TestEntryStoreCreateAndGetSYS011(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	ev := entryFixtureEvent(t, s)
	athlete := entryFixtureAthlete(t, s, "Anna")

	rec, err := CreateEntry(ctx, s.DB(), domain.Entry{
		EventID: ev.ID, AthleteID: athlete.ID, SeedPerformance: "12.85", SubmittedBy: "01ACC",
	})
	if err != nil {
		t.Fatalf("CreateEntry: %v", err)
	}
	if rec.Status != domain.EntryEntered {
		t.Errorf("status = %q, want entered", rec.Status)
	}
	if rec.Source != domain.EntrySourceOnline {
		t.Errorf("source = %q, want online", rec.Source)
	}

	got, err := GetEntry(ctx, s.DB(), rec.ID)
	if err != nil {
		t.Fatalf("GetEntry: %v", err)
	}
	if got.EventID != ev.ID || got.AthleteID != athlete.ID || got.SeedPerformance != "12.85" || got.SubmittedBy != "01ACC" {
		t.Errorf("round-tripped entry = %+v", got)
	}

	if _, err := GetEntry(ctx, s.DB(), "no-such-id"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetEntry(unknown) = %v, want ErrNotFound", err)
	}
}

// TestEntryStoreDuplicateRejectedSYS011UC003_1: an athlete entering the same
// event twice is rejected — the event-scoped uniqueness the UC-003 #1
// "confirm each entry" flow relies on to prevent silent double entries.
func TestEntryStoreDuplicateRejectedSYS011UC003_1(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	ev := entryFixtureEvent(t, s)
	athlete := entryFixtureAthlete(t, s, "Anna")

	if _, err := CreateEntry(ctx, s.DB(), domain.Entry{EventID: ev.ID, AthleteID: athlete.ID}); err != nil {
		t.Fatalf("first CreateEntry: %v", err)
	}
	if _, err := CreateEntry(ctx, s.DB(), domain.Entry{EventID: ev.ID, AthleteID: athlete.ID}); !errors.Is(err, ErrDuplicateEntry) {
		t.Errorf("duplicate CreateEntry = %v, want ErrDuplicateEntry", err)
	}
}

// TestEntryStoreCountActiveExcludesScratchedSYS015 backs the SYS-015
// entry-limit check: a scratched entry frees its slot.
func TestEntryStoreCountActiveExcludesScratchedSYS015(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	ev := entryFixtureEvent(t, s)
	a1 := entryFixtureAthlete(t, s, "Anna")
	a2 := entryFixtureAthlete(t, s, "Beat")

	e1, err := CreateEntry(ctx, s.DB(), domain.Entry{EventID: ev.ID, AthleteID: a1.ID})
	if err != nil {
		t.Fatalf("CreateEntry: %v", err)
	}
	if _, err := CreateEntry(ctx, s.DB(), domain.Entry{EventID: ev.ID, AthleteID: a2.ID}); err != nil {
		t.Fatalf("CreateEntry: %v", err)
	}
	n, err := CountActiveEntriesForEvent(ctx, s.DB(), ev.ID)
	if err != nil {
		t.Fatalf("CountActiveEntriesForEvent: %v", err)
	}
	if n != 2 {
		t.Fatalf("count = %d, want 2", n)
	}

	if _, err := UpdateEntryStatus(ctx, s.DB(), e1.ID, e1.Version, domain.EntryScratched); err != nil {
		t.Fatalf("UpdateEntryStatus: %v", err)
	}
	n, err = CountActiveEntriesForEvent(ctx, s.DB(), ev.ID)
	if err != nil {
		t.Fatalf("CountActiveEntriesForEvent: %v", err)
	}
	if n != 1 {
		t.Fatalf("count after scratch = %d, want 1", n)
	}
}

// TestEntryStoreListByMeetAndSubmitter covers the two read paths the
// exception report (UC-003 #5) and "my entries" (UC-003 #1/#2) views need.
func TestEntryStoreListByMeetAndSubmitter(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	ev := entryFixtureEvent(t, s)
	a1 := entryFixtureAthlete(t, s, "Anna")
	a2 := entryFixtureAthlete(t, s, "Beat")

	if _, err := CreateEntry(ctx, s.DB(), domain.Entry{EventID: ev.ID, AthleteID: a1.ID, SubmittedBy: "01SUB"}); err != nil {
		t.Fatalf("CreateEntry: %v", err)
	}
	if _, err := CreateEntry(ctx, s.DB(), domain.Entry{EventID: ev.ID, AthleteID: a2.ID, SubmittedBy: "02SUB"}); err != nil {
		t.Fatalf("CreateEntry: %v", err)
	}

	byMeet, err := ListEntriesByMeet(ctx, s.DB(), ev.MeetID)
	if err != nil {
		t.Fatalf("ListEntriesByMeet: %v", err)
	}
	if len(byMeet) != 2 {
		t.Fatalf("ListEntriesByMeet = %d entries, want 2", len(byMeet))
	}

	byEvent, err := ListEntriesByEvent(ctx, s.DB(), ev.ID)
	if err != nil {
		t.Fatalf("ListEntriesByEvent: %v", err)
	}
	if len(byEvent) != 2 {
		t.Fatalf("ListEntriesByEvent = %d entries, want 2", len(byEvent))
	}

	bySubmitter, err := ListEntriesBySubmitter(ctx, s.DB(), ev.MeetID, "01SUB")
	if err != nil {
		t.Fatalf("ListEntriesBySubmitter: %v", err)
	}
	if len(bySubmitter) != 1 || bySubmitter[0].AthleteID != a1.ID {
		t.Fatalf("ListEntriesBySubmitter(01SUB) = %+v, want exactly a1's entry", bySubmitter)
	}
}
