// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

// Command bahnfrei is the single self-contained executable (ADR-002/ADR-003).
// Role selection (venue|hub) and the "serve" subcommand are added by
// TASK-005; first-run setup is the browser /setup flow (TASK-006, SYS-131 —
// no config file, no CLI wizard); "backup" and "restore" (SYS-084,
// UC-020 #3) are TASK-014's; "demo" seeds the M1 club-demo meet (TASK-015,
// DEC-011); "timing-agent" is the watched-folder FinishLynx bridge that
// runs on the timing PC (TASK-020, ADR-006's hub-first amendment).
package main

import (
	"context"
	"fmt"
	"io"
	"os"
)

// version is stamped at build time via -ldflags (TASK-002/TASK-028).
var version = "dev"

func main() {
	run(context.Background(), os.Args[1:], os.Stdout)
}

func run(ctx context.Context, args []string, out io.Writer) {
	if len(args) > 0 {
		switch args[0] {
		case "--version":
			fmt.Fprintf(out, "bahnfrei %s\n", version)
			return
		case "serve":
			if err := runServe(ctx, args[1:], out); err != nil {
				fmt.Fprintf(out, "bahnfrei: %v\n", err)
			}
			return
		case "backup":
			if err := runBackup(ctx, args[1:], out); err != nil {
				fmt.Fprintf(out, "bahnfrei: %v\n", err)
			}
			return
		case "restore":
			if err := runRestore(ctx, args[1:], out); err != nil {
				fmt.Fprintf(out, "bahnfrei: %v\n", err)
			}
			return
		case "demo":
			if err := runDemo(ctx, args[1:], out); err != nil {
				fmt.Fprintf(out, "bahnfrei: %v\n", err)
			}
			return
		case "timing-agent":
			if err := runTimingAgent(ctx, args[1:], out); err != nil {
				fmt.Fprintf(out, "bahnfrei: %v\n", err)
			}
			return
		}
	}
	fmt.Fprintln(out, "bahnfrei: not yet operational — see docs/delivery/work-breakdown.md")
}
