package audit

import (
	"context"
	"slices"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	ar "github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	"github.com/jackc/pgx/v5"
)

func (s *Service) eventTx(ctx context.Context, tx pgx.Tx, a access.Actor, auth authorization, id string) (Event, rawEvent, error) {
	if !ar.Number(id, 1) {
		return Event{}, rawEvent{}, Problem("audit_not_found", 404)
	}
	raw, e := scanRaw(tx.QueryRow(ctx, `SELECT `+rawColumns+rawFrom+` WHERE e.id=$1`, id))
	if e == pgx.ErrNoRows {
		return Event{}, raw, Problem("audit_not_found", 404)
	}
	if e != nil {
		return Event{}, raw, e
	}
	if !scopeAllows(auth, raw) {
		return Event{}, raw, Problem("audit_not_found", 404)
	}
	projected, e := s.projectTx(ctx, tx, a, auth, []rawEvent{raw})
	if e != nil {
		return Event{}, raw, e
	}
	if projected[0] == nil {
		return Event{}, raw, Problem("audit_not_found", 404)
	}
	return *projected[0], raw, nil
}
func (s *Service) Event(ctx context.Context, a access.Actor, scope Scope, id string) (EventDetail, error) {
	out := EventDetail{Links: []ResolvedLink{}}
	e := s.transaction(ctx, a, scope, func(tx pgx.Tx, auth authorization) error {
		v, _, e := s.eventTx(ctx, tx, a, auth, id)
		if e != nil {
			return e
		}
		out.Event = v
		out.ObservedAt = s.now()
		out.AuthorizationRevision = auth.Vector
		if !v.Actions.Trace {
			return nil
		}
		out.Links, e = s.linksTx(ctx, tx, a, auth, id)
		return e
	})
	return out, e
}
func (s *Service) linksTx(ctx context.Context, tx pgx.Tx, a access.Actor, auth authorization, id string) ([]ResolvedLink, error) {
	rows, e := tx.Query(ctx, `SELECT id::text,link_kind,source_module,target_kind,target_id,COALESCE(generation::text,''),COALESCE(revision,''),relation,scope_kind,COALESCE(tenant_id::text,''),COALESCE(project_id::text,'') FROM audit_event_links WHERE event_id=$1 ORDER BY created_at,id LIMIT 33`, id)
	if e != nil {
		return nil, e
	}
	links := []ar.Link{}
	for rows.Next() {
		var l ar.Link
		if e = rows.Scan(&l.ID, &l.Kind, &l.Module, &l.TargetKind, &l.TargetID, &l.Generation, &l.Revision, &l.Relation, &l.Scope.Kind, &l.Scope.Tenant, &l.Scope.Project); e != nil {
			rows.Close()
			return nil, e
		}
		// Resolve a platform tenant-governance link through the platform detail permission.
		if auth.Scope.Kind == "platform" && l.Kind == "business" && l.Module == "tenancy" && l.TargetKind == "tenant" {
			l.Scope = auth.Scope
		}
		links = append(links, l)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	if len(links) > 32 {
		return nil, Problem("audit_query_budget_exceeded", 503)
	}
	if len(links) == 0 {
		return []ResolvedLink{}, nil
	}
	if s.trace == nil {
		return []ResolvedLink{{Kind: "unavailable", Label: "关联来源暂不可核实", State: "unavailable", ReasonCode: "audit_source_unavailable"}}, nil
	}
	bounded, cancel := context.WithTimeout(ctx, s.options.MetadataTimeout)
	defer cancel()
	resolved, e := s.trace.ResolveTx(bounded, tx, a, links, auth.Has("technical.read"))
	if e != nil {
		return []ResolvedLink{{Kind: "unavailable", Label: "关联来源暂不可核实", State: "unavailable", ReasonCode: "audit_source_unavailable"}}, nil
	}
	out := []ResolvedLink{}
	for _, link := range links {
		v, ok := resolved[link.ID]
		if !ok || v.State == "restricted" {
			continue
		}
		out = append(out, v)
	}
	return out, nil
}
func (s *Service) Target(ctx context.Context, a access.Actor, scope Scope, id, link string) (Target, error) {
	out := Target{Scope: scope}
	if !ar.ID(link) {
		return out, Problem("audit_not_found", 404)
	}
	detail, e := s.Event(ctx, a, scope, id)
	if e != nil {
		return out, e
	}
	for _, l := range detail.Links {
		if l.ID == link && l.Target != nil {
			return *l.Target, nil
		}
	}
	return out, Problem("audit_not_found", 404)
}
func (s *Service) Trace(ctx context.Context, a access.Actor, scope Scope, id string, f TraceFilter) (TracePage, error) {
	out := TracePage{Items: []Event{}, Edges: []Edge{}, Links: []ResolvedLink{}, ObservedAt: s.now()}
	e := s.transaction(ctx, a, scope, func(tx pgx.Tx, auth authorization) error {
		entry, raw, e := s.eventTx(ctx, tx, a, auth, id)
		if e != nil {
			return e
		}
		if !entry.Actions.Trace {
			out.Items = append(out.Items, entry)
			return nil
		}
		limit := f.Limit
		if limit == 0 {
			limit = 100
		}
		if limit < 1 || limit > s.options.TraceMaxNodes {
			return Problem("audit_filter_invalid", 400)
		}
		c := cursor{Actor: a.ID, Scope: scope.Key(), Vector: auth.Vector, Filter: ar.Hash([]any{id, limit}), Upper: s.now(), Purpose: "trace"}
		if f.Cursor != "" {
			v, e := parseCursor(f.Cursor, auth.Secret)
			if e != nil {
				return e
			}
			if v.Actor != c.Actor || v.Scope != c.Scope || v.Vector != c.Vector || v.Filter != c.Filter || v.Purpose != "trace" {
				return Problem("audit_cursor_stale", 409)
			}
			c = v
		}
		var at any
		if !c.At.IsZero() {
			at = c.At
		}
		after := c.ID
		if after == "" {
			after = "0"
		}
		rows, e := tx.Query(ctx, `SELECT `+rawColumns+rawFrom+` WHERE (e.operation_id=$1::uuid OR e.id IN(SELECT event_id FROM audit_event_links WHERE link_kind='caused_by' AND target_id=$2)) AND e.created_at<=$3 AND ($4::timestamptz IS NULL OR (e.created_at,e.id)<($4,$5::bigint)) AND e.visibility_class=ANY($6::text[]) AND COALESCE(e.tenant_id::text,'')=$7 AND COALESCE(e.project_id::text,'')=$8 ORDER BY e.created_at DESC,e.id DESC LIMIT $9`, raw.OperationID, id, c.Upper, at, after, visibleClasses(auth), raw.Scope.Tenant, raw.Scope.Project, limit+1)
		if e != nil {
			return e
		}
		candidates := []rawEvent{}
		for rows.Next() {
			v, e := scanRaw(rows)
			if e != nil {
				rows.Close()
				return e
			}
			candidates = append(candidates, v)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if len(candidates) > limit {
			last := candidates[limit-1]
			c.At = last.Recorded
			c.ID = last.ID
			out.NextCursor = signCursor(c, auth.Secret)
			candidates = candidates[:limit]
		}
		projected, e := s.projectTx(ctx, tx, a, auth, candidates)
		if e != nil {
			return e
		}
		ids := []string{}
		for _, v := range projected {
			if v == nil {
				out.Partial = true
				continue
			}
			out.Items = append(out.Items, *v)
			ids = append(ids, v.ID)
			if v.OperationID == entry.OperationID && v.ID != id {
				out.Edges = append(out.Edges, Edge{id, v.ID, "same_operation"})
			}
		}
		if !slices.Contains(ids, id) && f.Cursor == "" {
			out.Partial = true
		}
		out.Links, e = s.linksTx(ctx, tx, a, auth, id)
		return e
	})
	return out, e
}
