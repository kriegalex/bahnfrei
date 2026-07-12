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

	"github.com/kriegalex/bahnfrei/internal/app"
	"github.com/kriegalex/bahnfrei/internal/domain"
)

// scheduleAndPublishTimetable schedules meetID's single unit and publishes
// the timetable, over the organizer's HTTP forms — the shared setup every
// public-timetable-touching test in this file needs.
func scheduleAndPublishTimetable(t *testing.T, client *http.Client, base, meetID string) {
	t.Helper()
	d := bodyString(t, mustGet(t, client, base+"/meets/"+meetID))
	// The unit-schedule form action embeds the unit ID; extract it rather
	// than re-deriving it (matches the meets_test.go/capture_test.go style
	// of reading IDs back from rendered HTML where a regex is simplest).
	const marker = "/units/"
	i := strings.Index(d, marker)
	if i < 0 {
		t.Fatalf("meet page has no schedulable unit: %s", d)
	}
	rest := d[i+len(marker):]
	unitID := rest[:strings.Index(rest, "/schedule")]

	meetPage := base + "/meets/" + meetID
	resp := postForm(t, client, meetPage, meetPage+"/units/"+unitID+"/schedule", url.Values{
		"scheduled_at": {"2027-06-12T14:30"}, "location": {"Bahn 1"}, "version": {"1"},
	})
	_ = resp.Body.Close()
	if strings.Contains(resp.Header.Get("Location"), "err=") {
		t.Fatalf("schedule unit redirected to %q", resp.Header.Get("Location"))
	}
	resp = postForm(t, client, meetPage, meetPage+"/timetable/publish", url.Values{})
	_ = resp.Body.Close()
	if strings.Contains(resp.Header.Get("Location"), "err=") {
		t.Fatalf("publish timetable redirected to %q", resp.Header.Get("Location"))
	}
}

// TestPublicPagesAnonymousAccessSYS070UC017_2 covers SYS-070/UC-017 #2: the
// meet overview, timetable, start lists and results all fetch with no
// session cookie, return 200, render without a login wall, and are usable
// on a 360px viewport (the shared layout carries the viewport meta tag).
func TestPublicPagesAnonymousAccessSYS070UC017_2(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	meetID := createUCMeet(t, client, base)

	resp := addEvent(t, client, base, meetID, url.Values{
		"discipline": {"100m"}, "categories": {"U16 W"}, "round_final": {"1"},
	})
	_ = resp.Body.Close()
	scheduleAndPublishTimetable(t, client, base, meetID)

	anon, _ := newTestClient(t, deps) // fresh cookie jar: no session cookie at all
	for _, tc := range []struct {
		path string
		want []string
	}{
		{"/m/" + meetID, []string{"Abendmeeting Uster", "Stadion Buchholz"}},
		{"/m/" + meetID + "/timetable", []string{"Bahn 1"}},
		{"/m/" + meetID + "/startlists", []string{"Startlisten"}},
		{"/m/" + meetID + "/results", []string{"Resultate"}},
	} {
		resp := mustGet(t, anon, base+tc.path)
		body := bodyString(t, resp)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("anonymous GET %s = %d, want 200: %s", tc.path, resp.StatusCode, body)
		}
		if !strings.Contains(body, `name="viewport"`) {
			t.Errorf("%s missing viewport meta tag (SYS-070/UC-017 #2, 360px usability)", tc.path)
		}
		if strings.Contains(body, `action="/login"`) {
			t.Errorf("%s renders a login wall for an anonymous visitor", tc.path)
		}
		for _, want := range tc.want {
			if !strings.Contains(body, want) {
				t.Errorf("%s missing %q: %s", tc.path, want, body)
			}
		}
	}
}

// TestPublicPagesArchivedMeetUC017_3 covers UC-017 #3: a closed/archived
// meet's public URLs keep serving the archived results at the same stable
// URLs (SYS-070 archive requirement).
func TestPublicPagesArchivedMeetUC017_3(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	meetID := createUCMeet(t, client, base)

	resp := addEvent(t, client, base, meetID, url.Values{
		"discipline": {"100m"}, "categories": {"U16 W"}, "round_final": {"1"},
	})
	_ = resp.Body.Close()
	scheduleAndPublishTimetable(t, client, base, meetID)

	resp = postForm(t, client, base+"/meets/"+meetID+"/roster", base+"/meets/"+meetID+"/roster", url.Values{
		"first_name": {"Anna"}, "last_name": {"Muster"},
		"birth_year": {"2011"}, "sex": {"W"}, "bib": {"1"},
	})
	_ = resp.Body.Close()
	participants, err := deps.results.Participants(context.Background(), meetID)
	if err != nil || len(participants) != 1 {
		t.Fatalf("participants = %v, %v", participants, err)
	}
	officeActor := app.Session{AccountID: "01ARC", Username: "office", Role: app.RoleCompetitionOffice}
	if _, err := deps.results.SaveResult(context.Background(), officeActor, meetID, app.ResultInput{
		AthleteID: participants[0].AthleteID, DisciplineCode: "100m",
		Mark: "13.20", Timing: domain.TimingElectronic,
	}); err != nil {
		t.Fatalf("SaveResult: %v", err)
	}

	// Archive the meet (current version is 1, untouched since creation).
	resp = postForm(t, client, base+"/meets/"+meetID, base+"/meets/"+meetID+"/archive", url.Values{"version": {"1"}})
	_ = resp.Body.Close()

	anon, _ := newTestClient(t, deps)
	for _, tc := range []struct {
		path string
		want string
	}{
		{"/m/" + meetID, "Abendmeeting Uster"},
		{"/m/" + meetID + "/timetable", "Bahn 1"},
		{"/m/" + meetID + "/startlists", "Anna Muster"},
		{"/m/" + meetID + "/results", "13.20"},
	} {
		resp := mustGet(t, anon, base+tc.path)
		body := bodyString(t, resp)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("archived meet GET %s = %d, want 200: %s", tc.path, resp.StatusCode, body)
		}
		if !strings.Contains(body, tc.want) {
			t.Errorf("archived meet %s missing %q (UC-017 #3): %s", tc.path, tc.want, body)
		}
	}
}

// TestPublicResultsLiveUpdateSYS071UC017_1 covers UC-017 #1/SYS-071: saving
// a confirmed result publishes synchronously on the meet's SSE topic, and a
// fresh fetch of the public results live-refresh fragment shows the new
// mark — the server-side half of the ≤10s public-update budget.
func TestPublicResultsLiveUpdateSYS071UC017_1(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	meetID := createUCMeet(t, client, base)

	resp := addEvent(t, client, base, meetID, url.Values{
		"discipline": {"100m"}, "categories": {"U16 W"}, "round_final": {"1"},
	})
	_ = resp.Body.Close()
	resp = postForm(t, client, base+"/meets/"+meetID+"/roster", base+"/meets/"+meetID+"/roster", url.Values{
		"first_name": {"Anna"}, "last_name": {"Muster"},
		"birth_year": {"2011"}, "sex": {"W"}, "bib": {"1"},
	})
	_ = resp.Body.Close()
	participants, err := deps.results.Participants(context.Background(), meetID)
	if err != nil || len(participants) != 1 {
		t.Fatalf("participants = %v, %v", participants, err)
	}

	events, cancel := deps.bus.Subscribe("meet-" + meetID)
	defer cancel()

	officeActor := app.Session{AccountID: "01LIV", Username: "office", Role: app.RoleCompetitionOffice}
	if _, err := deps.results.SaveResult(context.Background(), officeActor, meetID, app.ResultInput{
		AthleteID: participants[0].AthleteID, DisciplineCode: "100m",
		Mark: "12.34", Timing: domain.TimingElectronic,
	}); err != nil {
		t.Fatalf("SaveResult: %v", err)
	}

	// The publish must already be on the channel by the time SaveResult
	// returns — a non-blocking receive proves it happened synchronously
	// with the save, not on some later best-effort tick.
	select {
	case ev := <-events:
		if ev.Name != "results" {
			t.Errorf("event name = %q, want results", ev.Name)
		}
	default:
		t.Fatal("SaveResult did not publish synchronously on the meet's SSE topic")
	}

	anon, _ := newTestClient(t, deps)
	live := bodyString(t, mustGet(t, anon, base+"/m/"+meetID+"/results/live"))
	if !strings.Contains(live, "12.34") {
		t.Errorf("public results live fragment missing the new mark: %s", live)
	}
	if !strings.Contains(live, "Anna Muster") {
		t.Errorf("public results live fragment missing the athlete: %s", live)
	}
}

// TestPublicResultsSYS076Label covers UC-017 #5 across both organizer
// positioning choices and both launch languages: federation_official
// carries the unofficial-results label with the source name/link;
// primary carries no label at all.
func TestPublicResultsSYS076Label(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)

	for _, loc := range []string{"de", "fr"} {
		anon, _ := newTestClient(t, deps)
		_ = mustGet(t, anon, base+"/locale?lang="+loc).Body.Close()

		// federation_official with a named, linked source.
		meetID := createUCMeet(t, client, base)
		editURL := base + "/meets/" + meetID + "/edit"
		resp := postForm(t, client, editURL, editURL, url.Values{
			"name": {"Abendmeeting Uster"}, "venue": {"Stadion Buchholz"},
			"homologation_ref": {"CH-ZH-042"},
			"start_date":       {"2027-06-12"}, "end_date": {"2027-06-13"},
			"tier": {"C-Meeting"}, "scheme": {"swiss-athletics"}, "version": {"1"},
			"results_positioning":  {"federation_official"},
			"official_source_name": {"Swiss Athletics"},
			"official_source_url":  {"https://www.swiss-athletics.ch/results"},
		})
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("[%s] edit meet (federation_official) = %d, want 303", loc, resp.StatusCode)
		}

		body := bodyString(t, mustGet(t, anon, base+"/m/"+meetID+"/results"))
		unofficial := map[string]string{"de": "Inoffizielle Ergebnisse", "fr": "Résultats non officiels"}[loc]
		if !strings.Contains(body, unofficial) {
			t.Errorf("[%s] federation_official results page missing unofficial label: %s", loc, body)
		}
		if !strings.Contains(body, "Swiss Athletics") || !strings.Contains(body, "https://www.swiss-athletics.ch/results") {
			t.Errorf("[%s] federation_official results page missing source name/link: %s", loc, body)
		}

		// primary: no label at all.
		meetID2 := createUCMeet(t, client, base)
		editURL2 := base + "/meets/" + meetID2 + "/edit"
		resp = postForm(t, client, editURL2, editURL2, url.Values{
			"name": {"Abendmeeting Uster"}, "venue": {"Stadion Buchholz"},
			"homologation_ref": {"CH-ZH-042"},
			"start_date":       {"2027-06-12"}, "end_date": {"2027-06-13"},
			"tier": {"C-Meeting"}, "scheme": {"swiss-athletics"}, "version": {"1"},
			"results_positioning": {"primary"},
		})
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("[%s] edit meet (primary) = %d, want 303", loc, resp.StatusCode)
		}

		body = bodyString(t, mustGet(t, anon, base+"/m/"+meetID2+"/results"))
		if strings.Contains(body, unofficial) {
			t.Errorf("[%s] primary-positioned meet still shows the unofficial label: %s", loc, body)
		}
	}
}

// TestPublicResultsLocalizedDisciplineLabelsSYS074 covers SYS-074: the
// public results page renders discipline labels in the page's own
// language rather than the catalog's canonical (English) name.
func TestPublicResultsLocalizedDisciplineLabelsSYS074(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	meetID := createUCMeet(t, client, base)

	resp := addEvent(t, client, base, meetID, url.Values{
		"discipline": {"SP"}, "categories": {"U16 W"}, "round_final": {"1"},
	})
	_ = resp.Body.Close()
	// A discipline only appears as a standings column once someone is
	// registered in a division that spans it.
	resp = postForm(t, client, base+"/meets/"+meetID+"/roster", base+"/meets/"+meetID+"/roster", url.Values{
		"first_name": {"Anna"}, "last_name": {"Muster"},
		"birth_year": {"2011"}, "sex": {"W"}, "bib": {"1"},
	})
	_ = resp.Body.Close()

	cases := []struct {
		loc     string
		want    string
		notWant string
	}{
		{"de", "Kugelstossen", "Shot Put"},
		{"fr", "Lancer du poids", "Shot Put"},
	}
	for _, tc := range cases {
		anon, _ := newTestClient(t, deps)
		_ = mustGet(t, anon, base+"/locale?lang="+tc.loc).Body.Close()
		body := bodyString(t, mustGet(t, anon, base+"/m/"+meetID+"/results"))
		if !strings.Contains(body, tc.want) {
			t.Errorf("[%s] results page missing localized discipline name %q: %s", tc.loc, tc.want, body)
		}
		if strings.Contains(body, tc.notWant) {
			t.Errorf("[%s] results page leaked the unlocalized catalog name %q", tc.loc, tc.notWant)
		}
	}
}

// isoDateRE matches a full YYYY-MM-DD date — the shape SYS-100 forbids on
// public surfaces for anything but the meet's own start/end dates (an
// athlete's full birth date, "beyond category-implied birth year", must
// never appear).
var isoDateRE = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}\b`)

// TestPublicPageMinimizationSYS100UC023_1 is the automated PII scanner
// UC-023 #1 requires: every public page type is crawled and asserted to
// expose at most the SYS-100 allowed fields. Concretely: the birth YEAR
// (category-implied) appears, but the only full ISO dates rendered
// anywhere on the page are the meet's own start/end dates — never a
// participant's full birth date (this system's roster and online-entry
// forms do not even collect one today; this test is the regression guard
// for when a richer person-data field lands) — and no licence-number-
// shaped token (the "SA-" Swiss Athletics licence prefix, ExternalIDs'
// namespace) ever appears, matching the central
// domain.PublicDisplayNameFor/PublicDisplayClubFor choke point every
// renderer here goes through. The fixture is a seeded meet (online
// entries confirmed through check-in, heats generated — TASK-016/018) so
// the start-list page's heat/lane sections are present and swept too, not
// just the flat roster.
func TestPublicPageMinimizationSYS100UC023_1(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	meetID, eventID, roundID := seededMeetFixture(t, deps, client, base, 3)
	registerRosterParticipant(t, client, base, meetID, nil)

	seedingPage := base + "/meets/" + meetID + "/events/" + eventID + "/rounds/" + roundID + "/seeding"
	genResp := postForm(t, client, seedingPage, seedingPage+"/generate", url.Values{"max_heat_size": {"8"}, "track_lanes": {"8"}})
	_ = genResp.Body.Close()

	anon, _ := newTestClient(t, deps)
	allowedDates := map[string]bool{"2027-06-12": true, "2027-06-13": true} // the meet's own start/end dates
	pagesWithParticipantRows := map[string]bool{"/m/" + meetID + "/startlists": true, "/m/" + meetID + "/results": true}
	for _, path := range []string{"/m/" + meetID, "/m/" + meetID + "/startlists", "/m/" + meetID + "/results", "/m/" + meetID + "/results/live"} {
		body := bodyString(t, mustGet(t, anon, base+path))
		if pagesWithParticipantRows[path] && !strings.Contains(body, "2011") {
			t.Errorf("%s missing the SYS-100-allowed birth year", path)
		}
		for _, m := range isoDateRE.FindAllString(body, -1) {
			if !allowedDates[m] {
				t.Errorf("%s exposes a full date %q beyond the meet's own dates (SYS-100 forbids a birth date beyond the year)", path, m)
			}
		}
		if strings.Contains(body, "SA-") {
			t.Errorf("%s exposes a licence-number-shaped token (SYS-100 forbids licence numbers on public surfaces)", path)
		}
	}

	// The heat sections must actually be on the swept start-list page —
	// otherwise the scan silently proved nothing about them.
	startlists := bodyString(t, mustGet(t, anon, base+"/m/"+meetID+"/startlists"))
	if !strings.Contains(startlists, "Athlete") {
		t.Fatalf("fixture error: start-list page has no seeded heat rows to scan: %s", startlists)
	}
}

// TestPublicHeatSheetConsentSuppressionSYS103UC023_2 covers SYS-103 for
// the TASK-018 heat/lane sections of the public start-list page: a
// withdrawn athlete's identity is suppressed in the heat rows (deny) while
// a non-withdrawn athlete in the same heat still shows (allow) — and the
// office seeding page keeps the real name (it is not a public surface;
// the office must be able to identify who they are seeding).
func TestPublicHeatSheetConsentSuppressionSYS103UC023_2(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	meetID, eventID, roundID := seededMeetFixture(t, deps, client, base, 2)

	// One more confirmed entry, publication withdrawn at submission time
	// (UC-023: the entry flow collects consent).
	ctx := context.Background()
	detail, err := deps.results.SubmitIndividualEntry(ctx, webSubmitter, meetID, app.IndividualEntryInput{
		EventID: eventID, FirstName: "Wanda", LastName: "Withdrawn",
		BirthYear: 2009, Sex: domain.SexFemale, SeedPerformance: "13.10",
		PublicationWithdrawn: true,
	})
	if err != nil {
		t.Fatalf("SubmitIndividualEntry: %v", err)
	}
	if err := deps.results.ConfirmCheckIn(ctx, webOffice, meetID, detail.ID, detail.Version); err != nil {
		t.Fatalf("ConfirmCheckIn: %v", err)
	}

	seedingPage := base + "/meets/" + meetID + "/events/" + eventID + "/rounds/" + roundID + "/seeding"
	genResp := postForm(t, client, seedingPage, seedingPage+"/generate", url.Values{"max_heat_size": {"8"}, "track_lanes": {"8"}})
	_ = genResp.Body.Close()

	anon, _ := newTestClient(t, deps)
	body := bodyString(t, mustGet(t, anon, base+"/m/"+meetID+"/startlists"))
	if strings.Contains(body, "Wanda Withdrawn") {
		t.Errorf("deny: public heat sheet leaked the withdrawn athlete's name: %s", body)
	}
	if !strings.Contains(body, "Athlete A") {
		t.Errorf("allow: public heat sheet missing the non-withdrawn athlete: %s", body)
	}
	if !strings.Contains(body, "—") {
		t.Errorf("public heat sheet missing the suppression marker: %s", body)
	}

	// The office seeding page is not a public surface: real name intact.
	officeBody := bodyString(t, mustGet(t, client, seedingPage))
	if !strings.Contains(officeBody, "Wanda Withdrawn") {
		t.Errorf("office seeding page must keep the real name (not a public surface): %s", officeBody)
	}
}

// TestPublicResultsConsentSuppressionSYS103UC023_2 covers SYS-103's allow
// and deny paths side by side on the same page: an athlete whose
// publication consent was withdrawn at entry is suppressed (deny) while
// another, ordinary athlete in the same division still shows their real
// name (allow) — proving suppression is per-athlete, not an
// accidental page-wide effect.
func TestPublicResultsConsentSuppressionSYS103UC023_2(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	meetID, units := ukcCaptureFixture(t, client, base)
	ljURL := base + "/meets/" + meetID + "/capture/" + units["Zone Long Jump (UKC)"]

	// Withdraw consent for bib 101 (Anna Muster) via the office privacy
	// worklist.
	privacyBody := bodyString(t, mustGet(t, client, base+"/meets/"+meetID+"/privacy"))
	i := strings.Index(privacyBody, "101")
	if i < 0 {
		t.Fatalf("privacy worklist missing bib 101: %s", privacyBody)
	}
	athleteID := athleteIDFromPrivacyPage(t, privacyBody[i:])
	resp := postForm(t, client, base+"/meets/"+meetID+"/privacy",
		base+"/meets/"+meetID+"/privacy/"+athleteID+"/consent", url.Values{"withdrawn": {"true"}})
	_ = resp.Body.Close()

	captureBody := bodyString(t, mustGet(t, client, ljURL))
	athletes := athleteIDsFrom(t, captureBody)
	resp = postForm(t, client, ljURL, ljURL+"/attempt", url.Values{
		"athlete": {athletes["101"]}, "seq": {"1"}, "value": {"4.12"}, "version": {"0"},
	})
	_ = bodyString(t, resp)

	anon, _ := newTestClient(t, deps)
	for _, path := range []string{"/m/" + meetID + "/startlists", "/m/" + meetID + "/results"} {
		body := bodyString(t, mustGet(t, anon, base+path))
		if strings.Contains(body, "Anna Muster") {
			t.Errorf("deny: %s leaked the withdrawn athlete's name", path)
		}
		if !strings.Contains(body, "Bea Beispiel") {
			t.Errorf("allow: %s missing the non-withdrawn athlete's name", path)
		}
		if !strings.Contains(body, "101") {
			t.Errorf("%s: bib 101 must still render (only identity is suppressed, not the row)", path)
		}
	}
}
