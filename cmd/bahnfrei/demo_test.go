// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"
)

var demoMeetIDRe = regexp.MustCompile(`meet id:\s+([0-9A-Z]+)`)

// TestDemoSeedProducesWorkingMeet proves the M1 demo seed (TASK-015,
// DEC-011): a fresh data dir seeded by "bahnfrei demo" boots into a meet
// the founder can demo immediately — every demo account logs in, the
// standings and public results pages already carry marks, and the field
// official opens its assigned capture unit under real per-event scoping.
// A second seed into the same dir is refused (non-fresh guard).
func TestDemoSeedProducesWorkingMeet(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Fresh dir → seed; the printed summary carries the meet ID.
	dataDir := t.TempDir()
	var seedOut bytes.Buffer
	if err := runDemo(ctx, []string{"--data-dir", dataDir}, &seedOut); err != nil {
		t.Fatalf("runDemo: %v", err)
	}
	m := demoMeetIDRe.FindStringSubmatch(seedOut.String())
	if m == nil {
		t.Fatalf("seed output has no meet id line:\n%s", seedOut.String())
	}
	meetID := m[1]

	// Seeding again into the same dir must refuse politely.
	if err := runDemo(ctx, []string{"--data-dir", dataDir}, io.Discard); err == nil {
		t.Error("second runDemo on the same dir succeeded, want refusal")
	}

	// Boot the real server on the seeded dir.
	cfg, err := parseServeFlags([]string{"--data-dir", dataDir}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	deps, err := buildServer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = deps.dbase.Close() }()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- deps.server.Serve(ctx, ln) }()
	base := "https://" + ln.Addr().String()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{
		Jar: jar,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Timeout: 10 * time.Second,
	}
	waitHealthy(t, client, base, serveErr)

	get := func(path string) (int, string) {
		t.Helper()
		resp, err := client.Get(base + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer func() { _ = resp.Body.Close() }()
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, string(b)
	}
	login := func(user, pass string) {
		t.Helper()
		_, page := get("/login")
		resp, err := client.PostForm(base+"/login", url.Values{
			"username": {user}, "password": {pass},
			"csrf_token": {csrfToken(t, page)},
		})
		if err != nil {
			t.Fatalf("POST /login (%s): %v", user, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("login %s = %d, want 303", user, resp.StatusCode)
		}
	}

	// The public results page needs no login and must already render
	// seeded marks: a roster athlete, a division and a captured mark.
	code, pub := get("/m/" + meetID + "/results")
	if code != http.StatusOK {
		t.Fatalf("public results = %d, want 200", code)
	}
	for _, want := range []string{"Brunner", "Moser", "W12", "8.42", "4.12"} {
		if !strings.Contains(pub, want) {
			t.Errorf("public results page missing %q (seeded results should render)", want)
		}
	}

	// Every demo account logs in with the printed credentials.
	login("organizer", "demo-organizer-pw")
	login("office", "demo-office-pw")

	// The office standings view is non-empty: seeded athletes rank with
	// totals in their divisions.
	code, standings := get("/meets/" + meetID + "/standings")
	if code != http.StatusOK {
		t.Fatalf("standings = %d, want 200", code)
	}
	for _, want := range []string{"Bachmann", "M12", "W10"} {
		if !strings.Contains(standings, want) {
			t.Errorf("standings page missing %q (seeded standings should be non-empty)", want)
		}
	}

	// The demo-script deliverables render from the seed: the UKC series
	// upload export and the result-list PDF are non-empty.
	code, export := get("/meets/" + meetID + "/export/ukc-series")
	if code != http.StatusOK || len(export) == 0 {
		t.Errorf("ukc-series export = %d (%d bytes), want 200 with content", code, len(export))
	}
	code, pdf := get("/meets/" + meetID + "/standings.pdf")
	if code != http.StatusOK || !strings.HasPrefix(pdf, "%PDF") {
		t.Errorf("standings.pdf = %d, want 200 with a PDF body", code)
	}

	// The field official sees exactly its two assigned units on the
	// capture index (per-event scoping, SYS-090) and can open one.
	login("official", "demo-official-pw")
	code, index := get("/meets/" + meetID + "/capture")
	if code != http.StatusOK {
		t.Fatalf("capture index = %d, want 200", code)
	}
	unitRe := regexp.MustCompile(`href="(/meets/` + meetID + `/capture/[0-9A-Z]+)"`)
	links := unitRe.FindAllStringSubmatch(index, -1)
	if len(links) != 2 {
		t.Fatalf("official's capture index lists %d units, want the 2 assigned", len(links))
	}
	code, unit := get(links[0][1])
	if code != http.StatusOK {
		t.Fatalf("assigned capture unit = %d, want 200", code)
	}
	if !strings.Contains(unit, "Brunner") && !strings.Contains(unit, "Moser") {
		t.Error("capture unit page shows no seeded participants")
	}

	cancel()
	if err := <-serveErr; err != nil {
		t.Fatalf("server shut down with error: %v", err)
	}
}
