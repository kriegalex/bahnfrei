-- SPDX-License-Identifier: AGPL-3.0-only
-- Copyright (c) 2026 Bahnfrei contributors
--
-- Admin-issued one-time password reset (TASK-053, DEC-030, SYS-090/091): a
-- meet-morning-lockout fix that needs no email infrastructure (offline-venue
-- posture). must_change_password marks a temporary password an instance
-- admin just set on an account: the next successful login is forced through
-- a change-password step (internal/web's forced-change gate) before
-- reaching anything else. Always false for existing rows and freshly
-- created accounts.

ALTER TABLE accounts ADD COLUMN must_change_password INTEGER NOT NULL DEFAULT 0;
