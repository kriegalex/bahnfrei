// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package domain

import (
	"testing"
	"time"
)

// TestPublicDisplayNameForSYS103UC023_2 covers the SYS-103 consent gate's
// allow and deny paths: an athlete whose publication consent was not
// withdrawn is displayed normally (allow); one whose consent was withdrawn
// renders the neutral marker instead of their real name (deny/suppress —
// UC-023 #2), never a mix of the two.
func TestPublicDisplayNameForSYS103UC023_2(t *testing.T) {
	t.Run("allow: consent not withdrawn shows the real name", func(t *testing.T) {
		first, last := PublicDisplayNameFor(PublicationConsent{}, "Anna", "Muster")
		if first != "Anna" || last != "Muster" {
			t.Fatalf("got (%q, %q), want (\"Anna\", \"Muster\")", first, last)
		}
	})
	t.Run("deny: withdrawn consent suppresses the name", func(t *testing.T) {
		first, last := PublicDisplayNameFor(PublicationConsent{ResultsPublicationWithdrawn: true}, "Anna", "Muster")
		if first != PublicSuppressedMarker || last != "" {
			t.Fatalf("got (%q, %q), want (%q, \"\")", first, last, PublicSuppressedMarker)
		}
		if first == "Anna" || last == "Muster" {
			t.Fatal("suppressed name must never leak the real first/last name")
		}
	})
}

// TestPublicDisplayClubForSYS103UC023_2 mirrors the name gate for club
// affiliation (also neutralized on withdrawal: SYS-103 rationale in
// internal/domain/privacy.go).
func TestPublicDisplayClubForSYS103UC023_2(t *testing.T) {
	t.Run("allow: consent not withdrawn shows the real club", func(t *testing.T) {
		if got := PublicDisplayClubFor(PublicationConsent{}, "LC Zürich"); got != "LC Zürich" {
			t.Fatalf("got %q, want \"LC Zürich\"", got)
		}
	})
	t.Run("deny: withdrawn consent suppresses the club", func(t *testing.T) {
		if got := PublicDisplayClubFor(PublicationConsent{ResultsPublicationWithdrawn: true}, "LC Zürich"); got != PublicSuppressedMarker {
			t.Fatalf("got %q, want %q", got, PublicSuppressedMarker)
		}
	})
}

// TestIsMinorSYS103 covers both the full-birth-date path and the
// birth-year-only fallback (which must round conservatively — never
// misclassify a minor as an adult just because the day/month is unknown).
func TestIsMinorSYS103(t *testing.T) {
	now := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)

	t.Run("full birth date: exactly 18 today is not a minor", func(t *testing.T) {
		bd := time.Date(2008, 7, 12, 0, 0, 0, 0, time.UTC)
		a := Athlete{BirthDate: &bd, BirthYear: 2008}
		if a.IsMinor(now) {
			t.Fatal("18th birthday today: IsMinor = true, want false")
		}
	})
	t.Run("full birth date: turns 18 tomorrow is still a minor", func(t *testing.T) {
		bd := time.Date(2008, 7, 13, 0, 0, 0, 0, time.UTC)
		a := Athlete{BirthDate: &bd, BirthYear: 2008}
		if !a.IsMinor(now) {
			t.Fatal("18th birthday tomorrow: IsMinor = false, want true")
		}
	})
	t.Run("birth year only: conservative approximation never misclassifies a minor as an adult", func(t *testing.T) {
		// Born sometime in 2008 (exact date unknown): the conservative
		// approximation assumes 31 December, so as of 2026-07-12 this
		// athlete is still treated as 17 (a minor), not 18.
		a := Athlete{BirthYear: 2008}
		if !a.IsMinor(now) {
			t.Fatal("birth-year-only 2008 as of 2026-07-12: IsMinor = false, want true (conservative)")
		}
	})
	t.Run("birth year only: clearly adult", func(t *testing.T) {
		a := Athlete{BirthYear: 2000}
		if a.IsMinor(now) {
			t.Fatal("birth-year-only 2000: IsMinor = true, want false")
		}
	})
}

// TestAnonymizePersonalDataSYS101UC024_2 covers the SYS-101 erasure
// request's field-by-field contract (UC-024 #2): identifying fields are
// cleared/pseudonymized, and the fields the sporting record's integrity
// depends on (BirthYear for category resolution, Sex, Nationality,
// ClubIDs) are explicitly preserved.
func TestAnonymizePersonalDataSYS101UC024_2(t *testing.T) {
	bd := time.Date(2010, 3, 4, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	a := Athlete{
		ID:          "01ATHLETE123456",
		FirstName:   "Anna",
		LastName:    "Muster",
		BirthDate:   &bd,
		BirthYear:   2010,
		Sex:         SexFemale,
		Nationality: "CH",
		ClubIDs:     []string{"club-1"},
		ExternalIDs: ExternalIDs{NamespaceSwissAthleticsLicence: "SA-999"},
	}
	a.AnonymizePersonalData(now)

	t.Run("identifying fields cleared", func(t *testing.T) {
		if a.FirstName == "Anna" || a.LastName == "Muster" {
			t.Fatal("erasure must not retain the original name")
		}
		if a.BirthDate != nil {
			t.Fatal("erasure must clear the full birth date")
		}
		if len(a.ExternalIDs) != 0 {
			t.Fatalf("erasure must clear external IDs (licence numbers), got %v", a.ExternalIDs)
		}
	})
	t.Run("sporting-record fields preserved", func(t *testing.T) {
		if a.BirthYear != 2010 {
			t.Fatalf("BirthYear must survive erasure (category integrity): got %d, want 2010", a.BirthYear)
		}
		if a.Sex != SexFemale {
			t.Fatalf("Sex must survive erasure: got %q", a.Sex)
		}
		if a.Nationality != "CH" {
			t.Fatalf("Nationality must survive erasure: got %q", a.Nationality)
		}
		if len(a.ClubIDs) != 1 || a.ClubIDs[0] != "club-1" {
			t.Fatalf("ClubIDs must survive erasure: got %v", a.ClubIDs)
		}
	})
	t.Run("markers set", func(t *testing.T) {
		if !a.Anonymized {
			t.Fatal("Anonymized flag must be set")
		}
		if a.AnonymizedAt == nil || !a.AnonymizedAt.Equal(now) {
			t.Fatalf("AnonymizedAt = %v, want %v", a.AnonymizedAt, now)
		}
	})
	t.Run("pseudonym is stable and derived only from the athlete's own ID", func(t *testing.T) {
		b := Athlete{ID: a.ID, FirstName: "Someone", LastName: "Else"}
		b.AnonymizePersonalData(now)
		if b.FirstName != a.FirstName || b.LastName != a.LastName {
			t.Fatalf("pseudonym for the same ID must be stable regardless of the erased name: got (%q,%q) vs (%q,%q)",
				b.FirstName, b.LastName, a.FirstName, a.LastName)
		}
	})
}
