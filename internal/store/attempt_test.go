// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import (
	"context"
	"errors"
	"testing"

	"github.com/kriegalex/bahnfrei/internal/domain"
)

func TestSaveAttemptInsertAndRecapture(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	_, unitID, athleteID := ukcFixture(t, s)

	w := 1.4
	first, err := SaveAttempt(ctx, s.DB(), unitID, athleteID,
		domain.Attempt{Seq: 1, Kind: domain.AttemptValid, Mark: "6.12", Wind: &w}, 0)
	if err != nil {
		t.Fatalf("SaveAttempt: %v", err)
	}
	if first.Version != 1 || first.Mark != "6.12" || *first.Wind != 1.4 {
		t.Errorf("first save = %+v", first)
	}

	// Re-capture (a correction) under the read version succeeds and bumps.
	second, err := SaveAttempt(ctx, s.DB(), unitID, athleteID,
		domain.Attempt{Seq: 1, Kind: domain.AttemptFoul}, first.Version)
	if err != nil {
		t.Fatalf("SaveAttempt(correction): %v", err)
	}
	if second.Version != 2 || second.Kind != domain.AttemptFoul || second.Mark != "" {
		t.Errorf("correction = %+v", second)
	}
	if second.ID != first.ID {
		t.Errorf("correction created a new row (%s → %s)", first.ID, second.ID)
	}
}

// TestSaveAttemptConflicts is UC-021 #2 at the storage layer: a stale write
// — whether a double insert or an outdated version — surfaces the conflict
// together with the currently stored attempt, never silent last-write-wins.
func TestSaveAttemptConflicts(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	_, unitID, athleteID := ukcFixture(t, s)

	stored, err := SaveAttempt(ctx, s.DB(), unitID, athleteID,
		domain.Attempt{Seq: 1, Kind: domain.AttemptValid, Mark: "6.12"}, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Insert of a trial another device already captured.
	current, err := SaveAttempt(ctx, s.DB(), unitID, athleteID,
		domain.Attempt{Seq: 1, Kind: domain.AttemptValid, Mark: "5.90"}, 0)
	if !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("double insert = %v, want ErrVersionConflict", err)
	}
	if current.Mark != "6.12" || current.Version != stored.Version {
		t.Errorf("conflict must carry the stored attempt, got %+v", current)
	}

	// Update against a stale version.
	if _, err := SaveAttempt(ctx, s.DB(), unitID, athleteID,
		domain.Attempt{Seq: 1, Kind: domain.AttemptPass}, stored.Version+7); !errors.Is(err, ErrVersionConflict) {
		t.Errorf("stale update = %v, want ErrVersionConflict", err)
	}
	// The stored mark is untouched by either conflicting write.
	got, err := GetAttempt(ctx, s.DB(), unitID, athleteID, 1)
	if err != nil || got.Mark != "6.12" {
		t.Errorf("stored attempt after conflicts = %+v, %v", got, err)
	}

	// Update of a trial that does not exist.
	if _, err := SaveAttempt(ctx, s.DB(), unitID, athleteID,
		domain.Attempt{Seq: 9, Kind: domain.AttemptPass}, 3); !errors.Is(err, ErrNotFound) {
		t.Errorf("update of missing trial = %v, want ErrNotFound", err)
	}
	if _, err := SaveAttempt(ctx, s.DB(), "", athleteID, domain.Attempt{Seq: 1}, 0); err == nil {
		t.Error("SaveAttempt without unit: want error")
	}
}

func TestListUnitAttemptsOrdering(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	_, unitID, athleteID := ukcFixture(t, s)
	other, err := CreateAthlete(ctx, s.DB(), domain.Athlete{
		LastName: "Beispiel", BirthYear: 2013, Sex: domain.SexMale,
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, in := range []struct {
		athlete string
		a       domain.Attempt
	}{
		{athleteID, domain.Attempt{Seq: 2, Kind: domain.AttemptFoul}},
		{athleteID, domain.Attempt{Seq: 1, Kind: domain.AttemptValid, Mark: "6.12"}},
		{other.ID, domain.Attempt{Seq: 1, Kind: domain.AttemptPass}},
	} {
		if _, err := SaveAttempt(ctx, s.DB(), unitID, in.athlete, in.a, 0); err != nil {
			t.Fatalf("SaveAttempt(%+v): %v", in.a, err)
		}
	}

	got, err := ListUnitAttempts(ctx, s.DB(), unitID)
	if err != nil {
		t.Fatalf("ListUnitAttempts: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d attempts, want 3", len(got))
	}
	for i := 1; i < len(got); i++ {
		prev, cur := got[i-1], got[i]
		if prev.AthleteID > cur.AthleteID ||
			(prev.AthleteID == cur.AthleteID && prev.Seq > cur.Seq) {
			t.Errorf("attempts not (athlete, seq)-ordered: %+v before %+v", prev, cur)
		}
	}
}

func TestResultStatusDetailRoundTrip(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	_, unitID, athleteID := ukcFixture(t, s)

	rec, err := SaveResult(ctx, s.DB(), domain.Result{
		UnitID: unitID, AthleteID: athleteID,
		Status: domain.StatusDQ, StatusDetail: "TR16.8",
	}, domain.TimingNone)
	if err != nil {
		t.Fatalf("SaveResult: %v", err)
	}
	if rec.StatusDetail != "TR16.8" {
		t.Errorf("status detail = %q, want TR16.8 (SYS-045)", rec.StatusDetail)
	}
	unitResults, err := ListUnitResults(ctx, s.DB(), unitID)
	if err != nil || len(unitResults) != 1 || unitResults[0].StatusDetail != "TR16.8" {
		t.Errorf("ListUnitResults = %+v, %v", unitResults, err)
	}
}
