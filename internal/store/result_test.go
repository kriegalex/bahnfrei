// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import (
	"context"
	"errors"
	"testing"

	"github.com/kriegalex/bahnfrei/internal/domain"
)

// ukcFixture builds a meet with one 60m event/round/unit plus one athlete
// in one club, returning the unit and athlete IDs.
func ukcFixture(t *testing.T, s *Store) (meetID, unitID, athleteID string) {
	t.Helper()
	ctx := context.Background()
	meet := testMeet(t, s)
	ev, err := CreateEvent(ctx, s.DB(), domain.Event{
		MeetID: meet.ID, DisciplineCode: "60m", CategoryCodes: []string{"W12"},
	})
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
	club, err := CreateClub(ctx, s.DB(), domain.Club{Name: "LC Test"})
	if err != nil {
		t.Fatalf("CreateClub: %v", err)
	}
	athlete, err := CreateAthlete(ctx, s.DB(), domain.Athlete{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2014,
		Sex: domain.SexFemale, ClubIDs: []string{club.ID},
	})
	if err != nil {
		t.Fatalf("CreateAthlete: %v", err)
	}
	return meet.ID, unit.ID, athlete.ID
}

func TestAthleteAndClubRoundTrip(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	_, _, athleteID := ukcFixture(t, s)

	got, err := GetAthlete(ctx, s.DB(), athleteID)
	if err != nil {
		t.Fatalf("GetAthlete: %v", err)
	}
	if got.FirstName != "Anna" || got.BirthYear != 2014 || got.Sex != domain.SexFemale {
		t.Errorf("athlete = %+v", got.Athlete)
	}
	if got.BirthDate != nil {
		t.Errorf("birth date = %v, want nil (only the year was known, SYS-010)", got.BirthDate)
	}
	if len(got.ClubIDs) != 1 {
		t.Fatalf("club ids = %v", got.ClubIDs)
	}
	names, err := ClubNames(ctx, s.DB(), got.ClubIDs)
	if err != nil {
		t.Fatalf("ClubNames: %v", err)
	}
	if names[got.ClubIDs[0]] != "LC Test" {
		t.Errorf("club names = %v", names)
	}
	club, err := GetClubByName(ctx, s.DB(), "LC Test")
	if err != nil || club.ID != got.ClubIDs[0] {
		t.Errorf("GetClubByName = %+v, %v", club, err)
	}
	if _, err := GetClubByName(ctx, s.DB(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetClubByName(unknown) = %v, want ErrNotFound", err)
	}
	if _, err := GetAthlete(ctx, s.DB(), "no-such-id"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetAthlete(unknown) = %v, want ErrNotFound", err)
	}
}

func TestParticipantsAndBibUniqueness(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	meetID, _, athleteID := ukcFixture(t, s)

	if _, err := RegisterParticipant(ctx, s.DB(), meetID, athleteID, "101"); err != nil {
		t.Fatalf("RegisterParticipant: %v", err)
	}
	// Same athlete again → duplicate; different athlete, same bib → duplicate.
	if _, err := RegisterParticipant(ctx, s.DB(), meetID, athleteID, "999"); !errors.Is(err, ErrDuplicateParticipant) {
		t.Errorf("re-register athlete = %v, want ErrDuplicateParticipant", err)
	}
	other, err := CreateAthlete(ctx, s.DB(), domain.Athlete{
		LastName: "Other", BirthYear: 2013, Sex: domain.SexMale,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RegisterParticipant(ctx, s.DB(), meetID, other.ID, "101"); !errors.Is(err, ErrDuplicateParticipant) {
		t.Errorf("duplicate bib = %v, want ErrDuplicateParticipant", err)
	}
	// Unassigned bibs ('') stay non-conflicting.
	if _, err := RegisterParticipant(ctx, s.DB(), meetID, other.ID, ""); err != nil {
		t.Fatalf("register without bib: %v", err)
	}
	third, err := CreateAthlete(ctx, s.DB(), domain.Athlete{LastName: "Third", BirthYear: 2012, Sex: domain.SexFemale})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RegisterParticipant(ctx, s.DB(), meetID, third.ID, ""); err != nil {
		t.Fatalf("second register without bib: %v", err)
	}

	rows, err := ListParticipants(ctx, s.DB(), meetID)
	if err != nil {
		t.Fatalf("ListParticipants: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d participants, want 3", len(rows))
	}
	if rows[len(rows)-1].Bib != "101" && rows[0].Athlete.LastName == "" {
		t.Errorf("rows not joined with athlete data: %+v", rows)
	}
}

func TestSaveResultUpsertAndListing(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	meetID, unitID, athleteID := ukcFixture(t, s)

	pts := 710
	first, err := SaveResult(ctx, s.DB(), domain.Result{
		UnitID: unitID, AthleteID: athleteID, Mark: "8.42", Points: &pts,
	}, domain.TimingElectronic)
	if err != nil {
		t.Fatalf("SaveResult: %v", err)
	}
	if first.Version != 1 || first.Timing != domain.TimingElectronic {
		t.Errorf("first save = %+v", first)
	}
	if first.Wind != nil || first.Placing != nil {
		t.Errorf("optional fields must round-trip as NULL: %+v", first)
	}

	// Correction replaces in place under the same (unit, athlete) key.
	pts2 := 705
	second, err := SaveResult(ctx, s.DB(), domain.Result{
		UnitID: unitID, AthleteID: athleteID, Mark: "8.43", Points: &pts2,
	}, domain.TimingElectronic)
	if err != nil {
		t.Fatalf("SaveResult(correction): %v", err)
	}
	if second.Version != 2 || second.Mark != "8.43" || *second.Points != 705 {
		t.Errorf("correction = %+v", second)
	}
	if second.ID != first.ID {
		t.Errorf("correction created a new row (%s → %s), want in-place replacement", first.ID, second.ID)
	}

	list, err := ListMeetResults(ctx, s.DB(), meetID)
	if err != nil {
		t.Fatalf("ListMeetResults: %v", err)
	}
	if len(list) != 1 || list[0].DisciplineCode != "60m" || list[0].Mark != "8.43" {
		t.Errorf("meet results = %+v", list)
	}
	if _, err := GetResult(ctx, s.DB(), unitID, "nobody"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetResult(unknown) = %v, want ErrNotFound", err)
	}
	if _, err := SaveResult(ctx, s.DB(), domain.Result{}, domain.TimingNone); err == nil {
		t.Error("SaveResult without unit/athlete: want error")
	}
}
