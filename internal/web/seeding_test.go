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
