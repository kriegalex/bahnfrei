// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import (
	"context"
	"errors"
	"testing"

	"github.com/kriegalex/bahnfrei/internal/domain"
)

func TestVerticalUnitConfigRoundTripSYS043(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	_, unitID, _ := ukcFixture(t, s)

	if _, err := GetVerticalUnitConfig(ctx, s.DB(), unitID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetVerticalUnitConfig before configuration = %v, want ErrNotFound", err)
	}

	cfg, err := SetVerticalUnitConfig(ctx, s.DB(), unitID, []string{"1.60", "1.65", "1.70"}, 0)
	if err != nil {
		t.Fatalf("SetVerticalUnitConfig: %v", err)
	}
	if cfg.Version != 1 || len(cfg.Heights) != 3 {
		t.Errorf("initial config = %+v", cfg)
	}

	got, err := GetVerticalUnitConfig(ctx, s.DB(), unitID)
	if err != nil {
		t.Fatalf("GetVerticalUnitConfig: %v", err)
	}
	if len(got.Heights) != 3 || got.Heights[2] != "1.70" {
		t.Errorf("stored heights = %v", got.Heights)
	}

	// Appending a jump-off height (SYS-043: the office extends the same
	// progression rather than a separate mechanism).
	updated, err := SetVerticalUnitConfig(ctx, s.DB(), unitID, []string{"1.60", "1.65", "1.70", "1.72"}, got.Version)
	if err != nil {
		t.Fatalf("SetVerticalUnitConfig (append jump-off): %v", err)
	}
	if updated.Version != 2 || len(updated.Heights) != 4 {
		t.Errorf("updated config = %+v", updated)
	}

	// A stale version is rejected.
	if _, err := SetVerticalUnitConfig(ctx, s.DB(), unitID, []string{"1.60"}, 1); !errors.Is(err, ErrVersionConflict) {
		t.Errorf("stale update = %v, want ErrVersionConflict", err)
	}

	if _, err := SetVerticalUnitConfig(ctx, s.DB(), unitID, nil, 0); err == nil {
		t.Error("SetVerticalUnitConfig with no heights: want error")
	}
}

func TestSaveVerticalTrialInsertAndRecaptureSYS043(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	_, unitID, athleteID := ukcFixture(t, s)

	first, err := SaveVerticalTrial(ctx, s.DB(), unitID, athleteID,
		domain.VerticalTrial{HeightIdx: 0, Seq: 1, Kind: domain.StatusX}, 0)
	if err != nil {
		t.Fatalf("SaveVerticalTrial: %v", err)
	}
	if first.Version != 1 || first.Kind != domain.StatusX {
		t.Errorf("first save = %+v", first)
	}

	second, err := SaveVerticalTrial(ctx, s.DB(), unitID, athleteID,
		domain.VerticalTrial{HeightIdx: 0, Seq: 1, Kind: domain.StatusO}, first.Version)
	if err != nil {
		t.Fatalf("SaveVerticalTrial(correction): %v", err)
	}
	if second.Version != 2 || second.Kind != domain.StatusO || second.ID != first.ID {
		t.Errorf("correction = %+v", second)
	}
}

// TestSaveVerticalTrialConflictsSYS043 mirrors TestSaveAttemptConflicts: a
// stale write surfaces the conflict with the currently stored trial, never
// silent last-write-wins (UC-021 #2, SYS-086).
func TestSaveVerticalTrialConflictsSYS043(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	_, unitID, athleteID := ukcFixture(t, s)

	stored, err := SaveVerticalTrial(ctx, s.DB(), unitID, athleteID,
		domain.VerticalTrial{HeightIdx: 1, Seq: 1, Kind: domain.StatusO}, 0)
	if err != nil {
		t.Fatal(err)
	}

	current, err := SaveVerticalTrial(ctx, s.DB(), unitID, athleteID,
		domain.VerticalTrial{HeightIdx: 1, Seq: 1, Kind: domain.StatusX}, 0)
	if !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("double insert = %v, want ErrVersionConflict", err)
	}
	if current.Kind != domain.StatusO || current.Version != stored.Version {
		t.Errorf("conflict must carry the stored trial, got %+v", current)
	}

	if _, err := SaveVerticalTrial(ctx, s.DB(), unitID, athleteID,
		domain.VerticalTrial{HeightIdx: 1, Seq: 1, Kind: domain.StatusPass}, stored.Version+7); !errors.Is(err, ErrVersionConflict) {
		t.Errorf("stale update = %v, want ErrVersionConflict", err)
	}

	if _, err := SaveVerticalTrial(ctx, s.DB(), "", athleteID, domain.VerticalTrial{HeightIdx: 0, Seq: 1, Kind: domain.StatusO}, 0); err == nil {
		t.Error("SaveVerticalTrial without unit: want error")
	}
}

func TestListUnitVerticalTrialsOrderingSYS043(t *testing.T) {
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
		trial   domain.VerticalTrial
	}{
		{athleteID, domain.VerticalTrial{HeightIdx: 1, Seq: 1, Kind: domain.StatusO}},
		{athleteID, domain.VerticalTrial{HeightIdx: 0, Seq: 1, Kind: domain.StatusO}},
		{other.ID, domain.VerticalTrial{HeightIdx: 0, Seq: 1, Kind: domain.StatusX}},
	} {
		if _, err := SaveVerticalTrial(ctx, s.DB(), unitID, in.athlete, in.trial, 0); err != nil {
			t.Fatalf("SaveVerticalTrial(%+v): %v", in.trial, err)
		}
	}

	got, err := ListUnitVerticalTrials(ctx, s.DB(), unitID)
	if err != nil {
		t.Fatalf("ListUnitVerticalTrials: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d trials, want 3", len(got))
	}
	for i := 1; i < len(got); i++ {
		prev, cur := got[i-1], got[i]
		if prev.AthleteID > cur.AthleteID ||
			(prev.AthleteID == cur.AthleteID && prev.HeightIdx > cur.HeightIdx) {
			t.Errorf("trials not (athlete, height)-ordered: %+v before %+v", prev, cur)
		}
	}
}
