# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versioning follows
[Semantic Versioning](https://semver.org/) as documented in `docs/ops/release-process.md`. Entries
reference the `TASK-###`/`SYS-###`/`UC-###` IDs used throughout this repository — see
`docs/requirements/traceability-matrix.md` for the full trace.

## [Unreleased]

Nothing yet since 0.1.0.

## [0.1.0] — first public release

The first end-to-end release: a complete athletics-meet lifecycle on one self-hosted, self-
contained binary — online entries, seeding, competition-day capture (track/field/vertical/
combined events), FinishLynx timing exchange, live public results, records, privacy controls,
DE/FR i18n, and accessibility — hardened and documented for public use per SYS-146.

### Added

- **Foundation (M0):** storage layer (`modernc.org/sqlite`, WAL, single-writer, optimistic
  versioning, append-only audit log), domain core (categories, disciplines, seeding rules), server
  shell with sessions/CSRF/security headers, CI gates (build, vet, race-tested tests, coverage,
  license headers, style tokens).
- **UBS Kids Cup PoC spine (M1):** meet/venue/timetable setup (UC-001), category schemes (UC-002),
  entry import, seeding (TR20.4 lane draws), competition-day capture with offline-tolerant field
  devices (UC-034, SYS-085–087), FinishLynx timing exchange (ADR-006), live public results (SSE),
  printed capture sheets, the UBS Kids Cup scoring template, and the official series upload file.
- **Full club-meet coverage (M2):** online entries (UC-003), entry import from file (UC-004),
  check-in, all discipline families (track, horizontal/vertical field, combined events), records
  and PB/SB flagging, team/relay handling, accounts/roles/privileged-action audit (UC-022),
  privacy controls — subject-access export, erasure/pseudonymization, consent enforcement,
  retention purge (SYS-100–105), DE/FR internationalization with a pseudo-locale CI check
  (SYS-110), WCAG 2.2 AA accessibility work, and the `omx/v1` open exchange schema (ADR-005).
- **Hardening & release 0.1 (M3):** OWASP ASVS L2 security review, a documented design system with
  a CI style-conformance check and component gallery (SYS-116), keyboard-only operator efficiency
  coverage (SYS-114), contextual help and inline field-validation errors (SYS-115/117),
  destructive-action confirmation flows (OQ-074), performance/recovery drills (SYS-120/121/130),
  per-OS release artifacts and a container image (`scripts/build-release.sh`, SYS-131/146), the
  operator documentation set under `docs/ops/` (quickstart, runbook, privacy, support matrix,
  defect policy, release process — SYS-104/131/132/143/146), and replacement of the remaining M0/M1
  scaffolding placeholders (the hub landing page, the CLI's no-subcommand usage text).
- **Founder-decision backlog (DEC-015…026):** pooled public read path meeting the SYS-122
  2,000-viewer budget with a per-meet render cache (TASK-035, ADR-004 §9); official UKC
  final-standings semantics — never-attempted unranked, no-valid-attempt 1-point floor,
  out-of-competition participants (TASK-036, DEC-016); tag-triggered release publication to
  GHCR with cosign keyless signing of image and checksums (TASK-037, DEC-017/018/019);
  roster/bib search by name, bib or club (TASK-038, DEC-021); optional licence numbers on
  online entries (TASK-039, DEC-023); field-event correction UI for horizontal and vertical
  grids (TASK-040, DEC-024); bulk "mark remaining as DNS" on open track units (TASK-041,
  DEC-025); a role-aware "my assignments" home for office, field officials and entry
  submitters (TASK-042, DEC-025); an office-reachable, capability-filtered meet hub
  (TASK-043); and rule-data verification against the Swiss Athletics WO 2026 and WA CR&TR
  2026 primary sources (youth discipline limits, TR 39 combined-events ties, TR 20.4 lane
  groups).
- **Volunteer usability wave (M4, TASK-044…050, from the 2026-08 volunteer walkthrough):**
  truthful capture-sync failure handling — per-operation rejections rendered at the cell with
  correct-or-discard, session-expiry re-authentication with the queue preserved, and a status
  indicator that can never contradict per-cell state (SYS-149, UC-040); phone-operable capture
  for every discipline family — no horizontal scrolling at 360 px, ≥44 px touch targets,
  numeric virtual keyboards with X/–/r quick-action marker buttons, visible pending/confirmed
  save badges and live result/points cells (SYS-147/148, UC-039); task-first navigation with
  localized, schedule-annotated assignments and a grouped meet hub (SYS-151, UC-041);
  honest empty states and inapplicable-action gating with affected-row counts on bulk
  confirmations (SYS-152); public find-your-athlete filtering and per-category jump
  navigation (SYS-153, UC-042); audited, version-guarded participant identity correction
  with automatic category re-derivation (SYS-150, UC-043); and a localization/hygiene pass
  (localized discipline names on operator surfaces, local-time timestamps, favicon, clean
  browser console).

### Known limitations at 0.1.0

- Erasure is pseudonymization, not full anonymization, by spec design (SYS-101) — see
  `docs/ops/privacy.md` §1.5.
- The requirements added from the 2026-08 volunteer walkthrough (STR-046, SYS-147–153) are
  implemented and test-evidenced but remain marked *proposed* pending founder ratification —
  see `docs/requirements/traceability-matrix.md`.
