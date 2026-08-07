# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versioning follows
[Semantic Versioning](https://semver.org/) as documented in `docs/ops/release-process.md`.

## [Unreleased]

### Added

- Admin-issued password reset: an instance administrator can set a new temporary password
  for any locked-out account from the accounts page. It signs that account out of every
  active session, and the next login is forced through a change-password step before
  reaching anything else — no email or outside network access required.

## [0.1.0] — first public release

The first end-to-end release: a complete athletics-meet lifecycle on one self-hosted,
self-contained binary.

### Meet preparation

- Meet, venue, event programme, and timetable setup entirely in the browser.
- Online entries for individuals, clubs, and relays — entry windows, optional licence
  numbers, per-event limits, fee summaries.
- Entry import from CSV with mapping profiles and eligibility validation.
- Category schemes and discipline catalogs as versioned data files; Swiss Athletics
  categories and the UBS Kids Cup format built in.
- Heat seeding with World Athletics TR 20.4 lane draws, and round progression.

### Competition day

- Result capture for all discipline families: track times, horizontal attempt series,
  vertical height progression, and combined events with official scoring tables.
- Phone-first capture for volunteer officials: fully usable at 360 px with no horizontal
  scrolling, large touch targets, numeric keyboards with one-tap X/–/r markers, and
  visible pending/confirmed save states with live result/points updates.
- Offline-tolerant capture: a durable local queue replays in order after connectivity
  loss; rejected saves are explained at the exact cell with a correct-or-discard choice
  and never block other saves; an expired session prompts re-login with the queue intact.
- Check-in with DNS handling, bulk "mark remaining as DNS", and keyboard-only operation
  of every operator flow.
- FinishLynx timing integration: start lists out (`.ppl`/`.sch`/`.evt`), results in
  (`.lif`) with conflict resolution, plus a watched-folder agent mode for the timing PC.
- Result corrections with mandatory reasons and a complete audit trail; participant
  identity correction with automatic category re-derivation; records and PB/SB flagging.
- Official UBS Kids Cup standings semantics, including the final-list convention for
  athletes missing a discipline and out-of-competition participants.

### Publication

- Live public results over server-sent events with stable URLs, name/bib/club filtering,
  and per-category jump navigation.
- Printable capture sheets and result lists (PDF).
- UBS Kids Cup series-upload export and the open `omx/v1` meet exchange format.

### Operations, privacy, and security

- Single-binary install with automatic TLS (self-signed for venue use, ACME for public
  hubs), one-directory backup/restore, and a seeded demo meet (`bahnfrei demo`).
- Privacy tooling for Swiss/EU law (nFADP, GDPR): data-minimized public pages, publication
  consent enforcement, subject-access export, erasure, retention purge.
- Accounts and roles (organizer, competition office, field official, entry submitter)
  with per-event official scoping and a privileged-action audit log.
- German and French user interface throughout; contextual help on non-obvious inputs;
  inline validation errors that preserve input; confirmation pages for destructive and
  bulk actions showing the affected count.
- Security review against OWASP ASVS L2; release binaries for five OS/architecture
  targets plus a container image on GHCR, with checksums and cosign signatures.
- Operator documentation set: quickstart, runbook (including a meet-day network kit),
  privacy guide, support matrix, defect policy, release process.

### Known limitations

- Relay results cannot be captured yet (relay entries, team composition, and bibs work).
- Athlete erasure is pseudonymization, not full anonymization, by design — see
  `docs/ops/privacy.md` §1.5.
