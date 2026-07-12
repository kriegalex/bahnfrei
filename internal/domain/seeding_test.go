// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package domain

import "testing"

func TestBuiltinSeedingRulesTR20SYS026(t *testing.T) {
	r, err := BuiltinSeedingRules(SeedingRulesTR20)
	if err != nil {
		t.Fatalf("load built-in TR20 seeding rules: %v", err)
	}
	if r.Distribution != DistributionSerpentine {
		t.Errorf("expected serpentine distribution, got %q", r.Distribution)
	}
	if !r.ClubSeparation {
		t.Error("expected club separation to be enabled (D2.2)")
	}
	if !r.IsLaneRace("400m") || !r.IsLaneRace("800m") {
		t.Error("expected 400m and 800m to be classified as lane races (D2.3)")
	}
	if r.IsLaneRace("5000m") {
		t.Error("expected 5000m not to be classified as a grouped lane race")
	}
	if _, ok := r.LaneGroupsFor(8); !ok {
		t.Error("expected an 8-lane group table")
	}
	if _, ok := r.LaneGroupsFor(6); ok {
		t.Error("did not expect a 6-lane group table (not shipped)")
	}
}

func TestBuiltinSeedingRulesUnknownID(t *testing.T) {
	if _, err := BuiltinSeedingRules("does-not-exist"); err == nil {
		t.Fatal("expected an error for an unknown seeding rules id")
	}
}

func TestBuiltinSeedingRules_ReadFileError(t *testing.T) {
	orig := builtinSeedingRulesFiles[SeedingRulesTR20]
	builtinSeedingRulesFiles[SeedingRulesTR20] = "data/seeding/does-not-exist.json"
	defer func() { builtinSeedingRulesFiles[SeedingRulesTR20] = orig }()

	if _, err := BuiltinSeedingRules(SeedingRulesTR20); err == nil {
		t.Fatal("expected an error when the embedded seeding-rules file is missing")
	}
}

func TestParseSeedingRulesRejectsMissingID(t *testing.T) {
	_, err := ParseSeedingRules([]byte(`{"version":"1","distribution":"serpentine"}`))
	if err == nil {
		t.Fatal("expected an error for a missing id")
	}
}

func TestParseSeedingRulesRejectsMissingVersion(t *testing.T) {
	_, err := ParseSeedingRules([]byte(`{"id":"x","distribution":"serpentine"}`))
	if err == nil {
		t.Fatal("expected an error for a missing version")
	}
}

func TestParseSeedingRulesRejectsUnsupportedDistribution(t *testing.T) {
	_, err := ParseSeedingRules([]byte(`{"id":"x","version":"1","distribution":"snake-draft"}`))
	if err == nil {
		t.Fatal("expected an error for an unsupported distribution method")
	}
}

func TestParseSeedingRulesRejectsMismatchedGroupSize(t *testing.T) {
	data := []byte(`{"id":"x","version":"1","distribution":"serpentine",
		"laneGroups":{"8":[{"rankFrom":1,"rankTo":4,"lanes":[4,5,6]}]}}`)
	if _, err := ParseSeedingRules(data); err == nil {
		t.Fatal("expected an error when a lane group's lane count does not match its rank span")
	}
}

func TestParseSeedingRulesRejectsDuplicateLane(t *testing.T) {
	data := []byte(`{"id":"x","version":"1","distribution":"serpentine",
		"laneGroups":{"8":[
			{"rankFrom":1,"rankTo":2,"lanes":[4,5]},
			{"rankFrom":3,"rankTo":4,"lanes":[5,6]}
		]}}`)
	if _, err := ParseSeedingRules(data); err == nil {
		t.Fatal("expected an error when a lane is reused across groups")
	}
}

func TestParseSeedingRulesRejectsInvalidRankRange(t *testing.T) {
	data := []byte(`{"id":"x","version":"1","distribution":"serpentine",
		"laneGroups":{"8":[{"rankFrom":4,"rankTo":1,"lanes":[]}]}}`)
	if _, err := ParseSeedingRules(data); err == nil {
		t.Fatal("expected an error for rankTo < rankFrom")
	}
}

func TestUnitAssignmentValidateSYS026(t *testing.T) {
	valid := UnitAssignment{ID: "a1", UnitID: "u1", EntryID: "e1"}
	if err := valid.Validate(); err != nil {
		t.Errorf("expected a minimal unit assignment to validate, got %v", err)
	}
	missingUnit := UnitAssignment{ID: "a1", EntryID: "e1"}
	if err := missingUnit.Validate(); err == nil {
		t.Error("expected an error for a missing unit id")
	}
	missingEntry := UnitAssignment{ID: "a1", UnitID: "u1"}
	if err := missingEntry.Validate(); err == nil {
		t.Error("expected an error for a missing entry id")
	}
}
