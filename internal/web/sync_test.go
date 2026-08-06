// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
)

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	return u
}

// postJSON sends a JSON body with the double-submit CSRF header (SYS-092) the
// client island uses, and returns the response.
func postJSON(t *testing.T, client *http.Client, target, csrf string, body any) *http.Response {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, target, bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrf)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", target, err)
	}
	return resp
}

// syncSetup logs in, creates the UKC fixture and returns the zone-long-jump
// unit URL, the athlete-id map, and the CSRF token.
func syncSetup(t *testing.T, client *http.Client, base string) (unitURL string, athletes map[string]string, csrf string) {
	t.Helper()
	setupAndLogin(t, client, base)
	meetID, units := ukcCaptureFixture(t, client, base)
	unitURL = base + "/meets/" + meetID + "/capture/" + units["Zonen-Weitsprung (UKC)"]
	body := bodyString(t, mustGet(t, client, unitURL)) // opening the unit checks it out (SYS-086)
	return unitURL, athleteIDsFrom(t, body), csrfTokenFrom(t, body)
}

func checkoutStamp(t *testing.T, client *http.Client, unitURL, csrf string) checkoutResponse {
	t.Helper()
	resp := postJSON(t, client, unitURL+"/checkout", csrf, checkoutRequest{DeviceLabel: "tablet-A"})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("checkout = %d, want 200", resp.StatusCode)
	}
	var co checkoutResponse
	if err := json.NewDecoder(resp.Body).Decode(&co); err != nil {
		t.Fatalf("decode checkout: %v", err)
	}
	return co
}

func replay(t *testing.T, client *http.Client, unitURL, csrf string, req syncRequest) syncResponse {
	t.Helper()
	resp := postJSON(t, client, unitURL+"/sync", csrf, req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("sync = %d, want 200", resp.StatusCode)
	}
	var out syncResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode sync: %v", err)
	}
	return out
}

// TestUC034_2_IdempotentReplayOverJSON drives the offline replay endpoint the
// way the client island will (SYS-085): an ordered batch applies, and a
// re-sent batch produces only duplicates — no second write, standings intact.
func TestUC034_2_IdempotentReplayOverJSON(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	unitURL, athletes, csrf := syncSetup(t, client, base)
	co := checkoutStamp(t, client, unitURL, csrf)

	req := syncRequest{
		Token: co.Token, DeviceLabel: "tablet-A", Generation: co.Generation, StartListVersion: co.StartListVersion,
		Ops: []syncOp{
			{OpID: "01OP0000000000000000000001", AthleteID: athletes["101"], Seq: 1, Value: "3.42"},
			{OpID: "01OP0000000000000000000002", AthleteID: athletes["102"], Seq: 1, Value: "3.50"},
		},
	}
	out := replay(t, client, unitURL, csrf, req)
	if len(out.Results) != 2 || out.Results[0].Status != "applied" || out.Results[1].Status != "applied" {
		t.Fatalf("first replay results = %+v, want two applied", out.Results)
	}

	out = replay(t, client, unitURL, csrf, req)
	for _, r := range out.Results {
		if r.Status != "duplicate" {
			t.Errorf("re-replay status = %q, want duplicate (SYS-085)", r.Status)
		}
	}

	standings := bodyString(t, mustGet(t, client, unitURL+"/standings"))
	if !strings.Contains(standings, "3.50") || !strings.Contains(standings, "3.42") {
		t.Errorf("standings missing replayed marks: %s", standings)
	}
}

// TestUC034_5_StaleCheckoutReconcilesOverJSON: after the office overrides the
// checkout, the original device's replay lands in reconciliation (surfaced on
// the office page), never silently applied (UC-034 #5, SYS-086).
func TestUC034_5_StaleCheckoutReconcilesOverJSON(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	unitURL, athletes, csrf := syncSetup(t, client, base)
	stale := checkoutStamp(t, client, unitURL, csrf)

	// Office overrides the checkout (bumps the generation).
	resp := postForm(t, client, unitURL, unitURL+"/override", url.Values{"device": {"tablet-B"}, "reason": {"device lost"}})
	_ = bodyString(t, resp)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("override = %d, want 303", resp.StatusCode)
	}

	out := replay(t, client, unitURL, csrf, syncRequest{
		Token: stale.Token, DeviceLabel: "tablet-A", Generation: stale.Generation, StartListVersion: stale.StartListVersion,
		Ops: []syncOp{{OpID: "01OP0000000000000000000010", AthleteID: athletes["101"], Seq: 1, Value: "3.42"}},
	})
	if len(out.Results) != 1 || out.Results[0].Status != "reconciliation" || out.Results[0].Reason != "stale_checkout" {
		t.Fatalf("stale replay results = %+v, want reconciliation/stale_checkout", out.Results)
	}

	// The office reconciliation page surfaces the item; standings show nothing.
	meetURL := unitURL[:strings.Index(unitURL, "/capture/")]
	recon := bodyString(t, mustGet(t, client, meetURL+"/reconciliation"))
	if !strings.Contains(recon, "Zone Long Jump (UKC)") {
		t.Errorf("reconciliation page missing the unit: %s", recon)
	}
	standings := bodyString(t, mustGet(t, client, unitURL+"/standings"))
	if strings.Contains(standings, "3.42") {
		t.Error("stale capture must not appear in standings (never silently applied)")
	}
}

// TestUC034_ReconciliationApplyOverJSON: an office operator applies a
// reconciled item from the page and it lands as a real capture; the pending
// list then empties (UC-034 #4/#5).
func TestUC034_ReconciliationApplyOverJSON(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	unitURL, athletes, csrf := syncSetup(t, client, base)
	co := checkoutStamp(t, client, unitURL, csrf)

	// Force a reconciliation via an office start-list revision.
	resp := postForm(t, client, unitURL, unitURL+"/revise-startlist", url.Values{})
	_ = bodyString(t, resp)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("revise-startlist = %d, want 303", resp.StatusCode)
	}
	out := replay(t, client, unitURL, csrf, syncRequest{
		Token: co.Token, DeviceLabel: "tablet-A", Generation: co.Generation, StartListVersion: co.StartListVersion,
		Ops: []syncOp{{OpID: "01OP0000000000000000000020", AthleteID: athletes["101"], Seq: 1, Value: "3.42"}},
	})
	if out.Results[0].Status != "reconciliation" || out.Results[0].Reason != "start_list_change" {
		t.Fatalf("results = %+v, want reconciliation/start_list_change", out.Results)
	}

	meetURL := unitURL[:strings.Index(unitURL, "/capture/")]
	recon := bodyString(t, mustGet(t, client, meetURL+"/reconciliation"))
	itemID := reconItemID(t, recon, "apply")

	applyResp := postForm(t, client, meetURL+"/reconciliation", meetURL+"/reconciliation/"+itemID+"/apply", url.Values{})
	_ = bodyString(t, applyResp)
	if applyResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("apply = %d, want 303", applyResp.StatusCode)
	}
	standings := bodyString(t, mustGet(t, client, unitURL+"/standings"))
	if !strings.Contains(standings, "3.42") {
		t.Error("applied reconciliation item must land in standings")
	}
	recon = bodyString(t, mustGet(t, client, meetURL+"/reconciliation"))
	if !strings.Contains(recon, "Keine offenen") { // "no pending" empty state (DE)
		t.Errorf("reconciliation queue should be empty after apply: %s", recon)
	}
}

// TestSyncPerOpRejectionSYS149UC040_1OverJSON is the F1 defect regression
// (usability-audit-volunteer-2026-08.md) at the HTTP boundary: saving an
// invalid mark ("abc") no longer fails the whole /sync request with 400 —
// the endpoint still answers 200 with a per-op "rejected" outcome, and a
// second, valid op in the SAME batch still applies (UC-040 #1/#2: a
// rejection never blocks the queue).
func TestSyncPerOpRejectionSYS149UC040_1OverJSON(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	unitURL, athletes, csrf := syncSetup(t, client, base)
	co := checkoutStamp(t, client, unitURL, csrf)

	resp := postJSON(t, client, unitURL+"/sync", csrf, syncRequest{
		Token: co.Token, DeviceLabel: "tablet-A", Generation: co.Generation, StartListVersion: co.StartListVersion,
		Ops: []syncOp{
			{OpID: "01OP00000000000000000J001", AthleteID: athletes["101"], Seq: 1, Value: "abc"},
			{OpID: "01OP00000000000000000J002", AthleteID: athletes["102"], Seq: 1, Value: "3.80"},
		},
	})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("sync with an invalid op = %d, want 200 (a per-op rejection is not a batch-wide HTTP error)", resp.StatusCode)
	}
	var out syncResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode sync: %v", err)
	}
	if len(out.Results) != 2 {
		t.Fatalf("results = %+v, want 2", out.Results)
	}
	if out.Results[0].Status != "rejected" || out.Results[0].Reason != "invalid_mark" {
		t.Errorf("op 0 = %+v, want rejected/invalid_mark", out.Results[0])
	}
	if out.Results[1].Status != "applied" {
		t.Errorf("op 1 = %+v, want applied (a rejection must not block the queue, UC-040 #2)", out.Results[1])
	}

	standings := bodyString(t, mustGet(t, client, unitURL+"/standings"))
	if !strings.Contains(standings, "3.80") {
		t.Error("the valid op behind the rejected one must still reach standings")
	}
}

// TestSyncSYS149UC040_4ExpiredSessionIs403 is the server half of UC-040 #4
// (SYS-149): a request whose session cookie no longer resolves to a live
// session — the real shape of the 12h TTL lapsing mid-queue
// (internal/web/config.go) — never reaches handleUnitSync's own error
// mapping at all: captureRole (routes.go, requireRole(RoleFieldOfficial))
// already answers with its own 403 upstream. This pins that behavior so a
// future routing change cannot silently regress the client's 401/403
// re-authentication classification (islands/src/capture-offline.ts) back
// into a 500/400 the client would treat as a generic sync error instead of
// a re-authentication prompt.
func TestSyncSYS149UC040_4ExpiredSessionIs403(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	unitURL, athletes, csrf := syncSetup(t, client, base)
	co := checkoutStamp(t, client, unitURL, csrf)

	// Corrupt the session cookie in place — same shape as a stale/expired
	// cookie the browser still sends but the server can no longer resolve.
	u := mustParseURL(t, base)
	client.Jar.SetCookies(u, []*http.Cookie{{Name: sessionCookieName, Value: "not-a-real-session-token"}})

	resp := postJSON(t, client, unitURL+"/sync", csrf, syncRequest{
		Token: co.Token, DeviceLabel: "tablet-A", Generation: co.Generation, StartListVersion: co.StartListVersion,
		Ops: []syncOp{{OpID: "01OP00000000000000000E001", AthleteID: athletes["101"], Seq: 1, Value: "3.40"}},
	})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("sync with an unresolvable session cookie = %d, want 403 (the client's reauth classification depends on 401/403)", resp.StatusCode)
	}
}

// TestUnitCheckoutRejectsUnassignedFieldOfficialWeb covers handleUnitCheckout's
// app.ErrUnitNotAssigned branch (SYS-090's per-event scoping): a field
// official never assigned to this meet's units is refused with a 403 JSON
// body, not a 200/500.
func TestUnitCheckoutRejectsUnassignedFieldOfficialWeb(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	unitURL, _, _ := syncSetup(t, client, base)

	createAccountWeb(t, client, base, "unassigned-fo", "field_official")
	logout(t, client, base)
	login(t, client, base, "unassigned-fo", "s3cret-passphrase")
	csrf := csrfTokenFrom(t, bodyString(t, mustGet(t, client, base+"/")))

	resp := postJSON(t, client, unitURL+"/checkout", csrf, checkoutRequest{DeviceLabel: "tablet-C"})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("checkout by an unassigned field official = %d, want 403", resp.StatusCode)
	}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body["error"] != "unit_not_assigned" {
		t.Errorf("error body = %+v, want unit_not_assigned", body)
	}
}

// TestUnitCheckoutConflictBetweenAccountsWeb covers handleUnitCheckout's
// app.ErrCheckedOutByAnother branch (SYS-086): once one account's device
// actively holds a unit's capture lock, a different account's checkout
// attempt is refused with 409 until the office overrides it — never
// silently reassigned.
func TestUnitCheckoutConflictBetweenAccountsWeb(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	// Opening the unit page as admin takes the checkout (EnsureCheckout,
	// SYS-086) under admin's account.
	unitURL, _, _ := syncSetup(t, client, base)
	createAccountWeb(t, client, base, "office2", "competition_office")

	jar2, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client2 := &http.Client{Jar: jar2, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	login(t, client2, base, "office2", "s3cret-passphrase")
	csrf2 := csrfTokenFrom(t, bodyString(t, mustGet(t, client2, base+"/")))

	resp := postJSON(t, client2, unitURL+"/checkout", csrf2, checkoutRequest{DeviceLabel: "tablet-B"})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("checkout by a different account while actively held = %d, want 409", resp.StatusCode)
	}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body["error"] != "checked_out_by_another" {
		t.Errorf("error body = %+v, want checked_out_by_another", body)
	}
}

// TestUnitSyncRejectsMalformedJSONAndUnassignedOfficialWeb covers
// handleUnitSync's two non-happy-path branches: a malformed JSON body never
// reaches ReplayCaptureBatch (400, not a panic or 500), and an unassigned
// field official's otherwise well-formed batch is refused (403,
// unit_not_assigned) — mirroring handleUnitCheckout's equivalent gates.
func TestUnitSyncRejectsMalformedJSONAndUnassignedOfficialWeb(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	unitURL, _, csrf := syncSetup(t, client, base)

	req, err := http.NewRequest(http.MethodPost, unitURL+"/sync", bytes.NewReader([]byte("{not valid json")))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrf)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = bodyString(t, resp)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("malformed sync body = %d, want 400", resp.StatusCode)
	}

	createAccountWeb(t, client, base, "unassigned-fo2", "field_official")
	logout(t, client, base)
	login(t, client, base, "unassigned-fo2", "s3cret-passphrase")
	csrf2 := csrfTokenFrom(t, bodyString(t, mustGet(t, client, base+"/")))

	syncResp := postJSON(t, client, unitURL+"/sync", csrf2, syncRequest{
		Ops: []syncOp{{OpID: "01OP0000000000000000000099", AthleteID: "irrelevant", Seq: 1, Value: "3.42"}},
	})
	defer func() { _ = syncResp.Body.Close() }()
	if syncResp.StatusCode != http.StatusForbidden {
		t.Fatalf("sync by an unassigned field official = %d, want 403", syncResp.StatusCode)
	}
	var body map[string]string
	if err := json.NewDecoder(syncResp.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body["error"] != "unit_not_assigned" {
		t.Errorf("error body = %+v, want unit_not_assigned", body)
	}
}

// TestCheckoutOverrideAndReviseStartListUnknownUnitIs404Web covers
// handleCheckoutOverride's and handleReviseStartList's shared error-mapping
// path: a syntactically fine but nonexistent unit id within a real meet
// surfaces the shared 404, not a 500.
func TestCheckoutOverrideAndReviseStartListUnknownUnitIs404Web(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	meetID := createUCMeet(t, client, base)
	page := base + "/meets/" + meetID
	badUnitURL := base + "/meets/" + meetID + "/capture/does-not-exist"

	overrideResp := postForm(t, client, page, badUnitURL+"/override", url.Values{"device": {"tablet-B"}, "reason": {"lost"}})
	_ = bodyString(t, overrideResp)
	if overrideResp.StatusCode != http.StatusNotFound {
		t.Errorf("checkout override on an unknown unit = %d, want 404", overrideResp.StatusCode)
	}

	reviseResp := postForm(t, client, page, badUnitURL+"/revise-startlist", url.Values{})
	_ = bodyString(t, reviseResp)
	if reviseResp.StatusCode != http.StatusNotFound {
		t.Errorf("revise-startlist on an unknown unit = %d, want 404", reviseResp.StatusCode)
	}
}

// TestReconciliationUnknownMeetAndItemIs404Web covers handleReconciliation's
// and handleReconciliationResolve's error-mapping paths.
func TestReconciliationUnknownMeetAndItemIs404Web(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)

	resp := mustGet(t, client, base+"/meets/does-not-exist/reconciliation")
	_ = bodyString(t, resp)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET reconciliation for an unknown meet = %d, want 404", resp.StatusCode)
	}

	meetID := createUCMeet(t, client, base)
	page := base + "/meets/" + meetID + "/reconciliation"
	applyResp := postForm(t, client, page, page+"/does-not-exist/apply", url.Values{})
	_ = bodyString(t, applyResp)
	if applyResp.StatusCode != http.StatusNotFound {
		t.Errorf("resolve an unknown reconciliation item = %d, want 404", applyResp.StatusCode)
	}
}

// TestDeviceSuffixRenders covers deviceSuffix's pure formatting: the default
// "web" label (and blank) render no suffix at all, a real device label
// renders parenthesized.
func TestDeviceSuffixRenders(t *testing.T) {
	for _, tc := range []struct{ label, want string }{
		{"", ""}, {"web", ""}, {"tablet-A", " (tablet-A)"},
	} {
		if got := deviceSuffix(tc.label); got != tc.want {
			t.Errorf("deviceSuffix(%q) = %q, want %q", tc.label, got, tc.want)
		}
	}
}

// reconItemID extracts the reconciliation item id from an apply/discard form
// action on the reconciliation page.
func reconItemID(t *testing.T, body, verb string) string {
	t.Helper()
	marker := "/reconciliation/"
	for {
		i := strings.Index(body, marker)
		if i < 0 {
			break
		}
		rest := body[i+len(marker):]
		end := strings.Index(rest, "/"+verb)
		if end >= 0 && end < 40 {
			return rest[:end]
		}
		body = rest
	}
	t.Fatalf("no reconciliation %s action found", verb)
	return ""
}
