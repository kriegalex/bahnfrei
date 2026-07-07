// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kriegalex/bahnfrei/internal/domain"
)

func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// testMeet inserts the UC-001 #2 fixture meet: two competition days, tier
// C-Meeting, status draft.
func testMeet(t *testing.T, s *Store) MeetRecord {
	t.Helper()
	m, err := CreateMeet(context.Background(), s.DB(), domain.Meet{
		Name:            "Abendmeeting Uster",
		Venue:           "Stadion Buchholz",
		HomologationRef: "CH-ZH-042",
		StartDate:       day(2027, 6, 12),
		EndDate:         day(2027, 6, 13),
		Organizer:       "TV Uster",
		Tier:            "C-Meeting",
	})
	if err != nil {
		t.Fatalf("CreateMeet: %v", err)
	}
	return m
}

// TestMeetCreateAndGet covers UC-001 #2: the meet appears in status draft
// with exactly the given attributes retrievable (SYS-001).
func TestMeetCreateAndGet(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	created := testMeet(t, s)

	if created.Status != domain.MeetDraft {
		t.Errorf("new meet status = %q, want draft", created.Status)
	}

	got, err := GetMeet(ctx, s.DB(), created.ID)
	if err != nil {
		t.Fatalf("GetMeet: %v", err)
	}
	if got.Name != "Abendmeeting Uster" || got.Venue != "Stadion Buchholz" ||
		got.HomologationRef != "CH-ZH-042" || got.Organizer != "TV Uster" ||
		got.Tier != "C-Meeting" || got.Status != domain.MeetDraft {
		t.Errorf("round-tripped meet = %+v", got)
	}
	if !got.StartDate.Equal(day(2027, 6, 12)) || !got.EndDate.Equal(day(2027, 6, 13)) {
		t.Errorf("dates = %v..%v, want 2027-06-12..13", got.StartDate, got.EndDate)
	}

	if _, err := GetMeet(ctx, s.DB(), "no-such-id"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetMeet(unknown) = %v, want ErrNotFound", err)
	}
}

func TestMeetCreateRejectsInvalid(t *testing.T) {
	s := openTest(t)
	_, err := CreateMeet(context.Background(), s.DB(), domain.Meet{
		Name: "", StartDate: day(2027, 6, 12), EndDate: day(2027, 6, 12),
	})
	if err == nil {
		t.Fatal("expected validation error for nameless meet")
	}
}

func TestMeetListNewestFirst(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	first := testMeet(t, s)
	second := testMeet(t, s)

	list, err := ListMeets(ctx, s.DB())
	if err != nil {
		t.Fatalf("ListMeets: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("ListMeets returned %d meets, want 2", len(list))
	}
	if list[0].ID != second.ID || list[1].ID != first.ID {
		t.Errorf("order = %s,%s want newest first", list[0].ID, list[1].ID)
	}
}

// TestMeetUpdateAndStatus covers SYS-001 edit/archive under optimistic
// concurrency (SYS-083 semantics from TASK-003).
func TestMeetUpdateAndStatus(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	m := testMeet(t, s)

	edited := m.Meet
	edited.Venue = "Stadion Letzigrund"
	v2, err := UpdateMeet(ctx, s.DB(), m.ID, m.Version, edited)
	if err != nil {
		t.Fatalf("UpdateMeet: %v", err)
	}

	// A stale second write must surface the conflict, never overwrite.
	if _, err := UpdateMeet(ctx, s.DB(), m.ID, m.Version, edited); !errors.Is(err, ErrVersionConflict) {
		t.Errorf("stale UpdateMeet = %v, want ErrVersionConflict", err)
	}

	// Invalid payloads are rejected before touching the row.
	bad := edited
	bad.EndDate = bad.StartDate.AddDate(0, 0, -1)
	if _, err := UpdateMeet(ctx, s.DB(), m.ID, v2, bad); err == nil {
		t.Error("expected validation error for inverted dates")
	}

	if _, err := SetMeetStatus(ctx, s.DB(), m.ID, v2, domain.MeetArchived); err != nil {
		t.Fatalf("SetMeetStatus: %v", err)
	}
	got, err := GetMeet(ctx, s.DB(), m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Venue != "Stadion Letzigrund" || got.Status != domain.MeetArchived {
		t.Errorf("after edit+archive: venue=%q status=%q", got.Venue, got.Status)
	}
}

// TestSessionsPerDay covers SYS-001 "sessions per day" / UC-001 #2 (two
// days, two sessions per day).
func TestSessionsPerDay(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	m := testMeet(t, s)

	for _, sess := range []domain.Session{
		{MeetID: m.ID, Day: day(2027, 6, 13), Label: "Nachmittag"},
		{MeetID: m.ID, Day: day(2027, 6, 12), Label: "Vormittag"},
		{MeetID: m.ID, Day: day(2027, 6, 12), Label: "Nachmittag"},
		{MeetID: m.ID, Day: day(2027, 6, 13), Label: "Vormittag"},
	} {
		if _, err := CreateSession(ctx, s.DB(), sess); err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
	}
	if _, err := CreateSession(ctx, s.DB(), domain.Session{MeetID: m.ID}); err == nil {
		t.Error("expected validation error for session without a day")
	}

	got, err := ListSessions(ctx, s.DB(), m.ID)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("got %d sessions, want 4", len(got))
	}
	// Day-ordered: both day-1 sessions before both day-2 sessions.
	if !got[0].Day.Equal(day(2027, 6, 12)) || !got[1].Day.Equal(day(2027, 6, 12)) ||
		!got[2].Day.Equal(day(2027, 6, 13)) || !got[3].Day.Equal(day(2027, 6, 13)) {
		t.Errorf("sessions not in day order: %+v", got)
	}

	if err := DeleteSessions(ctx, s.DB(), m.ID); err != nil {
		t.Fatalf("DeleteSessions: %v", err)
	}
	got, err = ListSessions(ctx, s.DB(), m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("after delete: %d sessions, want 0", len(got))
	}
}

// TestEventProgramme covers SYS-002/UC-001 #3: events as discipline ×
// category with round structure and entry deadline.
func TestEventProgramme(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	m := testMeet(t, s)

	deadline := time.Date(2027, 6, 1, 23, 59, 0, 0, time.UTC)
	ev, err := CreateEvent(ctx, s.DB(), domain.Event{
		MeetID:         m.ID,
		DisciplineCode: "100m",
		CategoryCodes:  []string{"U16W"},
		EntryDeadline:  &deadline,
	})
	if err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}
	if _, err := CreateEvent(ctx, s.DB(), domain.Event{MeetID: m.ID, DisciplineCode: "100m"}); err == nil {
		t.Error("expected validation error for event without categories")
	}

	events, err := ListEvents(ctx, s.DB(), m.ID)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	got := events[0]
	if got.DisciplineCode != "100m" || len(got.CategoryCodes) != 1 || got.CategoryCodes[0] != "U16W" {
		t.Errorf("event round-trip = %+v", got)
	}
	if got.EntryDeadline == nil || !got.EntryDeadline.Equal(deadline) {
		t.Errorf("entry deadline = %v, want %v", got.EntryDeadline, deadline)
	}
	if got.Status != domain.EventDraft {
		t.Errorf("status = %q, want draft", got.Status)
	}

	// Round structure in progression order (D2.1).
	for i, kind := range []domain.RoundKind{domain.RoundQualification, domain.RoundFinal} {
		if _, err := CreateRound(ctx, s.DB(), domain.Round{EventID: ev.ID, Kind: kind}, i); err != nil {
			t.Fatalf("CreateRound: %v", err)
		}
	}
	if _, err := CreateRound(ctx, s.DB(), domain.Round{}, 0); err == nil {
		t.Error("expected validation error for round without an event")
	}
	rounds, err := ListRounds(ctx, s.DB(), ev.ID)
	if err != nil {
		t.Fatalf("ListRounds: %v", err)
	}
	if len(rounds) != 2 || rounds[0].Kind != domain.RoundQualification || rounds[1].Kind != domain.RoundFinal {
		t.Errorf("rounds = %+v, want qualification then final", rounds)
	}
}

// TestTimetablePublishRetainsVersions covers SYS-004/UC-001 #4: publish,
// amend one unit's time, republish — both versions retained with
// timestamps, the latest shows the amended time.
func TestTimetablePublishRetainsVersions(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	m := testMeet(t, s)

	ev, err := CreateEvent(ctx, s.DB(), domain.Event{
		MeetID: m.ID, DisciplineCode: "100m", CategoryCodes: []string{"U16W"},
	})
	if err != nil {
		t.Fatal(err)
	}
	round, err := CreateRound(ctx, s.DB(), domain.Round{EventID: ev.ID, Kind: domain.RoundFinal}, 0)
	if err != nil {
		t.Fatal(err)
	}
	unit, err := CreateUnit(ctx, s.DB(), domain.Unit{RoundID: round.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CreateUnit(ctx, s.DB(), domain.Unit{}); err == nil {
		t.Error("expected validation error for unit without a round")
	}

	// Before anything is published there is no public timetable.
	if _, err := LatestTimetable(ctx, s.DB(), m.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("LatestTimetable before publish = %v, want ErrNotFound", err)
	}

	// Schedule and publish v1.
	t1 := time.Date(2027, 6, 12, 14, 30, 0, 0, time.UTC)
	v, err := UpdateUnitSchedule(ctx, s.DB(), unit.ID, unit.Version, t1, "Bahn 1")
	if err != nil {
		t.Fatalf("UpdateUnitSchedule: %v", err)
	}
	if _, err := UpdateUnitSchedule(ctx, s.DB(), unit.ID, v, time.Time{}, ""); err == nil {
		t.Error("expected validation error for zero scheduled time")
	}

	live, err := ListMeetUnits(ctx, s.DB(), m.ID)
	if err != nil {
		t.Fatalf("ListMeetUnits: %v", err)
	}
	if len(live) != 1 || live[0].ScheduledAt == nil || !live[0].ScheduledAt.Equal(t1) {
		t.Fatalf("live units = %+v, want one unit at %v", live, t1)
	}
	if live[0].DisciplineCode != "100m" || live[0].RoundKind != "final" {
		t.Errorf("live unit join = %+v", live[0])
	}

	v1, err := AppendTimetableVersion(ctx, s.DB(), m.ID, live)
	if err != nil {
		t.Fatalf("AppendTimetableVersion: %v", err)
	}
	if v1.Version != 1 || v1.PublishedAt.IsZero() {
		t.Errorf("v1 = version %d publishedAt %v", v1.Version, v1.PublishedAt)
	}

	// Amend the unit's time and republish.
	t2 := t1.Add(45 * time.Minute)
	if _, err := UpdateUnitSchedule(ctx, s.DB(), unit.ID, v, t2, "Bahn 1"); err != nil {
		t.Fatalf("amend schedule: %v", err)
	}
	live, err = ListMeetUnits(ctx, s.DB(), m.ID)
	if err != nil {
		t.Fatal(err)
	}
	v2, err := AppendTimetableVersion(ctx, s.DB(), m.ID, live)
	if err != nil {
		t.Fatalf("republish: %v", err)
	}
	if v2.Version != 2 {
		t.Errorf("republish version = %d, want 2", v2.Version)
	}

	// Current public timetable shows the amended time…
	latest, err := LatestTimetable(ctx, s.DB(), m.ID)
	if err != nil {
		t.Fatalf("LatestTimetable: %v", err)
	}
	if latest.Version != 2 || !latest.Entries[0].ScheduledAt.Equal(t2) {
		t.Errorf("latest = v%d at %v, want v2 at %v", latest.Version, latest.Entries[0].ScheduledAt, t2)
	}

	// …and both versions remain retrievable with timestamps (SYS-004).
	all, err := ListTimetableVersions(ctx, s.DB(), m.ID)
	if err != nil {
		t.Fatalf("ListTimetableVersions: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("got %d versions, want 2", len(all))
	}
	if all[0].Version != 2 || all[1].Version != 1 {
		t.Errorf("version order = %d,%d want 2,1", all[0].Version, all[1].Version)
	}
	if !all[1].Entries[0].ScheduledAt.Equal(t1) {
		t.Errorf("retained v1 time = %v, want original %v", all[1].Entries[0].ScheduledAt, t1)
	}
	if all[0].PublishedAt.Before(all[1].PublishedAt) {
		t.Errorf("v2 published %v before v1 %v", all[0].PublishedAt, all[1].PublishedAt)
	}
}

// TestMeetScanRejectsCorruptRows proves reads fail loudly (never return
// silently wrong data) when stored encodings are damaged — the same
// fail-closed posture as the startup consistency check (ADR-004 §1).
func TestMeetScanRejectsCorruptRows(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	m := testMeet(t, s)

	if _, err := s.DB().Exec(`UPDATE meets SET start_date = 'garbage' WHERE id = ?`, m.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := GetMeet(ctx, s.DB(), m.ID); err == nil {
		t.Error("GetMeet accepted a corrupt start date")
	}
	if _, err := ListMeets(ctx, s.DB()); err == nil {
		t.Error("ListMeets accepted a corrupt start date")
	}
	if _, err := s.DB().Exec(`UPDATE meets SET start_date = '2027-06-12', end_date = 'garbage' WHERE id = ?`, m.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := GetMeet(ctx, s.DB(), m.ID); err == nil {
		t.Error("GetMeet accepted a corrupt end date")
	}

	m2 := testMeet(t, s)
	if _, err := CreateSession(ctx, s.DB(), domain.Session{MeetID: m2.ID, Day: day(2027, 6, 12), Label: "S"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().Exec(`UPDATE meet_sessions SET day = 'garbage'`); err != nil {
		t.Fatal(err)
	}
	if _, err := ListSessions(ctx, s.DB(), m2.ID); err == nil {
		t.Error("ListSessions accepted a corrupt day")
	}

	ev, err := CreateEvent(ctx, s.DB(), domain.Event{MeetID: m2.ID, DisciplineCode: "100m", CategoryCodes: []string{"U16 W"}})
	if err != nil {
		t.Fatal(err)
	}
	round, err := CreateRound(ctx, s.DB(), domain.Round{EventID: ev.ID, Kind: domain.RoundFinal}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CreateUnit(ctx, s.DB(), domain.Unit{RoundID: round.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().Exec(`UPDATE events SET category_codes = 'not-json'`); err != nil {
		t.Fatal(err)
	}
	if _, err := ListEvents(ctx, s.DB(), m2.ID); err == nil {
		t.Error("ListEvents accepted corrupt category codes")
	}
	if _, err := ListMeetUnits(ctx, s.DB(), m2.ID); err == nil {
		t.Error("ListMeetUnits accepted corrupt category codes")
	}
	if _, err := s.DB().Exec(`UPDATE events SET category_codes = '["U16 W"]', entry_deadline = 'garbage'`); err != nil {
		t.Fatal(err)
	}
	if _, err := ListEvents(ctx, s.DB(), m2.ID); err == nil {
		t.Error("ListEvents accepted a corrupt entry deadline")
	}
	if _, err := s.DB().Exec(`UPDATE units SET scheduled_at = 'garbage'`); err != nil {
		t.Fatal(err)
	}
	if _, err := ListMeetUnits(ctx, s.DB(), m2.ID); err == nil {
		t.Error("ListMeetUnits accepted a corrupt scheduled time")
	}

	if _, err := AppendTimetableVersion(ctx, s.DB(), m2.ID, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().Exec(`UPDATE timetable_versions SET entries_json = 'not-json'`); err != nil {
		t.Fatal(err)
	}
	if _, err := LatestTimetable(ctx, s.DB(), m2.ID); err == nil {
		t.Error("LatestTimetable accepted corrupt entries")
	}
	if _, err := s.DB().Exec(`UPDATE timetable_versions SET entries_json = '[]', published_at = 'garbage'`); err != nil {
		t.Fatal(err)
	}
	if _, err := ListTimetableVersions(ctx, s.DB(), m2.ID); err == nil {
		t.Error("ListTimetableVersions accepted a corrupt timestamp")
	}
}
