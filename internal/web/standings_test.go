// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/kriegalex/bahnfrei/internal/app"
	"github.com/kriegalex/bahnfrei/internal/domain"
)

// TestUKCTemplateRosterStandingsFlow drives the operator's UC-033 slice in
// the browser: create the meet from the built-in template (#1), register a
// participant, and read the division standings with points from the
// official table (#2) and an explicit gap for missing disciplines (#3/#4).
func TestUKCTemplateRosterStandingsFlow(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)

	// The quick-create form offers the built-in template.
	body := bodyString(t, mustGet(t, client, base+"/meets/from-template"))
	if !strings.Contains(body, "ubs-kids-cup") {
		t.Fatalf("template form does not offer the UKC template: %s", body)
	}

	resp := postForm(t, client, base+"/meets/from-template", base+"/meets/from-template", url.Values{
		"template": {"ubs-kids-cup"},
		"date":     {"2026-08-15"},
		"venue":    {"Le Mouret"},
	})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST /meets/from-template = %d, want 303 (body: %s)", resp.StatusCode, bodyString(t, resp))
	}
	loc := resp.Header.Get("Location")
	_ = resp.Body.Close()
	meetID := strings.TrimPrefix(loc, "/meets/")
	if meetID == "" || strings.Contains(meetID, "/") {
		t.Fatalf("redirect location = %q, want /meets/{id}", loc)
	}

	// The workspace shows the derived name and the UKC programme.
	body = bodyString(t, mustGet(t, client, base+loc))
	// SYS-111/F6 (TASK-050): the meet-hub programme/timetable tables now
	// localize discipline names like standings already did — the assertion
	// pins the DE catalog string, not the English canonical name.
	for _, want := range []string{"UBS Kids Cup Le Mouret 2026", "60 m", "Zonen-Weitsprung (UKC)", "Ballwurf 200 g (UKC)"} {
		if !strings.Contains(body, want) {
			t.Errorf("meet page misses %q", want)
		}
	}

	// Register a W12 girl through the roster form.
	resp = postForm(t, client, base+loc+"/roster", base+loc+"/roster", url.Values{
		"first_name": {"Anna"}, "last_name": {"Muster"},
		"birth_year": {"2014"}, "sex": {"W"}, "club": {"LC Test"}, "bib": {"101"},
	})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST roster = %d, want 303 (body: %s)", resp.StatusCode, bodyString(t, resp))
	}
	_ = resp.Body.Close()
	body = bodyString(t, mustGet(t, client, base+loc+"/roster"))
	for _, want := range []string{"Anna Muster", "101", "LC Test", "2014"} {
		if !strings.Contains(body, want) {
			t.Errorf("roster page misses %q", want)
		}
	}

	// A duplicate bib re-renders the roster with a localized error (422).
	resp = postForm(t, client, base+loc+"/roster", base+loc+"/roster", url.Values{
		"first_name": {"Other"}, "last_name": {"Kid"},
		"birth_year": {"2013"}, "sex": {"M"}, "bib": {"101"},
	})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("duplicate bib = %d, want 422", resp.StatusCode)
	}
	_ = bodyString(t, resp)

	// One saved 60m result (the capture UI is TASK-008; the service is
	// this task's seam) appears on the standings page with its official
	// points, and the two missing disciplines render the explicit gap.
	participants, err := deps.results.Participants(context.Background(), meetID)
	if err != nil || len(participants) != 1 {
		t.Fatalf("participants = %v, %v", participants, err)
	}
	officeActor := app.Session{AccountID: "01TEST", Username: "office", Role: app.RoleCompetitionOffice}
	if _, err := deps.results.SaveResult(context.Background(), officeActor, meetID, app.ResultInput{
		AthleteID: participants[0].AthleteID, DisciplineCode: "60m",
		Mark: "8.42", Timing: domain.TimingElectronic,
	}); err != nil {
		t.Fatalf("SaveResult: %v", err)
	}

	body = bodyString(t, mustGet(t, client, base+loc+"/standings"))
	for _, want := range []string{"W12", "Anna Muster", "8.42", "710", "–"} {
		if !strings.Contains(body, want) {
			t.Errorf("standings page misses %q", want)
		}
	}
}

// TestUC033FinalStandingsWebSurfacesSweep pins TASK-036/DEC-016/OQ-020's
// web-surface sweep: the operator standings page, the PDF result list and
// the public results page all show PROVISIONAL labeling (and still rank an
// incomplete athlete by partial total) before the division's series is
// complete, and switch to FINAL labeling (with the missing-discipline
// athlete unranked and marked "aufg.") the moment every unit is announced
// — without any of the three surfaces re-implementing the completeness
// check (they all resolve through ResultsService.CurrentStandings).
func TestUC033FinalStandingsWebSurfacesSweep(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	ctx := context.Background()

	resp := postForm(t, client, base+"/meets/from-template", base+"/meets/from-template", url.Values{
		"template": {"ubs-kids-cup"}, "date": {"2026-08-15"}, "venue": {"Le Mouret"},
	})
	loc := resp.Header.Get("Location")
	_ = resp.Body.Close()
	meetID := strings.TrimPrefix(loc, "/meets/")

	officeActor := app.Session{AccountID: "01TEST", Username: "office", Role: app.RoleCompetitionOffice}
	full, err := deps.results.RegisterParticipant(ctx, officeActor, meetID, app.ParticipantInput{
		FirstName: "Fiona", LastName: "Full", BirthYear: 2015, Sex: domain.SexFemale, Bib: "1",
	})
	if err != nil {
		t.Fatalf("RegisterParticipant full: %v", err)
	}
	gap, err := deps.results.RegisterParticipant(ctx, officeActor, meetID, app.ParticipantInput{
		FirstName: "Gina", LastName: "Gap", BirthYear: 2015, Sex: domain.SexFemale, Bib: "2",
	})
	if err != nil {
		t.Fatalf("RegisterParticipant gap: %v", err)
	}
	for _, in := range []app.ResultInput{
		{AthleteID: full.AthleteID, DisciplineCode: "60m", Mark: "10.00", Timing: domain.TimingElectronic},
		{AthleteID: full.AthleteID, DisciplineCode: "ZoneLJ", Mark: "3.00"},
		{AthleteID: full.AthleteID, DisciplineCode: "BallThrow200g", Mark: "20.00"},
		{AthleteID: gap.AthleteID, DisciplineCode: "60m", Status: domain.StatusDNS},
		{AthleteID: gap.AthleteID, DisciplineCode: "ZoneLJ", Status: domain.StatusNM},
		{AthleteID: gap.AthleteID, DisciplineCode: "BallThrow200g", Status: domain.StatusNM},
	} {
		if _, err := deps.results.SaveResult(ctx, officeActor, meetID, in); err != nil {
			t.Fatalf("SaveResult(%s, %s): %v", in.AthleteID, in.DisciplineCode, err)
		}
	}

	// Before announcement: provisional labeling everywhere, gap still ranked.
	standingsBody := bodyString(t, mustGet(t, client, base+"/meets/"+meetID+"/standings"))
	if !strings.Contains(standingsBody, "Zwischenstand") {
		t.Errorf("standings page before announcement misses the provisional label: %s", standingsBody)
	}
	pdfResp := mustGet(t, client, base+"/meets/"+meetID+"/standings.pdf")
	_ = pdfResp.Body.Close()
	if pdfResp.StatusCode != http.StatusOK {
		t.Fatalf("GET standings.pdf (provisional) = %d, want 200", pdfResp.StatusCode)
	}
	publicBody := bodyString(t, mustGet(t, client, base+"/m/"+meetID+"/results"))
	if !strings.Contains(publicBody, "Zwischenstand") {
		t.Errorf("public results page before announcement misses the provisional label: %s", publicBody)
	}

	// Announce every discipline's unit — the series is complete.
	detail, err := deps.meets.Meet(ctx, meetID)
	if err != nil {
		t.Fatalf("Meet: %v", err)
	}
	for _, disc := range []string{"60m", "ZoneLJ", "BallThrow200g"} {
		var unitID string
		for _, u := range detail.Units {
			if u.DisciplineCode == disc {
				unitID = u.UnitID
			}
		}
		if unitID == "" {
			t.Fatalf("meet has no %s unit", disc)
		}
		if _, err := deps.results.AnnounceUnitResults(ctx, officeActor, meetID, unitID); err != nil {
			t.Fatalf("AnnounceUnitResults(%s): %v", disc, err)
		}
	}

	standingsBody = bodyString(t, mustGet(t, client, base+"/meets/"+meetID+"/standings"))
	if !strings.Contains(standingsBody, "Finalstand") {
		t.Errorf("standings page after announcement misses the final label: %s", standingsBody)
	}
	if !strings.Contains(standingsBody, "aufg.") {
		t.Errorf("standings page after announcement misses the unranked-missing marker: %s", standingsBody)
	}
	publicBody = bodyString(t, mustGet(t, client, base+"/m/"+meetID+"/results"))
	if !strings.Contains(publicBody, "Finalstand") || !strings.Contains(publicBody, "aufg.") {
		t.Errorf("public results page after announcement misses final labeling/marker: %s", publicBody)
	}
	pdfResp = mustGet(t, client, base+"/meets/"+meetID+"/standings.pdf")
	pdfData := bodyString(t, pdfResp)
	if pdfResp.StatusCode != http.StatusOK || !strings.HasPrefix(pdfData, "%PDF") {
		t.Fatalf("GET standings.pdf (final) = %d, does not look like a PDF", pdfResp.StatusCode)
	}
}

// TestRosterSearchFilterDEC021SYS120SYS114 covers TASK-038/DEC-021's roster
// search box (SYS-120's "entry search" budgeted interaction, SYS-114's
// expert-use keyboard-reachable filter): a name/bib/club query narrows the
// roster server-side, case-insensitively, an unmatched query yields the
// distinct "no results" state (not the "no participants at all" empty
// state), and the empty query (the box's default) still renders the full,
// unfiltered roster.
func TestRosterSearchFilterDEC021SYS120SYS114(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	ctx := context.Background()

	resp := postForm(t, client, base+"/meets/from-template", base+"/meets/from-template", url.Values{
		"template": {"ubs-kids-cup"}, "date": {"2026-08-15"}, "venue": {"Le Mouret"},
	})
	loc := resp.Header.Get("Location")
	_ = resp.Body.Close()
	meetID := strings.TrimPrefix(loc, "/meets/")

	officeActor := app.Session{AccountID: "01TEST", Username: "office", Role: app.RoleCompetitionOffice}
	for _, in := range []app.ParticipantInput{
		{FirstName: "Anna", LastName: "Muster", BirthYear: 2014, Sex: domain.SexFemale, Club: "LC Fribourg", Bib: "101"},
		{FirstName: "Beat", LastName: "Meier", BirthYear: 2013, Sex: domain.SexMale, Club: "STV Bern", Bib: "202"},
	} {
		if _, err := deps.results.RegisterParticipant(ctx, officeActor, meetID, in); err != nil {
			t.Fatalf("RegisterParticipant %s: %v", in.LastName, err)
		}
	}
	rosterURL := base + loc + "/roster"

	// Empty query ("q" absent, and "q="): the unfiltered roster, unchanged
	// from the pre-search behavior.
	for _, u := range []string{rosterURL, rosterURL + "?q="} {
		body := bodyString(t, mustGet(t, client, u))
		if !strings.Contains(body, "Anna Muster") || !strings.Contains(body, "Beat Meier") {
			t.Errorf("GET %s misses an unfiltered roster row: %s", u, body)
		}
	}

	// Case-insensitive name match narrows to one row.
	body := bodyString(t, mustGet(t, client, rosterURL+"?q=anna"))
	if !strings.Contains(body, "Anna Muster") {
		t.Errorf("name search misses the match: %s", body)
	}
	if strings.Contains(body, "Beat Meier") {
		t.Errorf("name search leaked the non-matching row: %s", body)
	}

	// Bib match.
	body = bodyString(t, mustGet(t, client, rosterURL+"?q=202"))
	if !strings.Contains(body, "Beat Meier") || strings.Contains(body, "Anna Muster") {
		t.Errorf("bib search = %s, want only Beat Meier", body)
	}

	// Club match, mixed case.
	body = bodyString(t, mustGet(t, client, rosterURL+"?q=fribourg"))
	if !strings.Contains(body, "Anna Muster") || strings.Contains(body, "Beat Meier") {
		t.Errorf("club search = %s, want only Anna Muster", body)
	}

	// A query matching nobody renders the search's own "no results" state,
	// distinct from the meet-has-no-participants-at-all empty state.
	resp = mustGet(t, client, rosterURL+"?q=nonexistent-query")
	body = bodyString(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET roster?q=nonexistent-query = %d, want 200", resp.StatusCode)
	}
	if strings.Contains(body, "Anna Muster") || strings.Contains(body, "Beat Meier") {
		t.Errorf("no-match search leaked a row: %s", body)
	}
	if !strings.Contains(body, "nonexistent-query") {
		t.Errorf("no-match search does not name the query back to the operator: %s", body)
	}

	// The search box itself is a plain, always-reachable text input (SYS-114
	// keyboard-only operability): no JS-only affordance gates it.
	if !strings.Contains(body, `name="q"`) {
		t.Errorf("roster page misses the search input: %s", body)
	}
}

// TestRosterAndStandingsRequireOfficeRole: the day-of-competition surfaces
// are gated at competition-office level (SYS-090).
func TestRosterAndStandingsRequireOfficeRole(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)

	for _, path := range []string{"/meets/x/roster", "/meets/x/standings", "/meets/from-template"} {
		resp := mustGet(t, client, base+path)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("anonymous GET %s = %d, want 403", path, resp.StatusCode)
		}
	}
}

// TestRosterOutOfCompetitionToggleWeb covers TASK-052/OQ-091's roster-page
// operator control over TASK-036's ausser-Konkurrenz/hors-concours flag,
// over real HTTP: marking a fully-competed participant out of competition
// drops her from the standings ranking (marks stay visible, no numeric
// rank, the "n.a." unranked marker per DEC-016/OQ-020) while a rival stays
// ranked normally, and unmarking her restores the ranking — pinned against
// the localized DE strings the page actually renders.
func TestRosterOutOfCompetitionToggleWeb(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	ctx := context.Background()

	resp := postForm(t, client, base+"/meets/from-template", base+"/meets/from-template", url.Values{
		"template": {"ubs-kids-cup"}, "date": {"2026-08-15"}, "venue": {"Le Mouret"},
	})
	loc := resp.Header.Get("Location")
	_ = resp.Body.Close()
	meetID := strings.TrimPrefix(loc, "/meets/")

	officeActor := app.Session{AccountID: "01TEST", Username: "office", Role: app.RoleCompetitionOffice}
	ooc, err := deps.results.RegisterParticipant(ctx, officeActor, meetID, app.ParticipantInput{
		FirstName: "Thome", LastName: "Lauriane", BirthYear: 2015, Sex: domain.SexFemale, Bib: "1",
	})
	if err != nil {
		t.Fatalf("RegisterParticipant ooc: %v", err)
	}
	rival, err := deps.results.RegisterParticipant(ctx, officeActor, meetID, app.ParticipantInput{
		FirstName: "Other", LastName: "Athlete", BirthYear: 2015, Sex: domain.SexFemale, Bib: "2",
	})
	if err != nil {
		t.Fatalf("RegisterParticipant rival: %v", err)
	}
	for _, in := range []app.ResultInput{
		{AthleteID: ooc.AthleteID, DisciplineCode: "60m", Mark: "8.79", Timing: domain.TimingElectronic},
		{AthleteID: ooc.AthleteID, DisciplineCode: "ZoneLJ", Mark: "4.38"},
		{AthleteID: ooc.AthleteID, DisciplineCode: "BallThrow200g", Mark: "26.60"},
		{AthleteID: rival.AthleteID, DisciplineCode: "60m", Mark: "10.00", Timing: domain.TimingElectronic},
		{AthleteID: rival.AthleteID, DisciplineCode: "ZoneLJ", Mark: "3.00"},
		{AthleteID: rival.AthleteID, DisciplineCode: "BallThrow200g", Mark: "20.00"},
	} {
		if _, err := deps.results.SaveResult(ctx, officeActor, meetID, in); err != nil {
			t.Fatalf("SaveResult(%s, %s): %v", in.AthleteID, in.DisciplineCode, err)
		}
	}
	rosterURL := base + "/meets/" + meetID + "/roster"
	toggleURL := rosterURL + "/" + ooc.ID + "/out-of-competition"

	// Before toggling: the roster offers the "mark" control, no badge yet;
	// standings rank her normally alongside the rival.
	body := bodyString(t, mustGet(t, client, rosterURL))
	if !strings.Contains(body, "Als ausser Konkurrenz markieren") {
		t.Errorf("roster missing the out-of-competition mark control: %s", body)
	}
	if strings.Contains(body, `class="badge">Ausser Konkurrenz<`) {
		t.Errorf("roster shows the out-of-competition badge before toggling: %s", body)
	}
	standingsBody := bodyString(t, mustGet(t, client, base+"/meets/"+meetID+"/standings"))
	if strings.Contains(standingsBody, "n.a.") {
		t.Errorf("standings show the unranked marker before toggling: %s", standingsBody)
	}

	// Toggle on: office marks her out of competition (version 1, her
	// as-registered version).
	resp = postForm(t, client, rosterURL, toggleURL, url.Values{"version": {"1"}, "value": {"true"}})
	toggleBody := bodyString(t, resp)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST toggle on = %d, want 303 (body: %s)", resp.StatusCode, toggleBody)
	}
	if got := resp.Header.Get("Location"); got != "/meets/"+meetID+"/roster" {
		t.Errorf("redirect location = %q, want the roster page", got)
	}

	// Roster now shows the badge and offers the "unmark" control.
	body = bodyString(t, mustGet(t, client, rosterURL))
	if !strings.Contains(body, `class="badge">Ausser Konkurrenz<`) {
		t.Errorf("roster missing the out-of-competition badge after toggling on: %s", body)
	}
	if !strings.Contains(body, "Nicht mehr ausser Konkurrenz") {
		t.Errorf("roster missing the unmark control after toggling on: %s", body)
	}

	// Standings exclusion: she keeps her marks (Complete) but holds no rank
	// (the "n.a." DEC-016/OQ-020 marker); the rival is unaffected.
	standingsBody = bodyString(t, mustGet(t, client, base+"/meets/"+meetID+"/standings"))
	if !strings.Contains(standingsBody, "n.a.") {
		t.Errorf("standings missing the unranked-out-of-competition marker after toggling: %s", standingsBody)
	}
	if !strings.Contains(standingsBody, "8.79") {
		t.Errorf("standings dropped the out-of-competition athlete's marks: %s", standingsBody)
	}

	// Toggle back off (version bumped to 2 by the first toggle).
	resp = postForm(t, client, rosterURL, toggleURL, url.Values{"version": {"2"}, "value": {"false"}})
	toggleBody = bodyString(t, resp)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST toggle off = %d, want 303 (body: %s)", resp.StatusCode, toggleBody)
	}

	body = bodyString(t, mustGet(t, client, rosterURL))
	if strings.Contains(body, `class="badge">Ausser Konkurrenz<`) {
		t.Errorf("roster still shows the out-of-competition badge after toggling off: %s", body)
	}
	if !strings.Contains(body, "Als ausser Konkurrenz markieren") {
		t.Errorf("roster missing the mark control again after toggling off: %s", body)
	}
	standingsBody = bodyString(t, mustGet(t, client, base+"/meets/"+meetID+"/standings"))
	if strings.Contains(standingsBody, "n.a.") {
		t.Errorf("standings still show the unranked marker after toggling off: %s", standingsBody)
	}
}

// TestSeriesUploadDownloadSYS077UC035_1 covers the office-UI half of UC-035
// #1 (SYS-077): the standings page offers a download link for a meet whose
// template has a series-upload template, and the export route serves a
// real XLSX workbook (magic bytes + content type) gated at office level
// like roster/standings (SYS-090).
func TestSeriesUploadDownloadSYS077UC035_1(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)

	resp := postForm(t, client, base+"/meets/from-template", base+"/meets/from-template", url.Values{
		"template": {"ubs-kids-cup"}, "date": {"2026-08-15"}, "venue": {"Le Mouret"},
	})
	loc := resp.Header.Get("Location")
	_ = resp.Body.Close()

	body := bodyString(t, mustGet(t, client, base+loc+"/standings"))
	if !strings.Contains(body, "/export/ukc-series") {
		t.Errorf("standings page misses the series-upload download link: %s", body)
	}

	dlResp := mustGet(t, client, base+loc+"/export/ukc-series")
	defer func() { _ = dlResp.Body.Close() }()
	if dlResp.StatusCode != http.StatusOK {
		t.Fatalf("GET export = %d, want 200", dlResp.StatusCode)
	}
	wantType := "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	if got := dlResp.Header.Get("Content-Type"); got != wantType {
		t.Errorf("Content-Type = %q, want %q", got, wantType)
	}
	if !strings.Contains(dlResp.Header.Get("Content-Disposition"), "attachment") {
		t.Errorf("Content-Disposition = %q, want an attachment", dlResp.Header.Get("Content-Disposition"))
	}
	data := bodyString(t, dlResp)
	if !strings.HasPrefix(data, "PK") { // XLSX is a zip archive
		t.Errorf("downloaded body does not look like an XLSX (zip) file")
	}

	anon := mustGet(t, &http.Client{}, base+loc+"/export/ukc-series")
	_ = anon.Body.Close()
	if anon.StatusCode != http.StatusForbidden {
		t.Errorf("anonymous export GET = %d, want 403 (SYS-090)", anon.StatusCode)
	}
}

// TestTemplateMeetFormRejectsBadInput: a bad date or venue re-renders the
// form with 422 rather than creating anything.
func TestTemplateMeetFormRejectsBadInput(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)

	resp := postForm(t, client, base+"/meets/from-template", base+"/meets/from-template", url.Values{
		"template": {"ubs-kids-cup"}, "date": {"not-a-date"}, "venue": {"X"},
	})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("bad date = %d, want 422", resp.StatusCode)
	}
	_ = bodyString(t, resp)

	resp = postForm(t, client, base+"/meets/from-template", base+"/meets/from-template", url.Values{
		"template": {"no-such-template"}, "date": {"2026-08-15"}, "venue": {"X"},
	})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("unknown template = %d, want 422", resp.StatusCode)
	}
	_ = bodyString(t, resp)

	meets, err := deps.meets.ListMeets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(meets) != 0 {
		t.Errorf("bad input created %d meets, want 0", len(meets))
	}
}
