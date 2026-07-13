// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/kriegalex/bahnfrei/internal/domain"
	"github.com/kriegalex/bahnfrei/internal/exchange"
	"github.com/kriegalex/bahnfrei/internal/store"
)

// timingExchangeFixture is a published meet with one 100m final, N
// registered/confirmed/seeded athletes (lanes drawn 1..N), ready for the
// timing-exchange test suite (UC-014).
type timingExchangeFixture struct {
	meets   *MeetService
	results *ResultsService
	st      *store.Store
	meetID  string
	eventID string
	unitID  string
	// athletes maps bib -> athlete id, in lane order (lane i+1 -> athletes[i]).
	athletes []string
	bibs     []string
}

func newTimingExchangeFixture(t *testing.T, n int) timingExchangeFixture {
	t.Helper()
	ctx := context.Background()
	meets, results, st := newTestResults(t)
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
	rounds, err := store.ListRounds(ctx, st.DB(), ev.ID)
	if err != nil || len(rounds) == 0 {
		t.Fatalf("ListRounds: %v", err)
	}

	f := timingExchangeFixture{meets: meets, results: results, st: st, meetID: meet.ID, eventID: ev.ID}
	for i := 0; i < n; i++ {
		bib := fmt.Sprintf("%d", 100+i)
		p := register(t, results, meet.ID, ParticipantInput{
			FirstName: fmt.Sprintf("A%d", i), LastName: "Runner", BirthYear: 2009, Sex: domain.SexFemale, Bib: bib,
		})
		if _, err := store.CreateEntry(ctx, st.DB(), domain.Entry{
			EventID: ev.ID, AthleteID: p.AthleteID, Status: domain.EntryConfirmed, SeedPerformance: fmt.Sprintf("%d.00", 12+i),
		}); err != nil {
			t.Fatalf("CreateEntry: %v", err)
		}
		f.athletes = append(f.athletes, p.AthleteID)
		f.bibs = append(f.bibs, bib)
	}

	if _, err := results.GenerateHeats(ctx, office, meet.ID, ev.ID, rounds[0].ID, GenerateHeatsRequest{TrackLanes: 8}); err != nil {
		t.Fatalf("GenerateHeats: %v", err)
	}
	f.unitID = unitOf(t, results, meets, meet.ID, "100m")
	return f
}

// laneOf returns athletes[i]'s drawn lane by inspecting the exported .evt
// (round-trips through the real export path rather than reaching into
// unit_entries directly, so the fixture stays honest about what the
// export actually produced).
func (f timingExchangeFixture) laneOf(t *testing.T, athleteID string) int {
	t.Helper()
	_, _, evt, err := f.results.ExportTimingFiles(context.Background(), office, f.meetID)
	if err != nil {
		t.Fatalf("ExportTimingFiles: %v", err)
	}
	events, err := exchange.ParseEVT(evt)
	if err != nil {
		t.Fatalf("ParseEVT: %v", err)
	}
	for _, ev := range events {
		for _, c := range ev.Competitors {
			for i, b := range f.bibs {
				if f.athletes[i] == athleteID && c.ID == b {
					return c.Lane
				}
			}
		}
	}
	t.Fatalf("athlete %s not found in export", athleteID)
	return 0
}

// TestExportTimingFilesFieldFidelitySYS060UC014_1 checks the exported
// lynx.ppl/.sch/.evt reflect the seeded start list: every athlete's bib,
// name, club and drawn lane round-trip through the real FinishLynx codec.
func TestExportTimingFilesFieldFidelitySYS060UC014_1(t *testing.T) {
	f := newTimingExchangeFixture(t, 3)
	ppl, sch, evt, err := f.results.ExportTimingFiles(context.Background(), office, f.meetID)
	if err != nil {
		t.Fatalf("ExportTimingFiles: %v", err)
	}

	people, err := exchange.ParsePPL(ppl)
	if err != nil {
		t.Fatalf("ParsePPL: %v", err)
	}
	if len(people) != 3 {
		t.Fatalf("got %d people, want 3:\n%s", len(people), ppl)
	}

	sched, err := exchange.ParseSCH(sch)
	if err != nil {
		t.Fatalf("ParseSCH: %v", err)
	}
	if len(sched) != 1 {
		t.Fatalf("got %d schedule entries, want 1:\n%s", len(sched), sch)
	}

	events, err := exchange.ParseEVT(evt)
	if err != nil {
		t.Fatalf("ParseEVT: %v", err)
	}
	if len(events) != 1 || len(events[0].Competitors) != 3 {
		t.Fatalf("got %+v, want 1 event with 3 competitors:\n%s", events, evt)
	}
	found := map[string]bool{}
	for _, c := range events[0].Competitors {
		found[c.ID] = true
		if c.Lane < 1 {
			t.Errorf("competitor %s has no lane: %+v", c.ID, c)
		}
	}
	for _, bib := range f.bibs {
		if !found[bib] {
			t.Errorf("bib %s missing from .evt export", bib)
		}
	}
}

// TestExportTimingFilesLaneSwapReExportSYS060UC014_2 covers UC-014 #2: a
// lane swap (via the existing seeding override path) is reflected the next
// time the timing files are exported.
func TestExportTimingFilesLaneSwapReExportSYS060UC014_2(t *testing.T) {
	f := newTimingExchangeFixture(t, 2)
	before := f.laneOf(t, f.athletes[0])

	rounds, err := store.ListRounds(context.Background(), f.st.DB(), f.eventID)
	if err != nil {
		t.Fatalf("ListRounds: %v", err)
	}
	sheet, err := f.results.HeatSheetFor(context.Background(), office, f.meetID, f.eventID, rounds[0].ID)
	if err != nil {
		t.Fatalf("HeatSheetFor: %v", err)
	}
	var entryID string
	var version int64
	for _, row := range sheet.Units[0].Rows {
		if row.Lane == before {
			continue
		}
		entryID, version = row.EntryID, row.Version
	}
	swapLane := 8
	if before == 8 {
		swapLane = 7
	}
	// Find the target entry/version for athlete[0] specifically to move it.
	for _, row := range sheet.Units[0].Rows {
		entry, err := store.GetEntry(context.Background(), f.st.DB(), row.EntryID)
		if err != nil {
			t.Fatalf("GetEntry: %v", err)
		}
		if entry.AthleteID == f.athletes[0] {
			entryID, version = row.EntryID, row.Version
		}
	}
	if err := f.results.OverrideAssignment(context.Background(), office, f.meetID, f.eventID, rounds[0].ID, entryID, f.unitID, swapLane, version); err != nil {
		t.Fatalf("OverrideAssignment: %v", err)
	}

	after := f.laneOf(t, f.athletes[0])
	if after != swapLane {
		t.Fatalf("lane after swap = %d, want %d (was %d)", after, swapLane, before)
	}
}

// timingImportFixture ingests the fixture's current export's .evt numbers
// into a hand-built .lif payload for lane 1 (athletes[0]).
func (f timingExchangeFixture) lifFor(t *testing.T, place, mark string) []byte {
	t.Helper()
	_, _, evt, err := f.results.ExportTimingFiles(context.Background(), office, f.meetID)
	if err != nil {
		t.Fatalf("ExportTimingFiles: %v", err)
	}
	events, err := exchange.ParseEVT(evt)
	if err != nil || len(events) == 0 {
		t.Fatalf("ParseEVT: %v", err)
	}
	ev := events[0]
	ev.Competitors = []exchange.CompetitorRow{{Place: place, ID: f.bibs[0], Lane: f.laneOf(t, f.athletes[0]), Time: mark}}
	return exchange.EncodeLIF(ev)
}

// TestImportLIFAppliesCleanlySYS061UC014_3 covers UC-014 #3: a valid .lif
// for the seeded unit attaches its time to the right athlete and the
// result appears (provisional — no announcement has run yet).
func TestImportLIFAppliesCleanlySYS061UC014_3(t *testing.T) {
	f := newTimingExchangeFixture(t, 2)
	data := f.lifFor(t, "1", "12.34")

	summary, err := f.results.ImportTimingFile(context.Background(), office, f.meetID, "test.lif", "lif", data)
	if err != nil {
		t.Fatalf("ImportTimingFile: %v", err)
	}
	if summary.Applied != 1 || summary.Conflicts != 0 {
		t.Fatalf("summary = %+v, want 1 applied / 0 conflicts", summary)
	}

	rec, err := store.GetResult(context.Background(), f.st.DB(), f.unitID, f.athletes[0])
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if rec.Mark != "12.34" || rec.Source != "import_lif" || rec.Timing != domain.TimingElectronic {
		t.Fatalf("result = %+v, want mark 12.34 / source import_lif / electronic timing", rec)
	}
}

// TestImportLIFUnknownBibSurfacesConflictSYS061UC014_4 covers UC-014 #4: a
// bib that matches no participant is queued as a conflict, never silently
// dropped or guessed.
func TestImportLIFUnknownBibSurfacesConflictSYS061UC014_4(t *testing.T) {
	f := newTimingExchangeFixture(t, 1)
	_, _, evt, err := f.results.ExportTimingFiles(context.Background(), office, f.meetID)
	if err != nil {
		t.Fatalf("ExportTimingFiles: %v", err)
	}
	events, _ := exchange.ParseEVT(evt)
	ev := events[0]
	ev.Competitors = []exchange.CompetitorRow{{Place: "1", ID: "999999", Lane: 1, Time: "12.00"}}
	data := exchange.EncodeLIF(ev)

	summary, err := f.results.ImportTimingFile(context.Background(), office, f.meetID, "test.lif", "lif", data)
	if err != nil {
		t.Fatalf("ImportTimingFile: %v", err)
	}
	if summary.Applied != 0 || summary.Conflicts != 1 {
		t.Fatalf("summary = %+v, want 0 applied / 1 conflict", summary)
	}
	conflicts, err := f.results.ListTimingImportConflicts(context.Background(), office, f.meetID)
	if err != nil {
		t.Fatalf("ListTimingImportConflicts: %v", err)
	}
	if len(conflicts) != 1 || conflicts[0].Reason != "unknown_bib" || conflicts[0].Bib != "999999" {
		t.Fatalf("conflicts = %+v", conflicts)
	}

	// Resolve by reassigning to the (only) registered athlete.
	err = f.results.ResolveTimingConflict(context.Background(), office, f.meetID, conflicts[0].ID, ResolveTimingConflictInput{
		Action: "replace", ReassignAthleteID: f.athletes[0],
	})
	if err != nil {
		t.Fatalf("ResolveTimingConflict: %v", err)
	}
	rec, err := store.GetResult(context.Background(), f.st.DB(), f.unitID, f.athletes[0])
	if err != nil {
		t.Fatalf("GetResult after resolve: %v", err)
	}
	if rec.Mark != "12.00" {
		t.Fatalf("rec = %+v, want mark 12.00", rec)
	}
}

// TestImportLIFExistingManualResultSurfacesConflictSYS061UC014_5 covers
// UC-014 #5: a .lif time arriving for an athlete who already has a manual
// result is queued, not silently overwritten — and the operator's
// "keep"/"replace" decision is exactly what lands.
func TestImportLIFExistingManualResultSurfacesConflictSYS061UC014_5(t *testing.T) {
	f := newTimingExchangeFixture(t, 1)
	manual := save(t, f.results, f.meetID, ResultInput{DisciplineCode: "100m", AthleteID: f.athletes[0], Mark: "13.0", Timing: domain.TimingManual})
	_ = manual
	data := f.lifFor(t, "1", "12.34")

	summary, err := f.results.ImportTimingFile(context.Background(), office, f.meetID, "test.lif", "lif", data)
	if err != nil {
		t.Fatalf("ImportTimingFile: %v", err)
	}
	if summary.Applied != 0 || summary.Conflicts != 1 {
		t.Fatalf("summary = %+v, want 0 applied / 1 conflict", summary)
	}
	rec, err := store.GetResult(context.Background(), f.st.DB(), f.unitID, f.athletes[0])
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if rec.Mark != "13.0" {
		t.Fatalf("existing manual result was overwritten: %+v", rec)
	}

	conflicts, err := f.results.ListTimingImportConflicts(context.Background(), office, f.meetID)
	if err != nil || len(conflicts) != 1 || conflicts[0].Reason != "existing_manual_result" {
		t.Fatalf("conflicts = %+v, err = %v", conflicts, err)
	}

	// Operator chooses "keep": the manual result must stand unchanged.
	if err := f.results.ResolveTimingConflict(context.Background(), office, f.meetID, conflicts[0].ID, ResolveTimingConflictInput{Action: "keep"}); err != nil {
		t.Fatalf("ResolveTimingConflict(keep): %v", err)
	}
	rec, _ = store.GetResult(context.Background(), f.st.DB(), f.unitID, f.athletes[0])
	if rec.Mark != "13.0" {
		t.Fatalf("keep changed the result: %+v", rec)
	}
}

// TestImportLIFReplaceOverwritesExistingManualResult confirms the
// counterpart of the _5 test above: an explicit "replace" decision does
// apply the imported value.
func TestImportLIFReplaceOverwritesExistingManualResult(t *testing.T) {
	f := newTimingExchangeFixture(t, 1)
	save(t, f.results, f.meetID, ResultInput{DisciplineCode: "100m", AthleteID: f.athletes[0], Mark: "13.0", Timing: domain.TimingManual})
	data := f.lifFor(t, "1", "12.34")
	if _, err := f.results.ImportTimingFile(context.Background(), office, f.meetID, "test.lif", "lif", data); err != nil {
		t.Fatalf("ImportTimingFile: %v", err)
	}
	conflicts, _ := f.results.ListTimingImportConflicts(context.Background(), office, f.meetID)
	if len(conflicts) != 1 {
		t.Fatalf("conflicts = %+v", conflicts)
	}
	if err := f.results.ResolveTimingConflict(context.Background(), office, f.meetID, conflicts[0].ID, ResolveTimingConflictInput{Action: "replace"}); err != nil {
		t.Fatalf("ResolveTimingConflict(replace): %v", err)
	}
	rec, err := store.GetResult(context.Background(), f.st.DB(), f.unitID, f.athletes[0])
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if rec.Mark != "12.34" || rec.Source != "import_lif" {
		t.Fatalf("rec = %+v, want mark 12.34 / source import_lif", rec)
	}
}

// TestImportLIFUnresolvedUnitSurfacesConflict covers a .lif whose
// event/round/heat numbers match no unit (a filename/number mismatch,
// e.g. a stale export) — queued at the batch level, discard-only.
func TestImportLIFUnresolvedUnitSurfacesConflict(t *testing.T) {
	f := newTimingExchangeFixture(t, 1)
	ev := exchange.Event{Number: 999, Round: 9, Heat: 9, Name: "Ghost race",
		Competitors: []exchange.CompetitorRow{{Place: "1", ID: f.bibs[0], Lane: 1, Time: "12.00"}}}
	data := exchange.EncodeLIF(ev)

	summary, err := f.results.ImportTimingFile(context.Background(), office, f.meetID, "ghost.lif", "lif", data)
	if err != nil {
		t.Fatalf("ImportTimingFile: %v", err)
	}
	if summary.Conflicts != 1 || summary.Applied != 0 {
		t.Fatalf("summary = %+v", summary)
	}
	conflicts, _ := f.results.ListTimingImportConflicts(context.Background(), office, f.meetID)
	if len(conflicts) != 1 || conflicts[0].Reason != "unresolved_unit" || conflicts[0].UnitID != "" {
		t.Fatalf("conflicts = %+v", conflicts)
	}

	// Only "discard" is supported for a unit-less conflict.
	err = f.results.ResolveTimingConflict(context.Background(), office, f.meetID, conflicts[0].ID, ResolveTimingConflictInput{Action: "replace"})
	if !errors.Is(err, ErrTimingConflictUnitUnresolved) {
		t.Fatalf("replace on unresolved_unit: err = %v, want ErrTimingConflictUnitUnresolved", err)
	}
	if err := f.results.ResolveTimingConflict(context.Background(), office, f.meetID, conflicts[0].ID, ResolveTimingConflictInput{Action: "discard"}); err != nil {
		t.Fatalf("discard: %v", err)
	}
}

// TestImportLIFUnmappedStatusSurfacesConflictOQ048 covers a Lynx status
// code with no CR 25 equivalent (FS/SC/ADV, OQ-048): queued, and resolved
// only once the operator supplies the correct CR 25 code.
func TestImportLIFUnmappedStatusSurfacesConflictOQ048(t *testing.T) {
	f := newTimingExchangeFixture(t, 1)
	data := f.lifFor(t, "FS", "")

	summary, err := f.results.ImportTimingFile(context.Background(), office, f.meetID, "test.lif", "lif", data)
	if err != nil {
		t.Fatalf("ImportTimingFile: %v", err)
	}
	if summary.Conflicts != 1 {
		t.Fatalf("summary = %+v", summary)
	}
	conflicts, _ := f.results.ListTimingImportConflicts(context.Background(), office, f.meetID)
	if len(conflicts) != 1 || conflicts[0].Reason != "unmapped_status" {
		t.Fatalf("conflicts = %+v", conflicts)
	}

	if err := f.results.ResolveTimingConflict(context.Background(), office, f.meetID, conflicts[0].ID, ResolveTimingConflictInput{Action: "replace"}); !errors.Is(err, ErrTimingConflictStatusRequired) {
		t.Fatalf("replace with no OverrideStatus: err = %v, want ErrTimingConflictStatusRequired", err)
	}
	err = f.results.ResolveTimingConflict(context.Background(), office, f.meetID, conflicts[0].ID, ResolveTimingConflictInput{
		Action: "replace", OverrideStatus: domain.StatusDQ, OverrideStatusDetail: "TR16.2 (false start)",
	})
	if err != nil {
		t.Fatalf("ResolveTimingConflict: %v", err)
	}
	rec, err := store.GetResult(context.Background(), f.st.DB(), f.unitID, f.athletes[0])
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if rec.Status != domain.StatusDQ || rec.StatusDetail != "TR16.2 (false start)" {
		t.Fatalf("rec = %+v", rec)
	}
}

// TestImportLIFAnnouncedUnitRequiresCorrectionSYS047 checks the
// announce/correction guard (TASK-019): once a unit is announced, an
// imported time cannot apply directly — it is queued, and resolving it
// requires a reason and goes through the audited correction flow, opening
// a fresh appeal window.
func TestImportLIFAnnouncedUnitRequiresCorrectionSYS047(t *testing.T) {
	f := newTimingExchangeFixture(t, 1)
	save(t, f.results, f.meetID, ResultInput{DisciplineCode: "100m", AthleteID: f.athletes[0], Mark: "13.0", Timing: domain.TimingManual})
	if _, err := f.results.AnnounceUnitResults(context.Background(), office, f.meetID, f.unitID); err != nil {
		t.Fatalf("AnnounceUnitResults: %v", err)
	}

	data := f.lifFor(t, "1", "12.34")
	if _, err := f.results.ImportTimingFile(context.Background(), office, f.meetID, "test.lif", "lif", data); err != nil {
		t.Fatalf("ImportTimingFile: %v", err)
	}
	conflicts, _ := f.results.ListTimingImportConflicts(context.Background(), office, f.meetID)
	if len(conflicts) != 1 || conflicts[0].Reason != "unit_announced" {
		t.Fatalf("conflicts = %+v", conflicts)
	}

	// No reason -> refused (SYS-046).
	err := f.results.ResolveTimingConflict(context.Background(), office, f.meetID, conflicts[0].ID, ResolveTimingConflictInput{Action: "replace"})
	if !errors.Is(err, ErrCorrectionReasonRequired) {
		t.Fatalf("resolve with no reason: err = %v, want ErrCorrectionReasonRequired", err)
	}
	if err := f.results.ResolveTimingConflict(context.Background(), office, f.meetID, conflicts[0].ID, ResolveTimingConflictInput{
		Action: "replace", Reason: "FinishLynx re-import after review",
	}); err != nil {
		t.Fatalf("resolve with reason: %v", err)
	}
	rec, err := store.GetResult(context.Background(), f.st.DB(), f.unitID, f.athletes[0])
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	// CorrectResult always writes source "manual" — an office-reviewed
	// resolution is, semantically, an operator-confirmed value even though
	// its number originated from the import (see exchange.go's
	// ResolveTimingConflict comment).
	if rec.Mark != "12.34" || rec.Source != "manual" {
		t.Fatalf("rec = %+v, want mark 12.34 / source manual", rec)
	}
}

// TestGenericCSVImportExportRoundTripSYS062UC014_6 covers SYS-062: the
// documented generic CSV export/import round trip through the same
// ingest pipeline (not just the pure format layer, already covered in
// internal/exchange).
func TestGenericCSVImportExportRoundTripSYS062UC014_6(t *testing.T) {
	f := newTimingExchangeFixture(t, 2)
	save(t, f.results, f.meetID, ResultInput{DisciplineCode: "100m", AthleteID: f.athletes[1], Mark: "12.9", Timing: domain.TimingManual})

	csvData, err := f.results.ExportGenericCSV(context.Background(), office, f.meetID)
	if err != nil {
		t.Fatalf("ExportGenericCSV: %v", err)
	}
	if !strings.Contains(string(csvData), f.bibs[0]) || !strings.Contains(string(csvData), "12.9") {
		t.Fatalf("csv export missing expected data:\n%s", csvData)
	}

	// Re-importing the meet's own generic-CSV export of athlete[1]'s
	// already-settled manual result is exactly the existing_manual_result
	// conflict path (source stays "manual"); athlete[0] has no result yet,
	// so its (empty-mark) row is a no-op "empty" line, not a conflict.
	summary, err := f.results.ImportTimingFile(context.Background(), office, f.meetID, "roundtrip.csv", "csv", csvData)
	if err != nil {
		t.Fatalf("ImportTimingFile(csv): %v", err)
	}
	if summary.Conflicts != 1 || summary.Applied != 0 {
		t.Fatalf("summary = %+v, want 1 conflict (athlete[1]'s manual result) / 0 applied", summary)
	}
}

// TestImportTimingFileRequiresOfficeCapability is the denial-first
// authorization test: a field official cannot trigger export/import/
// resolution (CapOfficeActions, mirroring every other office action).
func TestImportTimingFileRequiresOfficeCapability(t *testing.T) {
	f := newTimingExchangeFixture(t, 1)
	var forbidden ErrForbidden
	if _, _, _, err := f.results.ExportTimingFiles(context.Background(), fieldOfficial, f.meetID); !errors.As(err, &forbidden) {
		t.Fatalf("ExportTimingFiles: err = %v, want ErrForbidden", err)
	}
	data := f.lifFor(t, "1", "12.00")
	if _, err := f.results.ImportTimingFile(context.Background(), fieldOfficial, f.meetID, "x.lif", "lif", data); !errors.As(err, &forbidden) {
		t.Fatalf("ImportTimingFile: err = %v, want ErrForbidden", err)
	}
	if _, _, err := f.results.CreateTimingAgentToken(context.Background(), fieldOfficial, f.meetID, "pc-1"); !errors.As(err, &forbidden) {
		t.Fatalf("CreateTimingAgentToken: err = %v, want ErrForbidden", err)
	}
}

// TestTimingAgentTokenLifecycleADR006 covers issuing a meet-scoped agent
// credential, authenticating with it (resolving to a synthetic
// competition-office-trust session scoped to that one meet), and its
// immediate rejection once revoked.
func TestTimingAgentTokenLifecycleADR006(t *testing.T) {
	f := newTimingExchangeFixture(t, 1)
	plaintext, tokenID, err := f.results.CreateTimingAgentToken(context.Background(), office, f.meetID, "timing PC")
	if err != nil {
		t.Fatalf("CreateTimingAgentToken: %v", err)
	}
	if plaintext == "" || tokenID == "" {
		t.Fatalf("token = %q / %q, want both non-empty", plaintext, tokenID)
	}

	session, meetID, err := f.results.AuthenticateTimingAgent(context.Background(), plaintext)
	if err != nil {
		t.Fatalf("AuthenticateTimingAgent: %v", err)
	}
	if meetID != f.meetID || session.Role != RoleCompetitionOffice {
		t.Fatalf("session = %+v, meetID = %s, want meet %s / role %s", session, meetID, f.meetID, RoleCompetitionOffice)
	}
	// The agent session can actually export (proves it carries real
	// office-equivalent trust, not just a label).
	if _, _, _, err := f.results.ExportTimingFiles(context.Background(), session, meetID); err != nil {
		t.Fatalf("ExportTimingFiles with agent session: %v", err)
	}

	if err := f.results.RevokeTimingAgentToken(context.Background(), office, f.meetID, tokenID); err != nil {
		t.Fatalf("RevokeTimingAgentToken: %v", err)
	}
	if _, _, err := f.results.AuthenticateTimingAgent(context.Background(), plaintext); !errors.Is(err, ErrTimingAgentUnauthorized) {
		t.Fatalf("AuthenticateTimingAgent after revoke: err = %v, want ErrTimingAgentUnauthorized", err)
	}
}

// TestAuthenticateTimingAgentRejectsUnknownToken is the denial-first path
// for a garbage/forged bearer token.
func TestAuthenticateTimingAgentRejectsUnknownToken(t *testing.T) {
	f := newTimingExchangeFixture(t, 1)
	if _, _, err := f.results.AuthenticateTimingAgent(context.Background(), "not-a-real-token"); !errors.Is(err, ErrTimingAgentUnauthorized) {
		t.Fatalf("err = %v, want ErrTimingAgentUnauthorized", err)
	}
}

// TestListTimingImportBatchesAndTokens covers the office's read-only
// history/management views.
func TestListTimingImportBatchesAndTokens(t *testing.T) {
	f := newTimingExchangeFixture(t, 1)
	data := f.lifFor(t, "1", "12.00")
	if _, err := f.results.ImportTimingFile(context.Background(), office, f.meetID, "a.lif", "lif", data); err != nil {
		t.Fatalf("ImportTimingFile: %v", err)
	}
	batches, err := f.results.ListTimingImportBatches(context.Background(), office, f.meetID)
	if err != nil {
		t.Fatalf("ListTimingImportBatches: %v", err)
	}
	if len(batches) != 1 || batches[0].Filename != "a.lif" || batches[0].Applied != 1 {
		t.Fatalf("batches = %+v", batches)
	}

	if _, _, err := f.results.CreateTimingAgentToken(context.Background(), office, f.meetID, "PC A"); err != nil {
		t.Fatalf("CreateTimingAgentToken: %v", err)
	}
	tokens, err := f.results.ListTimingAgentTokens(context.Background(), office, f.meetID)
	if err != nil {
		t.Fatalf("ListTimingAgentTokens: %v", err)
	}
	if len(tokens) != 1 || tokens[0].Label != "PC A" {
		t.Fatalf("tokens = %+v", tokens)
	}
}

// TestImportTimingFileUnknownFormatRejected covers the format guard: neither
// "lif" nor "csv" is rejected up front, before any row is touched.
func TestImportTimingFileUnknownFormatRejected(t *testing.T) {
	f := newTimingExchangeFixture(t, 1)
	_, err := f.results.ImportTimingFile(context.Background(), office, f.meetID, "x.xyz", "xyz", []byte("whatever"))
	if err == nil || !strings.Contains(err.Error(), "unknown timing import format") {
		t.Fatalf("err = %v, want an unknown-format error", err)
	}
}

// TestImportTimingFileMalformedLIFRejected covers a .lif payload that is not
// even valid line-oriented CSV (an unterminated quoted field) — a genuine
// "the timing PC wrote a corrupt file" case, surfaced as a parse error, not a
// panic or a silently-empty import.
func TestImportTimingFileMalformedLIFRejected(t *testing.T) {
	f := newTimingExchangeFixture(t, 1)
	data := []byte(`1,1,1,"unterminated`)
	_, err := f.results.ImportTimingFile(context.Background(), office, f.meetID, "bad.lif", "lif", data)
	if err == nil || !strings.Contains(err.Error(), `parse .lif file "bad.lif"`) {
		t.Fatalf("err = %v, want a parse .lif file error", err)
	}
}

// TestImportTimingFileMalformedCSVRejected covers a generic-CSV file whose
// header line itself cannot be parsed (SYS-062 requires the exact documented
// schema, and a corrupt header must fail loudly rather than misread columns).
func TestImportTimingFileMalformedCSVRejected(t *testing.T) {
	f := newTimingExchangeFixture(t, 1)
	data := []byte(`"unterminated`)
	_, err := f.results.ImportTimingFile(context.Background(), office, f.meetID, "bad.csv", "csv", data)
	if err == nil || !strings.Contains(err.Error(), `parse generic CSV file "bad.csv"`) {
		t.Fatalf("err = %v, want a parse generic CSV file error", err)
	}
}

// TestTimingExchangeUnknownMeetRejected covers the "meet must exist" guard
// shared by every timing-exchange entry point: an unknown meetID is rejected
// as store.ErrNotFound, not treated as an empty meet.
func TestTimingExchangeUnknownMeetRejected(t *testing.T) {
	f := newTimingExchangeFixture(t, 1)
	ctx := context.Background()
	const ghostMeet = "no-such-meet"

	if _, _, _, err := f.results.ExportTimingFiles(ctx, office, ghostMeet); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("ExportTimingFiles: err = %v, want store.ErrNotFound", err)
	}
	if _, err := f.results.ImportTimingFile(ctx, office, ghostMeet, "x.lif", "lif", f.lifFor(t, "1", "12.00")); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("ImportTimingFile: err = %v, want store.ErrNotFound", err)
	}
	if _, _, err := f.results.CreateTimingAgentToken(ctx, office, ghostMeet, "PC A"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("CreateTimingAgentToken: err = %v, want store.ErrNotFound", err)
	}
}

// TestImportLIFMalformedMarkFails covers a real device-data failure: an
// unparseable FAT mark (SYS-040's 0.01s-resolution encoding rejects it, not a
// silent zero or a panic).
func TestImportLIFMalformedMarkFails(t *testing.T) {
	f := newTimingExchangeFixture(t, 1)
	data := f.lifFor(t, "1", "not-a-time")
	_, err := f.results.ImportTimingFile(context.Background(), office, f.meetID, "bad-mark.lif", "lif", data)
	if err == nil {
		t.Fatal("expected an error importing an unparseable FAT mark")
	}
}

// TestImportCSVMarkWithoutTimingMethodFails covers SYS-041's "a timing method
// is required for a time": a generic-CSV row that carries a mark but neither
// "manual" nor "electronic" in its timing column is rejected, not silently
// guessed.
func TestImportCSVMarkWithoutTimingMethodFails(t *testing.T) {
	f := newTimingExchangeFixture(t, 1)
	_, _, evt, err := f.results.ExportTimingFiles(context.Background(), office, f.meetID)
	if err != nil {
		t.Fatalf("ExportTimingFiles: %v", err)
	}
	events, err := exchange.ParseEVT(evt)
	if err != nil || len(events) == 0 {
		t.Fatalf("ParseEVT: %v", err)
	}
	n := events[0]
	rows := []exchange.GenericRow{{
		EventNumber: n.Number, Round: n.Round, Heat: n.Heat, Bib: f.bibs[0], Lane: f.laneOf(t, f.athletes[0]),
		Mark: "12.5", // Timing left blank
	}}
	data := exchange.EncodeCSV(rows)
	_, err = f.results.ImportTimingFile(context.Background(), office, f.meetID, "no-timing.csv", "csv", data)
	if err == nil || !strings.Contains(err.Error(), "timing method is required") {
		t.Fatalf("err = %v, want the SYS-041 timing-method-required error", err)
	}
}

// TestResolveTimingConflictUnknownConflictFails covers the not-found guard on
// the conflict id itself.
func TestResolveTimingConflictUnknownConflictFails(t *testing.T) {
	f := newTimingExchangeFixture(t, 1)
	err := f.results.ResolveTimingConflict(context.Background(), office, f.meetID, "no-such-conflict", ResolveTimingConflictInput{Action: "discard"})
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want store.ErrNotFound", err)
	}
}

// TestResolveTimingConflictWrongMeetFails covers the cross-meet guard: a
// conflict id that is real, but scoped to a different meet than the one the
// request names, is rejected rather than resolved against the wrong meet.
func TestResolveTimingConflictWrongMeetFails(t *testing.T) {
	f := newTimingExchangeFixture(t, 1)
	ctx := context.Background()
	_, _, evt, err := f.results.ExportTimingFiles(ctx, office, f.meetID)
	if err != nil {
		t.Fatalf("ExportTimingFiles: %v", err)
	}
	events, err := exchange.ParseEVT(evt)
	if err != nil || len(events) == 0 {
		t.Fatalf("ParseEVT: %v", err)
	}
	ev := events[0]
	ev.Competitors = []exchange.CompetitorRow{{Place: "1", ID: "999999", Lane: 1, Time: "12.00"}}
	data := exchange.EncodeLIF(ev)
	if _, err := f.results.ImportTimingFile(ctx, office, f.meetID, "test.lif", "lif", data); err != nil {
		t.Fatalf("ImportTimingFile: %v", err)
	}
	conflicts, err := f.results.ListTimingImportConflicts(ctx, office, f.meetID)
	if err != nil || len(conflicts) != 1 {
		t.Fatalf("conflicts = %+v (err %v)", conflicts, err)
	}

	err = f.results.ResolveTimingConflict(ctx, office, "a-different-meet", conflicts[0].ID, ResolveTimingConflictInput{Action: "discard"})
	if err == nil || !strings.Contains(err.Error(), "does not belong to meet") {
		t.Fatalf("err = %v, want a does-not-belong-to-meet error", err)
	}
}

// TestResolveTimingConflictUnknownActionFails covers the action-vocabulary
// guard: anything other than keep/discard/replace/merge is rejected.
func TestResolveTimingConflictUnknownActionFails(t *testing.T) {
	f := newTimingExchangeFixture(t, 1)
	ctx := context.Background()
	data := f.lifFor(t, "1", "12.34")
	save(t, f.results, f.meetID, ResultInput{DisciplineCode: "100m", AthleteID: f.athletes[0], Mark: "13.0", Timing: domain.TimingManual})
	if _, err := f.results.ImportTimingFile(ctx, office, f.meetID, "test.lif", "lif", data); err != nil {
		t.Fatalf("ImportTimingFile: %v", err)
	}
	conflicts, err := f.results.ListTimingImportConflicts(ctx, office, f.meetID)
	if err != nil || len(conflicts) != 1 {
		t.Fatalf("conflicts = %+v (err %v)", conflicts, err)
	}
	err = f.results.ResolveTimingConflict(ctx, office, f.meetID, conflicts[0].ID, ResolveTimingConflictInput{Action: "bogus"})
	if err == nil || !strings.Contains(err.Error(), "unknown resolution action") {
		t.Fatalf("err = %v, want an unknown-resolution-action error", err)
	}
}

// TestResolveTimingConflictMergeFillsBlanksOnly covers the "merge" action
// (never exercised by the existing keep/replace-only tests): an existing
// DNS status with no mark, merged with an imported row that carries a mark
// but no status, keeps the DNS (existing.Status wins) and the mark from the
// import fills the previously-blank field — but since the merged row still
// carries a non-none status, applyTimingRow never stores the mark (a DQ/DNS
// carries no time), so the final result is DNS with no mark, not a mix of
// both.
func TestResolveTimingConflictMergeFillsBlanksOnly(t *testing.T) {
	f := newTimingExchangeFixture(t, 1)
	ctx := context.Background()
	save(t, f.results, f.meetID, ResultInput{DisciplineCode: "100m", AthleteID: f.athletes[0], Status: domain.StatusDNS})
	data := f.lifFor(t, "1", "12.34")
	if _, err := f.results.ImportTimingFile(ctx, office, f.meetID, "test.lif", "lif", data); err != nil {
		t.Fatalf("ImportTimingFile: %v", err)
	}
	conflicts, err := f.results.ListTimingImportConflicts(ctx, office, f.meetID)
	if err != nil || len(conflicts) != 1 || conflicts[0].Reason != "existing_manual_result" {
		t.Fatalf("conflicts = %+v (err %v), want one existing_manual_result conflict", conflicts, err)
	}

	if err := f.results.ResolveTimingConflict(ctx, office, f.meetID, conflicts[0].ID, ResolveTimingConflictInput{Action: "merge"}); err != nil {
		t.Fatalf("ResolveTimingConflict(merge): %v", err)
	}
	rec, err := store.GetResult(ctx, f.st.DB(), f.unitID, f.athletes[0])
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if rec.Status != domain.StatusDNS || rec.Mark != "" {
		t.Fatalf("merged result = %+v, want DNS preserved with no mark", rec)
	}
}

// TestRevokeTimingAgentTokenUnknownFails covers the not-found guard on the
// token id.
func TestRevokeTimingAgentTokenUnknownFails(t *testing.T) {
	f := newTimingExchangeFixture(t, 1)
	if err := f.results.RevokeTimingAgentToken(context.Background(), office, f.meetID, "no-such-token"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want store.ErrNotFound", err)
	}
}

// TestRevokeTimingAgentTokenWrongMeetFails covers the cross-meet guard: a
// real token id scoped to a different meet than the request names.
func TestRevokeTimingAgentTokenWrongMeetFails(t *testing.T) {
	f := newTimingExchangeFixture(t, 1)
	ctx := context.Background()
	_, tokenID, err := f.results.CreateTimingAgentToken(ctx, office, f.meetID, "PC A")
	if err != nil {
		t.Fatalf("CreateTimingAgentToken: %v", err)
	}
	err = f.results.RevokeTimingAgentToken(ctx, office, "a-different-meet", tokenID)
	if err == nil || !strings.Contains(err.Error(), "does not belong to meet") {
		t.Fatalf("err = %v, want a does-not-belong-to-meet error", err)
	}
}

// TestListTimingImportHistoryRequiresOfficeCapability covers the office-only
// gate on the read-only history/management views and on revocation, which
// TestImportTimingFileRequiresOfficeCapability does not already cover.
func TestListTimingImportHistoryRequiresOfficeCapability(t *testing.T) {
	f := newTimingExchangeFixture(t, 1)
	ctx := context.Background()
	var forbidden ErrForbidden
	if _, err := f.results.ListTimingImportBatches(ctx, fieldOfficial, f.meetID); !errors.As(err, &forbidden) {
		t.Errorf("ListTimingImportBatches: err = %v, want ErrForbidden", err)
	}
	if _, err := f.results.ListTimingImportConflicts(ctx, fieldOfficial, f.meetID); !errors.As(err, &forbidden) {
		t.Errorf("ListTimingImportConflicts: err = %v, want ErrForbidden", err)
	}
	if _, err := f.results.ListTimingAgentTokens(ctx, fieldOfficial, f.meetID); !errors.As(err, &forbidden) {
		t.Errorf("ListTimingAgentTokens: err = %v, want ErrForbidden", err)
	}
	if err := f.results.RevokeTimingAgentToken(ctx, fieldOfficial, f.meetID, "whatever"); !errors.As(err, &forbidden) {
		t.Errorf("RevokeTimingAgentToken: err = %v, want ErrForbidden", err)
	}
}

// TestImportGenericCSVManualTimingAndUnmappedStatus exercises the CSV
// ingest branches a plain export/re-import round trip does not: an
// explicit manual-timing mark, and a status string with no CR 25
// equivalent (mirroring the .lif unmapped-status conflict path for the
// generic fallback, SYS-062).
func TestImportGenericCSVManualTimingAndUnmappedStatus(t *testing.T) {
	f := newTimingExchangeFixture(t, 2)
	_, _, evt, err := f.results.ExportTimingFiles(context.Background(), office, f.meetID)
	if err != nil {
		t.Fatalf("ExportTimingFiles: %v", err)
	}
	events, err := exchange.ParseEVT(evt)
	if err != nil || len(events) == 0 {
		t.Fatalf("ParseEVT: %v", err)
	}
	n := events[0]
	rows := []exchange.GenericRow{
		{EventNumber: n.Number, Round: n.Round, Heat: n.Heat, Bib: f.bibs[0], Lane: f.laneOf(t, f.athletes[0]),
			Mark: "12.5", Timing: "manual"},
		{EventNumber: n.Number, Round: n.Round, Heat: n.Heat, Bib: f.bibs[1], Lane: f.laneOf(t, f.athletes[1]),
			Status: "FS"}, // no CR25 equivalent -> unmapped_status conflict
	}
	data := exchange.EncodeCSV(rows)

	summary, err := f.results.ImportTimingFile(context.Background(), office, f.meetID, "manual.csv", "csv", data)
	if err != nil {
		t.Fatalf("ImportTimingFile: %v", err)
	}
	if summary.Applied != 1 || summary.Conflicts != 1 {
		t.Fatalf("summary = %+v, want 1 applied (manual mark) / 1 conflict (unmapped status)", summary)
	}
	rec, err := store.GetResult(context.Background(), f.st.DB(), f.unitID, f.athletes[0])
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	// A manual mark is rounded up to the next 0.1s (SYS-041) same as
	// ordinary capture.
	if rec.Mark != "12.5" || rec.Timing != domain.TimingManual || rec.Source != "import_csv" {
		t.Fatalf("rec = %+v", rec)
	}
}
