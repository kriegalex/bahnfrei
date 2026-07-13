#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-only
# Copyright (c) 2026 Bahnfrei contributors
#
# TASK-027 perf & recovery suite (SYS-120/121/122/130). These tests are
# guarded behind the `perf` build tag so the default `go test ./...` gate
# stays fast; this script is the documented way to run them. Results and
# environment caveats are recorded in
# docs/delivery/perf-and-recovery-task-027.md.
#
# MEMORY CONFINEMENT (mandatory — OOM incident 2026-07-13): an unconfined
# 2,000-viewer SYS-122 run reached ~27 GB RSS (per-request Standings
# allocations convoyed behind the single SQLite connection, OQ-066) and
# was OOM-killed by the kernel, taking the whole host session with it.
# Every perf invocation below therefore runs inside a systemd USER scope
# (no sudo needed) hard-capped at MemoryMax=12G with swap disabled, and
# with GOMEMLIMIT=10GiB so the Go runtime GC-throttles before hitting the
# hard cap. A runaway test dies inside the scope — never the host. If
# systemd-run is unavailable (non-systemd host), the script refuses to run
# the heavy load tests rather than run them unconfined.
#
# The SYS-105 egress-blocked public-asset check is NOT here: it is fast
# and deterministic, so it runs in the default gate
# (internal/web/egress_test.go, TestPublicSurfaceRendersWithEgressBlockedSYS105).
#
# Usage: scripts/run-perf-tests.sh [quick|full]   (default: quick)
#   quick — SYS-120/121 benchmarks + SYS-130 recovery drill + the
#           SYS-122 scaling profile at 100/250/500 viewers (~2 min)
#   full  — quick plus the literal 2,000-viewer SYS-122 run (several
#           minutes; expected to FAIL against the 3s p95 budget on the
#           current architecture — see OQ-066; deliberately not weakened)
set -eu

mode=${1:-quick}

MEM_MAX=12G
GO_MEM_LIMIT=10GiB

if command -v systemd-run >/dev/null 2>&1; then
    confined() {
        systemd-run --user --scope --quiet \
            -p MemoryMax="$MEM_MAX" -p MemorySwapMax=0 \
            -E GOMEMLIMIT="$GO_MEM_LIMIT" \
            -- "$@"
    }
else
    echo "WARNING: systemd-run not found — cannot confine memory." >&2
    echo "SYS-120/121/130 (bounded) will run unconfined; SYS-122 load tests are SKIPPED." >&2
    confined() { "$@"; }
    mode=nolimit
fi

echo "== SYS-120/121 benchmarks (internal/app) =="
confined go test -tags=perf -count=1 -timeout 600s -v \
    -run 'TestSYS120ReferenceScaleOperatorBudgets|TestSYS121SeedingGenerationBudget' \
    ./internal/app/

echo "== SYS-130 recovery drill (internal/web) =="
confined go test -tags=perf -count=1 -timeout 300s -v \
    -run 'TestServeRecoverySYS130' ./internal/web/

if [ "$mode" = "nolimit" ]; then
    echo "== SYS-122 load tests SKIPPED (no memory confinement available) =="
    exit 0
fi

echo "== SYS-122 scaling profile: 100/250/500 viewers (internal/web) =="
confined go test -tags=perf -count=1 -timeout 1800s -v \
    -run 'TestSYS122ViewerScalingProfile' ./internal/web/

if [ "$mode" = "full" ]; then
    echo "== SYS-122 full 2,000-concurrent-viewer run (internal/web) =="
    echo "   (expected FAIL against the literal 3s p95 budget — OQ-066)"
    confined go test -tags=perf -count=1 -timeout 1800s -v \
        -run 'TestSYS122TwoThousandConcurrentViewers$' ./internal/web/
fi
