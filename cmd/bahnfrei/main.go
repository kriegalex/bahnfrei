// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

// Command bahnfrei is the single self-contained executable (ADR-002/ADR-003).
// Role selection (venue|hub), the quickstart bootstrap, and backup/restore
// commands are added by TASK-005/TASK-006/TASK-014.
package main

import (
	"fmt"
	"io"
	"os"
)

// version is stamped at build time via -ldflags (TASK-002/TASK-028).
var version = "dev"

func main() {
	run(os.Args[1:], os.Stdout)
}

func run(args []string, out io.Writer) {
	if len(args) > 0 && args[0] == "--version" {
		fmt.Fprintf(out, "bahnfrei %s\n", version)
		return
	}
	fmt.Fprintln(out, "bahnfrei: not yet operational — see docs/delivery/work-breakdown.md")
}
