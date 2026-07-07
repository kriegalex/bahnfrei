#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-only
# Copyright (c) 2026 Bahnfrei contributors
#
# Enforces the SYS-140 coverage gates on a Go cover profile:
#   >=90% for domain logic (internal/domain/...), >=80% overall.
# Go's cover tooling measures statement coverage; that is the agreed
# verification method for SYS-140 (see traceability-matrix.md).
# A package group with zero statements is skipped (pre-implementation
# bootstrap) — the gate arms itself as code lands.
#
# Generated code (templ output, *_templ.go) is excluded from both gates:
# its source of truth is the corresponding .templ file, whose display
# logic IS exercised through rendering tests — the excluded statements are
# machine-emitted per-write io-error plumbing that cannot fail without a
# failing writer. SYS-140 measures authored project code (method note in
# traceability-matrix.md).
#
# Usage: check-coverage.sh [coverprofile]   (default: coverage.out)
set -eu

profile=${1:-coverage.out}
[ -f "$profile" ] || { echo "no cover profile at $profile" >&2; exit 1; }

awk '
NR == 1 { next }         # "mode:" header
$1 ~ /_templ\.go:/ { next }  # generated templ output (see header comment)
{
    # line format: <file>:<start>,<end> <numstmts> <hitcount>
    # -coverpkg can repeat a block across test binaries: merge by block key.
    stmts[$1] = $2
    hits[$1] += $3
}
END {
    for (k in stmts) {
        total += stmts[k]
        if (hits[k] > 0) covered += stmts[k]
        if (k ~ /^github\.com\/kriegalex\/bahnfrei\/internal\/domain\//) {
            dtotal += stmts[k]
            if (hits[k] > 0) dcovered += stmts[k]
        }
    }
    fail = 0
    if (dtotal == 0) {
        print "domain coverage: no statements yet - gate skipped (bootstrap)"
    } else {
        pct = 100 * dcovered / dtotal
        printf "domain coverage: %.1f%% (gate: >=90%%)\n", pct
        if (pct < 90) fail = 1
    }
    if (total == 0) {
        print "overall coverage: no statements yet - gate skipped (bootstrap)"
    } else {
        pct = 100 * covered / total
        printf "overall coverage: %.1f%% (gate: >=80%%)\n", pct
        if (pct < 80) fail = 1
    }
    if (fail) { print "coverage gate FAILED (SYS-140)"; exit 1 }
    print "coverage gate OK"
}' "$profile"
