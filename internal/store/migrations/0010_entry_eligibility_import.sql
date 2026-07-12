-- SPDX-License-Identifier: AGPL-3.0-only
-- Copyright (c) 2026 Bahnfrei contributors
--
-- Entry import & eligibility (TASK-017, UC-004/UC-005, SYS-013/014/010):
--
--   entry_eligibility   the most recently computed domain.EligibilityResult
--                       for one entry (SYS-014), plus an optional operator
--                       override (UC-005 #4). One row per entry, additive to
--                       the entries table shipped by TASK-016 (0009) so this
--                       migration touches no existing table/column — kept
--                       deliberately separate to avoid any conflict with
--                       parallel entries work (TASK-018).
--
--   outcome/flags_json  the outcome ("eligible"/"warning"/"blocked") and the
--                       full domain.EligibilityFlag list from the most
--                       recent evaluation (import time, or online-submission
--                       time), recomputed (and this row replaced) whenever
--                       the entry's evaluated facts could have changed.
--   overridden_by/      the authorized operator, reason and timestamp of an
--   override_reason/    eligibility override (UC-005 #4); empty means no
--   overridden_at       override has been recorded. An override does not
--                       erase outcome/flags_json — the original evaluation
--                       stays visible for audit — the *effective* status for
--                       gating start-list generation is "eligible OR
--                       overridden" (internal/app).

-- "id" holds the entry id (1:1 with entries) rather than a separate
-- "entry_id" column so this table can use the store package's
-- OptimisticUpdate helper as-is (it assumes an "id" primary key).
CREATE TABLE entry_eligibility (
    id               TEXT PRIMARY KEY REFERENCES entries(id),
    outcome          TEXT NOT NULL DEFAULT 'eligible',
    flags_json       TEXT NOT NULL DEFAULT '[]',
    overridden_by    TEXT NOT NULL DEFAULT '',
    override_reason  TEXT NOT NULL DEFAULT '',
    overridden_at    TEXT NOT NULL DEFAULT '',
    version          INTEGER NOT NULL DEFAULT 1
);
