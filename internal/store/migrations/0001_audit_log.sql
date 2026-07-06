-- SPDX-License-Identifier: AGPL-3.0-only
-- Copyright (c) 2026 Bahnfrei contributors
--
-- Append-only audit log, first-class table (ADR-004 §3, SYS-046).
-- AUTOINCREMENT guarantees a monotonic, never-reused sequence.
-- UPDATE/DELETE are blocked by triggers: corrections are new rows.
-- Erasure requests (SYS-101) will use a dedicated, migration-controlled
-- redaction routine (TASK-023) — never ad-hoc UPDATEs.

CREATE TABLE audit_log (
    seq         INTEGER PRIMARY KEY AUTOINCREMENT,
    ts          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    actor       TEXT NOT NULL,
    action      TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id   TEXT NOT NULL,
    before_json TEXT,
    after_json  TEXT,
    reason      TEXT
);

CREATE INDEX audit_log_entity ON audit_log (entity_type, entity_id, seq);

CREATE TRIGGER audit_log_no_update
BEFORE UPDATE ON audit_log
BEGIN
    SELECT RAISE(ABORT, 'audit log is append-only (SYS-046)');
END;

CREATE TRIGGER audit_log_no_delete
BEFORE DELETE ON audit_log
BEGIN
    SELECT RAISE(ABORT, 'audit log is append-only (SYS-046)');
END;
