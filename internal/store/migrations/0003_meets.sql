-- SPDX-License-Identifier: AGPL-3.0-only
-- Copyright (c) 2026 Bahnfrei contributors
--
-- Meet setup (TASK-006, UC-001): meets with sessions per competition day
-- (SYS-001), the event programme as discipline × category with round
-- structure and entry conditions (SYS-002), schedulable units, and the
-- append-only published-timetable history (SYS-004: every published
-- amendment is timestamped and retained — publishing INSERTs a new
-- version row, never rewrites one).
--
-- Dates/times are stored as RFC 3339 TEXT (UTC), matching audit_log and
-- accounts. category_codes / entries_json hold JSON produced and consumed
-- exclusively by internal/store.

CREATE TABLE meets (
    id               TEXT PRIMARY KEY,
    name             TEXT NOT NULL,
    venue            TEXT NOT NULL,
    homologation_ref TEXT NOT NULL DEFAULT '',
    start_date       TEXT NOT NULL,
    end_date         TEXT NOT NULL,
    organizer        TEXT NOT NULL,
    tier             TEXT NOT NULL,
    status           TEXT NOT NULL DEFAULT 'draft',
    category_scheme  TEXT NOT NULL DEFAULT 'swiss-athletics',
    created_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    version          INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE meet_sessions (
    id      TEXT PRIMARY KEY,
    meet_id TEXT NOT NULL REFERENCES meets(id),
    day     TEXT NOT NULL,
    label   TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX idx_meet_sessions_meet ON meet_sessions(meet_id);

CREATE TABLE events (
    id              TEXT PRIMARY KEY,
    meet_id         TEXT NOT NULL REFERENCES meets(id),
    discipline_code TEXT NOT NULL,
    category_codes  TEXT NOT NULL, -- JSON array of category codes (SYS-002)
    entry_standard  TEXT NOT NULL DEFAULT '',
    entry_deadline  TEXT,          -- NULL when no deadline configured
    status          TEXT NOT NULL DEFAULT 'draft',
    version         INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX idx_events_meet ON events(meet_id);

CREATE TABLE rounds (
    id       TEXT PRIMARY KEY,
    event_id TEXT NOT NULL REFERENCES events(id),
    kind     TEXT NOT NULL,
    seq      INTEGER NOT NULL, -- progression order within the event (D2.1)
    version  INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX idx_rounds_event ON rounds(event_id);

CREATE TABLE units (
    id           TEXT PRIMARY KEY,
    round_id     TEXT NOT NULL REFERENCES rounds(id),
    scheduled_at TEXT, -- NULL until the organizer schedules the unit
    location     TEXT NOT NULL DEFAULT '',
    version      INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX idx_units_round ON units(round_id);

-- One row per publish action; the meet's current public timetable is the
-- row with the highest version. Amendments republish, appending a new row
-- with a fresh timestamp, so every historical version stays retrievable
-- with the time it was published (SYS-004, UC-001 #4).
CREATE TABLE timetable_versions (
    id           TEXT PRIMARY KEY,
    meet_id      TEXT NOT NULL REFERENCES meets(id),
    version      INTEGER NOT NULL,
    published_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    entries_json TEXT NOT NULL,
    UNIQUE (meet_id, version)
);
