// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import (
	"context"
	"errors"
	"testing"
)

// TestEnsureParticipantIdempotentSYS018 covers the meet-wide participant
// slot online entries rely on for later bib assignment (SYS-018): calling
// it twice for the same athlete does not error and returns the same row.
func TestEnsureParticipantIdempotentSYS018(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	meet := testMeet(t, s)
	athlete := entryFixtureAthlete(t, s, "Anna")

	first, err := EnsureParticipant(ctx, s.DB(), meet.ID, athlete.ID)
	if err != nil {
		t.Fatalf("EnsureParticipant (first): %v", err)
	}
	if first.Bib != "" {
		t.Fatalf("first participant bib = %q, want empty", first.Bib)
	}

	second, err := EnsureParticipant(ctx, s.DB(), meet.ID, athlete.ID)
	if err != nil {
		t.Fatalf("EnsureParticipant (second): %v", err)
	}
	if second.ID != first.ID {
		t.Errorf("EnsureParticipant is not idempotent: %q != %q", second.ID, first.ID)
	}
}

// TestUpdateParticipantBibSYS018UC006_1_2 covers bib assignment/edit under
// optimistic concurrency, and that a manually assigned duplicate bib is
// rejected (UC-006 #2).
func TestUpdateParticipantBibSYS018UC006_1_2(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	meet := testMeet(t, s)
	a1 := entryFixtureAthlete(t, s, "Anna")
	a2 := entryFixtureAthlete(t, s, "Beat")

	p1, err := EnsureParticipant(ctx, s.DB(), meet.ID, a1.ID)
	if err != nil {
		t.Fatalf("EnsureParticipant: %v", err)
	}
	p2, err := EnsureParticipant(ctx, s.DB(), meet.ID, a2.ID)
	if err != nil {
		t.Fatalf("EnsureParticipant: %v", err)
	}

	newVersion, err := UpdateParticipantBib(ctx, s.DB(), p1.ID, p1.Version, "101")
	if err != nil {
		t.Fatalf("UpdateParticipantBib: %v", err)
	}
	if newVersion != p1.Version+1 {
		t.Fatalf("new version = %d, want %d", newVersion, p1.Version+1)
	}

	// UC-006 #2: assigning the same bib to a second athlete is rejected.
	if _, err := UpdateParticipantBib(ctx, s.DB(), p2.ID, p2.Version, "101"); !errors.Is(err, ErrDuplicateParticipant) {
		t.Errorf("duplicate bib assignment = %v, want ErrDuplicateParticipant", err)
	}

	got, err := GetParticipantByAthlete(ctx, s.DB(), meet.ID, a1.ID)
	if err != nil {
		t.Fatalf("GetParticipantByAthlete: %v", err)
	}
	if got.Bib != "101" {
		t.Errorf("bib = %q, want 101", got.Bib)
	}

	if _, err := GetParticipantByAthlete(ctx, s.DB(), meet.ID, "no-such-athlete"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetParticipantByAthlete(unknown) = %v, want ErrNotFound", err)
	}
}
