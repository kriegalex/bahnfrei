// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

// Command bahnfrei is the single self-contained executable (ADR-002/ADR-003).
// Role selection (venue|hub) and the "serve" subcommand are added by
// TASK-005; the ≤30-minute quickstart wizard and backup/restore commands
// are added by TASK-006/TASK-014.
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
		}
	}
	fmt.Fprintln(out, "bahnfrei: not yet operational — see docs/delivery/work-breakdown.md")
}
