# Release process (SYS-146)

**Traces:** SYS-146, SYS-144 → STR-036, STR-038, STR-037. **Status:** effective at release 0.1.

SYS-146 requires the repository to contain an OSI-approved LICENSE, a CONTRIBUTING guide, a code
of conduct, maintainer/governance documentation, and a documented release process. The first four
already exist and are not re-authored here — confirmed present: `LICENSE` (AGPL-3.0-only,
[ADR-001](../architecture/adr/ADR-001-license-agpl-3.0.md)), `CONTRIBUTING.md`,
`CODE_OF_CONDUCT.md`, `GOVERNANCE.md`, plus CI-enforced SPDX headers and DCO
(`.github/workflows/governance.yml`). This page is the missing fifth item: the release process
itself.

## 1. Versioning scheme

**Semantic versioning** (SYS-144: external interfaces, incl. the `omx/v1` schema and public URLs,
follow a documented deprecation policy — this project-level scheme extends the same discipline to
the whole release): `MAJOR.MINOR.PATCH`.

- **MAJOR** — a breaking change to a stable external interface (the `omx/v1` schema's own
  versioning policy lives in `docs/schemas/omx-v1.md`; a project MAJOR bump follows if a breaking
  change ships anywhere externally visible: import/export formats, public URLs, the timing-agent
  protocol).
- **MINOR** — new functionality, backward-compatible.
- **PATCH** — bug fixes only (see `docs/ops/defect-policy.md`), no new functionality.
- **0.x (current):** by SemVer convention, MINOR bumps may still include breaking changes before
  1.0.0 — release 0.1.0 is the first public release, not yet an API/format stability commitment.

Git tags are the versioning source of truth: `scripts/build-release.sh` derives the version from
`git describe --tags --always --dirty` unless one is passed explicitly, and stamps it into
`bahnfrei --version` via `-ldflags -X main.version=...` at build time (never at runtime, and never
by editing a checked-in version file).

## 2. Changelog

`CHANGELOG.md` at the repository root, in [Keep a Changelog](https://keepachangelog.com/) style: an
`## [Unreleased]` section accumulates entries as work merges, cut into a dated
`## [x.y.z] - YYYY-MM-DD` section at release time. Entries are grouped `Added`/`Changed`/`Fixed`/
`Security`/`Deprecated`/`Removed` and reference the `TASK-###`/`SYS-###`/`UC-###` they trace to,
consistent with how every other document in this repository cites IDs.

## 3. Cutting a release

1. Confirm the defect-policy gate: no known, unfixed Critical/High-severity defect
   (`docs/ops/defect-policy.md` §2).
2. Confirm the machine-checkable gate is green on the release commit: `go build ./... && go vet
   ./...`, `go test -race ./...`, `scripts/check-coverage.sh` (≥83% overall / ≥90% domain,
   SYS-140), `scripts/check-license-headers.sh`, `scripts/check-style-tokens.sh`, and the
   Playwright E2E suite (`e2e/`).
3. Move the `CHANGELOG.md` `[Unreleased]` section's entries under a new dated version heading.
4. Tag the commit: `git tag -a vX.Y.Z -m "vX.Y.Z"` (annotated, signed if the maintainer's key setup
   supports it — see §4 for the current unsigned-artifact stance either way).
5. Build artifacts: `scripts/build-release.sh X.Y.Z` — cross-compiles all five per-OS binaries
   (`linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64`), smoke-tests the
   host-matching one by actually running `--version`, writes `dist/X.Y.Z/checksums.txt`
   (SHA-256), and builds the container image locally if Docker is available (never pushes it —
   see §4).
6. Publish: attach the `dist/X.Y.Z/` artifacts (binaries + `checksums.txt`) to the tagged release;
   push the container image to whatever registry is decided (currently undecided — OQ-086); push
   the git tag.
7. Announce per `GOVERNANCE.md`'s existing communication channel.

Nothing in `scripts/build-release.sh` or this process runs `git push`, `docker push`, or any other
publishing action automatically — every publish step above is a deliberate, separate human (or
CI-workflow-gated) action.

## 4. Checksum / signing stance

Every release artifact ships a SHA-256 `checksums.txt` (produced by `scripts/build-release.sh`,
verifiable with `sha256sum -c`). **Binaries and the container image are not cryptographically
signed for 0.1** — no code-signing certificate for macOS notarization or Windows Authenticode, no
GPG-signed checksums file, no cosign/sigstore signature on the container image. This is a real,
open item: unsigned macOS/Windows binaries trigger OS Gatekeeper/SmartScreen warnings that add
friction to the SYS-131 ≤30-minute quickstart for non-technical operators, and unsigned artifacts
generally are a weaker supply-chain posture. Tracked as **OQ-087**; the checksum file is the whole
integrity story until it's resolved.

## 5. Container publication

The `Dockerfile` builds a working image (verified: `docker build .` succeeds, `docker run` starts
the server, `--version` reports the ldflags-stamped `VERSION` build arg). **Where the built image
gets published is undecided** — no registry account exists yet (GHCR, Docker Hub, or a
self-hosted registry are the candidates). Tracked as **OQ-086**. Until resolved, `docker build -t
bahnfrei .` from the tagged source is the documented path for anyone who wants the container
image; a resolved registry target updates this section and `docs/ops/quickstart.md` §1 Option B in
the same change.

## 6. Support window

**Interim policy, pending founder direction (OQ-088):** the **latest released MINOR version only**
receives fixes, on a best-effort basis — there is no committed multi-version backport/LTS policy
for 0.1. A Critical/High defect (`docs/ops/defect-policy.md`) found in the latest release gets a
PATCH release; older MINOR lines are not backported. This is documented explicitly (rather than
left implicit) so operators can plan upgrades accordingly: staying current is the only supported
path for 0.1.
