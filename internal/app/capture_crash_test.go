// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kriegalex/bahnfrei/internal/domain"
	"github.com/kriegalex/bahnfrei/internal/store"
)

// Capture-mid-burst durability drill (UC-020 #2, SYS-081, SYS-130). The
// storage-layer kill-9 harness (internal/store's TestKill9Durability,
// TASK-003) already proves durability for a generic paired
// data-row/audit-row write; this test extends it to the actual UC-020 #2
// scenario — "a simulated power loss (VM kill) during a burst of result
// saves" — by driving the real ResultsService.SaveResult capture path
// (the same code the capture UI calls) against a real UKC meet with
// registered participants, instead of a synthetic table.
//
// A child process replays confirmed result saves in a tight loop, printing
// "confirmed <athleteID> <mark>" only after ResultsService.SaveResult
// returns (its internal Commit is what makes a write durable under
// synchronous=FULL). The parent SIGKILLs it mid-burst, reopens the store
// (gated on the startup consistency check, SYS-081/130), and verifies
// every confirmed (athlete, mark) pair — the *last* one recorded per
// athlete, since capture legitimately corrects earlier marks — is present
// exactly as confirmed.
const (
	captureCrashChildEnv = "BAHNFREI_CAPTURE_CRASH_CHILD"
	captureCrashDBEnv    = "BAHNFREI_CAPTURE_CRASH_DB"
	captureCrashMeetEnv  = "BAHNFREI_CAPTURE_CRASH_MEET"
	captureConfirmations = 30
	captureParticipants  = 40 // comfortably more than captureConfirmations
)

// buildCaptureCrashServices opens dbPath and wires a ResultsService (and
// MeetService, for setup) against it — the same wiring buildServer uses in
// production, minus the web layer.
func buildCaptureCrashServices(dbPath string) (*store.Store, *MeetService, *ResultsService, error) {
	st, err := store.Open(context.Background(), dbPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("open: %w", err)
	}
	catalog, err := domain.BuiltinDisciplineCatalog()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("catalog: %w", err)
	}
	schemes, err := domain.BuiltinCategorySchemes()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("schemes: %w", err)
	}
	tables, err := domain.BuiltinScoringTables()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("tables: %w", err)
	}
	templates, err := domain.BuiltinMeetTemplates()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("templates: %w", err)
	}
	meets := NewMeetService(st.DB(), catalog, schemes, tables, templates)
	results := NewResultsService(st.DB(), catalog, schemes, tables, templates)
	return st, meets, results, nil
}

// TestCaptureCrashChildProcess is the harness child, driven by
// TestCaptureCrashRecoverySYS081UC020_2 — never run directly (skipped
// unless the parent's env vars are set).
func TestCaptureCrashChildProcess(t *testing.T) {
	if os.Getenv(captureCrashChildEnv) != "1" {
		t.Skip("harness child — driven by TestCaptureCrashRecoverySYS081UC020_2")
	}
	ctx := context.Background()
	_, _, results, err := buildCaptureCrashServices(os.Getenv(captureCrashDBEnv))
	if err != nil {
		fmt.Printf("child-error setup: %v\n", err)
		os.Exit(2)
	}
	meetID := os.Getenv(captureCrashMeetEnv)
	participants, err := results.Participants(ctx, meetID)
	if err != nil || len(participants) == 0 {
		fmt.Printf("child-error participants: %v (n=%d)\n", err, len(participants))
		os.Exit(2)
	}

	for n := 0; ; n++ {
		p := participants[n%len(participants)]
		mark := fmt.Sprintf("8.%02d", n%100)
		if _, err := results.SaveResult(ctx, office, meetID, ResultInput{
			AthleteID: p.AthleteID, DisciplineCode: "60m", Mark: mark, Timing: domain.TimingElectronic,
		}); err != nil {
			fmt.Printf("child-error save: %v\n", err)
			os.Exit(2)
		}
		// Only printed after SaveResult's internal Commit returns — durable
		// under synchronous=FULL (SYS-081).
		fmt.Printf("confirmed %s %s\n", p.AthleteID, mark)
	}
}

// TestCaptureCrashRecoverySYS081UC020_2 covers UC-020 #2 at the capture
// use-case level: kill -9 during a burst of real result saves, restart,
// confirm no confirmed save was lost and the startup consistency check
// passed (SYS-130's "operational again within 2 minutes").
func TestCaptureCrashRecoverySYS081UC020_2(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "capture-crash.db")

	// Setup (meet + roster) happens outside the crash window: only the
	// result-save burst that follows is what this drill kills mid-flight,
	// matching UC-020 #2's "burst of result saves" scoping.
	st, meets, results, err := buildCaptureCrashServices(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	rec, err := meets.CreateMeetFromTemplate(ctx, organizer, TemplateMeetRequest{
		TemplateID: domain.TemplateUBSKidsCup, Venue: "Crash Drill", Date: ukcDay(),
	})
	if err != nil {
		t.Fatalf("CreateMeetFromTemplate: %v", err)
	}
	for i := 0; i < captureParticipants; i++ {
		if _, err := results.RegisterParticipant(ctx, office, rec.ID, ParticipantInput{
			FirstName: fmt.Sprintf("Athlete%02d", i), LastName: "Crash",
			BirthYear: 2015, Sex: domain.SexFemale, Bib: strconv.Itoa(200 + i),
		}); err != nil {
			t.Fatalf("RegisterParticipant #%d: %v", i, err)
		}
	}
	if err := st.Close(); err != nil {
		t.Fatal(err) // release the lock before the child (re)opens the file
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=TestCaptureCrashChildProcess$")
	cmd.Env = append(os.Environ(),
		captureCrashChildEnv+"=1", captureCrashDBEnv+"="+dbPath, captureCrashMeetEnv+"="+rec.ID)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	// Track the *last* confirmed mark per athlete: capture legitimately
	// re-saves (corrections) the same athlete's result, so the map, not a
	// flat list, is the correct ground truth to verify against.
	confirmedByAthlete := make(map[string]string, captureConfirmations)
	seen := 0
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "child-error") {
			t.Fatalf("harness child failed: %s", line)
		}
		rest, ok := strings.CutPrefix(line, "confirmed ")
		if !ok {
			continue
		}
		parts := strings.SplitN(rest, " ", 2)
		if len(parts) != 2 {
			t.Fatalf("malformed confirmation line: %q", line)
		}
		confirmedByAthlete[parts[0]] = parts[1]
		seen++
		if seen >= captureConfirmations {
			break // kill mid-burst, child is still writing
		}
	}
	if seen < captureConfirmations {
		t.Fatalf("child produced only %d confirmations (scanner err: %v)", seen, scanner.Err())
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait() // expected to report the kill

	// UC-020 #2 / SYS-130: reopen (WAL replay + the startup consistency
	// check) and be operational within budget.
	start := time.Now()
	st2, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("reopen after kill -9: %v", err)
	}
	t.Cleanup(func() { _ = st2.Close() })
	if elapsed := time.Since(start); elapsed > 2*time.Minute {
		t.Errorf("recovery took %v, budget is 2 minutes (SYS-130)", elapsed)
	}

	// Every confirmed save is present with exactly its last confirmed mark
	// — no acknowledged result lost, no stale/partial value (UC-020 #2).
	settled, err := store.ListMeetResults(ctx, st2.DB(), rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	byAthlete := make(map[string]string, len(settled))
	for _, r := range settled {
		byAthlete[r.AthleteID] = r.Mark
	}
	for athleteID, wantMark := range confirmedByAthlete {
		gotMark, ok := byAthlete[athleteID]
		if !ok {
			t.Errorf("confirmed result for athlete %s lost after kill -9 (SYS-081)", athleteID)
			continue
		}
		if gotMark != wantMark {
			t.Errorf("athlete %s mark = %q after recovery, want last confirmed %q", athleteID, gotMark, wantMark)
		}
	}

	// No partially applied records: every settled result save is paired
	// with at least one audit entry (SYS-046) — at least as many
	// result.save audit rows as distinct confirmed writes observed (more
	// is expected: the child kept writing after the parent stopped
	// scanning but before the SIGKILL landed).
	var auditRows int64
	if err := st2.DB().QueryRowContext(ctx,
		`SELECT count(*) FROM audit_log WHERE action = 'result.save'`).Scan(&auditRows); err != nil {
		t.Fatal(err)
	}
	if auditRows < int64(seen) {
		t.Errorf("result.save audit rows = %d, want >= %d confirmed writes observed", auditRows, seen)
	}
}
