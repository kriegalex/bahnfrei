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
