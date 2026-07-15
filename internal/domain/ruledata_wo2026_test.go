// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package domain

import (
	"math/rand"
	"testing"
	"time"
)

func mustDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("parse date %q: %v", s, err)
	}
	return d
}

// These fixtures pin the rule data verified against the primary Swiss
// Athletics WO 2026 and WA CR&TR 2026 PDFs on 2026-07-15 (OQ-017, OQ-018,
// OQ-019, OQ-034 closures — see
// docs/requirements/open-questions-and-assumptions.md).

// TestSwissSchemeYouthSprintBarsWO15e pins WO 2026 §1.5e (SYS-005/SYS-014):
// U16-and-younger may not start in 150m/200m/300m/400m/300mH/400mH; U18 may.
// Relay codes are distinct disciplines and stay unbarred (the rule's SM
// Staffel exception).
func TestSwissSchemeYouthSprintBarsWO15e(t *testing.T) {
	s, err := BuiltinCategoryScheme("swiss-athletics")
	if err != nil {
		t.Fatalf("load swiss-athletics scheme: %v", err)
	}
	for _, disc := range []string{"150m", "200m", "300m", "400m", "300mH", "400mH"} {
		for _, cat := range []string{"U10 W", "U12 M", "U14 W", "U16 M", "U16 W"} {
			if ok, _ := s.CheckDisciplineEligibility(disc, cat); ok {
				t.Errorf("%s / %s: want barred per WO 2026 §1.5e, got allowed", disc, cat)
			}
		}
		for _, cat := range []string{"U18 M", "U18 W", "U20 W", "Men"} {
			if ok, citation := s.CheckDisciplineEligibility(disc, cat); !ok {
				t.Errorf("%s / %s: want allowed, got barred (%s)", disc, cat, citation)
			}
		}
	}
	// Steeplechase (§1.5c) is barred for U10/U12/U14 at every steeple
	// distance, but not for U16 and older.
	for _, disc := range []string{"3000mSC", "2000mSC", "1500mSC"} {
		if ok, _ := s.CheckDisciplineEligibility(disc, "U14 M"); ok {
			t.Errorf("%s / U14 M: want barred per WO 2026 §1.5c, got allowed", disc)
		}
		if ok, _ := s.CheckDisciplineEligibility(disc, "U16 W"); !ok {
			t.Errorf("%s / U16 W: want allowed (§1.5c stops at U14), got barred", disc)
		}
	}
	// The SM Staffel exception: relays are not barred as disciplines.
	if ok, _ := s.CheckDisciplineEligibility("4x100m", "U16 W"); !ok {
		t.Error("4x100m / U16 W: relay codes must not be barred (WO §1.5e exception: SM Staffel)")
	}
}

// TestSwissSchemeMastersM30WO11 pins the WO 2026 §1.1 M30/W30 band (30-34)
// the scheme previously carried without start-down permission: it is
// elective (never a default), reachable by age, and admits older athletes
// starting down (§1.2a).
func TestSwissSchemeMastersM30WO11(t *testing.T) {
	s, err := BuiltinCategoryScheme("swiss-athletics")
	if err != nil {
		t.Fatalf("load swiss-athletics scheme: %v", err)
	}
	m30, ok := s.CategoryByCode("M30")
	if !ok {
		t.Fatal("scheme must define M30 (WO 2026 §1.1)")
	}
	if !m30.Elective || !m30.StartDownAllowed || m30.StartUpAllowed {
		t.Errorf("M30 flags = elective:%v startDown:%v startUp:%v, want elective, start-down only (WO §1.1 Startberechtigung '30-jährig und älter')", m30.Elective, m30.StartDownAllowed, m30.StartUpAllowed)
	}
	// A 32-year-old's default stays Men/Women; M30 is in the eligible set.
	def, err := s.ResolveDefaultCategory(1994, SexFemale, mustDate(t, "2026-07-15"))
	if err != nil {
		t.Fatalf("ResolveDefaultCategory: %v", err)
	}
	if def.Code != "Women" {
		t.Errorf("32-year-old default = %q, want Women (M30 is elective)", def.Code)
	}
	var hasW30 bool
	for _, c := range s.EligibleCategories(1994, SexFemale, mustDate(t, "2026-07-15")) {
		if c.Code == "W30" {
			hasW30 = true
		}
	}
	if !hasW30 {
		t.Error("32-year-old's eligible categories must include W30")
	}
}

// TestCatalogYouthVariantsWO2026 pins the OQ-019 closure (SYS-003, UC-002
// #5): the previously unpopulated U20/U18/U16/U14 hurdle variants and the
// youth implement masses, all per WO 2026 §8.1.1-§8.1.3.
func TestCatalogYouthVariantsWO2026(t *testing.T) {
	c, err := BuiltinDisciplineCatalog()
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	wantHurdle := []struct {
		disc, cat string
		height    float64 // 0 = height must be absent (meet-defined)
		count     int
	}{
		{"110mH", "U20 M", 0.991, 10},
		{"110mH", "U18 M", 0.914, 10},
		{"100mH", "U20 W", 0.838, 10},
		{"100mH", "U18 W", 0.762, 10},
		{"100mH", "U16 M", 0.838, 10},
		{"80mH", "U16 W", 0.762, 8},
		{"80mH", "U14 M", 0.762, 8},
		{"60mH", "U14 W", 0.762, 6},
		{"60mH", "U12 W", 0, 6},
		{"300mH", "U18 M", 0.838, 7},
		{"300mH", "U20 W", 0.762, 7},
		{"400mH", "U18 M", 0.838, 10},
		{"2000mSC", "U18 M", 0.838, 0},
		{"1500mSC", "U18 W", 0.762, 0},
	}
	for _, w := range wantHurdle {
		d, ok := c.ByCode(w.disc)
		if !ok {
			t.Errorf("catalog is missing discipline %s", w.disc)
			continue
		}
		v, ok := d.VariantFor(w.cat)
		if !ok {
			t.Errorf("%s has no %s variant (WO 2026 §8.1.1-§8.1.3)", w.disc, w.cat)
			continue
		}
		if w.height == 0 {
			if w.disc == "60mH" && v.HurdleHeightM != nil {
				t.Errorf("%s/%s: height must be meet-defined (nil), got %v", w.disc, w.cat, *v.HurdleHeightM)
			}
		} else if v.HurdleHeightM == nil || *v.HurdleHeightM != w.height {
			t.Errorf("%s/%s hurdle height = %v, want %v", w.disc, w.cat, v.HurdleHeightM, w.height)
		}
		if w.count != 0 && (v.HurdleCount == nil || *v.HurdleCount != w.count) {
			t.Errorf("%s/%s hurdle count = %v, want %d", w.disc, w.cat, v.HurdleCount, w.count)
		}
	}
	wantMass := []struct {
		disc, cat string
		kg        float64
	}{
		{"SP", "U16 M", 4.00}, {"SP", "U14 W", 3.00}, {"SP", "U12 M", 2.50},
		{"DT", "U16 W", 0.75}, {"DT", "U14 M", 0.75},
		{"HT", "U16 M", 4.00}, {"HT", "U14 W", 3.00},
		{"JT", "U16 W", 0.400}, {"JT", "U16 M", 0.600}, {"JT", "U12 M", 0.400},
	}
	for _, w := range wantMass {
		d, _ := c.ByCode(w.disc)
		v, ok := d.VariantFor(w.cat)
		if !ok || v.ImplementMassKg == nil || *v.ImplementMassKg != w.kg {
			t.Errorf("%s/%s implement mass: got %+v, want %v kg (WO 2026 §8.1.1/§8.1.3)", w.disc, w.cat, v.ImplementMassKg, w.kg)
		}
	}
}

// TestLaneGroupClassesTR204 pins the TR 20.4.3-20.4.5 event-class lane
// tables (SYS-027, OQ-034 closure): straight races, 200m/300m races and
// 400m-class races draw from different rank-group → lane-set tables, on
// both 8- and 9-lane tracks; a discipline in no class falls back to the
// default (400m-class) table.
func TestLaneGroupClassesTR204(t *testing.T) {
	rules := mustSeedingRules(t)
	cases := []struct {
		disc      string
		laneCount int
		topLanes  []int
	}{
		{"100m", 8, []int{3, 4, 5, 6}},  // TR 20.4.3
		{"110mH", 8, []int{3, 4, 5, 6}}, // straight hurdles
		{"100m", 9, []int{4, 5, 6}},     // TR 20.4.3, 9 lanes
		{"200m", 8, []int{5, 6, 7}},     // TR 20.4.4
		{"300m", 9, []int{5, 6, 7, 8}},  // TR 20.4.4, 9 lanes
		{"400m", 8, []int{4, 5, 6, 7}},  // TR 20.4.5 (default table, UC-008 #3)
		{"800m", 9, []int{5, 6, 7}},     // TR 20.4.5, 9 lanes
		{"1500m", 8, []int{4, 5, 6, 7}}, // unmapped: default table fallback
	}
	for _, c := range cases {
		groups, ok := rules.LaneGroupsForDiscipline(c.disc, c.laneCount)
		if !ok {
			t.Errorf("%s/%d lanes: no group table", c.disc, c.laneCount)
			continue
		}
		top := groups[0]
		if top.RankFrom != 1 {
			t.Errorf("%s/%d lanes: first group starts at rank %d, want 1", c.disc, c.laneCount, top.RankFrom)
			continue
		}
		if !sameLaneSet(top.Lanes, c.topLanes) {
			t.Errorf("%s/%d lanes: top-group lanes = %v, want %v", c.disc, c.laneCount, top.Lanes, c.topLanes)
		}
		// Every table must be drawable for a full field.
		field := make([]string, c.laneCount)
		for i := range field {
			field[i] = string(rune('a' + i))
		}
		if _, err := DrawLanesGrouped(field, groups, rand.New(rand.NewSource(1))); err != nil {
			t.Errorf("%s/%d lanes: draw failed: %v", c.disc, c.laneCount, err)
		}
	}
}

func sameLaneSet(got, want []int) bool {
	if len(got) != len(want) {
		return false
	}
	set := map[int]bool{}
	for _, l := range got {
		set[l] = true
	}
	for _, l := range want {
		if !set[l] {
			return false
		}
	}
	return true
}
