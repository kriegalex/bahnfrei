-- SPDX-License-Identifier: AGPL-3.0-only
-- Copyright (c) 2026 Bahnfrei contributors
--
-- Full track capture & corrections (TASK-019, UC-010/UC-015, SYS-040/046/047):
--
-- unit_wind holds the single per-race wind reading (SYS-040: "per-race wind
-- reading where the discipline is wind-relevant") — one row per unit, applied
-- uniformly to every athlete's result in that race (UC-010 #4: "every mark in
-- that race is flagged wind-assisted"), never per-athlete.
--
-- result_announcements is the append-only publication history a unit's
-- protest clock derives from (SYS-047, D8.3): the office announces a unit's
-- result list (seq 1), opening a 30-minute protest window; any later
-- correction re-announces (seq 2, 3, …), reopening the appeal window
-- (UC-015 #2). The latest row by seq is the unit's current announcement; no
-- row means the unit's results are not yet posted.

CREATE TABLE unit_wind (
    unit_id TEXT PRIMARY KEY REFERENCES units(id),
    wind    REAL NOT NULL
);

CREATE TABLE result_announcements (
    id           TEXT PRIMARY KEY,
    unit_id      TEXT NOT NULL REFERENCES units(id),
    seq          INTEGER NOT NULL,
    announced_at TEXT NOT NULL,
    actor        TEXT NOT NULL,
    UNIQUE (unit_id, seq)
);
CREATE INDEX idx_result_announcements_unit ON result_announcements(unit_id, seq);
