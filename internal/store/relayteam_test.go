// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import (
	"context"
	"errors"
	"testing"

	"github.com/kriegalex/bahnfrei/internal/domain"
)

// TestRelayTeamCreateAndUpdateCompositionSYS012 covers UC-003 #4: the team
// entry stores the ordered composition, and revising it (until the deadline
// — enforced by the app layer) replaces the composition/reserves under
// optimistic concurrency.
func TestRelayTeamCreateAndUpdateCompositionSYS012(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	club, err := CreateClub(ctx, s.DB(), domain.Club{Name: "LC Test"})
	if err != nil {
		t.Fatalf("CreateClub: %v", err)
	}

	rec, err := CreateRelayTeam(ctx, s.DB(), domain.RelayTeam{
		ClubID: club.ID, Composition: []string{"a1", "a2", "a3", "a4"}, Reserves: []string{"a5", "a6"},
	})
	if err != nil {
		t.Fatalf("CreateRelayTeam: %v", err)
	}

	got, err := GetRelayTeam(ctx, s.DB(), rec.ID)
	if err != nil {
		t.Fatalf("GetRelayTeam: %v", err)
	}
	wantComp := []string{"a1", "a2", "a3", "a4"}
	for i, id := range wantComp {
		if got.Composition[i] != id {
			t.Fatalf("composition = %v, want ordered %v", got.Composition, wantComp)
		}
	}
	if len(got.Reserves) != 2 {
		t.Fatalf("reserves = %v, want 2", got.Reserves)
	}

	newVersion, err := UpdateRelayTeamComposition(ctx, s.DB(), rec.ID, rec.Version, []string{"a4", "a3", "a2", "a1"}, nil)
	if err != nil {
		t.Fatalf("UpdateRelayTeamComposition: %v", err)
	}
	if newVersion != rec.Version+1 {
		t.Fatalf("new version = %d, want %d", newVersion, rec.Version+1)
	}
	got, err = GetRelayTeam(ctx, s.DB(), rec.ID)
	if err != nil {
		t.Fatalf("GetRelayTeam: %v", err)
	}
	if got.Composition[0] != "a4" || len(got.Reserves) != 0 {
		t.Errorf("revised team = %+v", got)
	}

	if _, err := UpdateRelayTeamComposition(ctx, s.DB(), rec.ID, rec.Version /* stale */, []string{"x"}, nil); !errors.Is(err, ErrVersionConflict) {
		t.Errorf("stale update = %v, want ErrVersionConflict", err)
	}

	if _, err := GetRelayTeam(ctx, s.DB(), "no-such-id"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetRelayTeam(unknown) = %v, want ErrNotFound", err)
	}
}
