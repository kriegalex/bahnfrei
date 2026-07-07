# SPDX-License-Identifier: AGPL-3.0-only
# Copyright (c) 2026 Bahnfrei contributors
#
# Container quickstart image (SYS-131, UC-001 #1): one self-contained
# binary (ADR-002/003 — pure-Go SQLite, embedded assets), so the runtime
# stage needs nothing but CA roots (for ACME in hub mode) and tzdata.
# Hardening (non-root user, read-only rootfs guidance) is revisited in
# TASK-026 (OWASP ASVS L2 pass).

FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /bahnfrei ./cmd/bahnfrei

FROM gcr.io/distroless/static-debian12
COPY --from=build /bahnfrei /bahnfrei
VOLUME /data
EXPOSE 8443
ENTRYPOINT ["/bahnfrei", "serve", "--data-dir", "/data"]
