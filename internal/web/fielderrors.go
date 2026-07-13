// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

// FieldErrors is the reusable per-field validation-error mechanism behind
// OQ-075/UC-038 #4: a form's handler validates submitted input and, for each
// field that fails, records a localized message naming what to fix — keyed
// by the field's HTML `name` attribute. Templates pair it with the
// TASK-032-documented `.field-error`/`aria-invalid` convention
// (docs/architecture/design-system.md §2) via the fieldError component and
// fieldErrorID below, so every form gets the same markup shape instead of
// each handler inventing its own. A nil/empty FieldErrors means "no
// field-level errors": the form's existing page-level FlashError summary
// (layout.templ's `p.FlashError` mechanism) continues to cover business-
// level failures that are not attributable to one field (e.g. "entry
// deadline passed").
type FieldErrors map[string]string

// Has reports whether field carries a recorded error.
func (fe FieldErrors) Has(field string) bool {
	return fe[field] != ""
}

// Msg returns field's localized error message, or "" if it has none.
func (fe FieldErrors) Msg(field string) string {
	return fe[field]
}

// fieldErrorID is the DOM id an input's `aria-describedby` must reference to
// associate it with the `<p class="field-error">` fieldError renders for the
// same field name (SYS-117's "state what to fix" pairing).
func fieldErrorID(field string) string {
	return field + "-error"
}
