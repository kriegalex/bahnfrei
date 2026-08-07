# Bahnfrei

[![Release](https://img.shields.io/github/v/release/kriegalex/bahnfrei)](https://github.com/kriegalex/bahnfrei/releases/latest)
[![CI](https://github.com/kriegalex/bahnfrei/actions/workflows/ci.yml/badge.svg)](https://github.com/kriegalex/bahnfrei/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/kriegalex/bahnfrei)](https://goreportcard.com/report/github.com/kriegalex/bahnfrei)
[![License: AGPL-3.0](https://img.shields.io/badge/License-AGPL--3.0--only-blue.svg)](LICENSE)

**Bahnfrei** (from the starter's call *"Bahn frei!"* — "track clear!") is an open-source,
self-hosted management system for athletics (track & field) meets: meet setup, online
entries, seeding, competition-day result capture, timing-system integration, live results,
records, and federation reporting.

It is built for the way real meets actually run: volunteer officials capture results on
their own phones, the venue network fails at the worst possible moment, and the results
still have to be right. Clubs run their own instance — a single binary, no subscription,
no cloud dependency.

## Features

### Meet preparation

- Meet, venue, event programme, and timetable setup entirely in the browser — no
  configuration files.
- Online entries for individuals, clubs, and relays, with entry windows, optional licence
  numbers, per-event limits, and fee summaries.
- Entry import from CSV with mapping profiles and eligibility validation.
- Category schemes and discipline catalogs as versioned data files — Swiss Athletics
  categories and the UBS Kids Cup format ship built in.
- Heat seeding and lane draws following World Athletics TR 20.4, with round progression.

### Competition day

- Result capture for every discipline family: track times, horizontal attempt series,
  vertical height progression, and combined events with official scoring tables.
- Designed for officials' phones: responsive from 360 px, large touch targets, numeric
  keyboards with one-tap letter markers (X/–/r), visible pending/confirmed save states.
- Offline-tolerant capture: a durable local queue replays in order after connectivity
  loss; rejected entries are explained at the exact cell with a correct-or-discard choice,
  and never block other saves.
- Check-in with DNS handling and bulk actions; every operator flow works keyboard-only.
- FinishLynx timing integration: start lists out (`.ppl`/`.sch`/`.evt`), results in
  (`.lif`) with conflict resolution, including a watched-folder agent mode for the timing
  PC using the same binary.
- Result corrections with reasons and a complete audit trail; records and PB/SB flagging.

### Publication

- Live public results with server-sent updates, stable URLs, name/bib/club filtering, and
  per-category navigation — usable on a phone from the stands.
- Printable capture sheets and result lists (PDF).
- UBS Kids Cup series-upload export and an open, documented meet exchange format
  ([omx/v1](docs/schemas/omx-v1.md)).

### Privacy

- Built for Swiss and EU privacy law (nFADP, GDPR): public pages are data-minimized,
  publication consent is enforced, and subject-access export, erasure, and retention
  purge are first-class operator actions — see the [privacy guide](docs/ops/privacy.md).

### Technical

- A single self-contained Go binary; SQLite storage with WAL, a strict single-writer
  design, and an append-only audit log.
- Server-rendered HTML with HTMX and SSE; small TypeScript islands only where the UI
  needs them. No SPA framework.
- German and French out of the box; adding a language is a translation file, not a code
  change.
- Automatic TLS: locally generated certificates for offline venue use, ACME/Let's Encrypt
  for internet-facing installs.
- Serves 2,000 concurrent live-results viewers on modest hardware.
- Container images on GHCR; release binaries for Linux, macOS, and Windows with
  checksums and cosign signatures.

## Is Bahnfrei right for your meet?

Bahnfrei is aimed at meets where the organizer chooses the tooling: club meets, youth
series such as the UBS Kids Cup, and school sports days. Honest current limitations:

- **Relays** — entries, team composition, and bibs work; relay *result capture* is not
  implemented yet.
- **Swiss championship-tier meets** — the federation's competition rules mandate a
  specific system (TAF3) for official and championship competitions, so Bahnfrei cannot
  be the system of record there today.
- **Connectivity** — the venue needs an internet uplink; brief outages are handled
  (capture keeps working and syncs when the connection returns), but a fully offline
  venue mode is still on the roadmap.
- **Languages** — the user interface ships in German and French; the documentation is in
  English.
- **Para athletics** — para classifications are not supported yet.

## Getting started

Grab a [release binary](https://github.com/kriegalex/bahnfrei/releases/latest) for your
OS, or use the container image:

```
docker run -d --name bahnfrei -p 8443:8443 -v bahnfrei-data:/data ghcr.io/kriegalex/bahnfrei:latest
```

Or build from source (Go ≥ 1.26):

```
go build -o bahnfrei ./cmd/bahnfrei
./bahnfrei serve --data-dir ./data
```

Then open **https://localhost:8443**. In the default venue mode the TLS certificate is
locally generated and self-signed (works fully offline) — your browser warns once; accept
it. Internet-facing installs use `--role hub --acme-domain your.domain --acme-email
you@example.org` for a publicly trusted certificate instead.

The first visit walks you through creating the admin account; everything after that
happens in the operator UI. To explore with realistic data first, `bahnfrei demo` seeds a
complete demo meet.

All state lives in the data directory — back up that one directory and you have the whole
meet. The full walkthrough, including artifact verification, is in the
[quickstart](docs/ops/quickstart.md).

## Documentation

**Running an instance:**

- [Quickstart](docs/ops/quickstart.md) — installation to first meet in under 30 minutes.
- [Operator runbook](docs/ops/operator-runbook.md) — roles, the recommended meet-day
  network kit, timing-agent setup, backup and restore.
- [Privacy guide](docs/ops/privacy.md) — data-processing overview, a template privacy
  notice, and controller guidance.
- [Support matrix](docs/ops/support-matrix.md) · [Defect policy](docs/ops/defect-policy.md)
  · [Release process](docs/ops/release-process.md) · [Changelog](CHANGELOG.md)

**How it's built:** the project is spec-driven. Stakeholder and system requirements, use
cases with executable acceptance criteria, and a full requirements-to-test
[traceability matrix](docs/requirements/traceability-matrix.md) live under
[`docs/requirements/`](docs/requirements/); architecture decisions are recorded as ADRs
under [`docs/architecture/adr/`](docs/architecture/adr/). Domain research on how athletics
competitions work — rules, categories, timing, records — is under
[`docs/research/`](docs/research/).

## Building and testing

```
go build ./...
go test ./...
```

HTML templates (`*.templ`) are generated and checked in; after editing them, regenerate
with the templ version pinned in `go.mod`. `scripts/check-gate.sh` runs the full
CI-equivalent gate locally: lint, security scans, race-enabled tests with coverage floors,
design-token conformance, and the Playwright end-to-end suite.

## Licence and contributing

- Code: [AGPL-3.0-only](LICENSE) — the reasoning is in
  [ADR-001](docs/architecture/adr/ADR-001-license-agpl-3.0.md).
- Documentation (`docs/`): CC-BY-SA-4.0.
- Contributions are welcome under the DCO (no CLA): see
  [CONTRIBUTING.md](CONTRIBUTING.md), [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md), and
  [GOVERNANCE.md](GOVERNANCE.md). The project name and logo are held by the maintainer
  and are not covered by the code licence.
