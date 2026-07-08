-- SPDX-License-Identifier: AGPL-3.0-only
-- Copyright (c) 2026 Bahnfrei contributors
--
-- Attempt-level field capture (TASK-008, UC-011 / SYS-042): one row per
-- trial of a horizontal field event — a measured mark ('valid'), a foul,
-- a pass or a retirement, with the per-attempt wind where the discipline
-- is wind-relevant. The settled per-unit outcome stays in results (the
-- capture flow recomputes it from the series after every save); attempts
-- are the raw facts corrections and reconciliation work from.
--
-- results.status_detail qualifies a status where CR 25 demands it — the
-- DQ rule reference rendered as "DQ (TR16.8)" (SYS-045, UC-010 #3).

ALTER TABLE results ADD COLUMN status_detail TEXT NOT NULL DEFAULT '';

CREATE TABLE attempts (
    id         TEXT PRIMARY KEY,
    unit_id    TEXT NOT NULL REFERENCES units(id),
    athlete_id TEXT NOT NULL REFERENCES athletes(id),
    seq        INTEGER NOT NULL CHECK (seq >= 1), -- 1-based trial number
    kind       TEXT NOT NULL CHECK (kind IN ('valid', 'foul', 'pass', 'retire')),
    mark       TEXT NOT NULL DEFAULT '',          -- metres, 0.01 m, only for 'valid'
    wind       REAL,                              -- m/s, only where wind-relevant
    version    INTEGER NOT NULL DEFAULT 1,
    UNIQUE (unit_id, athlete_id, seq)
);
CREATE INDEX idx_attempts_unit ON attempts(unit_id);
