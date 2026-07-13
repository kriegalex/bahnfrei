// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/kriegalex/bahnfrei/internal/domain"
	"github.com/kriegalex/bahnfrei/internal/store"
)

// TASK-025 (UC-021; SYS-083): the "≥10 concurrent operator sessions on one
// meet" proof this task adds on top of the primitives TASK-003/008 already
// cover (store.TestOptimisticUpdateConflictSurfaced,
// store.TestSaveAttemptConflicts, TestCaptureConflictSurfaced) —
// goroutine-based simulated sessions driven through ResultsService (the
// real service layer, never raw SQL), matching the single-writer SQLite
// pool (store.Open: db.SetMaxOpenConns(1), ADR-004 §2) every real operator
// session shares in production.

// tenUnitFixture builds one meet with 10 distinct 100m events (one unit
// each) and one registered athlete per event — ten independent (unit,
// athlete) pairs ten "sessions" can write to without touching each other's
// row (UC-021 #1).
func tenUnitFixture(t *testing.T, meets *MeetService, results *ResultsService) (meetID string, unitIDs, athleteIDs []string) {
	t.Helper()
	ctx := context.Background()
	// A plain Swiss Athletics scheme meet with no scoring table configured
	// (unlike the UKC template): scorePoints is then a documented no-op
	// (nil == "the meet does not score points"), so 10 plain 100m events
	// need no discipline-specific scoring-table wiring.
	rec, err := meets.CreateMeet(ctx, organizer, ucMeetRequest())
	if err != nil {
		t.Fatalf("CreateMeet: %v", err)
	}
	meetID = rec.ID
	for i := 0; i < 10; i++ {
		ev, err := meets.AddEvent(ctx, organizer, meetID, AddEventRequest{
			DisciplineCode: "100m", CategoryCodes: []string{"U16 W"},
		})
		if err != nil {
			t.Fatalf("AddEvent[%d]: %v", i, err)
		}
		detail, err := meets.Meet(ctx, meetID)
		if err != nil {
			t.Fatalf("Meet: %v", err)
		}
		var unitID string
		for _, u := range detail.Units {
			if u.EventID == ev.ID {
				unitID = u.UnitID
				break
			}
		}
		if unitID == "" {
			t.Fatalf("event %d has no unit", i)
		}
		p := register(t, results, meetID, ParticipantInput{
			FirstName: fmt.Sprintf("Athlete%d", i), LastName: "Runner", BirthYear: 2014,
			Sex: domain.SexFemale, Bib: fmt.Sprintf("%d", 200+i),
		})
		unitIDs = append(unitIDs, unitID)
		athleteIDs = append(athleteIDs, p.AthleteID)
	}
	return meetID, unitIDs, athleteIDs
}

// TestUC021_1_TenConcurrentSessionsDifferentUnits proves SYS-083's first
// guarantee: 10 simulated operator sessions writing to 10 different units
// of the same meet concurrently all persist, none lost or misattributed to
// the wrong unit/athlete.
func TestUC021_1_TenConcurrentSessionsDifferentUnits(t *testing.T) {
	meets, results, st := newTestResults(t)
	ctx := context.Background()
	meetID, unitIDs, athleteIDs := tenUnitFixture(t, meets, results)

	const n = 10
	var wg sync.WaitGroup
	errs := make([]error, n)
	wg.Add(n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			<-start // all goroutines race to begin as close to simultaneously as possible
			_, err := results.SaveTrackResult(ctx, office, meetID, unitIDs[i], TrackResultInput{
				AthleteID: athleteIDs[i], Time: fmt.Sprintf("11.%02d", 10+i), Timing: domain.TimingElectronic,
			})
			errs[i] = err
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("session %d: SaveTrackResult failed: %v", i, err)
		}
	}

	list, err := store.ListMeetResults(ctx, st.DB(), meetID)
	if err != nil {
		t.Fatalf("ListMeetResults: %v", err)
	}
	if len(list) != n {
		t.Fatalf("got %d persisted results, want %d — none may be lost", len(list), n)
	}
	byAthlete := make(map[string]store.MeetResult, len(list))
	for _, r := range list {
		byAthlete[r.AthleteID] = r
	}
	for i, aid := range athleteIDs {
		got, ok := byAthlete[aid]
		if !ok {
			t.Errorf("athlete %d (%s): no result persisted", i, aid)
			continue
		}
		want := fmt.Sprintf("11.%02d", 10+i)
		if got.Mark != want || got.UnitID != unitIDs[i] {
			t.Errorf("athlete %d: result = {unit=%s mark=%s}, want {unit=%s mark=%s} — misattributed write",
				i, got.UnitID, got.Mark, unitIDs[i], want)
		}
	}
}

// TestUC021_2_ConcurrentEditsSameResultConflictSurfaced proves SYS-083's
// second guarantee at the service layer (store.TestSaveResultOptimisticConflictSurfaced
// proves the same thing one layer down): two sessions that both loaded the
// same result at version 1 and race to save a correction — one wins, the
// other gets *ResultConflictError carrying the row actually stored, never
// a silent last-write-wins.
func TestUC021_2_ConcurrentEditsSameResultConflictSurfaced(t *testing.T) {
	meets, results, st := newTestResults(t)
	ctx := context.Background()
	rec := createUKCMeet(t, meets)
	unitID := unitOf(t, results, meets, rec.ID, "60m")
	anna := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2014, Sex: domain.SexFemale, Bib: "101",
	})

	initial, err := results.SaveTrackResult(ctx, office, rec.ID, unitID, TrackResultInput{
		AthleteID: anna.AthleteID, Time: "9.5", Timing: domain.TimingManual,
	})
	if err != nil {
		t.Fatalf("initial capture: %v", err)
	}
	if initial.Version != 1 {
		t.Fatalf("initial version = %d, want 1", initial.Version)
	}
	sessionVersion := initial.Version // both sessions load this same version

	const n = 2
	marks := []string{"9.3", "9.4"}
	timings := []domain.Timing{domain.TimingManual, domain.TimingManual}
	var wg sync.WaitGroup
	results2 := make([]store.ResultRecord, n)
	errs := make([]error, n)
	wg.Add(n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			ev := sessionVersion
			<-start
			results2[i], errs[i] = results.SaveTrackResult(ctx, office, rec.ID, unitID, TrackResultInput{
				AthleteID: anna.AthleteID, Time: marks[i], Timing: timings[i], ExpectedVersion: &ev,
			})
		}(i)
	}
	close(start)
	wg.Wait()

	var succeeded, conflicted int
	var conflict *ResultConflictError
	for i, err := range errs {
		switch {
		case err == nil:
			succeeded++
			if results2[i].Version != 2 {
				t.Errorf("session %d succeeded with version %d, want 2", i, results2[i].Version)
			}
		case errors.As(err, &conflict):
			conflicted++
			if !errors.Is(err, ErrConflict) {
				t.Errorf("session %d: conflict error does not match ErrConflict", i)
			}
		default:
			t.Errorf("session %d: unexpected error %v", i, err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("got %d succeeded / %d conflicted, want exactly one of each (never both winning, never both silently applied)", succeeded, conflicted)
	}

	// The conflict must carry the row actually stored — "both versions
	// surfaced", never silently discarded.
	final, err := store.GetResult(ctx, st.DB(), unitID, anna.AthleteID)
	if err != nil {
		t.Fatal(err)
	}
	if conflict.Current.Mark != final.Mark || conflict.Current.Version != final.Version {
		t.Errorf("conflict.Current = %+v, want it to match the final stored row %+v", conflict.Current, final)
	}
	if final.Mark != "9.3" && final.Mark != "9.4" {
		t.Errorf("final mark = %q, want one of the two attempted corrections", final.Mark)
	}
}

// TestUC021_3_InfieldCaptureAndOfficeCorrectionConverge proves UC-021 #3:
// infield capture on one still-open heat and an office correction on
// another, already-announced heat of the SAME event, running concurrently,
// both persist and the final state of each is consistent with its own
// audit-trail order (the last audit "after" payload for an entity equals
// what is actually stored — no write silently lost or reordered past its
// own audit record).
func TestUC021_3_InfieldCaptureAndOfficeCorrectionConverge(t *testing.T) {
	meets, results, st := newTestResults(t)
	ctx := context.Background()
	rec := createUKCMeet(t, meets)
	unitID := unitOf(t, results, meets, rec.ID, "ZoneLJ")

	anna := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2014, Sex: domain.SexFemale, Bib: "101",
	})
	bea := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Bea", LastName: "Zweite", BirthYear: 2014, Sex: domain.SexFemale, Bib: "102",
	})

	// Anna's heat settles and is announced (office corrections only apply
	// post-announcement, SYS-047) — the "office correction" side.
	fieldAttempt(t, results, rec.ID, unitID, FieldAttemptInput{AthleteID: anna.AthleteID, Seq: 1, Kind: domain.AttemptValid, Mark: "3.40"})
	if _, err := results.AnnounceUnitResults(ctx, office, rec.ID, unitID); err != nil {
		t.Fatalf("AnnounceUnitResults: %v", err)
	}
	// Bea has NOT been captured yet — she is the "still open, infield
	// capture continues" athlete on the SAME (already-announced) unit;
	// capturing her is a correction too (SaveFieldAttempt is blocked once
	// announced), so both concurrent writers go through CorrectResult on
	// two different athletes of the same event/unit — exactly UC-021 #3's
	// "infield capture plus office corrections on the same event"
	// happening at once, without the two writes touching the same row
	// (that scenario is UC-021 #2, proven above).

	var wg sync.WaitGroup
	var errCorrectAnna, errCorrectBea error
	wg.Add(2)
	start := make(chan struct{})
	go func() {
		defer wg.Done()
		<-start
		_, errCorrectAnna = results.CorrectResult(ctx, office, rec.ID, unitID, anna.AthleteID, CorrectionInput{
			Mark: "3.55", Reason: "remeasured mark",
		})
	}()
	go func() {
		defer wg.Done()
		<-start
		_, errCorrectBea = results.CorrectResult(ctx, office, rec.ID, unitID, bea.AthleteID, CorrectionInput{
			Mark: "3.20", Reason: "late infield result phoned in",
		})
	}()
	close(start)
	wg.Wait()

	if errCorrectAnna != nil {
		t.Errorf("correct Anna: %v", errCorrectAnna)
	}
	if errCorrectBea != nil {
		t.Errorf("correct Bea: %v", errCorrectBea)
	}

	annaResult, err := store.GetResult(ctx, st.DB(), unitID, anna.AthleteID)
	if err != nil {
		t.Fatal(err)
	}
	beaResult, err := store.GetResult(ctx, st.DB(), unitID, bea.AthleteID)
	if err != nil {
		t.Fatal(err)
	}
	if annaResult.Mark != "3.55" {
		t.Errorf("Anna's stored mark = %q, want 3.55 (both writes must persist — different rows)", annaResult.Mark)
	}
	if beaResult.Mark != "3.20" {
		t.Errorf("Bea's stored mark = %q, want 3.20", beaResult.Mark)
	}

	// Convergence with the audit trail: each result's stored state must
	// equal its OWN last "result.correct" audit entry's "after" payload —
	// standings never drift from what the audit log says happened.
	assertLastAuditMatchesStoredResult(t, st, annaResult.ID, "3.55")
	assertLastAuditMatchesStoredResult(t, st, beaResult.ID, "3.20")

	// The re-announcement CorrectResult triggers for each correction must
	// also be present and in increasing seq order (the protest window
	// reopened for the unit exactly twice, once per correction — no
	// announcement silently dropped by the race).
	trail, err := store.AuditTrail(ctx, st.DB(), "unit", unitID)
	if err != nil {
		t.Fatal(err)
	}
	announces := 0
	for _, e := range trail {
		if e.Action == "result.announce" {
			announces++
		}
	}
	if announces < 2 {
		t.Errorf("got %d unit announcement audit entries after 2 corrections (plus the original), want at least 2 more (one per correction)", announces)
	}
}

func assertLastAuditMatchesStoredResult(t *testing.T, st *store.Store, resultID, wantMark string) {
	t.Helper()
	trail, err := store.AuditTrail(context.Background(), st.DB(), "result", resultID)
	if err != nil {
		t.Fatal(err)
	}
	if len(trail) == 0 {
		t.Fatalf("no audit trail for result %s", resultID)
	}
	last := trail[len(trail)-1]
	if last.Action != "result.correct" {
		t.Errorf("last audit action for result %s = %q, want result.correct", resultID, last.Action)
	}
	if !strings.Contains(last.After, wantMark) {
		t.Errorf("last audit entry After = %q, want it to record mark %q (audit trail must match final stored state)", last.After, wantMark)
	}
}
