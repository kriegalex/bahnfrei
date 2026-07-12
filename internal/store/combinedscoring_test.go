// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import (
	"context"
	"testing"
)

func TestMeetCombinedScoringTableRoundTripSYS044(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	meet := testMeet(t, s)

	if _, ok, err := GetMeetCombinedScoringTable(ctx, s.DB(), meet.ID); err != nil || ok {
		t.Fatalf("GetMeetCombinedScoringTable before configuration = (%v, %v, %v), want (\"\", false, nil)", "", ok, err)
	}

	if err := SetMeetCombinedScoringTable(ctx, s.DB(), meet.ID, "wa-combined-events-2001"); err != nil {
		t.Fatalf("SetMeetCombinedScoringTable: %v", err)
	}
	id, ok, err := GetMeetCombinedScoringTable(ctx, s.DB(), meet.ID)
	if err != nil {
		t.Fatalf("GetMeetCombinedScoringTable: %v", err)
	}
	if !ok || id != "wa-combined-events-2001" {
		t.Errorf("got (%q, %v), want (\"wa-combined-events-2001\", true)", id, ok)
	}

	// Overwriting replaces the prior choice (a meet has exactly one
	// combined-scoring table at a time).
	if err := SetMeetCombinedScoringTable(ctx, s.DB(), meet.ID, "wa-combined-events-2005"); err != nil {
		t.Fatalf("SetMeetCombinedScoringTable (overwrite): %v", err)
	}
	id, ok, err = GetMeetCombinedScoringTable(ctx, s.DB(), meet.ID)
	if err != nil || !ok || id != "wa-combined-events-2005" {
		t.Errorf("got (%q, %v, %v), want (\"wa-combined-events-2005\", true, nil)", id, ok, err)
	}

	if err := SetMeetCombinedScoringTable(ctx, s.DB(), "", "x"); err == nil {
		t.Error("SetMeetCombinedScoringTable without meet id: want error")
	}
	if err := SetMeetCombinedScoringTable(ctx, s.DB(), meet.ID, ""); err == nil {
		t.Error("SetMeetCombinedScoringTable without table id: want error")
	}
}
