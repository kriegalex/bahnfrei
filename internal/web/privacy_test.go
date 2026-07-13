// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// TestPrivacyRoutesRequireOfficeOrAdminRoleSYS101SYS103UC023UC024 covers
// the deny path for every TASK-023 route: an anonymous visitor gets 403
// on the office-gated data-subject-rights surfaces (SYS-090 least
// privilege) and on the instance-admin-gated retention-purge trigger,
// before any entity lookup happens.
func TestPrivacyRoutesRequireOfficeOrAdminRoleSYS101SYS103UC023UC024(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)

	officeGated := []struct {
		method, path string
	}{
		{"GET", "/meets/x/privacy"},
		{"POST", "/meets/x/privacy/y/consent"},
		{"GET", "/meets/x/privacy/y/export"},
		{"POST", "/meets/x/privacy/y/erase"},
	}
	for _, tc := range officeGated {
		req, err := http.NewRequest(tc.method, base+tc.path, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = bodyString(t, resp)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("anonymous %s %s = %d, want 403", tc.method, tc.path, resp.StatusCode)
		}
	}

	adminGated := []struct{ method, path string }{
		{"GET", "/admin/privacy"},
		{"POST", "/admin/privacy/purge"},
	}
	for _, tc := range adminGated {
		req, err := http.NewRequest(tc.method, base+tc.path, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = bodyString(t, resp)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("anonymous %s %s = %d, want 403", tc.method, tc.path, resp.StatusCode)
		}
	}
}

// registerRosterParticipant is the shared roster-add helper this file's
// tests use, mirroring public_test.go's inline roster POST but exposing
// the SYS-103 publication_withdrawn checkbox UC-023 requires the entry
// flow to collect.
func registerRosterParticipant(t *testing.T, client *http.Client, base, meetID string, extra url.Values) {
	t.Helper()
	values := url.Values{
		"first_name": {"Anna"}, "last_name": {"Muster"},
		"birth_year": {"2011"}, "sex": {"W"}, "bib": {"1"},
	}
	for k, v := range extra {
		values[k] = v
	}
	resp := postForm(t, client, base+"/meets/"+meetID+"/roster", base+"/meets/"+meetID+"/roster", values)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("register participant = %d, want 303", resp.StatusCode)
	}
}

// athleteIDFromPrivacyPage scrapes the athlete ID out of the privacy
// worklist's export link — the simplest way to recover the ID this
// package's HTTP-only tests never see directly (mirrors
// scheduleAndPublishTimetable's approach of reading an ID back out of
// rendered HTML rather than re-deriving it).
func athleteIDFromPrivacyPage(t *testing.T, body string) string {
	t.Helper()
	const marker = "/privacy/"
	i := strings.Index(body, marker)
	if i < 0 {
		t.Fatalf("privacy page has no export link: %s", body)
	}
	rest := body[i+len(marker):]
	return rest[:strings.Index(rest, "/export")]
}

// TestPrivacyConsentAtEntryAndToggleOverHTTPSYS103UC023 covers UC-023
// end to end over HTTP: the roster entry flow collects the SYS-103
// publication-consent choice up front (a minor registered with the
// withdrawal box checked never shows their name on the public pages —
// UC-023 #2), and toggling consent afterward on the privacy worklist
// changes the very next public fetch (UC-023 #3).
func TestPrivacyConsentAtEntryAndToggleOverHTTPSYS103UC023(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	meetID := createUCMeet(t, client, base)
	_ = addEvent(t, client, base, meetID, url.Values{
		"discipline": {"100m"}, "categories": {"U16 W"}, "round_final": {"1"},
	}).Body.Close()

	registerRosterParticipant(t, client, base, meetID, url.Values{"publication_withdrawn": {"true"}})

	anon, _ := newTestClient(t, deps)
	t.Run("deny: withdrawn-at-entry athlete never appears on public start lists", func(t *testing.T) {
		body := bodyString(t, mustGet(t, anon, base+"/m/"+meetID+"/startlists"))
		if strings.Contains(body, "Anna Muster") {
			t.Errorf("start list leaked a withdrawn athlete's name: %s", body)
		}
		if !strings.Contains(body, "—") {
			t.Errorf("start list missing the suppression marker: %s", body)
		}
	})

	privacyBody := bodyString(t, mustGet(t, client, base+"/meets/"+meetID+"/privacy"))
	if !strings.Contains(privacyBody, "Anna Muster") {
		t.Fatalf("office privacy worklist must still show the real name: %s", privacyBody)
	}
	athleteID := athleteIDFromPrivacyPage(t, privacyBody)

	t.Run("allow: office restores consent, reflected on the next public fetch (UC-023 #3)", func(t *testing.T) {
		resp := postForm(t, client, base+"/meets/"+meetID+"/privacy",
			base+"/meets/"+meetID+"/privacy/"+athleteID+"/consent", url.Values{"withdrawn": {"false"}})
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("consent toggle = %d, want 303", resp.StatusCode)
		}
		body := bodyString(t, mustGet(t, anon, base+"/m/"+meetID+"/startlists"))
		if !strings.Contains(body, "Anna Muster") {
			t.Errorf("restored consent must show the real name on the very next fetch: %s", body)
		}
	})
}

// TestPrivacyExportOverHTTPSYS101UC024_1 covers the SYS-101 subject-access
// export end to end: the office-gated route serves a JSON download
// containing the athlete's own data.
func TestPrivacyExportOverHTTPSYS101UC024_1(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	meetID := createUCMeet(t, client, base)
	registerRosterParticipant(t, client, base, meetID, nil)

	privacyBody := bodyString(t, mustGet(t, client, base+"/meets/"+meetID+"/privacy"))
	athleteID := athleteIDFromPrivacyPage(t, privacyBody)

	resp := mustGet(t, client, base+"/meets/"+meetID+"/privacy/"+athleteID+"/export")
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	body := bodyString(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("export = %d, want 200: %s", resp.StatusCode, body)
	}
	var export struct {
		FirstName      string `json:"first_name"`
		LastName       string `json:"last_name"`
		Participations []struct {
			Bib string `json:"Bib"`
		} `json:"participations"`
	}
	if err := json.Unmarshal([]byte(body), &export); err != nil {
		t.Fatalf("export body is not valid JSON: %v: %s", err, body)
	}
	if export.FirstName != "Anna" || export.LastName != "Muster" {
		t.Errorf("export identity = %+v", export)
	}
}

// TestPrivacyEraseOverHTTPSYS101UC024_2 covers the SYS-101 erasure
// use-case end to end: after erasure the office worklist shows the
// anonymized status and no longer offers the withdraw/restore/erase
// actions for that row (an already-anonymized athlete has nothing left to
// act on).
func TestPrivacyEraseOverHTTPSYS101UC024_2(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	meetID := createUCMeet(t, client, base)
	registerRosterParticipant(t, client, base, meetID, nil)

	privacyBody := bodyString(t, mustGet(t, client, base+"/meets/"+meetID+"/privacy"))
	athleteID := athleteIDFromPrivacyPage(t, privacyBody)

	// OQ-074 (TASK-034): erasure requires the confirm sub-page's typed
	// confirmation (the athlete's bib, "1" here per registerRosterParticipant)
	// in addition to the reason.
	resp := postForm(t, client, base+"/meets/"+meetID+"/privacy",
		base+"/meets/"+meetID+"/privacy/"+athleteID+"/erase",
		url.Values{"reason": {"subject request"}, "confirm_text": {"1"}})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("erase = %d, want 303", resp.StatusCode)
	}

	after := bodyString(t, mustGet(t, client, base+"/meets/"+meetID+"/privacy"))
	if strings.Contains(after, "Anna Muster") {
		t.Errorf("privacy worklist must not show the erased athlete's original name: %s", after)
	}
}

// TestRetentionPurgeOverHTTPSYS102UC024_3 covers the SYS-102 manual
// retention-purge trigger: an instance-admin can run it and sees the
// run's summary; a repeat run against a fresh instance (nothing yet out
// of retention) reports zero, proving the trigger is safe to click
// speculatively.
func TestRetentionPurgeOverHTTPSYS102UC024_3(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)

	form := bodyString(t, mustGet(t, client, base+"/admin/privacy"))
	if !strings.Contains(form, "90") {
		t.Errorf("retention-purge form should surface the configured retention period: %s", form)
	}

	// OQ-074 (TASK-034): the purge requires the confirm sub-page's fixed
	// typed-confirmation token in addition to the plain confirm step.
	resp := postForm(t, client, base+"/admin/privacy", base+"/admin/privacy/purge", url.Values{"confirm_text": {"PURGE"}})
	body := bodyString(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("purge = %d, want 200: %s", resp.StatusCode, body)
	}
}

// TestPrivacyExportUnknownAthleteReturns404 covers renderPrivacyError's
// app.ErrAthleteNotFound branch: handlePrivacyExport never scopes the
// athlete lookup to the meetID in the URL (CapPrivacyActions is instance-
// wide for the office role, unlike the per-event field-official scoping
// TASK-013 added elsewhere), so the adversarial case worth pinning here is
// simply an athlete ID that does not exist at all.
func TestPrivacyExportUnknownAthleteReturns404(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	meetID := createUCMeet(t, client, base)

	resp := mustGet(t, client, base+"/meets/"+meetID+"/privacy/does-not-exist/export")
	_ = bodyString(t, resp)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("export unknown athlete = %d, want 404", resp.StatusCode)
	}
}

// TestPrivacyEraseAlreadyAnonymizedRedirectsWithError covers the
// ErrAthleteAnonymized error path handlePrivacyErase falls into (redirect
// with "invalid" flash, rendered on the very next fetch) — a repeat erase
// request must not silently succeed or crash, since the office UI's erase
// button remains clickable on an already-anonymized row until the page is
// refreshed.
func TestPrivacyEraseAlreadyAnonymizedRedirectsWithError(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	meetID := createUCMeet(t, client, base)
	registerRosterParticipant(t, client, base, meetID, nil)

	privacyBody := bodyString(t, mustGet(t, client, base+"/meets/"+meetID+"/privacy"))
	athleteID := athleteIDFromPrivacyPage(t, privacyBody)

	// OQ-074 (TASK-034): erasure requires the confirm sub-page's typed
	// confirmation (the athlete's bib, "1" here per registerRosterParticipant).
	first := postForm(t, client, base+"/meets/"+meetID+"/privacy",
		base+"/meets/"+meetID+"/privacy/"+athleteID+"/erase",
		url.Values{"reason": {"subject request"}, "confirm_text": {"1"}})
	_ = first.Body.Close()
	if first.StatusCode != http.StatusSeeOther {
		t.Fatalf("first erase = %d, want 303", first.StatusCode)
	}

	// The second attempt targets an already-anonymized athlete. Erasure
	// only touches the athletes table, never the participant/bib
	// relationship, so the bib "1" (and therefore the confirm token) is
	// unchanged — the request reaches EraseAthlete, which is what exercises
	// the ErrAthleteAnonymized redirect this test pins.
	second := postForm(t, client, base+"/meets/"+meetID+"/privacy",
		base+"/meets/"+meetID+"/privacy/"+athleteID+"/erase",
		url.Values{"reason": {"repeat request"}, "confirm_text": {"1"}})
	_ = second.Body.Close()
	if second.StatusCode != http.StatusSeeOther {
		t.Fatalf("repeat erase = %d, want 303", second.StatusCode)
	}
	if loc := second.Header.Get("Location"); loc != "/meets/"+meetID+"/privacy?err=invalid" {
		t.Errorf("repeat erase Location = %q, want .../privacy?err=invalid", loc)
	}
	errBody := bodyString(t, mustGet(t, client, base+second.Header.Get("Location")))
	if !strings.Contains(errBody, "nicht ausgeführt") {
		t.Errorf("repeat erase should render the localized error message: %s", errBody)
	}
}

// TestPrivacyConsentToggleUnknownAthleteRedirectsError covers
// handlePrivacyConsentToggle's error branch: ResultsService.SetConsent
// fails for an athlete ID that does not exist, and the handler must
// redirect with the "invalid" flash rather than error out.
func TestPrivacyConsentToggleUnknownAthleteRedirectsError(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	meetID := createUCMeet(t, client, base)

	resp := postForm(t, client, base+"/meets/"+meetID+"/privacy",
		base+"/meets/"+meetID+"/privacy/does-not-exist/consent", url.Values{"withdrawn": {"true"}})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("consent toggle on unknown athlete = %d, want 303", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/meets/"+meetID+"/privacy?err=invalid" {
		t.Errorf("consent toggle on unknown athlete Location = %q, want .../privacy?err=invalid", loc)
	}
}

// TestPrivacyRoutesServe404WhenPrivacyServiceUnwired covers the s.privacy
// == nil defensive branch in handlePrivacyExport/handlePrivacyErase/
// handleRetentionPurge: a Server built without SetPrivacy (a deployment
// mode or wiring bug) must serve 404 rather than nil-dereference-panic.
func TestPrivacyRoutesServe404WhenPrivacyServiceUnwired(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	// Rebuild a Server against the same app-layer deps but skip SetPrivacy.
	cfg := Config{Addr: "127.0.0.1:0", TLS: TLSConfig{Mode: TLSModeLocal}, AppVersion: "test"}
	bareServer := New(cfg, deps.auth, deps.sessions, deps.meets, deps.results, deps.backup, deps.cats, deps.bus)
	srv := httptest.NewServer(bareServer.routes())
	defer srv.Close()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	base := srv.URL
	setupAndLogin(t, client, base)
	meetID := createUCMeet(t, client, base)
	registerRosterParticipant(t, client, base, meetID, nil)
	privacyBody := bodyString(t, mustGet(t, client, base+"/meets/"+meetID+"/privacy"))
	athleteID := athleteIDFromPrivacyPage(t, privacyBody)

	resp := mustGet(t, client, base+"/meets/"+meetID+"/privacy/"+athleteID+"/export")
	_ = bodyString(t, resp)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("export with no privacy service wired = %d, want 404", resp.StatusCode)
	}

	resp = postForm(t, client, base+"/meets/"+meetID+"/privacy",
		base+"/meets/"+meetID+"/privacy/"+athleteID+"/erase", url.Values{"reason": {"x"}})
	_ = bodyString(t, resp)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("erase with no privacy service wired = %d, want 404", resp.StatusCode)
	}

	resp = postForm(t, client, base+"/admin/privacy", base+"/admin/privacy/purge", url.Values{})
	_ = bodyString(t, resp)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("retention purge with no privacy service wired = %d, want 404", resp.StatusCode)
	}
}
