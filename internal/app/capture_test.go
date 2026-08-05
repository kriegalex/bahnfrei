// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/kriegalex/bahnfrei/internal/domain"
	"github.com/kriegalex/bahnfrei/internal/store"
)

var fieldOfficial = Session{AccountID: "01FLD", Username: "field", Role: RoleFieldOfficial}

// unitOf resolves the single unit of a meet's discipline in tests, and
// grants the shared fieldOfficial test account capture access to it
// (TASK-013 scopes field officials to assigned units, SYS-090; these
// fixtures predate that scoping, so granting it here keeps every existing
// capture/sync test exercising the same "authorized field official" it
// always has, without editing every call site). Tests that specifically
// exercise the scoping denial path construct their own unassigned session
// instead (see TestFieldOfficialEventScope* in accounts_test.go).
func unitOf(t *testing.T, results *ResultsService, meets *MeetService, meetID, disciplineCode string) string {
	t.Helper()
	detail, err := meets.Meet(context.Background(), meetID)
	if err != nil {
		t.Fatalf("Meet: %v", err)
	}
	for _, u := range detail.Units {
		if u.DisciplineCode == disciplineCode {
			if err := store.AssignFieldOfficialUnit(context.Background(), results.db, fieldOfficial.AccountID, meetID, u.UnitID); err != nil {
				t.Fatalf("AssignFieldOfficialUnit: %v", err)
			}
			return u.UnitID
		}
	}
	t.Fatalf("meet %s has no %s unit", meetID, disciplineCode)
	return ""
}

func fieldAttempt(t *testing.T, results *ResultsService, meetID, unitID string, in FieldAttemptInput) store.AttemptRecord {
	t.Helper()
	rec, err := results.SaveFieldAttempt(context.Background(), fieldOfficial, meetID, unitID, in)
	if err != nil {
		t.Fatalf("SaveFieldAttempt(%+v): %v", in, err)
	}
	return rec
}

// TestUC011_FieldCaptureGridAndTieBreak walks the UKC zone long jump
// through the horizontal-attempt grid (UC-011 #2/#4 at the service level):
// attempts settle into a scored best-mark result after every save, and the
// unit standings apply the next-best tie-break.
func TestUC011_FieldCaptureGridAndTieBreak(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec := createUKCMeet(t, meets)
	unitID := unitOf(t, results, meets, rec.ID, "ZoneLJ")
	anna := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2014, Sex: domain.SexFemale, Bib: "101",
	})
	bea := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Bea", LastName: "Beispiel", BirthYear: 2014, Sex: domain.SexFemale, Bib: "102",
	})

	// Anna: 3.42, X, 3.10 — Bea: 3.20, 3.42, 3.30. Equal bests; Bea's
	// better second-best (3.30 > 3.10) decides (UC-011 #2).
	fieldAttempt(t, results, rec.ID, unitID, FieldAttemptInput{AthleteID: anna.AthleteID, Seq: 1, Kind: domain.AttemptValid, Mark: "3.42"})
	fieldAttempt(t, results, rec.ID, unitID, FieldAttemptInput{AthleteID: anna.AthleteID, Seq: 2, Kind: domain.AttemptFoul})
	fieldAttempt(t, results, rec.ID, unitID, FieldAttemptInput{AthleteID: anna.AthleteID, Seq: 3, Kind: domain.AttemptValid, Mark: "3.10"})
	fieldAttempt(t, results, rec.ID, unitID, FieldAttemptInput{AthleteID: bea.AthleteID, Seq: 1, Kind: domain.AttemptValid, Mark: "3.20"})
	fieldAttempt(t, results, rec.ID, unitID, FieldAttemptInput{AthleteID: bea.AthleteID, Seq: 2, Kind: domain.AttemptValid, Mark: "3.42"})
	fieldAttempt(t, results, rec.ID, unitID, FieldAttemptInput{AthleteID: bea.AthleteID, Seq: 3, Kind: domain.AttemptValid, Mark: "3.30"})

	v, err := results.UnitCapture(ctx, rec.ID, unitID)
	if err != nil {
		t.Fatalf("UnitCapture: %v", err)
	}
	if v.Family != domain.FamilyFieldHorizontal || v.Config.Attempts != 3 || v.Config.CutAfter != 0 {
		t.Errorf("UKC config = %+v, want 3 trials, no cut (template rule data)", v.Config)
	}
	if v.WindRelevant {
		t.Error("ZoneLJ is not wind-relevant (catalog data)")
	}
	if len(v.Rows) != 2 || len(v.Rows[0].Attempts) != 3 {
		t.Fatalf("grid = %d rows × %d attempts", len(v.Rows), len(v.Rows[0].Attempts))
	}
	if got := v.Rows[0].Attempts[1]; got == nil || got.Kind != domain.AttemptFoul {
		t.Errorf("anna trial 2 = %+v, want foul", got)
	}
	if len(v.Standings) != 2 || v.Standings[0].AthleteID != bea.AthleteID || v.Standings[0].Rank != 1 {
		t.Fatalf("standings = %+v, want Bea first by next-best tie-break", v.Standings)
	}
	if v.Standings[1].AthleteID != anna.AthleteID || v.Standings[1].Rank != 2 {
		t.Errorf("standings = %+v, want Anna second", v.Standings)
	}
	if v.Continuation != nil {
		t.Errorf("continuation = %v, want none without a cut", v.Continuation)
	}

	// Every save settles a scored result (UC-033 #2 alignment): the best
	// mark carries the UKC points for the athlete's sex column.
	tables, err := domain.BuiltinScoringTables()
	if err != nil {
		t.Fatal(err)
	}
	wantPts, err := tables[domain.ScoringTableUBSKidsCup].Points("ZoneLJ", domain.TimingNone, domain.SexFemale, "3.42")
	if err != nil {
		t.Fatal(err)
	}
	if r := v.Rows[0].Result; r == nil || r.Mark != "3.42" || r.Points == nil || *r.Points != wantPts {
		t.Errorf("anna settled result = %+v, want best 3.42 with %d points", v.Rows[0].Result, wantPts)
	}
}

// TestUC011_3_RetireeStillRanks: after a valid mark, an `r` attempt keeps
// the best mark ranked with status r.
func TestUC011_3_RetireeStillRanks(t *testing.T) {
	meets, results, _ := newTestResults(t)
	rec := createUKCMeet(t, meets)
	unitID := unitOf(t, results, meets, rec.ID, "ZoneLJ")
	anna := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2014, Sex: domain.SexFemale, Bib: "101",
	})
	bea := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Bea", LastName: "Beispiel", BirthYear: 2014, Sex: domain.SexFemale, Bib: "102",
	})
	fieldAttempt(t, results, rec.ID, unitID, FieldAttemptInput{AthleteID: anna.AthleteID, Seq: 1, Kind: domain.AttemptValid, Mark: "3.50"})
	fieldAttempt(t, results, rec.ID, unitID, FieldAttemptInput{AthleteID: anna.AthleteID, Seq: 2, Kind: domain.AttemptRetire})
	fieldAttempt(t, results, rec.ID, unitID, FieldAttemptInput{AthleteID: bea.AthleteID, Seq: 1, Kind: domain.AttemptValid, Mark: "3.40"})

	v, err := results.UnitCapture(context.Background(), rec.ID, unitID)
	if err != nil {
		t.Fatal(err)
	}
	if v.Standings[0].AthleteID != anna.AthleteID || v.Standings[0].Rank != 1 || v.Standings[0].Status != domain.StatusR {
		t.Errorf("standings = %+v, want retiree first with status r (UC-011 #3)", v.Standings)
	}
	if v.Standings[0].Points == nil {
		t.Error("retiree's best mark must keep its points")
	}
}

// TestUC011_1_DefaultSeriesCutAndContinuation: a non-template meet gets the
// WA default series (3+3, cut to top 8) and, once round 3 is complete, the
// continuation in reverse-ranking order.
func TestUC011_1_DefaultSeriesCutAndContinuation(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	meet, err := meets.CreateMeet(ctx, organizer, MeetRequest{
		Name: "Abendmeeting", Venue: "Wankdorf",
		StartDate: ukcDay(), EndDate: ukcDay(),
		CategorySchemeID: domain.SchemeSwissAthletics,
	})
	if err != nil {
		t.Fatalf("CreateMeet: %v", err)
	}
	if _, err := meets.AddEvent(ctx, organizer, meet.ID, AddEventRequest{
		DisciplineCode: "LJ", CategoryCodes: []string{"U18 M"},
	}); err != nil {
		t.Fatalf("AddEvent: %v", err)
	}
	unitID := unitOf(t, results, meets, meet.ID, "LJ")

	w := 1.2
	var athletes []string
	for i := 1; i <= 9; i++ {
		p := register(t, results, meet.ID, ParticipantInput{
			FirstName: "A", LastName: fmt.Sprintf("Jumper%02d", i), BirthYear: 2009,
			Sex: domain.SexMale, Bib: fmt.Sprintf("%d", i),
		})
		athletes = append(athletes, p.AthleteID)
		// Bests 6.01..6.09; LJ is wind-relevant, so per-attempt wind is legal.
		fieldAttempt(t, results, meet.ID, unitID, FieldAttemptInput{
			AthleteID: p.AthleteID, Seq: 1, Kind: domain.AttemptValid,
			Mark: fmt.Sprintf("6.%02d", i), Wind: &w,
		})
		fieldAttempt(t, results, meet.ID, unitID, FieldAttemptInput{AthleteID: p.AthleteID, Seq: 2, Kind: domain.AttemptPass})
		fieldAttempt(t, results, meet.ID, unitID, FieldAttemptInput{AthleteID: p.AthleteID, Seq: 3, Kind: domain.AttemptFoul})
	}

	v, err := results.UnitCapture(ctx, meet.ID, unitID)
	if err != nil {
		t.Fatal(err)
	}
	if v.Config != (CaptureConfig{Attempts: 6, CutAfter: 3, CutTo: 8}) {
		t.Fatalf("default config = %+v", v.Config)
	}
	if len(v.Continuation) != 8 {
		t.Fatalf("continuation = %v, want the top 8 (UC-011 #1)", v.Continuation)
	}
	// Reverse-ranking order: 8th best (jumper 2, 6.02) jumps first, best
	// (jumper 9, 6.09) last; jumper 1 (6.01) is cut.
	if v.Continuation[0] != athletes[1] || v.Continuation[7] != athletes[8] {
		t.Errorf("continuation order = %v, want reverse ranking", v.Continuation)
	}
	for _, id := range v.Continuation {
		if id == athletes[0] {
			t.Error("the 9th-ranked athlete must be cut after round 3")
		}
	}
	// No points on a meet without a scoring table.
	if v.Standings[0].Points != nil {
		t.Errorf("points = %v, want nil without a scoring table", v.Standings[0].Points)
	}
}

// TestUC010_2_HandTimeRoundUpAndProvenance: a manual 60 m time rounds up to
// the next 0.1 s and stays distinguishable from FAT (SYS-041), scoring
// against the manual column of the UKC table.
func TestUC010_2_HandTimeRoundUpAndProvenance(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec := createUKCMeet(t, meets)
	unitID := unitOf(t, results, meets, rec.ID, "60m")
	anna := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2014, Sex: domain.SexFemale, Bib: "101",
	})
	bea := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Bea", LastName: "Beispiel", BirthYear: 2014, Sex: domain.SexFemale, Bib: "102",
	})

	manual, err := results.SaveTrackResult(ctx, fieldOfficial, rec.ID, unitID, TrackResultInput{
		AthleteID: anna.AthleteID, Time: "9.32", Timing: domain.TimingManual,
	})
	if err != nil {
		t.Fatalf("SaveTrackResult(manual): %v", err)
	}
	if manual.Mark != "9.4" || manual.Timing != domain.TimingManual {
		t.Errorf("manual result = mark %q timing %q, want 9.4/manual (D5.1 round-up)", manual.Mark, manual.Timing)
	}
	tables, err := domain.BuiltinScoringTables()
	if err != nil {
		t.Fatal(err)
	}
	wantPts, err := tables[domain.ScoringTableUBSKidsCup].Points("60m", domain.TimingManual, domain.SexFemale, "9.4")
	if err != nil {
		t.Fatal(err)
	}
	if manual.Points == nil || *manual.Points != wantPts {
		t.Errorf("manual points = %v, want %d (manual scoring column)", manual.Points, wantPts)
	}

	fat, err := results.SaveTrackResult(ctx, fieldOfficial, rec.ID, unitID, TrackResultInput{
		AthleteID: bea.AthleteID, Time: "9.32", Timing: domain.TimingElectronic,
	})
	if err != nil {
		t.Fatalf("SaveTrackResult(electronic): %v", err)
	}
	if fat.Mark != "9.32" || fat.Timing != domain.TimingElectronic {
		t.Errorf("FAT result = mark %q timing %q, want unrounded 9.32/electronic (SYS-040)", fat.Mark, fat.Timing)
	}

	v, err := results.UnitCapture(ctx, rec.ID, unitID)
	if err != nil {
		t.Fatal(err)
	}
	// 9.32 (FAT) ranks ahead of 9.4 (manual); provenance travels with each row.
	if v.Standings[0].AthleteID != bea.AthleteID || v.Standings[0].Timing != domain.TimingElectronic {
		t.Errorf("track standings = %+v", v.Standings)
	}
	if v.Standings[1].Timing != domain.TimingManual {
		t.Errorf("manual provenance lost: %+v", v.Standings[1])
	}
}

// TestUC010_3_StatusVocabulary: statuses are restricted to the CR 25
// capture set; a DQ without a rule reference is rejected (SYS-045).
func TestUC010_3_StatusVocabulary(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec := createUKCMeet(t, meets)
	unitID := unitOf(t, results, meets, rec.ID, "60m")
	anna := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2014, Sex: domain.SexFemale, Bib: "101",
	})

	if _, err := results.SaveTrackResult(ctx, fieldOfficial, rec.ID, unitID, TrackResultInput{
		AthleteID: anna.AthleteID, Status: domain.StatusDQ,
	}); err == nil {
		t.Fatal("DQ without rule reference must be rejected (SYS-045)")
	}
	if _, err := results.SaveTrackResult(ctx, fieldOfficial, rec.ID, unitID, TrackResultInput{
		AthleteID: anna.AthleteID, Status: domain.StatusQ,
	}); err == nil {
		t.Fatal("progression codes are not capture statuses (SYS-045)")
	}
	dq, err := results.SaveTrackResult(ctx, fieldOfficial, rec.ID, unitID, TrackResultInput{
		AthleteID: anna.AthleteID, Status: domain.StatusDQ, StatusDetail: "TR16.8",
	})
	if err != nil {
		t.Fatalf("SaveTrackResult(DQ TR16.8): %v", err)
	}
	if got := domain.RenderStatus(dq.Status, dq.StatusDetail); got != "DQ (TR16.8)" {
		t.Errorf("rendered status = %q (CR 25 convention)", got)
	}
	if dq.Points != nil {
		t.Errorf("a DQ scores no points, got %v", dq.Points)
	}
}

// TestCaptureConflictSurfaced is UC-021 #2 at the service level: a
// concurrent capture of the same trial returns AttemptConflictError with
// the stored attempt — both versions available, nothing overwritten.
func TestCaptureConflictSurfaced(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec := createUKCMeet(t, meets)
	unitID := unitOf(t, results, meets, rec.ID, "ZoneLJ")
	anna := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2014, Sex: domain.SexFemale, Bib: "101",
	})
	fieldAttempt(t, results, rec.ID, unitID, FieldAttemptInput{AthleteID: anna.AthleteID, Seq: 1, Kind: domain.AttemptValid, Mark: "3.42"})

	_, err := results.SaveFieldAttempt(ctx, fieldOfficial, rec.ID, unitID, FieldAttemptInput{
		AthleteID: anna.AthleteID, Seq: 1, Kind: domain.AttemptValid, Mark: "3.10",
	})
	var conflict *AttemptConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("concurrent capture = %v, want AttemptConflictError", err)
	}
	if !errors.Is(err, ErrConflict) {
		t.Error("conflict must match ErrConflict for generic handling")
	}
	if conflict.Current.Mark != "3.42" {
		t.Errorf("conflict carries %q, want the stored 3.42", conflict.Current.Mark)
	}

	// Re-capture with the stored version succeeds (an authorized correction).
	if _, err := results.SaveFieldAttempt(ctx, fieldOfficial, rec.ID, unitID, FieldAttemptInput{
		AthleteID: anna.AthleteID, Seq: 1, Kind: domain.AttemptValid, Mark: "3.10",
		ExpectedVersion: conflict.Current.Version,
	}); err != nil {
		t.Fatalf("versioned correction: %v", err)
	}
}

func TestCaptureValidationAndAuthorization(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec := createUKCMeet(t, meets)
	ljUnit := unitOf(t, results, meets, rec.ID, "ZoneLJ")
	trackUnit := unitOf(t, results, meets, rec.ID, "60m")
	anna := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2014, Sex: domain.SexFemale, Bib: "101",
	})
	submitter := Session{AccountID: "01SUB", Username: "club", Role: RoleEntrySubmitter}

	var forbidden ErrForbidden
	if _, err := results.SaveFieldAttempt(ctx, submitter, rec.ID, ljUnit, FieldAttemptInput{
		AthleteID: anna.AthleteID, Seq: 1, Kind: domain.AttemptValid, Mark: "3.00",
	}); !errors.As(err, &forbidden) {
		t.Errorf("entry submitter capturing = %v, want ErrForbidden (SYS-090)", err)
	}
	if _, err := results.SaveFieldAttempt(ctx, fieldOfficial, rec.ID, trackUnit, FieldAttemptInput{
		AthleteID: anna.AthleteID, Seq: 1, Kind: domain.AttemptValid, Mark: "3.00",
	}); err == nil {
		t.Error("attempt grid on a track unit must be rejected")
	}
	if _, err := results.SaveTrackResult(ctx, fieldOfficial, rec.ID, ljUnit, TrackResultInput{
		AthleteID: anna.AthleteID, Time: "9.40", Timing: domain.TimingManual,
	}); err == nil {
		t.Error("track capture on a field unit must be rejected")
	}
	if _, err := results.SaveFieldAttempt(ctx, fieldOfficial, rec.ID, ljUnit, FieldAttemptInput{
		AthleteID: "01GHOST", Seq: 1, Kind: domain.AttemptValid, Mark: "3.00",
	}); err == nil {
		t.Error("capture for an unregistered athlete must be rejected")
	}
	if _, err := results.SaveFieldAttempt(ctx, fieldOfficial, rec.ID, ljUnit, FieldAttemptInput{
		AthleteID: anna.AthleteID, Seq: 4, Kind: domain.AttemptValid, Mark: "3.00",
	}); err == nil {
		t.Error("trial 4 exceeds the UKC 3-trial series (SYS-042)")
	}
	w := 1.0
	if _, err := results.SaveFieldAttempt(ctx, fieldOfficial, rec.ID, ljUnit, FieldAttemptInput{
		AthleteID: anna.AthleteID, Seq: 1, Kind: domain.AttemptValid, Mark: "3.00", Wind: &w,
	}); err == nil {
		t.Error("wind on the non-wind-relevant ZoneLJ must be rejected")
	}
	if _, err := results.SaveTrackResult(ctx, fieldOfficial, rec.ID, trackUnit, TrackResultInput{
		AthleteID: anna.AthleteID, Time: "9.40",
	}); err == nil {
		t.Error("a time without a timing method must be rejected (SYS-041)")
	}
	if _, err := results.UnitCapture(ctx, rec.ID, "01NOUNIT"); !errors.Is(err, ErrMeetNotFound) {
		t.Errorf("UnitCapture(unknown unit) = %v, want not-found", err)
	}
}

// TestOnResultsChangedHook: every committed capture fires the live-update
// hook with the meet ID (UC-011 #4 — the SSE publication seam).
func TestOnResultsChangedHook(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec := createUKCMeet(t, meets)
	anna := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2014, Sex: domain.SexFemale, Bib: "101",
	})
	var fired []string
	results.OnResultsChanged(func(meetID string) { fired = append(fired, meetID) })

	fieldAttempt(t, results, rec.ID, unitOf(t, results, meets, rec.ID, "ZoneLJ"), FieldAttemptInput{
		AthleteID: anna.AthleteID, Seq: 1, Kind: domain.AttemptValid, Mark: "3.42",
	})
	if _, err := results.SaveTrackResult(ctx, fieldOfficial, rec.ID, unitOf(t, results, meets, rec.ID, "60m"), TrackResultInput{
		AthleteID: anna.AthleteID, Time: "9.32", Timing: domain.TimingManual,
	}); err != nil {
		t.Fatal(err)
	}
	if len(fired) != 2 || fired[0] != rec.ID || fired[1] != rec.ID {
		t.Errorf("hook fired = %v, want twice with the meet ID", fired)
	}
}

// TestUnitCaptureShowsLaneContextSYS026SYS027 proves the capture unit view
// gains lane context once heat seeding runs (TASK-018 unlocking full track
// capture, TASK-019): a generated heat's drawn lanes surface on the
// capture grid's rows, read-only, without touching attempt-capture
// semantics — SaveTrackResult still works exactly as before.
func TestUnitCaptureShowsLaneContextSYS026SYS027(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	meet, err := meets.CreateMeet(ctx, organizer, ucMeetRequest())
	if err != nil {
		t.Fatalf("CreateMeet: %v", err)
	}
	ev, err := meets.AddEvent(ctx, organizer, meet.ID, AddEventRequest{
		DisciplineCode: "100m", CategoryCodes: []string{"U18 W"},
	})
	if err != nil {
		t.Fatalf("AddEvent: %v", err)
	}
	if err := meets.PublishMeet(ctx, organizer, meet.ID, meet.Version); err != nil {
		t.Fatalf("PublishMeet: %v", err)
	}

	entrySubmitterSession := Session{AccountID: "01ESB", Role: RoleEntrySubmitter}
	var athleteIDs []string
	for i := 0; i < 4; i++ {
		detail, err := results.SubmitIndividualEntry(ctx, entrySubmitterSession, meet.ID, IndividualEntryInput{
			EventID: ev.ID, FirstName: "Athlete", LastName: fmt.Sprintf("L%d", i),
			BirthYear: 2009, Sex: domain.SexFemale, SeedPerformance: fmt.Sprintf("12.%02d", 50+i),
		})
		if err != nil {
			t.Fatalf("SubmitIndividualEntry: %v", err)
		}
		if err := results.ConfirmCheckIn(ctx, office, meet.ID, detail.ID, detail.Version); err != nil {
			t.Fatalf("ConfirmCheckIn: %v", err)
		}
		athleteIDs = append(athleteIDs, detail.AthleteID)
	}

	rounds, err := store.ListRounds(ctx, results.db, ev.ID)
	if err != nil {
		t.Fatalf("ListRounds: %v", err)
	}
	sheet, err := results.GenerateHeats(ctx, office, meet.ID, ev.ID, rounds[0].ID, GenerateHeatsRequest{MaxHeatSize: 4, TrackLanes: 8})
	if err != nil {
		t.Fatalf("GenerateHeats: %v", err)
	}
	if len(sheet.Units) != 1 {
		t.Fatalf("expected a single heat for 4 entries, got %d", len(sheet.Units))
	}
	unitID := sheet.Units[0].UnitID

	v, err := results.UnitCapture(ctx, meet.ID, unitID)
	if err != nil {
		t.Fatalf("UnitCapture: %v", err)
	}
	seenLane := 0
	for _, row := range v.Rows {
		if row.Lane != 0 {
			seenLane++
		}
	}
	if seenLane != 4 {
		t.Fatalf("expected all 4 seeded athletes to carry lane context on the capture grid, got %d", seenLane)
	}

	// Capturing a result still works exactly as before — lane context is
	// read-only display data, never part of attempt/result versioning.
	if _, err := results.SaveTrackResult(ctx, office, meet.ID, unitID, TrackResultInput{
		AthleteID: athleteIDs[0], Time: "12.34", Timing: domain.TimingElectronic,
	}); err != nil {
		t.Fatalf("SaveTrackResult unaffected by lane context: %v", err)
	}
}

// --- TASK-041: bulk "mark remaining as DNS" (DEC-025/OQ-070,
// SYS-114/SYS-046) ---

// bulkDNSFixture is a UKC 60m unit with three registered athletes, none
// captured yet, for the bulk-DNS test family below.
type bulkDNSFixture struct {
	results          *ResultsService
	meetID, unitID   string
	anna, bea, clara store.ParticipantRow
}

func newBulkDNSFixture(t *testing.T) bulkDNSFixture {
	t.Helper()
	meets, results, _ := newTestResults(t)
	rec := createUKCMeet(t, meets)
	unitID := unitOf(t, results, meets, rec.ID, "60m")
	anna := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2014, Sex: domain.SexFemale, Bib: "101",
	})
	bea := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Bea", LastName: "Beispiel", BirthYear: 2014, Sex: domain.SexFemale, Bib: "102",
	})
	clara := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Clara", LastName: "Muster", BirthYear: 2014, Sex: domain.SexFemale, Bib: "103",
	})
	return bulkDNSFixture{results: results, meetID: rec.ID, unitID: unitID, anna: anna, bea: bea, clara: clara}
}

// TestBulkMarkRemainingDNSOnlyUnresultedMarkedSYS114SYS046 is TASK-041's
// core scenario: Anna already has a captured time; the bulk action must
// mark only Bea and Clara DNS, leave Anna's real result untouched, and
// audit each DNS write per SYS-046 (mirroring CloseCheckIn's per-entry
// audit shape, checkin.go).
func TestBulkMarkRemainingDNSOnlyUnresultedMarkedSYS114SYS046(t *testing.T) {
	f := newBulkDNSFixture(t)
	ctx := context.Background()

	anna, err := f.results.SaveTrackResult(ctx, office, f.meetID, f.unitID, TrackResultInput{
		AthleteID: f.anna.AthleteID, Time: "9.32", Timing: domain.TimingElectronic,
	})
	if err != nil {
		t.Fatalf("SaveTrackResult(Anna): %v", err)
	}

	preview, err := f.results.BulkDNSCandidateCount(ctx, office, f.meetID, f.unitID)
	if err != nil {
		t.Fatalf("BulkDNSCandidateCount: %v", err)
	}
	if preview != 2 {
		t.Fatalf("BulkDNSCandidateCount = %d, want 2 (Bea, Clara)", preview)
	}

	n, err := f.results.BulkMarkRemainingDNS(ctx, office, f.meetID, f.unitID)
	if err != nil {
		t.Fatalf("BulkMarkRemainingDNS: %v", err)
	}
	if n != 2 {
		t.Fatalf("BulkMarkRemainingDNS marked %d entries, want 2 (Bea, Clara)", n)
	}

	// Anna's real result is untouched (not just "still DNS-free" but the
	// exact mark/timing she captured).
	gotAnna, err := store.GetResult(ctx, f.results.db, f.unitID, f.anna.AthleteID)
	if err != nil {
		t.Fatalf("GetResult(Anna): %v", err)
	}
	if gotAnna.Mark != anna.Mark || gotAnna.Status != domain.StatusNone {
		t.Errorf("Anna's result = %+v, want untouched (mark %q, no status)", gotAnna, anna.Mark)
	}

	for _, p := range []store.ParticipantRow{f.bea, f.clara} {
		got, err := store.GetResult(ctx, f.results.db, f.unitID, p.AthleteID)
		if err != nil {
			t.Fatalf("GetResult(%s): %v", p.Athlete.FirstName, err)
		}
		if got.Status != domain.StatusDNS {
			t.Errorf("%s's status = %q, want DNS", p.Athlete.FirstName, got.Status)
		}
	}

	// A second call is idempotent-safe: nothing left to mark.
	n2, err := f.results.BulkMarkRemainingDNS(ctx, office, f.meetID, f.unitID)
	if err != nil {
		t.Fatalf("BulkMarkRemainingDNS (second call): %v", err)
	}
	if n2 != 0 {
		t.Errorf("second BulkMarkRemainingDNS marked %d, want 0 (already resolved)", n2)
	}

	logged, err := store.ListAudit(ctx, f.results.db, 50)
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	seen := map[string]bool{}
	for _, e := range logged {
		if e.Action == "capture.bulk_dns" && e.Actor == office.AccountID {
			seen[e.EntityID] = true
		}
	}
	if len(seen) != 2 {
		t.Errorf("expected 2 distinct capture.bulk_dns audit entries, got %d (%v)", len(seen), logged)
	}
}

// TestBulkMarkRemainingDNSRejectsAnnouncedUnitSYS114 is a denial/edge-path
// test: a unit whose results are already announced must go through the
// correction flow (UC-015 #2), not the bulk action — the same guard plain
// capture uses (requireNotAnnounced).
func TestBulkMarkRemainingDNSRejectsAnnouncedUnitSYS114(t *testing.T) {
	f := newBulkDNSFixture(t)
	ctx := context.Background()
	if _, err := f.results.AnnounceUnitResults(ctx, office, f.meetID, f.unitID); err != nil {
		t.Fatalf("AnnounceUnitResults: %v", err)
	}
	if _, err := f.results.BulkDNSCandidateCount(ctx, office, f.meetID, f.unitID); !errors.Is(err, ErrCorrectionRequired) {
		t.Errorf("BulkDNSCandidateCount(announced unit) = %v, want ErrCorrectionRequired", err)
	}
	if _, err := f.results.BulkMarkRemainingDNS(ctx, office, f.meetID, f.unitID); !errors.Is(err, ErrCorrectionRequired) {
		t.Errorf("BulkMarkRemainingDNS(announced unit) = %v, want ErrCorrectionRequired", err)
	}
}

// TestBulkMarkRemainingDNSRejectsNonTrackUnitSYS114 is a denial/edge-path
// test: the action is scoped to UC-010 track result entry (OQ-070) —
// horizontal field units have no equivalent "no captured result" bulk-safe
// meaning (per-attempt series, not a single per-athlete status).
func TestBulkMarkRemainingDNSRejectsNonTrackUnitSYS114(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec := createUKCMeet(t, meets)
	unitID := unitOf(t, results, meets, rec.ID, "ZoneLJ")

	if _, err := results.BulkDNSCandidateCount(ctx, office, rec.ID, unitID); !errors.Is(err, ErrBulkDNSTrackOnly) {
		t.Errorf("BulkDNSCandidateCount(field unit) = %v, want ErrBulkDNSTrackOnly", err)
	}
	if _, err := results.BulkMarkRemainingDNS(ctx, office, rec.ID, unitID); !errors.Is(err, ErrBulkDNSTrackOnly) {
		t.Errorf("BulkMarkRemainingDNS(field unit) = %v, want ErrBulkDNSTrackOnly", err)
	}
}

// TestBulkMarkRemainingDNSRequiresOfficeCapabilitySYS114 is the
// authorization edge-path test: a field official (below the office floor)
// cannot invoke the bulk action, matching CloseCheckIn/AnnounceUnitResults'
// own CapOfficeActions gate.
func TestBulkMarkRemainingDNSRequiresOfficeCapabilitySYS114(t *testing.T) {
	f := newBulkDNSFixture(t)
	ctx := context.Background()
	if _, err := f.results.BulkMarkRemainingDNS(ctx, fieldOfficial, f.meetID, f.unitID); !errors.As(err, &ErrForbidden{}) {
		t.Errorf("BulkMarkRemainingDNS(field official) = %v, want ErrForbidden", err)
	}
	if _, err := f.results.BulkDNSCandidateCount(ctx, fieldOfficial, f.meetID, f.unitID); !errors.As(err, &ErrForbidden{}) {
		t.Errorf("BulkDNSCandidateCount(field official) = %v, want ErrForbidden", err)
	}
}
