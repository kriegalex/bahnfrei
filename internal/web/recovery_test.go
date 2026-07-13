// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

//go:build perf

package web

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kriegalex/bahnfrei/internal/apptest"
	"github.com/kriegalex/bahnfrei/internal/web/i18n"
)

// TASK-027 SYS-130 process-level recovery drill: "After an application or
// host failure during a meet, the system SHALL be operational again within
// 2 minutes of host availability (automatic restart/recovery)".
//
// Prior art already proves the STORAGE layer survives a kill-9
// (internal/store's TestKill9Durability, TASK-003) and that the real
// ResultsService.SaveResult capture path survives one too
// (internal/app's TestCaptureCrashRecoverySYS081UC020_2, TASK-014). Both
// stop at "the database reopens cleanly" — neither proves the actual HTTP
// server (the thing SYS-130 says must be "operational") comes back up and
// serves requests again. This drill closes that gap: it runs the real
// web.Server (routes, sessions, RBAC, SSE bus — everything but TLS, which
// is orthogonal to recovery) as a genuine, killable OS process, drives a
// burst of real result saves over HTTP (not a direct Go call) exactly like
// an office operator's browser would, SIGKILLs it mid-burst, restarts a
// fresh server against the SAME on-disk database, and asserts (a) the HTTP
// surface answers within the 2-minute budget and (b) every save the office
// received a 303 for is present with its saved mark — zero acknowledged-
// data loss, at the HTTP layer this time.
//
// Guarded behind the `perf` build tag (like TASK-027's other heavy
// drills/benchmarks) so `go test ./...` stays fast; run via
// scripts/run-perf-tests.sh. See docs/delivery/perf-and-recovery-task-027.md
// for the as-measured results.

const (
	recoveryChildEnv     = "BAHNFREI_RECOVERY_CHILD"
	recoveryDBEnv        = "BAHNFREI_RECOVERY_DB"
	recoveryConfirmCount = 25
)

// buildRecoveryServer wires a real *Server against a fixed-path store
// (surviving across the process restart this drill performs) — the same
// wiring newTestServer uses, minus the httptest.Server (this drill needs a
// listener it controls directly, so a subprocess can be killed out from
// under it).
func buildRecoveryServer(tb testing.TB, dbPath string) *Server {
	tb.Helper()
	fix := apptest.NewAtPath(tb, time.Hour, dbPath)
	cats, err := i18n.Load()
	if err != nil {
		tb.Fatalf("i18n.Load: %v", err)
	}
	bus := NewBus()
	cfg := Config{Addr: "127.0.0.1:0", TLS: TLSConfig{Mode: TLSModeOff}, AppVersion: "recovery-drill"}
	return New(cfg, fix.Auth, fix.Sessions, fix.Meets, fix.Results, fix.Backup, cats, bus).SetPrivacy(fix.Privacy)
}

// TestServeRecoveryChildProcess is the harness child, driven (and
// SIGKILLed) by TestServeRecoverySYS130 — skipped unless its env var is
// set. It starts a real HTTP server on an OS-chosen port, prints
// "listening <addr>" as its first line so the parent can reach it, then
// bootstraps an admin account, creates a UKC template meet, registers
// enough athletes for a confirmations burst, and loops real HTTP capture
// saves — printing "confirmed <athleteID> <mark>" only after each POST
// returns its success redirect (i.e., after the write is durable:
// synchronous=FULL under WAL means a returned response is a completed
// commit).
func TestServeRecoveryChildProcess(t *testing.T) {
	if os.Getenv(recoveryChildEnv) != "1" {
		t.Skip("harness child — driven by TestServeRecoverySYS130")
	}
	dbPath := os.Getenv(recoveryDBEnv)
	srv := buildRecoveryServer(t, dbPath)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Printf("child-error listen: %v\n", err)
		os.Exit(2)
	}
	fmt.Printf("listening %s\n", ln.Addr().String())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.Serve(ctx, ln) }()

	base := "http://" + ln.Addr().String()
	jar, err := cookiejar.New(nil)
	if err != nil {
		fmt.Printf("child-error cookiejar: %v\n", err)
		os.Exit(2)
	}
	client := &http.Client{Jar: jar, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	// Poll until the listener is actually accepting HTTP (Serve's goroutine
	// needs a moment to start).
	if !waitHealthy(client, base, 5*time.Second) {
		fmt.Printf("child-error server never became healthy\n")
		os.Exit(2)
	}

	setupAndLogin(t, client, base)
	meetID, units := ukcCaptureFixture(t, client, base)
	unitID, ok := units["60 metres"]
	if !ok {
		fmt.Printf("child-error: no 60 metres capture unit\n")
		os.Exit(2)
	}
	unitURL := base + "/meets/" + meetID + "/capture/" + unitID

	// ukcCaptureFixture registers 2 athletes (bibs 101/102); register more
	// so the burst has enough distinct athletes for recoveryConfirmCount
	// confirmations without repeatedly re-saving (and thus versioning) the
	// same two rows.
	const extraAthletes = 40
	for i := 0; i < extraAthletes; i++ {
		bib := fmt.Sprintf("%d", 200+i)
		resp := postForm(t, client, base+"/meets/"+meetID+"/roster", base+"/meets/"+meetID+"/roster", url.Values{
			"first_name": {fmt.Sprintf("Recovery%02d", i)}, "last_name": {"Drill"},
			"birth_year": {"2014"}, "sex": {"W"}, "bib": {bib},
		})
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusSeeOther {
			fmt.Printf("child-error: register athlete %d = %d\n", i, resp.StatusCode)
			os.Exit(2)
		}
	}

	body := bodyString(t, mustGet(t, client, unitURL))
	athletes := athleteIDsFrom(t, body)
	ids := make([]string, 0, len(athletes))
	for _, id := range athletes {
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		fmt.Printf("child-error: no athlete IDs on capture page\n")
		os.Exit(2)
	}

	for n := 0; ; n++ {
		athleteID := ids[n%len(ids)]
		// Electronic (FAT) timing keeps the submitted 2-decimal mark
		// exactly as entered — manual/hand timing would round it UP to
		// the next 0.1s (SYS-041's RoundUpHandTime), which would make the
		// printed "confirmed" line not match what is actually stored and
		// break the zero-data-loss comparison below for no good reason.
		mark := fmt.Sprintf("9.%02d", n%100)
		resp := postForm(t, client, unitURL, unitURL+"/track", url.Values{
			"athlete": {athleteID}, "time": {mark}, "timing": {"electronic"},
		})
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusSeeOther {
			fmt.Printf("child-error: capture save %d = %d\n", n, resp.StatusCode)
			os.Exit(2)
		}
		// Only printed after the POST's response is read — durable
		// (synchronous=FULL) and acknowledged over HTTP, matching what an
		// office operator's browser would have seen as "saved".
		fmt.Printf("confirmed %s %s\n", athleteID, mark)
	}
}

// waitHealthy polls base+"/healthz" until it answers 200 or timeout
// elapses.
func waitHealthy(client *http.Client, base string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := client.Get(base + "/healthz")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return true
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

// TestServeRecoverySYS130 is the parent: starts the child above as a real
// OS process, SIGKILLs it mid-burst, restarts a fresh server against the
// same on-disk database, and asserts the SYS-130 budget plus zero
// acknowledged-data loss.
func TestServeRecoverySYS130(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "recovery.db")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=TestServeRecoveryChildProcess$")
	cmd.Env = append(os.Environ(), recoveryChildEnv+"=1", recoveryDBEnv+"="+dbPath)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	var addr string
	confirmed := make(map[string]string, recoveryConfirmCount) // athleteID -> last confirmed mark
	seen := 0
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "child-error") {
			t.Fatalf("harness child failed: %s", line)
		}
		if a, ok := strings.CutPrefix(line, "listening "); ok {
			addr = a
			continue
		}
		rest, ok := strings.CutPrefix(line, "confirmed ")
		if !ok {
			continue
		}
		parts := strings.SplitN(rest, " ", 2)
		if len(parts) != 2 {
			t.Fatalf("malformed confirmation line: %q", line)
		}
		confirmed[parts[0]] = parts[1]
		seen++
		if seen >= recoveryConfirmCount {
			break // kill mid-burst, child is still writing
		}
	}
	if addr == "" {
		t.Fatal("child never reported its listen address")
	}
	if seen < recoveryConfirmCount {
		t.Fatalf("child produced only %d confirmations (scanner err: %v)", seen, scanner.Err())
	}

	if err := cmd.Process.Kill(); err != nil { // SIGKILL / TerminateProcess
		t.Fatal(err)
	}
	_ = cmd.Wait() // expected to report the kill

	// SYS-130: restart a fresh server against the SAME on-disk database and
	// measure time-to-operational from right after the kill.
	start := time.Now()
	srv2 := buildRecoveryServer(t, dbPath)
	ln2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	go func() { _ = srv2.Serve(ctx2, ln2) }()

	base2 := "http://" + ln2.Addr().String()
	client2 := &http.Client{Jar: mustJar(t), CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	operational := waitHealthy(client2, base2, 2*time.Minute)
	elapsed := time.Since(start)
	t.Logf("SYS-130 recovery: operational=%v after %v (budget 2m)", operational, elapsed)
	if !operational {
		t.Fatalf("server did not become operational within 2 minutes (SYS-130)")
	}
	if elapsed > 2*time.Minute {
		t.Errorf("recovery took %v, want <= 2 minutes (SYS-130)", elapsed)
	}

	// Zero acknowledged-data loss: log back in against the reopened store
	// (the admin account the child already bootstrapped is on disk, so
	// GET /setup here would just redirect to /login with no CSRF-bearing
	// form to scrape — unlike a fresh instance, so setupAndLogin does not
	// apply; log in directly instead) and confirm every acknowledged save
	// survived with its exact confirmed mark.
	loginResp := postForm(t, client2, base2+"/login", base2+"/login", url.Values{
		"username": {"admin"}, "password": {"s3cret-passphrase"},
	})
	_ = loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login after recovery = %d, want 303", loginResp.StatusCode)
	}

	fix2 := apptest.NewAtPath(t, time.Hour, dbPath)
	// Re-discover the meet/unit the child created: only one meet exists in
	// this fixture, so the office roster/meets list has exactly one entry.
	meets, err := fix2.Meets.ListMeets(context.Background())
	if err != nil || len(meets) != 1 {
		t.Fatalf("ListMeets after recovery: %v (n=%d)", err, len(meets))
	}
	meetID := meets[0].ID
	md, err := fix2.Meets.Meet(context.Background(), meetID)
	if err != nil {
		t.Fatalf("Meet after recovery: %v", err)
	}
	var unitID string
	for _, u := range md.Units {
		if u.DisciplineCode == "60m" {
			unitID = u.UnitID
		}
	}
	if unitID == "" {
		t.Fatal("60m unit not found after recovery")
	}
	view, err := fix2.Results.UnitCapture(context.Background(), meetID, unitID)
	if err != nil {
		t.Fatalf("UnitCapture after recovery: %v", err)
	}
	byAthlete := make(map[string]string, len(view.Rows))
	for _, row := range view.Rows {
		if row.Result != nil {
			byAthlete[row.AthleteID] = row.Result.Mark
		}
	}
	for athleteID, wantMark := range confirmed {
		gotMark, ok := byAthlete[athleteID]
		if !ok {
			t.Errorf("confirmed save for athlete %s lost after kill -9 (SYS-130)", athleteID)
			continue
		}
		if gotMark != wantMark {
			t.Errorf("athlete %s mark = %q after recovery, want confirmed %q", athleteID, gotMark, wantMark)
		}
	}
}

func mustJar(t *testing.T) *cookiejar.Jar {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return jar
}
