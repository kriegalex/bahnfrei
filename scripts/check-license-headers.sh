#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-only
# Copyright (c) 2026 Bahnfrei contributors
#
# Enforces ADR-001 §3: every source file carries an SPDX header naming
# AGPL-3.0-only within its first five lines. Run from the repo root.
# POSIX sh on purpose — must run on the 3-OS CI matrix (SYS-141).
set -eu

TAG="SPDX-License-Identifier: AGPL-3.0-only"
fail=0

# Tracked source files. Generated files (*_templ.go, *.pb.go) and testdata
# fixtures are exempt; extend the filters as those appear.
for f in $(git ls-files '*.go' '*.ts' '*.tsx' '*.sh' '*.fish' \
    ':!:*_templ.go' ':!:*.pb.go' ':!:*/testdata/*' ':!:testdata/*'); do
    if ! head -n 5 "$f" | grep -qF "$TAG"; then
        echo "missing SPDX header ($TAG): $f" >&2
        fail=1
    fi
done

if [ "$fail" -ne 0 ]; then
    echo "SPDX header check FAILED — see ADR-001 §3 and CONTRIBUTING.md" >&2
    exit 1
fi
echo "SPDX header check OK"
