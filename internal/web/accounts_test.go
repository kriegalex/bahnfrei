// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// login drives the login form for an already-provisioned account.
func login(t *testing.T, client *http.Client, base, username, password string) {
	t.Helper()
	token := csrfTokenFrom(t, bodyString(t, mustGet(t, client, base+"/login")))
	resp, err := client.PostForm(base+"/login", url.Values{
		"username": {username}, "password": {password}, "csrf_token": {token},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST /login (%s) = %d, want 303", username, resp.StatusCode)
	}
}

// logout drives the logout form present on every authenticated page.
func logout(t *testing.T, client *http.Client, base string) {
	t.Helper()
	token := csrfTokenFrom(t, bodyString(t, mustGet(t, client, base+"/")))
	resp, err := client.PostForm(base+"/logout", url.Values{"csrf_token": {token}})
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
}

// accountIDFromAdminPage extracts the account ID of username from the
// rendered /admin account list: each row's role-change form posts to
// "/admin/accounts/{id}/role".
func accountIDFromAdminPage(t *testing.T, body, username string) string {
	t.Helper()
	i := strings.Index(body, ">"+username+"<")
	if i < 0 {
		t.Fatalf("account row for %q not found on /admin page", username)
	}
	marker := `/admin/accounts/`
	j := strings.Index(body[i:], marker)
	if j < 0 {
		t.Fatalf("no account-scoped form found after %q's row", username)
	}
	rest := body[i+j+len(marker):]
	return rest[:strings.Index(rest, "/")]
}

// TestAccountAdminAndAuditRouteGatingSYS090UC022 proves the coarse HTTP
// gates on the new TASK-013 surfaces: anonymous requests are denied on the
// account admin, assignment and audit routes, and a field-official session
// is denied the office-level audit view (CapViewAudit floor, SYS-090).
func TestAccountAdminAndAuditRouteGatingSYS090UC022(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)

	for _, path := range []string{"/admin", "/audit", "/meets/some-id/officials"} {
		resp := mustGet(t, client, base+path)
		_ = bodyString(t, resp)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("anonymous GET %s = %d, want 403 (SYS-090)", path, resp.StatusCode)
		}
	}

	// A field official is above public but still below the audit view's
	// office floor and the admin view's instance-admin floor.
	setupAndLogin(t, client, base)
	resp := postForm(t, client, base+"/admin", base+"/admin/accounts", url.Values{
		"username": {"fo0"}, "display_name": {"Field Official Zero"},
		"password": {"s3cret-passphrase"}, "role": {"field_official"},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create field official = %d, want 303", resp.StatusCode)
	}
	logout(t, client, base)
	login(t, client, base, "fo0", "s3cret-passphrase")
	for _, path := range []string{"/admin", "/audit"} {
		resp := mustGet(t, client, base+path)
		_ = bodyString(t, resp)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("field official GET %s = %d, want 403 (SYS-090 least privilege)", path, resp.StatusCode)
		}
	}
}

// TestFieldOfficialEventScopeSYS090UC022_2 drives the whole TASK-013 slice
// end to end over real HTTP: an instance admin provisions a field-official
// account through the new /admin UI, scopes it to one event unit through
// the new /meets/{id}/officials UI (SYS-090 "assignable per meet"), and the
// field official's session is then let into the assigned unit but denied
// the unassigned one (UC-022 #1) — with the denial itself landing in the
// privileged audit surface the office/admin can review (UC-022 #2).
func TestFieldOfficialEventScopeSYS090UC022_2(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base) // admin

	meetID, units := ukcCaptureFixture(t, client, base)
	assignedURL := base + "/meets/" + meetID + "/capture/" + units["60 metres"]
	unassignedURL := base + "/meets/" + meetID + "/capture/" + units["Zone Long Jump (UKC)"]

	// Provision the field-official account through the account admin UI.
	resp := postForm(t, client, base+"/admin", base+"/admin/accounts", url.Values{
		"username": {"fo1"}, "display_name": {"Field Official One"},
		"password": {"s3cret-passphrase"}, "role": {"field_official"},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create field official = %d, want 303", resp.StatusCode)
	}
	acctID := accountIDFromAdminPage(t, bodyString(t, mustGet(t, client, base+"/admin")), "fo1")

	// Scope it to the 60 m unit only, through the assignment matrix UI.
	resp = postForm(t, client, base+"/meets/"+meetID+"/officials", base+"/meets/"+meetID+"/officials/assign", url.Values{
		"account_id": {acctID}, "unit_id": {units["60 metres"]},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("assign field official unit = %d, want 303", resp.StatusCode)
	}
	assignPage := bodyString(t, mustGet(t, client, base+"/meets/"+meetID+"/officials"))
	if !strings.Contains(assignPage, "Field Official One (fo1)") {
		t.Error("officials matrix must list the newly created field official")
	}

	logout(t, client, base)
	login(t, client, base, "fo1", "s3cret-passphrase")

	// Allow path (UC-022 #1 contrast case): the assigned unit opens fine
	// and accepts a capture.
	assignedResp := mustGet(t, client, assignedURL)
	body := bodyString(t, assignedResp)
	if assignedResp.StatusCode != http.StatusOK {
		t.Errorf("GET assigned unit = %d, want 200", assignedResp.StatusCode)
	}
	athletes := athleteIDsFrom(t, body)
	var anAthlete string
	for _, id := range athletes {
		anAthlete = id
		break
	}
	attemptResp := postForm(t, client, assignedURL, assignedURL+"/track", url.Values{
		"athlete": {anAthlete}, "time": {"9.99"}, "timing": {"manual"},
	})
	_ = bodyString(t, attemptResp)
	if attemptResp.StatusCode != http.StatusSeeOther {
		t.Errorf("capture on the assigned unit = %d, want 303 (allow path)", attemptResp.StatusCode)
	}

	// Deny path (UC-022 #1): the same account, on a unit it was never
	// assigned, is refused both the read view and the write attempt —
	// enforced server-side, not merely a hidden link (SYS-090).
	if resp := mustGet(t, client, unassignedURL); resp.StatusCode != http.StatusForbidden {
		t.Errorf("GET unassigned unit = %d, want 403 (SYS-090 per-event scoping)", resp.StatusCode)
	} else {
		_ = bodyString(t, resp)
	}
	deniedResp := postForm(t, client, assignedURL, unassignedURL+"/attempt", url.Values{
		"athlete": {anAthlete}, "seq": {"1"}, "value": {"3.00"}, "version": {"0"},
	})
	_ = bodyString(t, deniedResp)
	if deniedResp.StatusCode != http.StatusForbidden {
		t.Errorf("write attempt on unassigned unit = %d, want 403", deniedResp.StatusCode)
	}

	// UC-022 #2: the denied attempt itself is an audit event (actor,
	// action, target, timestamp) an office/admin operator can review — and
	// the routine capture on the assigned unit is NOT drowned into the
	// same privileged view (TASK-013 filters to privileged actions only).
	logout(t, client, base)
	login(t, client, base, "admin", "s3cret-passphrase")
	auditBody := bodyString(t, mustGet(t, client, base+"/audit"))
	if !strings.Contains(auditBody, "capture.access_denied") {
		t.Error("audit log must surface the denied scoped-capture attempt (UC-022 #1/#2)")
	}
	if !strings.Contains(auditBody, "Field Official One") {
		t.Error("audit log must attribute the denied attempt to the acting field official")
	}
	if strings.Contains(auditBody, "result.capture") {
		t.Error("routine track-capture events must not appear in the privileged audit view")
	}
}
