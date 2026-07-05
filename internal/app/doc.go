// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

// Package app contains use-case services orchestrating domain + store +
// audit, authorization checks (SYS-090), and the result-confirm flow with
// provenance (SYS-046/047). web never touches store directly — it goes
// through this package (architecture.md §3).
package app
