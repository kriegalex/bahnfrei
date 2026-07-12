// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/kriegalex/bahnfrei/internal/app"
	"github.com/kriegalex/bahnfrei/internal/exchange"
)

// webOrganizer mirrors webOffice/webSubmitter's precedent (seeding_test.go):
// bib assignment (SYS-018) is a meet-organizer action, one level above
// competition-office.
var webOrganizer = app.Session{AccountID: "01WOG", Username: "webOrganizer", Role: app.RoleMeetOrganizer}

// TestTimingRoutesRequireOfficeRoleSYS090Web is the denial-first
// authorization test for the office timing-exchange surface: anonymous
// requests are refused (SYS-090).
func TestTimingRoutesRequireOfficeRoleSYS090Web(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	for _, path := range []string{
		"/meets/x/timing", "/meets/x/timing/export/ppl", "/meets/x/timing/export/sch",
		"/meets/x/timing/export/evt", "/meets/x/timing/export/csv",
	} {
		resp := mustGet(t, client, base+path)
		_ = bodyString(t, resp)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("anonymous GET %s = %d, want 403 (SYS-090)", path, resp.StatusCode)
		}
	}
}

// TestAgentAPIRequiresBearerTokenWeb is the denial-first path for the
// unattended timing-agent API (ADR-006): no header, a garbage token, and a
// token scoped to a different meet must all be refused with 401 — the
// agent API is deliberately not gated by the browser-session role system
// at all (see middleware.go's CSRF exemption and routes.go's comment).
func TestAgentAPIRequiresBearerTokenWeb(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)
	meetID := createUCMeet(t, client, base)

	// No Authorization header at all.
	resp, err := client.Get(base + "/agent/v1/meets/" + meetID + "/exports/manifest")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("no auth header: status = %d, want 401", resp.StatusCode)
	}

	// A garbage bearer token.
	req, _ := http.NewRequest(http.MethodGet, base+"/agent/v1/meets/"+meetID+"/exports/manifest", nil)
	req.Header.Set("Authorization", "Bearer not-a-real-token")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("garbage token: status = %d, want 401", resp.StatusCode)
	}

	// A real token, but for a different meet id in the path.
	plaintext, _, err := deps.results.CreateTimingAgentToken(context.Background(), webOffice, meetID, "timing PC")
	if err != nil {
		t.Fatalf("CreateTimingAgentToken: %v", err)
	}
	req, _ = http.NewRequest(http.MethodGet, base+"/agent/v1/meets/some-other-meet/exports/manifest", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("mismatched meet: status = %d, want 401", resp.StatusCode)
	}
}

// bibParticipants assigns sequential bibs to every one of meetID's
// participants (SubmitIndividualEntry leaves bib empty, TASK-016;
// timing-exchange export/import needs a real bib to identify athletes) and
// returns them in participant-list order.
func bibParticipants(t *testing.T, deps *testServerDeps, meetID string, start int) []string {
	t.Helper()
	ctx := context.Background()
	participants, err := deps.results.Participants(ctx, meetID)
	if err != nil {
		t.Fatalf("Participants: %v", err)
	}
	var bibs []string
	for i, p := range participants {
		bib := strconv.Itoa(start + i)
		if err := deps.results.AssignBib(ctx, webOrganizer, meetID, p.ID, p.Version, bib); err != nil {
			t.Fatalf("AssignBib: %v", err)
		}
		bibs = append(bibs, bib)
	}
	return bibs
}

// TestTimingExportImportResolveEndToEndWeb drives the full UC-014 flow over
// real HTTP: export the seeded start list as FinishLynx files, upload a
// .lif with one unknown bib (a genuine conflict, SYS-061), see it queued on
// the timing page, and resolve it by discarding.
func TestTimingExportImportResolveEndToEndWeb(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	meetID, eventID, roundID := seededMeetFixture(t, deps, client, base, 2)
	bibs := bibParticipants(t, deps, meetID, 501)

	seedingPage := base + "/meets/" + meetID + "/events/" + eventID + "/rounds/" + roundID + "/seeding"
	genResp := postForm(t, client, seedingPage, seedingPage+"/generate", url.Values{"max_heat_size": {"8"}, "track_lanes": {"8"}})
	_ = genResp.Body.Close()
	if genResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("generate heats = %d, want 303", genResp.StatusCode)
	}

	timingPageURL := base + "/meets/" + meetID + "/timing"
	indexBody := bodyString(t, mustGet(t, client, timingPageURL))
	if !strings.Contains(indexBody, "lynx.ppl") {
		t.Fatalf("timing page missing export links:\n%s", indexBody)
	}

	evtResp := mustGet(t, client, base+"/meets/"+meetID+"/timing/export/evt")
	evtBody := []byte(bodyString(t, evtResp))
	if evtResp.StatusCode != http.StatusOK {
		t.Fatalf("GET export/evt = %d", evtResp.StatusCode)
	}
	events, err := exchange.ParseEVT(evtBody)
	if err != nil || len(events) != 1 || len(events[0].Competitors) != 2 {
		t.Fatalf("ParseEVT: events=%+v err=%v\n%s", events, err, evtBody)
	}

	// The other two FinishLynx files, plus the generic CSV fallback, also
	// download cleanly (UC-014 #1/#6).
	pplResp := mustGet(t, client, base+"/meets/"+meetID+"/timing/export/ppl")
	pplBody := []byte(bodyString(t, pplResp))
	if pplResp.StatusCode != http.StatusOK {
		t.Fatalf("GET export/ppl = %d", pplResp.StatusCode)
	}
	if people, err := exchange.ParsePPL(pplBody); err != nil || len(people) != 2 {
		t.Fatalf("ParsePPL: people=%+v err=%v\n%s", people, err, pplBody)
	}
	schResp := mustGet(t, client, base+"/meets/"+meetID+"/timing/export/sch")
	schBody := []byte(bodyString(t, schResp))
	if schResp.StatusCode != http.StatusOK {
		t.Fatalf("GET export/sch = %d", schResp.StatusCode)
	}
	if sched, err := exchange.ParseSCH(schBody); err != nil || len(sched) != 1 {
		t.Fatalf("ParseSCH: sched=%+v err=%v\n%s", sched, err, schBody)
	}
	csvResp := mustGet(t, client, base+"/meets/"+meetID+"/timing/export/csv")
	csvBody := bodyString(t, csvResp)
	if csvResp.StatusCode != http.StatusOK || !strings.Contains(csvBody, bibs[0]) {
		t.Fatalf("GET export/csv = %d:\n%s", csvResp.StatusCode, csvBody)
	}

	// Agent token issuance/revocation through the office UI (ADR-006).
	tokenResp := postForm(t, client, timingPageURL, timingPageURL+"/agents", url.Values{"label": {"Timing PC"}})
	tokenBody := bodyString(t, tokenResp)
	if tokenResp.StatusCode != http.StatusOK || !strings.Contains(tokenBody, "Timing PC") {
		t.Fatalf("POST timing/agents = %d:\n%s", tokenResp.StatusCode, tokenBody)
	}
	tokens, err := deps.results.ListTimingAgentTokens(context.Background(), webOffice, meetID)
	if err != nil || len(tokens) != 1 {
		t.Fatalf("ListTimingAgentTokens: %+v, err=%v", tokens, err)
	}
	revokeResp := postForm(t, client, timingPageURL, timingPageURL+"/agents/"+tokens[0].ID+"/revoke", url.Values{})
	_ = revokeResp.Body.Close()
	if revokeResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST agents/{id}/revoke = %d, want 303", revokeResp.StatusCode)
	}
	foundBib := map[string]bool{}
	for _, c := range events[0].Competitors {
		foundBib[c.ID] = true
	}
	for _, b := range bibs {
		if !foundBib[b] {
			t.Errorf("bib %s missing from exported .evt", b)
		}
	}

	// Upload a .lif for the same event/round/heat but with one bib that
	// matches no participant — a genuine SYS-061 conflict.
	ev := events[0]
	ev.Competitors = []exchange.CompetitorRow{{Place: "1", ID: "999999", Lane: 1, Time: "12.34"}}
	lif := exchange.EncodeLIF(ev)

	uploadResp := postMultipartFile(t, client, timingPageURL, timingPageURL+"/import", map[string]string{"format": "lif"}, "file", "meet.lif", lif)
	uploadBody := bodyString(t, uploadResp)
	if uploadResp.StatusCode != http.StatusOK {
		t.Fatalf("POST timing/import = %d:\n%s", uploadResp.StatusCode, uploadBody)
	}
	if !strings.Contains(uploadBody, "999999") {
		t.Fatalf("import result page missing the conflicting bib:\n%s", uploadBody)
	}

	conflicts, err := deps.results.ListTimingImportConflicts(context.Background(), webOffice, meetID)
	if err != nil {
		t.Fatalf("ListTimingImportConflicts: %v", err)
	}
	if len(conflicts) != 1 || conflicts[0].Reason != "unknown_bib" {
		t.Fatalf("conflicts = %+v", conflicts)
	}

	resolveResp := postForm(t, client, timingPageURL, timingPageURL+"/conflicts/"+conflicts[0].ID+"/resolve", url.Values{"action": {"discard"}})
	_ = resolveResp.Body.Close()
	if resolveResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("resolve conflict = %d, want 303", resolveResp.StatusCode)
	}
	remaining, err := deps.results.ListTimingImportConflicts(context.Background(), webOffice, meetID)
	if err != nil {
		t.Fatalf("ListTimingImportConflicts (after resolve): %v", err)
	}
	for _, c := range remaining {
		if c.Status == "pending" {
			t.Fatalf("conflict %s is still pending after discard", c.ID)
		}
	}
}

// TestAgentAPIExportAndImportWeb covers the timing agent's two real
// interactions: polling the export manifest/files and uploading a .lif —
// with a valid Bearer token, mirroring what cmd/bahnfrei/timingagent.go's
// HTTP client does.
func TestAgentAPIExportAndImportWeb(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	meetID, eventID, roundID := seededMeetFixture(t, deps, client, base, 1)
	bibs := bibParticipants(t, deps, meetID, 701)

	seedingPage := base + "/meets/" + meetID + "/events/" + eventID + "/rounds/" + roundID + "/seeding"
	genResp := postForm(t, client, seedingPage, seedingPage+"/generate", url.Values{"max_heat_size": {"8"}, "track_lanes": {"8"}})
	_ = genResp.Body.Close()

	plaintext, _, err := deps.results.CreateTimingAgentToken(context.Background(), webOffice, meetID, "timing PC")
	if err != nil {
		t.Fatalf("CreateTimingAgentToken: %v", err)
	}

	manifestReq, _ := http.NewRequest(http.MethodGet, base+"/agent/v1/meets/"+meetID+"/exports/manifest", nil)
	manifestReq.Header.Set("Authorization", "Bearer "+plaintext)
	manifestResp, err := client.Do(manifestReq)
	if err != nil {
		t.Fatal(err)
	}
	manifestBody := bodyString(t, manifestResp)
	if manifestResp.StatusCode != http.StatusOK {
		t.Fatalf("GET manifest = %d:\n%s", manifestResp.StatusCode, manifestBody)
	}
	var manifest struct{ PPLSHA256, SCHSHA256, EVTSHA256 string }
	if err := json.Unmarshal([]byte(manifestBody), &manifest); err != nil {
		t.Fatalf("unmarshal manifest: %v\n%s", err, manifestBody)
	}
	if manifest.EVTSHA256 == "" {
		t.Fatalf("manifest missing evt hash: %+v", manifest)
	}

	evtReq, _ := http.NewRequest(http.MethodGet, base+"/agent/v1/meets/"+meetID+"/exports/evt", nil)
	evtReq.Header.Set("Authorization", "Bearer "+plaintext)
	evtResp, err := client.Do(evtReq)
	if err != nil {
		t.Fatal(err)
	}
	evtBody := bodyString(t, evtResp)
	events, err := exchange.ParseEVT([]byte(evtBody))
	if err != nil || len(events) != 1 {
		t.Fatalf("ParseEVT: %v\n%s", err, evtBody)
	}
	ev := events[0]
	if len(ev.Competitors) != 1 || ev.Competitors[0].ID != bibs[0] {
		t.Fatalf("evt competitors = %+v, want bib %s", ev.Competitors, bibs[0])
	}

	// Agent uploads a matching .lif — should apply cleanly (SYS-061, UC-014
	// #3), no conflict.
	ev.Competitors[0].Place = "1"
	ev.Competitors[0].Time = "13.20"
	lif := exchange.EncodeLIF(ev)
	uploadReq, _ := http.NewRequest(http.MethodPost, base+"/agent/v1/meets/"+meetID+"/lif", bytes.NewReader(lif))
	uploadReq.Header.Set("Authorization", "Bearer "+plaintext)
	uploadReq.Header.Set("X-Filename", "agent-upload.lif")
	uploadResp, err := client.Do(uploadReq)
	if err != nil {
		t.Fatal(err)
	}
	uploadBody := bodyString(t, uploadResp)
	if uploadResp.StatusCode != http.StatusOK {
		t.Fatalf("agent lif upload = %d:\n%s", uploadResp.StatusCode, uploadBody)
	}
	var summary struct{ Applied, Conflicts int }
	if err := json.Unmarshal([]byte(uploadBody), &summary); err != nil {
		t.Fatalf("unmarshal upload response: %v\n%s", err, uploadBody)
	}
	if summary.Applied != 1 || summary.Conflicts != 0 {
		t.Fatalf("summary = %+v, want 1 applied / 0 conflicts", summary)
	}

	// Agent uploads the generic CSV fallback too (SYS-062): a refreshed
	// mark for the same athlete applies cleanly since the existing result
	// is itself import-sourced (not a manual entry — SYS-061 only guards
	// against overwriting a manual result).
	csvData := exchange.EncodeCSV([]exchange.GenericRow{
		{EventNumber: ev.Number, Round: ev.Round, Heat: ev.Heat, Bib: bibs[0], Lane: ev.Competitors[0].Lane, Mark: "13.10", Timing: "manual"},
	})
	csvReq, _ := http.NewRequest(http.MethodPost, base+"/agent/v1/meets/"+meetID+"/csv", bytes.NewReader(csvData))
	csvReq.Header.Set("Authorization", "Bearer "+plaintext)
	csvResp, err := client.Do(csvReq)
	if err != nil {
		t.Fatal(err)
	}
	csvBody := bodyString(t, csvResp)
	if csvResp.StatusCode != http.StatusOK {
		t.Fatalf("agent csv upload = %d:\n%s", csvResp.StatusCode, csvBody)
	}
	var csvSummary struct{ Applied, Conflicts int }
	if err := json.Unmarshal([]byte(csvBody), &csvSummary); err != nil {
		t.Fatalf("unmarshal csv upload response: %v\n%s", err, csvBody)
	}
	if csvSummary.Applied != 1 || csvSummary.Conflicts != 0 {
		t.Fatalf("csv summary = %+v, want 1 applied / 0 conflicts", csvSummary)
	}
}

// postMultipartFile uploads fileContent as a multipart file upload
// (mirrors postCSVImport's convention, internal/web/import_test.go),
// generalized to accept arbitrary extra form fields.
func postMultipartFile(t *testing.T, client *http.Client, page, target string, fields map[string]string, fileField, filename string, fileContent []byte) *http.Response {
	t.Helper()
	token := csrfTokenFrom(t, bodyString(t, mustGet(t, client, page)))
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if err := w.WriteField("csrf_token", token); err != nil {
		t.Fatal(err)
	}
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	fw, err := w.CreateFormFile(fileField, filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(fileContent); err != nil {
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
