-- SPDX-License-Identifier: AGPL-3.0-only
-- Copyright (c) 2026 Bahnfrei contributors
--
-- Athletes, meet participation and settled results (TASK-007, UC-033):
-- the storage the UBS Kids Cup scoring/standings slice needs. Athletes are
-- instance-global (SYS-010; one athlete competes across meets); a
-- participant row is an athlete's registration at one meet with their bib
-- (the printed start number, C7.3); a result row is the settled mark per
-- unit and athlete (SyRS §2 Participation/Result — attempt-level detail is
-- TASK-008's). Meets record which template/scoring table created them so
-- scoring survives restarts and data-file updates are explicit.
--
-- Dates/times are RFC 3339 TEXT (UTC); club_ids/record_flags hold JSON
-- produced and consumed exclusively by internal/store.

ALTER TABLE meets ADD COLUMN template_id TEXT NOT NULL DEFAULT '';
ALTER TABLE meets ADD COLUMN scoring_table TEXT NOT NULL DEFAULT '';

CREATE TABLE clubs (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL,
    external_ids TEXT NOT NULL DEFAULT '{}', -- JSON namespace map (ADR-005 §6)
    version      INTEGER NOT NULL DEFAULT 1,
    UNIQUE (name)
);

CREATE TABLE athletes (
    id           TEXT PRIMARY KEY,
    first_name   TEXT NOT NULL,
    last_name    TEXT NOT NULL,
    birth_date   TEXT,              -- NULL when only the year is known (SYS-010)
    birth_year   INTEGER NOT NULL,
    sex          TEXT NOT NULL,
    nationality  TEXT NOT NULL DEFAULT '',
    club_ids     TEXT NOT NULL DEFAULT '[]', -- JSON array of club ids
    external_ids TEXT NOT NULL DEFAULT '{}',
    version      INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE participants (
    id         TEXT PRIMARY KEY,
    meet_id    TEXT NOT NULL REFERENCES meets(id),
    athlete_id TEXT NOT NULL REFERENCES athletes(id),
    bib        TEXT NOT NULL DEFAULT '',
    version    INTEGER NOT NULL DEFAULT 1,
    UNIQUE (meet_id, athlete_id)
);
CREATE INDEX idx_participants_meet ON participants(meet_id);
-- Bib numbers are unique within a meet where assigned.
CREATE UNIQUE INDEX idx_participants_bib ON participants(meet_id, bib)
    WHERE bib <> '';

CREATE TABLE results (
    id           TEXT PRIMARY KEY,
    unit_id      TEXT NOT NULL REFERENCES units(id),
    athlete_id   TEXT NOT NULL REFERENCES athletes(id),
    mark         TEXT NOT NULL DEFAULT '',  -- encoded per discipline unit
    timing       TEXT NOT NULL DEFAULT '',  -- 'manual'/'electronic' for track marks
    status       TEXT NOT NULL DEFAULT '',  -- CR 25 vocabulary (DNS, NM, DQ, ...)
    points       INTEGER,                   -- NULL when the mark scores no points
    wind         REAL,
    lane         INTEGER NOT NULL DEFAULT 0,
    placing      INTEGER,
    record_flags TEXT NOT NULL DEFAULT '[]',
    version      INTEGER NOT NULL DEFAULT 1,
    UNIQUE (unit_id, athlete_id)
);
CREATE INDEX idx_results_unit ON results(unit_id);
CREATE INDEX idx_results_athlete ON results(athlete_id);
