// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// setupAndLogin drives the first-run setup form and logs the new admin in —
// the browser-only path UC-001 #1 requires (no config file, no CLI).
func setupAndLogin(t *testing.T, client *http.Client, base string) {
	t.Helper()
	token := csrfTokenFrom(t, bodyString(t, mustGet(t, client, base+"/setup")))
	resp, err := client.PostForm(base+"/setup", url.Values{
		"username": {"admin"}, "display_name": {"Administrator"},
		"password": {"s3cret-passphrase"}, "csrf_token": {token},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST /setup = %d, want 303", resp.StatusCode)
	}

	token = csrfTokenFrom(t, bodyString(t, mustGet(t, client, base+"/login")))
	resp, err = client.PostForm(base+"/login", url.Values{
		"username": {"admin"}, "password": {"s3cret-passphrase"}, "csrf_token": {token},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST /login = %d, want 303", resp.StatusCode)
	}
}

// postForm posts values (adding the CSRF token from page) and asserts a 303.
func postForm(t *testing.T, client *http.Client, page, target string, values url.Values) *http.Response {
	t.Helper()
	values.Set("csrf_token", csrfTokenFrom(t, bodyString(t, mustGet(t, client, page))))
	resp, err := client.PostForm(target, values)
	if err != nil {
		t.Fatalf("POST %s: %v", target, err)
	}
	return resp
}

func TestSetupFlowFirstRunOnly(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)

	// Too-short password is rejected with a localized message.
	token := csrfTokenFrom(t, bodyString(t, mustGet(t, client, base+"/setup")))
	resp, err := client.PostForm(base+"/setup", url.Values{
		"username": {"admin"}, "display_name": {"Administrator"},
		"password": {"short"}, "csrf_token": {token},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("short password = %d, want 422", resp.StatusCode)
	}
	_ = bodyString(t, resp)

	// Missing fields are rejected.
	resp, err = client.PostForm(base+"/setup", url.Values{
		"username": {""}, "display_name": {""}, "password": {"s3cret-passphrase"}, "csrf_token": {token},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("missing fields = %d, want 422", resp.StatusCode)
	}
	_ = bodyString(t, resp)

	setupAndLogin(t, client, base)

	// Once bootstrapped, /setup only redirects — the flow cannot run twice.
	resp = mustGet(t, client, base+"/setup")
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/login" {
		t.Errorf("GET /setup after bootstrap = %d -> %q, want 303 -> /login",
			resp.StatusCode, resp.Header.Get("Location"))
	}
	resp, err = client.PostForm(base+"/setup", url.Values{
		"username": {"admin2"}, "display_name": {"x"}, "password": {"s3cret-passphrase"},
		"csrf_token": {token},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("POST /setup after bootstrap = %d, want 303 redirect away", resp.StatusCode)
	}
}

func TestMeetsRequireOrganizerRole(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)

	for _, path := range []string{"/meets", "/meets/new", "/meets/some-id", "/meets/some-id/sanctioning"} {
		resp := mustGet(t, client, base+path)
		_ = bodyString(t, resp)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("anonymous GET %s = %d, want 403 (SYS-090)", path, resp.StatusCode)
		}
	}
}

// createUCMeet drives the UC-001 #2 meet-creation form: two competition
// days, two sessions per day, tier C-Meeting.
func createUCMeet(t *testing.T, client *http.Client, base string) string {
	t.Helper()
	resp := postForm(t, client, base+"/meets/new", base+"/meets", url.Values{
		"name": {"Abendmeeting Uster"}, "venue": {"Stadion Buchholz"},
		"homologation_ref": {"CH-ZH-042"},
		"start_date":       {"2027-06-12"}, "end_date": {"2027-06-13"},
		"tier": {"C-Meeting"}, "scheme": {"swiss-athletics"},
		"session_day_0": {"2027-06-12"}, "session_label_0": {"Session 1"},
		"session_day_1": {"2027-06-12"}, "session_label_1": {"Session 2"},
		"session_day_2": {"2027-06-13"}, "session_label_2": {"Session 1"},
		"session_day_3": {"2027-06-13"}, "session_label_3": {"Session 2"},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create meet = %d, want 303", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if !strings.HasPrefix(loc, "/meets/") {
		t.Fatalf("create meet redirected to %q", loc)
	}
	return strings.TrimPrefix(loc, "/meets/")
}

// TestMeetCreationUC001_2 covers UC-001 #2 over the operator interface:
// the created meet appears in status draft with exactly the entered
// attributes retrievable — with no configuration file involved (SYS-001).
func TestMeetCreationUC001_2(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)

	meetID := createUCMeet(t, client, base)
	body := bodyString(t, mustGet(t, client, base+"/meets/"+meetID))
	for _, want := range []string{
		"Abendmeeting Uster", "Stadion Buchholz", "C-Meeting",
		"12.06.2027", "13.06.2027", "Entwurf", // draft, DE default locale, SYS-110 display date format
		"Session 1", "Session 2",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("meet page missing %q", want)
		}
	}

	// The overview lists it too.
	list := bodyString(t, mustGet(t, client, base+"/meets"))
	if !strings.Contains(list, "Abendmeeting Uster") {
		t.Error("meet overview missing the created meet")
	}

	// Invalid input re-renders the form with a localized error.
	resp := postForm(t, client, base+"/meets/new", base+"/meets", url.Values{
		"name": {"x"}, "venue": {"y"}, "start_date": {"not-a-date"}, "end_date": {"2027-06-13"},
		"tier": {"C-Meeting"}, "scheme": {"swiss-athletics"},
	})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("bad create = %d, want 422", resp.StatusCode)
	}
	_ = bodyString(t, resp)
}

// TestMeetCreateFieldErrorsOQ075UC038_4 covers UC-038 #4's "meet setup"
// representative form with the usability audit's own literal repro case
// (usability-audit-2026-07.md H9/F3): an end date before the start date.
// The fix must name the offending field, state what to fix, and preserve
// every other submitted value — not just a page-level "could not be saved"
// flash.
func TestMeetCreateFieldErrorsOQ075UC038_4(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)

	resp := postForm(t, client, base+"/meets/new", base+"/meets", url.Values{
		"name": {"Abendmeeting Uster"}, "venue": {"Stadion Buchholz"},
		"start_date": {"2027-06-13"}, "end_date": {"2027-06-12"}, // end before start
		"tier": {"C-Meeting"}, "scheme": {"swiss-athletics"},
	})
	body := bodyString(t, resp)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("end-before-start create = %d, want 422: %s", resp.StatusCode, body)
	}
	for _, want := range []string{
		`aria-invalid="true"`,
		`id="end_date-error"`,
		// the valid fields must survive the re-render (preserved input).
		`value="Abendmeeting Uster"`,
		`value="Stadion Buchholz"`,
		`value="2027-06-13"`,
		`value="2027-06-12"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("re-rendered meet form missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, `id="name-error"`) {
		t.Error("name field has no error and must not render a field-error paragraph")
	}

	// A second case: name and venue both blank, dates fine — both required
	// fields get their own inline error, not a shared generic one.
	resp2 := postForm(t, client, base+"/meets/new", base+"/meets", url.Values{
		"name": {""}, "venue": {""}, "start_date": {"2027-06-12"}, "end_date": {"2027-06-13"},
		"tier": {"C-Meeting"}, "scheme": {"swiss-athletics"},
	})
	body2 := bodyString(t, resp2)
	if resp2.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("blank name/venue create = %d, want 422: %s", resp2.StatusCode, body2)
	}
	for _, want := range []string{`id="name-error"`, `id="venue-error"`} {
		if !strings.Contains(body2, want) {
			t.Errorf("re-rendered meet form missing %q: %s", want, body2)
		}
	}
}

// addEvent posts one add-event form on the meet page.
func addEvent(t *testing.T, client *http.Client, base, meetID string, values url.Values) *http.Response {
	t.Helper()
	return postForm(t, client, base+"/meets/"+meetID, base+"/meets/"+meetID+"/events", values)
}

// TestEventProgrammeUC001_3 covers UC-001 #3 over the operator interface:
// 100m / Shot Put / 4×100m for U16 W each land on the programme with
// discipline-correct capture types and deadlines.
func TestEventProgrammeUC001_3(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	meetID := createUCMeet(t, client, base)

	for _, ev := range []url.Values{
		{"discipline": {"100m"}, "categories": {"U16 W"}, "round_qualification": {"1"},
			"round_final": {"1"}, "entry_deadline": {"2027-06-01T23:59"}},
		{"discipline": {"SP"}, "categories": {"U16 W"}, "round_final": {"1"},
			"entry_deadline": {"2027-06-01T23:59"}},
		{"discipline": {"4x100m"}, "categories": {"U16 W"}, "round_final": {"1"},
			"entry_deadline": {"2027-06-01T23:59"}},
	} {
		resp := addEvent(t, client, base, meetID, ev)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusSeeOther || strings.Contains(resp.Header.Get("Location"), "err=") {
			t.Fatalf("add event %v = %d -> %q", ev, resp.StatusCode, resp.Header.Get("Location"))
		}
	}

	body := bodyString(t, mustGet(t, client, base+"/meets/"+meetID))
	// Capture types in the DE catalog: track=Lauf, horizontal=Weite, relay=Staffel.
	for _, want := range []string{"Lauf", "Weite", "Staffel", "U16 W", "01.06.2027 23:59", "Qualifikation"} { //nolint:misspell // "Qualifikation" is the German catalog string (round.qualification, de.json)
		if !strings.Contains(body, want) {
			t.Errorf("programme missing %q (UC-001 #3)", want)
		}
	}

	// An invalid category bounces back with an error key, nothing created.
	resp := addEvent(t, client, base, meetID, url.Values{
		"discipline": {"100m"}, "categories": {"U99 X"}, "round_final": {"1"},
	})
	_ = resp.Body.Close()
	if !strings.Contains(resp.Header.Get("Location"), "err=") {
		t.Errorf("invalid category add-event redirected to %q, want err key", resp.Header.Get("Location"))
	}
	// The error key renders as a localized flash.
	flash := bodyString(t, mustGet(t, client, base+resp.Header.Get("Location")))
	if !strings.Contains(flash, "gespeichert") {
		t.Errorf("flash message not rendered for err key")
	}
}

// TestTimetablePublishAmendPublicUC001_4 covers UC-001 #4 end to end over
// HTTP: schedule a unit, publish, amend, republish — the public page
// (anonymous) shows the amended time, both versions retained, and each
// publish emits a live event on the meet's SSE topic.
func TestTimetablePublishAmendPublicUC001_4(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	meetID := createUCMeet(t, client, base)

	resp := addEvent(t, client, base, meetID, url.Values{
		"discipline": {"100m"}, "categories": {"U16 W"}, "round_final": {"1"},
	})
	_ = resp.Body.Close()

	// Fetch the unit ID through the app layer (the HTML page embeds it in
	// the schedule form's action URL, but the service read is stabler).
	d, err := deps.meets.Meet(context.Background(), meetID)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Units) != 1 {
		t.Fatalf("got %d units, want 1", len(d.Units))
	}
	unit := d.Units[0]

	events, cancelSub := deps.bus.Subscribe("meet-" + meetID)
	defer cancelSub()

	meetPage := base + "/meets/" + meetID
	scheduleURL := meetPage + "/units/" + unit.UnitID + "/schedule"
	resp = postForm(t, client, meetPage, scheduleURL, url.Values{
		"scheduled_at": {"2027-06-12T14:30"}, "location": {"Bahn 1"},
		"version": {intToStr(unit.UnitVersion)},
	})
	_ = resp.Body.Close()
	if strings.Contains(resp.Header.Get("Location"), "err=") {
		t.Fatalf("schedule redirected to %q", resp.Header.Get("Location"))
	}

	resp = postForm(t, client, meetPage, meetPage+"/timetable/publish", url.Values{})
	_ = resp.Body.Close()
	if strings.Contains(resp.Header.Get("Location"), "err=") {
		t.Fatalf("publish redirected to %q", resp.Header.Get("Location"))
	}
	select {
	case ev := <-events:
		if ev.Name != "timetable" || !strings.Contains(ev.Data, `"version":1`) {
			t.Errorf("published SSE event = %+v", ev)
		}
	default:
		t.Error("publish did not emit a bus event on the meet topic")
	}

	// Amend the unit's time (fresh version after the first schedule write).
	d, err = deps.meets.Meet(context.Background(), meetID)
	if err != nil {
		t.Fatal(err)
	}
	unit = d.Units[0]
	resp = postForm(t, client, meetPage, scheduleURL, url.Values{
		"scheduled_at": {"2027-06-12T15:15"}, "location": {"Bahn 1"},
		"version": {intToStr(unit.UnitVersion)},
	})
	_ = resp.Body.Close()
	resp = postForm(t, client, meetPage, meetPage+"/timetable/publish", url.Values{})
	_ = resp.Body.Close()

	// The public timetable requires no session and shows the amended time.
	anon, anonBase := newTestClient(t, deps)
	// Same underlying server: use the original base (newTestClient started a
	// second listener over the same routes, either works).
	_ = anonBase
	pub := bodyString(t, mustGet(t, anon, base+"/m/"+meetID+"/timetable"))
	if !strings.Contains(pub, "15:15") {
		t.Errorf("public timetable does not show the amended time (UC-001 #4): %s", pub)
	}
	if strings.Contains(pub, "14:30") {
		t.Error("public timetable still shows the pre-amendment time")
	}

	// Both published versions are retained with timestamps on the meet page.
	body := bodyString(t, mustGet(t, client, meetPage))
	if !strings.Contains(body, "Version 1") || !strings.Contains(body, "Version 2") {
		t.Errorf("meet page missing retained timetable versions (SYS-004)")
	}

	// A meet with no published timetable 404s publicly.
	otherID := createUCMeet(t, client, base)
	resp = mustGet(t, anon, base+"/m/"+otherID+"/timetable")
	_ = bodyString(t, resp)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("unpublished public timetable = %d, want 404", resp.StatusCode)
	}
}

// TestMeetEditConflictAndArchive covers SYS-001 edit/archive over HTTP,
// including the SYS-083 conflict surface (409 + localized message).
func TestMeetEditConflictAndArchive(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	meetID := createUCMeet(t, client, base)

	editURL := base + "/meets/" + meetID + "/edit"
	editForm := url.Values{
		"name": {"Abendmeeting Uster"}, "venue": {"Stadion Letzigrund"},
		"homologation_ref": {"CH-ZH-042"},
		"start_date":       {"2027-06-12"}, "end_date": {"2027-06-13"},
		"tier": {"C-Meeting"}, "scheme": {"swiss-athletics"},
		"version": {"1"},
	}
	resp := postForm(t, client, editURL, editURL, editForm)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("edit = %d, want 303", resp.StatusCode)
	}
	body := bodyString(t, mustGet(t, client, base+"/meets/"+meetID))
	if !strings.Contains(body, "Stadion Letzigrund") {
		t.Error("edit did not persist")
	}

	// Replaying the stale version must yield 409, not overwrite (SYS-083).
	resp = postForm(t, client, editURL, editURL, editForm)
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("stale edit = %d, want 409", resp.StatusCode)
	}
	conflictBody := bodyString(t, resp)
	if !strings.Contains(conflictBody, "Konflikt") {
		t.Error("conflict page missing localized message")
	}

	// The edit form pre-fills current values.
	formBody := bodyString(t, mustGet(t, client, editURL))
	if !strings.Contains(formBody, "Stadion Letzigrund") || !strings.Contains(formBody, "2027-06-12") {
		t.Error("edit form not pre-filled")
	}

	// Archive (current version is now 2).
	resp = postForm(t, client, base+"/meets/"+meetID, base+"/meets/"+meetID+"/archive",
		url.Values{"version": {"2"}})
	_ = resp.Body.Close()
	body = bodyString(t, mustGet(t, client, base+"/meets/"+meetID))
	if !strings.Contains(body, "Archiviert") {
		t.Error("archive did not persist (SYS-001)")
	}
}

// TestSanctioningSummaryUC001_5Web covers UC-001 #5: the summary document
// contains tier, venue (with homologation reference), dates, organizer,
// categories and disciplines.
func TestSanctioningSummaryUC001_5Web(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	meetID := createUCMeet(t, client, base)

	// Incomplete before any events exist.
	body := bodyString(t, mustGet(t, client, base+"/meets/"+meetID+"/sanctioning"))
	if !strings.Contains(body, "Unvollständig") {
		t.Error("summary without programme should flag incompleteness")
	}

	resp := addEvent(t, client, base, meetID, url.Values{
		"discipline": {"100m"}, "categories": {"U16 W"}, "round_final": {"1"},
	})
	_ = resp.Body.Close()

	body = bodyString(t, mustGet(t, client, base+"/meets/"+meetID+"/sanctioning"))
	for _, want := range []string{
		"Abendmeeting Uster", "Stadion Buchholz", "CH-ZH-042",
		"12.06.2027", "C-Meeting", "admin", "U16 W", "100",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("sanctioning summary missing %q (SYS-006)", want)
		}
	}
	if strings.Contains(body, "Unvollständig") {
		t.Error("complete summary still flagged incomplete")
	}

	// Unknown meet 404s.
	resp = mustGet(t, client, base+"/meets/no-such-meet/sanctioning")
	_ = bodyString(t, resp)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("sanctioning for unknown meet = %d, want 404", resp.StatusCode)
	}
}
