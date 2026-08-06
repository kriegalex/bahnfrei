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

// TestCaptureBulkDNSConfirmFlowSYS114SYS046Web drives the TASK-041 bulk
// "mark remaining as DNS" action end to end from the browser: the confirm
// sub-page (TASK-034 pattern) previews the candidate count, a plain
// navigation away (Cancel) applies nothing, and only its own POST marks the
// still-unresulted entrant DNS while leaving an already-captured result
// untouched.
func TestCaptureBulkDNSConfirmFlowSYS114SYS046Web(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	meetID, units := ukcCaptureFixture(t, client, base)
	unitPath := "/meets/" + meetID + "/capture/" + units["60 metres"]
	unitURL := base + unitPath

	body := bodyString(t, mustGet(t, client, unitURL))
	athletes := athleteIDsFrom(t, body)

	// Anna (101) gets a real hand time; Bea (102) is left uncaptured.
	resp := postForm(t, client, unitURL, unitURL+"/track", url.Values{
		"athlete": {athletes["101"]}, "time": {"9.32"}, "timing": {"manual"},
	})
	_ = bodyString(t, resp)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("manual time = %d, want 303", resp.StatusCode)
	}

	// The capture page offers the bulk-DNS confirm link while one entrant
	// remains unresulted.
	confirmURL := unitURL + "/bulk-dns/confirm"
	body = bodyString(t, mustGet(t, client, unitURL))
	if !strings.Contains(body, `href="`+unitPath+`/bulk-dns/confirm"`) {
		t.Fatal("capture page misses the bulk-DNS confirm link (TASK-041)")
	}

	// The confirm sub-page previews the exact candidate count (just Bea)
	// and posts to the bulk-dns route.
	confirmBody := bodyString(t, mustGet(t, client, confirmURL))
	if !strings.Contains(confirmBody, "1 Teilnehmende ohne erfasstes Resultat") {
		t.Errorf("confirm page must preview the candidate count: %s", confirmBody)
	}
	if !strings.Contains(confirmBody, `action="`+unitPath+`/bulk-dns"`) {
		t.Errorf("confirm page must post to the bulk-dns route: %s", confirmBody)
	}
	if !strings.Contains(confirmBody, `href="`+unitPath+`"`) {
		t.Errorf("confirm page must offer a Cancel link back to the unit: %s", confirmBody)
	}

	// Cancel: merely visiting (and navigating away from) the confirm page
	// applies nothing — Bea still has no settled result.
	standings := bodyString(t, mustGet(t, client, unitURL+"/standings"))
	if strings.Contains(standings, "Bea Beispiel") {
		t.Fatal("visiting the confirm page must not itself apply the bulk DNS")
	}

	// Confirm: POST applies — Bea becomes DNS, Anna's hand time survives.
	resp = postForm(t, client, confirmURL, unitURL+"/bulk-dns", url.Values{})
	_ = bodyString(t, resp)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("bulk-dns POST = %d, want 303", resp.StatusCode)
	}
	standings = bodyString(t, mustGet(t, client, unitURL+"/standings"))
	if !strings.Contains(standings, "9.4 h") {
		t.Error("Anna's captured time must survive the bulk action")
	}
	beaPos := strings.Index(standings, "Bea Beispiel")
	if beaPos == -1 || !strings.Contains(standings[beaPos:], "DNS") {
		t.Errorf("Bea must now show DNS: %s", standings)
	}

	// Nothing is left to mark: the link disappears from the capture page.
	body = bodyString(t, mustGet(t, client, unitURL))
	if strings.Contains(body, unitPath+"/bulk-dns/confirm") {
		t.Error("bulk-DNS link should not render once every entrant is resulted")
	}

	// TASK-047/SYS-152/UC-041 #4: navigating straight to the confirm URL
	// once nothing applies (a stale link, or a typed one) must still render
	// an explanatory state, not a live confirm button that would just
	// apply a no-op.
	staleConfirmBody := bodyString(t, mustGet(t, client, confirmURL))
	if strings.Contains(staleConfirmBody, `action="`+unitPath+`/bulk-dns"`) {
		t.Errorf("bulk-DNS confirm with nothing to mark must not render a live form: %s", staleConfirmBody)
	}
	if !strings.Contains(staleConfirmBody, "Nichts zu markieren") {
		t.Errorf("bulk-DNS confirm with nothing to mark must state why: %s", staleConfirmBody)
	}
}

// TestCaptureStandingsEmptyCollapsesToOneMessageSYS152UC041 covers F8: a
// field unit's standings always rank every entrant, resulted or not
// (RankFieldSeries), and a track unit with a category split could
// otherwise repeat an empty message per category heading — before any
// result is captured, both must collapse to one concise empty message
// instead of a full-header table of blank rows (field) or a repeated
// per-section empty paragraph.
func TestCaptureStandingsEmptyCollapsesToOneMessageSYS152UC041(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	meetID, units := ukcCaptureFixture(t, client, base)

	for _, name := range []string{"Zone Long Jump (UKC)", "60 metres"} {
		unitURL := base + "/meets/" + meetID + "/capture/" + units[name]
		standings := bodyString(t, mustGet(t, client, unitURL+"/standings"))
		if strings.Contains(standings, "<table") {
			t.Errorf("%s: empty standings must not render a table: %s", name, standings)
		}
		if strings.Contains(standings, "Anna Muster") || strings.Contains(standings, "Bea Beispiel") {
			t.Errorf("%s: empty standings must not leak blank participant rows: %s", name, standings)
		}
		if !strings.Contains(standings, "Noch keine Resultate erfasst.") {
			t.Errorf("%s: empty standings must show the concise empty message: %s", name, standings)
		}
	}
}

// TestCaptureWindAppliesUniformlySYS040UC010_4Web drives the per-race wind
// form: the reading applies to the whole race, not per athlete, and the
// input re-renders pre-filled with the stored value.
func TestCaptureWindAppliesUniformlySYS040UC010_4Web(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	meetID, units := ukcCaptureFixture(t, client, base)
	unitURL := base + "/meets/" + meetID + "/capture/" + units["60 metres"]

	resp := postForm(t, client, unitURL, unitURL+"/wind", url.Values{"wind": {"1.4"}})
	_ = bodyString(t, resp)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("save wind = %d, want 303", resp.StatusCode)
	}

	body := bodyString(t, mustGet(t, client, unitURL))
	if !strings.Contains(body, `name="wind" size="4" value="1.4"`) {
		t.Error("wind form does not re-render the stored per-race reading (SYS-040)")
	}

	// An unparseable wind value re-renders with a localized error (422).
	resp = postForm(t, client, unitURL, unitURL+"/wind", url.Values{"wind": {"gusty"}})
	invalid := bodyString(t, resp)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("unparseable wind = %d, want 422", resp.StatusCode)
	}
	if !strings.Contains(invalid, "ungültig") {
		t.Error("invalid wind value must show the localized capture error")
	}

	// An unknown unit under a valid, authorized meet surfaces the generic
	// not-found path rather than a crash.
	badURL := base + "/meets/" + meetID + "/capture/01NOUNIT"
	resp = postForm(t, client, unitURL, badURL+"/wind", url.Values{"wind": {"1.0"}})
	_ = bodyString(t, resp)
	if resp.StatusCode < 400 {
		t.Errorf("wind on an unknown unit = %d, want an error status", resp.StatusCode)
	}
}

// TestCaptureAnnounceAndCorrectionFlowSYS046SYS047UC015Web drives UC-015
// end to end from the browser: announcing opens the protest banner, plain
// capture is rejected afterwards, and a reasoned correction succeeds and
// re-announces (opening a fresh appeal window).
func TestCaptureAnnounceAndCorrectionFlowSYS046SYS047UC015Web(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	meetID, units := ukcCaptureFixture(t, client, base)
	unitURL := base + "/meets/" + meetID + "/capture/" + units["60 metres"]

	body := bodyString(t, mustGet(t, client, unitURL))
	athletes := athleteIDsFrom(t, body)
	resp := postForm(t, client, unitURL, unitURL+"/track", url.Values{
		"athlete": {athletes["101"]}, "time": {"9.32"}, "timing": {"manual"},
	})
	_ = bodyString(t, resp)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("initial capture = %d, want 303", resp.StatusCode)
	}

	// Before announcement, no protest banner and no announce button... the
	// office session on this page does see the announce button.
	body = bodyString(t, mustGet(t, client, unitURL))
	if !strings.Contains(body, "capture.protest.announce") && !strings.Contains(body, "Resultate publizieren") {
		t.Error("office session must see the announce action before the unit is announced")
	}

	resp = postForm(t, client, unitURL, unitURL+"/announce", url.Values{})
	_ = bodyString(t, resp)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("announce = %d, want 303", resp.StatusCode)
	}

	body = bodyString(t, mustGet(t, client, unitURL))
	if !strings.Contains(body, "Provisorisch") {
		t.Error("page must show the provisional protest-clock banner after announcing (UC-015 #1)")
	}

	// Plain capture is rejected once announced (UC-015 #2 boundary).
	resp = postForm(t, client, unitURL, unitURL+"/track", url.Values{
		"athlete": {athletes["101"]}, "time": {"9.20"}, "timing": {"manual"},
	})
	blocked := bodyString(t, resp)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("post-announcement /track = %d, want 422", resp.StatusCode)
	}
	if !strings.Contains(blocked, "bereits publiziert") {
		t.Error("blocked capture must explain a correction is required")
	}

	// A correction without a reason is rejected — OQ-075/UC-038 #4: the
	// error must name the "reason" field on this athlete's row (not just a
	// page-level flash) and the row's just-typed time must survive the
	// re-render.
	resp = postForm(t, client, unitURL, unitURL+"/correct", url.Values{
		"athlete": {athletes["101"]}, "time": {"9.20"}, "timing": {"manual"},
	})
	noReason := bodyString(t, resp)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("correction without reason = %d, want 422", resp.StatusCode)
	}
	if !strings.Contains(noReason, "erfordert einen Grund") {
		t.Error("correction-without-reason error must be shown")
	}
	reasonFieldID := "reason-" + athletes["101"]
	for _, want := range []string{
		`aria-invalid="true"`,
		`id="` + reasonFieldID + `-error"`,
		`value="9.20"`, // the rejected row's submitted time is preserved
	} {
		if !strings.Contains(noReason, want) {
			t.Errorf("correction-without-reason re-render missing %q: %s", want, noReason)
		}
	}

	// A reasoned correction succeeds and re-announces.
	resp = postForm(t, client, unitURL, unitURL+"/correct", url.Values{
		"athlete": {athletes["101"]}, "time": {"9.20"}, "timing": {"manual"}, "reason": {"re-timed from video"},
	})
	_ = bodyString(t, resp)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("reasoned correction = %d, want 303", resp.StatusCode)
	}
	body = bodyString(t, mustGet(t, client, unitURL))
	if !strings.Contains(body, "9.2 h") {
		t.Errorf("corrected hand time not shown on the page: %q missing", "9.2 h")
	}

	// An invalid correction payload (no timing method) falls through to the
	// generic invalid-input flash rather than a 500.
	resp = postForm(t, client, unitURL, unitURL+"/correct", url.Values{
		"athlete": {athletes["101"]}, "time": {"9.20"}, "reason": {"typo"},
	})
	badPayload := bodyString(t, resp)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("correction without a timing method = %d, want 422", resp.StatusCode)
	}
	if !strings.Contains(badPayload, "ungültig") {
		t.Error("invalid correction payload must show the generic capture error")
	}

	// Re-announcing an unknown unit surfaces an error, not a crash.
	resp = postForm(t, client, unitURL, base+"/meets/"+meetID+"/capture/01NOUNIT/announce", url.Values{})
	_ = bodyString(t, resp)
	if resp.StatusCode < 400 {
		t.Errorf("announce on an unknown unit = %d, want an error status", resp.StatusCode)
	}
}

// TestCaptureFieldCorrectionFlowSYS046SYS047UC015Web mirrors
// TestCaptureAnnounceAndCorrectionFlowSYS046SYS047UC015Web for the
// horizontal-attempt grid (TASK-040/DEC-024): before announcement the grid
// offers no correction columns; once announced, plain attempt capture is
// rejected, the per-trial cells turn read-only, and the row's settled-level
// correction form (reason required, mirroring the track flow) lands a
// corrected mark.
func TestCaptureFieldCorrectionFlowSYS046SYS047UC015Web(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	meetID, units := ukcCaptureFixture(t, client, base)
	unitURL := base + "/meets/" + meetID + "/capture/" + units["Zone Long Jump (UKC)"]

	body := bodyString(t, mustGet(t, client, unitURL))
	athletes := athleteIDsFrom(t, body)
	if strings.Contains(body, "capture-correction-notice") {
		t.Error("an unannounced field unit must not show the correction notice")
	}

	resp := postForm(t, client, unitURL, unitURL+"/attempt", url.Values{
		"athlete": {athletes["101"]}, "seq": {"1"}, "value": {"3.42"}, "version": {"0"},
	})
	_ = bodyString(t, resp)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("initial attempt = %d, want 303", resp.StatusCode)
	}

	resp = postForm(t, client, unitURL, unitURL+"/announce", url.Values{})
	_ = bodyString(t, resp)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("announce = %d, want 303", resp.StatusCode)
	}

	body = bodyString(t, mustGet(t, client, unitURL))
	if !strings.Contains(body, "Provisorisch") {
		t.Error("page must show the provisional protest-clock banner after announcing (UC-015 #1)")
	}
	if !strings.Contains(body, "capture-correction-notice") {
		t.Error("an announced field unit must show the correction notice")
	}
	if strings.Contains(body, `action="`+unitURL+`/attempt"`) {
		t.Error("per-trial attempt forms must not render once the unit is announced")
	}
	if !strings.Contains(body, ">NM<") {
		t.Error("the field correction status select must offer NM (no mark)")
	}

	// Plain attempt capture is rejected once announced (UC-015 #2 boundary),
	// mirroring the track/vertical boundary.
	resp = postForm(t, client, unitURL, unitURL+"/attempt", url.Values{
		"athlete": {athletes["101"]}, "seq": {"2"}, "value": {"X"}, "version": {"0"},
	})
	blocked := bodyString(t, resp)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("post-announcement /attempt = %d, want 422", resp.StatusCode)
	}
	if !strings.Contains(blocked, "bereits publiziert") {
		t.Error("blocked field capture must explain a correction is required")
	}

	// A correction without a reason is rejected, the row's just-typed mark
	// preserved (OQ-075/UC-038 #4's pattern, extended to the field grid).
	resp = postForm(t, client, unitURL, unitURL+"/correct", url.Values{
		"athlete": {athletes["101"]}, "time": {"3.55"},
	})
	noReason := bodyString(t, resp)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("field correction without reason = %d, want 422", resp.StatusCode)
	}
	reasonFieldID := "reason-" + athletes["101"]
	for _, want := range []string{
		`aria-invalid="true"`,
		`id="` + reasonFieldID + `-error"`,
		`value="3.55"`,
	} {
		if !strings.Contains(noReason, want) {
			t.Errorf("field correction-without-reason re-render missing %q: %s", want, noReason)
		}
	}

	// A reasoned correction succeeds, re-announces, and the corrected mark
	// shows on the page — standings/exports flow through the same
	// CorrectResult path TestCorrectFieldResultSYS046UC015_2 already proves
	// at the service level.
	resp = postForm(t, client, unitURL, unitURL+"/correct", url.Values{
		"athlete": {athletes["101"]}, "time": {"3.55"}, "reason": {"remeasurement found a transcription error"},
	})
	_ = bodyString(t, resp)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("reasoned field correction = %d, want 303", resp.StatusCode)
	}
	body = bodyString(t, mustGet(t, client, unitURL))
	if !strings.Contains(body, "3.55") {
		t.Errorf("corrected field mark not shown on the page: %q missing", "3.55")
	}
}

// TestCaptureAnnounceAndCorrectRequireOfficeRoleSYS090UC015Web: an
// unauthenticated caller cannot announce or correct — office-only actions
// (UC-015's actor line), same as every other capture-role floor.
func TestCaptureAnnounceAndCorrectRequireOfficeRoleSYS090UC015Web(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)

	for _, path := range []string{"/meets/some-id/capture/u1/announce", "/meets/some-id/capture/u1/correct"} {
		resp, err := client.PostForm(base+path, url.Values{})
		if err != nil {
			t.Fatal(err)
		}
		_ = bodyString(t, resp)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("anonymous POST %s = %d, want 403 (SYS-090)", path, resp.StatusCode)
		}
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
