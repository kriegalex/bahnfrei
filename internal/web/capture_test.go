// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// ukcCaptureFixture creates a UKC template meet with two W12 girls over the
// operator forms and returns the meet ID and unit IDs by discipline name.
func ukcCaptureFixture(t *testing.T, client *http.Client, base string) (meetID string, units map[string]string) {
	t.Helper()
	resp := postForm(t, client, base+"/meets/from-template", base+"/meets/from-template", url.Values{
		"template": {"ubs-kids-cup"}, "date": {"2026-08-15"}, "venue": {"Le Mouret"},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create template meet = %d, want 303", resp.StatusCode)
	}
	meetID = strings.TrimPrefix(resp.Header.Get("Location"), "/meets/")

	for _, athlete := range []url.Values{
		{"first_name": {"Anna"}, "last_name": {"Muster"}, "birth_year": {"2014"}, "sex": {"W"}, "bib": {"101"}},
		{"first_name": {"Bea"}, "last_name": {"Beispiel"}, "birth_year": {"2014"}, "sex": {"W"}, "bib": {"102"}},
	} {
		resp = postForm(t, client, base+"/meets/"+meetID+"/roster", base+"/meets/"+meetID+"/roster", athlete)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("register participant = %d, want 303", resp.StatusCode)
		}
	}

	// The capture index links every capturable unit; map them by link text.
	body := bodyString(t, mustGet(t, client, base+"/meets/"+meetID+"/capture"))
	units = map[string]string{}
	re := regexp.MustCompile(`/meets/` + meetID + `/capture/([0-9A-Za-z]+)">([^<]+)<`)
	for _, m := range re.FindAllStringSubmatch(body, -1) {
		units[m[2]] = m[1]
	}
	if len(units) != 3 {
		t.Fatalf("capture index units = %v, want the 3 UKC disciplines", units)
	}
	return meetID, units
}

func TestCaptureRequiresFieldOfficialRole(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)

	for _, path := range []string{"/meets/some-id/capture", "/meets/some-id/capture/u1", "/meets/some-id/capture/u1/standings"} {
		resp := mustGet(t, client, base+path)
		_ = bodyString(t, resp)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("anonymous GET %s = %d, want 403 (SYS-090)", path, resp.StatusCode)
		}
	}
}

// TestUC011_CaptureGridFlow drives the horizontal-attempt grid in the
// browser: attempts save per cell, the grid re-renders X/–/marks, and the
// standings apply the tie-break with points from the official table.
func TestUC011_CaptureGridFlow(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	meetID, units := ukcCaptureFixture(t, client, base)
	unitURL := base + "/meets/" + meetID + "/capture/" + units["Zone Long Jump (UKC)"]

	// The grid shows 3 trial columns (template rule data) and no wind
	// inputs (ZoneLJ is not wind-relevant).
	body := bodyString(t, mustGet(t, client, unitURL))
	for _, want := range []string{"V1", "V2", "V3", "Anna Muster", "Bea Beispiel"} {
		if !strings.Contains(body, want) {
			t.Errorf("capture grid misses %q", want)
		}
	}
	if strings.Contains(body, `name="wind"`) {
		t.Error("ZoneLJ grid must not offer wind inputs (catalog data)")
	}

	athletes := athleteIDsFrom(t, body)
	saveAttempt := func(athlete, seq, value string) *http.Response {
		t.Helper()
		resp := postForm(t, client, unitURL, unitURL+"/attempt", url.Values{
			"athlete": {athlete}, "seq": {seq}, "value": {value}, "version": {"0"},
		})
		_ = bodyString(t, resp)
		return resp
	}
	// Anna (bib 101): 3.42, X — Bea (bib 102): 3.42, 3.30.
	for _, in := range [][3]string{
		{athletes["101"], "1", "3.42"}, {athletes["101"], "2", "X"},
		{athletes["102"], "1", "3.42"}, {athletes["102"], "2", "3.30"},
	} {
		if resp := saveAttempt(in[0], in[1], in[2]); resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("save attempt %v = %d, want 303", in, resp.StatusCode)
		}
	}

	body = bodyString(t, mustGet(t, client, unitURL))
	if !strings.Contains(body, "X") || !strings.Contains(body, "3.42") {
		t.Error("grid does not render the captured series")
	}
	// Standings: equal bests, Bea's 3.30 next-best wins (UC-011 #2); both
	// carry points from the official table.
	standings := bodyString(t, mustGet(t, client, unitURL+"/standings"))
	beaPos := strings.Index(standings, "Bea Beispiel")
	annaPos := strings.Index(standings, "Anna Muster")
	if beaPos == -1 || annaPos == -1 || beaPos > annaPos {
		t.Errorf("standings order wrong (next-best tie-break): bea@%d anna@%d", beaPos, annaPos)
	}

	// An invalid mark re-renders with a localized error (422).
	if resp := saveAttempt(athletes["101"], "3", "banana"); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("invalid mark = %d, want 422", resp.StatusCode)
	}
	// A concurrent capture of the same trial is a surfaced conflict
	// carrying the stored value (UC-021 #2).
	resp := postForm(t, client, unitURL, unitURL+"/attempt", url.Values{
		"athlete": {athletes["101"]}, "seq": {"1"}, "value": {"3.10"}, "version": {"0"},
	})
	conflictBody := bodyString(t, resp)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("conflicting capture = %d, want 422", resp.StatusCode)
	}
	if !strings.Contains(conflictBody, "3.42") {
		t.Error("conflict message must show the stored attempt (UC-021 #2)")
	}
}

// TestUC010_TrackCaptureFlow drives the 60 m manual-time form: round-up +
// hand-timed marking on the page (SYS-041), DQ rule enforcement (SYS-045),
// and the SSE results event for live standings (UC-011 #4 transport).
func TestUC010_TrackCaptureFlow(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	meetID, units := ukcCaptureFixture(t, client, base)
	unitURL := base + "/meets/" + meetID + "/capture/" + units["60 metres"]

	body := bodyString(t, mustGet(t, client, unitURL))
	athletes := athleteIDsFrom(t, body)

	// Live-update transport: subscribe to the meet topic before capturing.
	events, cancel := deps.bus.Subscribe("meet-" + meetID)
	defer cancel()

	resp := postForm(t, client, unitURL, unitURL+"/track", url.Values{
		"athlete": {athletes["101"]}, "time": {"9.32"}, "timing": {"manual"},
	})
	_ = bodyString(t, resp)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("manual time = %d, want 303", resp.StatusCode)
	}
	select {
	case ev := <-events:
		if ev.Name != "results" {
			t.Errorf("SSE event = %q, want results", ev.Name)
		}
	default:
		t.Error("no SSE event published on capture (UC-011 #4)")
	}

	// The page shows the rounded, provenance-marked hand time (D5.1).
	body = bodyString(t, mustGet(t, client, unitURL))
	if !strings.Contains(body, "9.4 h") {
		t.Errorf("page misses the rounded hand time %q", "9.4 h")
	}

	// DQ without a rule reference is rejected (SYS-045); with one it lands.
	resp = postForm(t, client, unitURL, unitURL+"/track", url.Values{
		"athlete": {athletes["102"]}, "status": {"DQ"},
	})
	_ = bodyString(t, resp)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("DQ without rule = %d, want 422", resp.StatusCode)
	}
	resp = postForm(t, client, unitURL, unitURL+"/track", url.Values{
		"athlete": {athletes["102"]}, "status": {"DQ"}, "status_detail": {"TR16.8"},
	})
	_ = bodyString(t, resp)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("DQ with rule = %d, want 303", resp.StatusCode)
	}
	body = bodyString(t, mustGet(t, client, unitURL+"/standings"))
	if !strings.Contains(body, "DQ (TR16.8)") {
		t.Error("standings must render the DQ with its rule reference (SYS-045)")
	}
}

// athleteIDsFrom maps bib → athlete ID from the capture page's hidden
// athlete inputs (each cell form names its row's athlete).
func athleteIDsFrom(t *testing.T, body string) map[string]string {
	t.Helper()
	out := map[string]string{}
	// Rows render "<td>{bib}</td><td>{name}</td>…" followed by the
	// athlete's hidden input (field grid) or form= references (track form).
	for _, re := range []*regexp.Regexp{
		regexp.MustCompile(`(?s)<tr>\s*<td>(\d+)</td>.*?name="athlete" value="([0-9A-Za-z]+)"`),
		regexp.MustCompile(`(?s)<tr>\s*<td>(\d+)</td>.*?form="track-([0-9A-Za-z]+)"`),
	} {
		for _, m := range re.FindAllStringSubmatch(body, -1) {
			out[m[1]] = m[2]
		}
		if len(out) > 0 {
			break
		}
	}
	if len(out) == 0 {
		t.Fatal("no athlete IDs found on the capture page")
	}
	return out
}
