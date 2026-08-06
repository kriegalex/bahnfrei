// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	extract "github.com/ledongthuc/pdf"
)

// extractPDFText fetches url, asserts it is a well-formed PDF response, and
// parses the body back into plain text with an independent PDF library —
// so assertions check real document structure/content, not just non-empty
// bytes.
func extractPDFText(t *testing.T, client *http.Client, url string) string {
	t.Helper()
	resp := mustGet(t, client, url)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200", url, resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/pdf" {
		t.Errorf("Content-Type = %q, want application/pdf", ct)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		t.Fatalf("body does not start with a PDF header: %q", data[:min(16, len(data))])
	}
	r, err := extract.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("parse generated pdf: %v", err)
	}
	rd, err := r.GetPlainText()
	if err != nil {
		t.Fatalf("extract plain text: %v", err)
	}
	text, err := io.ReadAll(rd)
	if err != nil {
		t.Fatalf("read extracted text: %v", err)
	}
	return string(text)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// TestCaptureSheetPDFRequiresFieldOfficialRoleSYS072 mirrors
// TestCaptureRequiresFieldOfficialRole for the new printable route: an
// anonymous request is refused (SYS-090), not silently served.
func TestCaptureSheetPDFRequiresFieldOfficialRoleSYS072(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)

	resp := mustGet(t, client, base+"/meets/some-id/capture/some-unit/sheet.pdf")
	_ = bodyString(t, resp)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("anonymous GET sheet.pdf = %d, want 403 (SYS-090)", resp.StatusCode)
	}
}

// TestResultListPDFRequiresOfficeRoleSYS072 mirrors the capture-sheet role
// check for the result-list route.
func TestResultListPDFRequiresOfficeRoleSYS072(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)

	resp := mustGet(t, client, base+"/meets/some-id/standings.pdf")
	_ = bodyString(t, resp)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("anonymous GET standings.pdf = %d, want 403 (SYS-090)", resp.StatusCode)
	}
}

// TestCaptureSheetPDFFieldGridSYS072UC018_1 drives UC-018 #1 for a
// field-horizontal unit: the printed attempt grid carries the meet name,
// the localized discipline subtitle, a generation timestamp, the trial
// columns and every participant, matching the current capture data
// (a captured mark appears; an uncaptured trial stays a blank cell rather
// than a placeholder).
func TestCaptureSheetPDFFieldGridSYS072UC018_1(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	meetID, units := ukcCaptureFixture(t, client, base)
	unitURL := base + "/meets/" + meetID + "/capture/" + units["Zonen-Weitsprung (UKC)"]

	// The unit list offers a download link for the sheet.
	indexBody := bodyString(t, mustGet(t, client, base+"/meets/"+meetID+"/capture"))
	if !strings.Contains(indexBody, units["Zonen-Weitsprung (UKC)"]+"/sheet.pdf") {
		t.Error("capture index does not link the unit's PDF capture sheet")
	}

	// Capture one attempt so the sheet must reflect "current data", not a
	// blank template.
	body := bodyString(t, mustGet(t, client, unitURL))
	athletes := athleteIDsFrom(t, body)
	resp := postForm(t, client, unitURL, unitURL+"/attempt", url.Values{
		"athlete": {athletes["101"]}, "seq": {"1"}, "value": {"4.12"}, "version": {"0"},
	})
	_ = bodyString(t, resp)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("save attempt = %d, want 303", resp.StatusCode)
	}

	text := extractPDFText(t, client, unitURL+"/sheet.pdf")
	for _, want := range []string{
		"UBS Kids Cup Le Mouret 2026", // meet name
		"Zonen-Weitsprung (UKC)",      // localized discipline subtitle (DE default locale)
		"Erfassungsblatt",             // pdf.capture_sheet.title (DE)
		"V1", "V2", "V3",              // trial columns (Config.Attempts = 3)
		"101", "Anna Muster", "4.12", // captured row
		"102", "Bea Beispiel", // not-yet-captured row still lists identity
	} {
		if !strings.Contains(text, want) {
			t.Errorf("capture sheet text misses %q\n--- full text ---\n%s", want, text)
		}
	}
}

// TestCaptureSheetPDFTrackLaneSYS072UC018_1 drives UC-018 #1 for the track
// family: bib/name populated, lane and time columns present for manual
// capture (lane draws are TASK-018/M2 — the PoC leaves the lane column
// blank, a documented limitation).
func TestCaptureSheetPDFTrackLaneSYS072UC018_1(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	meetID, units := ukcCaptureFixture(t, client, base)
	unitURL := base + "/meets/" + meetID + "/capture/" + units["60 m"]

	text := extractPDFText(t, client, unitURL+"/sheet.pdf")
	for _, want := range []string{
		"UBS Kids Cup Le Mouret 2026",
		"60 m", // discipline.60m (DE)
		"Bahn", "Zeit",
		"101", "Anna Muster",
		"102", "Bea Beispiel",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("track lane sheet text misses %q\n--- full text ---\n%s", want, text)
		}
	}
}

// TestResultListPDFSYS072UC018_2 drives UC-018 #2: the printed result list
// mirrors the on-screen standings (division heading, rank/bib/name/total,
// captured marks with points), reusing TASK-007's standings computation
// unchanged.
func TestResultListPDFSYS072UC018_2(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	meetID, units := ukcCaptureFixture(t, client, base)
	ljURL := base + "/meets/" + meetID + "/capture/" + units["Zonen-Weitsprung (UKC)"]

	body := bodyString(t, mustGet(t, client, ljURL))
	athletes := athleteIDsFrom(t, body)
	resp := postForm(t, client, ljURL, ljURL+"/attempt", url.Values{
		"athlete": {athletes["101"]}, "seq": {"1"}, "value": {"4.12"}, "version": {"0"},
	})
	_ = bodyString(t, resp)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("save attempt = %d, want 303", resp.StatusCode)
	}

	// The standings page links the PDF download.
	standingsBody := bodyString(t, mustGet(t, client, base+"/meets/"+meetID+"/standings"))
	if !strings.Contains(standingsBody, "/standings.pdf") {
		t.Error("standings page does not link the PDF result list")
	}

	text := extractPDFText(t, client, base+"/meets/"+meetID+"/standings.pdf")
	for _, want := range []string{
		"UBS Kids Cup Le Mouret 2026",
		"Rangliste", // standings.title (DE), reused as the PDF's document title
		"W12",       // division code for a 2014-born girl in 2026
		"101", "Anna Muster", "4.12",
		"102", "Bea Beispiel",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("result list text misses %q\n--- full text ---\n%s", want, text)
		}
	}
}

// TestResultListPDFMinimizationSYS103UC023_2 covers UC-023 #1/#2 for the
// printed result list specifically: SYS-100 treats a "publication-
// intended export" the same as a public web page (a result list is meant
// to be posted at the venue), so an athlete whose consent was withdrawn is
// suppressed here too — deny path — while a co-athlete in the same
// division still shows their real name — allow path — exactly mirroring
// TestPublicResultsConsentSuppressionSYS103UC023_2's web-page assertions.
// This is deliberately distinct from the office-only on-screen standings
// page (TestResultListPDFSYS072UC018_2's sibling, handleStandings), which
// stays unminimized since it is not a public/publication-intended surface.
func TestResultListPDFMinimizationSYS103UC023_2(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	meetID, units := ukcCaptureFixture(t, client, base)
	ljURL := base + "/meets/" + meetID + "/capture/" + units["Zonen-Weitsprung (UKC)"]

	privacyBody := bodyString(t, mustGet(t, client, base+"/meets/"+meetID+"/privacy"))
	i := strings.Index(privacyBody, "101")
	if i < 0 {
		t.Fatalf("privacy worklist missing bib 101: %s", privacyBody)
	}
	athleteID := athleteIDFromPrivacyPage(t, privacyBody[i:])
	resp := postForm(t, client, base+"/meets/"+meetID+"/privacy",
		base+"/meets/"+meetID+"/privacy/"+athleteID+"/consent", url.Values{"withdrawn": {"true"}})
	_ = resp.Body.Close()

	body := bodyString(t, mustGet(t, client, ljURL))
	athletes := athleteIDsFrom(t, body)
	resp = postForm(t, client, ljURL, ljURL+"/attempt", url.Values{
		"athlete": {athletes["101"]}, "seq": {"1"}, "value": {"4.12"}, "version": {"0"},
	})
	_ = bodyString(t, resp)

	text := extractPDFText(t, client, base+"/meets/"+meetID+"/standings.pdf")
	if strings.Contains(text, "Anna Muster") {
		t.Errorf("deny: printed result list leaked the withdrawn athlete's name\n--- full text ---\n%s", text)
	}
	if !strings.Contains(text, "Bea Beispiel") {
		t.Errorf("allow: printed result list missing the non-withdrawn athlete's name\n--- full text ---\n%s", text)
	}
	if !strings.Contains(text, "4.12") || !strings.Contains(text, "101") {
		t.Errorf("the mark and bib must still render (only identity is suppressed)\n--- full text ---\n%s", text)
	}
}
