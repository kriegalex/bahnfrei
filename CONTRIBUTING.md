# Contributing to Bahnfrei

Thanks for helping build an open athletics tournament system. This project is
**spec-driven**: requirements live in `docs/requirements/`, architecture decisions in
`docs/architecture/adr/`, and the active backlog in `docs/delivery/work-breakdown.md`.

## Licensing

- **Code** is licensed **AGPL-3.0-only** (see `LICENSE`, [ADR-001](docs/architecture/adr/ADR-001-license-agpl-3.0.md)).
- **Documentation** (`docs/`) is licensed **CC-BY-SA-4.0** (see `docs/LICENSE`).
- There is **no CLA**. Contributions are accepted under the
  [Developer Certificate of Origin](https://developercertificate.org/) (DCO).

### DCO sign-off (required)

Every commit must carry a `Signed-off-by` trailer certifying the DCO, using your real
name and a working e-mail address:

```
git commit -s -m "feat(domain): add category resolver"
```

CI rejects pull requests containing unsigned commits. To fix a branch retroactively:
`git rebase --signoff origin/main && git push --force-with-lease`.

### SPDX headers (required)

Every source file (`.go`, `.ts`, `.tsx`, `.sh`, `.fish`) starts with:

```go
// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors
```

`scripts/check-license-headers.sh` enforces this in CI.

### Dependencies

Only AGPL-compatible dependencies are accepted; permissive (MIT/BSD/Apache-2.0) is
preferred (ADR-001 §4). An automated licence scan gates CI.

## Development setup

- **Go** ≥ 1.26 (the only hard requirement; the project builds to a single static binary).
- **TypeScript** is used for small client-side islands only — no SPA framework, no bundler
  churn (ADR-003).
- Build and test (quick loop):

```
go build ./...
go vet ./...
go test ./...
```

- Full merge gate — the same checks CI runs, one command (run before pushing):

```
./scripts/check-gate.sh            # SKIP_E2E=1 to skip the Playwright suite
```

CI gates on the 3-OS build/test matrix (with `-shuffle=on`), `golangci-lint` (v2, pinned
version in `ci.yml`, config in `.golangci.yml` — including the architecture dependency
rules), `gosec`, the SYS-140 coverage thresholds (`scripts/check-coverage.sh`: ≥90%
domain, ≥83% overall, measured with `-race -covermode=atomic -coverpkg=./...` — the gate
script always regenerates the profile; a stale `coverage.out` lies), the design-token and
licence-header checks, a dependency licence allowlist, `govulncheck`, TS island typecheck
+ committed-JS freshness, and the Playwright E2E suite.

## How changes land

1. **No code without a traced spec.** Every PR references the work item and requirement IDs
   it implements (`TASK-###`, `UC-###`, `SYS-###`). If what you want to build has no spec,
   open an issue to get one first — for anything architectural (stack, data model, external
   integrations, licence), an ADR proposal is required before code.
2. **Small vertical slices.** PR-sized, independently verifiable, with automated tests
   mapped to the acceptance criteria of the UC/SYS they implement.
3. **Green gates.** Build, vet/lint, typecheck, tests, coverage, SPDX and DCO checks must
   pass. Nothing is "done" until its acceptance tests pass and trace to a `SYS-###`.
4. **Traceability.** PRs that satisfy or change a requirement update
   `docs/requirements/traceability-matrix.md` in the same change.

## Commit messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):
`type(scope): imperative lowercase description` — e.g.
`fix(store): retry busy sqlite writes on checkpoint`. Types: `feat` `fix` `refactor`
`perf` `style` `test` `docs` `build` `ci` `revert` `chore`.

## Conduct

Be excellent to each other — see [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md)
(Contributor Covenant 2.1).
