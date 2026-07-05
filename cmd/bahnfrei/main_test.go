// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package main

import (
	"strings"
	"testing"
)

func TestRunVersion(t *testing.T) {
	var sb strings.Builder
	run([]string{"--version"}, &sb)
	if got, want := sb.String(), "bahnfrei dev\n"; got != want {
		t.Errorf("run(--version) = %q, want %q", got, want)
	}
}

func TestRunDefault(t *testing.T) {
	var sb strings.Builder
	run(nil, &sb)
	if !strings.Contains(sb.String(), "not yet operational") {
		t.Errorf("run() = %q, want placeholder notice", sb.String())
	}
}

func TestMainEntry(t *testing.T) {
	main() // prints the placeholder notice to stdout; must not panic
}
