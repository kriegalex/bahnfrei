// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// postCSVImport uploads csvBody as a multipart file upload against target
// (the CSV import endpoint), with the given mapping profile and action
// ("preview" or "commit") — the import form's only field the codebase-wide
// postForm helper (application/x-www-form-urlencoded) cannot drive.
func postCSVImport(t *testing.T, client *http.Client, page, target, profile, action, csvBody string) *http.Response {
	t.Helper()
	token := csrfTokenFrom(t, bodyString(t, mustGet(t, client, page)))
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if err := w.WriteField("csrf_token", token); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteField("profile", profile); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteField("action", action); err != nil {
		t.Fatal(err)
	}
	fw, err := w.CreateFormFile("file", "entries.csv")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write([]byte(csvBody)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, target, &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", target, err)
	}
	return resp
}

// TestEntriesImportEligibilityRequireOfficeRoleSYS090Web covers the role
// floor for the TASK-017 surfaces: anonymous requests are refused, and an
// entry-submitter (below competition-office) cannot reach them either.
func TestEntriesImportEligibilityRequireOfficeRoleSYS090Web(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)

	for _, path := range []string{"/meets/x/entries/import", "/meets/x/entries/eligibility"} {
		resp := mustGet(t, client, base+path)
		_ = bodyString(t, resp)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("anonymous GET %s = %d, want 403 (SYS-090)", path, resp.StatusCode)
		}
	}

	setupAndLogin(t, client, base)
	meetID := createUCMeet(t, client, base)
	createAccountWeb(t, client, base, "sub1", "entry_submitter")
	logout(t, client, base)
	login(t, client, base, "sub1", "s3cret-passphrase")

	for _, path := range []string{"/meets/" + meetID + "/entries/import", "/meets/" + meetID + "/entries/eligibility"} {
		resp := mustGet(t, client, base+path)
		_ = bodyString(t, resp)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("entry-submitter GET %s = %d, want 403 (SYS-090)", path, resp.StatusCode)
		}
	}
}

// csvImportFixture creates a published meet with one open 100m/Women event
// and a competition-office account ("office1"), returning the meet ID.
func csvImportFixture(t *testing.T, client *http.Client, base string) string {
	t.Helper()
	setupAndLogin(t, client, base)
	meetID := createUCMeet(t, client, base)
	resp := addEvent(t, client, base, meetID, url.Values{
		"discipline": {"100m"}, "categories": {"Women"}, "round_final": {"1"},
	})
	_ = resp.Body.Close()
	publishMeetWeb(t, client, base, meetID)
	createAccountWeb(t, client, base, "office1", "competition_office")
	return meetID
}

// TestCSVImportFlowSYS013UC004Web drives UC-004 over real HTTP: a
// competition-office operator previews then commits a CSV entry import, and
// the imported athlete/eligibility outcome are visible on the report and on
// the eligibility exceptions worklist.
func TestCSVImportFlowSYS013UC004Web(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	meetID := csvImportFixture(t, client, base)

	logout(t, client, base)
	login(t, client, base, "office1", "s3cret-passphrase")

	csvBody := "first_name,last_name,birth_year,sex,club,licence_no,event_code,category_code,seed_performance\n" +
		"Anna,Muster,1995,W,LC Test,,100m/Women,,12.85\n"
	importPage := base + "/meets/" + meetID + "/entries/import"
	importTarget := base + "/meets/" + meetID + "/entries/import"

	preview := postCSVImport(t, client, importPage, importTarget, "system-native", "preview", csvBody)
	previewBody := bodyString(t, preview)
	if preview.StatusCode != http.StatusOK {
		t.Fatalf("preview import = %d, want 200; body: %s", preview.StatusCode, previewBody)
	}
	if !strings.Contains(previewBody, "Anna Muster") {
		t.Errorf("preview report missing the parsed athlete: %s", previewBody)
	}

	// Preview must not have persisted anything.
	exBody := bodyString(t, mustGet(t, client, base+"/meets/"+meetID+"/entries/eligibility"))
	if strings.Contains(exBody, "Anna Muster") {
		t.Error("preview import must not appear on the eligibility worklist (nothing persisted)")
	}

	commit := postCSVImport(t, client, importPage, importTarget, "system-native", "commit", csvBody)
	commitBody := bodyString(t, commit)
	if commit.StatusCode != http.StatusOK {
		t.Fatalf("commit import = %d, want 200; body: %s", commit.StatusCode, commitBody)
	}
	if !strings.Contains(commitBody, "Anna Muster") {
		t.Errorf("commit report missing the parsed athlete: %s", commitBody)
	}

	// The import created no licence number, so this C-Meeting entry gets a
	// non-blocking eligibility warning — it must now appear on the office
	// worklist.
	exBody = bodyString(t, mustGet(t, client, base+"/meets/"+meetID+"/entries/eligibility"))
	if !strings.Contains(exBody, "Anna Muster") {
		t.Errorf("committed import must appear on the eligibility worklist: %s", exBody)
	}
}

// TestCSVImportMalformedRowSYS013UC004_3Web covers UC-004 #3 over HTTP: a
// row with a missing birth year is reported rejected with its reason.
func TestCSVImportMalformedRowSYS013UC004_3Web(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	meetID := csvImportFixture(t, client, base)
	logout(t, client, base)
	login(t, client, base, "office1", "s3cret-passphrase")

	csvBody := "first_name,last_name,birth_year,sex,club,licence_no,event_code,category_code,seed_performance\n" +
		"Bea,NoBirthYear,,W,LC Test,,100m/Women,,13.00\n"
	page := base + "/meets/" + meetID + "/entries/import"
	resp := postCSVImport(t, client, page, page, "system-native", "commit", csvBody)
	body := bodyString(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("commit import = %d, want 200; body: %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, "NoBirthYear") {
		t.Errorf("report missing the rejected row's name: %s", body)
	}
}

// TestEligibilityOverrideFlowSYS014UC005_4Web drives UC-005 #4 over real
// HTTP: an online entry (which collects no licence number) is flagged on
// the office worklist, and an office operator's override with a reason
// clears it — the override actor/reason then render on the same page.
func TestEligibilityOverrideFlowSYS014UC005_4Web(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	meetID := csvImportFixture(t, client, base)
	createAccountWeb(t, client, base, "sub1", "entry_submitter")

	eventID := mustEventID(t, deps, meetID, "100m")
	logout(t, client, base)
	login(t, client, base, "sub1", "s3cret-passphrase")
	entriesPage := base + "/meets/" + meetID + "/entries"
	resp := postForm(t, client, entriesPage, base+"/meets/"+meetID+"/entries/individual", url.Values{
		"event": {eventID}, "first_name": {"Nora"}, "last_name": {"Weber"},
		"birth_year": {"1995"}, "sex": {"W"}, "club": {"LC Test"}, "seed": {"12.90"},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("submit individual entry = %d, want 303", resp.StatusCode)
	}

	logout(t, client, base)
	login(t, client, base, "office1", "s3cret-passphrase")
	eligPage := base + "/meets/" + meetID + "/entries/eligibility"
	body := bodyString(t, mustGet(t, client, eligPage))
	if !strings.Contains(body, "Nora Weber") {
		t.Fatalf("eligibility worklist missing the flagged entry: %s", body)
	}

	// Extract the override form's entry id from the action URL.
	const marker = "/entries/"
	i := strings.Index(body, marker)
	if i < 0 {
		t.Fatalf("no entry override form found in body: %s", body)
	}
	rest := body[i+len(marker):]
	entryID := rest[:strings.Index(rest, "/eligibility/override")]

	resp = postForm(t, client, eligPage, base+"/meets/"+meetID+"/entries/"+entryID+"/eligibility/override", url.Values{
		"version": {"1"}, "reason": {"licence confirmed by phone"},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("override eligibility = %d, want 303", resp.StatusCode)
	}

	body = bodyString(t, mustGet(t, client, eligPage))
	if !strings.Contains(body, "office1") || !strings.Contains(body, "licence confirmed by phone") {
		t.Errorf("eligibility worklist missing the recorded override: %s", body)
	}
}
