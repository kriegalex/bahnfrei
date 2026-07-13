#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-only
# Copyright (c) 2026 Bahnfrei contributors
#
# TASK-032 CI style-conformance check (SYS-116, UC-038 #1): fails when a
# stylesheet, template or handler declares a raw visual literal (hex color,
# rgb()/rgba() function, or an inline style="" attribute) outside the
# design-token definition file. The token set and its rationale are
# documented in docs/architecture/design-system.md; the tokens themselves
# live in internal/web/static/tokens.css — that file is the only place a
# raw color literal may appear.
#
# Mechanical, pragmatic scope (per UC-038 #1's "documented allowlist for
# the mechanical check's known limits"): this scans authored .css, .templ
# and .go source for the two bypass patterns above. It does NOT parse CSS
# and cannot catch every possible literal (e.g. a font-size expressed as a
# bare number in a future new file) — new bypass shapes get added here or
# to the allowlist below as they're found; the release usability audit
# (docs/requirements/usability-audit-checklist.md) is the non-mechanical
# backstop SYS-116 names for what this check structurally cannot catch.
#
# POSIX sh on purpose — must run on the 3-OS CI matrix (SYS-141), same as
# the other scripts/check-*.sh gates.
#
# Usage: check-style-tokens.sh   (run from repo root)
set -eu

TOKENS_FILE="internal/web/static/tokens.css"
[ -f "$TOKENS_FILE" ] || { echo "check-style-tokens: $TOKENS_FILE not found" >&2; exit 1; }

# Allowlist: paths exempt from the raw-literal scan, with rationale.
#   - the token file itself: it is the one place literals are DEFINED.
#   - vendored third-party assets (htmx): not project-authored style.
is_allowlisted() {
    case "$1" in
        "$TOKENS_FILE") return 0 ;;
        internal/web/static/htmx.min.js) return 0 ;;
        internal/web/static/htmx-LICENSE) return 0 ;;
        *) return 1 ;;
    esac
}

fail=0

# Hex color literals (#abc, #aabbcc, #aabbccdd) and rgb()/rgba() functions
# outside the token file, across every authored .css/.templ/.go source.
for f in $(git ls-files '*.css' '*.templ' '*.go' ':!:*_templ.go' ':!:*/testdata/*' ':!:testdata/*'); do
    is_allowlisted "$f" && continue
    if grep -nE '#[0-9a-fA-F]{3,8}\b|rgba?\(' "$f" >/dev/null 2>&1; then
        grep -nE '#[0-9a-fA-F]{3,8}\b|rgba?\(' "$f" | while IFS= read -r line; do
            echo "raw color literal outside $TOKENS_FILE: $f:$line" >&2
        done
        fail=1
    fi
done

# Inline style="" attributes in templates: a total bypass of the token
# layer regardless of what value it carries.
for f in $(git ls-files '*.templ'); do
    is_allowlisted "$f" && continue
    if grep -nE 'style="' "$f" >/dev/null 2>&1; then
        grep -nE 'style="' "$f" | while IFS= read -r line; do
            echo "inline style attribute bypasses design tokens: $f:$line" >&2
        done
        fail=1
    fi
done

if [ "$fail" -ne 0 ]; then
    echo "style-conformance check FAILED — see docs/architecture/design-system.md (SYS-116)" >&2
    exit 1
fi
echo "style-conformance check OK"
