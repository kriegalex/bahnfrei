// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import (
	"context"
	"testing"

	"github.com/kriegalex/bahnfrei/internal/domain"
)

// TestMeetFeeScheduleRoundTripSYS017 covers the SYS-017 configurable fee
// schedule storage: it defaults to zero and can be updated under optimistic
// concurrency.
func TestMeetFeeScheduleRoundTripSYS017(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	meet := testMeet(t, s)

	if meet.EntryFeeCents != 0 || meet.RelayFeeCents != 0 {
		t.Fatalf("new meet fee schedule = (%d, %d), want (0, 0)", meet.EntryFeeCents, meet.RelayFeeCents)
	}

	newVersion, err := UpdateMeetFeeSchedule(ctx, s.DB(), meet.ID, meet.Version, 1500, 4000)
	if err != nil {
		t.Fatalf("UpdateMeetFeeSchedule: %v", err)
	}
	if newVersion != meet.Version+1 {
		t.Fatalf("new version = %d, want %d", newVersion, meet.Version+1)
	}

	got, err := GetMeet(ctx, s.DB(), meet.ID)
	if err != nil {
		t.Fatalf("GetMeet: %v", err)
	}
	if got.EntryFeeCents != 1500 || got.RelayFeeCents != 4000 {
		t.Errorf("fee schedule = (%d, %d), want (1500, 4000)", got.EntryFeeCents, got.RelayFeeCents)
	}
}

// TestEventEntryLimitRoundTripSYS015 covers the per-event entry-limit column
// (SYS-015 "entry limits"): it round-trips through CreateEvent/ListEvents/
// GetEvent.
func TestEventEntryLimitRoundTripSYS015(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	meet := testMeet(t, s)

	ev, err := CreateEvent(ctx, s.DB(), domain.Event{
		MeetID: meet.ID, DisciplineCode: "100m", CategoryCodes: []string{"U18 W"}, EntryLimit: 24,
	})
	if err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}
	if ev.EntryLimit != 24 {
		t.Fatalf("created event entry limit = %d, want 24", ev.EntryLimit)
	}

	got, err := GetEvent(ctx, s.DB(), ev.ID)
	if err != nil {
		t.Fatalf("GetEvent: %v", err)
	}
	if got.EntryLimit != 24 {
		t.Errorf("GetEvent entry limit = %d, want 24", got.EntryLimit)
	}

	listed, err := ListEvents(ctx, s.DB(), meet.ID)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(listed) != 1 || listed[0].EntryLimit != 24 {
		t.Errorf("ListEvents = %+v, want one event with entry limit 24", listed)
	}
}
