# Quickstart — download to a working system in ≤30 minutes

**Traces:** SYS-131, SYS-001/002/004/006 → STR-039, STR-035; UC-001.
**Verified by:** `cmd/bahnfrei` `TestQuickstartFreshInstallE2E` (see "How this is verified" below).
**Audience:** a first-time operator (a club volunteer), on a fresh machine, following this page
verbatim. No configuration file is ever edited.

This is the same walkthrough as the README's "Quickstart" section, extended with the release-0.1
download path and pointers to what to read next. If you already have a Go toolchain and just want
to build from source, the README is the shorter version.

## 1. Get the binary

Three ways to get a running `bahnfrei`, in order of how release 0.1 expects most operators to
arrive:

**Option A — download a release binary.** Pick the file matching your OS/CPU from the release's
artifact list (`docs/ops/release-process.md` documents how these are built:
`scripts/build-release.sh`, targets `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`,
`windows/amd64`). Verify the download against the published `checksums.txt` before running it:

```
sha256sum -c checksums.txt --ignore-missing        # Linux/macOS (macOS: shasum -a 256 -c)
```

Make it executable (Linux/macOS) and run it — see step 2. There is nothing to install: the binary
is the whole application (ADR-002/ADR-003 — embedded assets, pure-Go SQLite, no runtime to set up).

**Option B — container image.** Requires Docker (or Podman). Images publish to GHCR
(`docs/ops/release-process.md` §5), signed keyless with cosign in CI — verify the image against
the release tag before running it (exact command and the Gatekeeper/SmartScreen equivalent for
unsigned binaries are in `docs/ops/release-process.md` §4):

```
cosign verify ghcr.io/kriegalex/bahnfrei:<version> \
  --certificate-identity "https://github.com/kriegalex/bahnfrei/.github/workflows/release.yml@refs/tags/v<version>" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com"
docker pull ghcr.io/kriegalex/bahnfrei:<version>
docker run -d --name bahnfrei -p 8443:8443 -v bahnfrei-data:/data ghcr.io/kriegalex/bahnfrei:<version>
```

Building the image locally instead (`docker build -t bahnfrei .` then `docker run -d --name
bahnfrei -p 8443:8443 -v bahnfrei-data:/data bahnfrei`) remains a valid alternative to pulling from
GHCR.

**Option C — build from source.** Requires Go ≥ 1.26:

```
go build -o bahnfrei ./cmd/bahnfrei
./bahnfrei serve --data-dir ./data
```

(`go build` alone, without `scripts/build-release.sh`, stamps no version — `bahnfrei --version`
reports `dev`; that's fine for evaluation, just not for a release artifact.)

## 2. Start it and open the browser

```
./bahnfrei serve --data-dir ./data
```

Open **https://localhost:8443**. In the default venue mode, the TLS certificate is locally
generated and self-signed — this works fully offline (SYS-093) — so your browser warns once;
accept it. For an internet-facing hub install, use
`./bahnfrei serve --role hub --acme-domain your.domain --acme-email you@example.org` instead, for
a publicly trusted certificate via ACME.

## 3. First-run setup — no config file

You land on the **setup page** automatically (a fresh install has no admin account yet, so `GET /`
redirects here — UC-001 #1). Create the admin account: username, display name, password
(≥ 8 characters). This is the *only* configuration step, and it happens in the browser, not in a
file.

## 4. Log in and create your first meet

Log in, then create a meet under **Wettkämpfe / Compétitions**: name, venue, competition days,
sessions per day, and tier. Add events with round structure and entry deadlines. The system is now
**ready for meet setup** — the SYS-131 bar this page is verified against.

Everything from here on (entries, seeding, competition-day capture, timing integration, public
results) is covered by the **operator runbook** (`docs/ops/operator-runbook.md`), not this page.

## What "ready" means, and how it's verified

SYS-131's bar is *"a working system ready for meet setup in ≤30 minutes wall-clock, no
configuration file hand-edited"* — not a finished meet. The machine-checkable proxy for this page
is `cmd/bahnfrei`'s `TestQuickstartFreshInstallE2E`: starting from an **empty data directory**, it
drives exactly the steps above over real HTTPS against the real server (no mocks) — setup, login,
meet creation, event programme, timetable publish/amend, sanctioning summary — and asserts each
step's result, end to end, with nothing manual left except reading pages. The ≤30-minute bound
itself is a human-pace bound (a person reading this page and clicking through takes minutes, not a
CI wall-clock assertion); the automation proves every step *works*, which is the part that could
silently break.

## All state lives in one place

The data directory (`bahnfrei.db` plus the TLS certificate cache) is the entire meet. Back it up
(`bahnfrei backup --data-dir ./data --out backup.bfbak`, or the office UI's **Backup** download at
`/admin/backup`) and you have everything — see `docs/ops/operator-runbook.md` §"Backup & restore"
(UC-020).
