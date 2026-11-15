package audit

import (
	"context"
	"encoding/base64"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	ar "github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	"github.com/jackc/pgx/v5"
	"strings"
	"time"
)

// LegacyEvents changes only the envelope. Authorization, filtering and values
// come from the same Reader as the new endpoints.
func (s *Service) LegacyEvents(ctx context.Context, a access.Actor, scope Scope, f access.Filter) (access.Page[access.AuditEvent], error) {
	if f.TenantID != "" {
		if !ar.ID(f.TenantID) || scope.Kind != "platform" && f.TenantID != scope.Tenant {
			return access.Page[access.AuditEvent]{}, Problem("audit_filter_invalid", 400)
		}
		if scope.Kind == "platform" {
			if e := s.access.Transact(ctx, func(tx pgx.Tx) error { return access.RequireTx(ctx, tx, a.ID, access.OpTenantRead) }); e != nil {
				return access.Page[access.AuditEvent]{}, e
			}
			// The old tenant card requested its platform governance notices. Keep
			// that exact subject filter; it must not aggregate tenant business data.
			if f.ObjectID != "" && f.ObjectID != f.TenantID || f.ObjectType != "" && f.ObjectType != "tenant" {
				return access.Page[access.AuditEvent]{}, Problem("audit_filter_invalid", 400)
			}
			f.ObjectType = "tenant"
			f.ObjectID = f.TenantID
		}
	}
	category := f.Category
	if category == "system" {
		category = "maintenance"
	}
	input := Filter{From: f.From, To: f.To, ActorID: f.ActorID, ObjectType: f.ObjectType, ObjectID: f.ObjectID, EventType: f.Event, Outcome: f.Outcome, Category: category, Query: f.Query, Cursor: f.Cursor, Limit: f.Limit}
	if input.Cursor != "" && !strings.Contains(input.Cursor, ".") {
		cursor, e := s.legacyCursor(ctx, a, scope, input)
		if e != nil {
			return access.Page[access.AuditEvent]{}, e
		}
		input.Cursor = cursor
	}
	page, e := s.Events(ctx, a, scope, input)
	if e != nil {
		return access.Page[access.AuditEvent]{}, e
	}
	out := access.Page[access.AuditEvent]{Items: []access.AuditEvent{}, NextCursor: page.NextCursor}
	for _, v := range page.Items {
		d := &access.AuditDisplay{Category: v.Category, Outcome: v.Outcome, ActorName: v.Actor.Name, ActorNameSource: v.Actor.NameSource, TargetName: v.Object.Label, TargetNameSource: v.Object.LabelSource, PermissionNames: map[string]string{}, RoleNames: map[string]string{}}
		d.ActorUsername = v.Actor.Username
		if v.Object.Type == "tenant" {
			d.TenantName = v.Object.Label
		}
		for _, permission := range access.Catalog() {
			d.PermissionNames[string(permission.Code)] = permission.Name
		}
		before, after := map[string]any{}, map[string]any{}
		for _, c := range v.Changes {
			if c.BeforeState == ar.Recorded || c.BeforeState == ar.Truncated {
				before[c.Key] = c.Before
			}
			if c.AfterState == ar.Recorded || c.AfterState == ar.Truncated {
				after[c.Key] = c.After
			}
		}
		item := access.AuditEvent{ID: v.ID, TenantID: v.Scope.Tenant, ProjectID: v.Scope.Project, ActorScope: v.Scope.Kind, EventType: v.EventType, ActorType: v.Actor.Type, ActorID: v.Actor.ID, ObjectType: v.Object.Type, ObjectID: v.Object.ID, Reason: v.Reason, CreatedAt: v.RecordedAt, Details: map[string]any{"before": before, "after": after, "changes": v.Changes, "evidence_quality": v.EvidenceQuality, "missing_fields": v.MissingFields}, Display: d}
		if v.Technical != nil {
			item.RequestID = v.Technical.RequestID
			d.SourceIP = v.Technical.IP
			d.UserAgent = v.Technical.UserAgent
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}

func (s *Service) legacyCursor(ctx context.Context, a access.Actor, scope Scope, input Filter) (string, error) {
	id := input.Cursor
	var claimed time.Time
	if !ar.Number(id, 1) {
		raw, e := base64.RawURLEncoding.DecodeString(id)
		if e != nil {
			return "", Problem("audit_cursor_stale", 409)
		}
		parts := strings.Split(string(raw), "|")
		if len(parts) != 2 || !ar.Number(parts[1], 1) {
			return "", Problem("audit_cursor_stale", 409)
		}
		claimed, e = time.Parse(time.RFC3339Nano, parts[0])
		if e != nil {
			return "", Problem("audit_cursor_stale", 409)
		}
		id = parts[1]
	}
	out := ""
	e := s.transaction(ctx, a, scope, func(tx pgx.Tx, auth authorization) error {
		f, e := s.normalizeFilter(input, auth, false)
		if e != nil {
			return e
		}
		position := cursor{Actor: a.ID, Scope: scope.Key(), Vector: auth.Vector, Filter: filterHash(input), Upper: s.now(), Purpose: "events"}
		rows, e := s.candidatesTx(ctx, tx, auth, f, position, 1, id)
		if e != nil {
			return e
		}
		if len(rows) != 1 || !claimed.IsZero() && !claimed.Equal(rows[0].Recorded) {
			return Problem("audit_cursor_stale", 409)
		}
		values, e := s.projectTx(ctx, tx, a, auth, rows)
		if e != nil {
			return e
		}
		if len(values) != 1 || values[0] == nil {
			return Problem("audit_cursor_stale", 409)
		}
		match, e := matchesTx(ctx, tx, f, *values[0])
		if e != nil {
			return e
		}
		if !match {
			return Problem("audit_cursor_stale", 409)
		}
		position.At = rows[0].Recorded
		position.ID = id
		out = signCursor(position, auth.Secret)
		return nil
	})
	return out, e
}
