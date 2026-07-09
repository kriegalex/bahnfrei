// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

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
	unitURL = base + "/meets/" + meetID + "/capture/" + units["Zone Long Jump (UKC)"]
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
