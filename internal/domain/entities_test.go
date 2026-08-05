// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package domain

import (
	"strings"
	"testing"
	"time"
)

// TestExternalIDs_Namespaced covers ADR-005 §6: external identifiers are
// typed, namespaced attributes, never overloaded onto an entity's own ID —
// an athlete can carry both a Swiss Athletics licence number and a World
// Athletics ID side by side, additively.
func TestExternalIDs_Namespaced(t *testing.T) {
	var ext ExternalIDs
	ext.Set(NamespaceSwissAthleticsLicence, "SA-123456")
	ext.Set(NamespaceWorldAthleticsID, "WA-987654")

	licence, ok := ext.Get(NamespaceSwissAthleticsLicence)
	if !ok || licence != "SA-123456" {
		t.Fatalf("swiss athletics licence: got (%q, %v)", licence, ok)
	}
	waID, ok := ext.Get(NamespaceWorldAthleticsID)
	if !ok || waID != "WA-987654" {
		t.Fatalf("world athletics id: got (%q, %v)", waID, ok)
	}
	if _, ok := ext.Get("unknown:namespace"); ok {
		t.Fatal("expected no value for an unset namespace")
	}
}

func TestExternalIDs_SetOnNilMap(t *testing.T) {
	var a Athlete
	a.ExternalIDs.Set(NamespaceSwissAthleticsLicence, "SA-1")
	if v, ok := a.ExternalIDs.Get(NamespaceSwissAthleticsLicence); !ok || v != "SA-1" {
		t.Fatalf("Set on a nil ExternalIDs map must initialize it: got (%q, %v)", v, ok)
	}
}

// TestValidLicenceNo covers the DEC-023/TASK-039 online-entry licence-number
// field's well-formedness check: deliberately permissive (letters, digits,
// space, dot, hyphen, slash; 1-40 chars) since no specific national licence
// format is verified (OQ-030) — it only catches obviously malformed input
// (control characters/newlines, empty, absurd length).
func TestValidLicenceNo(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"plain digits", "123456", true},
		{"letters and digits with hyphen", "SA-1234", true},
		{"with dot and slash", "CH.2026/001", true},
		{"single char", "A", true},
		{"unicode letters", "Müller-42", true},
		{"exactly 40 chars", strings.Repeat("x", 40), true},
		{"empty", "", false},
		{"too long (41 chars)", strings.Repeat("x", 41), false},
		{"embedded newline", "SA-1\n234", false},
		{"embedded tab", "SA-1\t234", false},
		{"control character", "SA-1\x00234", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ValidLicenceNo(tc.in); got != tc.want {
				t.Errorf("ValidLicenceNo(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestAthlete_Validate(t *testing.T) {
	valid := Athlete{ID: "01ATH", LastName: "Muster", BirthYear: 2012, Sex: SexMale}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid athlete, got error: %v", err)
	}

	noID := Athlete{LastName: "Muster", BirthYear: 2012, Sex: SexMale}
	if err := noID.Validate(); err == nil {
		t.Fatal("expected error for athlete without an id")
	}

	noLastName := Athlete{ID: "01ATH", BirthYear: 2012, Sex: SexMale}
	if err := noLastName.Validate(); err == nil {
		t.Fatal("expected error for athlete without a last name")
	}

	noBirthYear := Athlete{ID: "01ATH", LastName: "Muster", Sex: SexMale}
	if err := noBirthYear.Validate(); err == nil {
		t.Fatal("expected error for athlete without a birth year (SYS-010)")
	}

	badSex := Athlete{ID: "01ATH", LastName: "Muster", BirthYear: 2012, Sex: "X"}
	if err := badSex.Validate(); err == nil {
		t.Fatal("expected error for invalid sex")
	}

	// SYS-010: full birth date, when known, derives the birth year.
	bd := time.Date(2012, time.May, 4, 0, 0, 0, 0, time.UTC)
	withDate := Athlete{ID: "01ATH", LastName: "Muster", BirthDate: &bd, Sex: SexMale}
	if err := withDate.Validate(); err != nil {
		t.Fatalf("expected valid athlete derived from birth date, got: %v", err)
	}
	if withDate.BirthYear != 2012 {
		t.Fatalf("expected birth year derived from birth date, got %d", withDate.BirthYear)
	}
}

func TestMeet_Validate(t *testing.T) {
	start := time.Date(2027, time.June, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	valid := Meet{ID: "01MEET", Name: "Club Evening Meet", StartDate: start, EndDate: end}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid meet, got error: %v", err)
	}

	noID := Meet{Name: "x", StartDate: start, EndDate: end}
	if err := noID.Validate(); err == nil {
		t.Fatal("expected error for meet without an id")
	}

	noName := Meet{ID: "01MEET", StartDate: start, EndDate: end}
	if err := noName.Validate(); err == nil {
		t.Fatal("expected error for meet without a name")
	}

	invertedDates := Meet{ID: "01MEET", Name: "x", StartDate: end, EndDate: start}
	if err := invertedDates.Validate(); err == nil {
		t.Fatal("expected error for end date before start date")
	}
}

// TestExternalIDs_MeetAndClub covers ADR-005 §6 for Meet (federation
// competition identifiers) and Club (federation club code) entities.
func TestExternalIDs_MeetAndClub(t *testing.T) {
	m := Meet{ID: "01MEET", Name: "x"}
	m.ExternalIDs.Set(NamespaceSwissAthleticsMeetID, "WV-2027-042")
	m.ExternalIDs.Set(NamespaceWAGlobalCalendarID, "GC-2027-9981")
	if v, _ := m.ExternalIDs.Get(NamespaceSwissAthleticsMeetID); v != "WV-2027-042" {
		t.Fatalf("meet external id round-trip failed: got %q", v)
	}

	c := Club{ID: "01CLUB", Name: "LC Zürich"}
	c.ExternalIDs.Set(NamespaceSwissAthleticsClubCode, "ZH-01")
	if v, _ := c.ExternalIDs.Get(NamespaceSwissAthleticsClubCode); v != "ZH-01" {
		t.Fatalf("club external id round-trip failed: got %q", v)
	}
}

// TestSession_Validate covers the SYS-001 session invariants (competition
// days with sessions per day, UC-001 #2).
func TestSession_Validate(t *testing.T) {
	day := time.Date(2027, 6, 12, 0, 0, 0, 0, time.UTC)

	ok := Session{ID: "01SESS", MeetID: "01MEET", Day: day, Label: "Vormittag"}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid session rejected: %v", err)
	}

	noID := Session{MeetID: "01MEET", Day: day}
	if err := noID.Validate(); err == nil {
		t.Fatal("expected error for session without an id")
	}

	noMeet := Session{ID: "01SESS", Day: day}
	if err := noMeet.Validate(); err == nil {
		t.Fatal("expected error for session without a meet id")
	}

	noDay := Session{ID: "01SESS", MeetID: "01MEET"}
	if err := noDay.Validate(); err == nil {
		t.Fatal("expected error for session without a day")
	}
}
