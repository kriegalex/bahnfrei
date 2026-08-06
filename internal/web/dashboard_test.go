// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// TestAssignmentsDashboardOfficePanelSYS090DEC025 covers the competition-
// office panel of the "my assignments" dashboard (TASK-042, DEC-025 —
// closes OQ-089): SYS-090 gives that role no per-meet scoping, so its
// dashboard is every meet in the instance, each linking straight into its
// office-level roster/standings surfaces instead of the organizer-only
// /meets workspace it has no access to.
func TestAssignmentsDashboardOfficePanelSYS090DEC025(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base) // admin
	meetID := createUCMeet(t, client, base)
	createAccountWeb(t, client, base, "office0", "competition_office")

	logout(t, client, base)
	login(t, client, base, "office0", "s3cret-passphrase")

	body := bodyString(t, mustGet(t, client, base+"/"))
	if strings.Contains(body, `href="/meets"`) {
		t.Errorf("office dashboard must not offer the organizer-only /meets link: %s", body)
	}
	if strings.Contains(body, `href="/login"`) {
		t.Errorf("logged-in dashboard must not offer the login link: %s", body)
	}
	if !strings.Contains(body, `href="/meets/`+meetID+`/roster"`) {
		t.Errorf("office dashboard missing roster link for its meet: %s", body)
	}
	if !strings.Contains(body, `href="/meets/`+meetID+`/standings"`) {
		t.Errorf("office dashboard missing standings link for its meet: %s", body)
	}
}

// TestAssignmentsDashboardFieldPanelScopingSYS090UC022DEC025 is the
// authz-critical case: a field official's dashboard panel must show
// exactly the meets/units TASK-013 scoped it to and nothing else — neither
// an unassigned unit within a scoped meet, nor any unit of a meet it holds
// no assignment in at all.
func TestAssignmentsDashboardFieldPanelScopingSYS090UC022DEC025(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base) // admin

	meetA, unitsA := ukcCaptureFixture(t, client, base)
	meetB, _ := ukcCaptureFixture(t, client, base)

	resp := postForm(t, client, base+"/admin", base+"/admin/accounts", url.Values{
		"username": {"fo1"}, "display_name": {"Field Official One"},
		"password": {"s3cret-passphrase"}, "role": {"field_official"},
	})
	_ = resp.Body.Close()
	acctID := accountIDFromAdminPage(t, bodyString(t, mustGet(t, client, base+"/admin")), "fo1")

	// Scope fo1 to exactly one unit of meetA; meetB and meetA's other units
	// are left unassigned.
	resp = postForm(t, client, base+"/meets/"+meetA+"/officials", base+"/meets/"+meetA+"/officials/assign", url.Values{
		"account_id": {acctID}, "unit_id": {unitsA["60 metres"]},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("assign field official unit = %d, want 303", resp.StatusCode)
	}

	logout(t, client, base)
	login(t, client, base, "fo1", "s3cret-passphrase")

	body := bodyString(t, mustGet(t, client, base+"/"))
	assignedHref := `href="/meets/` + meetA + `/capture/` + unitsA["60 metres"] + `"`
	if !strings.Contains(body, assignedHref) {
		t.Errorf("field dashboard missing the assigned unit link: %s", body)
	}
	unassignedHref := `/meets/` + meetA + `/capture/` + unitsA["Zone Long Jump (UKC)"]
	if strings.Contains(body, unassignedHref) {
		t.Errorf("field dashboard leaked an unassigned unit of a scoped meet (SYS-090): %s", body)
	}
	if strings.Contains(body, meetB) {
		t.Errorf("field dashboard leaked a meet the official holds no assignment in at all (SYS-090): %s", body)
	}
}

// TestAssignmentsDashboardSubmitterPanelSYS090DEC025 covers the
// entry-submitter panel: an account sees only its own submitted entries at
// the meets it submitted to, never another submitter's entries at the same
// meet (SYS-090 least privilege — an entry-submitter session must not
// enumerate another account's submissions).
func TestAssignmentsDashboardSubmitterPanelSYS090DEC025(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	meetID, _ := entryFlowFixture(t, deps, client, base) // creates sub1, leaves admin logged in
	createAccountWeb(t, client, base, "sub2", "entry_submitter")
	eventID := mustEventID(t, deps, meetID, "100m")

	logout(t, client, base)
	login(t, client, base, "sub1", "s3cret-passphrase")
	resp := postForm(t, client, base+"/meets/"+meetID+"/entries", base+"/meets/"+meetID+"/entries/individual", url.Values{
		"event": {eventID}, "first_name": {"Anna"}, "last_name": {"Muster"},
		"birth_year": {"2011"}, "sex": {"W"}, "club": {"LC Test"}, "seed": {"13.50"},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("sub1 submit entry = %d, want 303", resp.StatusCode)
	}

	logout(t, client, base)
	login(t, client, base, "sub2", "s3cret-passphrase")
	resp = postForm(t, client, base+"/meets/"+meetID+"/entries", base+"/meets/"+meetID+"/entries/individual", url.Values{
		"event": {eventID}, "first_name": {"Berta"}, "last_name": {"Beispiel"},
		"birth_year": {"2011"}, "sex": {"W"}, "club": {"LC Test"}, "seed": {"14.00"},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("sub2 submit entry = %d, want 303", resp.StatusCode)
	}

	body := bodyString(t, mustGet(t, client, base+"/"))
	if !strings.Contains(body, "Berta Beispiel") {
		t.Errorf("sub2 dashboard missing its own entry: %s", body)
	}
	if strings.Contains(body, "Anna Muster") {
		t.Errorf("sub2 dashboard leaked sub1's entry at the same meet (SYS-090): %s", body)
	}
	if !strings.Contains(body, `href="/meets/`+meetID+`/entries"`) {
		t.Errorf("submitter dashboard missing a link back into its meet's entries page: %s", body)
	}

	logout(t, client, base)
	login(t, client, base, "sub1", "s3cret-passphrase")
	body = bodyString(t, mustGet(t, client, base+"/"))
	if !strings.Contains(body, "Anna Muster") {
		t.Errorf("sub1 dashboard missing its own entry: %s", body)
	}
	if strings.Contains(body, "Berta Beispiel") {
		t.Errorf("sub1 dashboard leaked sub2's entry at the same meet (SYS-090): %s", body)
	}
}

// TestAssignmentsDashboardEmptyStatesSYS090DEC025 covers the honest
// "nothing yet" copy each panel renders instead of a blank page when a
// session has no meets/assignments/entries yet.
func TestAssignmentsDashboardEmptyStatesSYS090DEC025(t *testing.T) {
	for _, role := range []string{"competition_office", "field_official", "entry_submitter"} {
		t.Run(role, func(t *testing.T) {
			deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
			client, base := newTestClient(t, deps)
			setupAndLogin(t, client, base) // admin
			createAccountWeb(t, client, base, "empty0", role)
			logout(t, client, base)
			login(t, client, base, "empty0", "s3cret-passphrase")

			body := bodyString(t, mustGet(t, client, base+"/"))
			if strings.Contains(body, `href="/meets/`) {
				t.Errorf("empty %s dashboard must not link to any meet: %s", role, body)
			}
			hasEmptyCopy := strings.Contains(body, "Noch keine Wettkämpfe.") ||
				strings.Contains(body, "Noch keine Zuweisung") ||
				strings.Contains(body, "Noch keine Meldungen abgegeben.")
			if !hasEmptyCopy {
				t.Errorf("empty %s dashboard missing honest empty-state copy: %s", role, body)
			}
		})
	}
}

// TestOfficeHomeDayOfLinksSYS151UC041_1 covers F5/SYS-151/UC-041 #1: the
// office dashboard panel previously linked only roster and standings per
// meet (OQ-111's own framing — check-in and capture/reconciliation were
// unreachable without a typed URL or an organizer handing one over). It now
// carries a direct link into the capture index and reconciliation, plus a
// link into the meet hub for check-in (a meet can have many events, so a
// single "check-in" link cannot target one unambiguously — the hub's
// programme table is where the per-event check-in links already live,
// unchanged by this task).
func TestOfficeHomeDayOfLinksSYS151UC041_1(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base) // admin
	meetID := createUCMeet(t, client, base)
	createAccountWeb(t, client, base, "office3", "competition_office")

	logout(t, client, base)
	login(t, client, base, "office3", "s3cret-passphrase")

	body := bodyString(t, mustGet(t, client, base+"/"))
	for _, want := range []string{
		`href="/meets/` + meetID + `"`,
		`href="/meets/` + meetID + `/capture"`,
		`href="/meets/` + meetID + `/reconciliation"`,
		`href="/meets/` + meetID + `/roster"`,
		`href="/meets/` + meetID + `/standings"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("office home missing %q: %s", want, body)
		}
	}
}

// TestFieldHomeLocalizedDisciplineAndScheduleSYS151UC041_2 covers F5/SYS-151
// /UC-041 #2: a field official's dashboard panel previously named each
// assigned unit with the catalog's English canonical discipline name and
// showed no scheduled time or location — a volunteer's first two questions
// (finding F5). It now reuses the localized-discipline path standings
// already relies on (localizedDisciplineName, SYS-111/SYS-074) and, where
// the unit has been scheduled, appends its time and location; an
// unscheduled unit still renders (by discipline name alone) rather than
// being hidden.
func TestFieldHomeLocalizedDisciplineAndScheduleSYS151UC041_2(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base) // admin, organizer-capable

	meetID, units := ukcCaptureFixture(t, client, base)
	zoneLJUnit := units["Zone Long Jump (UKC)"]
	sixtyMUnit := units["60 metres"]

	resp := postForm(t, client, base+"/admin", base+"/admin/accounts", url.Values{
		"username": {"fo2"}, "display_name": {"Field Official Two"},
		"password": {"s3cret-passphrase"}, "role": {"field_official"},
	})
	_ = resp.Body.Close()
	acctID := accountIDFromAdminPage(t, bodyString(t, mustGet(t, client, base+"/admin")), "fo2")

	for _, unitID := range []string{zoneLJUnit, sixtyMUnit} {
		resp = postForm(t, client, base+"/meets/"+meetID+"/officials", base+"/meets/"+meetID+"/officials/assign", url.Values{
			"account_id": {acctID}, "unit_id": {unitID},
		})
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("assign unit %s = %d, want 303", unitID, resp.StatusCode)
		}
	}

	// Schedule only the zone-long-jump unit; leave 60 metres unscheduled.
	d, err := deps.meets.Meet(context.Background(), meetID)
	if err != nil {
		t.Fatal(err)
	}
	var version int64
	for _, u := range d.Units {
		if u.UnitID == zoneLJUnit {
			version = u.UnitVersion
		}
	}
	resp = postForm(t, client, base+"/meets/"+meetID, base+"/meets/"+meetID+"/units/"+zoneLJUnit+"/schedule", url.Values{
		"version": {strconv.FormatInt(version, 10)}, "scheduled_at": {"2026-08-15T09:30"}, "location": {"Sektor B"},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("schedule unit = %d, want 303", resp.StatusCode)
	}

	logout(t, client, base)
	login(t, client, base, "fo2", "s3cret-passphrase")
	body := bodyString(t, mustGet(t, client, base+"/"))

	if !strings.Contains(body, "Zonen-Weitsprung (UKC)") {
		t.Errorf("field home missing localized discipline name: %s", body)
	}
	if strings.Contains(body, "Zone Long Jump (UKC)") {
		t.Errorf("field home renders the English catalog name instead of the localized one: %s", body)
	}
	if !strings.Contains(body, "Sektor B") {
		t.Errorf("field home missing the scheduled unit's location: %s", body)
	}
	if !strings.Contains(body, "09:30") {
		t.Errorf("field home missing the scheduled unit's time: %s", body)
	}
	if !strings.Contains(body, "60 m") {
		t.Errorf("field home missing the unscheduled unit's localized discipline name: %s", body)
	}
}
