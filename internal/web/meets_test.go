// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
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

// hrefRE pulls every anchor target out of rendered HTML, for the TASK-043
// link-walk test below.
var hrefRE = regexp.MustCompile(`href="([^"]*)"`)

// hasLink reports whether body renders an <a>/<form> pointing at exactly
// path (as opposed to a same-prefix path, e.g. "/meets/X/entries" vs.
// "/meets/X/entries/exceptions" — a plain strings.Contains on the bare path
// would false-positive on the longer one).
func hasLink(body, path string) bool {
	return strings.Contains(body, `href="`+path+`"`) || strings.Contains(body, `action="`+path+`"`)
}

// TestMeetDetailHubOfficeAccessTASK043 covers OQ-111/TASK-043 (SYS-090/091,
// SYS-114): the meet-detail hub — previously organizer()-only, so a
// competition-office session 403ed out of its own highest-frequency
// surfaces (check-in, seeding, timing exchange, entries import/eligibility,
// privacy) — now opens to office sessions with a capability-filtered
// action list. Organizer-only actions (edit, archive, publish, sanctioning,
// bib/fee/exception management, programme/timetable mutation) stay hidden
// for office and their underlying routes stay organizer-gated (proved by
// TestMeetEditConflictAndArchive/TestMeetsRequireOrganizerRole, unmodified
// by this task). Field-official and entry-submitter sessions keep today's
// behavior (403), unaffected by the office-level widening.
func TestMeetDetailHubOfficeAccessTASK043(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base) // bootstrap admin, meet-organizer-capable
	meetID := createUCMeet(t, client, base)
	meetPage := base + "/meets/" + meetID

	// A programme entry with a final round populates the checkin/seeding
	// (office-permitted) row and the timetable's unit-scheduling
	// (organizer-only) row, so both are exercised below.
	resp := addEvent(t, client, base, meetID, url.Values{
		"discipline": {"100m"}, "categories": {"U16 W"}, "round_final": {"1"},
	})
	_ = resp.Body.Close()

	d, err := deps.meets.Meet(context.Background(), meetID)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Programme) != 1 || len(d.Programme[0].Rounds) != 1 {
		t.Fatalf("fixture programme = %+v, want one event with one round", d.Programme)
	}
	eventID := d.Programme[0].ID
	roundID := d.Programme[0].Rounds[0].ID
	if len(d.Units) != 1 {
		t.Fatalf("fixture units = %+v, want one unit", d.Units)
	}
	unitID := d.Units[0].UnitID

	organizerOnlyLinks := []string{
		meetIDPath(meetID, "/edit"),
		meetIDPath(meetID, "/archive/confirm"),
		meetIDPath(meetID, "/entries/exceptions"),
		meetIDPath(meetID, "/bibs"),
		meetIDPath(meetID, "/fees"),
		meetIDPath(meetID, "/sanctioning"),
		meetIDPath(meetID, "/events"),
		meetIDPath(meetID, "/units/"+unitID+"/schedule"),
		meetIDPath(meetID, "/timetable/publish"),
	}
	officePermittedLinks := []string{
		meetIDPath(meetID, "/roster"),
		meetIDPath(meetID, "/standings"),
		meetIDPath(meetID, "/officials"),
		meetIDPath(meetID, "/privacy"),
		meetIDPath(meetID, "/timing"),
		meetIDPath(meetID, "/entries"),
		meetIDPath(meetID, "/entries/import"),
		meetIDPath(meetID, "/entries/eligibility"),
		meetIDPath(meetID, "/events/"+eventID+"/checkin"),
		meetIDPath(meetID, "/events/"+eventID+"/rounds/"+roundID+"/seeding"),
		// TASK-046 (OQ-113, SYS-151): the hub gained a forward link into the
		// capture index and reconciliation, previously reachable only via
		// the field-official dashboard panel or a typed URL.
		meetIDPath(meetID, "/capture"),
		meetIDPath(meetID, "/reconciliation"),
	}

	// --- Organizer view: unchanged — every action present. ---
	organizerBody := bodyString(t, mustGet(t, client, meetPage))
	for _, path := range append(append([]string{}, organizerOnlyLinks...), officePermittedLinks...) {
		if !hasLink(organizerBody, path) {
			t.Errorf("organizer hub missing %q (TASK-043 must leave the organizer view unchanged)", path)
		}
	}

	// --- Provision office/field-official/entry-submitter accounts. ---
	createAccountWeb(t, client, base, "office1", "competition_office")
	createAccountWeb(t, client, base, "fo1", "field_official")
	createAccountWeb(t, client, base, "sub1", "entry_submitter")

	// Field-official and entry-submitter sessions keep today's behavior:
	// still below the office floor the hub now requires, still 403.
	for _, cred := range []string{"fo1", "sub1"} {
		logout(t, client, base)
		login(t, client, base, cred, "s3cret-passphrase")
		resp := mustGet(t, client, meetPage)
		_ = bodyString(t, resp)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s GET /meets/%s = %d, want 403 (unchanged floor)", cred, meetID, resp.StatusCode)
		}
	}

	// --- Office session: hub opens, capability-filtered. ---
	logout(t, client, base)
	login(t, client, base, "office1", "s3cret-passphrase")

	resp = mustGet(t, client, meetPage)
	officeBody := bodyString(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("office GET /meets/%s = %d, want 200 (TASK-043, OQ-111)", meetID, resp.StatusCode)
	}

	for _, path := range organizerOnlyLinks {
		if hasLink(officeBody, path) {
			t.Errorf("office hub renders organizer-only action %q — authz weakened (TASK-043)", path)
		}
	}
	for _, path := range officePermittedLinks {
		if !hasLink(officeBody, path) {
			t.Errorf("office hub missing office-permitted action %q", path)
		}
	}

	// --- Link-walk: every href the office hub renders must resolve
	// non-403 for the office session (the row's stated acceptance
	// criterion — not just the hub's own 200). ---
	walked := 0
	for _, m := range hrefRE.FindAllStringSubmatch(officeBody, -1) {
		href := m[1]
		if !strings.HasPrefix(href, "/meets/") && !strings.HasPrefix(href, "/m/") {
			continue
		}
		walked++
		resp := mustGet(t, client, base+href)
		_ = bodyString(t, resp)
		if resp.StatusCode == http.StatusForbidden {
			t.Errorf("office session: rendered link %q resolves 403", href)
		}
	}
	if walked == 0 {
		t.Fatal("link-walk found no /meets or /m links to check — test fixture broken")
	}
}

// meetIDPath builds a /meets/{id}{suffix} path, matching how meetDetailPage
// (meets.templ) builds its hrefs.
func meetIDPath(meetID, suffix string) string {
	return "/meets/" + meetID + suffix
}

// TestRosterBackToMeetOfficeSessionTASK043 is the exact TASK-042/OQ-111
// repro: an office session lands on the roster page (its office-dashboard
// entry point), follows the page's "back to meet" link, and must land on
// the meet-detail hub rather than a 403 — the link (roster.back_to_meet)
// has always pointed at /meets/{id}; the bug was that route being
// organizer()-only.
func TestRosterBackToMeetOfficeSessionTASK043(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	meetID := createUCMeet(t, client, base)

	createAccountWeb(t, client, base, "office2", "competition_office")
	logout(t, client, base)
	login(t, client, base, "office2", "s3cret-passphrase")

	rosterBody := bodyString(t, mustGet(t, client, base+"/meets/"+meetID+"/roster"))
	backLink := "/meets/" + meetID
	if !hasLink(rosterBody, backLink) {
		t.Fatalf("roster page missing roster.back_to_meet link to %q", backLink)
	}

	resp := mustGet(t, client, base+backLink)
	_ = bodyString(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("office session following roster.back_to_meet = %d, want 200 (TASK-042/OQ-111 repro)", resp.StatusCode)
	}
}

// crawlLinks performs a breadth-first walk of every <a>/<form> target
// rendered starting from startPath (TASK-046, SYS-151/UC-041 #1/#3: "every
// operator surface SHALL be reachable through rendered links … within two
// link activations"). It follows only same-origin "/", "/meets/…" and
// "/m/…" targets (the operator/public surfaces this task cares about — not
// /admin, /login, static assets, or service-worker scope), for up to
// maxActivations hops, asserting along the way that nothing it reaches
// resolves 403 (a 403 there means a rendered link points at a surface its
// own role cannot open — either a stale/typed-URL-only route or a broken
// capability gate). It returns every path it actually requested, so a
// caller can assert a specific surface was reached within budget.
func crawlLinks(t *testing.T, client *http.Client, base, startPath string, maxActivations int) map[string]bool {
	t.Helper()
	visited := map[string]bool{}
	frontier := []string{startPath}
	for hop := 0; hop <= maxActivations && len(frontier) > 0; hop++ {
		var next []string
		for _, path := range frontier {
			if visited[path] {
				continue
			}
			visited[path] = true
			resp := mustGet(t, client, base+path)
			body := bodyString(t, resp)
			if resp.StatusCode == http.StatusForbidden {
				t.Errorf("crawl from %q: rendered link %q resolves 403 (typed-URL-only surface or broken gate)", startPath, path)
			}
			for _, m := range hrefRE.FindAllStringSubmatch(body, -1) {
				href := m[1]
				if href == "" || strings.Contains(href, "#") || strings.HasPrefix(href, "http") {
					continue
				}
				if href != "/" && href != "/meets" && !strings.HasPrefix(href, "/meets/") && !strings.HasPrefix(href, "/m/") {
					continue
				}
				if !visited[href] {
					next = append(next, href)
				}
			}
		}
		frontier = next
	}
	return visited
}

// TestHomeLinkWalkReachabilityUC041_1And3 covers UC-041 #1 and #3 (SYS-151,
// F5): starting from "/", each role's primary day-of surfaces are reachable
// within two link activations and every link the walk follows resolves
// non-403 — no operator surface requires a typed URL. This extends the
// TASK-043 link-walk pattern (TestMeetDetailHubOfficeAccessTASK043, which
// walks from the hub itself) one level further back, to the actual role
// landing page.
func TestHomeLinkWalkReachabilityUC041_1And3(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base) // admin, organizer-capable
	meetID := createUCMeet(t, client, base)
	resp := addEvent(t, client, base, meetID, url.Values{
		"discipline": {"100m"}, "categories": {"U16 W"}, "round_final": {"1"},
	})
	_ = resp.Body.Close()

	createAccountWeb(t, client, base, "office9", "competition_office")
	createAccountWeb(t, client, base, "fo9", "field_official")
	acctID := accountIDFromAdminPage(t, bodyString(t, mustGet(t, client, base+"/admin")), "fo9")
	d, err := deps.meets.Meet(context.Background(), meetID)
	if err != nil || len(d.Units) != 1 {
		t.Fatalf("fixture units = %+v, err = %v, want one unit", d.Units, err)
	}
	resp = postForm(t, client, base+"/meets/"+meetID+"/officials", base+"/meets/"+meetID+"/officials/assign", url.Values{
		"account_id": {acctID}, "unit_id": {d.Units[0].UnitID},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("assign field official unit = %d, want 303", resp.StatusCode)
	}

	// --- Office: check-in, capture, reconciliation, roster, standings all
	// reachable within two link activations from "/" (UC-041 #1). ---
	logout(t, client, base)
	login(t, client, base, "office9", "s3cret-passphrase")
	visited := crawlLinks(t, client, base, "/", 2)
	for _, want := range []string{
		meetIDPath(meetID, "/roster"),
		meetIDPath(meetID, "/standings"),
		meetIDPath(meetID, "/capture"),
		meetIDPath(meetID, "/reconciliation"),
	} {
		if !visited[want] {
			t.Errorf("office: %q not reached within two link activations from / (visited: %v)", want, visited)
		}
	}
	if !visited[meetIDPath(meetID, "/events/"+d.Programme[0].ID+"/checkin")] {
		t.Errorf("office: check-in not reached within two link activations from / (visited: %v)", visited)
	}

	// --- Field official: its assigned unit's capture page is reachable in
	// one activation, with no other operator surface needing a typed URL. ---
	logout(t, client, base)
	login(t, client, base, "fo9", "s3cret-passphrase")
	visited = crawlLinks(t, client, base, "/", 2)
	if !visited[meetIDPath(meetID, "/capture/"+d.Units[0].UnitID)] {
		t.Errorf("field official: assigned unit's capture page not reached from / (visited: %v)", visited)
	}

	// --- Organizer: unchanged /meets → hub flow stays link-walkable too. ---
	logout(t, client, base)
	login(t, client, base, "admin", "s3cret-passphrase")
	visited = crawlLinks(t, client, base, "/", 2)
	if !visited["/meets"] {
		t.Errorf("organizer: /meets not reached from / (visited: %v)", visited)
	}
	if !visited[meetIDPath(meetID, "")] {
		t.Errorf("organizer: meet hub not reached within two link activations from / (visited: %v)", visited)
	}
}
