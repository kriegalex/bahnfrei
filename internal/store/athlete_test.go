// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import (
	"context"
	"errors"
	"testing"

	"github.com/kriegalex/bahnfrei/internal/domain"
)

// TestFindAthleteByExternalIDSYS013 covers the CSV/Alabus re-import match
// key (UC-004 #2 idempotency): an athlete carrying a namespaced external ID
// (e.g. a Swiss Athletics licence number, ADR-005 §6) is found by that
// (namespace, id) pair, and an unknown pair reports ErrNotFound.
func TestFindAthleteByExternalIDSYS013(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	created, err := CreateAthlete(ctx, s.DB(), domain.Athlete{
		FirstName: "Lea", LastName: "Muster", BirthYear: 2010, Sex: domain.SexFemale,
		ExternalIDs: domain.ExternalIDs{domain.NamespaceSwissAthleticsLicence: "SA-12345"},
	})
	if err != nil {
		t.Fatalf("CreateAthlete: %v", err)
	}

	got, err := FindAthleteByExternalID(ctx, s.DB(), domain.NamespaceSwissAthleticsLicence, "SA-12345")
	if err != nil {
		t.Fatalf("FindAthleteByExternalID: %v", err)
	}
	if got.ID != created.ID {
		t.Fatalf("got athlete %s, want %s", got.ID, created.ID)
	}

	if _, err := FindAthleteByExternalID(ctx, s.DB(), domain.NamespaceSwissAthleticsLicence, "SA-99999"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for unknown licence, got %v", err)
	}
	if _, err := FindAthleteByExternalID(ctx, s.DB(), domain.NamespaceWorldAthleticsID, "SA-12345"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for a different namespace, got %v", err)
	}
}

// TestFindAthleteByNaturalKeySYS013 covers the fallback idempotency match
// (no licence number on the row, UC-004 #2): case-insensitive first/last
// name plus birth year and sex.
func TestFindAthleteByNaturalKeySYS013(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	created, err := CreateAthlete(ctx, s.DB(), domain.Athlete{
		FirstName: "Nils", LastName: "Meier", BirthYear: 2012, Sex: domain.SexMale,
	})
	if err != nil {
		t.Fatalf("CreateAthlete: %v", err)
	}

	got, err := FindAthleteByNaturalKey(ctx, s.DB(), "NILS", "meier", 2012, domain.SexMale)
	if err != nil {
		t.Fatalf("FindAthleteByNaturalKey: %v", err)
	}
	if got.ID != created.ID {
		t.Fatalf("got athlete %s, want %s (case-insensitive match)", got.ID, created.ID)
	}

	if _, err := FindAthleteByNaturalKey(ctx, s.DB(), "Nils", "Meier", 2013, domain.SexMale); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for a different birth year, got %v", err)
	}
	if _, err := FindAthleteByNaturalKey(ctx, s.DB(), "Nils", "Meier", 2012, domain.SexFemale); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for a different sex, got %v", err)
	}
}

// TestSetAthleteExternalIDSYS013 covers enriching an existing athlete with a
// licence number discovered on re-import (UC-004 #2), without disturbing
// other fields, and that setting the same value twice is an idempotent
// no-op (does not bump version).
func TestSetAthleteExternalIDSYS013(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	created, err := CreateAthlete(ctx, s.DB(), domain.Athlete{
		FirstName: "Timo", LastName: "Frei", BirthYear: 2011, Sex: domain.SexMale,
	})
	if err != nil {
		t.Fatalf("CreateAthlete: %v", err)
	}

	v, err := SetAthleteExternalID(ctx, s.DB(), created.ID, created.Version, domain.NamespaceSwissAthleticsLicence, "SA-777")
	if err != nil {
		t.Fatalf("SetAthleteExternalID: %v", err)
	}
	if v != created.Version+1 {
		t.Fatalf("version = %d, want %d", v, created.Version+1)
	}

	got, err := GetAthlete(ctx, s.DB(), created.ID)
	if err != nil {
		t.Fatalf("GetAthlete: %v", err)
	}
	if got.FirstName != "Timo" || got.LastName != "Frei" {
		t.Fatalf("SetAthleteExternalID must not disturb other fields, got %+v", got.Athlete)
	}
	if id, ok := got.ExternalIDs.Get(domain.NamespaceSwissAthleticsLicence); !ok || id != "SA-777" {
		t.Fatalf("licence external id = (%q, %v), want (\"SA-777\", true)", id, ok)
	}

	// Idempotent: setting the same value again is a no-op (returns the
	// current version unchanged).
	v2, err := SetAthleteExternalID(ctx, s.DB(), created.ID, v, domain.NamespaceSwissAthleticsLicence, "SA-777")
	if err != nil {
		t.Fatalf("SetAthleteExternalID (repeat): %v", err)
	}
	if v2 != v {
		t.Fatalf("repeat set with the same value should be a no-op, version = %d, want %d", v2, v)
	}
}

// TestUpdateAthleteIdentitySYS150UC043 covers the TASK-049 identity
// correction's storage primitive: name/birth-year/sex/club overwrite under
// optimistic concurrency, leaving external IDs and consent untouched (only
// UpdateParticipantIdentity's caller-facing contract, not this function,
// enforces the "never touches consent" invariant — this test pins that no
// column beyond the five documented ones changes).
func TestUpdateAthleteIdentitySYS150UC043(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	club, err := CreateClub(ctx, s.DB(), domain.Club{Name: "LC Test"})
	if err != nil {
		t.Fatalf("CreateClub: %v", err)
	}
	created, err := CreateAthlete(ctx, s.DB(), domain.Athlete{
		FirstName: "Ana", LastName: "Musterr", BirthYear: 2014, Sex: domain.SexFemale,
		ExternalIDs: domain.ExternalIDs{domain.NamespaceSwissAthleticsLicence: "SA-1"},
	})
	if err != nil {
		t.Fatalf("CreateAthlete: %v", err)
	}

	v, err := UpdateAthleteIdentity(ctx, s.DB(), created.ID, created.Version,
		"Anna", "Muster", 2013, domain.SexMale, []string{club.ID})
	if err != nil {
		t.Fatalf("UpdateAthleteIdentity: %v", err)
	}
	if v != created.Version+1 {
		t.Fatalf("version = %d, want %d", v, created.Version+1)
	}

	got, err := GetAthlete(ctx, s.DB(), created.ID)
	if err != nil {
		t.Fatalf("GetAthlete: %v", err)
	}
	if got.FirstName != "Anna" || got.LastName != "Muster" || got.BirthYear != 2013 || got.Sex != domain.SexMale {
		t.Errorf("identity after update = %+v, want Anna Muster/2013/M", got.Athlete)
	}
	if len(got.ClubIDs) != 1 || got.ClubIDs[0] != club.ID {
		t.Errorf("club ids after update = %v, want [%s]", got.ClubIDs, club.ID)
	}
	if id, ok := got.ExternalIDs.Get(domain.NamespaceSwissAthleticsLicence); !ok || id != "SA-1" {
		t.Errorf("UpdateAthleteIdentity disturbed external ids: %v/%v", id, ok)
	}

	// Clearing the club (empty slice) round-trips as no club, not a stray
	// element.
	if _, err := UpdateAthleteIdentity(ctx, s.DB(), created.ID, v, "Anna", "Muster", 2013, domain.SexMale, nil); err != nil {
		t.Fatalf("UpdateAthleteIdentity (clear club): %v", err)
	}
	got, err = GetAthlete(ctx, s.DB(), created.ID)
	if err != nil {
		t.Fatalf("GetAthlete: %v", err)
	}
	if len(got.ClubIDs) != 0 {
		t.Errorf("club ids after clearing = %v, want empty", got.ClubIDs)
	}

	// Stale version is rejected.
	if _, err := UpdateAthleteIdentity(ctx, s.DB(), created.ID, v, "Someone", "Else", 2013, domain.SexMale, nil); !errors.Is(err, ErrVersionConflict) {
		t.Errorf("stale-version update = %v, want ErrVersionConflict", err)
	}
}
