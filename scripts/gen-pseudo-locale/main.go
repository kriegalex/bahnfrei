// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

// Command gen-pseudo-locale regenerates the checked-in pseudo-locale
// catalog (locales/qps-ploc.json) from the DE reference catalog via
// i18n.GeneratePseudo. Run it whenever message keys are added or removed:
//
//	go run ./scripts/gen-pseudo-locale
//
// i18n.Load fails closed when any locale misses a reference key, so a
// stale pseudo-locale file breaks the build deliberately (UC-025 #2).
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kriegalex/bahnfrei/internal/web/i18n"
)

func main() {
	dir := filepath.Join("internal", "web", "i18n", "locales")
	ref, err := os.ReadFile(filepath.Join(dir, "de.json")) // #nosec G304 -- fixed in-repo path, dev tool run from the repo root
	if err != nil {
		fatal(err)
	}
	var cat i18n.Catalog
	if err := json.Unmarshal(ref, &cat); err != nil {
		fatal(fmt.Errorf("parse de.json: %w", err))
	}
	out, err := json.MarshalIndent(i18n.GeneratePseudo(cat), "", "  ")
	if err != nil {
		fatal(err)
	}
	target := filepath.Join(dir, string(i18n.Pseudo)+".json")
	// #nosec G306 -- committed locale JSON, not sensitive; must stay world-readable like the rest of the repo
	if err := os.WriteFile(target, append(out, '\n'), 0o644); err != nil {
		fatal(err)
	}
	fmt.Printf("wrote %s (%d keys)\n", target, len(cat))
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "gen-pseudo-locale:", err)
	os.Exit(1)
}
