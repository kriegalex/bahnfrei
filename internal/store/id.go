// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import "github.com/oklog/ulid/v2"

// NewID returns a ULID string: sortable, collision-free across nodes without
// coordination (ADR-004 consequences). Used for entity and queue record IDs.
func NewID() string {
	return ulid.Make().String()
}
