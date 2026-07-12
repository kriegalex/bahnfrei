-- SPDX-License-Identifier: AGPL-3.0-only
-- Copyright (c) 2026 Bahnfrei contributors
--
-- Vertical jump capture (TASK-021, UC-012 / SYS-043):
--
-- vertical_unit_config holds one unit's bar-height progression, office-
-- configurable (SYS-043: "office-configurable, with defaults per spec"):
-- an ascending JSON array of canonical decimal-metre height strings
-- ("1.60", "1.65", …). An office extending the progression with a
-- jump-off height simply appends to this array and bumps version — the
-- ranking engine (domain.RankVertical) needs no separate jump-off concept.
--
-- vertical_trials is one row per trial at one height for one athlete: the
-- D5.2 O/X/–/r vocabulary, mirroring the shape of `attempts`
-- (0005_field_attempts.sql) for the height-progression series instead of a
-- flat trial sequence.

CREATE TABLE vertical_unit_config (
    unit_id TEXT PRIMARY KEY REFERENCES units(id),
    heights TEXT NOT NULL, -- JSON array of ascending height strings
    version INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE vertical_trials (
    id         TEXT PRIMARY KEY,
    unit_id    TEXT NOT NULL REFERENCES units(id),
    athlete_id TEXT NOT NULL REFERENCES athletes(id),
    height_idx INTEGER NOT NULL CHECK (height_idx >= 0),
    seq        INTEGER NOT NULL CHECK (seq BETWEEN 1 AND 3),
    kind       TEXT NOT NULL CHECK (kind IN ('O', 'X', '-', 'r')),
    version    INTEGER NOT NULL DEFAULT 1,
    UNIQUE (unit_id, athlete_id, height_idx, seq)
);
CREATE INDEX idx_vertical_trials_unit ON vertical_trials(unit_id);
