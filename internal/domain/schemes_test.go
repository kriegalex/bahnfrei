// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package domain

import "testing"

// TestBuiltinCategoryScheme_ReadFileError exercises the embedded-file read
// error path, which cannot occur for the shipped data files in normal
// operation but must still fail closed rather than panic.
func TestBuiltinCategoryScheme_ReadFileError(t *testing.T) {
	orig := builtinSchemeFiles[SchemeSwissAthletics]
	builtinSchemeFiles[SchemeSwissAthletics] = "data/category-schemes/does-not-exist.json"
	defer func() { builtinSchemeFiles[SchemeSwissAthletics] = orig }()

	if _, err := BuiltinCategoryScheme(SchemeSwissAthletics); err == nil {
		t.Fatal("expected an error when the embedded scheme file is missing")
	}
}

func TestBuiltinCategorySchemes_PropagatesReadError(t *testing.T) {
	orig := builtinSchemeFiles[SchemeSwissAthletics]
	builtinSchemeFiles[SchemeSwissAthletics] = "data/category-schemes/does-not-exist.json"
	defer func() { builtinSchemeFiles[SchemeSwissAthletics] = orig }()

	if _, err := BuiltinCategorySchemes(); err == nil {
		t.Fatal("expected BuiltinCategorySchemes to propagate a per-scheme read error")
	}
}

func TestBuiltinDisciplineCatalog_ReadFileError(t *testing.T) {
	orig := builtinDisciplineCatalogFile
	builtinDisciplineCatalogFile = "data/disciplines/does-not-exist.json"
	defer func() { builtinDisciplineCatalogFile = orig }()

	if _, err := BuiltinDisciplineCatalog(); err == nil {
		t.Fatal("expected an error when the embedded discipline catalog file is missing")
	}
}
