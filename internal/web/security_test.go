// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// TestLoginRateLimiterUnit exercises the failure-budget bookkeeping in
// isolation with a controllable clock (SYS-092, ASVS L2 V2.2.1).
func TestLoginRateLimiterUnit(t *testing.T) {
	now := time.Unix(0, 0)
	l := newLoginRateLimiter(3, time.Minute)
	l.now = func() time.Time { return now }

	const ip = "10.0.0.1"
	if l.blocked(ip) {
		t.Fatal("fresh key must not be blocked")
	}
	for i := 0; i < 3; i++ {
		if l.blocked(ip) {
			t.Fatalf("blocked prematurely after %d failures", i)
		}
		l.fail(ip)
	}
	if !l.blocked(ip) {
		t.Fatal("key must be blocked once the failure budget is spent")
	}

	// A different source is unaffected (per-key isolation).
	if l.blocked("10.0.0.2") {
		t.Fatal("an unrelated source must not be blocked")
	}

	// Window expiry clears the block.
	now = now.Add(time.Minute + time.Second)
	if l.blocked(ip) {
		t.Fatal("block must lapse once the window has elapsed")
	}

	// A successful login resets an in-progress counter.
	l.fail(ip)
	l.fail(ip)
	l.reset(ip)
	l.fail(ip)
	if l.blocked(ip) {
		t.Fatal("reset must clear the accumulated failures")
	}
}

// TestLoginBruteForceThrottledSYS092Web drives the anti-automation gate over
// real HTTP: repeated wrong-password submissions from one source are 401 up
// to the budget, then 429; a correct login before the budget is spent clears
// the counter (SYS-092, ASVS L2 V2.2.1).
func TestLoginBruteForceThrottledSYS092Web(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	if _, err := deps.auth.Bootstrap(context.Background(), "admin", "Administrator", "s3cret-passphrase"); err != nil {
		t.Fatal(err)
	}
	token := csrfTokenFrom(t, bodyString(t, mustGet(t, client, base+"/login")))

	wrong := func() *http.Response {
		resp, err := client.PostForm(base+"/login", url.Values{
			"username": {"admin"}, "password": {"nope"}, "csrf_token": {token},
		})
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	for i := 0; i < loginFailLimit; i++ {
		resp := wrong()
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("failed login %d = %d, want 401", i+1, resp.StatusCode)
		}
	}
	// Budget spent: the next attempt is throttled before any password check.
	blockedResp := wrong()
	_ = blockedResp.Body.Close()
	if blockedResp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("over-budget login = %d, want 429", blockedResp.StatusCode)
	}
	if blockedResp.Header.Get("Retry-After") == "" {
		t.Error("429 response must carry a Retry-After header")
	}

	// Even the correct password is refused while blocked (fail-closed).
	goodWhileBlocked, err := client.PostForm(base+"/login", url.Values{
		"username": {"admin"}, "password": {"s3cret-passphrase"}, "csrf_token": {token},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = goodWhileBlocked.Body.Close()
	if goodWhileBlocked.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("correct login while blocked = %d, want 429", goodWhileBlocked.StatusCode)
	}
}

// TestLoginSuccessResetsThrottleSYS092Web confirms a successful login clears
// the failure counter, so honest operators who mistype once are never locked
// out (SYS-092, ASVS L2 V2.2.1).
func TestLoginSuccessResetsThrottleSYS092Web(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	if _, err := deps.auth.Bootstrap(context.Background(), "admin", "Administrator", "s3cret-passphrase"); err != nil {
		t.Fatal(err)
	}
	token := csrfTokenFrom(t, bodyString(t, mustGet(t, client, base+"/login")))

	// A handful of failures short of the budget.
	for i := 0; i < loginFailLimit-1; i++ {
		resp, err := client.PostForm(base+"/login", url.Values{
			"username": {"admin"}, "password": {"nope"}, "csrf_token": {token},
		})
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
	}
	// A correct login resets the counter.
	good, err := client.PostForm(base+"/login", url.Values{
		"username": {"admin"}, "password": {"s3cret-passphrase"}, "csrf_token": {token},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = good.Body.Close()
	if good.StatusCode != http.StatusSeeOther {
		t.Fatalf("correct login = %d, want 303", good.StatusCode)
	}
	// A subsequent wrong login is 401 again, not immediately 429 — proof the
	// counter was cleared.
	after, err := client.PostForm(base+"/login", url.Values{
		"username": {"admin"}, "password": {"nope"}, "csrf_token": {token},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = after.Body.Close()
	if after.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong login after a successful reset = %d, want 401", after.StatusCode)
	}
}

// TestHSTSHeaderOnlyInACMEModeSYS092Web verifies HSTS is emitted only where a
// publicly-trusted certificate exists (ACME mode) and never in venue-local
// self-signed mode, where it would forbid the operator's cert-warning
// click-through and lock the venue out (SYS-092/SYS-093, ASVS L2 V14.4.5).
func TestHSTSHeaderOnlyInACMEModeSYS092Web(t *testing.T) {
	const hdr = "Strict-Transport-Security"

	acme := newTestServer(t, TLSConfig{Mode: TLSModeACME})
	acmeClient, acmeBase := newTestClient(t, acme)
	resp := mustGet(t, acmeClient, acmeBase+"/healthz")
	_ = resp.Body.Close()
	if resp.Header.Get(hdr) == "" {
		t.Error("ACME mode must send Strict-Transport-Security")
	}

	local := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	localClient, localBase := newTestClient(t, local)
	resp = mustGet(t, localClient, localBase+"/healthz")
	_ = resp.Body.Close()
	if got := resp.Header.Get(hdr); got != "" {
		t.Errorf("venue-local mode must NOT send HSTS, got %q", got)
	}
}

// TestAuthenticatedResponsesAreNoStoreSYS092Web verifies personal-data-bearing
// authenticated responses are marked no-store while anonymous pages are not
// (SYS-092, ASVS L2 V8.2.1 / nFADP minimization).
func TestAuthenticatedResponsesAreNoStoreSYS092Web(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)

	// Anonymous login page: not marked no-store.
	anon := mustGet(t, client, base+"/login")
	_ = anon.Body.Close()
	if cc := anon.Header.Get("Cache-Control"); strings.Contains(cc, "no-store") {
		t.Errorf("anonymous page Cache-Control = %q, must not be no-store", cc)
	}

	setupAndLogin(t, client, base)

	authed := mustGet(t, client, base+"/admin")
	_ = authed.Body.Close()
	if cc := authed.Header.Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Errorf("authenticated page Cache-Control = %q, want no-store", cc)
	}
}

// TestOversizedImportUploadRejectedSYS092Web proves the global request-body
// cap (limitRequestBody) refuses an over-ceiling CSV-import upload with 413
// before the CSRF middleware ever buffers it to memory or a temp file
// (SYS-092, ASVS L2 V12.1.1). The timing-import form is covered by
// TestOversizedTimingUploadRejectedSYS092Web; both share the middleware guard.
func TestOversizedImportUploadRejectedSYS092Web(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	meetID := createUCMeet(t, client, base)

	oversized := strings.Repeat("x", maxRequestBodyBytes+1<<10)
	page := base + "/meets/" + meetID + "/entries/import"
	resp := postCSVImport(t, client, page, page, "system-native", "preview", oversized)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized import upload = %d, want 413", resp.StatusCode)
	}
}

// TestOversizedTimingUploadRejectedSYS092Web proves the timing-import form is
// bounded by the same global request-body cap (SYS-092, ASVS L2 V12.1.1).
func TestOversizedTimingUploadRejectedSYS092Web(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	meetID := createUCMeet(t, client, base)
	timingImport := base + "/meets/" + meetID + "/timing/import"

	oversized := bytes.Repeat([]byte("x"), maxRequestBodyBytes+1<<10)
	resp := postMultipartFile(t, client, base+"/meets/"+meetID+"/timing", timingImport,
		map[string]string{"format": "lif"}, "file", "big.lif", oversized)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized timing upload = %d, want 413", resp.StatusCode)
	}
}
