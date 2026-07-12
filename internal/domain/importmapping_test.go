// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package domain

import "testing"

// TestBuiltinImportMappingProfiles_SystemNativeAndAlabusSYS013 covers
// SYS-013(a)/(b): both shipped mapping profiles (system-native CSV and the
// Alabus assumption profile, OQ-030) load, validate, and map every required
// field (UC-004 #1/#4).
func TestBuiltinImportMappingProfiles_SystemNativeAndAlabusSYS013(t *testing.T) {
	profiles, err := BuiltinImportMappingProfiles()
	if err != nil {
		t.Fatalf("load built-in import mapping profiles: %v", err)
	}
	for _, id := range []string{ImportProfileSystemNative, ImportProfileAlabus} {
		p, ok := profiles[id]
		if !ok {
			t.Fatalf("missing built-in profile %q", id)
		}
		for _, f := range RequiredImportFields {
			if _, ok := p.HeaderFor(f); !ok {
				t.Errorf("profile %q: required field %q has no header mapping", id, f)
			}
		}
		if _, ok := p.HeaderFor(ImportFieldLicenceNo); !ok {
			t.Errorf("profile %q: expected a licenceNo column (SYS-013(b), WO §5.3b)", id)
		}
	}
}

// TestParseImportMappingProfile_RequiredFieldMissing covers the structural
// validation guard: a profile that omits a required field is rejected.
func TestParseImportMappingProfile_RequiredFieldMissing(t *testing.T) {
	data := []byte(`{
		"id": "bad", "version": "1",
		"columns": [
			{"field": "firstName", "header": "first"},
			{"field": "lastName", "header": "last"}
		]
	}`)
	if _, err := ParseImportMappingProfile(data); err == nil {
		t.Fatal("expected an error for a profile missing required fields (birthYear, sex, eventCode, categoryCode)")
	}
}

// TestParseImportMappingProfile_DuplicateHeaderRejected covers the guard
// against two fields mapped to the same CSV column.
func TestParseImportMappingProfile_DuplicateHeaderRejected(t *testing.T) {
	data := []byte(`{
		"id": "bad", "version": "1",
		"columns": [
			{"field": "firstName", "header": "name"},
			{"field": "lastName", "header": "name"},
			{"field": "birthYear", "header": "yr"},
			{"field": "sex", "header": "sex"},
			{"field": "eventCode", "header": "ev"},
			{"field": "categoryCode", "header": "cat"}
		]
	}`)
	if _, err := ParseImportMappingProfile(data); err == nil {
		t.Fatal("expected an error for two fields mapped to the same header")
	}
}

// TestParseImportMappingProfile_UnknownFieldRejected covers the field
// vocabulary guard.
func TestParseImportMappingProfile_UnknownFieldRejected(t *testing.T) {
	data := []byte(`{
		"id": "bad", "version": "1",
		"columns": [{"field": "notAField", "header": "x"}]
	}`)
	if _, err := ParseImportMappingProfile(data); err == nil {
		t.Fatal("expected an error for an unknown field name")
	}
}

// TestImportMappingProfile_CustomProfileNoCodeChange mirrors the
// TestParseCategoryScheme_CustomSchemeNoCodeChange precedent (ADR-005 §4):
// an organizer-authored mapping profile with every required field loads and
// resolves through the same generic interpreter as the built-ins.
func TestImportMappingProfile_CustomProfileNoCodeChange(t *testing.T) {
	data := []byte(`{
		"id": "club-x", "version": "1", "name": "Club X custom export",
		"columns": [
			{"field": "firstName", "header": "Vorname"},
			{"field": "lastName", "header": "Nachname"},
			{"field": "birthYear", "header": "Jg"},
			{"field": "sex", "header": "G"},
			{"field": "eventCode", "header": "Bewerb"},
			{"field": "categoryCode", "header": "Kat"}
		]
	}`)
	p, err := ParseImportMappingProfile(data)
	if err != nil {
		t.Fatalf("parse custom profile: %v", err)
	}
	if h, ok := p.HeaderFor(ImportFieldFirstName); !ok || h != "Vorname" {
		t.Fatalf("HeaderFor(firstName) = (%q, %v), want (\"Vorname\", true)", h, ok)
	}
	if _, ok := p.HeaderFor(ImportFieldClub); ok {
		t.Fatal("club was not mapped in this profile, HeaderFor should report not-found")
	}
}
