// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package domain

import "testing"

// TestTrackAdvancementSYS029UC009_1 reproduces UC-009 #1: 3 heats with
// results and progression "top 2 places + 2 fastest" advance exactly 6 Q
// and 2 q athletes, q selected by time from a single timing source, ready
// to build the semi-final start list.
func TestTrackAdvancementSYS029UC009_1(t *testing.T) {
	heats := [][]HeatFinish{
		{
			{EntryID: "h1p1", Place: 1, Mark: "11.00"},
			{EntryID: "h1p2", Place: 2, Mark: "11.20"},
			{EntryID: "h1p3", Place: 3, Mark: "11.50"},
			{EntryID: "h1p4", Place: 4, Mark: "11.90"},
		},
		{
			{EntryID: "h2p1", Place: 1, Mark: "11.05"},
			{EntryID: "h2p2", Place: 2, Mark: "11.30"},
			{EntryID: "h2p3", Place: 3, Mark: "11.60"},
			{EntryID: "h2p4", Place: 4, Mark: "12.00"},
		},
		{
			{EntryID: "h3p1", Place: 1, Mark: "11.10"},
			{EntryID: "h3p2", Place: 2, Mark: "11.25"},
			{EntryID: "h3p3", Place: 3, Mark: "11.40"}, // fastest non-Q time (11.40)
			{EntryID: "h3p4", Place: 4, Mark: "12.10"},
		},
	}
	advanced, tie := ComputeTrackAdvancement(heats, AdvancementRule{TopN: 2, FastestK: 2})
	if tie != nil {
		t.Fatalf("unexpected tie: %+v", tie)
	}
	byID := map[string]QualificationStatus{}
	for _, a := range advanced {
		byID[a.EntryID] = a.Code
	}
	wantQ := []string{"h1p1", "h1p2", "h2p1", "h2p2", "h3p1", "h3p2"}
	for _, id := range wantQ {
		if byID[id] != StatusQ {
			t.Errorf("%s: expected Q, got %q", id, byID[id])
		}
	}
	wantQt := []string{"h3p3", "h1p3"} // 11.40 and 11.50 are the two fastest non-Q times
	for _, id := range wantQt {
		if byID[id] != StatusQt {
			t.Errorf("%s: expected q, got %q", id, byID[id])
		}
	}
	if len(advanced) != 8 {
		t.Fatalf("expected exactly 6 Q + 2 q = 8 advancing, got %d: %+v", len(advanced), advanced)
	}
}

// TestTrackAdvancementTimeTieSYS029UC009_2 reproduces UC-009 #2: a tie for
// the last time-qualifier spot is surfaced to the operator rather than
// auto-resolved.
func TestTrackAdvancementTimeTieSYS029UC009_2(t *testing.T) {
	heats := [][]HeatFinish{
		{
			{EntryID: "h1p1", Place: 1, Mark: "11.00"},
			{EntryID: "h1p3", Place: 3, Mark: "11.50"},
		},
		{
			{EntryID: "h2p1", Place: 1, Mark: "11.05"},
			{EntryID: "h2p3", Place: 3, Mark: "11.50"}, // tied with h1p3 for the single remaining q slot
		},
	}
	advanced, tie := ComputeTrackAdvancement(heats, AdvancementRule{TopN: 1, FastestK: 1})
	for _, a := range advanced {
		if a.EntryID == "h1p3" || a.EntryID == "h2p3" {
			t.Fatalf("tied entry %s must not be auto-advanced", a.EntryID)
		}
	}
	if tie == nil {
		t.Fatal("expected a TimeTie for the tied last qualifying spot")
	}
	if tie.RemainingSlots != 1 {
		t.Errorf("expected 1 remaining slot, got %d", tie.RemainingSlots)
	}
	if len(tie.EntryIDs) != 2 {
		t.Errorf("expected both tied entries surfaced, got %v", tie.EntryIDs)
	}
}

// TestTrackAdvancementManualCodesSYS029UC009_3 reproduces UC-009 #3: a
// referee decision advances an athlete with code qR — the domain vocabulary
// (not ComputeTrackAdvancement, which only ever emits Q/q) accepts and
// distinguishes the manual codes so they can be written onto a
// UnitAssignment and appear on the next round's list.
func TestTrackAdvancementManualCodesSYS029UC009_3(t *testing.T) {
	for _, code := range []QualificationStatus{StatusQR, StatusQJ, StatusQD} {
		a := UnitAssignment{ID: "a1", UnitID: "u1", EntryID: "e1", Qualification: code}
		if err := a.Validate(); err != nil {
			t.Errorf("code %q should validate as a legal manual-advancement code (D2.4): %v", code, err)
		}
	}
	bad := UnitAssignment{ID: "a1", UnitID: "u1", EntryID: "e1", Qualification: "qX"}
	if err := bad.Validate(); err == nil {
		t.Error("expected an invalid qualification code to be rejected (SYS-045/D5.2 vocabulary)")
	}
}

// TestFieldAdvancementSYS030UC009_4 reproduces UC-009 #4: a long-throw
// qualification with standard 14.00m and finals capacity 12 — 5 athletes
// beat the standard (Q), the next 7 by mark are q, and the final's start
// order is generated (12 total advance, none left over).
func TestFieldAdvancementSYS030UC009_4(t *testing.T) {
	entries := []FieldFinish{
		{EntryID: "a1", Mark: "15.20"},
		{EntryID: "a2", Mark: "14.80"},
		{EntryID: "a3", Mark: "14.50"},
		{EntryID: "a4", Mark: "14.30"},
		{EntryID: "a5", Mark: "14.05"}, // 5th and last to beat the 14.00 standard
		{EntryID: "b1", Mark: "13.95"},
		{EntryID: "b2", Mark: "13.80"},
		{EntryID: "b3", Mark: "13.70"},
		{EntryID: "b4", Mark: "13.60"},
		{EntryID: "b5", Mark: "13.50"},
		{EntryID: "b6", Mark: "13.40"},
		{EntryID: "b7", Mark: "13.30"}, // 7th and last of the "next 7 by mark"
		{EntryID: "c1", Mark: "13.20"}, // misses the cut entirely
		{EntryID: "c2", Mark: "13.10"},
	}
	advanced, tie := ComputeFieldAdvancement(entries, "14.00", "higher", 12)
	if tie != nil {
		t.Fatalf("unexpected tie: %+v", tie)
	}
	if len(advanced) != 12 {
		t.Fatalf("expected exactly 12 advancing (5 Q + 7 q), got %d: %+v", len(advanced), advanced)
	}
	byID := map[string]QualificationStatus{}
	for _, a := range advanced {
		byID[a.EntryID] = a.Code
	}
	for _, id := range []string{"a1", "a2", "a3", "a4", "a5"} {
		if byID[id] != StatusQ {
			t.Errorf("%s: expected Q (beats standard), got %q", id, byID[id])
		}
	}
	for _, id := range []string{"b1", "b2", "b3", "b4", "b5", "b6", "b7"} {
		if byID[id] != StatusQt {
			t.Errorf("%s: expected q (next by mark), got %q", id, byID[id])
		}
	}
	for _, id := range []string{"c1", "c2"} {
		if _, ok := byID[id]; ok {
			t.Errorf("%s: expected to miss the cut, but advanced as %q", id, byID[id])
		}
	}
}

// TestFieldAdvancementNoStandardConfigured is a denial/edge-path test: an
// event with no qualifying standard fills capacity by mark alone.
func TestFieldAdvancementNoStandardConfigured(t *testing.T) {
	entries := []FieldFinish{
		{EntryID: "a", Mark: "10.00"},
		{EntryID: "b", Mark: "9.50"},
		{EntryID: "c", Mark: "9.00"},
	}
	advanced, tie := ComputeFieldAdvancement(entries, "", "higher", 2)
	if tie != nil {
		t.Fatalf("unexpected tie: %+v", tie)
	}
	if len(advanced) != 2 {
		t.Fatalf("expected 2 advancing, got %d", len(advanced))
	}
	for _, a := range advanced {
		if a.Code != StatusQt {
			t.Errorf("%s: expected q with no standard configured, got %q", a.EntryID, a.Code)
		}
	}
}

// TestTrackAdvancementDNFExcludedFromTimeQualification is a denial/edge-path
// test: a DNF/DQ finisher never qualifies by time even if its recorded mark
// would otherwise beat the cutoff.
func TestTrackAdvancementDNFExcludedFromTimeQualification(t *testing.T) {
	heats := [][]HeatFinish{
		{
			{EntryID: "ok1", Place: 1, Mark: "11.00"},
			{EntryID: "dnf1", Mark: "10.50", Status: StatusDNF}, // would be fastest, but DNF
			{EntryID: "ok2", Place: 2, Mark: "11.80"},
		},
	}
	advanced, tie := ComputeTrackAdvancement(heats, AdvancementRule{TopN: 1, FastestK: 1})
	if tie != nil {
		t.Fatalf("unexpected tie: %+v", tie)
	}
	for _, a := range advanced {
		if a.EntryID == "dnf1" {
			t.Fatal("a DNF entry must never qualify by time")
		}
	}
	found := false
	for _, a := range advanced {
		if a.EntryID == "ok2" && a.Code == StatusQt {
			found = true
		}
	}
	if !found {
		t.Error("expected the next-fastest legitimate finisher (ok2) to qualify by time")
	}
}

// TestFieldAdvancementCapacityZero is a denial/edge-path test: capacity
// already exhausted by standard-qualifiers leaves nothing for the by-mark
// channel.
func TestFieldAdvancementCapacityZero(t *testing.T) {
	entries := []FieldFinish{
		{EntryID: "a", Mark: "16.00"},
		{EntryID: "b", Mark: "15.00"},
		{EntryID: "c", Mark: "10.00"},
	}
	advanced, tie := ComputeFieldAdvancement(entries, "14.00", "higher", 2)
	if tie != nil {
		t.Fatalf("unexpected tie: %+v", tie)
	}
	if len(advanced) != 2 {
		t.Fatalf("expected exactly the 2 standard-qualifiers, got %d: %+v", len(advanced), advanced)
	}
	for _, a := range advanced {
		if a.Code != StatusQ {
			t.Errorf("%s: expected Q, got %q", a.EntryID, a.Code)
		}
	}
}
