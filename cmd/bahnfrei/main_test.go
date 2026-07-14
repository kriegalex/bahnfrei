// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package main

import (
	"context"
	"strings"
	"testing"
)

func TestRunVersion(t *testing.T) {
	var sb strings.Builder
	run(context.Background(), []string{"--version"}, &sb)
	if got, want := sb.String(), "bahnfrei dev\n"; got != want {
		t.Errorf("run(--version) = %q, want %q", got, want)
	}
}

func TestRunDefault(t *testing.T) {
	var sb strings.Builder
	run(context.Background(), nil, &sb)
	got := sb.String()
	// Every subcommand run() actually dispatches must be listed, so the
	// usage text can never silently drift from the real command set
	// (TASK-028, replacing the M0/M1 "not yet operational" placeholder).
	for _, want := range []string{"Usage:", "serve", "backup", "restore", "demo", "timing-agent", "--version"} {
		if !strings.Contains(got, want) {
			t.Errorf("run() = %q, want it to mention %q", got, want)
		}
	}
	if strings.Contains(got, "not yet operational") {
		t.Errorf("run() = %q, want the scaffolding placeholder gone", got)
	}
}

func TestRunUnknownSubcommandShowsUsage(t *testing.T) {
	var sb strings.Builder
	run(context.Background(), []string{"bogus"}, &sb)
	if !strings.Contains(sb.String(), "Usage:") {
		t.Errorf("run([bogus]) = %q, want usage text", sb.String())
	}
}

func TestMainEntry(t *testing.T) {
	main() // prints the usage text to stdout; must not panic
}

func TestRunServeSubcommandSurfacesErrors(t *testing.T) {
	var sb strings.Builder
	run(context.Background(), []string{"serve", "--role=bogus"}, &sb)
	if !strings.Contains(sb.String(), "bahnfrei:") {
		t.Errorf("run([serve --role=bogus]) = %q, want an error line prefixed \"bahnfrei:\"", sb.String())
	}
}

func TestRunServeSubcommandStartsAndStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-canceled: exercises the real serve path without blocking the test

	var sb strings.Builder
	run(ctx, []string{"serve", "--addr=127.0.0.1:0", "--data-dir=" + t.TempDir()}, &sb)
	if !strings.Contains(sb.String(), "listening on") {
		t.Errorf("run([serve ...]) = %q, want a \"listening on\" announcement", sb.String())
	}
}
