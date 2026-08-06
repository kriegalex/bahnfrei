// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package main

import (
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

// TestQuickstartFreshInstallE2E is the UC-001 fresh-install end-to-end
// test (SYS-131 + SYS-001/002/004/006): starting from an empty data
// directory, it drives exactly what the README quickstart tells a
// first-time operator to do — over HTTPS against the real server wiring,
// with no configuration file and no direct database access:
//
//  1. first visit lands on setup, an admin account is created (#1),
//  2. a meet with two days, two sessions/day, tier C-Meeting is created (#2),
//  3. 100m / Shot Put / 4×100m for U16 W join the programme with
//     discipline-correct capture types and deadlines (#3),
//  4. the timetable is published, amended, republished — the public page
//     shows the amended time and both versions stay retrievable (#4),
//  5. the sanctioning summary contains every SYS-006 field (#5).
//
// The ≤30-minute wall-clock budget of UC-001 #1 is a human-pace bound; the
// machine-checkable proxy here is that the whole flow is automated
// end-to-end (nothing manual left besides reading pages) and completes in
// test time.
func TestQuickstartFreshInstallE2E(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Fresh install: an empty data dir, venue defaults (self-signed TLS).
	cfg, err := parseServeFlags([]string{"--data-dir", t.TempDir()}, io.Discard)
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
			// The venue-local certificate is self-signed by design
			// (SYS-093); the README tells the operator to accept it once.
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Timeout: 10 * time.Second,
	}

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
	post := func(fromPath, toPath string, form url.Values) *http.Response {
		t.Helper()
		_, page := get(fromPath)
		form.Set("csrf_token", csrfToken(t, page))
		resp, err := client.PostForm(base+toPath, form)
		if err != nil {
			t.Fatalf("POST %s: %v", toPath, err)
		}
		_ = resp.Body.Close()
		return resp
	}

	// Wait for the listener to answer.
	waitHealthy(t, client, base, serveErr)

	// #1 — the first visit lands on setup; creating the admin account and
	// logging in happens entirely in the browser flow.
	if code, _ := get("/"); code != http.StatusSeeOther {
		t.Fatalf("fresh GET / = %d, want redirect to /setup", code)
	}
	resp := post("/setup", "/setup", url.Values{
		"username": {"admin"}, "display_name": {"Meet Admin"},
		"password": {"korrekt-pferd-batterie"},
	})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("setup = %d, want 303", resp.StatusCode)
	}
	resp = post("/login", "/login", url.Values{
		"username": {"admin"}, "password": {"korrekt-pferd-batterie"},
	})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login = %d, want 303", resp.StatusCode)
	}

	// #2 — create the meet: two days, two sessions per day, C-Meeting.
	resp = post("/meets/new", "/meets", url.Values{
		"name": {"Abendmeeting Uster"}, "venue": {"Stadion Buchholz"},
		"homologation_ref": {"CH-ZH-042"},
		"start_date":       {"2027-06-12"}, "end_date": {"2027-06-13"},
		"tier": {"C-Meeting"}, "scheme": {"swiss-athletics"},
		"session_day_0": {"2027-06-12"}, "session_label_0": {"Session 1"},
		"session_day_1": {"2027-06-12"}, "session_label_1": {"Session 2"},
		"session_day_2": {"2027-06-13"}, "session_label_2": {"Session 1"},
		"session_day_3": {"2027-06-13"}, "session_label_3": {"Session 2"},
	})
	meetPath := resp.Header.Get("Location")
	if resp.StatusCode != http.StatusSeeOther || !strings.HasPrefix(meetPath, "/meets/") {
		t.Fatalf("create meet = %d -> %q", resp.StatusCode, meetPath)
	}
	code, page := get(meetPath)
	if code != http.StatusOK {
		t.Fatalf("meet page = %d", code)
	}
	for _, want := range []string{"Abendmeeting Uster", "C-Meeting", "Entwurf", "Session 2"} {
		if !strings.Contains(page, want) {
			t.Errorf("meet page missing %q (UC-001 #2)", want)
		}
	}

	// #3 — the three UC-001 events with rounds and entry deadlines.
	for _, ev := range []url.Values{
		{"discipline": {"100m"}, "categories": {"U16 W"}, "round_qualification": {"1"},
			"round_final": {"1"}, "entry_deadline": {"2027-06-01T23:59"}},
		{"discipline": {"SP"}, "categories": {"U16 W"}, "round_final": {"1"},
			"entry_deadline": {"2027-06-01T23:59"}},
		{"discipline": {"4x100m"}, "categories": {"U16 W"}, "round_final": {"1"},
			"entry_deadline": {"2027-06-01T23:59"}},
	} {
		resp = post(meetPath, meetPath+"/events", ev)
		if loc := resp.Header.Get("Location"); strings.Contains(loc, "err=") {
			t.Fatalf("add event %v failed: %q", ev, loc)
		}
	}
	_, page = get(meetPath)
	// The deadline was submitted as "2027-06-01T23:59" with no zone (parsed
	// as UTC); the programme renders it in the server's local zone
	// (SYS-110/F6, TASK-050's PageData.FormatDateTime), so the expected
	// wall clock is computed the same way rather than assuming it equals
	// the stored UTC value.
	wantDeadline := time.Date(2027, time.June, 1, 23, 59, 0, 0, time.UTC).Local().Format("02.01.2006 15:04")
	for _, want := range []string{"Lauf", "Weite", "Staffel", wantDeadline} {
		if !strings.Contains(page, want) {
			t.Errorf("programme missing %q (UC-001 #3: capture types + deadlines)", want)
		}
	}

	// #4 — schedule the 100m final unit, publish, amend, republish.
	unitPath, version := firstScheduleForm(t, page)
	resp = post(meetPath, unitPath, url.Values{
		"scheduled_at": {"2027-06-12T14:30"}, "location": {"Bahn"}, "version": {version},
	})
	if loc := resp.Header.Get("Location"); strings.Contains(loc, "err=") {
		t.Fatalf("schedule failed: %q", loc)
	}
	resp = post(meetPath, meetPath+"/timetable/publish", url.Values{})
	if loc := resp.Header.Get("Location"); strings.Contains(loc, "err=") {
		t.Fatalf("publish failed: %q", loc)
	}

	_, page = get(meetPath)
	unitPath, version = firstScheduleForm(t, page)
	resp = post(meetPath, unitPath, url.Values{
		"scheduled_at": {"2027-06-12T15:15"}, "location": {"Bahn"}, "version": {version},
	})
	if loc := resp.Header.Get("Location"); strings.Contains(loc, "err=") {
		t.Fatalf("amend failed: %q", loc)
	}
	resp = post(meetPath, meetPath+"/timetable/publish", url.Values{})
	if loc := resp.Header.Get("Location"); strings.Contains(loc, "err=") {
		t.Fatalf("republish failed: %q", loc)
	}

	meetID := strings.TrimPrefix(meetPath, "/meets/")
	code, pub := get("/m/" + meetID + "/timetable")
	if code != http.StatusOK {
		t.Fatalf("public timetable = %d", code)
	}
	// Both times were submitted with no zone (parsed as UTC); the public
	// timetable renders them in the server's local zone (SYS-110/F6,
	// TASK-050's FormatDateTime), so the expected wall clock is computed
	// the same way rather than assuming it equals the stored UTC value.
	amendedLocal := time.Date(2027, time.June, 12, 15, 15, 0, 0, time.UTC).Local().Format("15:04")
	preAmendLocal := time.Date(2027, time.June, 12, 14, 30, 0, 0, time.UTC).Local().Format("15:04")
	if !strings.Contains(pub, amendedLocal) || strings.Contains(pub, preAmendLocal) {
		t.Errorf("public timetable does not show the amended time (UC-001 #4)")
	}
	_, page = get(meetPath)
	if !strings.Contains(page, "Version 1") || !strings.Contains(page, "Version 2") {
		t.Errorf("meet page does not retain both published versions (SYS-004)")
	}

	// #5 — the sanctioning summary carries every SYS-006 field.
	code, sum := get(meetPath + "/sanctioning")
	if code != http.StatusOK {
		t.Fatalf("sanctioning summary = %d", code)
	}
	for _, want := range []string{
		"Abendmeeting Uster", "Stadion Buchholz", "CH-ZH-042",
		"12.06.2027", "C-Meeting", "admin", "U16 W",
	} {
		if !strings.Contains(sum, want) {
			t.Errorf("sanctioning summary missing %q (UC-001 #5)", want)
		}
	}
	if strings.Contains(sum, "Unvollständig") {
		t.Error("sanctioning summary flagged incomplete despite full data")
	}

	cancel()
	if err := <-serveErr; err != nil {
		t.Fatalf("server shut down with error: %v", err)
	}
}

func waitHealthy(t *testing.T, client *http.Client, base string, serveErr <-chan error) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-serveErr:
			t.Fatalf("server exited during startup: %v", err)
		default:
		}
		resp, err := client.Get(base + "/healthz")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("server did not become healthy in time")
}

var csrfRe = regexp.MustCompile(`name="csrf_token" value="([^"]+)"`)

func csrfToken(t *testing.T, page string) string {
	t.Helper()
	m := csrfRe.FindStringSubmatch(page)
	if m == nil {
		t.Fatalf("no csrf_token field found in page: %.300s", page)
	}
	return m[1]
}

// firstScheduleForm extracts the action path and current optimistic
// version of the first unit-schedule form on a meet page — the same
// information an operator's browser submits.
var scheduleFormRe = regexp.MustCompile(
	`(?s)action="(/meets/[^"]+/units/[^"]+/schedule)".*?name="version" value="(\d+)"`)

func firstScheduleForm(t *testing.T, page string) (string, string) {
	t.Helper()
	m := scheduleFormRe.FindStringSubmatch(page)
	if m == nil {
		t.Fatal("no unit-schedule form found on the meet page")
	}
	return m[1], m[2]
}
