// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// TestAccountResetPasswordFullFlowSYS090091 is the TASK-053/DEC-030
// meet-morning-lockout repro end to end over real HTTP: an instance admin
// resets a locked-out operator's password through the accounts UI, the
// operator's old session dies immediately, the temporary password logs
// them back in but the forced-change gate redirects every other
// authenticated route to /change-password until they complete it, and both
// the reset and the completed change land in the privileged audit view.
func TestAccountResetPasswordFullFlowSYS090091(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	adminClient, base := newTestClient(t, deps)
	setupAndLogin(t, adminClient, base)

	resp := postForm(t, adminClient, base+"/admin", base+"/admin/accounts", url.Values{
		"username": {"office1"}, "display_name": {"Office One"},
		"password": {"p4ssword-here"}, "role": {"competition_office"},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create office1 = %d, want 303", resp.StatusCode)
	}
	acctID := accountIDFromAdminPage(t, bodyString(t, mustGet(t, adminClient, base+"/admin")), "office1")

	// office1 logs in on its own client and confirms a normal authenticated
	// route (competition-office floor) works before the reset.
	officeClient, _ := newTestClient(t, deps)
	login(t, officeClient, base, "office1", "p4ssword-here")
	if resp := mustGet(t, officeClient, base+"/audit"); resp.StatusCode != http.StatusOK {
		_ = bodyString(t, resp)
		t.Fatalf("GET /audit before reset = %d, want 200", resp.StatusCode)
	} else {
		_ = bodyString(t, resp)
	}

	// Admin resets the password through the confirm-page flow.
	confirmBody := bodyString(t, mustGet(t, adminClient, base+"/admin/accounts/"+acctID+"/reset/confirm"))
	if !strings.Contains(confirmBody, "office1") {
		t.Fatalf("reset confirm page must name the target account: %s", confirmBody)
	}
	resetResp := postForm(t, adminClient, base+"/admin/accounts/"+acctID+"/reset/confirm",
		base+"/admin/accounts/"+acctID+"/reset", url.Values{"new_password": {"temp-passphrase-1"}})
	_ = resetResp.Body.Close()
	if resetResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST reset = %d, want 303", resetResp.StatusCode)
	}
	if loc := resetResp.Header.Get("Location"); loc != "/admin" {
		t.Errorf("POST reset Location = %q, want /admin", loc)
	}

	// office1's OLD session is dead immediately, not merely at next login:
	// the same cookie now behaves like an anonymous request.
	if resp := mustGet(t, officeClient, base+"/audit"); resp.StatusCode != http.StatusForbidden {
		_ = bodyString(t, resp)
		t.Errorf("GET /audit on the pre-reset session = %d, want 403 (session revoked)", resp.StatusCode)
	} else {
		_ = bodyString(t, resp)
	}

	// The old password no longer works; the temporary one logs office1 back
	// in — but the forced-change gate now redirects every other
	// authenticated route to /change-password.
	loginToken := csrfTokenFrom(t, bodyString(t, mustGet(t, officeClient, base+"/login")))
	badLogin, err := officeClient.PostForm(base+"/login", url.Values{
		"username": {"office1"}, "password": {"p4ssword-here"}, "csrf_token": {loginToken},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = badLogin.Body.Close()
	if badLogin.StatusCode != http.StatusUnauthorized {
		t.Errorf("login with the old password after reset = %d, want 401", badLogin.StatusCode)
	}
	login(t, officeClient, base, "office1", "temp-passphrase-1")

	for _, path := range []string{"/", "/audit", "/meets"} {
		resp := mustGet(t, officeClient, base+path)
		_ = bodyString(t, resp)
		if resp.StatusCode != http.StatusSeeOther {
			t.Errorf("GET %s while forced to change password = %d, want 303", path, resp.StatusCode)
			continue
		}
		if loc := resp.Header.Get("Location"); loc != "/change-password" {
			t.Errorf("GET %s while forced Location = %q, want /change-password", path, loc)
		}
	}

	// /change-password, /logout and static assets remain reachable.
	changeFormBody := bodyString(t, mustGet(t, officeClient, base+"/change-password"))
	if !strings.Contains(changeFormBody, `name="current_password"`) || !strings.Contains(changeFormBody, `name="new_password"`) {
		t.Fatalf("change-password form missing expected fields: %s", changeFormBody)
	}

	// Wrong current password: rejected with a field-level error, session
	// still forced.
	wrongCurrent := postForm(t, officeClient, base+"/change-password", base+"/change-password", url.Values{
		"current_password": {"not-the-temp-password"}, "new_password": {"durable-passphrase-2"}, "confirm_password": {"durable-passphrase-2"},
	})
	wrongCurrentBody := bodyString(t, wrongCurrent)
	if wrongCurrent.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("change-password with wrong current password = %d, want 422", wrongCurrent.StatusCode)
	}
	if !strings.Contains(wrongCurrentBody, `id="current_password-error"`) {
		t.Errorf("change-password wrong-current-password response missing the field error: %s", wrongCurrentBody)
	}

	// Mismatched confirmation.
	mismatch := postForm(t, officeClient, base+"/change-password", base+"/change-password", url.Values{
		"current_password": {"temp-passphrase-1"}, "new_password": {"durable-passphrase-2"}, "confirm_password": {"something-else"},
	})
	mismatchBody := bodyString(t, mismatch)
	if mismatch.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("change-password with mismatched confirmation = %d, want 422", mismatch.StatusCode)
	}
	if !strings.Contains(mismatchBody, `id="confirm_password-error"`) {
		t.Errorf("change-password mismatch response missing the field error: %s", mismatchBody)
	}

	// Too-short new password.
	tooShort := postForm(t, officeClient, base+"/change-password", base+"/change-password", url.Values{
		"current_password": {"temp-passphrase-1"}, "new_password": {"short"}, "confirm_password": {"short"},
	})
	tooShortBody := bodyString(t, tooShort)
	if tooShort.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("change-password with a too-short new password = %d, want 422", tooShort.StatusCode)
	}
	if !strings.Contains(tooShortBody, `id="new_password-error"`) {
		t.Errorf("change-password too-short response missing the field error: %s", tooShortBody)
	}

	// The correct change completes the forced flow.
	completeResp := postForm(t, officeClient, base+"/change-password", base+"/change-password", url.Values{
		"current_password": {"temp-passphrase-1"}, "new_password": {"durable-passphrase-2"}, "confirm_password": {"durable-passphrase-2"},
	})
	_ = completeResp.Body.Close()
	if completeResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("completing change-password = %d, want 303", completeResp.StatusCode)
	}
	if loc := completeResp.Header.Get("Location"); loc != "/" {
		t.Errorf("completing change-password Location = %q, want /", loc)
	}

	// Normal access resumes on the SAME session — no fresh login required.
	if resp := mustGet(t, officeClient, base+"/audit"); resp.StatusCode != http.StatusOK {
		_ = bodyString(t, resp)
		t.Errorf("GET /audit after completing the forced change = %d, want 200", resp.StatusCode)
	} else {
		_ = bodyString(t, resp)
	}

	// Both the reset and the completed change are audited (SYS-046),
	// visible on the office/admin privileged-audit surface.
	auditBody := bodyString(t, mustGet(t, adminClient, base+"/audit"))
	if !strings.Contains(auditBody, "account.password_reset") {
		t.Error("audit log must surface the admin-issued password reset")
	}
	if !strings.Contains(auditBody, "account.password_change") {
		t.Error("audit log must surface the completed forced change-password step")
	}
}

// TestAccountResetPasswordRequiresInstanceAdminRole proves the reset
// surface is gated exactly like every other account mutation (SYS-090
// least privilege): a non-admin session is refused both the confirm page
// and the POST, and anonymous requests are refused too.
func TestAccountResetPasswordRequiresInstanceAdminRole(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)

	resp := postForm(t, client, base+"/admin", base+"/admin/accounts", url.Values{
		"username": {"fo9"}, "display_name": {"Field Official Nine"},
		"password": {"s3cret-passphrase"}, "role": {"field_official"},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create field official = %d, want 303", resp.StatusCode)
	}
	fo9ID := accountIDFromAdminPage(t, bodyString(t, mustGet(t, client, base+"/admin")), "fo9")

	// Anonymous.
	anon, anonBase := newTestClient(t, deps)
	if resp := mustGet(t, anon, anonBase+"/admin/accounts/"+fo9ID+"/reset/confirm"); resp.StatusCode != http.StatusForbidden {
		_ = bodyString(t, resp)
		t.Errorf("anonymous GET reset confirm = %d, want 403", resp.StatusCode)
	} else {
		_ = bodyString(t, resp)
	}

	// A field-official session (below the instance-admin floor).
	logout(t, client, base)
	login(t, client, base, "fo9", "s3cret-passphrase")
	if resp := mustGet(t, client, base+"/admin/accounts/"+fo9ID+"/reset/confirm"); resp.StatusCode != http.StatusForbidden {
		_ = bodyString(t, resp)
		t.Errorf("field-official GET reset confirm = %d, want 403", resp.StatusCode)
	} else {
		_ = bodyString(t, resp)
	}
	resp2 := postForm(t, client, base+"/", base+"/admin/accounts/"+fo9ID+"/reset", url.Values{"new_password": {"whatever-new-1"}})
	_ = bodyString(t, resp2)
	if resp2.StatusCode != http.StatusForbidden {
		t.Errorf("field-official POST reset = %d, want 403", resp2.StatusCode)
	}
}

// TestAccountResetPasswordRejectsShortPasswordWeb covers the confirm page's
// server-side re-render on a too-short temporary password (defense in
// depth behind the form's client-side minlength).
func TestAccountResetPasswordRejectsShortPasswordWeb(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)

	resp := postForm(t, client, base+"/admin", base+"/admin/accounts", url.Values{
		"username": {"office2"}, "display_name": {"Office Two"},
		"password": {"p4ssword-here"}, "role": {"competition_office"},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create office2 = %d, want 303", resp.StatusCode)
	}
	acctID := accountIDFromAdminPage(t, bodyString(t, mustGet(t, client, base+"/admin")), "office2")

	shortResp := postForm(t, client, base+"/admin/accounts/"+acctID+"/reset/confirm",
		base+"/admin/accounts/"+acctID+"/reset", url.Values{"new_password": {"short"}})
	shortBody := bodyString(t, shortResp)
	if shortResp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("reset with a too-short password = %d, want 422", shortResp.StatusCode)
	}
	if !strings.Contains(shortBody, `id="new_password-error"`) {
		t.Errorf("reset too-short response missing the field error: %s", shortBody)
	}
}

// TestChangePasswordSubmitSharesLoginRateLimitSYS092Web verifies (rather
// than assumes, per TASK-053's brief) that repeated wrong-current-password
// submissions on /change-password spend the SAME per-IP budget as /login
// (TASK-026, ratelimit.go): this endpoint re-verifies a live credential
// exactly like Login does, so it must not be a brute-force side door.
func TestChangePasswordSubmitSharesLoginRateLimitSYS092Web(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)

	resp := postForm(t, client, base+"/admin", base+"/admin/accounts", url.Values{
		"username": {"office3"}, "display_name": {"Office Three"},
		"password": {"p4ssword-here"}, "role": {"competition_office"},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create office3 = %d, want 303", resp.StatusCode)
	}
	acctID := accountIDFromAdminPage(t, bodyString(t, mustGet(t, client, base+"/admin")), "office3")
	resetResp := postForm(t, client, base+"/admin/accounts/"+acctID+"/reset/confirm",
		base+"/admin/accounts/"+acctID+"/reset", url.Values{"new_password": {"temp-passphrase-3"}})
	_ = resetResp.Body.Close()
	if resetResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("reset office3 = %d, want 303", resetResp.StatusCode)
	}

	officeClient, _ := newTestClient(t, deps)
	login(t, officeClient, base, "office3", "temp-passphrase-3")

	wrongChange := func() *http.Response {
		return postForm(t, officeClient, base+"/change-password", base+"/change-password", url.Values{
			"current_password": {"not-the-temp-password"}, "new_password": {"durable-passphrase-4"}, "confirm_password": {"durable-passphrase-4"},
		})
	}
	for i := 0; i < loginFailLimit; i++ {
		resp := wrongChange()
		body := bodyString(t, resp)
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("wrong current_password attempt %d = %d, want 422: %s", i+1, resp.StatusCode, body)
		}
	}
	blocked := wrongChange()
	blockedBody := bodyString(t, blocked)
	if blocked.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("over-budget change-password attempt = %d, want 429: %s", blocked.StatusCode, blockedBody)
	}
	if blocked.Header.Get("Retry-After") == "" {
		t.Error("429 response must carry a Retry-After header")
	}

	// The SAME IP is now blocked on /login too — proof the two paths share
	// one counter rather than each getting their own separate budget.
	// Logout first (exempt from the forced-change gate, unlike "/" which
	// the logout helper would otherwise fetch its CSRF token from) so the
	// request reaches the login handler rather than being redirected back
	// to /change-password by the still-live forced session.
	logoutToken := csrfTokenFrom(t, bodyString(t, mustGet(t, officeClient, base+"/change-password")))
	logoutResp, err := officeClient.PostForm(base+"/logout", url.Values{"csrf_token": {logoutToken}})
	if err != nil {
		t.Fatal(err)
	}
	_ = logoutResp.Body.Close()

	loginToken := csrfTokenFrom(t, bodyString(t, mustGet(t, officeClient, base+"/login")))
	loginResp, err := officeClient.PostForm(base+"/login", url.Values{
		"username": {"office3"}, "password": {"temp-passphrase-3"}, "csrf_token": {loginToken},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("login from the same throttled IP = %d, want 429 (shared budget)", loginResp.StatusCode)
	}
}
