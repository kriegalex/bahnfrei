-- SPDX-License-Identifier: AGPL-3.0-only
-- Copyright (c) 2026 Bahnfrei contributors
--
-- Online entries (TASK-016, UC-003/UC-006, SYS-011/012/015/017/018):
-- individual/club-bulk/relay entries against the event programme, bib
-- assignment and a configurable fee schedule.
--
--   meets.entry_fee_cents/relay_fee_cents   the SYS-017 fee schedule
--                                           (Rappen/cents, CHF), configured
--                                           per meet.
--   events.entry_limit                     the SYS-015 per-event entry cap
--                                           (0 = unlimited).
--   relay_teams                            a club's ordered leg composition
--                                           plus reserves for one relay
--                                           entry (SYS-012); composition_json
--                                           / reserves_json hold JSON arrays
--                                           of athlete ids, produced and
--                                           consumed exclusively by
--                                           internal/store.
--   entries                                one Entry per athlete×event or
--                                           relay-team×event (SYS-011/012);
--                                           athlete_id/relay_team_id are
--                                           mutually exclusive (enforced in
--                                           internal/domain), each unique
--                                           within its event where set.
--
-- Bib assignment (SYS-018) reuses the existing participants table
-- (0004_athletes_results.sql) — entries ensure a meet-wide participant row
-- exists for their athlete(s) so bib assignment stays meet-scoped, not
-- event-scoped (an athlete entering several events still gets one bib).

ALTER TABLE meets ADD COLUMN entry_fee_cents INTEGER NOT NULL DEFAULT 0;
ALTER TABLE meets ADD COLUMN relay_fee_cents INTEGER NOT NULL DEFAULT 0;

ALTER TABLE events ADD COLUMN entry_limit INTEGER NOT NULL DEFAULT 0;

CREATE TABLE relay_teams (
    id               TEXT PRIMARY KEY,
    club_id          TEXT NOT NULL REFERENCES clubs(id),
    composition_json TEXT NOT NULL DEFAULT '[]', -- JSON array of athlete ids, leg order
    reserves_json    TEXT NOT NULL DEFAULT '[]', -- JSON array of reserve athlete ids
    version          INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE entries (
    id               TEXT PRIMARY KEY,
    event_id         TEXT NOT NULL REFERENCES events(id),
    athlete_id       TEXT NOT NULL DEFAULT '',
    relay_team_id    TEXT NOT NULL DEFAULT '',
    seed_performance TEXT NOT NULL DEFAULT '',
    status           TEXT NOT NULL DEFAULT 'entered',
    source           TEXT NOT NULL DEFAULT 'online',
    started_up       INTEGER NOT NULL DEFAULT 0,
    started_down     INTEGER NOT NULL DEFAULT 0,
    fails_standard   INTEGER NOT NULL DEFAULT 0,
    submitted_by     TEXT NOT NULL DEFAULT '',
    created_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    version          INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX idx_entries_event ON entries(event_id);
CREATE INDEX idx_entries_athlete ON entries(athlete_id);
CREATE INDEX idx_entries_relay_team ON entries(relay_team_id);
CREATE INDEX idx_entries_submitted_by ON entries(submitted_by);
-- An athlete/relay team enters a given event at most once (re-submission is
-- a duplicate, UC-003 #1).
CREATE UNIQUE INDEX idx_entries_event_athlete ON entries(event_id, athlete_id)
    WHERE athlete_id <> '';
CREATE UNIQUE INDEX idx_entries_event_relay ON entries(event_id, relay_team_id)
    WHERE relay_team_id <> '';
