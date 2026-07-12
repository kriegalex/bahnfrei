// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package domain

import "testing"

// TestEvaluateEntryStandardSYS015UC003_5 is the UC-003 #5 fixture verbatim:
// a 100 m entry standard "12.20" fails a "12.85" seed (track: lower is
// better), and passes a faster one.
func TestEvaluateEntryStandardSYS015UC003_5(t *testing.T) {
	fails, err := EvaluateEntryStandard("12.85", "12.20", FamilyTrack)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fails {
		t.Fatal("seed 12.85 must fail standard 12.20 for a track discipline")
	}

	fails, err = EvaluateEntryStandard("12.00", "12.20", FamilyTrack)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fails {
		t.Fatal("seed 12.00 must pass standard 12.20 for a track discipline")
	}
}

// TestEvaluateEntryStandard_FieldDirection covers the opposite-direction
// case: for field disciplines a higher mark is better, so a seed below the
// standard fails.
func TestEvaluateEntryStandard_FieldDirection(t *testing.T) {
	fails, err := EvaluateEntryStandard("5.50", "6.00", FamilyFieldHorizontal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fails {
		t.Fatal("seed 5.50 must fail standard 6.00 for a field discipline")
	}

	fails, err = EvaluateEntryStandard("6.50", "6.00", FamilyFieldHorizontal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fails {
		t.Fatal("seed 6.50 must pass standard 6.00 for a field discipline")
	}
}

// TestEvaluateEntryStandard_NoStandardConfigured: an event with no entry
// standard never fails, regardless of the seed (SYS-015: the condition is
// per-event and optional).
func TestEvaluateEntryStandard_NoStandardConfigured(t *testing.T) {
	fails, err := EvaluateEntryStandard("99.99", "", FamilyTrack)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fails {
		t.Fatal("no configured standard must never fail")
	}
	// Even an empty seed is fine when no standard is configured — nothing to
	// evaluate against.
	if fails, err = EvaluateEntryStandard("", "", FamilyTrack); err != nil || fails {
		t.Fatalf("empty seed with no standard: got (%v, %v)", fails, err)
	}
}

// TestEvaluateEntryStandard_SeedRequiredWhenStandardConfigured is SYS-015's
// "required seed-performance information": once a standard is configured, a
// missing seed cannot be evaluated and is an error, not a silent pass.
func TestEvaluateEntryStandard_SeedRequiredWhenStandardConfigured(t *testing.T) {
	if _, err := EvaluateEntryStandard("", "12.20", FamilyTrack); err == nil {
		t.Fatal("expected an error for a missing seed against a configured standard")
	}
}

func TestEvaluateEntryStandard_InvalidMarks(t *testing.T) {
	if _, err := EvaluateEntryStandard("not-a-mark", "12.20", FamilyTrack); err == nil {
		t.Fatal("expected an error for an unparseable seed")
	}
	if _, err := EvaluateEntryStandard("12.20", "not-a-mark", FamilyTrack); err == nil {
		t.Fatal("expected an error for an unparseable standard")
	}
}

func TestEntry_Validate(t *testing.T) {
	valid := Entry{ID: "01E", EventID: "01EV", AthleteID: "01ATH"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid entry, got error: %v", err)
	}

	relay := Entry{ID: "01E", EventID: "01EV", RelayTeamID: "01RT"}
	if err := relay.Validate(); err != nil {
		t.Fatalf("expected valid relay entry, got error: %v", err)
	}

	cases := []Entry{
		{EventID: "01EV", AthleteID: "01ATH"},                              // no id
		{ID: "01E", AthleteID: "01ATH"},                                    // no event id
		{ID: "01E", EventID: "01EV"},                                       // neither athlete nor relay team
		{ID: "01E", EventID: "01EV", AthleteID: "01ATH", RelayTeamID: "x"}, // both
	}
	for i, c := range cases {
		if err := c.Validate(); err == nil {
			t.Errorf("case %d: expected a validation error, got none", i)
		}
	}
}

func TestRelayTeam_Validate(t *testing.T) {
	valid := RelayTeam{ID: "01RT", ClubID: "01C", Composition: []string{"a1", "a2", "a3", "a4"}, Reserves: []string{"a5", "a6"}}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid relay team, got error: %v", err)
	}

	cases := []RelayTeam{
		{ClubID: "01C", Composition: []string{"a1"}},                                       // no id
		{ID: "01RT", Composition: []string{"a1"}},                                          // no club
		{ID: "01RT", ClubID: "01C"},                                                        // empty composition
		{ID: "01RT", ClubID: "01C", Composition: []string{"a1", "a1"}},                     // duplicate leg
		{ID: "01RT", ClubID: "01C", Composition: []string{"a1"}, Reserves: []string{"a1"}}, // leg also a reserve
	}
	for i, c := range cases {
		if err := c.Validate(); err == nil {
			t.Errorf("case %d: expected a validation error, got none", i)
		}
	}
}
