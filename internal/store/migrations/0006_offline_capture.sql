-- SPDX-License-Identifier: AGPL-3.0-only
-- Copyright (c) 2026 Bahnfrei contributors
--
-- Offline-tolerant field capture & walk-by sync (TASK-009, UC-034 /
-- SYS-085/086; ADR-004 §8). Three tables back the server half of the
-- offline capture queue:
--
--   unit_checkouts       the event-unit checkout lock (SYS-086): at most one
--                        active holder per unit, a monotonic per-unit
--                        `generation` bumped whenever the office overrides or
--                        a device re-takes the lock, and a `start_list_version`
--                        the office bumps when it revises the unit's start
--                        list. Replays are validated against these.
--   capture_ops          the exactly-once ledger: one terminal decision per
--                        client-generated op ULID, so repeated/flaky batches
--                        replay idempotently (SYS-085).
--   reconciliation_items captures that could NOT be applied — a stale
--                        checkout, an office start-list change, or an apply
--                        conflict — land here for the office to apply or
--                        discard. Never silently discarded (SYS-086; the
--                        documented Web.TEC 2 loss mode C2.1 is the named
--                        anti-pattern).

CREATE TABLE unit_checkouts (
    unit_id            TEXT PRIMARY KEY REFERENCES units(id),
    account_id         TEXT NOT NULL,
    device_label       TEXT NOT NULL DEFAULT '',
    token              TEXT NOT NULL,              -- opaque holder credential (ULID), reissued on reassign
    generation         INTEGER NOT NULL,           -- monotonic per unit; bumped on override/reassign
    start_list_version INTEGER NOT NULL DEFAULT 0, -- bumped when the office revises the start list
    active             INTEGER NOT NULL DEFAULT 1, -- 0 once released on unit completion
    checked_out_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    version            INTEGER NOT NULL DEFAULT 1  -- optimistic-concurrency guard
);

CREATE TABLE capture_ops (
    op_id      TEXT PRIMARY KEY,   -- client-generated ULID (SYS-085 idempotency key)
    unit_id    TEXT NOT NULL REFERENCES units(id),
    status     TEXT NOT NULL CHECK (status IN ('applied', 'reconciliation')),
    reason     TEXT,               -- reconciliation reason when status='reconciliation'
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX idx_capture_ops_unit ON capture_ops(unit_id);

CREATE TABLE reconciliation_items (
    id                 TEXT PRIMARY KEY,
    op_id              TEXT NOT NULL,
    unit_id            TEXT NOT NULL REFERENCES units(id),
    athlete_id         TEXT NOT NULL,
    reason             TEXT NOT NULL CHECK (reason IN ('stale_checkout', 'start_list_change', 'conflict')),
    payload_json       TEXT NOT NULL,               -- the captured attempt, for apply/discard
    captured_by        TEXT NOT NULL,               -- account id that captured/replayed it
    device_label       TEXT NOT NULL DEFAULT '',
    generation         INTEGER NOT NULL,            -- generation the op was captured against
    start_list_version INTEGER NOT NULL,
    status             TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'applied', 'discarded')),
    created_at         TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    resolved_at        TEXT,
    resolved_by        TEXT
);
CREATE INDEX idx_reconciliation_unit ON reconciliation_items(unit_id, status);
