-- SPDX-License-Identifier: AGPL-3.0-only
-- Copyright (c) 2026 Bahnfrei contributors
--
-- Combined-events scoring (TASK-021, UC-013 / SYS-044):
--
-- meet_combined_scoring names, per meet, which WA combined-events formula
-- table (domain.CombinedScoringTable, e.g. "wa-combined-events-2001")
-- scores its results — kept as its own side table rather than a new column
-- on `meets` so a combined-events meet is additive, not a schema change to
-- the shared meets table. A meet with no row here is not a combined-events
-- meet: its results/scoring (if any) go through the ordinary
-- meets.scoring_table lookup-table path (0003_meets.sql) instead. The two
-- are mutually exclusive per meet — a meet's constituent discipline codes
-- (e.g. "100m") can appear in both kinds of table, so the meet itself must
-- say which scoring mode applies.

CREATE TABLE meet_combined_scoring (
    meet_id           TEXT PRIMARY KEY REFERENCES meets(id),
    scoring_table_id  TEXT NOT NULL
);
