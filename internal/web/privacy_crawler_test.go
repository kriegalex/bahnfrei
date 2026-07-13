// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/kriegalex/bahnfrei/internal/app"
	"github.com/kriegalex/bahnfrei/internal/domain"
)

// --- TASK-029 privacy-review finding #4: a public-path crawler regression
// (docs/delivery/reviews/privacy-review-task-023.md). The existing
// SYS-100/SYS-103 minimization tests (public_test.go) enumerate a fixed
// list of page paths by hand; the review's concern is that the choke
// point (domain.PublicDisplayNameFor/PublicDisplayClubFor) is
// discipline-enforced, not type-enforced — a future public renderer could
// bypass it, and a hand-maintained path list would not catch a NEW public
// page nobody remembered to add to it. This crawls the public meet
// surface by following <a href> links from the meet overview (the same
// way a real visitor reaches every page), so a newly linked public page
// is swept automatically; the known routes that are never plain-linked
// (the SSE-refreshed live-results fragment) are still explicitly probed,
// since a pure link-follow crawl would never reach them. ---

// publicHrefRE extracts every href="..." attribute value from a rendered
// page — the crawler's link-discovery mechanism.
var publicHrefRE = regexp.MustCompile(`href="([^"]+)"`)

// crawlPublicSurface walks every public (/m/{id}/...) page reachable by
// following <a href> links from start, breadth-first, plus the given
// mustVisit paths, and returns every visited path's response body. Only
// /m/... hrefs are followed — office/login links that happen to appear in
// the rendered nav are not part of the public surface this crawler
// audits, and following them would require an authenticated session to
// mean anything.
func crawlPublicSurface(t *testing.T, client *http.Client, base, start string, mustVisit []string) map[string]string {
	t.Helper()
	visited := map[string]string{}
	queue := append([]string{start}, mustVisit...)
	for len(queue) > 0 {
		path := queue[0]
		queue = queue[1:]
		if _, ok := visited[path]; ok {
			continue
		}
		resp := mustGet(t, client, base+path)
		body := bodyString(t, resp)
		visited[path] = body
		if resp.StatusCode != http.StatusOK {
			continue
		}
		for _, m := range publicHrefRE.FindAllStringSubmatch(body, -1) {
			href := m[1]
			if !strings.HasPrefix(href, "/m/") {
				continue
			}
			if _, ok := visited[href]; !ok {
				queue = append(queue, href)
			}
		}
	}
	return visited
}

// TestPublicPathCrawlerNeverLeaksWithdrawnOrErasedNameSYS100SYS101UC024
// is the crawler regression the privacy review asked for: a
// publication-withdrawn athlete's name (SYS-103) and an erased athlete's
// pre-erasure name (SYS-101) must appear on NO page the crawl reaches —
// not just the specific pages an earlier test happened to check. The
// fixture seeds heats (so the start-list page's heat/lane sections are
// swept too) and publishes the timetable, so every one of the five known
// public route shapes is populated with real content, not an empty page
// that would trivially "pass" by having nothing to leak.
func TestPublicPathCrawlerNeverLeaksWithdrawnOrErasedNameSYS100SYS101UC024(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	meetID, eventID, roundID := seededMeetFixture(t, deps, client, base, 1)
	ctx := context.Background()

	// A publication-withdrawn entrant (SYS-103): entered with consent
	// withdrawn up front, confirmed and seeded like any other entrant, so
	// their row exists everywhere a normal entrant's would — only the
	// identity must be suppressed.
	withdrawn, err := deps.results.SubmitIndividualEntry(ctx, webSubmitter, meetID, app.IndividualEntryInput{
		EventID: eventID, FirstName: "Wanda", LastName: "Withdrawn",
		BirthYear: 2009, Sex: domain.SexFemale, SeedPerformance: "13.10",
		PublicationWithdrawn: true,
	})
	if err != nil {
		t.Fatalf("SubmitIndividualEntry (withdrawn): %v", err)
	}
	if err := deps.results.ConfirmCheckIn(ctx, webOffice, meetID, withdrawn.ID, withdrawn.Version); err != nil {
		t.Fatalf("ConfirmCheckIn (withdrawn): %v", err)
	}

	// An entrant who is public right up until an SYS-101 erasure request
	// runs (below) — the crawler must find no trace of the pre-erasure
	// name anywhere either, the same standard as the withdrawn case.
	erasee, err := deps.results.SubmitIndividualEntry(ctx, webSubmitter, meetID, app.IndividualEntryInput{
		EventID: eventID, FirstName: "Erik", LastName: "Erased",
		BirthYear: 2009, Sex: domain.SexFemale, SeedPerformance: "13.20",
	})
	if err != nil {
		t.Fatalf("SubmitIndividualEntry (erasee): %v", err)
	}
	if err := deps.results.ConfirmCheckIn(ctx, webOffice, meetID, erasee.ID, erasee.Version); err != nil {
		t.Fatalf("ConfirmCheckIn (erasee): %v", err)
	}

	seedingPage := base + "/meets/" + meetID + "/events/" + eventID + "/rounds/" + roundID + "/seeding"
	genResp := postForm(t, client, seedingPage, seedingPage+"/generate", url.Values{"max_heat_size": {"8"}, "track_lanes": {"8"}})
	_ = genResp.Body.Close()

	scheduleAndPublishTimetable(t, client, base, meetID)

	if err := deps.privacy.EraseAthlete(ctx, webOffice, erasee.AthleteID, "subject request"); err != nil {
		t.Fatalf("EraseAthlete: %v", err)
	}

	anon, _ := newTestClient(t, deps)
	mustVisit := []string{
		"/m/" + meetID,
		"/m/" + meetID + "/timetable",
		"/m/" + meetID + "/startlists",
		"/m/" + meetID + "/results",
		"/m/" + meetID + "/results/live", // never plain-linked; SSE-fragment swap only
	}
	visited := crawlPublicSurface(t, anon, base, "/m/"+meetID, mustVisit)

	for _, want := range mustVisit {
		if _, ok := visited[want]; !ok {
			t.Errorf("crawler never reached %s", want)
		}
	}

	startlists, ok := visited["/m/"+meetID+"/startlists"]
	if !ok || !strings.Contains(startlists, "Athlete") {
		t.Fatalf("fixture error: start-list page has no seeded heat rows to scan: %s", startlists)
	}

	for path, body := range visited {
		if strings.Contains(body, "Wanda") && strings.Contains(body, "Withdrawn") {
			t.Errorf("%s leaks the withdrawn athlete's name", path)
		}
		if strings.Contains(body, "Erik") && strings.Contains(body, "Erased") {
			t.Errorf("%s leaks the erased athlete's pre-erasure name", path)
		}
	}
}
