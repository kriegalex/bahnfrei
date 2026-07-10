// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kriegalex/bahnfrei/internal/store"
)

// BackupManifest aliases the store type so the web layer never needs to
// import internal/store directly (architecture.md §3 / depguard
// "web-goes-through-app", the same pattern MeetRecord etc. use in meet.go).
type BackupManifest = store.BackupManifest

// BackupService hosts the UC-020 #3 one-action backup use-case (SYS-084):
// an authorized operator triggers a single consistent snapshot of the
// whole instance database, audited like any other privileged action
// (SYS-046).
type BackupService struct {
	st *store.Store
}

// NewBackupService wires a BackupService against the instance's one store
// (ADR-004 §1 — backup necessarily works at the whole-database level, not
// per-meet, since that is the unit VACUUM INTO snapshots).
func NewBackupService(st *store.Store) *BackupService {
	return &BackupService{st: st}
}

// Backup produces a one-action backup artifact at destPath (SYS-084).
// appVersion is stamped into the artifact's manifest for provenance
// (e.g. "bahnfrei 0.3.1"). The action is authorized (CapManageBackup,
// instance-admin per UC-020's actor line) and audited.
func (b *BackupService) Backup(ctx context.Context, actor Session, destPath, appVersion string) (BackupManifest, error) {
	if err := Authorize(actor.Role, CapManageBackup); err != nil {
		return BackupManifest{}, err
	}

	manifest, err := b.st.Backup(ctx, destPath, appVersion)
	if err != nil {
		return BackupManifest{}, err
	}

	// The audit entry is written to the live database, after the
	// snapshot: it documents "a backup was taken, with this fingerprint",
	// not part of the artifact itself (which is already sealed by the
	// time this row exists).
	tx, err := b.st.DB().BeginTx(ctx, nil)
	if err != nil {
		return manifest, fmt.Errorf("audit backup: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op after a successful Commit

	after, _ := json.Marshal(struct {
		MeetCount   int64  `json:"meet_count"`
		ResultCount int64  `json:"result_count"`
		Checksum    string `json:"checksum"`
	}{manifest.MeetCount, manifest.ResultCount, manifest.Checksum})
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: "backup.create",
		EntityType: "instance", EntityID: "backup", After: string(after),
	}); err != nil {
		return manifest, fmt.Errorf("audit backup: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return manifest, fmt.Errorf("audit backup: %w", err)
	}
	return manifest, nil
}
