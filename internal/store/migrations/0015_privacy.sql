-- SPDX-License-Identifier: AGPL-3.0-only
-- Copyright (c) 2026 Bahnfrei contributors
--
-- Privacy: per-athlete SYS-103 publication-consent flags and SYS-101
-- erasure/anonymization markers (TASK-023, UC-023/UC-024).
--
-- Consent defaults are the privacy-protective baseline documented on
-- domain.PublicationConsent (internal/domain/privacy.go):
--   * results_publication_withdrawn = 0 ("not withdrawn" — an athlete is
--     publicly listed by default, matching this system's behaviour before
--     consent tracking existed and standard federation practice of
--     publishing competition results as the sporting record; an explicit
--     withdrawal, e.g. for a minor, suppresses identity on public surfaces
--     going forward, SYS-103/UC-023 #2).
--   * photo_consent_given / extended_data_consent_given = 0 ("not given")
--     — opt-in, since no lawful basis exists to presume either.
--
-- anonymized/anonymized_at record a completed SYS-101 erasure request
-- (internal/domain/privacy.go Athlete.AnonymizePersonalData) so the
-- retention purge job and any future re-run never reprocess an
-- already-anonymized athlete.

ALTER TABLE athletes ADD COLUMN results_publication_withdrawn INTEGER NOT NULL DEFAULT 0;
ALTER TABLE athletes ADD COLUMN photo_consent_given INTEGER NOT NULL DEFAULT 0;
ALTER TABLE athletes ADD COLUMN extended_data_consent_given INTEGER NOT NULL DEFAULT 0;
ALTER TABLE athletes ADD COLUMN consent_recorded_at TEXT;
ALTER TABLE athletes ADD COLUMN consent_recorded_by TEXT NOT NULL DEFAULT '';
ALTER TABLE athletes ADD COLUMN anonymized INTEGER NOT NULL DEFAULT 0;
ALTER TABLE athletes ADD COLUMN anonymized_at TEXT;
