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

// TestUC016_RecordFlagRendersAndChecklistLinksThroughWeb drives UC-016 #5
// (PB flagging) and #4 (the record-documentation checklist) end to end
// through the HTTP surface: a plain meet with two "100m" units for the same
// athlete (two separate races), a better second mark earns a PB flag that
// renders on the capture standings page, and the flag links to a checklist
// page showing the in-system Rekordprotokoll data with explicit gaps for
// what this system never captures (SYS-049/SYS-051).
func TestUC016_RecordFlagRendersAndChecklistLinksThroughWeb(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)

	resp := postForm(t, client, base+"/meets", base+"/meets", url.Values{
		"name": {"100m Meeting"}, "venue": {"Fribourg"},
		"start_date": {"2027-06-10"}, "end_date": {"2027-06-10"},
		"scheme": {"swiss-athletics"},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create meet = %d, want 303", resp.StatusCode)
	}
	meetID := strings.TrimPrefix(resp.Header.Get("Location"), "/meets/")
	meetURL := base + "/meets/" + meetID

	for i := 0; i < 2; i++ {
		resp = postForm(t, client, meetURL, meetURL+"/events", url.Values{
			"discipline": {"100m"}, "categories": {"U18 W"},
		})
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("add 100m event %d = %d, want 303", i, resp.StatusCode)
		}
	}

	resp = postForm(t, client, meetURL+"/roster", meetURL+"/roster", url.Values{
		"first_name": {"Anna"}, "last_name": {"Muster"}, "birth_year": {"2010"}, "sex": {"W"}, "bib": {"1"},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("register participant = %d, want 303", resp.StatusCode)
	}

	body := bodyString(t, mustGet(t, client, meetURL+"/capture"))
	// The capture index localizes discipline names (SYS-111/F6, TASK-050):
	// the DE catalog renders code "100m" as "100 m", not the catalog's
	// canonical English "100 metres".
	re := regexp.MustCompile(`/meets/` + meetID + `/capture/([0-9A-Za-z]+)">100 m<`)
	matches := re.FindAllStringSubmatch(body, -1)
	if len(matches) != 2 {
		t.Fatalf("capture index 100m links = %d, want 2 units: %s", len(matches), body)
	}
	oldUnitURL := meetURL + "/capture/" + matches[0][1]
	newUnitURL := meetURL + "/capture/" + matches[1][1]

	athletes := athleteIDsFrom(t, bodyString(t, mustGet(t, client, oldUnitURL)))
	athleteID := athletes["1"]
	if athleteID == "" {
		t.Fatalf("athlete id for bib 1 not found in capture page")
	}

	resp = postForm(t, client, oldUnitURL, oldUnitURL+"/track", url.Values{
		"athlete": {athleteID}, "time": {"12.02"}, "timing": {"electronic"},
	})
	_ = bodyString(t, resp)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("save old mark = %d, want 303", resp.StatusCode)
	}
	resp = postForm(t, client, newUnitURL, newUnitURL+"/track", url.Values{
		"athlete": {athleteID}, "time": {"11.95"}, "timing": {"electronic"},
	})
	_ = bodyString(t, resp)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("save new (better) mark = %d, want 303", resp.StatusCode)
	}

	standings := bodyString(t, mustGet(t, client, newUnitURL+"/standings"))
	linkRe := regexp.MustCompile(`href="(/meets/` + meetID + `/capture/[0-9A-Za-z]+/record-checklist/` + athleteID + `)">([A-Z, ]+)</a>`)
	m := linkRe.FindStringSubmatch(standings)
	if m == nil {
		t.Fatalf("standings page has no record-flag link: %s", standings)
	}
	if !strings.Contains(m[2], "PB") {
		t.Errorf("flag link text = %q, want it to contain PB", m[2])
	}

	checklist := bodyString(t, mustGet(t, client, base+m[1]))
	if !strings.Contains(checklist, "PB") {
		t.Error("checklist page does not show the PB flag")
	}
	if !strings.Contains(checklist, "FAT") {
		t.Error("checklist page does not show the FAT timing class (SYS-051)")
	}
	// Fields this system never captures must render as explicit gaps
	// (UC-016 #4), localized: DE's "fehlt — manuell zu ergänzen".
	if strings.Count(checklist, "fehlt") < 2 {
		t.Errorf("checklist must mark at least the zero-test and photo-finish fields as gaps: %s", checklist)
	}
}
