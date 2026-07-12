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
