// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

// Package web serves HTTP: SSR templates, HTMX endpoints, SSE, i18n
// rendering, and public pages including unofficial-results labeling
// (SYS-070–076, SYS-110–114). The public-surface privacy allowlist
// (SYS-100) is enforced centrally in view models here, not per-template.
package web
