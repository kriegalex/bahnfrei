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
	for _, want := range []string{"UBS Kids Cup Le Mouret 2026", "60 metres", "Zone Long Jump (UKC)", "200 g Ball Throw (UKC)"} {
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
