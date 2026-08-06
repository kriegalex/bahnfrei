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

// createUKCMeetWeb drives the quick-create template flow (mirrors
// TestUKCTemplateRosterStandingsFlow's precedent): the UBS Kids Cup
// per-birth-year category scheme is what proves UC-043 #1's category
// re-derivation (W12 → W13 on a birth-year correction).
func createUKCMeetWeb(t *testing.T, client *http.Client, base string) (meetID string) {
	t.Helper()
	resp := postForm(t, client, base+"/meets/from-template", base+"/meets/from-template", url.Values{
		"template": {"ubs-kids-cup"}, "date": {"2026-08-15"}, "venue": {"Le Mouret"},
	})
	loc := resp.Header.Get("Location")
	_ = resp.Body.Close()
	return strings.TrimPrefix(loc, "/meets/")
}

// TestParticipantEditFormShowsCurrentValuesSYS150UC043Web: the GET edit
// form pre-fills every field with the participant's current values (UC-043:
// "the form must show current values").
func TestParticipantEditFormShowsCurrentValuesSYS150UC043Web(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	ctx := context.Background()
	meetID := createUKCMeetWeb(t, client, base)

	officeActor := app.Session{AccountID: "01TEST", Username: "office", Role: app.RoleCompetitionOffice}
	anna, err := deps.results.RegisterParticipant(ctx, officeActor, meetID, app.ParticipantInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2014, Sex: domain.SexFemale, Club: "LC Test", Bib: "101",
	})
	if err != nil {
		t.Fatalf("RegisterParticipant: %v", err)
	}

	body := bodyString(t, mustGet(t, client, base+"/meets/"+meetID+"/roster/"+anna.ID+"/edit"))
	for _, want := range []string{
		`value="Anna"`, `value="Muster"`, `value="2014"`, `value="LC Test"`, `value="101"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("edit form missing %q: %s", want, body)
		}
	}
	if !strings.Contains(body, `name="version" value="1"`) {
		t.Errorf("edit form missing the hidden version field: %s", body)
	}
	// The roster page itself offers the edit link.
	rosterBody := bodyString(t, mustGet(t, client, base+"/meets/"+meetID+"/roster"))
	if !strings.Contains(rosterBody, "/meets/"+meetID+"/roster/"+anna.ID+"/edit") {
		t.Errorf("roster page missing the edit link for %s: %s", anna.ID, rosterBody)
	}
}

// TestParticipantEditFlowSYS150UC043Web drives the full UC-043 #1 happy
// path over real HTTP: correcting a name, club and birth year takes effect
// on the roster, the standings division (category re-derivation, W12 →
// W13) and the public results page — all from the one edit form submit.
func TestParticipantEditFlowSYS150UC043Web(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	ctx := context.Background()
	meetID := createUKCMeetWeb(t, client, base)

	officeActor := app.Session{AccountID: "01TEST", Username: "office", Role: app.RoleCompetitionOffice}
	anna, err := deps.results.RegisterParticipant(ctx, officeActor, meetID, app.ParticipantInput{
		FirstName: "Ana", LastName: "Musterr", BirthYear: 2014, Sex: domain.SexFemale, Club: "LC Typo", Bib: "101",
	})
	if err != nil {
		t.Fatalf("RegisterParticipant: %v", err)
	}

	// Sanity: before correction, standings show her in W12 under the typo'd
	// name.
	standingsBody := bodyString(t, mustGet(t, client, base+"/meets/"+meetID+"/standings"))
	if !strings.Contains(standingsBody, "Musterr") || !strings.Contains(standingsBody, "W12") {
		t.Fatalf("pre-correction standings missing the fixture: %s", standingsBody)
	}

	editURL := base + "/meets/" + meetID + "/roster/" + anna.ID + "/edit"
	resp := postForm(t, client, editURL, editURL, url.Values{
		"first_name": {"Anna"}, "last_name": {"Muster"}, "birth_year": {"2013"},
		"sex": {"W"}, "club": {"LC Test"}, "bib": {"101"}, "reason": {"typo + wrong club/year at registration"},
		"version": {"1"},
	})
	editBody := bodyString(t, resp)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST edit = %d, want 303 (body: %s)", resp.StatusCode, editBody)
	}
	if got := resp.Header.Get("Location"); got != "/meets/"+meetID+"/roster" {
		t.Errorf("redirect location = %q, want the roster page", got)
	}

	// Roster.
	rosterBody := bodyString(t, mustGet(t, client, base+"/meets/"+meetID+"/roster"))
	if !strings.Contains(rosterBody, "Anna Muster") || !strings.Contains(rosterBody, "LC Test") {
		t.Errorf("roster after correction missing the corrected name/club: %s", rosterBody)
	}
	if strings.Contains(rosterBody, "Musterr") {
		t.Errorf("roster after correction still shows the old (typo'd) name: %s", rosterBody)
	}

	// Standings: category re-derivation moved her from W12 to W13.
	standingsBody = bodyString(t, mustGet(t, client, base+"/meets/"+meetID+"/standings"))
	if !strings.Contains(standingsBody, "W13") {
		t.Errorf("standings after birth-year correction missing the new W13 division: %s", standingsBody)
	}
	w12Idx := strings.Index(standingsBody, "<h2>W12</h2>")
	w13Idx := strings.Index(standingsBody, "<h2>W13</h2>")
	if w12Idx >= 0 && w13Idx > w12Idx {
		w12Section := standingsBody[w12Idx:w13Idx]
		if strings.Contains(w12Section, "101") {
			t.Errorf("bib 101 still listed under W12 after the birth-year correction: %s", w12Section)
		}
	}

	// Public results page (propagation across surfaces).
	publicBody := bodyString(t, mustGet(t, client, base+"/m/"+meetID+"/results"))
	if !strings.Contains(publicBody, "Anna Muster") {
		t.Errorf("public results page missing the corrected name: %s", publicBody)
	}
}

// TestParticipantEditInvalidSubmitFieldErrorsSYS150UC043Web: an invalid
// submit (blank last name) re-renders the form with inline field errors and
// the submitted values preserved (OQ-075/UC-038 #4), not a redirect.
func TestParticipantEditInvalidSubmitFieldErrorsSYS150UC043Web(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	ctx := context.Background()
	meetID := createUKCMeetWeb(t, client, base)

	officeActor := app.Session{AccountID: "01TEST", Username: "office", Role: app.RoleCompetitionOffice}
	anna, err := deps.results.RegisterParticipant(ctx, officeActor, meetID, app.ParticipantInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2014, Sex: domain.SexFemale, Bib: "101",
	})
	if err != nil {
		t.Fatalf("RegisterParticipant: %v", err)
	}

	editURL := base + "/meets/" + meetID + "/roster/" + anna.ID + "/edit"
	resp := postForm(t, client, editURL, editURL, url.Values{
		"first_name": {"Keeps"}, "last_name": {""}, "birth_year": {"2014"}, "sex": {"W"}, "bib": {"101"},
	})
	body := bodyString(t, resp)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("blank last name POST = %d, want 422: %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, `id="last_name-error"`) || !strings.Contains(body, `aria-invalid="true"`) {
		t.Errorf("invalid submit missing the inline field error: %s", body)
	}
	if !strings.Contains(body, `value="Keeps"`) {
		t.Errorf("invalid submit did not preserve the submitted first name: %s", body)
	}

	// The rejected submit must not have written anything.
	rosterBody := bodyString(t, mustGet(t, client, base+"/meets/"+meetID+"/roster"))
	if !strings.Contains(rosterBody, "Anna Muster") {
		t.Errorf("roster changed despite the rejected submit: %s", rosterBody)
	}
}

// TestParticipantEditConflictSYS150UC043Web: a stale version renders the
// standard conflict alert with reload guidance (mirrors meetFormPage's
// ErrConflict precedent), status 409.
func TestParticipantEditConflictSYS150UC043Web(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	ctx := context.Background()
	meetID := createUKCMeetWeb(t, client, base)

	officeActor := app.Session{AccountID: "01TEST", Username: "office", Role: app.RoleCompetitionOffice}
	anna, err := deps.results.RegisterParticipant(ctx, officeActor, meetID, app.ParticipantInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2014, Sex: domain.SexFemale, Bib: "101",
	})
	if err != nil {
		t.Fatalf("RegisterParticipant: %v", err)
	}
	// A concurrent edit lands first (through the app layer directly),
	// bumping the participant's version past what the browser's form still
	// holds.
	if _, err := deps.results.UpdateParticipantIdentity(ctx, officeActor, meetID, anna.ID, anna.Version, app.ParticipantIdentityInput{
		FirstName: "Anna", LastName: "Concurrent", BirthYear: 2014, Sex: domain.SexFemale, Bib: "101",
	}); err != nil {
		t.Fatalf("concurrent UpdateParticipantIdentity: %v", err)
	}

	editURL := base + "/meets/" + meetID + "/roster/" + anna.ID + "/edit"
	resp := postForm(t, client, editURL, editURL, url.Values{
		"first_name": {"Anna"}, "last_name": {"Stale"}, "birth_year": {"2014"},
		"sex": {"W"}, "bib": {"101"}, "version": {"1"}, // stale: the concurrent edit already moved it to 2
	})
	body := bodyString(t, resp)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("stale-version POST = %d, want 409: %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, "zwischenzeitlich geändert") {
		t.Errorf("conflict response missing the standard reload-guidance alert: %s", body)
	}

	rosterBody := bodyString(t, mustGet(t, client, base+"/meets/"+meetID+"/roster"))
	if !strings.Contains(rosterBody, "Anna Concurrent") || strings.Contains(rosterBody, "Anna Stale") {
		t.Errorf("roster after rejected conflict = %s, want only the concurrent edit to have won", rosterBody)
	}
}

// TestParticipantEditRequiresOfficeRoleSYS150UC043Web: field officials and
// entry submitters — logged-in accounts below competition-office level —
// are refused (SYS-090), matching every other roster mutation.
func TestParticipantEditRequiresOfficeRoleSYS150UC043Web(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	ctx := context.Background()
	meetID := createUKCMeetWeb(t, client, base)

	officeActor := app.Session{AccountID: "01TEST", Username: "office", Role: app.RoleCompetitionOffice}
	anna, err := deps.results.RegisterParticipant(ctx, officeActor, meetID, app.ParticipantInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2014, Sex: domain.SexFemale, Bib: "101",
	})
	if err != nil {
		t.Fatalf("RegisterParticipant: %v", err)
	}
	editURL := base + "/meets/" + meetID + "/roster/" + anna.ID + "/edit"

	for _, role := range []string{"field_official", "entry_submitter"} {
		createAccountWeb(t, client, base, "u-"+role, role)
		logout(t, client, base)
		login(t, client, base, "u-"+role, "s3cret-passphrase")

		resp := mustGet(t, client, editURL)
		_ = bodyString(t, resp)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("GET edit form as %s = %d, want 403", role, resp.StatusCode)
		}

		logout(t, client, base)
		login(t, client, base, "admin", "s3cret-passphrase")
	}
}

// TestParticipantEditErasedNotEditableSYS150UC043Web: an erased participant
// has no edit link on the roster page, and the edit route itself 404s
// rather than exposing a form that would error on submit.
func TestParticipantEditErasedNotEditableSYS150UC043Web(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	ctx := context.Background()
	meetID := createUKCMeetWeb(t, client, base)

	officeActor := app.Session{AccountID: "01TEST", Username: "office", Role: app.RoleCompetitionOffice}
	anna, err := deps.results.RegisterParticipant(ctx, officeActor, meetID, app.ParticipantInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2014, Sex: domain.SexFemale, Bib: "101",
	})
	if err != nil {
		t.Fatalf("RegisterParticipant: %v", err)
	}
	if err := deps.privacy.EraseAthlete(ctx, officeActor, anna.AthleteID, "subject request"); err != nil {
		t.Fatalf("EraseAthlete: %v", err)
	}

	rosterBody := bodyString(t, mustGet(t, client, base+"/meets/"+meetID+"/roster"))
	if strings.Contains(rosterBody, "/meets/"+meetID+"/roster/"+anna.ID+"/edit") {
		t.Errorf("roster page still offers an edit link for an erased participant: %s", rosterBody)
	}

	resp := mustGet(t, client, base+"/meets/"+meetID+"/roster/"+anna.ID+"/edit")
	_ = bodyString(t, resp)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET edit form for an erased participant = %d, want 404", resp.StatusCode)
	}
}
