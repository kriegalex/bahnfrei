// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/kriegalex/bahnfrei/internal/app"
	"github.com/kriegalex/bahnfrei/internal/domain"
)

var webSubmitter = app.Session{AccountID: "01WSB", Username: "sub", Role: app.RoleEntrySubmitter}
var webOffice = app.Session{AccountID: "01WOF", Username: "webOffice", Role: app.RoleCompetitionOffice}

// seededMeetFixture creates a published meet with one 100m/U18 W event
// (qualification + final rounds) over real HTTP, plus n confirmed entries
// created directly through the app layer (the online-entry HTTP flow is
// TASK-016's own test surface). Returns the meet, event and qualification
// round IDs.
func seededMeetFixture(t *testing.T, deps *testServerDeps, client *http.Client, base string, n int) (meetID, eventID, roundID string) {
	t.Helper()
	setupAndLogin(t, client, base)
	meetID = createUCMeet(t, client, base)
	resp := addEvent(t, client, base, meetID, url.Values{
		"discipline": {"100m"}, "categories": {"U18 W"},
		"round_qualification": {"1"}, "round_final": {"1"},
	})
	_ = resp.Body.Close()
	publishMeetWeb(t, client, base, meetID)
	eventID = mustEventID(t, deps, meetID, "100m")

	ctx := context.Background()
	for i := 0; i < n; i++ {
		detail, err := deps.results.SubmitIndividualEntry(ctx, webSubmitter, meetID, app.IndividualEntryInput{
			EventID: eventID, FirstName: "Athlete", LastName: strings.Repeat("A", i+1),
			BirthYear: 2009, Sex: domain.SexFemale, SeedPerformance: "13." + strconv.Itoa(50-i),
		})
		if err != nil {
			t.Fatalf("SubmitIndividualEntry %d: %v", i, err)
		}
		if err := deps.results.ConfirmCheckIn(ctx, webOffice, meetID, detail.ID, detail.Version); err != nil {
			t.Fatalf("ConfirmCheckIn %d: %v", i, err)
		}
	}

	md, err := deps.meets.Meet(ctx, meetID)
	if err != nil {
		t.Fatalf("Meet: %v", err)
	}
	for _, pe := range md.Programme {
		if pe.ID == eventID {
			roundID = pe.Rounds[0].ID
		}
	}
	if roundID == "" {
		t.Fatal("could not resolve the qualification round id")
	}
	return meetID, eventID, roundID
}

// TestCheckInFlowHTTPSYS025UC007 drives check-in over real HTTP: an entered
// entry can be confirmed, closing check-in DNSes the rest, and a DNS entry
// can be reinstated.
func TestCheckInFlowHTTPSYS025UC007(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	meetID := createUCMeet(t, client, base)
	resp := addEvent(t, client, base, meetID, url.Values{
		"discipline": {"100m"}, "categories": {"U18 W"}, "round_final": {"1"},
	})
	_ = resp.Body.Close()
	publishMeetWeb(t, client, base, meetID)
	eventID := mustEventID(t, deps, meetID, "100m")

	ctx := context.Background()
	var entryIDs []string
	for i := 0; i < 3; i++ {
		detail, err := deps.results.SubmitIndividualEntry(ctx, webSubmitter, meetID, app.IndividualEntryInput{
			EventID: eventID, FirstName: "Athlete", LastName: strings.Repeat("B", i+1),
			BirthYear: 2009, Sex: domain.SexFemale, SeedPerformance: "13.50",
		})
		if err != nil {
			t.Fatalf("SubmitIndividualEntry: %v", err)
		}
		entryIDs = append(entryIDs, detail.ID)
	}

	checkinPage := base + "/meets/" + meetID + "/events/" + eventID + "/checkin"
	body := bodyString(t, mustGet(t, client, checkinPage))
	if !strings.Contains(body, "checkin") && !strings.Contains(body, "Check-in") {
		t.Errorf("check-in page missing expected content: %s", body)
	}

	confirmResp := postForm(t, client, checkinPage, base+"/meets/"+meetID+"/entries/"+entryIDs[0]+"/confirm", url.Values{"version": {"1"}})
	_ = confirmResp.Body.Close()
	if confirmResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("confirm entry = %d, want 303", confirmResp.StatusCode)
	}

	closeResp := postForm(t, client, checkinPage, base+"/meets/"+meetID+"/events/"+eventID+"/checkin/close", url.Values{})
	_ = closeResp.Body.Close()
	if closeResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("close check-in = %d, want 303", closeResp.StatusCode)
	}

	roster, err := deps.results.CheckInRoster(ctx, webOffice, meetID, eventID)
	if err != nil {
		t.Fatalf("CheckInRoster: %v", err)
	}
	var confirmed, dns int
	var dnsID string
	var dnsVersion int64
	for _, row := range roster {
		switch row.Status {
		case domain.EntryConfirmed:
			confirmed++
		case domain.EntryDNS:
			dns++
			dnsID, dnsVersion = row.ID, row.Version
		}
	}
	if confirmed != 1 || dns != 2 {
		t.Fatalf("expected 1 confirmed + 2 DNS after close, got confirmed=%d dns=%d", confirmed, dns)
	}

	reinstateResp := postForm(t, client, checkinPage, base+"/meets/"+meetID+"/entries/"+dnsID+"/reinstate",
		url.Values{"version": {strconv.FormatInt(dnsVersion, 10)}})
	_ = reinstateResp.Body.Close()
	if reinstateResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("reinstate entry = %d, want 303", reinstateResp.StatusCode)
	}
	roster2, err := deps.results.CheckInRoster(ctx, webOffice, meetID, eventID)
	if err != nil {
		t.Fatalf("CheckInRoster (after reinstate): %v", err)
	}
	confirmed = 0
	for _, row := range roster2 {
		if row.Status == domain.EntryConfirmed {
			confirmed++
		}
	}
	if confirmed != 2 {
		t.Errorf("expected 2 confirmed after reinstatement, got %d", confirmed)
	}
}

// TestCheckinEmptyStateSYS152UC041_4 covers F7/SYS-152/UC-041 #4: an event
// with zero online entries states why it is empty (OQ-117's honest
// both-readings copy — roster-managed template meets and not-yet-submitted
// online entries) and links to the capture page, where a roster-seeded
// meet's participants would show instead; it never offers the destructive
// "Check-in schliessen" action when there is nothing to close.
func TestCheckinEmptyStateSYS152UC041_4(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	meetID := createUCMeet(t, client, base)
	resp := addEvent(t, client, base, meetID, url.Values{
		"discipline": {"100m"}, "categories": {"U18 W"}, "round_final": {"1"},
	})
	_ = resp.Body.Close()
	publishMeetWeb(t, client, base, meetID)
	eventID := mustEventID(t, deps, meetID, "100m")

	checkinPage := base + "/meets/" + meetID + "/events/" + eventID + "/checkin"
	body := bodyString(t, mustGet(t, client, checkinPage))
	if !strings.Contains(body, "Roster verwaltet") {
		t.Errorf("empty check-in must explain why (OQ-117): %s", body)
	}
	if !strings.Contains(body, `href="/meets/`+meetID+`/capture"`) {
		t.Errorf("empty check-in must link to the capture page as the next step: %s", body)
	}
	if strings.Contains(body, "checkin/close/confirm") {
		t.Error("empty check-in must not offer the close action — nothing to close")
	}
	if strings.Contains(body, `action="/meets/`+meetID+`/events/`+eventID+`/checkin/close"`) {
		t.Error("empty check-in must not render a live close form")
	}
}

// TestCheckInCloseInapplicableWhenAllResolvedSYS152UC041_4 covers the other
// half of F7/SYS-152/UC-041 #4: entries exist but every one is already
// resolved (confirmed, here — seededMeetFixture confirms each entry it
// creates), so there is nothing left for "close check-in" to do. The list
// page hides the action behind an explanatory hint instead of a live
// control, and navigating straight to the close-confirm URL (a stale link,
// or a typed one) renders the same explanation rather than a live confirm
// button.
func TestCheckInCloseInapplicableWhenAllResolvedSYS152UC041_4(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	meetID, eventID, _ := seededMeetFixture(t, deps, client, base, 2)

	checkinPage := base + "/meets/" + meetID + "/events/" + eventID + "/checkin"
	body := bodyString(t, mustGet(t, client, checkinPage))
	if strings.Contains(body, "checkin/close/confirm") {
		t.Error("check-in with nothing left to close must not offer the close action")
	}
	if !strings.Contains(body, "Nichts zu schliessen") {
		t.Errorf("check-in with nothing left to close must state why: %s", body)
	}

	confirmBody := bodyString(t, mustGet(t, client, checkinPage+"/close/confirm"))
	if strings.Contains(confirmBody, `action="/meets/`+meetID+`/events/`+eventID+`/checkin/close"`) {
		t.Errorf("close-confirm with nothing applicable must not render a live form: %s", confirmBody)
	}
	if !strings.Contains(confirmBody, "Nichts zu schliessen") {
		t.Errorf("close-confirm with nothing applicable must render the explanation: %s", confirmBody)
	}
}

// TestCheckInCloseConfirmShowsCountAndClosesSYS152UC041_5 covers UC-041 #5:
// the close-confirm sub-page states the number of entries it will affect
// before its confirm button, and only its own POST applies the close.
func TestCheckInCloseConfirmShowsCountAndClosesSYS152UC041_5(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	meetID := createUCMeet(t, client, base)
	resp := addEvent(t, client, base, meetID, url.Values{
		"discipline": {"100m"}, "categories": {"U18 W"}, "round_final": {"1"},
	})
	_ = resp.Body.Close()
	publishMeetWeb(t, client, base, meetID)
	eventID := mustEventID(t, deps, meetID, "100m")

	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if _, err := deps.results.SubmitIndividualEntry(ctx, webSubmitter, meetID, app.IndividualEntryInput{
			EventID: eventID, FirstName: "Athlete", LastName: strings.Repeat("C", i+1),
			BirthYear: 2009, Sex: domain.SexFemale, SeedPerformance: "13.50",
		}); err != nil {
			t.Fatalf("SubmitIndividualEntry: %v", err)
		}
	}

	checkinPage := base + "/meets/" + meetID + "/events/" + eventID + "/checkin"
	confirmHref := "/meets/" + meetID + "/events/" + eventID + "/checkin/close/confirm"
	body := bodyString(t, mustGet(t, client, checkinPage))
	if !strings.Contains(body, `href="`+confirmHref+`"`) {
		t.Fatalf("check-in with unresolved entries must link to the close-confirm page: %s", body)
	}

	confirmBody := bodyString(t, mustGet(t, client, base+confirmHref))
	if !strings.Contains(confirmBody, "2 nicht bestätigte") {
		t.Errorf("close-confirm must state the affected count (UC-041 #5): %s", confirmBody)
	}
	closeAction := "/meets/" + meetID + "/events/" + eventID + "/checkin/close"
	if !strings.Contains(confirmBody, `action="`+closeAction+`"`) {
		t.Errorf("close-confirm must post to the close route: %s", confirmBody)
	}

	resp = postForm(t, client, base+confirmHref, base+closeAction, url.Values{})
	_ = bodyString(t, resp)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("confirmed close = %d, want 303", resp.StatusCode)
	}

	roster, err := deps.results.CheckInRoster(ctx, webOffice, meetID, eventID)
	if err != nil {
		t.Fatalf("CheckInRoster: %v", err)
	}
	for _, row := range roster {
		if row.Status != domain.EntryDNS {
			t.Errorf("entry %s should be DNS after the confirmed close, got %s", row.ID, row.Status)
		}
	}
}

// TestCheckInMobileCaptureMarkupSYS147UC039TASK045 pins the check-in
// table's mobile-ergonomics markup (SYS-147, UC-039 #1): the row-card
// responsive class and the restored table-semantics roles/data-label the
// row-card CSS depends on, plus the compact breadcrumb-style heading — the
// actual no-horizontal-scroll rendering is proven live in the browser by
// e2e/tests/mobile-capture-UC039.spec.ts.
func TestCheckInMobileCaptureMarkupSYS147UC039TASK045(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	meetID := createUCMeet(t, client, base)
	resp := addEvent(t, client, base, meetID, url.Values{
		"discipline": {"100m"}, "categories": {"U18 W"}, "round_final": {"1"},
	})
	_ = resp.Body.Close()
	publishMeetWeb(t, client, base, meetID)
	eventID := mustEventID(t, deps, meetID, "100m")

	ctx := context.Background()
	if _, err := deps.results.SubmitIndividualEntry(ctx, webSubmitter, meetID, app.IndividualEntryInput{
		EventID: eventID, FirstName: "Athlete", LastName: "Test",
		BirthYear: 2009, Sex: domain.SexFemale, SeedPerformance: "13.50",
	}); err != nil {
		t.Fatalf("SubmitIndividualEntry: %v", err)
	}

	checkinPage := base + "/meets/" + meetID + "/events/" + eventID + "/checkin"
	body := bodyString(t, mustGet(t, client, checkinPage))
	for _, want := range []string{
		`class="stack-table"`,
		`role="table"`,
		`role="columnheader"`,
		`role="cell"`,
		`data-label=`,
		`class="capture-title"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("check-in page missing %q", want)
		}
	}
	if strings.Contains(body, "<h1>") {
		t.Error("check-in page should render the compact <h1 class=\"capture-title\">, not a bare <h1>")
	}
}

// TestSeedingGenerateAndOverrideHTTPSYS026UC008 drives heat generation and a
// manual override over real HTTP.
func TestSeedingGenerateAndOverrideHTTPSYS026UC008(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	meetID, eventID, roundID := seededMeetFixture(t, deps, client, base, 10)

	seedingPage := base + "/meets/" + meetID + "/events/" + eventID + "/rounds/" + roundID + "/seeding"
	empty := bodyString(t, mustGet(t, client, seedingPage))
	if !strings.Contains(empty, "seeding") && !strings.Contains(empty, "Seeding") && !strings.Contains(empty, "Setzung") && !strings.Contains(empty, "Séries") {
		t.Errorf("seeding page missing expected content before generation: %s", empty)
	}
	// OQ-135/TASK-052 empty-state sweep: the "no heats yet" state names why
	// (not generated yet) and the same-page next step (the generate form
	// directly above it), not a bare "no heats" sentence.
	if !strings.Contains(empty, "Noch keine Läufe generiert") || !strings.Contains(empty, "über das Formular oben") {
		t.Errorf("seeding empty state missing the why/next-step copy: %s", empty)
	}

	genResp := postForm(t, client, seedingPage, seedingPage+"/generate", url.Values{
		"max_heat_size": {"4"}, "track_lanes": {"0"},
	})
	_ = genResp.Body.Close()
	if genResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("generate heats = %d, want 303", genResp.StatusCode)
	}

	sheet, err := deps.results.HeatSheetFor(context.Background(), webOffice, meetID, eventID, roundID)
	if err != nil {
		t.Fatalf("HeatSheetFor: %v", err)
	}
	if len(sheet.Units) != 3 { // 10 entries / max 4 per heat -> 3 heats
		t.Fatalf("expected 3 heats, got %d", len(sheet.Units))
	}

	after := bodyString(t, mustGet(t, client, seedingPage))
	if !strings.Contains(after, sheet.Units[0].Rows[0].EntryID) {
		t.Error("expected the generated heat sheet to render on the seeding page")
	}

	// Manual override: move the first entry of heat 0 into heat 1.
	moveEntry := sheet.Units[0].Rows[0].EntryID
	overrideResp := postForm(t, client, seedingPage, seedingPage+"/override", url.Values{
		"entry_id": {moveEntry}, "target_unit": {sheet.Units[1].UnitID}, "lane": {"0"},
		"version": {strconv.FormatInt(sheet.Units[0].Rows[0].Version, 10)},
	})
	_ = overrideResp.Body.Close()
	if overrideResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("override assignment = %d, want 303", overrideResp.StatusCode)
	}
	moved, err := deps.results.HeatSheetFor(context.Background(), webOffice, meetID, eventID, roundID)
	if err != nil {
		t.Fatalf("HeatSheetFor (after override): %v", err)
	}
	found := false
	for _, row := range moved.Units[1].Rows {
		if row.EntryID == moveEntry {
			found = true
			if !row.ManualOverride {
				t.Error("expected the moved entry to carry manual_override")
			}
		}
	}
	if !found {
		t.Error("expected the manually overridden entry to appear in its new heat")
	}
}

// TestSeedingGenerateEmptyPoolHTTPSYS026UC008 is a denial/edge-path test
// (TASK-054/OQ-140, SYS-026/117/152): clicking "generate heats" on a round
// with no confirmed/checked-in entries must not silently redirect to the
// unchanged empty state — it re-renders the seeding page with an
// actionable, localized error telling the operator to confirm or check in
// entries first, at 422, and the page's normal content (including the
// still-empty heat list) still renders underneath.
func TestSeedingGenerateEmptyPoolHTTPSYS026UC008(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	meetID, eventID, roundID := seededMeetFixture(t, deps, client, base, 0)

	seedingPage := base + "/meets/" + meetID + "/events/" + eventID + "/rounds/" + roundID + "/seeding"
	genResp := postForm(t, client, seedingPage, seedingPage+"/generate", url.Values{
		"max_heat_size": {"4"}, "track_lanes": {"0"},
	})
	body := bodyString(t, genResp)
	if genResp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("generate heats (empty pool) = %d, want 422", genResp.StatusCode)
	}
	const wantMsg = "Keine bestätigten oder eingecheckten Meldungen für diese Runde — bitte zuerst Meldungen bestätigen oder einchecken."
	if !strings.Contains(body, wantMsg) {
		t.Errorf("expected the localized empty-pool error message, got: %s", body)
	}
	if !strings.Contains(body, "Noch keine Läufe generiert — über das Formular oben") {
		t.Error("expected the seeding page's normal (still-empty) content to render alongside the error")
	}

	sheet, err := deps.results.HeatSheetFor(context.Background(), webOffice, meetID, eventID, roundID)
	if err != nil {
		t.Fatalf("HeatSheetFor: %v", err)
	}
	if len(sheet.Units) != 0 {
		t.Errorf("expected no heats to have been generated, got %d units", len(sheet.Units))
	}
}

// TestSeedingOverrideVersionConflictHTTPSYS026UC008 is a denial/edge-path
// test (TASK-055, SYS-026/117): overriding a heat/lane assignment with a
// stale version — another session already moved this entry — is rejected
// with the "reload and retry" conflict wording shared with the roster/
// standings/entries/bibs/fees/meet surfaces, at 422, rather than silently
// discarding the operator's edit and redirecting as if it had applied.
func TestSeedingOverrideVersionConflictHTTPSYS026UC008(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	meetID, eventID, roundID := seededMeetFixture(t, deps, client, base, 4)

	seedingPage := base + "/meets/" + meetID + "/events/" + eventID + "/rounds/" + roundID + "/seeding"
	genResp := postForm(t, client, seedingPage, seedingPage+"/generate", url.Values{
		"max_heat_size": {"4"}, "track_lanes": {"0"},
	})
	_ = genResp.Body.Close()

	sheet, err := deps.results.HeatSheetFor(context.Background(), webOffice, meetID, eventID, roundID)
	if err != nil {
		t.Fatalf("HeatSheetFor: %v", err)
	}
	target := sheet.Units[0].Rows[0]
	staleVersion := target.Version + 1000

	overrideResp := postForm(t, client, seedingPage, seedingPage+"/override", url.Values{
		"entry_id": {target.EntryID}, "target_unit": {sheet.Units[0].UnitID}, "lane": {"2"},
		"version": {strconv.FormatInt(staleVersion, 10)},
	})
	body := bodyString(t, overrideResp)
	if overrideResp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("override with a stale version = %d, want 422", overrideResp.StatusCode)
	}
	const wantMsg = "Konflikt: Der Datensatz wurde zwischenzeitlich geändert. Bitte neu laden und erneut versuchen."
	if !strings.Contains(body, wantMsg) {
		t.Errorf("expected the localized conflict error message, got: %s", body)
	}

	after, err := deps.results.HeatSheetFor(context.Background(), webOffice, meetID, eventID, roundID)
	if err != nil {
		t.Fatalf("HeatSheetFor (after rejected override): %v", err)
	}
	if after.Units[0].Rows[0].Lane != target.Lane {
		t.Errorf("lane after a rejected override = %d, want unchanged %d", after.Units[0].Rows[0].Lane, target.Lane)
	}
}

// TestAdvanceRoundHTTPSYS029UC009 drives round progression over real HTTP:
// settled track results feed AdvanceRound, and the qualifiers can seed the
// next round.
func TestAdvanceRoundHTTPSYS029UC009(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	meetID, eventID, roundID := seededMeetFixture(t, deps, client, base, 4)

	seedingPage := base + "/meets/" + meetID + "/events/" + eventID + "/rounds/" + roundID + "/seeding"
	genResp := postForm(t, client, seedingPage, seedingPage+"/generate", url.Values{"max_heat_size": {"4"}, "track_lanes": {"0"}})
	_ = genResp.Body.Close()

	ctx := context.Background()
	sheet, err := deps.results.HeatSheetFor(ctx, webOffice, meetID, eventID, roundID)
	if err != nil {
		t.Fatalf("HeatSheetFor: %v", err)
	}
	unit := sheet.Units[0]
	roster, err := deps.results.CheckInRoster(ctx, webOffice, meetID, eventID)
	if err != nil {
		t.Fatalf("CheckInRoster: %v", err)
	}
	athleteByEntry := map[string]string{}
	for _, r := range roster {
		athleteByEntry[r.ID] = r.AthleteID
	}
	marks := []string{"12.00", "12.50", "13.00", "13.50"}
	for i, row := range unit.Rows {
		if _, err := deps.results.SaveTrackResult(ctx, webOffice, meetID, unit.UnitID, app.TrackResultInput{
			AthleteID: athleteByEntry[row.EntryID], Time: marks[i], Timing: domain.TimingElectronic,
		}); err != nil {
			t.Fatalf("SaveTrackResult: %v", err)
		}
	}

	advResp := postForm(t, client, seedingPage, base+"/meets/"+meetID+"/events/"+eventID+"/rounds/"+roundID+"/advance", url.Values{
		"top_n": {"2"}, "fastest_k": {"0"},
	})
	_ = advResp.Body.Close()
	if advResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("advance round = %d, want 303", advResp.StatusCode)
	}

	updated, err := deps.results.HeatSheetFor(ctx, webOffice, meetID, eventID, roundID)
	if err != nil {
		t.Fatalf("HeatSheetFor (after advance): %v", err)
	}
	qCount := 0
	for _, row := range updated.Units[0].Rows {
		if row.Qualification == domain.StatusQ {
			qCount++
		}
	}
	if qCount != 2 {
		t.Fatalf("expected 2 Q after advancing top 2, got %d", qCount)
	}
}

// TestAdvanceRoundRejectionsHTTPSYS029UC009 covers handleAdvanceRound's two
// known, operator-actionable rejections (TASK-055, SYS-029/117): a round
// that was never seeded at all (app.ErrRoundNotSeeded — no "generate heats"
// run yet) and a seeded round with a heat that has no settled results yet
// (app.ErrRoundNotComplete). Both re-render the seeding page with their own
// localized, actionable message at 422 instead of silently redirecting, and
// neither writes any qualification code.
func TestAdvanceRoundRejectionsHTTPSYS029UC009(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	meetID, eventID, roundID := seededMeetFixture(t, deps, client, base, 4)

	seedingPage := base + "/meets/" + meetID + "/events/" + eventID + "/rounds/" + roundID + "/seeding"
	advanceURL := base + "/meets/" + meetID + "/events/" + eventID + "/rounds/" + roundID + "/advance"

	// Never seeded: no heat generation has run for this round.
	notSeededResp := postForm(t, client, seedingPage, advanceURL, url.Values{"top_n": {"2"}, "fastest_k": {"0"}})
	notSeededBody := bodyString(t, notSeededResp)
	if notSeededResp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("advance an unseeded round = %d, want 422", notSeededResp.StatusCode)
	}
	const wantNotSeededMsg = "Für diese Runde wurden noch keine Läufe generiert — bitte zuerst über das Formular oben Läufe erzeugen."
	if !strings.Contains(notSeededBody, wantNotSeededMsg) {
		t.Errorf("expected the localized not-seeded error message, got: %s", notSeededBody)
	}

	// Seeded but incomplete: heats generated, no results saved.
	genResp := postForm(t, client, seedingPage, seedingPage+"/generate", url.Values{"max_heat_size": {"4"}, "track_lanes": {"0"}})
	_ = genResp.Body.Close()

	incompleteResp := postForm(t, client, seedingPage, advanceURL, url.Values{"top_n": {"2"}, "fastest_k": {"0"}})
	incompleteBody := bodyString(t, incompleteResp)
	if incompleteResp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("advance a round with no settled results = %d, want 422", incompleteResp.StatusCode)
	}
	const wantIncompleteMsg = "Nicht alle Läufe dieser Runde haben erfasste Resultate — bitte zuerst alle Resultate erfassen, bevor die Runde ausgewertet wird."
	if !strings.Contains(incompleteBody, wantIncompleteMsg) {
		t.Errorf("expected the localized incomplete-round error message, got: %s", incompleteBody)
	}

	sheet, err := deps.results.HeatSheetFor(context.Background(), webOffice, meetID, eventID, roundID)
	if err != nil {
		t.Fatalf("HeatSheetFor: %v", err)
	}
	for _, u := range sheet.Units {
		for _, row := range u.Rows {
			if row.Qualification != domain.StatusNone {
				t.Errorf("entry %s carries qualification %q after a rejected advance, want none", row.EntryID, row.Qualification)
			}
		}
	}
}

// TestPublicStartListShowsHeatsAndLanesSYS026SYS027 proves the public
// start-list page reflects generated heats/lanes (TASK-018): once heats are
// generated, the public page shows a lane/qualification breakdown per heat
// alongside the existing flat roster.
func TestPublicStartListShowsHeatsAndLanesSYS026SYS027(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	meetID, eventID, roundID := seededMeetFixture(t, deps, client, base, 4)

	seedingPage := base + "/meets/" + meetID + "/events/" + eventID + "/rounds/" + roundID + "/seeding"
	genResp := postForm(t, client, seedingPage, seedingPage+"/generate", url.Values{"max_heat_size": {"8"}, "track_lanes": {"8"}})
	_ = genResp.Body.Close()

	anon, _ := newTestClient(t, deps)
	body := bodyString(t, mustGet(t, anon, base+"/m/"+meetID+"/startlists"))
	if !strings.Contains(body, "Athlete") {
		t.Fatalf("public start-list page missing seeded athletes: %s", body)
	}
	sheet, err := deps.results.HeatSheetFor(context.Background(), webOffice, meetID, eventID, roundID)
	if err != nil {
		t.Fatalf("HeatSheetFor: %v", err)
	}
	laneStr := strconv.Itoa(sheet.Units[0].Rows[0].Lane)
	if !strings.Contains(body, laneStr) {
		t.Errorf("expected the public start-list page to show at least one drawn lane (%s): missing", laneStr)
	}
}

// TestManualAdvanceRecordsCodeSYS029UC009_2Web drives handleManualAdvance
// (0% baseline coverage) over real HTTP: an office operator's manual
// referee/jury/draw decision (UC-009 #2) is recorded on the target entry's
// assignment; an illegal code and an unknown entry id are both rejected with
// a localized, actionable page-level error (TASK-055/SYS-117) rather than
// silently discarded, and neither ever records anything illegal.
func TestManualAdvanceRecordsCodeSYS029UC009_2Web(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	meetID, eventID, roundID := seededMeetFixture(t, deps, client, base, 4)

	seedingPage := base + "/meets/" + meetID + "/events/" + eventID + "/rounds/" + roundID + "/seeding"
	genResp := postForm(t, client, seedingPage, seedingPage+"/generate", url.Values{"max_heat_size": {"4"}, "track_lanes": {"0"}})
	_ = genResp.Body.Close()

	ctx := context.Background()
	sheet, err := deps.results.HeatSheetFor(ctx, webOffice, meetID, eventID, roundID)
	if err != nil {
		t.Fatalf("HeatSheetFor: %v", err)
	}
	entryID := sheet.Units[0].Rows[0].EntryID
	otherEntry := sheet.Units[0].Rows[1].EntryID

	manualURL := base + "/meets/" + meetID + "/events/" + eventID + "/rounds/" + roundID + "/manual-advance"
	resp := postForm(t, client, seedingPage, manualURL, url.Values{"entry_id": {entryID}, "code": {"Q"}})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("manual advance = %d, want 303", resp.StatusCode)
	}

	qualificationOf := func(id string) domain.QualificationStatus {
		t.Helper()
		updated, err := deps.results.HeatSheetFor(ctx, webOffice, meetID, eventID, roundID)
		if err != nil {
			t.Fatalf("HeatSheetFor: %v", err)
		}
		for _, u := range updated.Units {
			for _, row := range u.Rows {
				if row.EntryID == id {
					return row.Qualification
				}
			}
		}
		t.Fatalf("entry %s not found in heat sheet", id)
		return ""
	}
	if got := qualificationOf(entryID); got != domain.StatusQ {
		t.Fatalf("qualification after manual advance = %q, want Q", got)
	}

	// An illegal code (not one of Q/q/qR/qJ/qD) is rejected by ManualAdvance
	// (app.ErrInvalidQualificationCode) and re-renders the seeding page with
	// a localized, actionable error at 422 — nothing illegal ever lands on
	// the assignment.
	badResp := postForm(t, client, seedingPage, manualURL, url.Values{"entry_id": {otherEntry}, "code": {"DNS"}})
	badBody := bodyString(t, badResp)
	if badResp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("manual advance with an illegal code = %d, want 422", badResp.StatusCode)
	}
	if !strings.Contains(badBody, "Ungültiger Qualifikationscode — bitte eine der angebotenen Optionen wählen.") {
		t.Errorf("expected the localized invalid-code error message, got: %s", badBody)
	}
	if got := qualificationOf(otherEntry); got != domain.StatusNone {
		t.Errorf("illegal manual-advance code must not be recorded, got %q", got)
	}

	// An unknown entry id (never seeded in this round) is rejected by
	// ManualAdvance (app.ErrEntryNotInRound) with its own actionable error.
	unknownResp := postForm(t, client, seedingPage, manualURL, url.Values{"entry_id": {"does-not-exist"}, "code": {"Q"}})
	unknownBody := bodyString(t, unknownResp)
	if unknownResp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("manual advance for an unknown entry = %d, want 422", unknownResp.StatusCode)
	}
	if !strings.Contains(unknownBody, "Diese Meldung ist nicht (mehr) in dieser Runde gesetzt — bitte Läufe/Setzung prüfen und erneut versuchen.") {
		t.Errorf("expected the localized entry-not-in-round error message, got: %s", unknownBody)
	}
}

// TestSeedingUnknownEventOrRoundIs404Web covers the seedingView error
// mapping (handleSeeding/handleSeedingGenerate's shared read path): a
// syntactically fine but nonexistent event/round id within a real meet
// surfaces the shared 404.
func TestSeedingUnknownEventOrRoundIs404Web(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	meetID, _, _ := seededMeetFixture(t, deps, client, base, 1)

	resp := mustGet(t, client, base+"/meets/"+meetID+"/events/does-not-exist/rounds/does-not-exist/seeding")
	_ = bodyString(t, resp)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET seeding for an unknown event/round = %d, want 404", resp.StatusCode)
	}
}

// TestSeedingRequiresOfficeCapabilityHTTP is a denial/edge-path test:
// anonymous requests to the check-in/seeding surfaces are refused
// (SYS-090).
func TestSeedingRequiresOfficeCapabilityHTTP(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	for _, path := range []string{
		"/meets/x/events/y/checkin",
		"/meets/x/events/y/rounds/z/seeding",
	} {
		resp := mustGet(t, client, base+path)
		_ = bodyString(t, resp)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("anonymous GET %s = %d, want 403 (SYS-090)", path, resp.StatusCode)
		}
	}
}

// TestCheckInConfirmRejectsOffOriginRefererOQ079 pins the check-in redirect
// handlers to the sameOriginRedirectTarget guard (the OQ-079 class, found
// unguarded here by the OQ-078 gosec pass): a crafted off-origin Referer
// must never survive into the redirect Location.
func TestCheckInConfirmRejectsOffOriginRefererOQ079(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	meetID := createUCMeet(t, client, base)

	token := csrfTokenFrom(t, bodyString(t, mustGet(t, client, base+"/meets/"+meetID)))
	form := url.Values{"version": {"1"}, "csrf_token": {token}}
	req, err := http.NewRequest(http.MethodPost, base+"/meets/"+meetID+"/entries/no-such-entry/confirm", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", "https://evil.example/phish")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("confirm with off-origin Referer = %d, want 303", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/" {
		t.Errorf("redirect Location = %q, want %q (off-origin Referer must not survive)", loc, "/")
	}
}
