// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package domain

import (
	"testing"
	"time"
)

func mustSwissScheme(t *testing.T) *CategoryScheme {
	t.Helper()
	s, err := BuiltinCategoryScheme(SchemeSwissAthletics)
	if err != nil {
		t.Fatalf("load swiss-athletics scheme: %v", err)
	}
	return s
}

// TestResolveDefaultCategory_SwissAthletics_UC002_1 covers UC-002 #1: given a
// new meet in year 2027, an athlete born 2012 resolves to U16, and one born
// 1990 resolves to Men/Women, per the WO 2026 birth-year windows (D4.2).
func TestResolveDefaultCategory_SwissAthletics_UC002_1(t *testing.T) {
	s := mustSwissScheme(t)
	asOf := time.Date(2027, time.June, 1, 0, 0, 0, 0, time.UTC)

	got, err := s.ResolveDefaultCategory(2012, SexMale, asOf)
	if err != nil {
		t.Fatalf("resolve 2012-born male: %v", err)
	}
	if got.Code != "U16 M" {
		t.Fatalf("2012-born male in 2027: got %q, want U16 M", got.Code)
	}

	got, err = s.ResolveDefaultCategory(1990, SexFemale, asOf)
	if err != nil {
		t.Fatalf("resolve 1990-born female: %v", err)
	}
	if got.Code != "Women" {
		t.Fatalf("1990-born female in 2027: got %q, want Women (elective Masters band must not be the default)", got.Code)
	}
}

// TestResolveDefaultCategory_CalendarYearTransition covers UC-002 #2:
// category assignment transitions by calendar year — an athlete U16 on 31
// Dec is U18 on 1 Jan when crossing the bound.
func TestResolveDefaultCategory_CalendarYearTransition(t *testing.T) {
	s := mustSwissScheme(t)
	birthYear := 2011 // age 15 in 2026, age 16 in 2027

	dec31, _ := s.ResolveDefaultCategory(birthYear, SexMale, time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC))
	if dec31.Code != "U16 M" {
		t.Fatalf("31 Dec 2026: got %q, want U16 M", dec31.Code)
	}

	jan1, _ := s.ResolveDefaultCategory(birthYear, SexMale, time.Date(2027, time.January, 1, 0, 0, 0, 0, time.UTC))
	if jan1.Code != "U18 M" {
		t.Fatalf("1 Jan 2027: got %q, want U18 M", jan1.Code)
	}
}

// TestEvaluateEntry_StartUpAndDisciplineBar covers UC-002 #3: a U14 athlete
// entered in a U16 event where the scheme permits starting up is accepted
// and marked "started up"; the same athlete entering a discipline barred for
// U16-and-younger (D4.3) is rejected with the rule shown.
func TestEvaluateEntry_StartUpAndDisciplineBar(t *testing.T) {
	s := mustSwissScheme(t)
	asOf := time.Date(2027, time.June, 1, 0, 0, 0, 0, time.UTC)

	// Birth year 2015 -> age 12 in 2027, the nominal U14 band (12-13, D4.2).
	defaultCat, err := s.ResolveDefaultCategory(2015, SexMale, asOf)
	if err != nil {
		t.Fatalf("resolve U14 default: %v", err)
	}
	if defaultCat.Code != "U14 M" {
		t.Fatalf("expected default U14 M, got %q", defaultCat.Code)
	}

	u16, ok := s.CategoryByCode("U16 M")
	if !ok {
		t.Fatal("scheme missing U16 M category")
	}
	decision := s.EvaluateEntry(defaultCat, u16)
	if !decision.Allowed || !decision.StartedUp {
		t.Fatalf("U14 M entering U16 M event: got %+v, want allowed+startedUp", decision)
	}

	allowed, reason := s.CheckDisciplineEligibility("400mH", defaultCat.Code)
	if allowed {
		t.Fatal("U14 M entering 400mH: want barred per D4.3, got allowed")
	}
	if reason == "" {
		t.Fatal("barred entry must carry a cited reason (\"the rule shown\", UC-002 #3)")
	}
}

// TestEvaluateEntry_RejectsStartUpWhenNotPermitted exercises the negative
// path of EvaluateEntry for a scheme that does not permit starting up.
func TestEvaluateEntry_RejectsStartUpWhenNotPermitted(t *testing.T) {
	scheme := &CategoryScheme{
		ID:      "no-start-up",
		Version: "1",
		Categories: []Category{
			{Code: "A", Sex: SexMale, MinAge: 0, MaxAge: intPtr(9), Rank: 1, StartUpAllowed: false},
			{Code: "B", Sex: SexMale, MinAge: 10, MaxAge: intPtr(19), Rank: 2, StartUpAllowed: false},
		},
	}
	a, _ := scheme.CategoryByCode("A")
	b, _ := scheme.CategoryByCode("B")
	decision := scheme.EvaluateEntry(a, b)
	if decision.Allowed {
		t.Fatalf("expected start-up to be rejected when StartUpAllowed is false, got %+v", decision)
	}
	if decision.Reason == "" {
		t.Fatal("rejected entry must carry a reason")
	}
}

// TestEvaluateEntry_RejectsCrossSex ensures categories of a different sex
// never match, regardless of rank/age.
func TestEvaluateEntry_RejectsCrossSex(t *testing.T) {
	s := mustSwissScheme(t)
	m, _ := s.CategoryByCode("U16 M")
	w, _ := s.CategoryByCode("U16 W")
	decision := s.EvaluateEntry(m, w)
	if decision.Allowed {
		t.Fatal("expected cross-sex entry to be rejected")
	}
}

// TestParseCategoryScheme_CustomSchemeNoCodeChange covers UC-002 #4: an
// admin-authored custom scheme (arbitrary codes, birth-year windows, up/down
// rules) loads and resolves through the same generic interpreter as the
// built-ins — no code change required.
func TestParseCategoryScheme_CustomSchemeNoCodeChange(t *testing.T) {
	custom := []byte(`{
		"id": "club-custom",
		"version": "2027.1",
		"name": "Club custom scheme",
		"categories": [
			{"code": "Mini", "sex": "M", "minAge": 0, "maxAge": 6, "rank": 1, "startUpAllowed": true},
			{"code": "Junior", "sex": "M", "minAge": 7, "maxAge": 12, "rank": 2, "startUpAllowed": true}
		]
	}`)
	scheme, err := ParseCategoryScheme(custom)
	if err != nil {
		t.Fatalf("parse custom scheme: %v", err)
	}
	got, err := scheme.ResolveDefaultCategory(2023, SexMale, time.Date(2027, time.March, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("resolve in custom scheme: %v", err)
	}
	if got.Code != "Mini" {
		t.Fatalf("custom scheme resolution: got %q, want Mini", got.Code)
	}
}

func TestParseCategoryScheme_RejectsMissingVersion(t *testing.T) {
	_, err := ParseCategoryScheme([]byte(`{"id":"x","categories":[{"code":"A","minAge":0}]}`))
	if err == nil {
		t.Fatal("expected error for scheme without a version identity")
	}
}

func TestParseCategoryScheme_RejectsDuplicateCodes(t *testing.T) {
	_, err := ParseCategoryScheme([]byte(`{
		"id":"x","version":"1",
		"categories":[{"code":"A","minAge":0},{"code":"A","minAge":10}]
	}`))
	if err == nil {
		t.Fatal("expected error for duplicate category codes")
	}
}

func TestParseCategoryScheme_RejectsUnknownRestrictionCategory(t *testing.T) {
	_, err := ParseCategoryScheme([]byte(`{
		"id":"x","version":"1",
		"categories":[{"code":"A","minAge":0}],
		"disciplineRestrictions":[{"disciplineCode":"400mH","barredCategoryCodes":["B"]}]
	}`))
	if err == nil {
		t.Fatal("expected error for a restriction referencing an unknown category code")
	}
}

func TestParseCategoryScheme_RejectsMalformedJSON(t *testing.T) {
	if _, err := ParseCategoryScheme([]byte(`{not json`)); err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestParseCategoryScheme_RejectsEmptyCategories(t *testing.T) {
	if _, err := ParseCategoryScheme([]byte(`{"id":"x","version":"1","categories":[]}`)); err == nil {
		t.Fatal("expected error for a scheme with no categories")
	}
}

func TestParseCategoryScheme_RejectsInvertedAgeRange(t *testing.T) {
	_, err := ParseCategoryScheme([]byte(`{
		"id":"x","version":"1",
		"categories":[{"code":"A","minAge":20,"maxAge":10}]
	}`))
	if err == nil {
		t.Fatal("expected error for maxAge < minAge")
	}
}

// TestUBSKidsCupScheme_PerBirthYearDivisions covers the UKC scheme's
// per-birth-year (M7...M15/W7...W15) divisions (SYS-005, competitive-
// analysis.md §C7.3), independent of Swiss Athletics.
func TestUBSKidsCupScheme_PerBirthYearDivisions(t *testing.T) {
	s, err := BuiltinCategoryScheme(SchemeUBSKidsCup)
	if err != nil {
		t.Fatalf("load ubs-kids-cup scheme: %v", err)
	}
	asOf := time.Date(2026, time.July, 4, 0, 0, 0, 0, time.UTC) // observed field date

	got, err := s.ResolveDefaultCategory(2014, SexFemale, asOf) // age 12
	if err != nil {
		t.Fatalf("resolve W12: %v", err)
	}
	if got.Code != "W12" {
		t.Fatalf("2014-born female in 2026: got %q, want W12", got.Code)
	}

	// The open/mixed "UKC for all" division is elective: never the default,
	// but present among eligible categories at any UKC age.
	eligible := s.EligibleCategories(2014, SexFemale, asOf)
	found := false
	for _, c := range eligible {
		if c.Code == "UKC for all" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected \"UKC for all\" among eligible categories")
	}
}

func TestUBSKidsCupScheme_LoadsAndValidates(t *testing.T) {
	if _, err := BuiltinCategoryScheme(SchemeUBSKidsCup); err != nil {
		t.Fatalf("ubs-kids-cup scheme must load and validate: %v", err)
	}
}

func TestBuiltinCategoryScheme_UnknownID(t *testing.T) {
	if _, err := BuiltinCategoryScheme("does-not-exist"); err == nil {
		t.Fatal("expected error for unknown built-in scheme id")
	}
}

func TestBuiltinCategorySchemes_LoadsAll(t *testing.T) {
	all, err := BuiltinCategorySchemes()
	if err != nil {
		t.Fatalf("load all built-in schemes: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 built-in schemes, got %d", len(all))
	}
}

func TestCategory_InAgeRangeAndMatchesSex(t *testing.T) {
	c := Category{Sex: SexMale, MinAge: 10, MaxAge: intPtr(19)}
	if c.InAgeRange(9) || !c.InAgeRange(10) || !c.InAgeRange(19) || c.InAgeRange(20) {
		t.Fatal("InAgeRange bounds incorrect")
	}
	if !c.MatchesSex(SexMale) || c.MatchesSex(SexFemale) {
		t.Fatal("MatchesSex incorrect for sex-specific category")
	}
	open := Category{Sex: ""}
	if !open.MatchesSex(SexMale) || !open.MatchesSex(SexFemale) {
		t.Fatal("MatchesSex incorrect for sex-agnostic category")
	}
}

func intPtr(v int) *int { return &v }

func TestCategoryByCode_NotFound(t *testing.T) {
	s := mustSwissScheme(t)
	if _, ok := s.CategoryByCode("does-not-exist"); ok {
		t.Fatal("expected no category for an unknown code")
	}
}

func TestParseCategoryScheme_RejectsEmptyID(t *testing.T) {
	_, err := ParseCategoryScheme([]byte(`{"version":"1","categories":[{"code":"A","minAge":0}]}`))
	if err == nil {
		t.Fatal("expected error for a scheme without an id")
	}
}

func TestParseCategoryScheme_RejectsEmptyCategoryCode(t *testing.T) {
	_, err := ParseCategoryScheme([]byte(`{"id":"x","version":"1","categories":[{"code":"","minAge":0}]}`))
	if err == nil {
		t.Fatal("expected error for a category with an empty code")
	}
}

// TestResolveDefaultCategory_NoMatch covers the "no category fits this
// age/sex" error path (e.g. an age outside every band in the scheme).
func TestResolveDefaultCategory_NoMatch(t *testing.T) {
	scheme := &CategoryScheme{
		ID:      "narrow",
		Version: "1",
		Categories: []Category{
			{Code: "A", Sex: SexMale, MinAge: 10, MaxAge: intPtr(12), Rank: 1},
		},
	}
	if _, err := scheme.ResolveDefaultCategory(2000, SexMale, time.Date(2027, time.January, 1, 0, 0, 0, 0, time.UTC)); err == nil {
		t.Fatal("expected an error when no category matches the resolved age")
	}
}

// TestResolveDefaultCategory_SkipsElectiveEvenOutOfOrder covers the elective
// "continue" branch directly: an elective category ranked ahead of a
// matching non-elective one must be skipped by default resolution.
func TestResolveDefaultCategory_SkipsElectiveEvenOutOfOrder(t *testing.T) {
	scheme := &CategoryScheme{
		ID:      "elective-first",
		Version: "1",
		Categories: []Category{
			{Code: "Elective", Sex: SexMale, MinAge: 0, MaxAge: intPtr(99), Rank: 1, Elective: true},
			{Code: "Default", Sex: SexMale, MinAge: 0, MaxAge: intPtr(99), Rank: 2},
		},
	}
	got, err := scheme.ResolveDefaultCategory(2000, SexMale, time.Date(2027, time.January, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got.Code != "Default" {
		t.Fatalf("expected the elective category to be skipped, got %q", got.Code)
	}
}

// TestEvaluateEntry_SameCategoryAndStartDown covers the "already in this
// category" fast path and the permitted start-down path (e.g. a Masters
// athlete starting down into the open Men category, WO §1.2).
func TestEvaluateEntry_SameCategoryAndStartDown(t *testing.T) {
	s := mustSwissScheme(t)
	men, _ := s.CategoryByCode("Men")
	m35, _ := s.CategoryByCode("M35")

	same := s.EvaluateEntry(men, men)
	if !same.Allowed || same.StartedUp || same.StartedDown {
		t.Fatalf("entering one's own category: got %+v", same)
	}

	down := s.EvaluateEntry(m35, men)
	if !down.Allowed || !down.StartedDown {
		t.Fatalf("M35 starting down into Men: got %+v", down)
	}
}

// TestEvaluateEntry_RejectsStartDownWhenNotPermitted covers the negative
// start-down path.
func TestEvaluateEntry_RejectsStartDownWhenNotPermitted(t *testing.T) {
	scheme := &CategoryScheme{
		ID:      "no-start-down",
		Version: "1",
		Categories: []Category{
			{Code: "A", Sex: SexMale, MinAge: 0, MaxAge: intPtr(9), Rank: 1, StartDownAllowed: false},
			{Code: "B", Sex: SexMale, MinAge: 10, MaxAge: intPtr(19), Rank: 2, StartDownAllowed: false},
		},
	}
	a, _ := scheme.CategoryByCode("A")
	b, _ := scheme.CategoryByCode("B")
	decision := scheme.EvaluateEntry(b, a)
	if decision.Allowed {
		t.Fatalf("expected start-down to be rejected when StartDownAllowed is false, got %+v", decision)
	}
}

// TestEvaluateEntry_SameRankDifferentCode covers the conservative
// same-rank/different-code rejection branch.
func TestEvaluateEntry_SameRankDifferentCode(t *testing.T) {
	scheme := &CategoryScheme{
		ID:      "same-rank",
		Version: "1",
		Categories: []Category{
			{Code: "A", Sex: SexMale, MinAge: 0, MaxAge: intPtr(9), Rank: 1},
			{Code: "B", Sex: SexMale, MinAge: 0, MaxAge: intPtr(9), Rank: 1},
		},
	}
	a, _ := scheme.CategoryByCode("A")
	b, _ := scheme.CategoryByCode("B")
	decision := scheme.EvaluateEntry(a, b)
	if decision.Allowed {
		t.Fatalf("expected same-rank/different-code entry to be rejected, got %+v", decision)
	}
	if decision.Reason == "" {
		t.Fatal("rejected entry must carry a reason")
	}
}

// TestRankedCategories_SortsOutOfOrderInput covers rankedCategories' swap
// path directly: an input list not already in rank order must still sort
// correctly (a scheme author has no obligation to list categories in rank
// order).
func TestRankedCategories_SortsOutOfOrderInput(t *testing.T) {
	unsorted := []Category{
		{Code: "C", Rank: 3},
		{Code: "A", Rank: 1},
		{Code: "B", Rank: 2},
	}
	sorted := rankedCategories(unsorted)
	got := []string{sorted[0].Code, sorted[1].Code, sorted[2].Code}
	want := []string{"A", "B", "C"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("rankedCategories order = %v, want %v", got, want)
		}
	}
}
