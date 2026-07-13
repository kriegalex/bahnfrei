// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/kriegalex/bahnfrei/internal/domain"
)

// verticalCaptureFixture creates a standalone HJ meet with two athletes and
// returns the meet ID and the HJ unit's capture URL.
func verticalCaptureFixture(t *testing.T, client *http.Client, base string) (meetID, unitURL string) {
	t.Helper()
	resp := postForm(t, client, base+"/meets", base+"/meets", url.Values{
		"name": {"HJ Test Meet"}, "venue": {"Fribourg"},
		"start_date": {"2026-08-15"}, "end_date": {"2026-08-15"},
		"scheme": {"swiss-athletics"},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create meet = %d, want 303", resp.StatusCode)
	}
	meetID = strings.TrimPrefix(resp.Header.Get("Location"), "/meets/")

	meetURL := base + "/meets/" + meetID
	resp = postForm(t, client, meetURL, meetURL+"/events", url.Values{
		"discipline": {"HJ"}, "categories": {"Men", "Women"},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("add HJ event = %d, want 303", resp.StatusCode)
	}

	for _, athlete := range []url.Values{
		{"first_name": {"Anna"}, "last_name": {"Muster"}, "birth_year": {"1998"}, "sex": {"W"}, "bib": {"1"}},
		{"first_name": {"Bea"}, "last_name": {"Beispiel"}, "birth_year": {"1997"}, "sex": {"W"}, "bib": {"2"}},
	} {
		resp = postForm(t, client, meetURL+"/roster", meetURL+"/roster", athlete)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("register participant = %d, want 303", resp.StatusCode)
		}
	}

	body := bodyString(t, mustGet(t, client, meetURL+"/capture"))
	re := regexp.MustCompile(`/meets/` + meetID + `/capture/([0-9A-Za-z]+)"`)
	m := re.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("capture index has no unit link: %s", body)
	}
	if !strings.Contains(body, "field-vertical") && !strings.Contains(body, "Höhe") && !strings.Contains(body, "Vertikale") {
		t.Errorf("capture index does not label the HJ unit as vertical: %s", body)
	}
	return meetID, meetURL + "/capture/" + m[1]
}

// TestVerticalCaptureRequiresHeightsThenGridWebSYS043UC012 drives the
// vertical-jump capture page end to end: before configuration it prompts
// for heights; after configuration the height-progression grid accepts
// O/X/–/r trials and the countback standings render.
func TestVerticalCaptureRequiresHeightsThenGridWebSYS043UC012(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	_, unitURL := verticalCaptureFixture(t, client, base)

	body := bodyString(t, mustGet(t, client, unitURL))
	if !strings.Contains(body, `name="heights"`) {
		t.Errorf("unconfigured vertical unit should render the heights-configuration form: %s", body)
	}

	resp := postForm(t, client, unitURL, unitURL+"/vertical-heights", url.Values{
		"heights": {"1.60", "1.65", "1.70"}, "version": {"0"},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("configure heights = %d, want 303", resp.StatusCode)
	}

	body = bodyString(t, mustGet(t, client, unitURL))
	for _, want := range []string{"1.60", "1.65", "1.70", "Anna Muster", "Bea Beispiel"} {
		if !strings.Contains(body, want) {
			t.Errorf("vertical grid misses %q", want)
		}
	}

	athletes := athleteIDsFrom(t, body)
	saveTrial := func(athlete string, height, seq int, value string) *http.Response {
		t.Helper()
		resp := postForm(t, client, unitURL, unitURL+"/vertical-trial", url.Values{
			"athlete": {athlete}, "height": {intToStr(int64(height))}, "seq": {intToStr(int64(seq))},
			"value": {value}, "version": {"0"},
		})
		_ = bodyString(t, resp)
		return resp
	}
	// Anna clears 1.60 and 1.65 first try; Bea needs a second try at 1.65.
	for _, in := range []struct {
		bib         string
		height, seq int
		value       string
	}{
		{"1", 0, 1, "o"}, {"1", 1, 1, "o"},
		{"2", 0, 1, "o"}, {"2", 1, 1, "x"}, {"2", 1, 2, "o"},
	} {
		if resp := saveTrial(athletes[in.bib], in.height, in.seq, in.value); resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("save trial %+v = %d, want 303", in, resp.StatusCode)
		}
	}

	standings := bodyString(t, mustGet(t, client, unitURL+"/standings"))
	annaPos := strings.Index(standings, "Anna Muster")
	beaPos := strings.Index(standings, "Bea Beispiel")
	if annaPos == -1 || beaPos == -1 || annaPos > beaPos {
		t.Errorf("standings order wrong (fewer attempts at 1.65 ranks first): anna@%d bea@%d", annaPos, beaPos)
	}
	if !strings.Contains(standings, "1.65") {
		t.Error("standings must show the best cleared height")
	}

	// A double-insert of the same cell surfaces a conflict (UC-021 #2),
	// matching the horizontal grid's behaviour.
	resp = postForm(t, client, unitURL, unitURL+"/vertical-trial", url.Values{
		"athlete": {athletes["1"]}, "height": {"0"}, "seq": {"1"}, "value": {"x"}, "version": {"0"},
	})
	conflictBody := bodyString(t, resp)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("conflicting vertical capture = %d, want 422", resp.StatusCode)
	}
	if !strings.Contains(conflictBody, "o") {
		t.Error("conflict message should reflect the already-stored trial")
	}
}

// TestVerticalHeightsConfigureIsOfficeOnlyWebSYS043 checks the office-only
// gate on the height-configuration route at the HTTP layer.
func TestVerticalHeightsConfigureIsOfficeOnlyWebSYS043(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	_, unitURL := verticalCaptureFixture(t, client, base)

	// Anonymous (no session) POST is denied.
	resp, err := http.Post(unitURL+"/vertical-heights", "application/x-www-form-urlencoded",
		strings.NewReader("heights=1.60&version=0"))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("anonymous configure heights = %d, want 403", resp.StatusCode)
	}
}

// TestVerticalDisplayAndParseVerticalValueSYS043 covers the grid notation's
// two pure mapping functions directly: every stored trial kind renders its
// single-letter cell, an unrecognized/empty kind renders blank, and
// parseVerticalValue accepts the D5.2 vocabulary case-insensitively (both
// pass-mark spellings) while rejecting anything else.
func TestVerticalDisplayAndParseVerticalValueSYS043(t *testing.T) {
	for _, tc := range []struct {
		kind domain.QualificationStatus
		want string
	}{
		{domain.StatusO, "o"}, {domain.StatusX, "x"}, {domain.StatusPass, "-"},
		{domain.StatusR, "r"}, {domain.StatusNone, ""}, {domain.StatusQ, ""},
	} {
		if got := verticalDisplay(tc.kind); got != tc.want {
			t.Errorf("verticalDisplay(%q) = %q, want %q", tc.kind, got, tc.want)
		}
	}

	for _, tc := range []struct {
		in       string
		wantKind domain.QualificationStatus
		wantOK   bool
	}{
		{"o", domain.StatusO, true}, {"O", domain.StatusO, true},
		{"x", domain.StatusX, true}, {"X", domain.StatusX, true},
		{"-", domain.StatusPass, true}, {"–", domain.StatusPass, true},
		{"r", domain.StatusR, true}, {" R ", domain.StatusR, true},
		{"", "", false}, {"q", "", false}, {"garbage", "", false},
	} {
		kind, ok := parseVerticalValue(tc.in)
		if kind != tc.wantKind || ok != tc.wantOK {
			t.Errorf("parseVerticalValue(%q) = (%q, %v), want (%q, %v)", tc.in, kind, ok, tc.wantKind, tc.wantOK)
		}
	}
}

// TestNextHeightHintEdgeCasesSYS043 covers nextHeightHint's two "no
// suggestion" branches alongside its two known-discipline increments: an
// unknown discipline code (no WA default increment) and an unparseable
// last-configured height both yield no hint rather than a garbage one.
func TestNextHeightHintEdgeCasesSYS043(t *testing.T) {
	if got := nextHeightHint("1.70", "HJ"); got != "1.73" {
		t.Errorf("nextHeightHint(1.70, HJ) = %q, want 1.73", got)
	}
	if got := nextHeightHint("4.50", "PV"); got != "4.60" {
		t.Errorf("nextHeightHint(4.50, PV) = %q, want 4.60", got)
	}
	if got := nextHeightHint("1.70", "LJ"); got != "" {
		t.Errorf("nextHeightHint for a non-vertical discipline = %q, want \"\"", got)
	}
	if got := nextHeightHint("not-a-height", "HJ"); got != "" {
		t.Errorf("nextHeightHint with an unparseable height = %q, want \"\"", got)
	}
}

// TestVerticalCaptureUnknownUnitIs404Web covers the shared verticalCaptureView
// error mapping (reused by the unit page, its standings fragment and the PDF
// capture sheet): a syntactically fine but nonexistent unit id within a real
// meet surfaces the app's not-found error, not a crash or a blank 200.
func TestVerticalCaptureUnknownUnitIs404Web(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	meetID, _ := verticalCaptureFixture(t, client, base)
	badUnitURL := base + "/meets/" + meetID + "/capture/does-not-exist"

	for _, path := range []string{"", "/standings", "/sheet.pdf"} {
		resp := mustGet(t, client, badUnitURL+path)
		_ = bodyString(t, resp)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", badUnitURL+path, resp.StatusCode)
		}
	}
}

// TestVerticalCaptureTrialInvalidValueWeb covers handleCaptureVerticalTrial's
// parseVerticalValue failure branch: a garbage grid value never reaches
// SaveVerticalTrial and instead re-renders the grid with a 422 and a
// localized "invalid" flash (mirrors the horizontal-attempt grid's
// equivalent behaviour).
func TestVerticalCaptureTrialInvalidValueWeb(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	_, unitURL := verticalCaptureFixture(t, client, base)

	resp := postForm(t, client, unitURL, unitURL+"/vertical-heights", url.Values{
		"heights": {"1.60"}, "version": {"0"},
	})
	_ = resp.Body.Close()

	body := bodyString(t, mustGet(t, client, unitURL))
	athletes := athleteIDsFrom(t, body)
	var athlete string
	for _, id := range athletes {
		athlete = id
		break
	}
	resp = postForm(t, client, unitURL, unitURL+"/vertical-trial", url.Values{
		"athlete": {athlete}, "height": {"0"}, "seq": {"1"}, "value": {"maybe"}, "version": {"0"},
	})
	invalidBody := bodyString(t, resp)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("garbage trial value = %d, want 422", resp.StatusCode)
	}
	if !strings.Contains(invalidBody, `name="heights"`) && !strings.Contains(invalidBody, "1.60") {
		t.Errorf("re-rendered grid missing after invalid value: %s", invalidBody)
	}
}

// TestVerticalCaptureTrialUnitNotAssignedWeb covers handleCaptureVerticalTrial's
// app.ErrUnitNotAssigned branch (SYS-090's per-event scoping, UC-022 #1): a
// field-official account never assigned to this meet's units is refused
// with the shared 403 page, not a capture-grid error banner.
func TestVerticalCaptureTrialUnitNotAssignedWeb(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	_, unitURL := verticalCaptureFixture(t, client, base)
	resp := postForm(t, client, unitURL, unitURL+"/vertical-heights", url.Values{
		"heights": {"1.60"}, "version": {"0"},
	})
	_ = resp.Body.Close()

	createAccountWeb(t, client, base, "unassigned-fo", "field_official")
	logout(t, client, base)
	login(t, client, base, "unassigned-fo", "s3cret-passphrase")

	// The GET itself is already denied (CheckUnitAccess in handleCaptureUnit),
	// so which athlete id is posted below is irrelevant — SaveVerticalTrial's
	// authorizeCaptureAccess must refuse before ever looking one up. postForm
	// still finds a CSRF token on the 403 page: layout() embeds the logout
	// form's hidden field regardless of status code.
	resp = postForm(t, client, unitURL, unitURL+"/vertical-trial", url.Values{
		"athlete": {"irrelevant-athlete-id"}, "height": {"0"}, "seq": {"1"}, "value": {"o"}, "version": {"0"},
	})
	_ = bodyString(t, resp)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("unassigned field official's vertical-trial POST = %d, want 403 (SYS-090)", resp.StatusCode)
	}
}

// TestVerticalCaptureTrialCorrectionRequiredWeb covers
// handleCaptureVerticalTrial's app.ErrCorrectionRequired branch: once a
// unit's results are announced (SYS-046/047), a further trial write is
// refused with the "use the correction flow" flash rather than silently
// mutating an announced result.
func TestVerticalCaptureTrialCorrectionRequiredWeb(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	_, unitURL := verticalCaptureFixture(t, client, base)
	resp := postForm(t, client, unitURL, unitURL+"/vertical-heights", url.Values{
		"heights": {"1.60"}, "version": {"0"},
	})
	_ = resp.Body.Close()

	body := bodyString(t, mustGet(t, client, unitURL))
	athletes := athleteIDsFrom(t, body)
	first := athletes["1"]
	resp = postForm(t, client, unitURL, unitURL+"/vertical-trial", url.Values{
		"athlete": {first}, "height": {"0"}, "seq": {"1"}, "value": {"o"}, "version": {"0"},
	})
	_ = bodyString(t, resp)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("save trial = %d, want 303", resp.StatusCode)
	}

	announceResp := postForm(t, client, unitURL, unitURL+"/announce", url.Values{})
	_ = bodyString(t, announceResp)
	if announceResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("announce = %d, want 303", announceResp.StatusCode)
	}

	second := athletes["2"]
	resp = postForm(t, client, unitURL, unitURL+"/vertical-trial", url.Values{
		"athlete": {second}, "height": {"0"}, "seq": {"1"}, "value": {"o"}, "version": {"0"},
	})
	correctionBody := bodyString(t, resp)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("trial write after announce = %d, want 422 (correction required)", resp.StatusCode)
	}
	if !strings.Contains(correctionBody, "1.60") {
		t.Errorf("correction-required page should still render the grid: %s", correctionBody)
	}
}

// TestVerticalCaptureSheetPDFWeb covers handleVerticalCaptureSheetPDF (0%
// baseline coverage): its own height-column PDF, distinct from
// captureSheetDocument's trial-column shape for horizontal/track units.
func TestVerticalCaptureSheetPDFWeb(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	_, unitURL := verticalCaptureFixture(t, client, base)
	resp := postForm(t, client, unitURL, unitURL+"/vertical-heights", url.Values{
		"heights": {"1.60", "1.65"}, "version": {"0"},
	})
	_ = resp.Body.Close()

	pdfResp := mustGet(t, client, unitURL+"/sheet.pdf")
	defer func() { _ = pdfResp.Body.Close() }()
	if pdfResp.StatusCode != http.StatusOK {
		t.Fatalf("GET vertical sheet.pdf = %d, want 200", pdfResp.StatusCode)
	}
	if ct := pdfResp.Header.Get("Content-Type"); ct != "application/pdf" {
		t.Errorf("Content-Type = %q, want application/pdf", ct)
	}
	data := bodyString(t, pdfResp)
	if !strings.HasPrefix(data, "%PDF") {
		t.Error("downloaded body does not look like a PDF")
	}

	anon := mustGet(t, &http.Client{}, unitURL+"/sheet.pdf")
	_ = anon.Body.Close()
	if anon.StatusCode != http.StatusForbidden {
		t.Errorf("anonymous vertical sheet.pdf GET = %d, want 403 (SYS-090)", anon.StatusCode)
	}
}
