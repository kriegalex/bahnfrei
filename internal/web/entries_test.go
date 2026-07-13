// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/kriegalex/bahnfrei/internal/app"
)

// publishMeetWeb drives the publish action on a freshly created meet — its
// version is always 1 at that point (AddEvent never touches the meets row).
func publishMeetWeb(t *testing.T, client *http.Client, base, meetID string) {
	t.Helper()
	page := base + "/meets/" + meetID
	resp := postForm(t, client, page, page+"/publish", url.Values{"version": {"1"}})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("publish meet = %d, want 303", resp.StatusCode)
	}
}

// createAccountWeb provisions an account of the given role through the
// admin UI (the actor must already be logged in as an instance admin).
func createAccountWeb(t *testing.T, client *http.Client, base, username, role string) {
	t.Helper()
	resp := postForm(t, client, base+"/admin", base+"/admin/accounts", url.Values{
		"username": {username}, "display_name": {username}, "password": {"s3cret-passphrase"}, "role": {role},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create account %q = %d, want 303", username, resp.StatusCode)
	}
}

// TestEntriesBibsFeesRequireRoleSYS090: anonymous requests to every TASK-016
// surface are refused (SYS-090 least privilege) — mirrors the coarse-gate
// pattern already proven for /meets, /admin, /audit.
func TestEntriesBibsFeesRequireRoleSYS090(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)

	for _, path := range []string{
		"/meets/x/entries", "/meets/x/entries/exceptions", "/meets/x/bibs", "/meets/x/bibs.pdf", "/meets/x/fees",
	} {
		resp := mustGet(t, client, base+path)
		_ = bodyString(t, resp)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("anonymous GET %s = %d, want 403 (SYS-090)", path, resp.StatusCode)
		}
	}
}

// entryFlowFixture creates a published meet with one open 100m/U16 W event
// and one closed (deadline-passed) 200m/U16 W event, plus an entry-submitter
// account "sub1", over real HTTP. mustEventID resolves either event's raw
// ID through the app layer directly — the closed event never appears in any
// rendered page, so HTML scraping cannot recover it.
func entryFlowFixture(t *testing.T, deps *testServerDeps, client *http.Client, base string) (meetID string, closedEventID string) {
	t.Helper()
	setupAndLogin(t, client, base)
	meetID = createUCMeet(t, client, base)
	resp := addEvent(t, client, base, meetID, url.Values{
		"discipline": {"100m"}, "categories": {"U16 W"}, "round_final": {"1"},
	})
	_ = resp.Body.Close()
	resp = addEvent(t, client, base, meetID, url.Values{
		"discipline": {"200m"}, "categories": {"U16 W"}, "round_final": {"1"},
		"entry_deadline": {"2020-01-01T00:00"},
	})
	_ = resp.Body.Close()
	publishMeetWeb(t, client, base, meetID)
	createAccountWeb(t, client, base, "sub1", "entry_submitter")

	closedEventID = mustEventID(t, deps, meetID, "200m")
	return meetID, closedEventID
}

// mustEventID resolves a discipline code's event ID by fetching the meet
// detail directly through the app layer — the entries form itself only
// exposes labels (and only for open events), never a scrapeable raw ID for
// a closed one.
func mustEventID(t *testing.T, deps *testServerDeps, meetID, disciplineCode string) string {
	t.Helper()
	detail, err := deps.meets.Meet(context.Background(), meetID)
	if err != nil {
		t.Fatalf("Meet: %v", err)
	}
	for _, pe := range detail.Programme {
		if pe.DisciplineCode == disciplineCode {
			return pe.ID
		}
	}
	t.Fatalf("no event for discipline %q", disciplineCode)
	return ""
}

// TestOnlineEntryIndividualFlowSYS011UC003_1Web drives UC-003 #1 over real
// HTTP: an entry-submitter account submits an individual entry and sees it
// on their own entries page with status "entered".
func TestOnlineEntryIndividualFlowSYS011UC003_1Web(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	meetID, _ := entryFlowFixture(t, deps, client, base)
	eventID := mustEventID(t, deps, meetID, "100m")

	logout(t, client, base)
	login(t, client, base, "sub1", "s3cret-passphrase")

	page := base + "/meets/" + meetID + "/entries"
	body := bodyString(t, mustGet(t, client, page))
	if !strings.Contains(body, "100 metres") {
		t.Errorf("entries page must offer the open 100m event: %s", body)
	}

	resp := postForm(t, client, page, base+"/meets/"+meetID+"/entries/individual", url.Values{
		"event": {eventID}, "first_name": {"Anna"}, "last_name": {"Muster"},
		"birth_year": {"2011"}, "sex": {"W"}, "club": {"LC Test"}, "seed": {"13.50"},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("submit individual entry = %d, want 303", resp.StatusCode)
	}

	body = bodyString(t, mustGet(t, client, page))
	for _, want := range []string{"Anna Muster", "LC Test", "13.50"} {
		if !strings.Contains(body, want) {
			t.Errorf("entries page missing %q: %s", want, body)
		}
	}
}

// TestOnlineEntryDeadlinePassedRejectedWebSYS011UC003_3 covers UC-003 #3's
// "direct request forgery" case over real HTTP: a POST naming an event ID
// the submission form never offered (its deadline has passed) is rejected
// server-side, with no entry created.
func TestOnlineEntryDeadlinePassedRejectedWebSYS011UC003_3(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	meetID, closedEventID := entryFlowFixture(t, deps, client, base)

	logout(t, client, base)
	login(t, client, base, "sub1", "s3cret-passphrase")

	page := base + "/meets/" + meetID + "/entries"
	resp := postForm(t, client, page, base+"/meets/"+meetID+"/entries/individual", url.Values{
		"event": {closedEventID}, "first_name": {"Anna"}, "last_name": {"Muster"},
		"birth_year": {"2011"}, "sex": {"W"}, "seed": {"27.00"},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("forged entry POST = %d, want 303 (redirect with flash)", resp.StatusCode)
	}
	if !strings.Contains(resp.Header.Get("Location"), "err=") {
		t.Errorf("redirect location = %q, want an err= flash", resp.Header.Get("Location"))
	}

	body := bodyString(t, mustGet(t, client, page))
	if strings.Contains(body, "Anna Muster") {
		t.Error("no entry should have been created for the deadline-passed event")
	}
}

// TestBibAssignmentFlowSYS018UC006_1_2Web drives UC-006 #1/#2 over real
// HTTP: bulk bib assignment by club produces unique bibs, and a manual
// duplicate assignment is rejected.
func TestBibAssignmentFlowSYS018UC006_1_2Web(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	meetID, _ := entryFlowFixture(t, deps, client, base)
	eventID := mustEventID(t, deps, meetID, "100m")

	logout(t, client, base)
	login(t, client, base, "sub1", "s3cret-passphrase")
	entriesPage := base + "/meets/" + meetID + "/entries"
	for _, name := range []string{"Anna", "Beat", "Clara"} {
		resp := postForm(t, client, entriesPage, entriesPage+"/individual", url.Values{
			"event": {eventID}, "first_name": {name}, "last_name": {"Muster"},
			"birth_year": {"2011"}, "sex": {"W"}, "club": {"LC Bib"}, "seed": {"13.50"},
		})
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("submit entry for %s = %d, want 303", name, resp.StatusCode)
		}
	}

	logout(t, client, base)
	login(t, client, base, "admin", "s3cret-passphrase")
	bibsPage := base + "/meets/" + meetID + "/bibs"
	body := bodyString(t, mustGet(t, client, bibsPage))
	if !strings.Contains(body, "LC Bib") {
		t.Fatalf("bibs page missing the LC Bib club option: %s", body)
	}
	clubID := clubIDFromBibsPage(t, body, "LC Bib")

	resp := postForm(t, client, bibsPage, bibsPage+"/bulk", url.Values{"club": {clubID}, "start": {"200"}})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("bulk bib assign = %d, want 303", resp.StatusCode)
	}

	body = bodyString(t, mustGet(t, client, bibsPage))
	for _, bib := range []string{"200", "201", "202"} {
		if !strings.Contains(body, ">"+bib+"<") {
			t.Errorf("bibs page missing assigned bib %q: %s", bib, body)
		}
	}

	// UC-006 #2: a manual duplicate is rejected.
	participantID, version := bibRowFrom(t, body, "200")
	resp = postForm(t, client, bibsPage, bibsPage+"/"+participantID, url.Values{"bib": {"201"}, "version": {version}})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("duplicate bib POST = %d, want 303 (redirect with flash)", resp.StatusCode)
	}
	if !strings.Contains(resp.Header.Get("Location"), "err=duplicate") {
		t.Errorf("duplicate bib redirect = %q, want err=duplicate", resp.Header.Get("Location"))
	}
}

// clubIDFromBibsPage extracts the <option value="ID">clubName</option> id
// from the bulk-assign club select.
func clubIDFromBibsPage(t *testing.T, body, clubName string) string {
	t.Helper()
	marker := ">" + clubName + "<"
	i := strings.Index(body, marker)
	if i < 0 {
		t.Fatalf("club %q not found on bibs page", clubName)
	}
	prefix := body[:i]
	j := strings.LastIndex(prefix, `value="`)
	if j < 0 {
		t.Fatalf("no option value before club %q", clubName)
	}
	rest := prefix[j+len(`value="`):]
	return rest[:strings.Index(rest, `"`)]
}

// bibRowFrom finds the participant ID and version of the row whose current
// bib matches want, from the per-row assignment form's action URL and
// hidden version field.
func bibRowFrom(t *testing.T, body, want string) (participantID, version string) {
	t.Helper()
	marker := ">" + want + "<"
	i := strings.Index(body, marker)
	if i < 0 {
		t.Fatalf("bib %q not found on bibs page", want)
	}
	rest := body[i:]
	actionMarker := `/bibs/`
	j := strings.Index(rest, actionMarker)
	if j < 0 {
		t.Fatalf("no bib-assignment form found after bib %q", want)
	}
	rest = rest[j+len(actionMarker):]
	participantID = rest[:strings.Index(rest, `"`)]
	verMarker := `name="version" value="`
	k := strings.Index(rest, verMarker)
	if k < 0 {
		t.Fatalf("no version field found after bib %q", want)
	}
	rest = rest[k+len(verMarker):]
	version = rest[:strings.Index(rest, `"`)]
	return participantID, version
}

// TestFeeSummaryFlowSYS017UC006_3Web drives UC-006 #3 over real HTTP: the
// organizer configures a fee schedule and the per-club summary reflects the
// entries × schedule arithmetic.
func TestFeeSummaryFlowSYS017UC006_3Web(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	meetID, _ := entryFlowFixture(t, deps, client, base)
	eventID := mustEventID(t, deps, meetID, "100m")

	logout(t, client, base)
	login(t, client, base, "sub1", "s3cret-passphrase")
	entriesPage := base + "/meets/" + meetID + "/entries"
	for _, name := range []string{"Anna", "Beat"} {
		resp := postForm(t, client, entriesPage, entriesPage+"/individual", url.Values{
			"event": {eventID}, "first_name": {name}, "last_name": {"Muster"},
			"birth_year": {"2011"}, "sex": {"W"}, "club": {"LC Fee"}, "seed": {"13.50"},
		})
		_ = resp.Body.Close()
	}

	logout(t, client, base)
	login(t, client, base, "admin", "s3cret-passphrase")
	feesPage := base + "/meets/" + meetID + "/fees"
	current, err := deps.meets.Meet(context.Background(), meetID)
	if err != nil {
		t.Fatalf("Meet: %v", err)
	}
	resp := postForm(t, client, feesPage, feesPage+"/schedule", url.Values{
		"version": {intToStr(current.Version)}, "entry_fee": {"10.00"}, "relay_fee": {"30.00"},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("set fee schedule = %d, want 303", resp.StatusCode)
	}

	body := bodyString(t, mustGet(t, client, feesPage))
	for _, want := range []string{"LC Fee", "20.00"} {
		if !strings.Contains(body, want) {
			t.Errorf("fees page missing %q: %s", want, body)
		}
	}

	csv := bodyString(t, mustGet(t, client, feesPage+"/export"))
	if !strings.Contains(csv, "LC Fee") || !strings.Contains(csv, "20.00") {
		t.Errorf("fee export CSV missing expected data: %s", csv)
	}
}

// TestOnlineEntryClubBulkFlowSYS011UC003_2Web drives UC-003 #2 over real
// HTTP: a club submitter's bulk-entry form creates every non-blank row.
func TestOnlineEntryClubBulkFlowSYS011UC003_2Web(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	meetID, _ := entryFlowFixture(t, deps, client, base)
	eventID := mustEventID(t, deps, meetID, "100m")

	logout(t, client, base)
	login(t, client, base, "sub1", "s3cret-passphrase")
	page := base + "/meets/" + meetID + "/entries"
	resp := postForm(t, client, page, page+"/bulk", url.Values{
		"club":              {"LC Bulk"},
		"bulk_first_name_0": {"Anna"}, "bulk_last_name_0": {"Muster"}, "bulk_birth_year_0": {"2011"},
		"bulk_sex_0": {"W"}, "bulk_event_0": {eventID}, "bulk_seed_0": {"13.50"},
		"bulk_first_name_1": {"Beat"}, "bulk_last_name_1": {"Muster"}, "bulk_birth_year_1": {"2011"},
		"bulk_sex_1": {"W"}, "bulk_event_1": {eventID}, "bulk_seed_1": {"13.60"},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("submit bulk entries = %d, want 303", resp.StatusCode)
	}

	body := bodyString(t, mustGet(t, client, page))
	for _, want := range []string{"Anna Muster", "Beat Muster"} {
		if !strings.Contains(body, want) {
			t.Errorf("entries page missing bulk-submitted %q: %s", want, body)
		}
	}
}

// TestOnlineEntryRelayFlowSYS012UC003_4Web drives UC-003 #4 over real HTTP:
// a relay entry's ordered composition is submitted and then revised.
func TestOnlineEntryRelayFlowSYS012UC003_4Web(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	meetID, _ := entryFlowFixture(t, deps, client, base)
	resp := addEvent(t, client, base, meetID, url.Values{
		"discipline": {"4x100m"}, "categories": {"U16 W"}, "round_final": {"1"},
	})
	_ = resp.Body.Close()

	logout(t, client, base)
	login(t, client, base, "sub1", "s3cret-passphrase")
	page := base + "/meets/" + meetID + "/entries"
	relayEventID := mustEventID(t, deps, meetID, "4x100m")

	resp = postForm(t, client, page, page+"/relay", url.Values{
		"relay_event": {relayEventID}, "relay_club": {"LC Relay"},
		"leg_first_name_0": {"Leg1"}, "leg_last_name_0": {"Runner"}, "leg_birth_year_0": {"2011"}, "leg_sex_0": {"W"},
		"leg_first_name_1": {"Leg2"}, "leg_last_name_1": {"Runner"}, "leg_birth_year_1": {"2011"}, "leg_sex_1": {"W"},
		"leg_first_name_2": {"Leg3"}, "leg_last_name_2": {"Runner"}, "leg_birth_year_2": {"2011"}, "leg_sex_2": {"W"},
		"leg_first_name_3": {"Leg4"}, "leg_last_name_3": {"Runner"}, "leg_birth_year_3": {"2011"}, "leg_sex_3": {"W"},
		"reserve_first_name_0": {"Res1"}, "reserve_last_name_0": {"Runner"}, "reserve_birth_year_0": {"2011"}, "reserve_sex_0": {"W"},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("submit relay entry = %d, want 303", resp.StatusCode)
	}

	body := bodyString(t, mustGet(t, client, page))
	if !strings.Contains(body, "LC Relay") || !strings.Contains(body, "Leg1 Runner") {
		t.Fatalf("entries page missing the relay entry: %s", body)
	}
	entryID, relayVersion := relayEditFormFrom(t, body, meetID)

	resp = postForm(t, client, page, page+"/"+entryID+"/relay-composition", url.Values{
		"relay_version":         {relayVersion},
		"edit_leg_first_name_0": {"NewLeg1"}, "edit_leg_last_name_0": {"Runner"}, "edit_leg_birth_year_0": {"2011"}, "edit_leg_sex_0": {"W"},
		"edit_leg_first_name_1": {"Leg2"}, "edit_leg_last_name_1": {"Runner"}, "edit_leg_birth_year_1": {"2011"}, "edit_leg_sex_1": {"W"},
		"edit_leg_first_name_2": {"Leg3"}, "edit_leg_last_name_2": {"Runner"}, "edit_leg_birth_year_2": {"2011"}, "edit_leg_sex_2": {"W"},
		"edit_leg_first_name_3": {"Leg4"}, "edit_leg_last_name_3": {"Runner"}, "edit_leg_birth_year_3": {"2011"}, "edit_leg_sex_3": {"W"},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("revise relay composition = %d, want 303", resp.StatusCode)
	}
	body = bodyString(t, mustGet(t, client, page))
	if !strings.Contains(body, "NewLeg1 Runner") {
		t.Errorf("entries page missing the revised leg: %s", body)
	}
}

// relayEditFormFrom extracts the entry ID (from the composition-edit form's
// action URL) and the relay team's version (from its hidden field).
func relayEditFormFrom(t *testing.T, body, meetID string) (entryID, relayVersion string) {
	t.Helper()
	marker := "/meets/" + meetID + "/entries/"
	i := strings.LastIndex(body, marker)
	if i < 0 {
		t.Fatalf("no relay-composition edit form found: %s", body)
	}
	rest := body[i+len(marker):]
	suffixMarker := "/relay-composition"
	j := strings.Index(rest, suffixMarker)
	if j < 0 {
		t.Fatalf("no relay-composition suffix found: %s", body)
	}
	entryID = rest[:j]
	rest = body[i:]
	verMarker := `name="relay_version" value="`
	k := strings.Index(rest, verMarker)
	if k < 0 {
		t.Fatalf("no relay_version field found: %s", body)
	}
	rest = rest[k+len(verMarker):]
	relayVersion = rest[:strings.Index(rest, `"`)]
	return entryID, relayVersion
}

// TestEntryExceptionReportSYS015UC003_5Web drives the organizer's exception
// report over real HTTP: a failing-standard entry appears on it.
func TestEntryExceptionReportSYS015UC003_5Web(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	meetID, _ := entryFlowFixture(t, deps, client, base)

	// Add an event with an entry standard the seed below will fail.
	resp := addEvent(t, client, base, meetID, url.Values{
		"discipline": {"100m"}, "categories": {"U16 W"}, "round_final": {"1"}, "entry_standard": {"12.20"},
	})
	_ = resp.Body.Close()
	body := bodyString(t, mustGet(t, client, base+"/meets/"+meetID))
	if !strings.Contains(body, "Limite") { // programme.entry_standard's DE label
		t.Fatalf("meet page missing the entry standard: %s", body)
	}

	// Resolve the standard event's ID via the app layer — it is the second
	// 100m event added.
	detail, err := deps.meets.Meet(context.Background(), meetID)
	if err != nil {
		t.Fatalf("Meet: %v", err)
	}
	var standardEventID string
	for _, pe := range detail.Programme {
		if pe.DisciplineCode == "100m" && pe.EntryStandard == "12.20" {
			standardEventID = pe.ID
		}
	}
	if standardEventID == "" {
		t.Fatal("could not resolve the entry-standard event's id")
	}

	logout(t, client, base)
	login(t, client, base, "sub1", "s3cret-passphrase")
	entriesPage := base + "/meets/" + meetID + "/entries"
	resp = postForm(t, client, entriesPage, entriesPage+"/individual", url.Values{
		"event": {standardEventID}, "first_name": {"Anna"}, "last_name": {"Muster"},
		"birth_year": {"2011"}, "sex": {"W"}, "seed": {"12.85"},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("submit failing-standard entry = %d, want 303", resp.StatusCode)
	}

	logout(t, client, base)
	login(t, client, base, "admin", "s3cret-passphrase")
	body = bodyString(t, mustGet(t, client, base+"/meets/"+meetID+"/entries/exceptions"))
	for _, want := range []string{"Anna Muster", "12.85", "12.20"} {
		if !strings.Contains(body, want) {
			t.Errorf("exception report missing %q: %s", want, body)
		}
	}
}

// TestBibsPDFDownloadSYS018UC006_1Web covers UC-006 #1's "printable bib
// list per club" over real HTTP: a real PDF downloads, organizer-gated.
func TestBibsPDFDownloadSYS018UC006_1Web(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	meetID, _ := entryFlowFixture(t, deps, client, base)
	eventID := mustEventID(t, deps, meetID, "100m")

	logout(t, client, base)
	login(t, client, base, "sub1", "s3cret-passphrase")
	entriesPage := base + "/meets/" + meetID + "/entries"
	resp := postForm(t, client, entriesPage, entriesPage+"/individual", url.Values{
		"event": {eventID}, "first_name": {"Anna"}, "last_name": {"Muster"},
		"birth_year": {"2011"}, "sex": {"W"}, "club": {"LC Bib"}, "seed": {"13.50"},
	})
	_ = resp.Body.Close()

	logout(t, client, base)
	login(t, client, base, "admin", "s3cret-passphrase")
	pdfResp := mustGet(t, client, base+"/meets/"+meetID+"/bibs.pdf")
	defer func() { _ = pdfResp.Body.Close() }()
	if pdfResp.StatusCode != http.StatusOK {
		t.Fatalf("GET bibs.pdf = %d, want 200", pdfResp.StatusCode)
	}
	if ct := pdfResp.Header.Get("Content-Type"); ct != "application/pdf" {
		t.Errorf("Content-Type = %q, want application/pdf", ct)
	}
	data := bodyString(t, pdfResp)
	if !strings.HasPrefix(data, "%PDF") {
		t.Error("downloaded body does not look like a PDF")
	}

	anon := mustGet(t, &http.Client{}, base+"/meets/"+meetID+"/bibs.pdf")
	_ = anon.Body.Close()
	if anon.StatusCode != http.StatusForbidden {
		t.Errorf("anonymous bibs.pdf GET = %d, want 403 (SYS-090)", anon.StatusCode)
	}
}

// TestJoinStringsCentsToCHFAndChfToCents covers entries.go's pure rendering
// helpers directly: the "–" empty-list placeholder (never a blank cell),
// CHF cent formatting including the negative-amount sign, and cent parsing's
// comma-decimal acceptance, blank-is-zero convention and reject-on-garbage
// error path.
func TestJoinStringsCentsToCHFAndChfToCents(t *testing.T) {
	if got := joinStrings(nil); got != "–" {
		t.Errorf("joinStrings(nil) = %q, want \"–\"", got)
	}
	if got := joinStrings([]string{"Anna", "Beat"}); got != "Anna, Beat" {
		t.Errorf("joinStrings = %q, want \"Anna, Beat\"", got)
	}

	if got := centsToCHF(0); got != "0.00" {
		t.Errorf("centsToCHF(0) = %q, want 0.00", got)
	}
	if got := centsToCHF(1234); got != "12.34" {
		t.Errorf("centsToCHF(1234) = %q, want 12.34", got)
	}
	if got := centsToCHF(-150); got != "-1.50" {
		t.Errorf("centsToCHF(-150) = %q, want -1.50", got)
	}

	if got, err := chfToCents(""); err != nil || got != 0 {
		t.Errorf("chfToCents(\"\") = (%d, %v), want (0, nil)", got, err)
	}
	if got, err := chfToCents("12,50"); err != nil || got != 1250 {
		t.Errorf("chfToCents(12,50) = (%d, %v), want (1250, nil)", got, err)
	}
	if got, err := chfToCents(" 9.5 "); err != nil || got != 950 {
		t.Errorf("chfToCents(\" 9.5 \") = (%d, %v), want (950, nil)", got, err)
	}
	if _, err := chfToCents("not-a-number"); err == nil {
		t.Error("chfToCents(\"not-a-number\") = nil error, want an error")
	}
}

// TestEntryFlashKeyMapsKnownErrors covers entryFlashKey's full switch: every
// sentinel entry-submission error maps to its own "entries.error.*" suffix,
// and anything else falls back to "invalid" rather than leaking a raw
// Go error string into the redirect (SYS-011/012/015).
func TestEntryFlashKeyMapsKnownErrors(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{app.ErrEntryDeadlinePassed, "deadline"},
		{app.ErrEntryLimitReached, "limit"},
		{app.ErrSeedPerformanceRequired, "seed_required"},
		{app.ErrDuplicateEntry, "duplicate"},
		{app.ErrEntriesClosed, "closed"},
		{app.ErrNotRelayEntry, "not_relay"},
		{app.ErrConflict, "conflict"},
		{errors.New("some other failure"), "invalid"},
	} {
		if got := entryFlashKey(tc.err); got != tc.want {
			t.Errorf("entryFlashKey(%v) = %q, want %q", tc.err, got, tc.want)
		}
	}
}

// TestBibFlashKeyMapsKnownErrors mirrors TestEntryFlashKeyMapsKnownErrors for
// bibFlashKey's smaller switch (SYS-018).
func TestBibFlashKeyMapsKnownErrors(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{app.ErrBibRequired, "required"},
		{app.ErrDuplicateParticipant, "duplicate"},
		{app.ErrConflict, "conflict"},
		{errors.New("some other failure"), "invalid"},
	} {
		if got := bibFlashKey(tc.err); got != tc.want {
			t.Errorf("bibFlashKey(%v) = %q, want %q", tc.err, got, tc.want)
		}
	}
}

// TestEntriesAndBibsUnknownMeetIs404Web covers handleEntries'/handleBibs'
// error-mapping branch through entriesView/bibsView: a syntactically fine
// but nonexistent meet id surfaces the shared 404, not a 500 or a blank page.
func TestEntriesAndBibsUnknownMeetIs404Web(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)

	for _, path := range []string{"/meets/does-not-exist/entries", "/meets/does-not-exist/bibs"} {
		resp := mustGet(t, client, base+path)
		_ = bodyString(t, resp)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, resp.StatusCode)
		}
	}
}

// TestEntriesFlashErrorQueryParamWeb covers handleEntries' "?err=" flash
// rendering branch (the redirect target every entry-submission error lands
// on).
func TestEntriesFlashErrorQueryParamWeb(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	meetID, _ := entryFlowFixture(t, deps, client, base)

	logout(t, client, base)
	login(t, client, base, "sub1", "s3cret-passphrase")
	body := bodyString(t, mustGet(t, client, base+"/meets/"+meetID+"/entries?err=duplicate"))
	if !strings.Contains(body, "gemeldet") && !strings.Contains(body, "déjà") {
		t.Errorf("entries page with ?err=duplicate should render a localized flash: %s", body)
	}
}

// TestOnlineEntryRelayEmptyCompositionRejectedWeb covers
// handleEntryRelaySubmit's error path: a relay entry with no leg rows at all
// is rejected server-side (a malformed/forged submission, not a client-side-
// only validation), redirecting with a flash rather than creating a
// zero-athlete team.
func TestOnlineEntryRelayEmptyCompositionRejectedWeb(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	meetID, _ := entryFlowFixture(t, deps, client, base)
	resp := addEvent(t, client, base, meetID, url.Values{
		"discipline": {"4x100m"}, "categories": {"U16 W"}, "round_final": {"1"},
	})
	_ = resp.Body.Close()

	logout(t, client, base)
	login(t, client, base, "sub1", "s3cret-passphrase")
	page := base + "/meets/" + meetID + "/entries"
	relayEventID := mustEventID(t, deps, meetID, "4x100m")

	resp = postForm(t, client, page, page+"/relay", url.Values{
		"relay_event": {relayEventID}, "relay_club": {"LC Empty"},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("empty-composition relay submit = %d, want 303", resp.StatusCode)
	}
	if !strings.Contains(resp.Header.Get("Location"), "err=invalid") {
		t.Errorf("redirect location = %q, want err=invalid", resp.Header.Get("Location"))
	}
	body := bodyString(t, mustGet(t, client, page))
	if strings.Contains(body, "LC Empty") {
		t.Error("no relay entry should have been created with an empty composition")
	}
}

// TestEntryRelayCompositionUpdateUnknownEntryWeb covers
// handleEntryRelayCompositionUpdate's error path with an entry id the
// submission form never offered (a forged path segment): store.GetEntry's
// not-found surfaces as a redirect-with-flash, matching the deadline-forgery
// pattern already proven for individual entries.
func TestEntryRelayCompositionUpdateUnknownEntryWeb(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	meetID, _ := entryFlowFixture(t, deps, client, base)

	logout(t, client, base)
	login(t, client, base, "sub1", "s3cret-passphrase")
	page := base + "/meets/" + meetID + "/entries"
	resp := postForm(t, client, page, page+"/does-not-exist/relay-composition", url.Values{
		"relay_version":         {"0"},
		"edit_leg_first_name_0": {"X"}, "edit_leg_last_name_0": {"Y"}, "edit_leg_birth_year_0": {"2011"}, "edit_leg_sex_0": {"W"},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("relay-composition update for an unknown entry = %d, want 303", resp.StatusCode)
	}
	if !strings.Contains(resp.Header.Get("Location"), "err=") {
		t.Errorf("redirect location = %q, want an err= flash", resp.Header.Get("Location"))
	}
}

// TestBibAssignEmptyBibRejectedWeb covers handleBibAssign's ErrBibRequired
// branch (bibFlashKey "required"): submitting a blank bib value is refused,
// not silently cleared.
func TestBibAssignEmptyBibRejectedWeb(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	meetID, _ := entryFlowFixture(t, deps, client, base)
	eventID := mustEventID(t, deps, meetID, "100m")

	logout(t, client, base)
	login(t, client, base, "sub1", "s3cret-passphrase")
	entriesPage := base + "/meets/" + meetID + "/entries"
	resp := postForm(t, client, entriesPage, entriesPage+"/individual", url.Values{
		"event": {eventID}, "first_name": {"Anna"}, "last_name": {"Muster"},
		"birth_year": {"2011"}, "sex": {"W"}, "club": {"LC Empty Bib"}, "seed": {"13.50"},
	})
	_ = resp.Body.Close()

	logout(t, client, base)
	login(t, client, base, "admin", "s3cret-passphrase")
	bibsPage := base + "/meets/" + meetID + "/bibs"
	body := bodyString(t, mustGet(t, client, bibsPage))
	// The fixture registers exactly one participant (no bib assigned yet),
	// so its per-row assignment form is the only one on the page —
	// bibRowFrom's "><" marker is ambiguous against adjacent HTML tags in
	// general, so extract the row directly instead.
	re := regexp.MustCompile(`(?s)/bibs/([0-9A-Z]{20,30})" method="post".*?name="version" value="(\d+)"`)
	m := re.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("bibs page has no per-row assignment form: %s", body)
	}
	participantID, version := m[1], m[2]
	resp = postForm(t, client, bibsPage, bibsPage+"/"+participantID, url.Values{"bib": {"   "}, "version": {version}})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("empty bib assign = %d, want 303", resp.StatusCode)
	}
	if !strings.Contains(resp.Header.Get("Location"), "err=required") {
		t.Errorf("redirect location = %q, want err=required", resp.Header.Get("Location"))
	}
}

// TestBibBulkAssignNonPositiveStartRejectedWeb covers handleBibBulkAssign's
// error path: a non-positive starting bib number (a blank/zero "start"
// field) is rejected rather than silently assigning bib "0".
func TestBibBulkAssignNonPositiveStartRejectedWeb(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	meetID, _ := entryFlowFixture(t, deps, client, base)
	eventID := mustEventID(t, deps, meetID, "100m")

	logout(t, client, base)
	login(t, client, base, "sub1", "s3cret-passphrase")
	entriesPage := base + "/meets/" + meetID + "/entries"
	resp := postForm(t, client, entriesPage, entriesPage+"/individual", url.Values{
		"event": {eventID}, "first_name": {"Anna"}, "last_name": {"Muster"},
		"birth_year": {"2011"}, "sex": {"W"}, "club": {"LC Zero Start"}, "seed": {"13.50"},
	})
	_ = resp.Body.Close()

	logout(t, client, base)
	login(t, client, base, "admin", "s3cret-passphrase")
	bibsPage := base + "/meets/" + meetID + "/bibs"
	body := bodyString(t, mustGet(t, client, bibsPage))
	clubID := clubIDFromBibsPage(t, body, "LC Zero Start")

	resp = postForm(t, client, bibsPage, bibsPage+"/bulk", url.Values{"club": {clubID}, "start": {"0"}})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("bulk bib assign with start=0 = %d, want 303", resp.StatusCode)
	}
	if !strings.Contains(resp.Header.Get("Location"), "err=invalid") {
		t.Errorf("redirect location = %q, want err=invalid", resp.Header.Get("Location"))
	}
}
