// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"testing"
)

// --- SYS-105 egress-blocked public-asset check (TASK-027): "In
// self-hosted deployment, all personal data SHALL remain on
// operator-controlled infrastructure; no telemetry or external service
// calls containing personal data SHALL occur without explicit opt-in."
// ADR-003 already commits to vendoring every asset the shell serves
// (htmx, base.css, capture/public-live JS — internal/web/static/*,
// embedded via go:embed; confirmed by repo-wide grep: no cdn/googleapis/
// jsdelivr/unpkg/htmx.org reference exists in any template or static
// asset today). This test turns that one-time confirmation into a
// machine-checked regression guard, two ways:
//
//  1. A poisoned net/http transport installed as http.DefaultTransport for
//     the crawl's duration: it dials loopback addresses normally and fails
//     the test immediately on any other dial attempt. This is the
//     stronger of the two approaches the task brief allows ("a Go test
//     intercepting outbound dials... or a Playwright route-block
//     approach") — it proves the SERVER PROCESS itself never reaches out
//     while rendering these pages, not just that the rendered HTML
//     happens to omit an external URL. A Playwright route-block only
//     proves a BROWSER wouldn't fetch externally; it says nothing about
//     server-side egress (e.g. a future telemetry call fired from Go
//     code, never touching the page DOM at all), and it would add a
//     browser dependency plus the e2e suite's own flake risk
//     (chaos-m1.spec.ts already has a documented one under load) to a
//     check a fast, deterministic Go test proves more directly. Chosen
//     over Playwright for exactly that reason: stronger guarantee, zero
//     added flake/tooling cost, runs in the default `go test ./...` gate.
//  2. A static scan of every crawled page's rendered HTML for an absolute
//     http(s):// resource reference (href/src/action) pointing off-origin
//     — the same thing a real browser would have tried to fetch, closing
//     the gap the transport-level check alone would miss (a reference a
//     browser would follow but this test's own Go HTTP client never
//     issues, since crawlPublicSurface only follows /m/... hrefs).
//
// Together they prove the public surface renders fully (SYS-070/UC-017)
// with every asset while genuinely unable to reach any non-local host.
func TestPublicSurfaceRendersWithEgressBlockedSYS105(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	meetID, eventID, roundID := seededMeetFixture(t, deps, client, base, 6)

	seedingPage := base + "/meets/" + meetID + "/events/" + eventID + "/rounds/" + roundID + "/seeding"
	genResp := postForm(t, client, seedingPage, seedingPage+"/generate", url.Values{"max_heat_size": {"8"}, "track_lanes": {"8"}})
	_ = genResp.Body.Close()
	scheduleAndPublishTimetable(t, client, base, meetID)

	restore := blockNonLocalEgress(t)
	defer restore()

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
		body, ok := visited[want]
		if !ok {
			t.Errorf("egress-blocked crawl never reached %s", want)
			continue
		}
		if body == "" {
			t.Errorf("%s rendered an empty body under egress block", want)
		}
	}

	// Every static asset the shell references must also fetch fine with
	// egress blocked — they are embedded in the binary, never proxied or
	// fetched from anywhere (ADR-003).
	for _, asset := range []string{"/static/htmx.min.js", "/static/base.css", "/static/public-live.js"} {
		resp := mustGet(t, anon, base+asset)
		body := bodyString(t, resp)
		if resp.StatusCode != http.StatusOK || body == "" {
			t.Errorf("GET %s under egress block = %d (want 200 with a non-empty body)", asset, resp.StatusCode)
		}
	}

	assertNoExternalResourceReferences(t, visited)
}

// externalResourceRE matches href/src/action attributes whose value is an
// absolute http(s):// URL. A same-origin reference in every internal/web
// template is always a bare path (/static/..., /m/...) — ADR-003 commits
// to no CDN, no external fetches.
var externalResourceRE = regexp.MustCompile(`(?:href|src|action)="(https?://[^"]+)"`)

func assertNoExternalResourceReferences(t *testing.T, pages map[string]string) {
	t.Helper()
	for path, body := range pages {
		for _, m := range externalResourceRE.FindAllStringSubmatch(body, -1) {
			t.Errorf("%s references an absolute external URL %q (SYS-105: no external fetches)", path, m[1])
		}
	}
}

// blockNonLocalEgress replaces the process-global http.DefaultTransport
// (what an http.Client with a zero-value Transport field uses — every
// client this test package's helpers construct) with one that dials
// loopback addresses normally and fails the test immediately on any other
// dial attempt. It returns a restore func the caller MUST defer: other
// tests in this package share the same process-global default.
func blockNonLocalEgress(t *testing.T) (restore func()) {
	t.Helper()
	original := http.DefaultTransport
	dialer := &net.Dialer{}
	http.DefaultTransport = &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				host = addr
			}
			if !isLoopbackHost(host) {
				t.Errorf("egress-blocked test attempted a non-loopback dial to %s (SYS-105)", addr)
				return nil, errors.New("blocked: non-loopback dial (SYS-105 egress guard)")
			}
			return dialer.DialContext(ctx, network, addr)
		},
	}
	return func() { http.DefaultTransport = original }
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
