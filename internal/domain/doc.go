// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

// Package domain holds the pure domain model: entities (SyRS §2) and rule
// engines — category resolver, seeding, progression, scoring, countback,
// eligibility, wind legality, records flagging. No I/O; every engine ships
// with fixture suites traced to D-references (SYS-142).
//
// Dependency rule (architecture.md §3): domain imports nothing above it.
package domain
