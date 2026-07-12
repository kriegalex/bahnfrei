// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package domain

import (
	"testing"
	"time"
)

func mustStadiumCatalog(t *testing.T) *DisciplineCatalog {
	t.Helper()
	c, err := BuiltinDisciplineCatalog()
	if err != nil {
		t.Fatalf("load built-in discipline catalog: %v", err)
	}
	return c
}

func hasFlag(res EligibilityResult, code string) (EligibilityFlag, bool) {
	for _, f := range res.Flags {
		if f.Code == code {
			return f, true
		}
	}
	return EligibilityFlag{}, false
}

// TestEvaluateEligibility_EligibleWhenNoViolationsSYS014UC005 covers the
// baseline positive path: a licensed athlete entering their own default
// category/discipline at a licence-required tier is eligible with no flags.
func TestEvaluateEligibility_EligibleWhenNoViolationsSYS014UC005(t *testing.T) {
	scheme := mustSwissScheme(t)
	catalog := mustStadiumCatalog(t)
	asOf := time.Date(2027, time.June, 1, 0, 0, 0, 0, time.UTC)

	res := EvaluateEligibility(scheme, catalog, EligibilityInput{
		BirthYear: 1995, Sex: SexMale, TargetCategoryCode: "Men", DisciplineCode: "100m",
		HasLicence: true, MeetTier: "B-Meeting", AsOf: asOf,
	})
	if res.Outcome != EligibilityEligible {
		t.Fatalf("expected eligible, got %q with flags %+v", res.Outcome, res.Flags)
	}
	if len(res.Flags) != 0 {
		t.Fatalf("expected no flags, got %+v", res.Flags)
	}
}

// TestEvaluateEligibility_LicenceMissingBlockedSYS014UC005_1 covers UC-005
// #1: a licence-required meet tier (Swiss B-Meeting) with no licence number
// is flagged licence-missing before start-list generation.
func TestEvaluateEligibility_LicenceMissingBlockedSYS014UC005_1(t *testing.T) {
	scheme := mustSwissScheme(t)
	catalog := mustStadiumCatalog(t)
	asOf := time.Date(2027, time.June, 1, 0, 0, 0, 0, time.UTC)

	res := EvaluateEligibility(scheme, catalog, EligibilityInput{
		BirthYear: 1995, Sex: SexMale, TargetCategoryCode: "Men", DisciplineCode: "100m",
		HasLicence: false, MeetTier: "B-Meeting", AsOf: asOf,
	})
	if res.Outcome != EligibilityBlocked {
		t.Fatalf("expected blocked, got %q", res.Outcome)
	}
	if _, ok := hasFlag(res, FlagLicenceMissing); !ok {
		t.Fatalf("expected %s flag, got %+v", FlagLicenceMissing, res.Flags)
	}
}

// TestEvaluateEligibility_LicenceMissingWarningAtCMeeting covers the
// C-Meeting case (D1.2): unlicensed athletes may compete, but a warning is
// raised since their results are excluded from ranking lists — not a block.
func TestEvaluateEligibility_LicenceMissingWarningAtCMeeting(t *testing.T) {
	scheme := mustSwissScheme(t)
	catalog := mustStadiumCatalog(t)
	asOf := time.Date(2027, time.June, 1, 0, 0, 0, 0, time.UTC)

	res := EvaluateEligibility(scheme, catalog, EligibilityInput{
		BirthYear: 1995, Sex: SexMale, TargetCategoryCode: "Men", DisciplineCode: "100m",
		HasLicence: false, MeetTier: "C-Meeting", AsOf: asOf,
	})
	if res.Outcome != EligibilityWarning {
		t.Fatalf("expected warning at C-Meeting, got %q with flags %+v", res.Outcome, res.Flags)
	}
	if _, ok := hasFlag(res, FlagLicenceMissingWarn); !ok {
		t.Fatalf("expected %s flag, got %+v", FlagLicenceMissingWarn, res.Flags)
	}
}

// TestEvaluateEligibility_YouthMaxDistanceBlockedSYS014UC005_2 covers UC-005
// #2 verbatim: an athlete born 2015 (U12 in 2026) entered for 3000m stadium
// (max 2000m per D4.3) is flagged with the youth-protection rule reference.
func TestEvaluateEligibility_YouthMaxDistanceBlockedSYS014UC005_2(t *testing.T) {
	scheme := mustSwissScheme(t)
	catalog := mustStadiumCatalog(t)
	asOf := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)

	// Sanity: birth year 2015 resolves to U12 M in 2026 (age 11).
	def, err := scheme.ResolveDefaultCategory(2015, SexMale, asOf)
	if err != nil || def.Code != "U12 M" {
		t.Fatalf("expected 2015-born male to resolve U12 M in 2026, got %+v (err %v)", def, err)
	}

	res := EvaluateEligibility(scheme, catalog, EligibilityInput{
		BirthYear: 2015, Sex: SexMale, TargetCategoryCode: "U12 M", DisciplineCode: "3000m",
		HasLicence: true, MeetTier: "C-Meeting", AsOf: asOf,
	})
	if res.Outcome != EligibilityBlocked {
		t.Fatalf("expected blocked, got %q with flags %+v", res.Outcome, res.Flags)
	}
	flag, ok := hasFlag(res, FlagYouthMaxDistance)
	if !ok {
		t.Fatalf("expected %s flag, got %+v", FlagYouthMaxDistance, res.Flags)
	}
	if flag.Citation == "" {
		t.Fatal("youth-protection flag must carry the D4.3 rule citation (UC-005 #2 \"flagged with the youth-protection rule reference\")")
	}
}

// TestEvaluateEligibility_YouthOneRacePerDayBlockedSYS014UC005_3 covers
// UC-005 #3: a U12 athlete already entered for a 600m-equivalent race is
// flagged when a second race at/above the 600m threshold is added.
func TestEvaluateEligibility_YouthOneRacePerDayBlockedSYS014UC005_3(t *testing.T) {
	scheme := mustSwissScheme(t)
	catalog := mustStadiumCatalog(t)
	asOf := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)

	res := EvaluateEligibility(scheme, catalog, EligibilityInput{
		BirthYear: 2015, Sex: SexMale, TargetCategoryCode: "U12 M", DisciplineCode: "800m",
		HasLicence: true, MeetTier: "C-Meeting", AsOf: asOf,
		OtherRaceDistancesM: []int{600},
	})
	if res.Outcome != EligibilityBlocked {
		t.Fatalf("expected blocked, got %q with flags %+v", res.Outcome, res.Flags)
	}
	if _, ok := hasFlag(res, FlagYouthOneRacePerDay); !ok {
		t.Fatalf("expected %s flag, got %+v", FlagYouthOneRacePerDay, res.Flags)
	}
}

// TestEvaluateEligibility_YouthOneRacePerDayAllowsFirstRace confirms the
// one-race cap does not fire on the athlete's first ≥600m race of the day
// (no OtherRaceDistancesM yet) — only a *second* one is a violation.
func TestEvaluateEligibility_YouthOneRacePerDayAllowsFirstRace(t *testing.T) {
	scheme := mustSwissScheme(t)
	catalog := mustStadiumCatalog(t)
	asOf := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)

	res := EvaluateEligibility(scheme, catalog, EligibilityInput{
		BirthYear: 2015, Sex: SexMale, TargetCategoryCode: "U12 M", DisciplineCode: "800m",
		HasLicence: true, MeetTier: "C-Meeting", AsOf: asOf,
	})
	if res.Outcome != EligibilityEligible {
		t.Fatalf("first ≥600m race of the day: expected eligible, got %q with flags %+v", res.Outcome, res.Flags)
	}
}

// TestEvaluateEligibility_BarredDisciplineBlockedSYS014UC005 exercises the
// D4.3 barred-discipline path within the eligibility engine (already unit
// tested directly on CategoryScheme; this proves it composes correctly here).
func TestEvaluateEligibility_BarredDisciplineBlockedSYS014UC005(t *testing.T) {
	scheme := mustSwissScheme(t)
	catalog := mustStadiumCatalog(t)
	asOf := time.Date(2027, time.June, 1, 0, 0, 0, 0, time.UTC)

	res := EvaluateEligibility(scheme, catalog, EligibilityInput{
		BirthYear: 2015, Sex: SexMale, TargetCategoryCode: "U14 M", DisciplineCode: "400mH",
		HasLicence: true, MeetTier: "C-Meeting", AsOf: asOf,
	})
	if res.Outcome != EligibilityBlocked {
		t.Fatalf("expected blocked, got %q", res.Outcome)
	}
	if _, ok := hasFlag(res, FlagBarredDiscipline); !ok {
		t.Fatalf("expected %s flag, got %+v", FlagBarredDiscipline, res.Flags)
	}
}

// TestEvaluateEligibility_CategoryMismatchBlocked covers the category
// start-up/down composition path: a category the scheme does not permit
// starting into is blocked with a category_mismatch flag.
func TestEvaluateEligibility_CategoryMismatchBlocked(t *testing.T) {
	scheme := &CategoryScheme{
		ID: "no-start-up", Version: "1",
		Categories: []Category{
			{Code: "A", Sex: SexMale, MinAge: 0, MaxAge: intPtr(9), Rank: 1},
			{Code: "B", Sex: SexMale, MinAge: 10, MaxAge: intPtr(19), Rank: 2},
		},
	}
	asOf := time.Date(2027, time.January, 1, 0, 0, 0, 0, time.UTC)
	res := EvaluateEligibility(scheme, nil, EligibilityInput{
		BirthYear: 2022, Sex: SexMale, TargetCategoryCode: "B", DisciplineCode: "80m",
		HasLicence: true, MeetTier: "C-Meeting", AsOf: asOf,
	})
	if res.Outcome != EligibilityBlocked {
		t.Fatalf("expected blocked, got %q", res.Outcome)
	}
	if _, ok := hasFlag(res, FlagCategoryMismatch); !ok {
		t.Fatalf("expected %s flag, got %+v", FlagCategoryMismatch, res.Flags)
	}
}

// TestEvaluateEligibility_UnresolvedCategoryBlocked covers the edge case
// where no default category resolves for the athlete's age at all.
func TestEvaluateEligibility_UnresolvedCategoryBlocked(t *testing.T) {
	scheme := mustSwissScheme(t)
	asOf := time.Date(2027, time.June, 1, 0, 0, 0, 0, time.UTC)
	res := EvaluateEligibility(scheme, nil, EligibilityInput{
		BirthYear: 3000, Sex: SexMale, TargetCategoryCode: "Men", DisciplineCode: "100m",
		HasLicence: true, MeetTier: "C-Meeting", AsOf: asOf,
	})
	if res.Outcome != EligibilityBlocked {
		t.Fatalf("expected blocked, got %q", res.Outcome)
	}
	if _, ok := hasFlag(res, FlagCategoryUnresolved); !ok {
		t.Fatalf("expected %s flag, got %+v", FlagCategoryUnresolved, res.Flags)
	}
}

// TestEvaluateEligibility_UnknownCategoryBlocked covers an event category
// code the scheme does not define at all.
func TestEvaluateEligibility_UnknownCategoryBlocked(t *testing.T) {
	scheme := mustSwissScheme(t)
	asOf := time.Date(2027, time.June, 1, 0, 0, 0, 0, time.UTC)
	res := EvaluateEligibility(scheme, nil, EligibilityInput{
		BirthYear: 1995, Sex: SexMale, TargetCategoryCode: "Not A Real Category", DisciplineCode: "100m",
		HasLicence: true, MeetTier: "C-Meeting", AsOf: asOf,
	})
	if res.Outcome != EligibilityBlocked {
		t.Fatalf("expected blocked, got %q", res.Outcome)
	}
	if _, ok := hasFlag(res, FlagUnknownCategory); !ok {
		t.Fatalf("expected %s flag, got %+v", FlagUnknownCategory, res.Flags)
	}
}

// TestRequiresLicenceSYS014 covers the D1.2/D3.1 tier table: A/B-Meeting
// require a licence, C-Meeting and unsanctioned/custom tiers do not.
func TestRequiresLicenceSYS014(t *testing.T) {
	cases := []struct {
		tier MeetTier
		want bool
	}{
		{"A-Meeting", true},
		{"B-Meeting", true},
		{"C-Meeting", false},
		{"club meeting", false},
		{"", false},
	}
	for _, c := range cases {
		if got := RequiresLicence(c.tier); got != c.want {
			t.Errorf("RequiresLicence(%q) = %v, want %v", c.tier, got, c.want)
		}
	}
}

// TestTrackDistanceMetersSYS014 covers the discipline-code distance parser
// the youth-protection checks key off.
func TestTrackDistanceMetersSYS014(t *testing.T) {
	cases := []struct {
		code     string
		wantDist int
		wantOK   bool
	}{
		{"800m", 800, true},
		{"3000mSC", 3000, true},
		{"300mH", 300, true},
		{"60m", 60, true},
		{"HJ", 0, false},
		{"SP", 0, false},
		{"", 0, false},
	}
	for _, c := range cases {
		dist, ok := TrackDistanceMeters(c.code)
		if ok != c.wantOK || dist != c.wantDist {
			t.Errorf("TrackDistanceMeters(%q) = (%d, %v), want (%d, %v)", c.code, dist, ok, c.wantDist, c.wantOK)
		}
	}
}
