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

### Known limitations at 0.1.0

- **SYS-122's 2,000-concurrent-public-viewer target is not met** on the current read
  architecture — measured, honestly-supportable capacity is on the order of 100 concurrent
  viewers on reference-class hardware. See `docs/ops/support-matrix.md` §3 and
  `docs/requirements/open-questions-and-assumptions.md` OQ-066.
- Release artifacts are unsigned (SHA-256 checksums only) — see `docs/ops/release-process.md` §4
  and OQ-087.
- No container registry publication target is chosen yet — see `docs/ops/release-process.md` §5
  and OQ-086.
- Erasure is pseudonymization, not full anonymization, by spec design (SYS-101) — see
  `docs/ops/privacy.md` §1.5.
