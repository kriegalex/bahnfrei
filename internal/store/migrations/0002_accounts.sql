-- SPDX-License-Identifier: AGPL-3.0-only
-- Copyright (c) 2026 Bahnfrei contributors
--
-- Local accounts (SYS-090/091, ADR-003 shell layer). One row per human
-- operator; `role` holds the coarse, instance-wide RBAC role primitive
-- (SYS-090's enumerated roles). Per-meet capability scoping is a later
-- layer (TASK-013) added on top of this table, not a schema change here.
-- Credentials are never stored in cleartext or a reversible form: only
-- the adaptive-hash encoding produced by internal/app (SYS-091).

CREATE TABLE accounts (
    id            TEXT PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    display_name  TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL,
    created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    version       INTEGER NOT NULL DEFAULT 1
);
