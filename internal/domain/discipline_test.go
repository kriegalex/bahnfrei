// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package domain

import "testing"

func mustCatalog(t *testing.T) *DisciplineCatalog {
	t.Helper()
	c, err := BuiltinDisciplineCatalog()
	if err != nil {
		t.Fatalf("load built-in discipline catalog: %v", err)
	}
	return c
}

// TestDiscipline_CategoryCorrectTechnicalVariant covers UC-002 #5: listing a
// discipline for two categories carries each category's correct technical
// variant from the built-in data (shot put mass differs U18 W vs Women; the
// UC's own illustrative example, 100mH hurdle specs, is exercised in
// TestDiscipline_HurdleVariants below).
func TestDiscipline_CategoryCorrectTechnicalVariant(t *testing.T) {
	c := mustCatalog(t)
	sp, ok := c.ByCode("SP")
	if !ok {
		t.Fatal("catalog missing SP (Shot Put)")
	}
	women, ok := sp.VariantFor("Women")
	if !ok {
		t.Fatal("SP missing Women variant")
	}
	u18w, ok := sp.VariantFor("U18 W")
	if !ok {
		t.Fatal("SP missing U18 W variant")
	}
	if women.ImplementMassKg == nil || u18w.ImplementMassKg == nil {
		t.Fatal("SP variants must carry an implement mass")
	}
	if *women.ImplementMassKg == *u18w.ImplementMassKg {
		t.Fatalf("SP Women (%.2fkg) and U18 W (%.2fkg) must differ", *women.ImplementMassKg, *u18w.ImplementMassKg)
	}
	if women.Citation == "" || u18w.Citation == "" {
		t.Fatal("technical variants must cite their source")
	}
}

// TestDiscipline_HurdleVariants exercises the UC-002 #5 illustrative example
// directly: 100mH carries a category-correct hurdle height/spacing variant.
func TestDiscipline_HurdleVariants(t *testing.T) {
	c := mustCatalog(t)
	h, ok := c.ByCode("100mH")
	if !ok {
		t.Fatal("catalog missing 100mH")
	}
	v, ok := h.VariantFor("Women")
	if !ok {
		t.Fatal("100mH missing Women variant")
	}
	if v.HurdleHeightM == nil || v.HurdleSpacingM == nil {
		t.Fatal("100mH Women variant must carry hurdle height and spacing")
	}
}

// TestDisciplineCatalog_SYS003Coverage checks the built-in catalog covers
// track, field (horizontal/vertical), relay, and combined-event families
// per SYS-003, and that every discipline declares a known family.
func TestDisciplineCatalog_SYS003Coverage(t *testing.T) {
	c := mustCatalog(t)
	families := map[DisciplineFamily]bool{}
	for _, d := range c.Disciplines {
		families[d.Family] = true
	}
	for _, want := range []DisciplineFamily{FamilyTrack, FamilyFieldHorizontal, FamilyFieldVertical, FamilyCombined, FamilyRelay} {
		if !families[want] {
			t.Fatalf("built-in discipline catalog missing family %q (SYS-003)", want)
		}
	}
	for _, code := range []string{"100m", "200m", "400m", "800m", "1500m", "HJ", "PV", "LJ", "TJ", "SP", "DT", "HT", "JT", "4x100m", "Decathlon", "Heptathlon"} {
		if _, ok := c.ByCode(code); !ok {
			t.Fatalf("built-in discipline catalog missing %q", code)
		}
	}
}

// TestDisciplineCatalog_UBSKidsCupDisciplines covers the 3 UKC disciplines
// (60m, zone long jump, 200g ball throw) required by SYS-053/UC-033, shipped
// in the same built-in catalog.
func TestDisciplineCatalog_UBSKidsCupDisciplines(t *testing.T) {
	c := mustCatalog(t)
	for _, code := range []string{"60m", "ZoneLJ", "BallThrow200g"} {
		if _, ok := c.ByCode(code); !ok {
			t.Fatalf("built-in discipline catalog missing UKC discipline %q", code)
		}
	}
}

func TestParseDisciplineCatalog_CustomCatalogNoCodeChange(t *testing.T) {
	custom := []byte(`{
		"id": "club-custom",
		"version": "2027.1",
		"disciplines": [
			{"code": "TugOfWar", "family": "relay", "unit": "points"}
		]
	}`)
	c, err := ParseDisciplineCatalog(custom)
	if err != nil {
		t.Fatalf("parse custom catalog: %v", err)
	}
	if _, ok := c.ByCode("TugOfWar"); !ok {
		t.Fatal("custom catalog lost its only discipline")
	}
}

func TestParseDisciplineCatalog_RejectsMissingVersion(t *testing.T) {
	_, err := ParseDisciplineCatalog([]byte(`{"id":"x","disciplines":[{"code":"A","family":"track"}]}`))
	if err == nil {
		t.Fatal("expected error for catalog without a version identity")
	}
}

func TestParseDisciplineCatalog_RejectsDuplicateCodes(t *testing.T) {
	_, err := ParseDisciplineCatalog([]byte(`{
		"id":"x","version":"1",
		"disciplines":[{"code":"A","family":"track"},{"code":"A","family":"track"}]
	}`))
	if err == nil {
		t.Fatal("expected error for duplicate discipline codes")
	}
}

func TestParseDisciplineCatalog_RejectsUnknownFamily(t *testing.T) {
	_, err := ParseDisciplineCatalog([]byte(`{
		"id":"x","version":"1",
		"disciplines":[{"code":"A","family":"underwater-basket-weaving"}]
	}`))
	if err == nil {
		t.Fatal("expected error for unknown discipline family")
	}
}

func TestParseDisciplineCatalog_RejectsEmptyDisciplines(t *testing.T) {
	if _, err := ParseDisciplineCatalog([]byte(`{"id":"x","version":"1","disciplines":[]}`)); err == nil {
		t.Fatal("expected error for a catalog with no disciplines")
	}
}

func TestParseDisciplineCatalog_RejectsMalformedJSON(t *testing.T) {
	if _, err := ParseDisciplineCatalog([]byte(`{not json`)); err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestDiscipline_VariantForMissing(t *testing.T) {
	d := Discipline{Code: "100m"}
	if _, ok := d.VariantFor("Women"); ok {
		t.Fatal("expected no variant for a discipline with none defined")
	}
}

func TestDisciplineCatalog_ByCode_NotFound(t *testing.T) {
	c := mustCatalog(t)
	if _, ok := c.ByCode("does-not-exist"); ok {
		t.Fatal("expected no discipline for an unknown code")
	}
}

func TestParseDisciplineCatalog_RejectsEmptyID(t *testing.T) {
	_, err := ParseDisciplineCatalog([]byte(`{"version":"1","disciplines":[{"code":"A","family":"track"}]}`))
	if err == nil {
		t.Fatal("expected error for a catalog without an id")
	}
}

func TestParseDisciplineCatalog_RejectsEmptyDisciplineCode(t *testing.T) {
	_, err := ParseDisciplineCatalog([]byte(`{"id":"x","version":"1","disciplines":[{"code":"","family":"track"}]}`))
	if err == nil {
		t.Fatal("expected error for a discipline with an empty code")
	}
}
