// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import (
	"context"
	"testing"

	"github.com/kriegalex/bahnfrei/internal/domain"
)

// TestUnitWindRoundTripSYS040 covers the per-race wind reading: unset until
// SetUnitWind runs, then applied uniformly to every settled result of the
// unit (UC-010 #4).
func TestUnitWindRoundTripSYS040(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	_, unitID, athleteID := ukcFixture(t, s)

	if wind, err := GetUnitWind(ctx, s.DB(), unitID); err != nil || wind != nil {
		t.Fatalf("GetUnitWind before set = %v, %v, want nil, nil", wind, err)
	}

	if _, err := SaveResult(ctx, s.DB(), domain.Result{UnitID: unitID, AthleteID: athleteID, Mark: "9.32"}, domain.TimingElectronic); err != nil {
		t.Fatalf("SaveResult: %v", err)
	}

	if err := SetUnitWind(ctx, s.DB(), unitID, 1.4); err != nil {
		t.Fatalf("SetUnitWind: %v", err)
	}
	wind, err := GetUnitWind(ctx, s.DB(), unitID)
	if err != nil || wind == nil || *wind != 1.4 {
		t.Fatalf("GetUnitWind after set = %v, %v, want 1.4", wind, err)
	}

	w := 1.4
	if err := UpdateResultsWind(ctx, s.DB(), unitID, &w); err != nil {
		t.Fatalf("UpdateResultsWind: %v", err)
	}
	rec, err := GetResult(ctx, s.DB(), unitID, athleteID)
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if rec.Wind == nil || *rec.Wind != 1.4 {
		t.Errorf("result wind = %v, want 1.4 (applied uniformly, UC-010 #4)", rec.Wind)
	}
	if rec.Version != 2 {
		t.Errorf("version = %d, want 2 (wind update bumps version)", rec.Version)
	}

	// Re-setting overwrites, not duplicates (unit_id primary key).
	if err := SetUnitWind(ctx, s.DB(), unitID, 2.3); err != nil {
		t.Fatalf("SetUnitWind (overwrite): %v", err)
	}
	wind, err = GetUnitWind(ctx, s.DB(), unitID)
	if err != nil || wind == nil || *wind != 2.3 {
		t.Fatalf("GetUnitWind after overwrite = %v, %v, want 2.3", wind, err)
	}
}

// TestAnnounceUnitSequenceAndLatestSYS047 covers the SYS-047 announcement
// history: sequential, monotonic, and LatestAnnouncement always reports the
// most recent one — the protest-clock's read side.
func TestAnnounceUnitSequenceAndLatestSYS047(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	_, unitID, _ := ukcFixture(t, s)

	if _, found, err := LatestAnnouncement(ctx, s.DB(), unitID); err != nil || found {
		t.Fatalf("LatestAnnouncement before any = %v, %v, want not found", found, err)
	}

	seq1, at1, err := AnnounceUnit(ctx, s.DB(), unitID, "01OFF")
	if err != nil {
		t.Fatalf("AnnounceUnit #1: %v", err)
	}
	if seq1 != 1 {
		t.Errorf("first announcement seq = %d, want 1", seq1)
	}

	seq2, at2, err := AnnounceUnit(ctx, s.DB(), unitID, "01OFF")
	if err != nil {
		t.Fatalf("AnnounceUnit #2: %v", err)
	}
	if seq2 != 2 {
		t.Errorf("second announcement seq = %d, want 2 (a correction re-announces, UC-015 #2)", seq2)
	}
	if !at2.After(at1) && !at2.Equal(at1) {
		t.Errorf("second announcement at %v must not precede the first %v", at2, at1)
	}

	latest, found, err := LatestAnnouncement(ctx, s.DB(), unitID)
	if err != nil || !found {
		t.Fatalf("LatestAnnouncement after 2 = %v, %v", found, err)
	}
	if !latest.Equal(at2) {
		t.Errorf("latest announcement = %v, want the second one %v", latest, at2)
	}

	// A different unit's announcements never mix in.
	meet := testMeet(t, s)
	ev, err := CreateEvent(ctx, s.DB(), domain.Event{MeetID: meet.ID, DisciplineCode: "60m", CategoryCodes: []string{"W12"}})
	if err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}
	round, err := CreateRound(ctx, s.DB(), domain.Round{EventID: ev.ID, Kind: domain.RoundFinal}, 0)
	if err != nil {
		t.Fatalf("CreateRound: %v", err)
	}
	other, err := CreateUnit(ctx, s.DB(), domain.Unit{RoundID: round.ID})
	if err != nil {
		t.Fatalf("CreateUnit: %v", err)
	}
	otherUnit := other.ID
	if _, found, err := LatestAnnouncement(ctx, s.DB(), otherUnit); err != nil || found {
		t.Fatalf("LatestAnnouncement(otherUnit) = %v, %v, want not found", found, err)
	}
}
