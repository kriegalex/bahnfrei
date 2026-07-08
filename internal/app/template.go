// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/kriegalex/bahnfrei/internal/domain"
	"github.com/kriegalex/bahnfrei/internal/store"
)

// MeetTemplates lists the loaded competition templates, sorted by ID (for
// the meet-creation form, SYS-053).
func (s *MeetService) MeetTemplates() []*domain.MeetTemplate {
	out := make([]*domain.MeetTemplate, 0, len(s.templates))
	for _, t := range s.templates {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Template returns a loaded meet template by ID.
func (s *MeetService) Template(id string) (*domain.MeetTemplate, bool) {
	t, ok := s.templates[id]
	return t, ok
}

// TemplateMeetRequest is everything a template meet needs from the
// organizer: the template, a date and a venue — nothing else (UC-033 #1
// "with no further configuration"). Name is optional; empty derives
// "<template> <venue> <year>".
type TemplateMeetRequest struct {
	TemplateID string
	Name       string
	Venue      string
	Date       time.Time
}

// CreateMeetFromTemplate creates a complete meet from a competition
// template (SYS-053, UC-033 #1): category scheme, scoring table, one
// session on the given date, and one event per template discipline whose
// field spans every default division of the scheme (SYS-052
// mixed-category unit; standings split per division). Each event gets a
// final round with one schedulable unit, like AddEvent.
func (s *MeetService) CreateMeetFromTemplate(ctx context.Context, actor Session, req TemplateMeetRequest) (MeetRecord, error) {
	if err := Authorize(actor.Role, CapOrganizeMeet); err != nil {
		return MeetRecord{}, err
	}
	tpl, ok := s.templates[req.TemplateID]
	if !ok {
		return MeetRecord{}, fmt.Errorf("unknown meet template %q", req.TemplateID)
	}
	scheme, ok := s.schemes[tpl.CategorySchemeID]
	if !ok {
		return MeetRecord{}, fmt.Errorf("template %q references unknown category scheme %q", tpl.ID, tpl.CategorySchemeID)
	}
	if tpl.ScoringTableID != "" {
		if _, ok := s.tables[tpl.ScoringTableID]; !ok {
			return MeetRecord{}, fmt.Errorf("template %q references unknown scoring table %q", tpl.ID, tpl.ScoringTableID)
		}
	}
	for _, ev := range tpl.Events {
		if _, ok := s.catalog.ByCode(ev.DisciplineCode); !ok {
			return MeetRecord{}, fmt.Errorf("template %q references unknown discipline %q", tpl.ID, ev.DisciplineCode)
		}
	}
	if req.Date.IsZero() {
		return MeetRecord{}, fmt.Errorf("a meet date is required")
	}
	if req.Venue == "" {
		return MeetRecord{}, fmt.Errorf("a venue is required")
	}
	name := req.Name
	if name == "" {
		name = fmt.Sprintf("%s %s %d", tpl.Name, req.Venue, req.Date.Year())
	}
	// The default divisions of the scheme, in scheme order (M/W 7–15 for
	// the UKC scheme). Elective divisions ("UKC for all") stay opt-in.
	var divisions []string
	for _, c := range scheme.Categories {
		if !c.Elective {
			divisions = append(divisions, c.Code)
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return MeetRecord{}, fmt.Errorf("create meet from template: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rec, err := store.CreateMeet(ctx, tx, domain.Meet{
		Name:             name,
		Venue:            req.Venue,
		StartDate:        req.Date,
		EndDate:          req.Date,
		Organizer:        actor.Username,
		Tier:             domain.MeetTier(tpl.Tier),
		CategorySchemeID: tpl.CategorySchemeID,
		TemplateID:       tpl.ID,
		ScoringTableID:   tpl.ScoringTableID,
	})
	if err != nil {
		return MeetRecord{}, err
	}
	if _, err := store.CreateSession(ctx, tx, domain.Session{
		MeetID: rec.ID, Day: req.Date, Label: tpl.Name,
	}); err != nil {
		return MeetRecord{}, err
	}
	for _, tev := range tpl.Events {
		ev, err := store.CreateEvent(ctx, tx, domain.Event{
			MeetID:         rec.ID,
			DisciplineCode: tev.DisciplineCode,
			CategoryCodes:  divisions,
		})
		if err != nil {
			return MeetRecord{}, err
		}
		round, err := store.CreateRound(ctx, tx, domain.Round{EventID: ev.ID, Kind: domain.RoundFinal}, 0)
		if err != nil {
			return MeetRecord{}, err
		}
		if _, err := store.CreateUnit(ctx, tx, domain.Unit{RoundID: round.ID}); err != nil {
			return MeetRecord{}, err
		}
	}

	after, _ := json.Marshal(map[string]string{
		"template": tpl.ID, "templateVersion": tpl.Version, "name": name,
	})
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: "meet.create-from-template",
		EntityType: "meet", EntityID: rec.ID, After: string(after),
	}); err != nil {
		return MeetRecord{}, fmt.Errorf("audit meet create-from-template: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return MeetRecord{}, fmt.Errorf("create meet from template: %w", err)
	}
	return rec, nil
}
