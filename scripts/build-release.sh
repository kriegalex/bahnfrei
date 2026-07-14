#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-only
# Copyright (c) 2026 Bahnfrei contributors
#
# TASK-028 release build (SYS-131, SYS-146): produces versioned, reproducible
# per-OS binaries plus SHA-256 checksums for the platforms named in the
# support matrix (docs/ops/support-matrix.md), and — if a local Docker (or
# Podman-via-docker-shim) daemon is available — the container image from the
# repo Dockerfile. The stack is pure Go (modernc.org/sqlite, no cgo,
# ADR-003/ADR-004), so plain `go build` cross-compilation is expected to
# work for every target; this script actually builds every one of them
# (never just type-checks) and smoke-tests the one binary that matches the
# host's own OS/ARCH by actually running it.
#
# This script never publishes anything: it does not run `docker push`,
# `git tag`, or any upload. It writes local artifacts under dist/ for a
# human (or a future CI release workflow) to publish per
# docs/ops/release-process.md, which also documents the checksum/signing
# stance (SHA-256 only for 0.1 — see OQ-087 for the open code-signing
# question) and the undecided container registry target (OQ-086).
#
# POSIX sh on purpose — must run on the 3-OS CI matrix (SYS-141), same as
# the other scripts/check-*.sh gates.
#
# Usage: scripts/build-release.sh [version] [--skip-image]
#   version        defaults to `git describe --tags --always --dirty`,
#                  falling back to "dev" outside a git checkout or before
#                  any tag exists.
#   --skip-image   skip the container-image build step even if Docker is
#                  available (binaries only).
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT_DIR"

VERSION="dev"
SKIP_IMAGE=0
for arg in "$@"; do
    case "$arg" in
        --skip-image) SKIP_IMAGE=1 ;;
        *) VERSION="$arg" ;;
    esac
done
if [ "$VERSION" = "dev" ] && git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo dev)
fi

BIN_NAME=bahnfrei
DIST_DIR="dist/${VERSION}"

rm -rf "$DIST_DIR"
mkdir -p "$DIST_DIR"

echo "bahnfrei release build: version=${VERSION} dist=${DIST_DIR}"

# GOOS/GOARCH pairs from the SYS-132 support matrix (docs/ops/support-matrix.md
# "Hub / venue-local host OS" table) — keep both lists in lockstep.
targets="linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64"

for target in $targets; do
    goos=${target%/*}
    goarch=${target#*/}
    ext=""
    [ "$goos" = "windows" ] && ext=".exe"
    out="${DIST_DIR}/${BIN_NAME}-${VERSION}-${goos}-${goarch}${ext}"
    echo "  building ${target}..."
    CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build -trimpath \
        -ldflags="-s -w -X main.version=${VERSION}" \
        -o "$out" ./cmd/bahnfrei
done

# Smoke-test: actually run the one binary matching this host (not just
# compile it) and confirm the ldflags-stamped version round-trips through
# the real `--version` flag (cmd/bahnfrei/main.go).
host_goos=$(go env GOOS)
host_goarch=$(go env GOARCH)
host_ext=""
[ "$host_goos" = "windows" ] && host_ext=".exe"
host_bin="${DIST_DIR}/${BIN_NAME}-${VERSION}-${host_goos}-${host_goarch}${host_ext}"
if [ -x "$host_bin" ]; then
    got=$("$host_bin" --version)
    want="bahnfrei ${VERSION}"
    if [ "$got" != "$want" ]; then
        echo "smoke test FAILED: ${host_bin} --version = '${got}', want '${want}'" >&2
        exit 1
    fi
    echo "  smoke test OK (${host_goos}/${host_goarch}): ${got}"
fi

# SHA-256 checksums (OQ-087: no code-signing/notarization for 0.1 — this is
# the whole verification story for now), one file listing every artifact,
# written with paths relative to $DIST_DIR so `sha256sum -c` works from
# inside it.
if command -v sha256sum >/dev/null 2>&1; then
    checksum() { sha256sum "$@"; }
elif command -v shasum >/dev/null 2>&1; then
    checksum() { shasum -a 256 "$@"; }
else
    echo "no sha256sum or shasum found; cannot write checksums.txt" >&2
    exit 1
fi
(
    cd "$DIST_DIR"
    checksum "${BIN_NAME}"-* >checksums.txt
)
echo "  wrote ${DIST_DIR}/checksums.txt"

# Container image (SYS-146 "container publication"): build-only, tagged
# locally with the version, never pushed. Skipped (not failed) when Docker
# isn't available, e.g. this sandbox's CI runner — the Dockerfile is still
# exercised by `docker build` locally whenever Docker is present.
if [ "$SKIP_IMAGE" -eq 1 ]; then
    echo "  container image build skipped (--skip-image)"
elif command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
    echo "  building container image bahnfrei:${VERSION}..."
    docker build --build-arg "VERSION=${VERSION}" -t "bahnfrei:${VERSION}" .
    echo "  built image bahnfrei:${VERSION} (not pushed — publication is a human/CI release-workflow step, docs/ops/release-process.md)"
else
    echo "  no usable Docker daemon found; skipping container image build (verify the Dockerfile by inspection instead)"
fi

echo "release build complete: ${DIST_DIR}"
