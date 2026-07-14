# Bahnfrei — Open-Source Athletics Tournament Management System

**Status: Phase B — both human gates passed 2026-07-05** (requirements baseline + all six
architecture ADRs ratified, DEC-014). Implementation follows the backlog in
`docs/delivery/work-breakdown.md`.

**Bahnfrei** (from the starter's call *"Bahn frei!"* — "track clear!") is an open-source,
**AGPL-3.0-only** system to manage athletics (track & field) tournaments end-to-end: meet
setup, entries, eligibility, seeding, competition-day capture, timing-system exchange, live
results, records, and federation reporting. Swiss/EU context: nFADP+GDPR, DE/FR first,
**hub-first and self-hosted** (clubs run their own instance; no subscription SaaS), tolerant
to venue connectivity loss by design. Parallel systems studied: **Seltec** (TAF3/LA.portal)
and the **Swiss Athletics** ecosystem (Alabus, federation portals).

Architecture in one line: a single self-contained **Go + SQLite** binary serving
server-rendered HTML (HTMX/SSE) with small TypeScript islands — see `docs/architecture/adr/`.

This engagement is spec-driven and ran in two phases with hard human gates between them
(see `CLAUDE.md` / `plan.md`).

## How to read this package

Read in this order:

1. **`docs/research/domain-athletics.md`** — how athletics competitions actually work
   (rules, categories, timing, records, officiating), verified against World Athletics and
   Swiss Athletics primary sources. Sections `D1…D11`.
2. **`docs/research/competitive-analysis.md`** — the incumbent landscape (Seltec, Swiss
   Athletics' stack, timing ecosystem, data standards), gaps, and the OSS opportunity.
   Sections `C1…C6`.
3. **`docs/requirements/stakeholder-requirements.md`** (StRS) — 14 stakeholder classes and
   41 implementation-free stakeholder requirements `STR-###`, with MVP/Later priorities and
   scope boundaries (§4).
4. **`docs/requirements/system-requirements.md`** (SyRS) — testable system requirements
   `SYS-###`: functional, quantified non-functional (performance, offline, privacy,
   accessibility, i18n, quality gates), conceptual data model, interfaces, constraints.
5. **`docs/requirements/use-cases.md`** — 32 vertical slices `UC-###` with executable
   Given/When/Then acceptance criteria: the agent-facing units of work for Phase B.
6. **`docs/requirements/traceability-matrix.md`** — the zero-orphan proof:
   `STR → SYS → UC → test → verification method`, plus the Phase A QA self-check.
7. **`docs/requirements/open-questions-and-assumptions.md`** — **founder attention needed**:
   open questions `OQ-###`, working assumptions `A-###`, TBD register.
8. **`docs/requirements/glossary.md`** — domain and project terms (DE/FR equivalents).
9. **`docs/ops/`** — operator-facing documentation for running an instance: quickstart,
   operator runbook (network kit, timing-agent mode, backup/restore), privacy documentation
   (SYS-104), support matrix (SYS-132), defect policy (SYS-143), and release process (SYS-146).

## ID scheme (stable, never renumbered)

| Prefix | Layer |
|--------|-------|
| `STR-###` | Stakeholder requirement (implementation-free) |
| `SYS-###` | System requirement (testable, traced to STR) |
| `UC-###` | Use-case / vertical slice (executable acceptance criteria) |
| `ADR-###` | Architecture decision record (Phase B) |
| `TASK-###` | Work item (Phase B) |

## Phase gate (current state)

Both human gates are **passed** (2026-07-05, DEC-014): the requirements baseline is
approved and ADR-001…006 are ratified. Implementation follows the milestone plan in
`docs/delivery/work-breakdown.md` (M0 foundation → M1 UBS Kids Cup PoC → M2 full club
meet → M3 release 0.1). Remaining founder inputs are tracked in
`docs/requirements/open-questions-and-assumptions.md`.

## Quickstart (≤30 minutes from nothing to a working system)

No configuration file is ever edited: the first browser visit walks you through creating
the admin account, and everything else happens in the operator UI (SYS-131, UC-001). This is
the short version; **`docs/ops/quickstart.md`** has the full walkthrough including the
release-artifact download path and what to read next (the operator runbook).

**Option A — binary.** A per-OS release binary (built by `scripts/build-release.sh` for
`linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64`, with a
`checksums.txt` to verify against) is attached to each tagged release. Or build from source,
requires Go ≥ 1.26:

```
go build -o bahnfrei ./cmd/bahnfrei
./bahnfrei serve --data-dir ./data
```

**Option B — container.** Requires Docker (or Podman):

```
docker build -t bahnfrei .
docker run -d --name bahnfrei -p 8443:8443 -v bahnfrei-data:/data bahnfrei
```

Then, either way:

1. Open **https://localhost:8443**. In the default venue mode the TLS certificate is
   locally generated and self-signed (works fully offline, SYS-093) — your browser will
   warn once; accept it. Internet-facing hub installs use `--role hub
   --acme-domain your.domain --acme-email you@example.org` for a publicly trusted
   certificate instead.
2. You land on the **setup page**: create the admin account (username, display name,
   password ≥ 8 characters).
3. Log in and create your first meet under **Wettkämpfe / Compétitions** — venue, days,
   sessions, tier, then the event programme and timetable.

All state lives in the data directory (`bahnfrei.db` plus the TLS cache); back it up and
you have the whole meet (UC-020, TASK-014).

## Building & testing

```
go build ./...
go test ./...
```

Templates (`*.templ`) are pre-generated and checked in; after editing them run
`go run github.com/a-h/templ/cmd/templ@latest generate` (or the pinned version from
`go.mod`).

## Operations, privacy & release

- **`docs/ops/operator-runbook.md`** — roles, the ADR-002 v2 network kit for a meet day,
  timing-agent setup, backup/restore, retention.
- **`docs/ops/privacy.md`** — data-processing overview, template meet privacy notice, and
  controller guidance (SYS-104), including the pseudonymization-vs-anonymization distinction
  for athlete erasure (SYS-101).
- **`docs/ops/support-matrix.md`** — supported OS/hardware and browsers (SYS-132), and the
  measured public-results-viewer capacity (SYS-122's 2,000-viewer target is not yet met — see
  `docs/requirements/open-questions-and-assumptions.md` OQ-066).
- **`docs/ops/defect-policy.md`** — severity definitions and the regression-test requirement
  (SYS-143).
- **`docs/ops/release-process.md`** — versioning, changelog, artifact build
  (`scripts/build-release.sh`), checksum/signing stance, container publication, support
  window (SYS-146). See **`CHANGELOG.md`** for what shipped in each release.

## Licence & contributing

- Code: **AGPL-3.0-only** (`LICENSE`) — see
  [ADR-001](docs/architecture/adr/ADR-001-license-agpl-3.0.md) for the rationale.
- Documentation (`docs/`): **CC-BY-SA-4.0** (`docs/LICENSE`).
- Contributions are welcome under the **DCO** (no CLA): see
  [`CONTRIBUTING.md`](CONTRIBUTING.md), [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md), and
  [`GOVERNANCE.md`](GOVERNANCE.md). The project name and logo are held by the founder and
  are not covered by the code licence.
