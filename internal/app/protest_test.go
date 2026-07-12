// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kriegalex/bahnfrei/internal/domain"
	"github.com/kriegalex/bahnfrei/internal/store"
)

// trackUnitFixture creates a meet with one wind-relevant (100m) and one
// non-wind-relevant (800m) track unit, plus one registered athlete, for the
// protest-clock/correction/wind test suite.
func trackUnitFixture(t *testing.T, meets *MeetService, results *ResultsService) (meetID, windUnit, noWindUnit, athleteID string) {
	t.Helper()
	ctx := context.Background()
	rec, err := meets.CreateMeet(ctx, organizer, ucMeetRequest())
	if err != nil {
		t.Fatalf("CreateMeet: %v", err)
	}
	if _, err := meets.AddEvent(ctx, organizer, rec.ID, AddEventRequest{
		DisciplineCode: "100m", CategoryCodes: []string{"U18 W"},
	}); err != nil {
		t.Fatalf("AddEvent(100m): %v", err)
	}
	if _, err := meets.AddEvent(ctx, organizer, rec.ID, AddEventRequest{
		DisciplineCode: "800m", CategoryCodes: []string{"U18 W"},
	}); err != nil {
		t.Fatalf("AddEvent(800m): %v", err)
	}
	windUnit = unitOf(t, results, meets, rec.ID, "100m")
	noWindUnit = unitOf(t, results, meets, rec.ID, "800m")
	athlete := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2009, Sex: domain.SexFemale, Bib: "1",
	})
	return rec.ID, windUnit, noWindUnit, athlete.AthleteID
}

// backdateAnnouncement rewrites a unit's latest announcement timestamp
// directly (bypassing the service, which always stamps now()) so tests can
// exercise the "protest window has elapsed" branch without sleeping 30
// real minutes.
func backdateAnnouncement(t *testing.T, results *ResultsService, unitID string, at time.Time) {
	t.Helper()
	if _, err := results.db.ExecContext(context.Background(),
		`UPDATE result_announcements SET announced_at = ? WHERE unit_id = ? AND seq = (SELECT MAX(seq) FROM result_announcements WHERE unit_id = ?)`,
		at.UTC().Format(time.RFC3339Nano), unitID, unitID); err != nil {
		t.Fatalf("backdateAnnouncement: %v", err)
	}
}

// TestSetUnitWindAppliesUniformlySYS040UC010_4 covers UC-010 #4: a wind
// reading is per race, not per athlete — it applies to every mark already
// captured on the unit, and to marks captured afterwards.
func TestSetUnitWindAppliesUniformlySYS040UC010_4(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec, windUnit, _, anna := trackUnitFixture(t, meets, results)

	if _, err := results.SaveTrackResult(ctx, fieldOfficial, rec, windUnit, TrackResultInput{
		AthleteID: anna, Time: "12.30", Timing: domain.TimingElectronic,
	}); err != nil {
		t.Fatalf("SaveTrackResult (pre-wind): %v", err)
	}

	if err := results.SetUnitWind(ctx, fieldOfficial, rec, windUnit, 1.4); err != nil {
		t.Fatalf("SetUnitWind: %v", err)
	}
	if wind, err := results.UnitWind(ctx, rec, windUnit); err != nil || wind == nil || *wind != 1.4 {
		t.Fatalf("UnitWind = %v, %v, want 1.4", wind, err)
	}
	got, err := store.GetResult(ctx, results.db, windUnit, anna)
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if got.Wind == nil || *got.Wind != 1.4 {
		t.Fatalf("existing result wind = %v, want 1.4 applied retroactively", got.Wind)
	}

	bea := register(t, results, rec, ParticipantInput{
		FirstName: "Bea", LastName: "Beispiel", BirthYear: 2009, Sex: domain.SexFemale, Bib: "2",
	})
	if _, err := results.SaveTrackResult(ctx, fieldOfficial, rec, windUnit, TrackResultInput{
		AthleteID: bea.AthleteID, Time: "12.10", Timing: domain.TimingElectronic,
	}); err != nil {
		t.Fatalf("SaveTrackResult (post-wind): %v", err)
	}
	got2, err := store.GetResult(ctx, results.db, windUnit, bea.AthleteID)
	if err != nil {
		t.Fatalf("GetResult(bea): %v", err)
	}
	if got2.Wind == nil || *got2.Wind != 1.4 {
		t.Errorf("newly captured result wind = %v, want the unit's current 1.4 (UC-010 #4)", got2.Wind)
	}
}

// TestSetUnitWindRejectsNonWindDisciplineSYS040 is the denial path: an 800m
// (not wind-relevant) unit must reject a wind reading outright.
func TestSetUnitWindRejectsNonWindDisciplineSYS040(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec, _, noWindUnit, _ := trackUnitFixture(t, meets, results)

	if err := results.SetUnitWind(ctx, fieldOfficial, rec, noWindUnit, 1.0); err == nil {
		t.Fatal("wind on a non-wind-relevant discipline (800m) must be rejected (SYS-040)")
	}
}

// TestAnnounceOpensProtestWindowSYS047UC015_1 covers UC-015 #1: announcing
// a unit records the timestamp and starts the 30-minute countdown.
func TestAnnounceOpensProtestWindowSYS047UC015_1(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec, windUnit, _, anna := trackUnitFixture(t, meets, results)
	if _, err := results.SaveTrackResult(ctx, fieldOfficial, rec, windUnit, TrackResultInput{
		AthleteID: anna, Time: "12.30", Timing: domain.TimingElectronic,
	}); err != nil {
		t.Fatalf("SaveTrackResult: %v", err)
	}

	before, err := results.UnitProtestState(ctx, rec, windUnit)
	if err != nil {
		t.Fatalf("UnitProtestState (before): %v", err)
	}
	if before.Announced {
		t.Fatal("a never-announced unit must report Announced=false")
	}

	state, err := results.AnnounceUnitResults(ctx, office, rec, windUnit)
	if err != nil {
		t.Fatalf("AnnounceUnitResults: %v", err)
	}
	if !state.Announced || state.Official {
		t.Fatalf("freshly announced state = %+v, want announced/provisional", state)
	}
	if state.Remaining <= 29*time.Minute || state.Remaining > domain.ProtestWindow {
		t.Errorf("remaining = %v, want close to 30m", state.Remaining)
	}

	// Field-official level cannot announce (office/UC-015 actor only).
	if _, err := results.AnnounceUnitResults(ctx, fieldOfficial, rec, windUnit); err == nil {
		t.Error("a field official must not be able to announce results (UC-015 actor is competition office)")
	}
}

// TestCaptureAfterAnnouncementRequiresCorrectionSYS046SYS047UC015_2 proves
// the capture/correction boundary: plain SaveTrackResult is rejected once a
// unit is announced, and CorrectResult with a reason succeeds, re-announcing
// the unit (opening a fresh appeal window) and leaving an audit trail.
func TestCaptureAfterAnnouncementRequiresCorrectionSYS046SYS047UC015_2(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec, windUnit, _, anna := trackUnitFixture(t, meets, results)
	if _, err := results.SaveTrackResult(ctx, fieldOfficial, rec, windUnit, TrackResultInput{
		AthleteID: anna, Time: "12.30", Timing: domain.TimingElectronic,
	}); err != nil {
		t.Fatalf("SaveTrackResult: %v", err)
	}
	if _, err := results.AnnounceUnitResults(ctx, office, rec, windUnit); err != nil {
		t.Fatalf("AnnounceUnitResults: %v", err)
	}

	if _, err := results.SaveTrackResult(ctx, fieldOfficial, rec, windUnit, TrackResultInput{
		AthleteID: anna, Time: "12.20", Timing: domain.TimingElectronic,
	}); !errors.Is(err, ErrCorrectionRequired) {
		t.Fatalf("post-announcement capture = %v, want ErrCorrectionRequired", err)
	}

	firstState, err := results.UnitProtestState(ctx, rec, windUnit)
	if err != nil {
		t.Fatal(err)
	}

	corrected, err := results.CorrectResult(ctx, office, rec, windUnit, anna, CorrectionInput{
		Mark: "12.20", Timing: domain.TimingElectronic, Reason: "referee upheld a protest on the timing pad",
	})
	if err != nil {
		t.Fatalf("CorrectResult: %v", err)
	}
	if corrected.Mark != "12.20" {
		t.Errorf("corrected mark = %q, want 12.20", corrected.Mark)
	}

	// UC-015 #2: the amended list gets a new announcement timestamp.
	secondState, err := results.UnitProtestState(ctx, rec, windUnit)
	if err != nil {
		t.Fatal(err)
	}
	if !secondState.AnnouncedAt.After(firstState.AnnouncedAt) {
		t.Errorf("correction must re-announce: second %v, first %v", secondState.AnnouncedAt, firstState.AnnouncedAt)
	}

	// UC-015 #3: the audit trail holds actor, before/after and reason.
	entries, err := store.ListAudit(ctx, results.db, 50)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, e := range entries {
		if e.Action != "result.correct" {
			continue
		}
		found = true
		if e.Actor != office.AccountID {
			t.Errorf("audit actor = %q, want %q", e.Actor, office.AccountID)
		}
		if e.Reason == "" {
			t.Error("correction audit entry must carry the reason")
		}
		if e.Before == "" || e.After == "" {
			t.Error("correction audit entry must carry before/after")
		}
	}
	if !found {
		t.Fatal("no result.correct audit entry found")
	}
}

// TestCorrectionWithoutReasonRejectedSYS046UC015_3 is the denial path: a
// correction carrying no reason is rejected outright — never silently
// applied without an audit reason (SYS-046).
func TestCorrectionWithoutReasonRejectedSYS046UC015_3(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec, windUnit, _, anna := trackUnitFixture(t, meets, results)
	if _, err := results.SaveTrackResult(ctx, fieldOfficial, rec, windUnit, TrackResultInput{
		AthleteID: anna, Time: "12.30", Timing: domain.TimingElectronic,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := results.AnnounceUnitResults(ctx, office, rec, windUnit); err != nil {
		t.Fatal(err)
	}

	if _, err := results.CorrectResult(ctx, office, rec, windUnit, anna, CorrectionInput{
		Mark: "12.20", Timing: domain.TimingElectronic,
	}); !errors.Is(err, ErrCorrectionReasonRequired) {
		t.Fatalf("correction without reason = %v, want ErrCorrectionReasonRequired", err)
	}

	// Field officials cannot correct at all (office/referee actor only, UC-015).
	if _, err := results.CorrectResult(ctx, fieldOfficial, rec, windUnit, anna, CorrectionInput{
		Mark: "12.20", Timing: domain.TimingElectronic, Reason: "typo",
	}); err == nil {
		t.Error("a field official must not be able to correct a result (UC-015 actor is competition office)")
	}
}

// TestCorrectionAfterWindowExpiryRequiresEscalationSYS047UC015_2 proves the
// TASK-019 scope note: correcting an already-official result (protest
// window elapsed) demands an explicit escalation reference in addition to
// the ordinary reason.
func TestCorrectionAfterWindowExpiryRequiresEscalationSYS047UC015_2(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec, windUnit, _, anna := trackUnitFixture(t, meets, results)
	if _, err := results.SaveTrackResult(ctx, fieldOfficial, rec, windUnit, TrackResultInput{
		AthleteID: anna, Time: "12.30", Timing: domain.TimingElectronic,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := results.AnnounceUnitResults(ctx, office, rec, windUnit); err != nil {
		t.Fatal(err)
	}
	backdateAnnouncement(t, results, windUnit, time.Now().UTC().Add(-2*time.Hour))

	state, err := results.UnitProtestState(ctx, rec, windUnit)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Official {
		t.Fatalf("state after backdating 2h = %+v, want official (window elapsed, UC-015 #4)", state)
	}

	if _, err := results.CorrectResult(ctx, office, rec, windUnit, anna, CorrectionInput{
		Mark: "12.20", Timing: domain.TimingElectronic, Reason: "late-discovered timing error",
	}); !errors.Is(err, ErrEscalationRequired) {
		t.Fatalf("correction of an official result without escalation = %v, want ErrEscalationRequired", err)
	}

	corrected, err := results.CorrectResult(ctx, office, rec, windUnit, anna, CorrectionInput{
		Mark: "12.20", Timing: domain.TimingElectronic,
		Reason: "late-discovered timing error", Escalation: "Jury of Appeal decision #3",
	})
	if err != nil {
		t.Fatalf("CorrectResult with escalation: %v", err)
	}
	if corrected.Mark != "12.20" {
		t.Errorf("corrected mark = %q, want 12.20", corrected.Mark)
	}
}

// TestCorrectFieldResultSYS046UC015_2 proves corrections also work for
// horizontal field units at the settled-result granularity (OQ-036).
func TestCorrectFieldResultSYS046UC015_2(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec := createUKCMeet(t, meets)
	unitID := unitOf(t, results, meets, rec.ID, "ZoneLJ")
	anna := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2014, Sex: domain.SexFemale, Bib: "101",
	})
	fieldAttempt(t, results, rec.ID, unitID, FieldAttemptInput{AthleteID: anna.AthleteID, Seq: 1, Kind: domain.AttemptValid, Mark: "3.42"})

	if _, err := results.AnnounceUnitResults(ctx, office, rec.ID, unitID); err != nil {
		t.Fatalf("AnnounceUnitResults: %v", err)
	}
	// Further attempt capture is blocked once announced.
	if _, err := results.SaveFieldAttempt(ctx, fieldOfficial, rec.ID, unitID, FieldAttemptInput{
		AthleteID: anna.AthleteID, Seq: 2, Kind: domain.AttemptFoul,
	}); !errors.Is(err, ErrCorrectionRequired) {
		t.Fatalf("post-announcement field capture = %v, want ErrCorrectionRequired", err)
	}

	corrected, err := results.CorrectResult(ctx, office, rec.ID, unitID, anna.AthleteID, CorrectionInput{
		Mark: "3.55", Reason: "remeasurement found a transcription error",
	})
	if err != nil {
		t.Fatalf("CorrectResult (field): %v", err)
	}
	if corrected.Mark != "3.55" {
		t.Errorf("corrected field mark = %q, want 3.55", corrected.Mark)
	}
}

// TestCorrectResultUnknownAthleteOrUnitSYS046 rounds out the validation
// edge paths: an unregistered athlete and an unseen unit are both rejected.
func TestCorrectResultUnknownAthleteOrUnitSYS046(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec, windUnit, _, _ := trackUnitFixture(t, meets, results)
	if _, err := results.AnnounceUnitResults(ctx, office, rec, windUnit); err != nil {
		t.Fatal(err)
	}
	if _, err := results.CorrectResult(ctx, office, rec, windUnit, "01GHOST", CorrectionInput{
		Mark: "12.00", Timing: domain.TimingElectronic, Reason: "test",
	}); err == nil {
		t.Error("correcting an unregistered athlete must be rejected")
	}
	if _, err := results.CorrectResult(ctx, office, rec, "01NOUNIT", "01GHOST", CorrectionInput{
		Reason: "test",
	}); !errors.Is(err, ErrMeetNotFound) {
		t.Errorf("correcting an unknown unit = %v, want not-found", err)
	}
}

// TestUnitWindAndProtestStateUnknownUnitSYS040SYS047 covers the read-side
// not-found paths: UnitWind and UnitProtestState on a unit that does not
// exist both surface the not-found error rather than a zero value.
func TestUnitWindAndProtestStateUnknownUnitSYS040SYS047(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec, _, _, _ := trackUnitFixture(t, meets, results)

	if _, err := results.UnitWind(ctx, rec, "01NOUNIT"); !errors.Is(err, ErrMeetNotFound) {
		t.Errorf("UnitWind(unknown unit) = %v, want not-found", err)
	}
	if _, err := results.UnitProtestState(ctx, rec, "01NOUNIT"); !errors.Is(err, ErrMeetNotFound) {
		t.Errorf("UnitProtestState(unknown unit) = %v, want not-found", err)
	}
	if _, err := results.AnnounceUnitResults(ctx, office, rec, "01NOUNIT"); !errors.Is(err, ErrMeetNotFound) {
		t.Errorf("AnnounceUnitResults(unknown unit) = %v, want not-found", err)
	}
}

// TestSetUnitWindRejectsFieldFamilySYS040 covers the family guard: a
// wind-relevant horizontal-field unit (long jump) still rejects the
// settled-result-level wind API — field wind stays per-attempt (OQ-036).
func TestSetUnitWindRejectsFieldFamilySYS040(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	meet, err := meets.CreateMeet(ctx, organizer, ucMeetRequest())
	if err != nil {
		t.Fatalf("CreateMeet: %v", err)
	}
	if _, err := meets.AddEvent(ctx, organizer, meet.ID, AddEventRequest{
		DisciplineCode: "LJ", CategoryCodes: []string{"U18 W"},
	}); err != nil {
		t.Fatalf("AddEvent(LJ): %v", err)
	}
	ljUnit := unitOf(t, results, meets, meet.ID, "LJ")

	if err := results.SetUnitWind(ctx, fieldOfficial, meet.ID, ljUnit, 1.0); err == nil {
		t.Error("settled-result wind on a horizontal field unit must be rejected (field wind is per-attempt, OQ-036)")
	}
}

// TestCorrectResultNewAthleteAfterAnnouncementSYS046UC015_2 covers the
// hadExisting=false branch: correcting an athlete never captured before
// announcement still works (a late scratch reinstated after publication),
// audited with an empty "before".
func TestCorrectResultNewAthleteAfterAnnouncementSYS046UC015_2(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec, windUnit, _, anna := trackUnitFixture(t, meets, results)
	if _, err := results.SaveTrackResult(ctx, fieldOfficial, rec, windUnit, TrackResultInput{
		AthleteID: anna, Time: "12.30", Timing: domain.TimingElectronic,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := results.AnnounceUnitResults(ctx, office, rec, windUnit); err != nil {
		t.Fatal(err)
	}

	bea := register(t, results, rec, ParticipantInput{
		FirstName: "Bea", LastName: "Beispiel", BirthYear: 2009, Sex: domain.SexFemale, Bib: "3",
	})
	rec2, err := results.CorrectResult(ctx, office, rec, windUnit, bea.AthleteID, CorrectionInput{
		Mark: "12.50", Timing: domain.TimingElectronic, Reason: "late addition after publication",
	})
	if err != nil {
		t.Fatalf("CorrectResult (never captured before): %v", err)
	}
	if rec2.Mark != "12.50" {
		t.Errorf("mark = %q, want 12.50", rec2.Mark)
	}
}
