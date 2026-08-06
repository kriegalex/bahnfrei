-- SPDX-License-Identifier: AGPL-3.0-only
-- Copyright (c) 2026 Bahnfrei contributors
--
-- Per-op sync rejection status (TASK-044, SYS-149, UC-040): the offline
-- capture sync protocol (0006_offline_capture.sql, internal/sync/doc.go)
-- previously acknowledged a queued op as only 'applied' or 'reconciliation'
-- — a validation failure (e.g. an unparseable mark) or a business-rule
-- rejection (an unregistered athlete, an already-announced unit) had no
-- terminal per-op outcome, so the server fell through to failing the WHOLE
-- batch with HTTP 400 and the client's queue got stuck retrying it forever
-- (usability-audit-volunteer-2026-08.md finding F1). 'rejected' is a new
-- terminal, non-retryable status: recorded in the exactly-once ledger like
-- 'applied'/'reconciliation' so a resent op id (e.g. the ack itself was
-- lost) re-acknowledges the same rejection instead of re-validating, but
-- — unlike 'reconciliation' — never queued for office review: the client
-- removes it from its local queue and offers the operator correct-or-
-- discard at the point of capture (UC-040 #1/#3).
--
-- SQLite's CHECK constraints cannot be altered in place, so capture_ops is
-- rebuilt with the extended vocabulary; capture_ops_new keeps every column,
-- index and existing row unchanged.

CREATE TABLE capture_ops_new (
    op_id      TEXT PRIMARY KEY,
    unit_id    TEXT NOT NULL REFERENCES units(id),
    status     TEXT NOT NULL CHECK (status IN ('applied', 'reconciliation', 'rejected')),
    reason     TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
INSERT INTO capture_ops_new (op_id, unit_id, status, reason, created_at)
    SELECT op_id, unit_id, status, reason, created_at FROM capture_ops;
DROP TABLE capture_ops;
ALTER TABLE capture_ops_new RENAME TO capture_ops;
CREATE INDEX idx_capture_ops_unit ON capture_ops(unit_id);
