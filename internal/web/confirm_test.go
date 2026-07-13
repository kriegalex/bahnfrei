// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// TestMeetArchiveConfirmFlowOQ074 covers the OQ-074 confirmation flow for
// meet archive: the GET confirm sub-page names the meet, a Cancel
// navigation away never archives it, and only the confirm page's own POST
// does.
func TestMeetArchiveConfirmFlowOQ074(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	meetID := createUCMeet(t, client, base)

	confirmURL := base + "/meets/" + meetID + "/archive/confirm"
	confirmBody := bodyString(t, mustGet(t, client, confirmURL))
	if !strings.Contains(confirmBody, "Abendmeeting Uster") {
		t.Errorf("archive confirm page must name the meet: %s", confirmBody)
	}
	if !strings.Contains(confirmBody, `action="/meets/`+meetID+`/archive"`) {
		t.Errorf("archive confirm page must post to the archive route: %s", confirmBody)
	}
	// Cancel is a plain navigation back to the meet page — never a POST —
	// so simply not following it is enough to prove it archives nothing.
	if !strings.Contains(confirmBody, `href="/meets/`+meetID+`"`) {
		t.Errorf("archive confirm page must offer a Cancel link back to the meet: %s", confirmBody)
	}

	detail := bodyString(t, mustGet(t, client, base+"/meets/"+meetID))
	if strings.Contains(detail, "Archiviert") {
		t.Fatal("visiting the confirm page must not archive the meet by itself")
	}

	// Confirming (the confirm page's own form) does archive it.
	resp := postForm(t, client, confirmURL, base+"/meets/"+meetID+"/archive", url.Values{"version": {"1"}})
	_ = bodyString(t, resp)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("confirmed archive = %d, want 303", resp.StatusCode)
	}
	detail = bodyString(t, mustGet(t, client, base+"/meets/"+meetID))
	if !strings.Contains(detail, "Archiviert") {
		t.Error("confirmed archive did not persist")
	}
}

// TestAccountDisableConfirmFlowOQ074 covers the OQ-074 confirmation flow
// for account disable: the GET confirm page names the account, and only
// its own POST disables it.
func TestAccountDisableConfirmFlowOQ074(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	createAccountWeb(t, client, base, "office1", "competition_office")

	adminBody := bodyString(t, mustGet(t, client, base+"/admin"))
	officeID := accountIDFromAdminPage(t, adminBody, "office1")

	confirmURL := base + "/admin/accounts/" + officeID + "/disable/confirm"
	confirmBody := bodyString(t, mustGet(t, client, confirmURL))
	if !strings.Contains(confirmBody, "office1") {
		t.Errorf("disable confirm page must name the account: %s", confirmBody)
	}

	// Visiting the confirm page alone must not disable the account.
	stillEnabled := bodyString(t, mustGet(t, client, base+"/admin"))
	if strings.Contains(stillEnabled, "office1") && !strings.Contains(stillEnabled[strings.Index(stillEnabled, "office1"):], "Aktiv") {
		t.Error("account status changed before the confirm form was submitted")
	}

	resp := postForm(t, client, confirmURL, base+"/admin/accounts/"+officeID+"/disable", url.Values{})
	_ = bodyString(t, resp)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("confirmed disable = %d, want 303", resp.StatusCode)
	}
	after := bodyString(t, mustGet(t, client, base+"/admin"))
	if !strings.Contains(after, "Deaktiviert") {
		t.Error("confirmed disable did not persist")
	}
}

// TestPrivacyEraseConfirmFlowOQ074 covers the OQ-074 confirmation flow for
// athlete erasure, including its typed-confirmation friction (ASVS-review
// defense-in-depth on this irreversible-and-unrecoverable action): the
// confirm page shows the athlete's bib as the token to type, a mismatched
// token is rejected with an inline field error and erases nothing, and the
// correct token erases.
func TestPrivacyEraseConfirmFlowOQ074(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	meetID := createUCMeet(t, client, base)
	registerRosterParticipant(t, client, base, meetID, nil)

	privacyBody := bodyString(t, mustGet(t, client, base+"/meets/"+meetID+"/privacy"))
	athleteID := athleteIDFromPrivacyPage(t, privacyBody)

	confirmURL := base + "/meets/" + meetID + "/privacy/" + athleteID + "/erase/confirm"
	confirmBody := bodyString(t, mustGet(t, client, confirmURL))
	if !strings.Contains(confirmBody, "Anna Muster") {
		t.Errorf("erase confirm page must name the athlete: %s", confirmBody)
	}
	if !strings.Contains(confirmBody, `name="confirm_text"`) {
		t.Errorf("erase confirm page must offer the typed-confirmation field: %s", confirmBody)
	}
	if !strings.Contains(confirmBody, "&#34;1&#34;") {
		t.Errorf("erase confirm page must show the bib (\"1\") as the token to type: %s", confirmBody)
	}

	eraseURL := base + "/meets/" + meetID + "/privacy/" + athleteID + "/erase"

	// A mismatched token is rejected: the confirm page re-renders with an
	// inline error, and the athlete is not erased.
	mismatch := postForm(t, client, confirmURL, eraseURL,
		url.Values{"reason": {"subject request"}, "confirm_text": {"wrong"}})
	mismatchBody := bodyString(t, mismatch)
	if mismatch.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("mismatched erase confirmation = %d, want 422", mismatch.StatusCode)
	}
	if !strings.Contains(mismatchBody, `aria-invalid="true"`) || !strings.Contains(mismatchBody, `id="confirm_text-error"`) {
		t.Errorf("mismatched erase confirmation must show an inline field error: %s", mismatchBody)
	}
	stillThere := bodyString(t, mustGet(t, client, base+"/meets/"+meetID+"/privacy"))
	if !strings.Contains(stillThere, "Anna Muster") {
		t.Error("a mismatched confirmation token must not erase the athlete")
	}

	// The correct token (the bib) erases.
	ok := postForm(t, client, confirmURL, eraseURL,
		url.Values{"reason": {"subject request"}, "confirm_text": {"1"}})
	_ = bodyString(t, ok)
	if ok.StatusCode != http.StatusSeeOther {
		t.Fatalf("correctly confirmed erase = %d, want 303", ok.StatusCode)
	}
	erased := bodyString(t, mustGet(t, client, base+"/meets/"+meetID+"/privacy"))
	if strings.Contains(erased, "Anna Muster") {
		t.Error("correctly confirmed erase must remove the athlete's original name")
	}
}

// TestRetentionPurgeConfirmFlowOQ074 covers the OQ-074 confirmation flow
// for the instance-wide retention purge: the confirm page names the fixed
// typed-confirmation token, a wrong token is rejected with an inline field
// error, and the exact token runs the purge (TestRetentionPurgeOverHTTPSYS102UC024_3
// covers the successful-run summary; this test focuses on the gate itself).
func TestRetentionPurgeConfirmFlowOQ074(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)

	confirmURL := base + "/admin/privacy/purge/confirm"
	confirmBody := bodyString(t, mustGet(t, client, confirmURL))
	if !strings.Contains(confirmBody, "PURGE") {
		t.Errorf("purge confirm page must show the literal token to type: %s", confirmBody)
	}

	mismatch := postForm(t, client, confirmURL, base+"/admin/privacy/purge", url.Values{"confirm_text": {"nope"}})
	mismatchBody := bodyString(t, mismatch)
	if mismatch.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("mismatched purge confirmation = %d, want 422", mismatch.StatusCode)
	}
	if !strings.Contains(mismatchBody, `aria-invalid="true"`) || !strings.Contains(mismatchBody, `id="confirm_text-error"`) {
		t.Errorf("mismatched purge confirmation must show an inline field error: %s", mismatchBody)
	}

	ok := postForm(t, client, confirmURL, base+"/admin/privacy/purge", url.Values{"confirm_text": {"PURGE"}})
	okBody := bodyString(t, ok)
	if ok.StatusCode != http.StatusOK {
		t.Fatalf("correctly confirmed purge = %d, want 200: %s", ok.StatusCode, okBody)
	}
}

// TestMeetArchiveConfirmUnknownMeetIs404 covers handleMeetArchiveConfirm's
// error path: an unknown meet id 404s (via renderMeetError), matching every
// other meet-scoped GET route rather than panicking or 500ing.
func TestMeetArchiveConfirmUnknownMeetIs404(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)

	resp := mustGet(t, client, base+"/meets/does-not-exist/archive/confirm")
	_ = bodyString(t, resp)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("archive confirm for an unknown meet = %d, want 404", resp.StatusCode)
	}
}

// TestAccountDisableConfirmUnknownAccountIs404 covers
// handleAccountDisableConfirm's not-found path: an account id that does not
// exist 404s rather than rendering a confirm page for nothing.
func TestAccountDisableConfirmUnknownAccountIs404(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)

	resp := mustGet(t, client, base+"/admin/accounts/does-not-exist/disable/confirm")
	_ = bodyString(t, resp)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("disable confirm for an unknown account = %d, want 404", resp.StatusCode)
	}
}

// TestPrivacyEraseConfirmUnknownAthleteIs404 covers
// handlePrivacyEraseConfirm's not-found path.
func TestPrivacyEraseConfirmUnknownAthleteIs404(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	meetID := createUCMeet(t, client, base)

	resp := mustGet(t, client, base+"/meets/"+meetID+"/privacy/does-not-exist/erase/confirm")
	_ = bodyString(t, resp)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("erase confirm for an unknown athlete = %d, want 404", resp.StatusCode)
	}
}

// TestPrivacyEraseConfirmTokenFallsBackToERASEWithoutBib covers
// eraseConfirmToken's fallback branch: a participant registered with no bib
// yet must still get a workable typed-confirmation token (the literal
// "ERASE"), and the erase POST must accept exactly that token.
func TestPrivacyEraseConfirmTokenFallsBackToERASEWithoutBib(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	meetID := createUCMeet(t, client, base)
	// No "bib" value at all — registerRosterParticipant's default always
	// sets one, so post the roster form directly without it.
	rosterURL := base + "/meets/" + meetID + "/roster"
	resp := postForm(t, client, rosterURL, rosterURL, url.Values{
		"first_name": {"Anna"}, "last_name": {"Muster"}, "birth_year": {"2011"}, "sex": {"W"},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("register participant without a bib = %d, want 303", resp.StatusCode)
	}

	privacyBody := bodyString(t, mustGet(t, client, base+"/meets/"+meetID+"/privacy"))
	athleteID := athleteIDFromPrivacyPage(t, privacyBody)
	confirmBody := bodyString(t, mustGet(t, client, base+"/meets/"+meetID+"/privacy/"+athleteID+"/erase/confirm"))
	if !strings.Contains(confirmBody, "&#34;ERASE&#34;") {
		t.Errorf("erase confirm page without a bib must show the ERASE fallback token: %s", confirmBody)
	}

	eraseResp := postForm(t, client, base+"/meets/"+meetID+"/privacy/"+athleteID+"/erase/confirm",
		base+"/meets/"+meetID+"/privacy/"+athleteID+"/erase",
		url.Values{"reason": {"subject request"}, "confirm_text": {"ERASE"}})
	_ = bodyString(t, eraseResp)
	if eraseResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("erase with the ERASE fallback token = %d, want 303", eraseResp.StatusCode)
	}
}

// TestDestructiveActionConfirmRoutesRequireRoleOQ074 mirrors the coarse
// SYS-090 role-gating pattern already proven for the POST routes: the four
// new GET confirm sub-pages are gated exactly like the actions they front.
func TestDestructiveActionConfirmRoutesRequireRoleOQ074(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)

	for _, path := range []string{
		"/admin/privacy/purge/confirm",
		"/admin/accounts/x/disable/confirm",
		"/meets/x/archive/confirm",
		"/meets/x/privacy/y/erase/confirm",
	} {
		resp := mustGet(t, client, base+path)
		_ = bodyString(t, resp)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("anonymous GET %s = %d, want 403 (SYS-090)", path, resp.StatusCode)
		}
	}
}
