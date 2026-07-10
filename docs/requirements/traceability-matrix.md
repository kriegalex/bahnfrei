# Requirements Traceability Matrix

**Document status:** Phase A baseline. Chain: `STR → SYS → UC → test → verification method`.
The **test** column is populated in Phase B (test IDs/paths); at baseline it is `Phase B`.
**Verification methods:** T = automated Test, D = Demonstration (scripted manual),
I = Inspection (artifact/document review), A = Analysis (benchmark/load/measurement).

## 1. Stakeholder → System requirements (forward coverage)

Every STR maps to ≥1 SYS. (Priorities per StRS §3.)

| STR | System requirements |
|-----|---------------------|
| STR-001 | SYS-001, SYS-002, SYS-003 |
| STR-002 | SYS-004, SYS-071 |
| STR-003 | SYS-006 |
| STR-004 | SYS-001, SYS-120 |
| STR-005 | SYS-010, SYS-011, SYS-012, SYS-015 |
| STR-006 | SYS-013 |
| STR-007 | SYS-005, SYS-014, SYS-015 |
| STR-008 | SYS-016, SYS-028, SYS-046 |
| STR-009 | SYS-017 |
| STR-010 | SYS-018 |
| STR-011 | SYS-025 |
| STR-012 | SYS-026, SYS-027, SYS-028, SYS-030, SYS-121 |
| STR-013 | SYS-029, SYS-030 |
| STR-014 | SYS-040, SYS-041, SYS-045, SYS-050, SYS-142 |
| STR-015 | SYS-042, SYS-043 |
| STR-016 | SYS-031, SYS-044, SYS-053, SYS-121, SYS-142 |
| STR-017 | SYS-048 *(Later)* |
| STR-018 | SYS-060, SYS-061, SYS-062, SYS-063 *(L)*, SYS-064 *(L)*, SYS-078 *(L)* |
| STR-019 | SYS-046, SYS-047 |
| STR-020 | SYS-080 *(L — DEC-013)*, SYS-082 *(L)*, SYS-085, SYS-087, SYS-093, SYS-130 |
| STR-021 | SYS-083, SYS-085, SYS-086 |
| STR-022 | SYS-070, SYS-071, SYS-074, SYS-113, SYS-122 |
| STR-023 | SYS-072, SYS-111 |
| STR-024 | SYS-041, SYS-049, SYS-050, SYS-051, SYS-142 |
| STR-025 | SYS-073, SYS-077, SYS-078 *(L)* |
| STR-026 | SYS-073 |
| STR-027 | SYS-075 *(Later)* |
| STR-028 | SYS-070 |
| STR-029 | SYS-003, SYS-005, SYS-010, SYS-052 |
| STR-030 | SYS-052, SYS-010 (para extension: Later, DEC-007) |
| STR-031 | SYS-091, SYS-092, SYS-093, SYS-100, SYS-101, SYS-102, SYS-104, SYS-105 |
| STR-032 | SYS-100, SYS-102, SYS-103 |
| STR-033 | SYS-074, SYS-110, SYS-111 |
| STR-034 | SYS-112, SYS-113 |
| STR-035 | SYS-114, SYS-120, SYS-131 |
| STR-036 | SYS-092, SYS-132, SYS-133, SYS-146, CON-02 |
| STR-037 | SYS-062, SYS-073, SYS-078 *(L)*, SYS-105, SYS-144 |
| STR-038 | SYS-092, SYS-104, SYS-140, SYS-141, SYS-142, SYS-143, SYS-144, SYS-145, SYS-146 |
| STR-039 | SYS-084, SYS-131, SYS-132 |
| STR-040 | SYS-090, SYS-091, SYS-086 |
| STR-041 | SYS-081, SYS-084, SYS-085, SYS-086, SYS-087, SYS-130 |
| STR-042 | SYS-076, SYS-077, SYS-078 *(L)* |
| STR-043 | SYS-053, SYS-077 |

**Coverage check:** 43/43 STR covered. Zero orphan stakeholder requirements.
*(2026-07-05 delta: STR-042/043 added from founder answers; STR-030 re-scoped per DEC-007.)*

## 2. System requirements → Use-cases → verification

Every SYS maps to ≥1 STR (see §1) and is verified via a UC criterion, a CI gate, or an
inspection/analysis procedure.

| SYS | Verified by (UC / procedure) | Method | Test (Phase B) |
|-----|------------------------------|--------|----------------|
| SYS-001 | UC-001 #2 | T | TASK-006 (meet setup): `internal/app` `TestCreateMeetUC001_2`, `TestUpdateAndArchiveMeet`, `TestCreateMeetAuthorization`; `internal/web` `TestMeetCreationUC001_2`, `TestMeetEditConflictAndArchive`; `internal/store` `TestMeetCreateAndGet`, `TestSessionsPerDay`; `cmd/bahnfrei` `TestQuickstartFreshInstallE2E` |
| SYS-002 | UC-001 #3 | T | TASK-006 (meet setup): `internal/app` `TestAddEventsUC001_3`, `TestAddEventValidation`; `internal/web` `TestEventProgrammeUC001_3`; `internal/store` `TestEventProgramme`; `cmd/bahnfrei` `TestQuickstartFreshInstallE2E` |
| SYS-003 | UC-002 #5 | T | TASK-004 (domain core): `internal/domain` `TestDisciplineCatalog_SYS003Coverage` (all 5 SyRS §2 families + core WA codes present), `TestDiscipline_CategoryCorrectTechnicalVariant`, `TestDiscipline_HurdleVariants` (per-category technical variants), `TestDisciplineCatalog_UBSKidsCupDisciplines`; organizer-defined custom disciplines proven generic by `TestParseDisciplineCatalog_CustomCatalogNoCodeChange` |
| SYS-004 | UC-001 #4 | T | TASK-006 (meet setup): `internal/app` `TestTimetablePublishAndAmendUC001_4`; `internal/web` `TestTimetablePublishAmendPublicUC001_4`; `internal/store` `TestTimetablePublishRetainsVersions`; `cmd/bahnfrei` `TestQuickstartFreshInstallE2E` |
| SYS-005 | UC-002 #1–#4 | T | TASK-004 (domain core): `internal/domain` `TestResolveDefaultCategory_SwissAthletics_UC002_1`, `TestResolveDefaultCategory_CalendarYearTransition`, `TestEvaluateEntry_StartUpAndDisciplineBar`, `TestParseCategoryScheme_CustomSchemeNoCodeChange` (built-in Swiss Athletics + UBS Kids Cup schemes, data-interpreter resolver, custom-scheme load) |
| SYS-006 | UC-001 #5 | T | TASK-006 (meet setup): `internal/app` `TestSanctioningSummaryUC001_5`; `internal/web` `TestSanctioningSummaryUC001_5Web`; `cmd/bahnfrei` `TestQuickstartFreshInstallE2E` |
| SYS-010 | UC-004 #1, UC-028 #1 | T | Phase B |
| SYS-011 | UC-003 #1–#3 | T | Phase B |
| SYS-012 | UC-003 #4 | T | Phase B |
| SYS-013 | UC-004 #1–#4 | T | Phase B |
| SYS-014 | UC-005 #1–#5 | T | Phase B |
| SYS-015 | UC-003 #5 | T | Phase B |
| SYS-016 | UC-008 #5 (scratch), UC-015 #3 (audit) | T | Phase B |
| SYS-017 | UC-006 #3 | T | Phase B |
| SYS-018 | UC-006 #1–#2 | T | Phase B |
| SYS-025 | UC-007 #1–#3 | T | Phase B |
| SYS-026 | UC-008 #1–#2 | T | Phase B |
| SYS-027 | UC-008 #3–#4 | T | Phase B |
| SYS-028 | UC-008 #5 | T | Phase B |
| SYS-029 | UC-009 #1–#3 | T | Phase B |
| SYS-030 | UC-009 #4 | T | Phase B |
| SYS-031 | UC-013 #1 | T | Phase B |
| SYS-040 | UC-010 #1 | T | Phase B *(partial: TASK-008 proves 0.01 s FAT-resolution capture for manually entered electronic times — `internal/domain` `TestValidateFATTime`; `internal/app` `TestUC010_2_HandTimeRoundUpAndProvenance`; per-race wind, finishing order and reaction times land with UC-010 full track capture, TASK-019/020)* |
| SYS-041 | UC-010 #2, #5 | T | TASK-008 (field & track capture): `internal/domain` `TestRoundUpHandTime` (D5.1 round-up fixtures); `internal/app` `TestUC010_2_HandTimeRoundUpAndProvenance` (manual vs electronic scoring columns, provenance kept); `internal/web` `TestUC010_TrackCaptureFlow` (hand marker "h" on every rendered output) — road-event whole-second rounding: TASK-019 |
| SYS-042 | UC-011 #1–#4 | T | TASK-008 (field & track capture): `internal/domain` `TestAttemptValidate` (0.01 m marks, X/–/r, wind only where relevant), `TestRankFieldSeries_NextBestTieBreak`/`_IdenticalSeriesShareRank` (UC-011 #2), `_RetireeStillRanks` (UC-011 #3), `TestFieldContinuation_CutTop8`/`_BoundaryTieAllAdvance`/`_RetireeAndNMExcluded` (UC-011 #1); `internal/app` `TestUC011_FieldCaptureGridAndTieBreak` (UC-011 #4 recompute-on-save), `TestUC011_1_DefaultSeriesCutAndContinuation`, `TestUC011_3_RetireeStillRanks`, `TestCaptureValidationAndAuthorization`, `TestOnResultsChangedHook`; `internal/store` `TestSaveAttemptInsertAndRecapture`, `TestSaveAttemptConflicts`, `TestListUnitAttemptsOrdering`; `internal/web` `TestUC011_CaptureGridFlow` (grid + live standings over SSE) |
| SYS-043 | UC-012 #1–#4 | T | Phase B |
| SYS-044 | UC-013 #1–#3 | T | Phase B |
| SYS-045 | UC-010 #3 | T | TASK-008 (field & track capture): `internal/domain` `TestValidateCaptureStatus` (CR 25 capture vocabulary; DQ requires rule reference), `TestRenderStatus` ("DQ (TR16.8)" convention); `internal/app` `TestUC010_3_StatusVocabulary`; `internal/store` `TestResultStatusDetailRoundTrip`; `internal/web` `TestUC010_TrackCaptureFlow` — qualification codes (Q/q/…) render with round progression, TASK-018 |
| SYS-046 | UC-015 #2–#3 | T | TASK-003 (storage layer): `internal/store` `TestAuditImmutableByTrigger`, `TestAuditAppendAndTrail`; correction-propagation flow lands with TASK-019 |
| SYS-047 | UC-015 #1, #4 | T | Phase B |
| SYS-048 *(L)* | UC-029 | T | Phase B (Later) |
| SYS-049 | UC-016 #1, #5 | T | Phase B |
| SYS-050 | UC-010 #4, UC-016 #2–#3, UC-013 #4 | T | Phase B |
| SYS-051 | UC-016 #4 | T | Phase B |
| SYS-052 | UC-028 #1–#2 (para: UC-028 #3, Later) | T | Phase B *(partial: TASK-007 proves the per-division split of a mixed-division field for UKC standings — `internal/app` `TestUC033_2_ScoringAndStandings`; combined-race presentation and re-ranked per-category extraction land with UC-028/TASK-022)* |
| SYS-053 | UC-033 #1–#5 | T | TASK-007 (UKC template & scoring): `internal/domain` `TestUKCScoringOfficialFixtures` (fixtures from the published Wertungstabelle incl. next-lower-points rounding), `TestScoringTableIsData` + `TestUC033_5_ScoringTableSwapAtServiceLevel` (UC-033 #5 data swap), `TestBuiltinUKCTemplate`, `TestRankCombinedReglementRules`/`TestRankCombinedHighestSingleTieBreak`/`TestRankCombinedTrueTie`/`TestRankCombinedMissingDiscipline` (Reglement Rangierung; UC-033 #3 per OQ-020); `internal/app` `TestUC033_1_CreateUKCMeetFromTemplate`, `TestUC033_2_ScoringAndStandings`, `TestUC033_3_MissingDiscipline`, `TestResultsAuthorizationAndValidation`; `internal/store` `TestSaveResultUpsertAndListing`, `TestParticipantsAndBibUniqueness`; `internal/web` `TestUKCTemplateRosterStandingsFlow` |
| SYS-060 | UC-014 #1–#2 | T | Phase B |
| SYS-061 | UC-014 #3–#5 | T | Phase B |
| SYS-062 | UC-014 #6 | T | Phase B |
| SYS-063 *(L)* | UC-032 | T | Phase B (Later) |
| SYS-064 *(L)* | UC-031 | T | Phase B (Later) |
| SYS-070 | UC-017 #2–#3 | T | TASK-010 (public live results, PoC scope): `internal/web` `TestPublicPagesAnonymousAccessSYS070UC017_2` (overview/timetable/start lists/results fetch with no session cookie, 200, no login wall, viewport meta present), `TestPublicPagesArchivedMeetUC017_3` (same stable `/m/{id}/...` URLs still 200 with results after archiving) |
| SYS-071 | UC-017 #1 | T/A | T: TASK-010: `internal/web` `TestPublicResultsLiveUpdateSYS071UC017_1` (`ResultsService.SaveResult` publishes on the meet's SSE topic synchronously with the save — asserted via a non-blocking channel receive immediately after `SaveResult` returns — and a fresh fetch of the public results live-refresh fragment shows the new mark). A (load-bearing p95 latency under concurrent load): TASK-027 (M3, SYS-122) |
| SYS-072 | UC-018 #1–#3 | T | TASK-011 (printables, PoC scope): `internal/pdf` (generic, domain-agnostic renderer over `codeberg.org/go-pdf/fpdf`) `TestBuildCaptureSheetGridSYS072`, `TestBuildTrackLaneSheetSYS072`, `TestBuildResultListMultiDivisionSYS072`, `TestBuildEmptySectionRendersHeaderOnly`, `TestBuildAccentedTextRoundTrips` (all parse the generated bytes back to text via `github.com/ledongthuc/pdf`, asserting real document structure, not just non-empty output); `internal/web` `TestCaptureSheetPDFFieldGridSYS072UC018_1` (horizontal-attempt grid: meet/discipline header, generation timestamp, trial columns, captured mark plus a still-blank trial — UC-018 #1), `TestCaptureSheetPDFTrackLaneSYS072UC018_1` (track lane sheet: bib/name populated, lane/time left blank for manual capture), `TestResultListPDFSYS072UC018_2` (UKC result list: division heading, rank/bib/name/total, reuses TASK-007's standings computation unchanged — UC-018 #2), `TestCaptureSheetPDFRequiresFieldOfficialRoleSYS072`/`TestResultListPDFRequiresOfficeRoleSYS072` (anonymous 403, SYS-090) *(PoC scope: only track and horizontal-field capture sheets plus the UKC division result list, matching `ResultsService.CaptureUnits`' current unit coverage; vertical-jump height columns and start lists land with TASK-021/M2. Track lane sheets render blank lane cells for hand-writing — lane draws are not modeled until TASK-018/M2. Document headings use the same `discipline.<code>`/domain-glossary catalog keys as the standings page (UC-018 #3), so DE/FR coverage matches SYS-074's PoC scope — see docs/requirements/open-questions-and-assumptions.md)* |
| SYS-073 | UC-027 #1–#3 | T | Phase B |
| SYS-074 | UC-017 #2 + UC-025 #1 | T | TASK-010: `internal/web` `TestPublicResultsLocalizedDisciplineLabelsSYS074` (public results page renders discipline names from the DE/FR `discipline.<code>` catalog keys, not the catalog's canonical English name) *(PoC scope: discipline localization keys are shipped for the disciplines exercised by the built-in template and test fixtures, not the full catalog — see docs/requirements/open-questions-and-assumptions.md; category codes, e.g. "U16 W"/"M7", are treated as locale-invariant identifiers throughout the app, consistent with the existing office standings/roster pages)* |
| SYS-075 *(L)* | UC-030 | T | Phase B (Later) |
| SYS-076 | UC-017 #5 | T | TASK-010: `internal/store` `TestMeetResultsPositioningSYS076` (default federation_official on create, organizer override to primary with source name/URL round-tripping through `UpdateMeet`, invalid positioning rejected by both `CreateMeet` and `UpdateMeet`); `internal/web` `TestPublicResultsSYS076Label` (federation_official carries the "unofficial results" label with the source name/link in DE and FR; primary carries no label; organizer-configurable via the meet edit form, `internal/web/meets.templ`) |
| SYS-077 | UC-035 #1–#3 | T | TASK-012 (series upload export, MVP scope only — Visana Sprint/Mille Gruyère remain *Later*): `internal/domain` `TestBuiltinUKCSeriesUploadTemplate` (shipped template shape: sheet name, identity columns cover athlete-identity fields, missing-mark placeholder), `TestParseSeriesUploadTemplateRejectsBadData`, `TestSeriesUploadTemplateIsData` (UC-035 #3 data-swap at the parser level); `internal/app` `TestSeriesUploadExportSYS077UC035_1` (exported workbook's sheet/header/column structure and per-participant marks+points+division data against the built-in template fixture), `TestSeriesUploadExportSYS077UC035_2` (missing discipline and DNS status render explicitly — never a dropped row — per the template's placeholder/status convention), `TestSeriesUploadExportSYS077UC035_3` (a revised template document changes the exported sheet/columns with no code change), `TestSeriesUploadExportNotConfigured`; `internal/web` `TestSeriesUploadDownloadSYS077UC035_1` (office-gated download route serves a real XLSX with the standings-page link). *Provenance caveat (OQ-023): the shipped template's exact column layout is an assumption — the official "Vorlage Resultatimport" file itself could not be retrieved offline; see docs/requirements/open-questions-and-assumptions.md.* |
| SYS-078 *(Later)* | UC-036 #1–#2 | T/D | Phase B |
| SYS-080 *(Later — DEC-013)* | UC-019 #1, #3 | T | Phase B (venue-node gate) |
| SYS-081 | UC-020 #1–#2 | T | TASK-003 (storage layer): `internal/store` `TestKill9Durability` (kill-9 mid-burst, confirmed-write survival, consistency-gated reopen), `TestOpenDurabilityPragmas` (WAL + synchronous=FULL); app-level drill in TASK-014 |
| SYS-082 *(Later — DEC-013)* | UC-019 #2 | T | Phase B (venue-node gate) |
| SYS-083 | UC-021 #1–#3 | T | TASK-003 (primitive): `internal/store` `TestOptimisticUpdateConflictSurfaced` (conflict surfaced, stale write rejected); TASK-008 (capture path, UC-021 #2): `internal/store` `TestSaveAttemptConflicts`, `internal/app` `TestCaptureConflictSurfaced` (both versions surfaced, never silent last-write-wins); full 10-operator suite in TASK-025 |
| SYS-084 | UC-020 #3 | T | Phase B |
| SYS-085 | UC-034 #1–#3, #6 | T | TASK-009 (offline capture queue): `internal/app` `TestUC034_2_IdempotentReplay` (exactly-once, ordered replay under repeated batches), `TestUC034_2_OpIDReuseAcrossUnitsFails` (cross-unit op-id reuse fails loudly, never a false duplicate); `internal/store` `TestCaptureOpDedupeLedger`; `internal/web` `TestUC034_2_IdempotentReplayOverJSON`; Playwright `e2e/tests/uc-034.spec.ts` "UC-034 #1" (capture continues offline with indicator, network cut mid-event), "#2" (auto in-order sync; flaky-reconnect no-duplicates incl. dropped-ack replay), "#3" (offline reload/browser-restart survival via service worker + IndexedDB queue), "#6" (transport-agnostic by construction: same-origin relative URLs, no transport-specific paths) |
| SYS-086 | UC-034 #4–#5 | T | TASK-009 (checkout + reconciliation): `internal/domain` `TestDecideSync_Routing`; `internal/store` `TestCheckoutLifecycle`, `TestReconciliationItemsRoundTrip`; `internal/app` `TestUC034_4_StartListChangeRoutesToReconciliation`, `TestUC034_5_StaleCheckoutNeverSilentlyApplied` (override bumps generation, audited per SYS-046), `TestUC034_5_WrongTokenNeverApplies` (replays accepted only from the current holder), `TestUC034_CheckoutByAnotherRequiresOverride`, `TestUC034_ReconciliationResolveApplyAndDiscard` (discard explicit + audited — anti-Web.TEC-C2.1); `internal/web` `TestUC034_5_StaleCheckoutReconcilesOverJSON`, `TestUC034_ReconciliationApplyOverJSON`; Playwright "UC-034 #4" (office start-list revision → reconciliation, nothing silently discarded), "#5" (office override → capture resumes on second device; stale device's ops land in reconciliation) |
| SYS-087 | UC-034 #7 | T | TASK-009 (blip tolerance): `internal/web` `TestLayoutWiresOfficeBanner` (degraded-state banner on authenticated operator surfaces); Playwright "UC-034 #7" (≤5-min office blip: banner shown, typed input survives, auto-resume on reconnect, no re-login — session TTL asserted > blip window) |
| SYS-090 | UC-022 #1, #3 | T | TASK-005 (RBAC primitive): `internal/app` `TestRoleAtLeast`, `TestAuthorize` (least-privilege capability checks), `TestParseRole`/`TestParseRoleInvalid`; `internal/web` `TestAdminRouteRequiresInstanceAdminRole` (anonymous 403, instance-admin 200 end to end). Per-meet grant assignment/scoping is TASK-013 |
| SYS-091 | UC-022 #2, #4 | T/I | TASK-005: `internal/app` `TestHashAndVerifyPassword`, `TestNeedsRehash`, `TestLoginUpgradesWeakHash` (adaptive argon2id hash + upgrade-on-verify), `TestSessionExpiry`, `TestSessionSweep` (configurable session TTL), `TestCreateAccountRequiresCapability` (privileged action recorded to audit trail, SYS-046); `internal/web` `TestLoginFlowSuccessAndFailure`, `TestLoginRejectsWithoutCSRFToken`, `TestAdminRouteRequiresInstanceAdminRole` (authN required for non-public capability) |
| SYS-092 | CI dependency/static scans + ASVS L2 audit checklist per release | A/I | Phase B |
| SYS-093 | UC-019 #4 + TLS config inspection | T/I | TASK-005: `internal/web` `TestGenerateSelfSignedCovers`, `TestLoadOrGenerateSelfSignedPersistsAndReuses`, `TestLoadOrGenerateSelfSignedRegeneratesWhenNearExpiry`, `TestLocalTLSConfigDefaultsToLocalhost` (venue-local self-signed cert, no internet-dependent CA), `TestBuildACMEConfigWiresIssuer`, `TestAcmeTLSConfigRequiresDomain` (hub/ACME config wiring; real ACME issuance via `ManageSync` is a deliberate offline-suite seam, not unit-tested), `TestServerServeAndShutdown` (TLS listener lifecycle end to end) |
| SYS-100 | UC-023 #1 | T | Phase B |
| SYS-101 | UC-024 #1–#2 | T | Phase B |
| SYS-102 | UC-024 #3 | T | Phase B |
| SYS-103 | UC-023 #2–#3 | T | Phase B |
| SYS-104 | Document inspection per release | I | Phase B |
| SYS-105 | Egress-blocked suite (UC-019 #3) + code/config inspection | T/I | Phase B |
| SYS-110 | UC-025 #1–#3 | T | TASK-005 (i18n mechanism): `internal/web/i18n` `TestLoadShipsCompleteDEAndFR` (DE/FR ship complete, zero missing-key fallback), `TestLoadDiscoversPseudoLocaleWithNoCodeChange` + `TestPseudoLocaleInSyncWithReference` (pseudo-locale build, translation-file-only extensibility, gated by plain `go test ./...` already in CI), `TestMissingKeyFallsBackThenBrackets`; `internal/web` `TestLocaleSwitchPersistsAcrossRequests`, `TestLocaleSwitchIgnoresUnknownLocale` (per-session switch). Locale-correct date/number/mark-notation rendering ships with the content that needs it (later tasks) |
| SYS-111 | UC-025 #1 + UC-018 #3 | T | Phase B |
| SYS-112 | UC-026 #1–#2 | T/I | Phase B |
| SYS-113 | UC-017 #2, UC-026 #1 | T | Phase B |
| SYS-114 | Keyboard-only E2E pass of check-in/result-entry flows | T/D | Phase B |
| SYS-120 | Benchmark on reference dataset (1,500 athletes / 4,000 entries / 250 units) | A | Phase B |
| SYS-121 | Benchmark (seeding 200 entries ≤10 s; recompute ≤5 s) | A | Phase B |
| SYS-122 | Load test (UC-017 #4) | A | Phase B |
| SYS-130 | UC-020 #1 (restart ≤2 min) | T | Phase B |
| SYS-131 | UC-001 #1 (timed quickstart run) | D | TASK-006: `cmd/bahnfrei` `TestQuickstartFreshInstallE2E` (automated fresh-install flow: empty data dir → setup → admin account → meet, no config file; README quickstart + Dockerfile). The human-paced ≤30-min demonstration remains for the M1 club demo (DEC-011) |
| SYS-132 | Support-matrix CI (build/run on documented platforms & browsers) | T/I | Phase B |
| SYS-133 | Dependency inventory inspection (no paid service required) | I | Phase B |
| SYS-140 | Coverage gates in CI (≥90% branch domain logic; ≥80% line overall) | A | TASK-002: `scripts/check-coverage.sh` in `ci.yml` coverage job. *Method note:* Go's cover tooling measures **statement** coverage; the ≥90%/≥80% gates are enforced on statement coverage as the agreed proxy for branch coverage. Generated code (`*_templ.go`, checked-in templ output) is excluded from the measurement (TASK-006): its authored source is the `.templ` file, whose display logic is exercised by `internal/web` rendering tests; the excluded statements are machine-emitted per-write io-error plumbing |
| SYS-141 | CI pipeline configuration inspection + gate behaviour test | I/T | TASK-002: `.github/workflows/ci.yml` — 3-OS build+test matrix, golangci-lint v2 (incl. depguard architecture rules), guarded `tsc` job, coverage gates, `go-licenses` allowlist (ADR-001 §4), `govulncheck`; SPDX+DCO in `governance.yml` (TASK-001) |
| SYS-142 | Reference fixture suites for every rule engine (UC-005 #5, UC-010 #5, UC-012 #4, UC-013 #1) | T | Phase B |
| SYS-143 | Release checklist + defect-tracker inspection per release | I | Phase B |
| SYS-144 | Schema/API docs versioned in repo; CI schema-diff check | I/T | Phase B |
| SYS-145 | ADR inventory inspection at each gate | I | Phase B |
| SYS-146 | Repository inspection before first release | I | TASK-001: `LICENSE` (AGPL-3.0-only), `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `GOVERNANCE.md`, CI SPDX+DCO gates (`.github/workflows/governance.yml`); release process pending TASK-028 |

**Coverage check:** all SYS verified; every UC traces to ≥1 SYS (see UC index). Zero orphans
in either direction.

## 3. Baseline QA status (Phase A definition-of-done self-check)

| Check | Status |
|-------|--------|
| Every requirement uniquely identified, atomic, testable | ✅ STR-001…043, SYS-001…146 (blocks), UC-001…033 |
| Zero orphan requirements (both directions) | ✅ §1–§2 above |
| Vague founder language converted to measurable targets | ✅ "well tested/bug free/state of the art" → SYS-140…146, SYS-092, SYS-120…122; no adjectives used as requirements |
| External facts cited; named systems verified by exact spelling | ✅ Seltec (not "setlec") confirmed; Swiss Athletics stack verified; SVM (not "CSI"); see `../research/*.md` |
| Assumptions explicit, open questions consolidated | ✅ `open-questions-and-assumptions.md` (OQ-001…012, A-001…012, TBD-001…009) |
| Scope boundaries stated (MVP / Later / out-of-scope) | ✅ StRS §4 |
| Implementation-free (no stack/scaffolding choices) | ✅ CON-05; verification mechanisms named only where rule- or format-defined (e.g., FinishLynx file family is an external interface, not a stack choice) |
| IDs stable, deprecation-only policy stated | ✅ headers of each document |

Known baseline limitations (tracked, non-blocking): TBD-001…009 in
`open-questions-and-assumptions.md`; Later-priority SYS/UC intentionally not decomposed further
until MVP is ratified.
