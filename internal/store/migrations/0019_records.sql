-- SPDX-License-Identifier: AGPL-3.0-only
-- Copyright (c) 2026 Bahnfrei contributors
--
-- Records & bests flagging (TASK-022, UC-016, SYS-049-051):
--
--   meet_record_lists   names, per meet, which loadable record lists
--                        (domain.RecordList — meeting/national/area/world
--                        reference marks, D6.1) evaluate that meet's
--                        results, mirroring 0014_combined_scoring.sql's
--                        per-meet side-table precedent rather than a new
--                        `meets` column: a meet with no rows here still
--                        gets PB/SB flagging from in-system athlete history
--                        (SYS-049), just no reference-record flagging. A
--                        meet may load more than one list at once (e.g. its
--                        own meeting-record list plus a shared national
--                        list), so this is a plain many-to-many join table,
--                        not a single foreign key.
--
-- results.record_flags already exists (0004_athletes_results.sql) as a
-- JSON array column — no results-table change needed; this migration only
-- adds the meet->record-list association.

CREATE TABLE meet_record_lists (
    meet_id        TEXT NOT NULL REFERENCES meets(id),
    record_list_id TEXT NOT NULL,
    PRIMARY KEY (meet_id, record_list_id)
);
