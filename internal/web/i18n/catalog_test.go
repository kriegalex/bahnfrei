// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package i18n

import "testing"

func TestLoadShipsCompleteDEAndFR(t *testing.T) {
	cats, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cats.Has(DE) {
		t.Error("DE catalog must be loaded (SYS-110: DE ships complete)")
	}
	if !cats.Has(FR) {
		t.Error("FR catalog must be loaded (SYS-110: FR ships complete)")
	}
	ref := cats.ReferenceKeys()
	if len(ref) == 0 {
		t.Fatal("reference (DE) catalog has no keys")
	}
	for _, loc := range []Locale{DE, FR} {
		for _, k := range ref {
			if cats[loc][k] == "" {
				t.Errorf("locale %q missing key %q (UC-025 #1: zero missing-key fallbacks)", loc, k)
			}
		}
	}
}

func TestLoadDiscoversPseudoLocaleWithNoCodeChange(t *testing.T) {
	// This is the mechanical proof for SYS-110's "adding a language SHALL
	// require only a translation file (no code change)": Load() enumerates
	// locales/*.json rather than a hard-coded list, so the pseudo-locale
	// file — added exactly like a real translation would be — is picked
	// up automatically (UC-025 #2).
	cats, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cats.Has(Pseudo) {
		t.Fatalf("pseudo-locale %q was not discovered under locales/ — Load must not hard-code the locale list", Pseudo)
	}
	for _, k := range cats.ReferenceKeys() {
		if cats[Pseudo][k] == "" {
			t.Errorf("pseudo-locale missing key %q — UC-025 #2 requires full coverage", k)
		}
	}
}

func TestMissingKeyFallsBackThenBrackets(t *testing.T) {
	cats := Catalogs{
		DE: Catalog{"greeting": "Hallo"},
		FR: Catalog{}, // intentionally incomplete for this unit test
	}
	if got, want := cats.Text(DE, "greeting"), "Hallo"; got != want {
		t.Errorf("Text(DE, greeting) = %q, want %q", got, want)
	}
	if got, want := cats.Text(FR, "greeting"), "Hallo"; got != want {
		t.Errorf("Text(FR, greeting) falling back to reference = %q, want %q", got, want)
	}
	if got := cats.Text(FR, "no.such.key"); got != "[[no.such.key]]" {
		t.Errorf("Text(missing key) = %q, want bracketed key", got)
	}
}

func TestTextSubstitutesPlaceholders(t *testing.T) {
	cats := Catalogs{DE: Catalog{"welcome": "Hallo {name}, willkommen"}}
	got := cats.Text(DE, "welcome", "name", "Alice")
	want := "Hallo Alice, willkommen"
	if got != want {
		t.Errorf("Text with placeholder = %q, want %q", got, want)
	}
}

func TestLocalesSorted(t *testing.T) {
	cats := Catalogs{FR: Catalog{}, DE: Catalog{}, Pseudo: Catalog{}}
	got := cats.Locales()
	want := []Locale{DE, FR, Pseudo}
	if len(got) != len(want) {
		t.Fatalf("Locales() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Locales()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
