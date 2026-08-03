#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-only
# Copyright (c) 2026 Bahnfrei contributors
#
# Full local merge gate — one command, same checks as .github/workflows/ci.yml
# (SYS-140/141). Run before every push to main; a stage that CI runs but this
# script skipped is how main has gone red in the past.
#
# The coverage profile is ALWAYS regenerated (never reuses a stale
# coverage.out — stale profiles have produced phantom readings), with CI's
# exact flags: -race -shuffle=on -covermode=atomic -coverpkg=./...
#
# Usage: scripts/check-gate.sh          (from the repo root)
#   SKIP_E2E=1  skips the Playwright suite (CI still runs it).
#
# golangci-lint: uses the PATH binary if present, else installs the pinned
# version (same pin as ci.yml — keep in lockstep) into a cache dir.
set -eu

cd "$(dirname "$0")/.."

GOLANGCI_LINT_VERSION=v2.12.1   # keep in lockstep with ci.yml lint job (v2.12.2: SA5011 regression)
GOSEC_VERSION=v2.28.0           # keep in lockstep with ci.yml sast job

command -v go >/dev/null 2>&1 || {
    echo "go not on PATH (this machine may use mise: mise x go@latest -- $0)" >&2
    exit 1
}

step() {
    printf '\n=== gate: %s ===\n' "$1"
}

step "go build"
go build ./...

step "go vet"
go vet ./...

step "licence headers (ADR-001 §3)"
./scripts/check-license-headers.sh

step "design tokens (SYS-116)"
./scripts/check-style-tokens.sh

step "golangci-lint ($GOLANGCI_LINT_VERSION)"
lint_bin=$(command -v golangci-lint || true)
if [ -z "$lint_bin" ]; then
    cache="${XDG_CACHE_HOME:-$HOME/.cache}/bahnfrei/golangci-lint/$GOLANGCI_LINT_VERSION"
    lint_bin="$cache/golangci-lint"
    if [ ! -x "$lint_bin" ]; then
        mkdir -p "$cache"
        curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh \
            | sh -s -- -b "$cache" "$GOLANGCI_LINT_VERSION"
    fi
fi
"$lint_bin" run

step "gosec (SYS-092, OQ-078)"
go run "github.com/securego/gosec/v2/cmd/gosec@$GOSEC_VERSION" -quiet -exclude-generated ./...

step "tests: -race -shuffle=on + coverage (SYS-140)"
go test -race -shuffle=on -covermode=atomic -coverprofile=coverage.out -coverpkg=./... ./...
./scripts/check-coverage.sh coverage.out

step "govulncheck (SYS-141)"
go run golang.org/x/vuln/cmd/govulncheck@latest ./...

step "TS islands: typecheck + committed JS freshness"
# Same walk as ci.yml's typecheck-islands job (islands/ has no package.json
# by design — tsc comes via npx); .claude/ holds gitignored agent worktrees
# a fresh CI checkout never sees.
find . -name tsconfig.json -not -path '*/node_modules/*' -not -path './.claude/*' \
    | while IFS= read -r cfg; do
    dir=$(dirname "$cfg")
    if [ -f "$dir/package.json" ] && [ ! -d "$dir/node_modules" ]; then
        (cd "$dir" && npm ci)
    fi
    npx --yes -p typescript tsc -p "$cfg" --noEmit
done
./scripts/build-islands
git diff --exit-code internal/web/static

if [ "${SKIP_E2E:-0}" = "1" ]; then
    printf '\n=== gate: e2e SKIPPED (SKIP_E2E=1) — CI will still run it ===\n'
else
    step "Playwright e2e"
    (
        cd e2e
        [ -d node_modules ] || npm ci
        npx playwright test
    )
fi

printf '\ngate OK\n'
