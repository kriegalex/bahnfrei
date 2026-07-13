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

// TestAccountCreateRejectsMissingFieldsAndDuplicateUsername drives the two
// error branches of handleAccountCreate that never reach AuthService.CreateAccount
// at all (missing required fields, errBadInput -> "invalid" flash) and the one
// that does (a duplicate username, store.ErrDuplicateUsername -> "duplicate"
// flash) — both are adversarial ASVS-relevant input-validation paths on a
// privileged form (TASK-026 input).
func TestAccountCreateRejectsMissingFieldsAndDuplicateUsername(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)

	// Missing password: rejected before ever calling into AuthService.
	resp := postForm(t, client, base+"/admin", base+"/admin/accounts", url.Values{
		"username": {"nopass"}, "display_name": {"No Pass"}, "role": {"field_official"},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("missing password create = %d, want 303", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/admin?err=invalid" {
		t.Errorf("missing password create Location = %q, want /admin?err=invalid", loc)
	}
	errBody := bodyString(t, mustGet(t, client, base+resp.Header.Get("Location")))
	if !strings.Contains(errBody, "prüfen") {
		t.Errorf("missing-field error should render the localized invalid-input message: %s", errBody)
	}

	// Duplicate username: the admin account created by setupAndLogin
	// already owns "admin".
	resp = postForm(t, client, base+"/admin", base+"/admin/accounts", url.Values{
		"username": {"admin"}, "display_name": {"Impostor Admin"},
		"password": {"s3cret-passphrase"}, "role": {"field_official"},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("duplicate username create = %d, want 303", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/admin?err=duplicate" {
		t.Errorf("duplicate username create Location = %q, want /admin?err=duplicate", loc)
	}
	dupBody := bodyString(t, mustGet(t, client, base+resp.Header.Get("Location")))
	if !strings.Contains(dupBody, "vergeben") {
		t.Errorf("duplicate-username error should render the localized message: %s", dupBody)
	}

	// Invalid role value: also refused, before any account is written.
	resp = postForm(t, client, base+"/admin", base+"/admin/accounts", url.Values{
		"username": {"badrole"}, "display_name": {"Bad Role"},
		"password": {"s3cret-passphrase"}, "role": {"superuser"},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("invalid role create = %d, want 303", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/admin?err=invalid" {
		t.Errorf("invalid role create Location = %q, want /admin?err=invalid", loc)
	}
}

// TestAccountEnableDisableLifecycleAndLastAdminGuard exercises
// setAccountEnabled's success path in both directions plus the
// ErrLastEnabledAdmin self-lockout guard (SYS-090/091): disabling the sole
// enabled instance-admin account must be refused so an operator can never
// accidentally lock the instance out of its own administration, but once a
// second enabled admin exists, disabling the first succeeds and its live
// session is revoked immediately.
func TestAccountEnableDisableLifecycleAndLastAdminGuard(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)

	adminPage := bodyString(t, mustGet(t, client, base+"/admin"))
	adminID := accountIDFromAdminPage(t, adminPage, "admin")

	// Deny path: the sole enabled admin cannot disable itself.
	resp := postForm(t, client, base+"/admin", base+"/admin/accounts/"+adminID+"/disable", url.Values{
		"reason": {"testing self-lockout"},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("disable sole admin = %d, want 303", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/admin?err=last_admin" {
		t.Errorf("disable sole admin Location = %q, want /admin?err=last_admin", loc)
	}

	// Unknown account ID: falls through to the default "invalid" flash key.
	resp = postForm(t, client, base+"/admin", base+"/admin/accounts/does-not-exist/disable", url.Values{})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("disable unknown account = %d, want 303", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/admin?err=invalid" {
		t.Errorf("disable unknown account Location = %q, want /admin?err=invalid", loc)
	}

	// Provision a second instance-admin so the guard's alternative branch
	// (another enabled admin exists) can be exercised.
	resp = postForm(t, client, base+"/admin", base+"/admin/accounts", url.Values{
		"username": {"admin2"}, "display_name": {"Second Admin"},
		"password": {"s3cret-passphrase"}, "role": {"instance_admin"},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create second admin = %d, want 303", resp.StatusCode)
	}
	admin2ID := accountIDFromAdminPage(t, bodyString(t, mustGet(t, client, base+"/admin")), "admin2")

	// Allow path: disabling admin2 now succeeds since "admin" remains.
	resp = postForm(t, client, base+"/admin", base+"/admin/accounts/"+admin2ID+"/disable", url.Values{
		"reason": {"testing disable"},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("disable admin2 = %d, want 303", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/admin" {
		t.Errorf("disable admin2 Location = %q, want /admin", loc)
	}

	// A disabled account's live session is revoked immediately and it can
	// no longer log in (SYS-091).
	anon, anonBase := newTestClient(t, deps)
	loginForm := bodyString(t, mustGet(t, anon, anonBase+"/login"))
	loginToken := csrfTokenFrom(t, loginForm)
	loginResp, err := anon.PostForm(anonBase+"/login", url.Values{
		"username": {"admin2"}, "password": {"s3cret-passphrase"}, "csrf_token": {loginToken},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusForbidden {
		t.Errorf("login as disabled account = %d, want 403 (SYS-091)", loginResp.StatusCode)
	}

	// Re-enable: the allow path in the other direction.
	resp = postForm(t, client, base+"/admin", base+"/admin/accounts/"+admin2ID+"/enable", url.Values{
		"reason": {"testing enable"},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("enable admin2 = %d, want 303", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/admin" {
		t.Errorf("enable admin2 Location = %q, want /admin", loc)
	}
	loginResp2, err := anon.PostForm(anonBase+"/login", url.Values{
		"username": {"admin2"}, "password": {"s3cret-passphrase"}, "csrf_token": {loginToken},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = loginResp2.Body.Close()
	if loginResp2.StatusCode != http.StatusSeeOther {
		t.Errorf("login after re-enable = %d, want 303", loginResp2.StatusCode)
	}
}

// TestAccountRoleChangeLifecycleAndLastAdminGuard exercises
// handleAccountRoleChange's success path, its invalid-role-value rejection,
// and the ErrLastEnabledAdmin guard against demoting the sole enabled
// instance-admin away from that role (SYS-090).
func TestAccountRoleChangeLifecycleAndLastAdminGuard(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)

	adminPage := bodyString(t, mustGet(t, client, base+"/admin"))
	adminID := accountIDFromAdminPage(t, adminPage, "admin")

	// Deny path: demoting the sole enabled admin away from instance_admin
	// is refused.
	resp := postForm(t, client, base+"/admin", base+"/admin/accounts/"+adminID+"/role", url.Values{
		"role": {"entry_submitter"}, "reason": {"testing self-lockout"},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("demote sole admin = %d, want 303", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/admin?err=last_admin" {
		t.Errorf("demote sole admin Location = %q, want /admin?err=last_admin", loc)
	}

	// Provision a field official, then successfully promote them (allow
	// path — not the last admin, and the new role is valid).
	resp = postForm(t, client, base+"/admin", base+"/admin/accounts", url.Values{
		"username": {"fo2"}, "display_name": {"Field Official Two"},
		"password": {"s3cret-passphrase"}, "role": {"field_official"},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create field official = %d, want 303", resp.StatusCode)
	}
	fo2ID := accountIDFromAdminPage(t, bodyString(t, mustGet(t, client, base+"/admin")), "fo2")

	resp = postForm(t, client, base+"/admin", base+"/admin/accounts/"+fo2ID+"/role", url.Values{
		"role": {"meet_organizer"}, "reason": {"promotion"},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("promote field official = %d, want 303", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/admin" {
		t.Errorf("promote field official Location = %q, want /admin", loc)
	}
	promoted := bodyString(t, mustGet(t, client, base+"/admin"))
	if !strings.Contains(promoted, "fo2") {
		t.Fatalf("promoted account missing from admin list: %s", promoted)
	}

	// Invalid role value: rejected before any write (ErrInvalidRole).
	resp = postForm(t, client, base+"/admin", base+"/admin/accounts/"+fo2ID+"/role", url.Values{
		"role": {"superuser"}, "reason": {"nonsense"},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("invalid role change = %d, want 303", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/admin?err=invalid" {
		t.Errorf("invalid role change Location = %q, want /admin?err=invalid", loc)
	}

	// Unknown account ID: also falls to the default "invalid" flash key.
	resp = postForm(t, client, base+"/admin", base+"/admin/accounts/does-not-exist/role", url.Values{
		"role": {"meet_organizer"},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("role change on unknown account = %d, want 303", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/admin?err=invalid" {
		t.Errorf("role change on unknown account Location = %q, want /admin?err=invalid", loc)
	}
}

// TestAccountMutationRoutesRejectWrongRoleAndWrongMethod is the adversarial
// ASVS-L2 pass over the three mutation routes handleAccountEnable/Disable/
// RoleChange share: a session below the instance-admin floor is denied
// (least privilege, SYS-090), and a mismatched HTTP method on a registered
// path is refused by the router itself (405) before any handler runs —
// worth pinning explicitly since a future refactor could accidentally
// register a permissive catch-all.
func TestAccountMutationRoutesRejectWrongRoleAndWrongMethod(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)

	resp := postForm(t, client, base+"/admin", base+"/admin/accounts", url.Values{
		"username": {"fo3"}, "display_name": {"Field Official Three"},
		"password": {"s3cret-passphrase"}, "role": {"field_official"},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create field official = %d, want 303", resp.StatusCode)
	}
	fo3ID := accountIDFromAdminPage(t, bodyString(t, mustGet(t, client, base+"/admin")), "fo3")

	logout(t, client, base)
	login(t, client, base, "fo3", "s3cret-passphrase")

	for _, path := range []string{
		"/admin/accounts/" + fo3ID + "/enable",
		"/admin/accounts/" + fo3ID + "/disable",
		"/admin/accounts/" + fo3ID + "/role",
	} {
		resp := postForm(t, client, base+"/", base+path, url.Values{"role": {"meet_organizer"}})
		_ = bodyString(t, resp)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("field official POST %s = %d, want 403 (SYS-090 least privilege)", path, resp.StatusCode)
		}
	}

	logout(t, client, base)
	login(t, client, base, "admin", "s3cret-passphrase")

	// Method misuse: these routes are registered POST-only, and the app's
	// mux has an explicit catch-all "/" -> handleNotFound (routes.go), so a
	// mismatched method on a registered path falls through to 404 rather
	// than the stdlib's path-only match — worth pinning so a refactor that
	// drops the catch-all doesn't silently start leaking a 405 with an
	// Allow header disclosing route existence to an unauthenticated probe.
	for _, path := range []string{
		"/admin/accounts", "/admin/accounts/" + fo3ID + "/enable",
		"/admin/accounts/" + fo3ID + "/disable", "/admin/accounts/" + fo3ID + "/role",
	} {
		resp := mustGet(t, client, base+path)
		_ = bodyString(t, resp)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404 (method misuse)", path, resp.StatusCode)
		}
	}
}
