package monitoring

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"slices"
	"strings"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dr "github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	"github.com/jackc/pgx/v5"
)

// Drilldown never resolves current group membership. The parent's fixed public
// metadata and private channel selection define its only eligible descendants.
func (s *Service) DisplayDrilldownTx(ctx context.Context, tx pgx.Tx, p dr.Plan, proof json.RawMessage, q *dr.DrilldownRequest) (dr.Scope, json.RawMessage, error) {
	if !slices.Contains([]string{"details", "trends", "statistics"}, q.Purpose) || !slices.Contains([]string{"raw_active", "raw_samples", "derived_logical", "derived_samples"}, q.Basis) {
		return dr.Scope{}, nil, dr.Invalid("/purpose", "监测下钻的用途和口径不匹配。")
	}
	var w displayWork
	if e := json.Unmarshal(proof, &w); e != nil {
		return dr.Scope{}, nil, e
	}
	request := p.Request.(*dr.MonitoringStatisticsRequest)
	kind := "direct"
	if strings.HasPrefix(q.Basis, "derived_") {
		kind = "derived"
	}
	if !slices.Contains(request.SourceKinds, kind) {
		return dr.Scope{}, nil, dr.Invalid("/basis", "父查询未请求这种数据。")
	}
	resources := map[string]dr.ResourceSummary{}
	for _, r := range w.Resources {
		resources[r.ID] = r
	}
	scope := dr.Scope{Domain: "channels", Members: []dr.Member{}, Windows: map[string][]access.ReadWindow{}}
	channels := []dr.ChannelSummary{}
	for _, c := range w.Channels {
		if c.SourceKind != kind {
			continue
		}
		key := "restricted_context"
		if r, ok := resources[c.Facility.ID]; ok && r.Visibility == "full" {
			if request.GroupBy == "facility" {
				key = r.ID
			} else if r.Type != nil {
				key = r.Type.DefinitionID
			}
		}
		matches := q.GroupKey == "" || key == q.GroupKey
		if dr.NewStructureKind(request.GroupBy) {
			matches = q.GroupKey == ""
			for _, g := range w.StructureGroups[dr.RefKey(dr.Ref{Kind: "channel", ID: c.ID})] {
				if g.Key == q.GroupKey {
					matches = true
				}
			}
		}
		if !matches {
			continue
		}
		channels = append(channels, c)
		scope.Members = append(scope.Members, dr.Member{Kind: "channel", ID: c.ID})
		for _, part := range w.Windows[c.ID] {
			scope.Windows[c.ID] = append(scope.Windows[c.ID], access.ReadWindow{From: part.From, To: part.To})
		}
	}
	w.Channels = channels
	w.Basis = q.Basis
	w.Action = q.Purpose
	w.Empty = len(channels) == 0
	w.Rows = nil
	w.Statistics = nil
	w.Coverage = nil
	w.Input.Cursor = ""
	for _, part := range p.Scopes {
		if part.Domain == "channels" && part.StructureModel == 2 {
			scope.StructureModel = 2
		}
	}
	scope.Hash = dr.Hash([]any{p.Query.ID, q, scope.Members, scope.Windows})
	return scope, dr.Bytes(w), nil
}

func (s *Service) DisplaySelectionTx(ctx context.Context, tx pgx.Tx, p dr.Plan, raw json.RawMessage) (dr.Materialized, error) {
	var w displayWork
	if e := json.Unmarshal(raw, &w); e != nil {
		return dr.Materialized{}, e
	}
	q := p.Request.(*dr.MonitoringRequest)
	allowed := p.Scopes["target"].IDs("channel")
	selected := []dr.ChannelSummary{}
	ids := []string{}
	for _, c := range w.Channels {
		if !slices.Contains(allowed, c.ID) || len(q.Filters.SourceKinds) > 0 && !slices.Contains(q.Filters.SourceKinds, c.SourceKind) || len(q.Filters.FacilityIDs) > 0 && !slices.Contains(q.Filters.FacilityIDs, c.Facility.ID) || len(q.Filters.Metrics) > 0 && !slices.Contains(q.Filters.Metrics, c.Metric) || len(q.Filters.Owners) > 0 && !slices.Contains(q.Filters.Owners, c.Owner) {
			continue
		}
		selected = append(selected, c)
		ids = append(ids, c.ID)
	}
	if p.Method == "monitoring.trends" && len(ids) > dr.MaxTrendChannels {
		return dr.Materialized{}, dr.Problem("budget_exceeded", "趋势最多 50 个通道，请在下钻选择内收窄。", 429)
	}
	w.Channels = selected
	w.Empty = len(ids) == 0
	if w.Empty {
		return dr.Materialized{Work: dr.Bytes(w), Proof: dr.Bytes(w)}, nil
	}
	actor := p.ReadSubject().ReadActor()
	scope := displayScope(p)
	auth, e := s.displayAuthorizationTx(ctx, tx, actor, scope)
	if e != nil {
		return dr.Materialized{}, e
	}
	parent, e := s.queryPlanTx(ctx, tx, actor, scope, w.Input, "overview", "read", false, auth)
	if e != nil {
		return dr.Materialized{}, e
	}
	selection, e := s.selectTx(ctx, tx, actor, scope, SelectionInput{Channels: ids}, auth)
	if e != nil {
		return dr.Materialized{}, e
	}
	windows := map[string][]Window{}
	vector := snapshotVector{Integrity: map[string]string{}, Revokes: map[string]string{}, Contents: map[string]string{}, Calculated: map[string]CalculationHead{}, Secret: rand.Text() + rand.Text()}
	for _, id := range ids {
		windows[id] = w.Windows[id]
		vector.Integrity[id] = parent.vector.Integrity[id]
		vector.Revokes[id] = parent.vector.Revokes[id]
		vector.Contents[id] = parent.vector.Contents[id]
		if h, ok := parent.vector.Calculated[id]; ok {
			vector.Calculated[id] = h
		}
	}
	in := w.Input
	in.Selection = selection.ID
	in.Snapshot = ""
	in.Cursor = ""
	in.Limit = dr.MaxDetails
	in.PlotWidth = q.PlotWidth
	in.PlotBudget = q.PlotBudget
	in.Period = q.Period
	if w.Basis == "raw_active" || w.Basis == "derived_logical" {
		in.Qualities = slices.Clone(allQualities)
	}
	in, e = normalizedQuery(in)
	if e != nil {
		return dr.Materialized{}, e
	}
	_, e = tx.Exec(ctx, `UPDATE monitoring_selections SET read_windows=$2,expires_at=LEAST(expires_at,$3) WHERE id=$1`, selection.ID, encodeJSON(windows), p.Query.ExpiresAt)
	if e != nil {
		return dr.Materialized{}, e
	}
	e = tx.QueryRow(ctx, `INSERT INTO monitoring_snapshots(tenant_id,selection_id,actor_id,display_subject_id,watermark,route_revision,query_hash,revision_vector,authorization_revision,expires_at) VALUES($1,$2,$3,$4,$5::bigint,$6::bigint,$7,$8,$9,$10) RETURNING id::text`, scope.Tenant, in.Selection, actor.OwnerAccount(), actor.OwnerSubject(), parent.snapshot.Watermark, parent.snapshot.RouteRevision, filterHash(in), encodeJSON(vector), authorizationVersion(auth), p.Query.ExpiresAt).Scan(&in.Snapshot)
	if e != nil {
		return dr.Materialized{}, e
	}
	w.Input = in
	w.Windows = windows
	w.Heads = vector.Calculated
	w.Rows = nil
	w.Statistics = nil
	w.Coverage = nil
	return dr.Materialized{Work: dr.Bytes(w), Proof: dr.Bytes(w)}, nil
}
