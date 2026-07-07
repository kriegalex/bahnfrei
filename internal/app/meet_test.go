// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/kriegalex/bahnfrei/internal/domain"
	"github.com/kriegalex/bahnfrei/internal/store"
)

func newTestMeets(t *testing.T) (*MeetService, *store.Store) {
	t.Helper()
	s, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	catalog, err := domain.BuiltinDisciplineCatalog()
	if err != nil {
		t.Fatalf("BuiltinDisciplineCatalog: %v", err)
	}
	schemes, err := domain.BuiltinCategorySchemes()
	if err != nil {
		t.Fatalf("BuiltinCategorySchemes: %v", err)
	}
	return NewMeetService(s.DB(), catalog, schemes), s
}

var organizer = Session{AccountID: "01ORG", Username: "orga", Role: RoleMeetOrganizer}

func meetDay(d int) time.Time { return time.Date(2027, 6, 10+d, 0, 0, 0, 0, time.UTC) }

// ucMeetRequest is the UC-001 #2 fixture: name, venue, two competition
// days, two sessions/day, tier C-Meeting.
func ucMeetRequest() MeetRequest {
	return MeetRequest{
		Name:             "Abendmeeting Uster",
		Venue:            "Stadion Buchholz",
		HomologationRef:  "CH-ZH-042",
		StartDate:        meetDay(0),
		EndDate:          meetDay(1),
		Tier:             "C-Meeting",
		CategorySchemeID: domain.SchemeSwissAthletics,
		Sessions: []SessionPlan{
			{Day: meetDay(0), Label: "Session 1"},
			{Day: meetDay(0), Label: "Session 2"},
			{Day: meetDay(1), Label: "Session 1"},
			{Day: meetDay(1), Label: "Session 2"},
		},
	}
}

// TestCreateMeetUC001_2 covers UC-001 #2: creating a meet with two days,
// two sessions/day and tier C-Meeting yields a draft meet with exactly
// those attributes retrievable — and the creation is audited.
func TestCreateMeetUC001_2(t *testing.T) {
	svc, st := newTestMeets(t)
	ctx := context.Background()

	rec, err := svc.CreateMeet(ctx, organizer, ucMeetRequest())
	if err != nil {
		t.Fatalf("CreateMeet: %v", err)
	}
	if rec.Status != domain.MeetDraft {
		t.Errorf("status = %q, want draft", rec.Status)
	}
	if rec.Organizer != "orga" {
		t.Errorf("organizer = %q, want the acting account (SYS-001)", rec.Organizer)
	}

	d, err := svc.Meet(ctx, rec.ID)
	if err != nil {
		t.Fatalf("Meet: %v", err)
	}
	if d.Name != "Abendmeeting Uster" || d.Venue != "Stadion Buchholz" || d.Tier != "C-Meeting" {
		t.Errorf("attributes = %+v", d.MeetRecord)
	}
	if len(d.Sessions) != 4 {
		t.Fatalf("got %d sessions, want 4 (two days × two sessions)", len(d.Sessions))
	}

	trail, err := store.AuditTrail(ctx, st.DB(), "meet", rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(trail) != 1 || trail[0].Action != "meet.create" || trail[0].Actor != "01ORG" {
		t.Errorf("audit trail = %+v, want one meet.create by 01ORG", trail)
	}
}

func TestCreateMeetAuthorization(t *testing.T) {
	svc, _ := newTestMeets(t)
	official := Session{AccountID: "01FLD", Username: "field", Role: RoleFieldOfficial}
	var forbidden ErrForbidden
	if _, err := svc.CreateMeet(context.Background(), official, ucMeetRequest()); !errors.As(err, &forbidden) {
		t.Errorf("CreateMeet as field official = %v, want ErrForbidden (SYS-090)", err)
	}
}

func TestCreateMeetValidation(t *testing.T) {
	svc, _ := newTestMeets(t)
	ctx := context.Background()

	bad := ucMeetRequest()
	bad.CategorySchemeID = "no-such-scheme"
	if _, err := svc.CreateMeet(ctx, organizer, bad); err == nil {
		t.Error("expected error for unknown category scheme")
	}

	bad = ucMeetRequest()
	bad.Sessions = append(bad.Sessions, SessionPlan{Day: meetDay(5), Label: "stray"})
	if _, err := svc.CreateMeet(ctx, organizer, bad); err == nil {
		t.Error("expected error for session day outside the meet dates")
	}
}

func TestUpdateAndArchiveMeet(t *testing.T) {
	svc, _ := newTestMeets(t)
	ctx := context.Background()
	rec, err := svc.CreateMeet(ctx, organizer, ucMeetRequest())
	if err != nil {
		t.Fatal(err)
	}

	edit := ucMeetRequest()
	edit.Venue = "Stadion Letzigrund"
	edit.Sessions = edit.Sessions[:2] // shrink to one day's plan
	if err := svc.UpdateMeet(ctx, organizer, rec.ID, rec.Version, edit); err != nil {
		t.Fatalf("UpdateMeet: %v", err)
	}
	// The stale first version must lose with a conflict, not overwrite.
	if err := svc.UpdateMeet(ctx, organizer, rec.ID, rec.Version, edit); !errors.Is(err, ErrConflict) {
		t.Errorf("stale UpdateMeet = %v, want ErrConflict (SYS-083)", err)
	}

	d, err := svc.Meet(ctx, rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if d.Venue != "Stadion Letzigrund" || len(d.Sessions) != 2 {
		t.Errorf("after edit: venue=%q sessions=%d", d.Venue, len(d.Sessions))
	}

	if err := svc.ArchiveMeet(ctx, organizer, rec.ID, d.Version); err != nil {
		t.Fatalf("ArchiveMeet: %v", err)
	}
	d, err = svc.Meet(ctx, rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != domain.MeetArchived {
		t.Errorf("status = %q, want archived (SYS-001)", d.Status)
	}
}

// TestAddEventsUC001_3 covers UC-001 #3: adding 100m / U16 W (track),
// Shot Put / U16 W (horizontal field) and 4×100m / U16 W (relay), each
// with round structure and entry deadline, yields a programme listing all
// three with discipline-correct capture types and deadlines.
func TestAddEventsUC001_3(t *testing.T) {
	svc, _ := newTestMeets(t)
	ctx := context.Background()
	rec, err := svc.CreateMeet(ctx, organizer, ucMeetRequest())
	if err != nil {
		t.Fatal(err)
	}

	deadline := time.Date(2027, 6, 1, 23, 59, 0, 0, time.UTC)
	for _, tc := range []struct {
		code   string
		rounds []domain.RoundKind
	}{
		{"100m", []domain.RoundKind{domain.RoundQualification, domain.RoundFinal}},
		{"SP", nil}, // defaults to a single final
		{"4x100m", []domain.RoundKind{domain.RoundFinal}},
	} {
		if _, err := svc.AddEvent(ctx, organizer, rec.ID, AddEventRequest{
			DisciplineCode: tc.code,
			CategoryCodes:  []string{"U16 W"},
			Rounds:         tc.rounds,
			EntryDeadline:  &deadline,
		}); err != nil {
			t.Fatalf("AddEvent %s: %v", tc.code, err)
		}
	}

	d, err := svc.Meet(ctx, rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Programme) != 3 {
		t.Fatalf("programme has %d events, want 3", len(d.Programme))
	}
	wantFamily := map[string]domain.DisciplineFamily{
		"100m":   domain.FamilyTrack,
		"SP":     domain.FamilyFieldHorizontal,
		"4x100m": domain.FamilyRelay,
	}
	for _, pe := range d.Programme {
		if pe.Family != wantFamily[pe.DisciplineCode] {
			t.Errorf("%s capture type = %q, want %q (UC-001 #3)", pe.DisciplineCode, pe.Family, wantFamily[pe.DisciplineCode])
		}
		if pe.EntryDeadline == nil || !pe.EntryDeadline.Equal(deadline) {
			t.Errorf("%s deadline = %v, want %v", pe.DisciplineCode, pe.EntryDeadline, deadline)
		}
		if len(pe.Rounds) == 0 {
			t.Errorf("%s has no rounds", pe.DisciplineCode)
		}
	}
	// One schedulable unit per round exists (4 rounds total).
	if len(d.Units) != 4 {
		t.Errorf("got %d units, want 4 (one per round)", len(d.Units))
	}
}

func TestAddEventValidation(t *testing.T) {
	svc, _ := newTestMeets(t)
	ctx := context.Background()
	rec, err := svc.CreateMeet(ctx, organizer, ucMeetRequest())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.AddEvent(ctx, organizer, rec.ID, AddEventRequest{
		DisciplineCode: "CaberToss", CategoryCodes: []string{"U16 W"},
	}); err == nil {
		t.Error("expected error for a discipline not in the catalog")
	}
	if _, err := svc.AddEvent(ctx, organizer, rec.ID, AddEventRequest{
		DisciplineCode: "100m", CategoryCodes: []string{"U99 X"},
	}); err == nil {
		t.Error("expected error for a category not in the meet's scheme")
	}
	if _, err := svc.AddEvent(ctx, organizer, rec.ID, AddEventRequest{
		DisciplineCode: "100m",
	}); err == nil {
		t.Error("expected error for an event without categories (SYS-002)")
	}
	if _, err := svc.AddEvent(ctx, organizer, "no-such-meet", AddEventRequest{
		DisciplineCode: "100m", CategoryCodes: []string{"U16 W"},
	}); !errors.Is(err, ErrMeetNotFound) {
		t.Errorf("AddEvent on unknown meet = %v, want ErrMeetNotFound", err)
	}
}

// TestTimetablePublishAndAmendUC001_4 covers UC-001 #4 at the service
// level: publish, amend one unit's time, republish — both versions
// retained with timestamps, the public timetable shows the amended time.
func TestTimetablePublishAndAmendUC001_4(t *testing.T) {
	svc, _ := newTestMeets(t)
	ctx := context.Background()
	rec, err := svc.CreateMeet(ctx, organizer, ucMeetRequest())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddEvent(ctx, organizer, rec.ID, AddEventRequest{
		DisciplineCode: "100m", CategoryCodes: []string{"U16 W"},
	}); err != nil {
		t.Fatal(err)
	}

	d, err := svc.Meet(ctx, rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	unit := d.Units[0]
	t1 := time.Date(2027, 6, 12, 14, 30, 0, 0, time.UTC)
	if err := svc.ScheduleUnit(ctx, organizer, unit.UnitID, unit.UnitVersion, t1, "Bahn 1"); err != nil {
		t.Fatalf("ScheduleUnit: %v", err)
	}
	if _, err := svc.PublishTimetable(ctx, organizer, rec.ID); err != nil {
		t.Fatalf("PublishTimetable: %v", err)
	}

	// Amend the unit's time and republish.
	d, err = svc.Meet(ctx, rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	unit = d.Units[0]
	t2 := t1.Add(45 * time.Minute)
	if err := svc.ScheduleUnit(ctx, organizer, unit.UnitID, unit.UnitVersion, t2, "Bahn 1"); err != nil {
		t.Fatalf("amend: %v", err)
	}
	v2, err := svc.PublishTimetable(ctx, organizer, rec.ID)
	if err != nil {
		t.Fatalf("republish: %v", err)
	}
	if v2.Version != 2 {
		t.Errorf("republished version = %d, want 2", v2.Version)
	}

	_, current, err := svc.PublicTimetable(ctx, rec.ID)
	if err != nil {
		t.Fatalf("PublicTimetable: %v", err)
	}
	if current.Version != 2 || !current.Entries[0].ScheduledAt.Equal(t2) {
		t.Errorf("public timetable = v%d at %v, want v2 at %v (UC-001 #4)",
			current.Version, current.Entries[0].ScheduledAt, t2)
	}

	versions, err := svc.TimetableVersions(ctx, rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 {
		t.Fatalf("got %d retained versions, want 2 (SYS-004)", len(versions))
	}
	if versions[1].PublishedAt.IsZero() || !versions[1].Entries[0].ScheduledAt.Equal(t1) {
		t.Errorf("retained v1 = %+v, want original time %v with timestamp", versions[1], t1)
	}

	// Before anything is published, the public read reports not-found.
	other, err := svc.CreateMeet(ctx, organizer, ucMeetRequest())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.PublicTimetable(ctx, other.ID); !errors.Is(err, ErrMeetNotFound) {
		t.Errorf("PublicTimetable before publish = %v, want not-found", err)
	}
}

// TestSanctioningSummaryUC001_5 covers UC-001 #5: the summary contains
// tier, venue (with homologation reference), dates, organizer, categories
// and disciplines, with completeness checked against SYS-006.
func TestSanctioningSummaryUC001_5(t *testing.T) {
	svc, _ := newTestMeets(t)
	ctx := context.Background()
	rec, err := svc.CreateMeet(ctx, organizer, ucMeetRequest())
	if err != nil {
		t.Fatal(err)
	}

	// No programme yet: incomplete (categories/disciplines missing).
	sum, err := svc.SanctioningSummary(ctx, rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ok, missing := sum.Complete(); ok || len(missing) == 0 {
		t.Errorf("summary without programme reported complete")
	}

	for _, code := range []string{"100m", "SP"} {
		if _, err := svc.AddEvent(ctx, organizer, rec.ID, AddEventRequest{
			DisciplineCode: code, CategoryCodes: []string{"U16 W"},
		}); err != nil {
			t.Fatal(err)
		}
	}
	sum, err = svc.SanctioningSummary(ctx, rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ok, missing := sum.Complete(); !ok {
		t.Errorf("summary incomplete, missing %v (SYS-006)", missing)
	}
	if len(sum.Categories) != 1 || sum.Categories[0] != "U16 W" {
		t.Errorf("categories = %v, want [U16 W]", sum.Categories)
	}
	if len(sum.Disciplines) != 2 {
		t.Errorf("disciplines = %v, want the two programme disciplines", sum.Disciplines)
	}
	if sum.Meet.Tier != "C-Meeting" || sum.Meet.HomologationRef != "CH-ZH-042" {
		t.Errorf("summary meet fields = %+v", sum.Meet)
	}
	if len(sum.Sessions) != 4 || sum.GeneratedAt.IsZero() {
		t.Errorf("sessions=%d generatedAt=%v", len(sum.Sessions), sum.GeneratedAt)
	}
}

func TestCategorySchemesAndCatalogAccessors(t *testing.T) {
	svc, _ := newTestMeets(t)
	ids := svc.CategorySchemes()
	if len(ids) != 2 {
		t.Fatalf("CategorySchemes = %v, want the two built-ins", ids)
	}
	if _, ok := svc.Scheme(domain.SchemeSwissAthletics); !ok {
		t.Error("Scheme(swiss-athletics) not found")
	}
	if svc.Catalog() == nil {
		t.Error("Catalog() is nil")
	}
}

// TestMeetServiceAuthorizationMatrix verifies every mutation rejects an
// actor without CapOrganizeMeet (SYS-090 least privilege), and read paths
// surface not-found for unknown meets.
func TestMeetServiceAuthorizationMatrix(t *testing.T) {
	svc, _ := newTestMeets(t)
	ctx := context.Background()
	office := Session{AccountID: "01OFF", Username: "office", Role: RoleEntrySubmitter}

	var forbidden ErrForbidden
	if err := svc.UpdateMeet(ctx, office, "m", 1, ucMeetRequest()); !errors.As(err, &forbidden) {
		t.Errorf("UpdateMeet = %v, want ErrForbidden", err)
	}
	if err := svc.ArchiveMeet(ctx, office, "m", 1); !errors.As(err, &forbidden) {
		t.Errorf("ArchiveMeet = %v, want ErrForbidden", err)
	}
	if _, err := svc.AddEvent(ctx, office, "m", AddEventRequest{}); !errors.As(err, &forbidden) {
		t.Errorf("AddEvent = %v, want ErrForbidden", err)
	}
	if err := svc.ScheduleUnit(ctx, office, "u", 1, time.Now(), ""); !errors.As(err, &forbidden) {
		t.Errorf("ScheduleUnit = %v, want ErrForbidden", err)
	}
	if _, err := svc.PublishTimetable(ctx, office, "m"); !errors.As(err, &forbidden) {
		t.Errorf("PublishTimetable = %v, want ErrForbidden", err)
	}

	// Unknown targets surface not-found (never a silent no-op).
	if err := svc.UpdateMeet(ctx, organizer, "no-such", 1, ucMeetRequest()); !errors.Is(err, ErrMeetNotFound) {
		t.Errorf("UpdateMeet unknown meet = %v, want not-found", err)
	}
	if err := svc.ArchiveMeet(ctx, organizer, "no-such", 1); !errors.Is(err, ErrMeetNotFound) {
		t.Errorf("ArchiveMeet unknown meet = %v, want not-found", err)
	}
	if _, err := svc.PublishTimetable(ctx, organizer, "no-such"); !errors.Is(err, ErrMeetNotFound) {
		t.Errorf("PublishTimetable unknown meet = %v, want not-found", err)
	}
	if err := svc.ScheduleUnit(ctx, organizer, "no-such", 1, time.Now(), ""); !errors.Is(err, ErrMeetNotFound) {
		t.Errorf("ScheduleUnit unknown unit = %v, want not-found", err)
	}
	if _, err := svc.Meet(ctx, "no-such"); !errors.Is(err, ErrMeetNotFound) {
		t.Errorf("Meet unknown = %v, want not-found", err)
	}
	if _, err := svc.SanctioningSummary(ctx, "no-such"); !errors.Is(err, ErrMeetNotFound) {
		t.Errorf("SanctioningSummary unknown = %v, want not-found", err)
	}
}

// TestUpdateMeetValidationAndConflictPaths exercises the request
// validation and stale-version branches of UpdateMeet.
func TestUpdateMeetValidationPaths(t *testing.T) {
	svc, _ := newTestMeets(t)
	ctx := context.Background()
	rec, err := svc.CreateMeet(ctx, organizer, ucMeetRequest())
	if err != nil {
		t.Fatal(err)
	}

	bad := ucMeetRequest()
	bad.CategorySchemeID = "nope"
	if err := svc.UpdateMeet(ctx, organizer, rec.ID, rec.Version, bad); err == nil {
		t.Error("expected scheme validation error")
	}
	bad = ucMeetRequest()
	bad.EndDate = bad.StartDate.AddDate(0, 0, -1)
	bad.Sessions = nil
	if err := svc.UpdateMeet(ctx, organizer, rec.ID, rec.Version, bad); err == nil {
		t.Error("expected date validation error")
	}
}
