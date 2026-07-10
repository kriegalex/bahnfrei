// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/kriegalex/bahnfrei/internal/app"
)

// handleBackupDownload performs the SYS-084 one-action backup — a single
// click (or CLI invocation) yields one portable artifact — and streams the
// resulting snapshot straight to the browser as a file download. The
// artifact is written to a private temp directory removed once the
// response has been served; nothing is left behind on the server.
func (s *Server) handleBackupDownload(w http.ResponseWriter, r *http.Request) {
	session, ok := sessionFromContext(r.Context())
	if !ok {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	tmpDir, err := os.MkdirTemp("", "bahnfrei-backup-")
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()
	dest := filepath.Join(tmpDir, "backup.db")

	manifest, err := s.backup.Backup(r.Context(), session, dest, s.cfg.AppVersion)
	if err != nil {
		if _, forbidden := err.(app.ErrForbidden); forbidden {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		http.Error(w, "backup failed", http.StatusInternalServerError)
		return
	}

	f, err := os.Open(dest)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer func() { _ = f.Close() }()

	filename := fmt.Sprintf("bahnfrei-backup-%s.db", manifest.CreatedAt.Format("20060102-150405"))
	w.Header().Set("Content-Type", "application/vnd.sqlite3")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	_, _ = io.Copy(w, f)
}
