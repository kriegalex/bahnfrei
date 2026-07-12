-- SPDX-License-Identifier: AGPL-3.0-only
-- Copyright (c) 2026 Bahnfrei contributors
--
-- Timing exchange & timing agent (TASK-020, UC-014, SYS-060-062, ADR-006):
--
--   results.source           capture provenance (SYS-041/ADR-006 §4):
--                             'manual' (every pre-existing call site,
--                             unchanged default) vs 'import_lif'/
--                             'import_csv' (the ingest pipeline below).
--                             Lets the import conflict check tell an
--                             existing manual result (never silently
--                             overwritten, SYS-061) from a row a previous
--                             import itself wrote (safe to refresh).
--
--   timing_unit_numbers       the stable FinishLynx event/round/heat
--                             numeric handles a unit is assigned the first
--                             time it is exported (SYS-060). FinishLynx
--                             treats these numbers as an event's
--                             persistent identity across the whole meet
--                             (Database Files' own sample data keeps the
--                             same event number across re-exports);
--                             recomputing them on every export would
--                             silently break a timing PC's saved event
--                             list, so they are assigned once, from
--                             internal/app/exchange.go's stable
--                             event-then-round-then-heat ordering, and
--                             never recomputed for a unit that already has
--                             one — even if a later unit is added ahead of
--                             it in nominal ordering.
--
--   timing_import_batches /
--   timing_import_conflicts   one row per ingested .lif/.csv file, and one
--                             row per athlete/bib inside it that could not
--                             be applied automatically (SYS-061, UC-014
--                             #4/#5): an unknown bib, a file whose
--                             event/round/heat numbers match no unit, an
--                             existing *manual* result already on that
--                             unit/athlete, the unit already being
--                             announced (SYS-047 — any write past that
--                             point must go through the audited correction
--                             flow, TASK-019), or a Lynx status code with
--                             no CR 25 equivalent (OQ-048). Conflicts are
--                             queued for the operator to resolve per
--                             athlete (kept/replaced/merged/discarded) —
--                             never silently applied or dropped, mirroring
--                             the reconciliation_items precedent in
--                             0006_offline_capture.sql.
--
--   timing_agent_tokens       opaque bearer credentials for the unattended
--                             timing-agent process on the timing PC
--                             (ADR-006's hub-first amendment: "bridges it
--                             to the server over HTTPS"). Hashed at rest —
--                             SHA-256 is sufficient for a high-entropy
--                             random token (no offline-guessing surface
--                             the way a human password has), persisted
--                             (unlike in-memory browser sessions) because
--                             the agent must keep working across hub
--                             restarts.

ALTER TABLE results ADD COLUMN source TEXT NOT NULL DEFAULT 'manual'
    CHECK (source IN ('manual', 'import_lif', 'import_csv'));

CREATE TABLE timing_unit_numbers (
    unit_id      TEXT PRIMARY KEY REFERENCES units(id),
    event_number INTEGER NOT NULL,
    round_number INTEGER NOT NULL,
    heat_number  INTEGER NOT NULL,
    assigned_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    UNIQUE (event_number, round_number, heat_number)
);

CREATE TABLE timing_import_batches (
    id          TEXT PRIMARY KEY,
    meet_id     TEXT NOT NULL REFERENCES meets(id),
    format      TEXT NOT NULL CHECK (format IN ('lif', 'csv')),
    filename    TEXT NOT NULL,
    unit_id     TEXT REFERENCES units(id), -- NULL when the file's unit itself could not be resolved
    applied     INTEGER NOT NULL DEFAULT 0,
    conflicted  INTEGER NOT NULL DEFAULT 0,
    imported_by TEXT NOT NULL DEFAULT '', -- account id, or the timing-agent token label
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX idx_timing_import_batches_meet ON timing_import_batches(meet_id, created_at);

CREATE TABLE timing_import_conflicts (
    id           TEXT PRIMARY KEY,
    batch_id     TEXT NOT NULL REFERENCES timing_import_batches(id),
    unit_id      TEXT REFERENCES units(id),
    athlete_id   TEXT REFERENCES athletes(id),
    bib          TEXT NOT NULL DEFAULT '',
    lane         INTEGER NOT NULL DEFAULT 0,
    reason       TEXT NOT NULL CHECK (reason IN
        ('unknown_bib', 'unresolved_unit', 'existing_manual_result', 'unit_announced', 'unmapped_status')),
    payload_json TEXT NOT NULL DEFAULT '{}',
    status       TEXT NOT NULL DEFAULT 'pending' CHECK (status IN
        ('pending', 'kept', 'replaced', 'merged', 'discarded')),
    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    resolved_at  TEXT,
    resolved_by  TEXT
);
CREATE INDEX idx_timing_import_conflicts_batch ON timing_import_conflicts(batch_id, status);

CREATE TABLE timing_agent_tokens (
    id         TEXT PRIMARY KEY,
    meet_id    TEXT NOT NULL REFERENCES meets(id),
    label      TEXT NOT NULL DEFAULT '',
    token_hash TEXT NOT NULL UNIQUE,
    created_by TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    revoked_at TEXT
);
CREATE INDEX idx_timing_agent_tokens_meet ON timing_agent_tokens(meet_id);
