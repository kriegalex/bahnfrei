// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

//go:build perf

// Package app_test holds TASK-027's heavy SYS-120/121 performance
// benchmarks, guarded behind the `perf` build tag so `go test ./...` (the
// default CI gate, and the SYS-140 coverage run) never pays their cost —
// see scripts/run-perf-tests.sh for how to run them, and
// docs/delivery/perf-and-recovery-task-027.md for the as-measured results.
//
// This is an EXTERNAL test package (app_test, not app) because it imports
// internal/apptest for its SeedLargeMeet fixture builder, and apptest
// itself imports internal/app — an internal (package app) test file
// importing apptest would be an import cycle. Every MeetService/
// ResultsService method these benchmarks call is already part of app's
// public API (the same one internal/web calls), so nothing here needs
// unexported access.
package app_test

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/kriegalex/bahnfrei/internal/app"
	"github.com/kriegalex/bahnfrei/internal/apptest"
	"github.com/kriegalex/bahnfrei/internal/domain"
)

var (
	perfOrganizer = app.Session{AccountID: "01PERFORGANIZER00000000000", Username: "perf-organizer", Role: app.RoleMeetOrganizer}
	perfOffice    = app.Session{AccountID: "01PERFOFFICE0000000000000", Username: "perf-office", Role: app.RoleCompetitionOffice}
	perfSubmitter = app.Session{AccountID: "01PERFSUBMITTER000000000", Username: "perf-submitter", Role: app.RoleEntrySubmitter}
)

// percentile returns the p-th percentile (0..100) of samples using a
// simple nearest-rank estimator. This is a regression-guard proxy for
// SYS-120/121's p95 budgets, not a claim of statistically rigorous
// production SLO measurement — that needs real traffic telemetry, out of
// scope for a one-process benchmark (see the perf report's caveats).
func percentile(samples []time.Duration, p float64) time.Duration {
	sorted := append([]time.Duration(nil), samples...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	idx := int(float64(len(sorted))*p/100.0 + 0.5)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	if idx < 0 {
		idx = 0
	}
	return sorted[idx]
}

// timeReps runs fn n times and returns each call's wall-clock duration.
func timeReps(tb testing.TB, n int, fn func()) []time.Duration {
	tb.Helper()
	out := make([]time.Duration, n)
	for i := 0; i < n; i++ {
		start := time.Now()
		fn()
		out[i] = time.Since(start)
	}
	return out
}

// sys120CIMultiplier headrooms SYS-120's spec budget for this shared,
// often-noisy/oversubscribed dev/CI hardware, which is not the "commodity
// 4-core CPU, 8GB RAM class" machine SYS-120 names as its reference
// environment. The literal spec budget is still logged and reported
// against separately in docs/delivery/perf-and-recovery-task-027.md.
const sys120CIMultiplier = 4

// assertBudget logs the measured p95 against budget and fails if it
// exceeds budget*sys120CIMultiplier (the CI-headroomed ceiling).
func assertBudget(t *testing.T, name string, samples []time.Duration, budget time.Duration) {
	t.Helper()
	p95 := percentile(samples, 95)
	ciCeiling := budget * sys120CIMultiplier
	t.Logf("%s: p95=%v (n=%d reps), spec budget=%v, CI ceiling=%v", name, p95, len(samples), budget, ciCeiling)
	if p95 > ciCeiling {
		t.Errorf("%s: p95=%v exceeds CI ceiling %v (spec budget %v)", name, p95, ciCeiling, budget)
	}
}

// TestSYS120ReferenceScaleOperatorBudgets builds ONE SYS-120 reference-scale
// meet (1,500 athletes / 4,000 entries / 250 event-units / 3 days) and
// times the operator-visible interactions SYS-120 names against it: list
// loads, entry search, and result save (≤2s p95); plus SYS-121's standings
// recompute-after-correction (≤5s), measured on this same worst-case
// (largest) corpus rather than a small single event.
func TestSYS120ReferenceScaleOperatorBudgets(t *testing.T) {
	fix := apptest.New(t, time.Hour)
	lf := apptest.SeedLargeMeet(t, fix, apptest.SYS120Scale)
	ctx := context.Background()

	t.Run("MeetDetailListLoad", func(t *testing.T) {
		samples := timeReps(t, 30, func() {
			if _, err := fix.Meets.Meet(ctx, lf.MeetID); err != nil {
				t.Fatalf("Meet: %v", err)
			}
		})
		assertBudget(t, "SYS-120 meet-detail list load", samples, 2*time.Second)
	})

	t.Run("ParticipantRosterSearch", func(t *testing.T) {
		// SYS-120 names "entry search" as an operator-visible interaction.
		// No dedicated search/filter endpoint exists anywhere in the repo
		// today (confirmed absent — grep for "Search" across internal/app
		// and internal/web finds nothing): the closest existing operation
		// an operator uses to find an entry among many is the full
		// roster/participants list (the office roster page reads exactly
		// this), so it stands in as the search-proxy benchmark here. If a
		// dedicated filtered search ships later, add its own budget test
		// alongside this one rather than replacing it.
		samples := timeReps(t, 20, func() {
			if _, err := fix.Results.Participants(ctx, lf.MeetID); err != nil {
				t.Fatalf("Participants: %v", err)
			}
		})
		assertBudget(t, "SYS-120 participant roster load (entry-search proxy)", samples, 2*time.Second)
	})

	t.Run("ResultSave", func(t *testing.T) {
		// ResultsService.SaveResult (the DisciplineCode-keyed overload)
		// requires its discipline to map to exactly one unit in the meet —
		// true for a small single-round meet, but this fixture reuses each
		// of the 23 track/field-horizontal codes across ~11 units to reach
		// 250 event-units. SaveTrackResult (the unit-keyed overload the
		// real capture UI calls, capture.go) is what a genuinely large
		// meet's capture flow uses, so it is the more representative
		// "result save" operation to benchmark here.
		unitID := ""
		for i, code := range lf.DisciplineCodes {
			catalog, err := domain.BuiltinDisciplineCatalog()
			if err != nil {
				t.Fatalf("BuiltinDisciplineCatalog: %v", err)
			}
			disc, ok := catalog.ByCode(code)
			if ok && disc.Family == domain.FamilyTrack {
				unitID = lf.UnitIDs[i]
				break
			}
		}
		if unitID == "" || len(lf.AthleteIDs) == 0 {
			t.Fatal("fixture has no track unit/athletes")
		}
		athlete := lf.AthleteIDs[0]
		samples := timeReps(t, 30, func() {
			if _, err := fix.Results.SaveTrackResult(ctx, perfOffice, lf.MeetID, unitID, app.TrackResultInput{
				AthleteID: athlete, Time: "11.11", Timing: domain.TimingElectronic,
			}); err != nil {
				t.Fatalf("SaveTrackResult: %v", err)
			}
		})
		assertBudget(t, "SYS-120 result save", samples, 2*time.Second)
	})

	t.Run("StandingsRecomputeAfterCorrection", func(t *testing.T) {
		unitID := lf.UnitIDs[0]
		athleteID := lf.AthleteIDs[0]
		if _, err := fix.Results.CorrectResult(ctx, perfOffice, lf.MeetID, unitID, athleteID, app.CorrectionInput{
			Mark: "10.99", Timing: domain.TimingElectronic, Reason: "TASK-027 perf drill correction",
		}); err != nil {
			t.Fatalf("CorrectResult: %v", err)
		}
		samples := timeReps(t, 10, func() {
			if _, err := fix.Results.Standings(ctx, lf.MeetID); err != nil {
				t.Fatalf("Standings: %v", err)
			}
		})
		assertBudget(t, "SYS-121 standings recompute after correction", samples, 5*time.Second)
	})
}

// TestSYS121SeedingGenerationBudget covers SYS-121's other half: "seeding
// generation for an event with 200 entries (heats + lanes) SHALL complete
// in ≤10s". Mirrors internal/web's seededMeetFixture pattern (a single
// 100m/U18 W event, qualification + final rounds, confirmed entries via the
// app-layer entry/check-in path) at the spec's literal 200-entry scale.
func TestSYS121SeedingGenerationBudget(t *testing.T) {
	fix := apptest.New(t, time.Hour)
	ctx := context.Background()

	start := time.Now().AddDate(0, 0, 1)
	rec, err := fix.Meets.CreateMeet(ctx, perfOrganizer, app.MeetRequest{
		Name: "TASK-027 seeding perf meet", Venue: "Reference venue",
		StartDate: start, EndDate: start, Tier: "C-Meeting",
		CategorySchemeID: domain.SchemeSwissAthletics,
	})
	if err != nil {
		t.Fatalf("CreateMeet: %v", err)
	}
	if err := fix.Meets.PublishMeet(ctx, perfOrganizer, rec.ID, rec.Version); err != nil {
		t.Fatalf("PublishMeet: %v", err)
	}
	ev, err := fix.Meets.AddEvent(ctx, perfOrganizer, rec.ID, app.AddEventRequest{
		DisciplineCode: "100m", CategoryCodes: []string{"U18 W"},
		Rounds: []domain.RoundKind{domain.RoundQualification, domain.RoundFinal},
	})
	if err != nil {
		t.Fatalf("AddEvent: %v", err)
	}

	const entries = 200
	for i := 0; i < entries; i++ {
		detail, err := fix.Results.SubmitIndividualEntry(ctx, perfSubmitter, rec.ID, app.IndividualEntryInput{
			EventID: ev.ID, FirstName: "Athlete", LastName: fmt.Sprintf("Perf%03d", i),
			BirthYear: 2009, Sex: domain.SexFemale, SeedPerformance: fmt.Sprintf("13.%02d", 99-i%99),
		})
		if err != nil {
			t.Fatalf("SubmitIndividualEntry %d: %v", i, err)
		}
		if err := fix.Results.ConfirmCheckIn(ctx, perfOffice, rec.ID, detail.ID, detail.Version); err != nil {
			t.Fatalf("ConfirmCheckIn %d: %v", i, err)
		}
	}

	md, err := fix.Meets.Meet(ctx, rec.ID)
	if err != nil {
		t.Fatalf("Meet: %v", err)
	}
	var roundID string
	for _, pe := range md.Programme {
		if pe.ID == ev.ID {
			roundID = pe.Rounds[0].ID
		}
	}
	if roundID == "" {
		t.Fatal("qualification round not found")
	}

	genStart := time.Now()
	if _, err := fix.Results.GenerateHeats(ctx, perfOffice, rec.ID, ev.ID, roundID, app.GenerateHeatsRequest{
		MaxHeatSize: 8, TrackLanes: 8,
	}); err != nil {
		t.Fatalf("GenerateHeats: %v", err)
	}
	elapsed := time.Since(genStart)
	ciCeiling := 10 * time.Second * sys120CIMultiplier
	t.Logf("SYS-121 seeding generation (200 entries): %v (spec budget 10s, CI ceiling %v)", elapsed, ciCeiling)
	if elapsed > ciCeiling {
		t.Errorf("seeding generation took %v, want <= CI ceiling %v (spec budget 10s)", elapsed, ciCeiling)
	}
}
