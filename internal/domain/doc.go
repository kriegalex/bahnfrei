// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

// Package domain holds the pure domain model: entities (SyRS §2) and rule
// engines — category resolver, seeding, progression, scoring, countback,
// eligibility, wind legality, records flagging. No I/O; every engine ships
// with fixture suites traced to D-references (SYS-142).
//
// Dependency rule (architecture.md §3): domain imports nothing above it.
//
// TASK-004 implements the SyRS §2 entities (incl. namespaced ExternalIDs,
// ADR-005 §6), the category-scheme resolver (category.go — a data
// interpreter over CategoryScheme, per ADR-005 §4), and the built-in
// discipline catalog (discipline.go). Built-in category schemes (Swiss
// Athletics, UBS Kids Cup) and the discipline catalog ship as versioned
// JSON under data/ (schemes.go embeds them) — replaceable without a code
// change (UC-002 #4, SYS-005). Seeding, progression, and scoring engines are
// later TASKs (TASK-018/021).
package domain
