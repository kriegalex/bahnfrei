-- SPDX-License-Identifier: AGPL-3.0-only
-- Copyright (c) 2026 Bahnfrei contributors
--
-- Public results positioning (TASK-010, SYS-076): per meet, the organizer
-- configures whether a federation channel is the official results source
-- (every public results page/export then carries an "unofficial results"
-- label naming/linking that source) or whether this system is the primary
-- publication (no label). federation_official is the conservative default —
-- a freshly created meet never silently implies it is an authoritative
-- publication.

ALTER TABLE meets ADD COLUMN results_positioning TEXT NOT NULL
    DEFAULT 'federation_official'
    CHECK (results_positioning IN ('federation_official', 'primary'));
ALTER TABLE meets ADD COLUMN official_source_name TEXT NOT NULL DEFAULT '';
ALTER TABLE meets ADD COLUMN official_source_url TEXT NOT NULL DEFAULT '';
