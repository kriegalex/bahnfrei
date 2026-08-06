// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/kriegalex/bahnfrei/internal/app"
)

// Offline-tolerant field capture & walk-by sync (TASK-009, UC-034 /
// SYS-085/086). The checkout and replay endpoints below are this app's ONE
// JSON API: every other surface is server-rendered HTML progressively
// enhanced with HTMX (ADR-003), but the offline capture queue is a
// machine-to-machine batch protocol the client island (slice 2) drives from
// durable browser storage — a form post cannot express an ordered,
// idempotent, per-op-acknowledged batch. The wire contract is documented in
// internal/sync/doc.go; the client island is built against exactly that.

// writeJSON writes v as an application/json response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// checkoutResponse is the stamp a device caches and stamps every queued op
// with (SYS-086): the opaque holder token, the checkout generation, and the
// start-list version captured against.
type checkoutResponse struct {
	Token            string `json:"token"`
	Generation       int64  `json:"generation"`
	StartListVersion int64  `json:"startListVersion"`
}

type checkoutRequest struct {
	DeviceLabel string `json:"deviceLabel"`
}

// handleUnitCheckout takes (or re-takes) the unit's capture lock for the
// authenticated field official and returns the stamp (SYS-086). JSON in/out.
func (s *Server) handleUnitCheckout(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID, unitID := r.PathValue("id"), r.PathValue("unit")
	var req checkoutRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	co, err := s.results.CheckoutUnit(r.Context(), actor, meetID, unitID, deviceLabelOr(req.DeviceLabel))
	if err != nil {
		switch {
		case errors.Is(err, app.ErrCheckedOutByAnother):
			writeJSON(w, http.StatusConflict, map[string]string{"error": "checked_out_by_another"})
		case errors.Is(err, app.ErrUnitNotAssigned):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "unit_not_assigned"})
		default:
			http.Error(w, "checkout failed", http.StatusBadRequest)
		}
		return
	}
	writeJSON(w, http.StatusOK, checkoutResponse{Token: co.Token, Generation: co.Generation, StartListVersion: co.StartListVersion})
}

// syncOp is one queued capture operation on the wire: a client ULID plus the
// grid value in the same notation the online grid uses (a mark, or X/–/r).
type syncOp struct {
	OpID      string `json:"opId"`
	AthleteID string `json:"athleteId"`
	Seq       int    `json:"seq"`
	Value     string `json:"value"`
	Wind      string `json:"wind,omitempty"`
	Version   int64  `json:"version,omitempty"`
}

// syncRequest is an ordered batch of queued ops stamped with the device's
// checkout (SYS-085).
type syncRequest struct {
	Token            string   `json:"token"`
	DeviceLabel      string   `json:"deviceLabel"`
	Generation       int64    `json:"generation"`
	StartListVersion int64    `json:"startListVersion"`
	Ops              []syncOp `json:"ops"`
}

// syncOpResult is the per-op acknowledgement.
type syncOpResult struct {
	OpID    string `json:"opId"`
	Status  string `json:"status"`            // applied | duplicate | reconciliation
	Reason  string `json:"reason,omitempty"`  // set when status == reconciliation
	Version int64  `json:"version,omitempty"` // authoritative attempt version (applied|duplicate)
	// Result and Points (SYS-148, UC-039 #4) are the athlete's current
	// settled display values, set only for applied|duplicate — the client
	// uses them to update the capture grid's Result/Points cells in place
	// without a reload, the same authoritative-server-truth principle
	// Version already applies. Deliberately not omitempty: an athlete with
	// no result yet after a foul/pass-only series is a real, meaningful ""
	// the client should still apply (clearing any stale display), not an
	// absent field to be ignored.
	Result string `json:"result"`
	Points string `json:"points"`
}

type syncResponse struct {
	Results []syncOpResult `json:"results"`
}

// handleUnitSync replays an ordered batch of queued field captures
// idempotently (SYS-085): applied / duplicate / reconciliation / rejected
// per op. JSON in/out.
//
// A missing/expired session (SYS-149, UC-040 #4) never reaches this
// handler's error branches at all: captureRole (routes.go) wraps this route
// in requireRole(RoleFieldOfficial, …), which already answers an
// unauthenticated or under-role request with its own 403 before
// handleUnitSync runs — exactly what happens once the 12h TTL
// (internal/web/config.go) lapses and the session cookie is rejected on
// lookup. The client's re-authentication classification (401/403,
// islands/src/capture-offline.ts) already covers that response.
func (s *Server) handleUnitSync(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID, unitID := r.PathValue("id"), r.PathValue("unit")

	var req syncRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	batch := app.ReplayBatch{
		Token: req.Token, DeviceLabel: deviceLabelOr(req.DeviceLabel),
		Generation: req.Generation, StartListVersion: req.StartListVersion,
	}
	for _, o := range req.Ops {
		kind, mark := parseAttemptValue(o.Value)
		op := app.ReplayOp{OpID: o.OpID, AthleteID: o.AthleteID, Seq: o.Seq, Kind: kind, Mark: mark, ExpectedVersion: o.Version}
		if ws := strings.TrimSpace(o.Wind); ws != "" {
			if wind, err := strconv.ParseFloat(strings.ReplaceAll(ws, ",", "."), 64); err == nil {
				op.Wind = &wind
			}
		}
		batch.Ops = append(batch.Ops, op)
	}

	res, err := s.results.ReplayCaptureBatch(r.Context(), actor, meetID, unitID, batch)
	if err != nil {
		if errors.Is(err, app.ErrUnitNotAssigned) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "unit_not_assigned"})
			return
		}
		// Every per-op validation/business-rule failure is classified inside
		// ReplayCaptureBatch as a "rejected" outcome in the 200 response
		// (SYS-149, UC-040) rather than an error here, so anything that still
		// reaches this branch is an unexpected/infrastructure fault, not the
		// operator's mistake — a 500 tells the client this is retryable
		// (today's backoff/offline messaging), not a batch-wide validation
		// rejection to give up on.
		http.Error(w, "replay failed", http.StatusInternalServerError)
		return
	}
	out := syncResponse{}
	for _, o := range res.Outcomes {
		reason := string(o.Reason)
		if o.RejectReason != "" {
			reason = string(o.RejectReason)
		}
		out.Results = append(out.Results, syncOpResult{OpID: o.OpID, Status: string(o.Status), Reason: reason, Version: o.Version, Result: o.Result, Points: o.Points})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleCheckoutOverride is the office reassigning a unit's lock (SYS-086,
// audited). Form post from the reconciliation/capture surface; redirects back.
func (s *Server) handleCheckoutOverride(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID, unitID := r.PathValue("id"), r.PathValue("unit")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if _, err := s.results.OverrideCheckout(r.Context(), actor, meetID, unitID, deviceLabelOr(r.FormValue("device")), strings.TrimSpace(r.FormValue("reason"))); err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	http.Redirect(w, r, "/meets/"+meetID+"/reconciliation", http.StatusSeeOther)
}

// handleReviseStartList records an office start-list change on a checked-out
// unit (SYS-086, audited). Form post; redirects back.
func (s *Server) handleReviseStartList(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID, unitID := r.PathValue("id"), r.PathValue("unit")
	if _, err := s.results.ReviseStartList(r.Context(), actor, meetID, unitID); err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	http.Redirect(w, r, "/meets/"+meetID+"/capture/"+unitID, http.StatusSeeOther)
}

// --- reconciliation office view ---

type reconciliationEntryView struct {
	ItemID      string
	AthleteID   string
	Reason      string
	CapturedBy  string
	DeviceLabel string
	Trial       string
	Value       string
}

type reconciliationGroupView struct {
	Discipline string
	Entries    []reconciliationEntryView
}

type reconciliationView struct {
	MeetID   string
	MeetName string
	Groups   []reconciliationGroupView
}

func (s *Server) handleReconciliation(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID := r.PathValue("id")
	detail, err := s.meets.Meet(r.Context(), meetID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	groups, err := s.results.ReconciliationItems(r.Context(), actor, meetID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	v := reconciliationView{MeetID: detail.ID, MeetName: detail.Name}
	for _, g := range groups {
		gv := reconciliationGroupView{Discipline: g.DisciplineName}
		for _, e := range g.Entries {
			gv.Entries = append(gv.Entries, reconciliationEntryView{
				ItemID: e.ItemID, AthleteID: e.AthleteID, Reason: string(e.Reason),
				CapturedBy: e.CapturedBy, DeviceLabel: e.DeviceLabel,
				Trial: strconv.Itoa(e.Trial), Value: e.Value,
			})
		}
		v.Groups = append(v.Groups, gv)
	}
	p := basePageData(r, s.cats)
	p.Title = v.MeetName + " — " + p.T("reconciliation.title")
	_ = reconciliationPage(p, v).Render(r.Context(), w)
}

func (s *Server) handleReconciliationResolve(apply bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, _ := sessionFromContext(r.Context())
		meetID, itemID := r.PathValue("id"), r.PathValue("item")
		if err := s.results.ResolveReconciliation(r.Context(), actor, meetID, itemID, apply); err != nil {
			s.renderMeetError(w, r, err)
			return
		}
		http.Redirect(w, r, "/meets/"+meetID+"/reconciliation", http.StatusSeeOther)
	}
}

// deviceLabelOr defaults a missing device label to "web".
func deviceLabelOr(label string) string {
	if l := strings.TrimSpace(label); l != "" {
		return l
	}
	return "web"
}

// deviceSuffix renders " (label)" for a non-empty, non-default device label.
func deviceSuffix(label string) string {
	if label == "" || label == "web" {
		return ""
	}
	return " (" + label + ")"
}
