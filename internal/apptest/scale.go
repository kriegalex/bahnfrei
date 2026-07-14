// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package apptest

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/kriegalex/bahnfrei/internal/domain"
	"github.com/kriegalex/bahnfrei/internal/store"
)

// ScaleParams sizes a synthetic large-meet fixture (TASK-027, SYS-120/121/
// 122's reference-scale corpus).
type ScaleParams struct {
	Athletes int
	Entries  int
	Units    int // event-units: this builder creates one round + one unit per event, so Units == event count
	Days     int
}

// SYS120Scale is the SYS-120 reference-dataset floor: "a meet of at least
// 1,500 athletes, 4,000 entries, 250 event-units, 3 competition days".
var SYS120Scale = ScaleParams{Athletes: 1500, Entries: 4000, Units: 250, Days: 3}

// LargeMeetFixture is what SeedLargeMeet built.
type LargeMeetFixture struct {
	MeetID   string
	EventIDs []string // one per unit, parallel to UnitIDs/DisciplineCodes
	UnitIDs  []string
	// DisciplineCodes[i] is EventIDs[i]/UnitIDs[i]'s discipline code, so
	// callers can drive ResultsService.SaveResult (which takes a
	// DisciplineCode, not a UnitID) against a known-good event.
	DisciplineCodes []string
	AthleteIDs      []string
}

// scaleBatch bounds how many rows share one transaction: large enough to
// amortize the WAL/fsync-per-commit cost that dominates at this scale,
// small enough that one failure doesn't force redoing the entire fixture.
const scaleBatch = 400

// SeedLargeMeet builds a ScaleParams-sized meet DIRECTLY against the store,
// bypassing the app-service layer's entry-submission path
// (ResultsService.SubmitIndividualEntry always mints a brand-new athlete
// per call and commits once per row — it cannot express "4,000 entries
// across 1,500 athletes" and would need thousands of individual
// transactions, taking far too long for a benchmark/load-test fixture).
// This is fixture SETUP, not the operation under test: TASK-027's
// benchmarks and load tests measure read/save performance against this
// corpus, not entry-creation performance itself (UC-003's own tests already
// cover that path). f must come from New/NewAtPath (its Store field is
// required).
//
// Every event gets exactly one round (a "final") and one unit, so
// event-unit count == p.Units. Events alternate the Swiss Athletics scheme's
// "Men"/"Women" open categories (STR-adult, no birth-year upper bound) and
// cycle through the catalog's track/field-horizontal disciplines only —
// vertical-jump/relay/combined events are excluded here because
// ResultsService.CorrectResult (exercised by the SYS-121 recompute
// benchmark) only supports track and field-horizontal families. Athletes
// alternate sex to match; every athlete is birth-year 1990 so category
// resolution is uniform regardless of which event an entry targets.
func SeedLargeMeet(tb testing.TB, f Fixture, p ScaleParams) LargeMeetFixture {
	tb.Helper()
	if f.Store == nil {
		tb.Fatal("apptest.SeedLargeMeet: f.Store is nil — build the fixture with apptest.New or apptest.NewAtPath")
	}
	ctx := context.Background()
	db := f.Store.DB()

	meetID := seedLargeMeetRecord(tb, ctx, db, p)
	eventIDs, unitIDs, disciplineCodes, disciplineUnit := seedLargeEvents(tb, ctx, db, meetID, p)
	athleteIDs, sexOf := seedLargeAthletes(tb, ctx, db, meetID, p)
	seedLargeEntriesAndResults(tb, ctx, db, eventIDs, unitIDs, disciplineUnit, athleteIDs, sexOf, p)

	return LargeMeetFixture{
		MeetID: meetID, EventIDs: eventIDs, UnitIDs: unitIDs,
		DisciplineCodes: disciplineCodes, AthleteIDs: athleteIDs,
	}
}

func seedLargeMeetRecord(tb testing.TB, ctx context.Context, db *sql.DB, p ScaleParams) string {
	tb.Helper()
	start := time.Now().AddDate(0, 0, 1).Truncate(24 * time.Hour)
	rec, err := store.CreateMeet(ctx, db, domain.Meet{
		Name: "TASK-027 reference-scale meet", Venue: "Reference venue",
		StartDate: start, EndDate: start.AddDate(0, 0, max(p.Days-1, 0)),
		Organizer: "TASK-027", Tier: "C-Meeting",
		CategorySchemeID: domain.SchemeSwissAthletics,
		// Published so the public surfaces (SYS-122 load test) render it.
		Status: domain.MeetPublished,
	})
	if err != nil {
		tb.Fatalf("apptest.SeedLargeMeet: create meet: %v", err)
	}
	return rec.ID
}

// trackAndFieldHorizontalCodes returns the catalog's track/field-horizontal
// discipline codes — the two families ResultsService.CorrectResult
// supports (protest.go), and simple enough to fabricate a plausible Mark
// string for (time or distance).
func trackAndFieldHorizontalCodes(tb testing.TB) []domain.Discipline {
	tb.Helper()
	catalog, err := domain.BuiltinDisciplineCatalog()
	if err != nil {
		tb.Fatalf("apptest.SeedLargeMeet: load discipline catalog: %v", err)
	}
	var out []domain.Discipline
	for _, d := range catalog.Disciplines {
		if d.Family == domain.FamilyTrack || d.Family == domain.FamilyFieldHorizontal {
			out = append(out, d)
		}
	}
	if len(out) == 0 {
		tb.Fatal("apptest.SeedLargeMeet: discipline catalog has no track/field-horizontal disciplines")
	}
	return out
}

// seedLargeEvents creates p.Units events (one round + one unit each),
// alternating "Men"/"Women" categories, spread evenly across p.Days.
// Returns the events' IDs, their units' IDs (parallel), each unit's
// discipline code, and each unit's discipline unit-of-measure (for
// fabricating a plausible Mark later).
func seedLargeEvents(tb testing.TB, ctx context.Context, db *sql.DB, meetID string, p ScaleParams) (eventIDs, unitIDs, disciplineCodes []string, disciplineUnit []domain.DisciplineUnit) {
	tb.Helper()
	disciplines := trackAndFieldHorizontalCodes(tb)
	start := time.Now().AddDate(0, 0, 1).Truncate(24 * time.Hour)

	eventIDs = make([]string, 0, p.Units)
	unitIDs = make([]string, 0, p.Units)
	disciplineCodes = make([]string, 0, p.Units)
	disciplineUnit = make([]domain.DisciplineUnit, 0, p.Units)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		tb.Fatalf("apptest.SeedLargeMeet: begin events tx: %v", err)
	}
	commit := func() {
		if err := tx.Commit(); err != nil {
			tb.Fatalf("apptest.SeedLargeMeet: commit events tx: %v", err)
		}
	}
	for i := 0; i < p.Units; i++ {
		if i > 0 && i%scaleBatch == 0 {
			commit()
			tx, err = db.BeginTx(ctx, nil)
			if err != nil {
				tb.Fatalf("apptest.SeedLargeMeet: begin events tx: %v", err)
			}
		}
		disc := disciplines[i%len(disciplines)]
		category := "Men"
		if i%2 == 1 {
			category = "Women"
		}
		day := start.AddDate(0, 0, i%max(p.Days, 1)).Add(9 * time.Hour)

		ev, err := store.CreateEvent(ctx, tx, domain.Event{
			MeetID: meetID, DisciplineCode: disc.Code,
			CategoryCodes: []string{category}, Status: domain.EventPublished,
		})
		if err != nil {
			tb.Fatalf("apptest.SeedLargeMeet: create event %d: %v", i, err)
		}
		round, err := store.CreateRound(ctx, tx, domain.Round{EventID: ev.ID, Kind: domain.RoundFinal}, 0)
		if err != nil {
			tb.Fatalf("apptest.SeedLargeMeet: create round %d: %v", i, err)
		}
		unit, err := store.CreateUnit(ctx, tx, domain.Unit{
			RoundID: round.ID, ScheduledAt: day, Location: fmt.Sprintf("Venue %d", i%8+1),
		})
		if err != nil {
			tb.Fatalf("apptest.SeedLargeMeet: create unit %d: %v", i, err)
		}
		eventIDs = append(eventIDs, ev.ID)
		unitIDs = append(unitIDs, unit.ID)
		disciplineCodes = append(disciplineCodes, disc.Code)
		disciplineUnit = append(disciplineUnit, disc.Unit)
	}
	commit()
	return eventIDs, unitIDs, disciplineCodes, disciplineUnit
}

// seedLargeAthletes creates p.Athletes athletes (split across a small pool
// of clubs, alternating sex) plus their meet participant rows. Returns the
// athlete IDs and each one's sex (so entries can be matched to a
// same-sex-category event).
func seedLargeAthletes(tb testing.TB, ctx context.Context, db *sql.DB, meetID string, p ScaleParams) (athleteIDs []string, sexOf map[string]domain.Sex) {
	tb.Helper()
	const clubCount = 24
	clubIDs := make([]string, 0, clubCount)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		tb.Fatalf("apptest.SeedLargeMeet: begin clubs tx: %v", err)
	}
	for i := 0; i < clubCount; i++ {
		club, err := store.CreateClub(ctx, tx, domain.Club{Name: fmt.Sprintf("TASK-027 Club %02d", i)})
		if err != nil {
			tb.Fatalf("apptest.SeedLargeMeet: create club %d: %v", i, err)
		}
		clubIDs = append(clubIDs, club.ID)
	}
	if err := tx.Commit(); err != nil {
		tb.Fatalf("apptest.SeedLargeMeet: commit clubs tx: %v", err)
	}

	athleteIDs = make([]string, 0, p.Athletes)
	sexOf = make(map[string]domain.Sex, p.Athletes)

	tx, err = db.BeginTx(ctx, nil)
	if err != nil {
		tb.Fatalf("apptest.SeedLargeMeet: begin athletes tx: %v", err)
	}
	commit := func() {
		if err := tx.Commit(); err != nil {
			tb.Fatalf("apptest.SeedLargeMeet: commit athletes tx: %v", err)
		}
	}
	for i := 0; i < p.Athletes; i++ {
		if i > 0 && i%scaleBatch == 0 {
			commit()
			tx, err = db.BeginTx(ctx, nil)
			if err != nil {
				tb.Fatalf("apptest.SeedLargeMeet: begin athletes tx: %v", err)
			}
		}
		sex := domain.SexMale
		if i%2 == 1 {
			sex = domain.SexFemale
		}
		athlete, err := store.CreateAthlete(ctx, tx, domain.Athlete{
			FirstName: fmt.Sprintf("Athlete%04d", i), LastName: "Fixture",
			BirthYear: 1990, Sex: sex, ClubIDs: []string{clubIDs[i%clubCount]},
		})
		if err != nil {
			tb.Fatalf("apptest.SeedLargeMeet: create athlete %d: %v", i, err)
		}
		if _, err := store.EnsureParticipant(ctx, tx, meetID, athlete.ID); err != nil {
			tb.Fatalf("apptest.SeedLargeMeet: ensure participant %d: %v", i, err)
		}
		athleteIDs = append(athleteIDs, athlete.ID)
		sexOf[athlete.ID] = sex
	}
	commit()
	return athleteIDs, sexOf
}

// syntheticMark fabricates a plausible-looking Mark string for unit u,
// varied by seed so results are not all identical.
func syntheticMark(u domain.DisciplineUnit, seed int) string {
	switch u {
	case domain.UnitDistance, domain.UnitHeight:
		return fmt.Sprintf("%d.%02d", 1+seed%20, seed%99)
	default: // domain.UnitTime and anything else: a plausible sub-30s time
		return fmt.Sprintf("%d.%02d", 9+seed%20, seed%99)
	}
}

// seedLargeEntriesAndResults creates p.Entries entries, each matching an
// athlete to a same-sex-category event/unit (round-robin over both), and
// settles a result for roughly two-thirds of them — enough for
// ResultsService.Standings (TASK-027's SYS-121 recompute target) to rank a
// realistic, non-empty corpus.
func seedLargeEntriesAndResults(tb testing.TB, ctx context.Context, db *sql.DB, eventIDs, unitIDs []string, disciplineUnit []domain.DisciplineUnit, athleteIDs []string, sexOf map[string]domain.Sex, p ScaleParams) {
	tb.Helper()
	var menIdx, womenIdx []int
	for i := range eventIDs {
		if i%2 == 0 {
			menIdx = append(menIdx, i)
		} else {
			womenIdx = append(womenIdx, i)
		}
	}
	if len(menIdx) == 0 || len(womenIdx) == 0 {
		tb.Fatal("apptest.SeedLargeMeet: need at least one event per sex category")
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		tb.Fatalf("apptest.SeedLargeMeet: begin entries tx: %v", err)
	}
	commit := func() {
		if err := tx.Commit(); err != nil {
			tb.Fatalf("apptest.SeedLargeMeet: commit entries tx: %v", err)
		}
	}
	for i := 0; i < p.Entries; i++ {
		if i > 0 && i%scaleBatch == 0 {
			commit()
			tx, err = db.BeginTx(ctx, nil)
			if err != nil {
				tb.Fatalf("apptest.SeedLargeMeet: begin entries tx: %v", err)
			}
		}
		athleteIdx := i % len(athleteIDs)
		athleteID := athleteIDs[athleteIdx]
		pool := menIdx
		if sexOf[athleteID] == domain.SexFemale {
			pool = womenIdx
		}
		// Entries has a UNIQUE(event_id, athlete_id) constraint, and
		// p.Entries typically exceeds p.Athletes (each athlete enters
		// several events): repeat cycle k = i/len(athleteIDs) shifts which
		// pool slot this athlete lands on each time it repeats, so the
		// same athlete never targets the same event twice as long as
		// ceil(p.Entries/p.Athletes) <= len(pool).
		repeat := i / len(athleteIDs)
		idx := pool[(athleteIdx+repeat)%len(pool)]

		entry, err := store.CreateEntry(ctx, tx, domain.Entry{
			EventID: eventIDs[idx], AthleteID: athleteID,
			SeedPerformance: syntheticMark(disciplineUnit[idx], i),
			Status:          domain.EntryConfirmed, Source: domain.EntrySourceOnline,
		})
		if err != nil {
			tb.Fatalf("apptest.SeedLargeMeet: create entry %d: %v", i, err)
		}
		if i%3 != 2 { // ~2/3 of entries get a settled result
			_, err := store.SaveResult(ctx, tx, domain.Result{
				UnitID: unitIDs[idx], AthleteID: athleteID,
				Mark: syntheticMark(disciplineUnit[idx], i), Status: domain.StatusNone,
			}, domain.TimingElectronic)
			if err != nil {
				tb.Fatalf("apptest.SeedLargeMeet: save result for entry %s: %v", entry.ID, err)
			}
		}
	}
	commit()
}
