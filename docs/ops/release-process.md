# Release process

**Status:** effective at release 0.1.

An open-source release needs the repository to contain an OSI-approved LICENSE, a CONTRIBUTING
guide, a code of conduct, maintainer/governance documentation, and a documented release process.
The first four already exist and are not re-authored here — confirmed present: `LICENSE`
(AGPL-3.0-only,
[ADR-001](../architecture/adr/ADR-001-license-agpl-3.0.md)), `CONTRIBUTING.md`,
`CODE_OF_CONDUCT.md`, `GOVERNANCE.md`, plus CI-enforced SPDX headers and DCO
(`.github/workflows/governance.yml`). This page is the missing fifth item: the release process
itself.

## 1. Versioning scheme

**Semantic versioning**: external interfaces, including the `omx/v1` schema and public URLs,
follow a documented deprecation policy, and this project-level scheme extends the same discipline
to the whole release: `MAJOR.MINOR.PATCH`.

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
`Security`/`Deprecated`/`Removed` and are written for users in plain language — internal
requirement and work-item traceability lives in `docs/requirements/traceability-matrix.md`,
not in the changelog.

## 3. Cutting a release

1. Merge `develop` into `main` (releases are the only time `main` advances; day-to-day
   work lands on `develop`).
2. Confirm the defect-policy gate: no known, unfixed Critical/High-severity defect
   (`docs/ops/defect-policy.md` §2).
3. Confirm the machine-checkable gate is green on the release commit: run
   `scripts/check-gate.sh` (build, vet, lint, gosec, `-race -shuffle=on` tests with the
   coverage floors CI enforces — ≥83% overall / ≥90% domain, licence-header, design-token and
   `govulncheck` checks, TS-island freshness, and the Playwright E2E suite), and verify the
   CI run on the same commit is green.
4. Move the `CHANGELOG.md` `[Unreleased]` section's entries under a new dated version heading.
5. Tag the commit: `git tag -a vX.Y.Z -m "vX.Y.Z"` (annotated, signed if the maintainer's key setup
   supports it — see §4 for the current unsigned-artifact stance either way).
6. Build artifacts: `scripts/build-release.sh X.Y.Z` — cross-compiles all five per-OS binaries
   (`linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64`), smoke-tests the
   host-matching one by actually running `--version`, writes `dist/X.Y.Z/checksums.txt`
   (SHA-256), and builds the container image locally if Docker is available (never pushes it —
   see §4).
7. Push the git tag (`git push origin vX.Y.Z`). Pushing a `vX.Y.Z` tag triggers
   `.github/workflows/release.yml`, which is the **sole publish path**: it rebuilds the binaries
   and `checksums.txt` (`scripts/build-release.sh X.Y.Z --skip-image`, reproducing step 5 in CI),
   builds and pushes the container image to GHCR, cosign-signs the image and `checksums.txt` (see
   §4), and attaches every `dist/X.Y.Z/` artifact plus the cosign bundle to the GitHub release for
   the tag (creating it if it doesn't exist). Nothing in `scripts/build-release.sh` itself runs
   `docker push`, `cosign`, or `gh release` — every publish action is gated on this workflow, which
   only runs on a human deliberately pushing a tag.
8. Announce per `GOVERNANCE.md`'s existing communication channel.

## 4. Checksum / signing stance

Every release artifact ships a SHA-256 `checksums.txt` (produced by `scripts/build-release.sh`,
verifiable with `sha256sum -c`). The **container image and `checksums.txt` are signed keyless with
[sigstore/cosign](https://docs.sigstore.dev/cosign/) in CI** (`.github/workflows/release.yml`),
using GitHub Actions OIDC as the identity provider — no long-lived signing key to generate, store,
or rotate. **Binaries themselves, and macOS notarization / Windows Authenticode certificates, are
deliberately out of scope for 0.x** (no adopter demand yet to justify the cost of a code-signing
certificate) — `checksums.txt`, itself cosign-signed, is their integrity story.

Verify the container image against a specific release tag:

```
cosign verify ghcr.io/kriegalex/bahnfrei:X.Y.Z \
  --certificate-identity "https://github.com/kriegalex/bahnfrei/.github/workflows/release.yml@refs/tags/vX.Y.Z" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com"
```

Verify `checksums.txt` against the bundle attached to the same release (this also transitively
verifies every binary, once `sha256sum -c` passes against the verified file):

```
cosign verify-blob checksums.txt \
  --bundle checksums.txt.cosign.bundle \
  --certificate-identity "https://github.com/kriegalex/bahnfrei/.github/workflows/release.yml@refs/tags/vX.Y.Z" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com"
sha256sum -c checksums.txt --ignore-missing
```

Both commands fail closed: a tampered artifact, a bundle from a different tag/workflow, or a
missing/altered signature all produce a non-zero exit and an explicit error, never a silent pass.

**Unsigned macOS/Windows binaries trigger OS Gatekeeper/SmartScreen warnings** — expected, not a
bug, given the deliberate scope above. Once `checksums.txt` has been verified (or, on a container
install, the image signature above), work around the OS warning:

- **macOS Gatekeeper:** either clear the quarantine attribute after checksum verification —
  `xattr -d com.apple.quarantine ./bahnfrei-X.Y.Z-darwin-<arch>` — or right-click the binary in
  Finder, choose **Open**, and confirm **Open** in the dialog (this path re-prompts once per
  binary but does not require the terminal).
- **Windows SmartScreen:** on the "Windows protected your PC" dialog, click **More info**, then
  **Run anyway**.

## 5. Container publication

The `Dockerfile` builds a working image (verified: `docker build .` succeeds, `docker run` starts
the server, `--version` reports the ldflags-stamped `VERSION` build arg). Images publish to
**GHCR** (`ghcr.io/kriegalex/bahnfrei`), pushed by `.github/workflows/release.yml` on every
`vX.Y.Z` tag push, tagged both with the release version and `latest` (see §6 for what `latest`
means as a support commitment). `docker build -t bahnfrei .` from the tagged source remains a
valid local alternative for anyone who wants to build the image themselves rather than pull it.

## 6. Support window

The **latest released MINOR version only** receives fixes, on a best-effort basis — there is no
multi-version backport/LTS policy for 0.x. A Critical/High defect (`docs/ops/defect-policy.md`)
found in the latest release gets a PATCH release; older MINOR lines are not backported. This is
documented explicitly (rather than left implicit) so operators can plan upgrades accordingly:
staying current is the only supported path while the project is pre-1.0. The GHCR `latest` tag
always points at this same supported line.
