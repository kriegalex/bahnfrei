// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import "time"

// SessionTTLDefault is the out-of-the-box session lifetime (SYS-091:
// "sessions SHALL expire configurably"). The web package itself is handed
// an already-constructed *app.SessionManager (see New), so this constant
// is consumed by the caller that builds it — cmd/bahnfrei's --session-ttl
// flag default — not read by Config itself.
const SessionTTLDefault = 12 * time.Hour

// Config holds the server shell's runtime configuration: everything
// TASK-005 primitives need, independent of any meet/domain feature (those
// configure through app/domain services added by later tasks).
type Config struct {
	// Addr is the listen address, e.g. ":8443".
	Addr string
	// TLS configures certificate acquisition (SYS-093).
	TLS TLSConfig
}
