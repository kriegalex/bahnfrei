-- SPDX-License-Identifier: AGPL-3.0-only
-- Copyright (c) 2026 Bahnfrei contributors
--
-- Out-of-competition participants (TASK-036, DEC-016/OQ-020 investigation
-- of the LV Langenthal Gesamtrangliste, 17.05.2025 —
-- https://lvl.ch/images/resultate/2025/Gesamtrangliste_UBSKidsCup_2025.pdf):
-- one row (Thome Lauriane, W12) has every discipline mark present but is
-- still shown unranked, total "n.a." — the observed TAF3 convention for an
-- athlete competing ausser Konkurrenz/hors concours (e.g. a guest, or an
-- athlete outside the division's eligibility). participants.out_of_competition
-- lets the office flag a registration that way; internal/app's Standings/
-- FinalStandings (domain.CombinedStanding.OutOfCompetition) then keeps that
-- athlete's marks visible but never assigns them a numeric rank, in
-- provisional or final standings alike. Defaults to 0 so every pre-existing
-- and ordinary RegisterParticipant call is unaffected.

ALTER TABLE participants ADD COLUMN out_of_competition INTEGER NOT NULL DEFAULT 0;
