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
