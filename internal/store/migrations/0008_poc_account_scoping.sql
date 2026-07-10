-- SPDX-License-Identifier: AGPL-3.0-only
-- Copyright (c) 2026 Bahnfrei contributors
--
-- PoC account lifecycle & field-official per-event scoping (TASK-013,
-- SYS-090/091, UC-022). Two additions on top of the TASK-005 accounts
-- shell (0002_accounts.sql):
--
--   accounts.enabled       an office/admin action can disable an account
--                          without deleting its audit history (SYS-091);
--                          a disabled account cannot log in and its live
--                          sessions are revoked (internal/app.AuthService).
--                          Defaults to enabled so every pre-existing row
--                          (and every ordinary CreateAccount call) is
--                          unaffected.
--   field_official_units   per-meet capability grant (SYS-090: "assignable
--                          per meet"): the event units a field-official
--                          account may open for capture. Absence of a row
--                          for (account, unit) denies capture access to
--                          that unit for that account — enforced
--                          server-side in internal/app (UC-022 #1), never
--                          just hidden in the UI. account_id is not
--                          foreign-keyed to accounts(id), matching
--                          0006_offline_capture.sql's unit_checkouts /
--                          capture_ops precedent: the actor identity that
--                          flows through here is an opaque account ID, the
--                          same convention audit_log.actor already uses.

ALTER TABLE accounts ADD COLUMN enabled INTEGER NOT NULL DEFAULT 1;

CREATE TABLE field_official_units (
    id         TEXT PRIMARY KEY,
    account_id TEXT NOT NULL,
    meet_id    TEXT NOT NULL REFERENCES meets(id),
    unit_id    TEXT NOT NULL REFERENCES units(id),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    UNIQUE (account_id, unit_id)
);
CREATE INDEX idx_field_official_units_account ON field_official_units(account_id);
CREATE INDEX idx_field_official_units_meet ON field_official_units(meet_id);
