// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import (
	"context"
	"sort"
	"testing"

	"github.com/kriegalex/bahnfrei/internal/domain"
)

func TestMeetRecordListsRoundTripSYS049(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	meet := testMeet(t, s)

	ids, err := ListMeetRecordListIDs(ctx, s.DB(), meet.ID)
	if err != nil || len(ids) != 0 {
		t.Fatalf("ListMeetRecordListIDs before configuration = (%v, %v), want (empty, nil)", ids, err)
	}

	if err := AddMeetRecordList(ctx, s.DB(), meet.ID, "list-a"); err != nil {
		t.Fatalf("AddMeetRecordList: %v", err)
	}
	// Re-adding the same pair is a no-op, not a duplicate/error.
	if err := AddMeetRecordList(ctx, s.DB(), meet.ID, "list-a"); err != nil {
		t.Fatalf("AddMeetRecordList (repeat): %v", err)
	}
	if err := AddMeetRecordList(ctx, s.DB(), meet.ID, "list-b"); err != nil {
		t.Fatalf("AddMeetRecordList: %v", err)
	}
	ids, err = ListMeetRecordListIDs(ctx, s.DB(), meet.ID)
	if err != nil {
		t.Fatalf("ListMeetRecordListIDs: %v", err)
	}
	sort.Strings(ids)
	if len(ids) != 2 || ids[0] != "list-a" || ids[1] != "list-b" {
		t.Fatalf("ids = %v, want [list-a list-b]", ids)
	}

	if err := RemoveMeetRecordList(ctx, s.DB(), meet.ID, "list-a"); err != nil {
		t.Fatalf("RemoveMeetRecordList: %v", err)
	}
	if ids, err = ListMeetRecordListIDs(ctx, s.DB(), meet.ID); err != nil || len(ids) != 1 || ids[0] != "list-b" {
		t.Fatalf("ids after remove = %v, %v, want [list-b]", ids, err)
	}

	if err := ReplaceMeetRecordLists(ctx, s.DB(), meet.ID, []string{"list-c", "list-d"}); err != nil {
		t.Fatalf("ReplaceMeetRecordLists: %v", err)
	}
	ids, err = ListMeetRecordListIDs(ctx, s.DB(), meet.ID)
	sort.Strings(ids)
	if err != nil || len(ids) != 2 || ids[0] != "list-c" || ids[1] != "list-d" {
		t.Fatalf("ids after replace = %v, %v, want [list-c list-d]", ids, err)
	}

	if err := ReplaceMeetRecordLists(ctx, s.DB(), meet.ID, nil); err != nil {
		t.Fatalf("ReplaceMeetRecordLists(nil): %v", err)
	}
	if ids, err := ListMeetRecordListIDs(ctx, s.DB(), meet.ID); err != nil || len(ids) != 0 {
		t.Fatalf("ids after clearing = %v, %v, want empty", ids, err)
	}

	if err := AddMeetRecordList(ctx, s.DB(), "", "x"); err == nil {
		t.Error("AddMeetRecordList without meet id: want error")
	}
	if err := AddMeetRecordList(ctx, s.DB(), meet.ID, ""); err == nil {
		t.Error("AddMeetRecordList without list id: want error")
	}
	if err := ReplaceMeetRecordLists(ctx, s.DB(), "", nil); err == nil {
		t.Error("ReplaceMeetRecordLists without meet id: want error")
	}
}

// TestListAthleteResultsByDisciplineSYS049 covers the PB/SB history query:
// only settled, marked results in the named discipline come back
// (status-only rows like DNS are excluded), across every meet the athlete
// competed in, and excludeUnitID lets a caller drop one specific row.
func TestListAthleteResultsByDisciplineSYS049(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()

	athlete, err := CreateAthlete(ctx, s.DB(), domain.Athlete{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2009, Sex: domain.SexFemale,
	})
	if err != nil {
		t.Fatalf("CreateAthlete: %v", err)
	}

	newUnit := func(startYear int, startMonth int) UnitRecord {
		meet, err := CreateMeet(ctx, s.DB(), domain.Meet{
			Name: "Meet", Venue: "V", StartDate: day(startYear, 6, startMonth), EndDate: day(startYear, 6, startMonth),
			Organizer: "o", Tier: "C-Meeting",
		})
		if err != nil {
			t.Fatalf("CreateMeet: %v", err)
		}
		ev, err := CreateEvent(ctx, s.DB(), domain.Event{MeetID: meet.ID, DisciplineCode: "100m", CategoryCodes: []string{"U18 W"}})
		if err != nil {
			t.Fatalf("CreateEvent: %v", err)
		}
		round, err := CreateRound(ctx, s.DB(), domain.Round{EventID: ev.ID, Kind: domain.RoundFinal}, 0)
		if err != nil {
			t.Fatalf("CreateRound: %v", err)
		}
		unit, err := CreateUnit(ctx, s.DB(), domain.Unit{RoundID: round.ID})
		if err != nil {
			t.Fatalf("CreateUnit: %v", err)
		}
		return unit
	}

	unit2025 := newUnit(2025, 1)
	unit2026 := newUnit(2026, 1)
	unitDNS := newUnit(2026, 2)

	if _, err := SaveResult(ctx, s.DB(), domain.Result{UnitID: unit2025.ID, AthleteID: athlete.ID, Mark: "12.10"}, domain.TimingElectronic); err != nil {
		t.Fatalf("SaveResult 2025: %v", err)
	}
	if _, err := SaveResult(ctx, s.DB(), domain.Result{UnitID: unit2026.ID, AthleteID: athlete.ID, Mark: "12.02"}, domain.TimingElectronic); err != nil {
		t.Fatalf("SaveResult 2026: %v", err)
	}
	if _, err := SaveResult(ctx, s.DB(), domain.Result{UnitID: unitDNS.ID, AthleteID: athlete.ID, Status: domain.StatusDNS}, domain.TimingNone); err != nil {
		t.Fatalf("SaveResult DNS: %v", err)
	}

	hist, err := ListAthleteResultsByDiscipline(ctx, s.DB(), athlete.ID, "100m", "")
	if err != nil {
		t.Fatalf("ListAthleteResultsByDiscipline: %v", err)
	}
	if len(hist) != 2 {
		t.Fatalf("history = %+v, want 2 rows (DNS excluded)", hist)
	}
	years := map[int]bool{}
	for _, h := range hist {
		years[h.MeetStartDate.Year()] = true
	}
	if !years[2025] || !years[2026] {
		t.Errorf("history years = %v, want 2025 and 2026 present", years)
	}

	excluded, err := ListAthleteResultsByDiscipline(ctx, s.DB(), athlete.ID, "100m", unit2026.ID)
	if err != nil {
		t.Fatalf("ListAthleteResultsByDiscipline (exclude): %v", err)
	}
	if len(excluded) != 1 || excluded[0].Mark != "12.10" {
		t.Fatalf("history excluding unit2026 = %+v, want just the 12.10 row", excluded)
	}

	other, err := ListAthleteResultsByDiscipline(ctx, s.DB(), athlete.ID, "800m", "")
	if err != nil {
		t.Fatalf("ListAthleteResultsByDiscipline (other discipline): %v", err)
	}
	if len(other) != 0 {
		t.Errorf("history for unrelated discipline = %+v, want none", other)
	}
}
