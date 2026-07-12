-- SPDX-License-Identifier: AGPL-3.0-only
-- Copyright (c) 2026 Bahnfrei contributors
--
-- Check-in, heat seeding and round progression (TASK-018, UC-007/008/009,
-- SYS-025-030): one row per entry placed in a round's unit (heat/flight),
-- carrying its seed rank, drawn lane (0 = none — a by-lot or non-laned
-- event) and CR 25 qualification code once progression runs
-- (Q/q/qR/qJ/qD, D2.4/D5.2). entries.status already carries the SYS-025
-- check-in lifecycle (entered/confirmed/scratched/dns) added by
-- 0009_online_entries.sql — no schema change needed there.
--
-- manual_override (SYS-028) marks a heat/lane the operator hand-edited: a
-- later regeneration (internal/app seeding service) leaves these rows
-- untouched unless explicitly released.

CREATE TABLE unit_entries (
    id              TEXT PRIMARY KEY,
    unit_id         TEXT NOT NULL REFERENCES units(id),
    entry_id        TEXT NOT NULL REFERENCES entries(id),
    seed_rank       INTEGER NOT NULL DEFAULT 0,
    lane            INTEGER NOT NULL DEFAULT 0,
    qualification   TEXT NOT NULL DEFAULT '',
    manual_override INTEGER NOT NULL DEFAULT 0,
    version         INTEGER NOT NULL DEFAULT 1,
    UNIQUE (unit_id, entry_id)
);
CREATE INDEX idx_unit_entries_unit ON unit_entries(unit_id);
CREATE INDEX idx_unit_entries_entry ON unit_entries(entry_id);
-- Two entries never share a drawn lane within the same unit (lane 0 = "no
-- lane assigned" is exempt — many entries can share "no lane").
CREATE UNIQUE INDEX idx_unit_entries_unit_lane ON unit_entries(unit_id, lane)
    WHERE lane <> 0;
