// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package apptest

import (
	"context"
	"testing"
	"time"

	"github.com/kriegalex/bahnfrei/internal/domain"
)

// TestSeedLargeMeetSmallScale exercises the TASK-027 bulk-fixture builder
// in the DEFAULT gate at a deliberately small (but batching-exercising)
// scale, so the builder itself stays compile- and behaviour-checked on
// every `go test ./...` run — the full SYS-120 reference scale
// (1,500/4,000/250) runs only under the `perf` tag
// (internal/app/perf_bench_test.go, internal/web/load_test.go). Entries
// (450) deliberately exceed scaleBatch (400) so the mid-loop
// commit-and-reopen path runs here too.
func TestSeedLargeMeetSmallScale(t *testing.T) {
	f := New(t, time.Hour)
	p := ScaleParams{Athletes: 120, Entries: 450, Units: 12, Days: 2}
	lf := SeedLargeMeet(t, f, p)

	if lf.MeetID == "" {
		t.Fatal("no meet ID")
	}
	if got := len(lf.EventIDs); got != p.Units {
		t.Errorf("events = %d, want %d", got, p.Units)
	}
	if got := len(lf.UnitIDs); got != p.Units {
		t.Errorf("units = %d, want %d", got, p.Units)
	}
	if got := len(lf.DisciplineCodes); got != p.Units {
		t.Errorf("discipline codes = %d, want %d", got, p.Units)
	}
	if got := len(lf.AthleteIDs); got != p.Athletes {
		t.Errorf("athletes = %d, want %d", got, p.Athletes)
	}

	ctx := context.Background()

	// The meet is published (the public surfaces must render it) and
	// carries the requested day span.
	md, err := f.Meets.Meet(ctx, lf.MeetID)
	if err != nil {
		t.Fatalf("Meet: %v", err)
	}
	if md.Status != domain.MeetPublished {
		t.Errorf("meet status = %q, want %q", md.Status, domain.MeetPublished)
	}
	if got := len(md.Programme); got != p.Units {
		t.Errorf("programme events = %d, want %d", got, p.Units)
	}
	if days := int(md.EndDate.Sub(md.StartDate).Hours()/24) + 1; days != p.Days {
		t.Errorf("meet spans %d days, want %d", days, p.Days)
	}

	// Every athlete is a participant of the meet.
	participants, err := f.Results.Participants(ctx, lf.MeetID)
	if err != nil {
		t.Fatalf("Participants: %v", err)
	}
	if got := len(participants); got != p.Athletes {
		t.Errorf("participants = %d, want %d", got, p.Athletes)
	}

	// Standings (the SYS-121 recompute target) rank a non-empty corpus:
	// ~2/3 of entries carry settled results by construction.
	st, err := f.Results.Standings(ctx, lf.MeetID)
	if err != nil {
		t.Fatalf("Standings: %v", err)
	}
	if len(st.Divisions) == 0 {
		t.Error("standings have no divisions — seeded results missing")
	}
	rows := 0
	for _, d := range st.Divisions {
		rows += len(d.Rows)
	}
	if rows != p.Athletes {
		t.Errorf("standings rows = %d, want %d (every participant ranked)", rows, p.Athletes)
	}
}
