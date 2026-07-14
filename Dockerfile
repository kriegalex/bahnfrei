# SPDX-License-Identifier: AGPL-3.0-only
# Copyright (c) 2026 Bahnfrei contributors
#
# Container quickstart image (SYS-131, UC-001 #1): one self-contained
# binary (ADR-002/003 — pure-Go SQLite, embedded assets), so the runtime
# stage needs nothing but CA roots (for ACME in hub mode) and tzdata.
# Hardening (non-root user, read-only rootfs guidance) is revisited in
# TASK-026 (OWASP ASVS L2 pass). VERSION is stamped into the `bahnfrei
# --version` output the same way scripts/build-release.sh does for the
# per-OS binaries (TASK-028); it is a build arg, not a tag pushed anywhere
# by this file — publication is a separate, human/CI release step
# (docs/ops/release-process.md).

FROM golang:1.26 AS build
WORKDIR /src
ARG VERSION=dev
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /bahnfrei ./cmd/bahnfrei

FROM gcr.io/distroless/static-debian12
COPY --from=build /bahnfrei /bahnfrei
VOLUME /data
EXPOSE 8443
ENTRYPOINT ["/bahnfrei", "serve", "--data-dir", "/data"]
