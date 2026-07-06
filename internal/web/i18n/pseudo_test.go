// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package i18n

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGeneratePseudoCoversEveryKeyAndMarksIt(t *testing.T) {
	ref := Catalog{
		"a": "Hello",
		"b": "World {name}",
	}
	got := GeneratePseudo(ref)
	if len(got) != len(ref) {
		t.Fatalf("GeneratePseudo produced %d keys, want %d", len(got), len(ref))
	}
	for k := range ref {
		v, ok := got[k]
		if !ok || v == "" {
			t.Errorf("GeneratePseudo missing key %q", k)
		}
		if v[0] != '[' {
			t.Errorf("pseudo value %q should be bracket-marked", v)
		}
	}
}

func TestGeneratePseudoPreservesPlaceholders(t *testing.T) {
	got := GeneratePseudo(Catalog{"greet": "Hello {name}, you have {count} messages"})["greet"]
	if !strings.Contains(got, "{name}") || !strings.Contains(got, "{count}") {
		t.Errorf("pseudoized value %q must keep placeholders literal for Text() substitution", got)
	}
}

// TestPseudoLocaleInSyncWithReference regenerates the pseudo-locale from
// the checked-in DE reference catalog and requires it to match
// locales/qps-ploc.json byte-for-byte. This is the "pseudo-locale CI
// build" for SYS-110/UC-025 #2: any change to de.json that isn't
// accompanied by regenerating qps-ploc.json fails plain `go test ./...`,
// which is already a CI gate (no separate workflow step needed) — the
// same mechanism a real translation-file addition would exercise.
func TestPseudoLocaleInSyncWithReference(t *testing.T) {
	data, err := localesFS.ReadFile("locales/de.json")
	if err != nil {
		t.Fatalf("read de.json: %v", err)
	}
	var ref Catalog
	if err := json.Unmarshal(data, &ref); err != nil {
		t.Fatalf("parse de.json: %v", err)
	}
	want := GeneratePseudo(ref)

	pseudoData, err := localesFS.ReadFile("locales/qps-ploc.json")
	if err != nil {
		t.Fatalf("read qps-ploc.json: %v", err)
	}
	var got Catalog
	if err := json.Unmarshal(pseudoData, &got); err != nil {
		t.Fatalf("parse qps-ploc.json: %v", err)
	}

	if len(got) != len(want) {
		t.Fatalf("qps-ploc.json has %d keys, regenerated has %d — run the pseudo-locale generator and commit the result", len(got), len(want))
	}
	for k, wantV := range want {
		if got[k] != wantV {
			t.Errorf("qps-ploc.json[%q] = %q, want %q (regenerate from de.json)", k, got[k], wantV)
		}
	}
}
