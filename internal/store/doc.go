// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

// Package store provides SQLite persistence: migrations, optimistic
// versioning, the append-only audit log, backup, and retention jobs
// (ADR-004; SYS-081/084/101/102). Implemented by TASK-003.
package store
