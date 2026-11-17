package monitoring

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dr "github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicecoord"
	"github.com/jackc/pgx/v5"
)

func (s *Service) displayAuthorizationTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope) (authorization, error) {
	auth := authorization{Scope: scope, Account: a.ID, Subject: a.ReadSubject(), ResourceRevision: "0", DataRevision: "0"}
	var e error
	if a.Check == nil || scope.Entry != "project" {
		return auth, access.ErrDenied
	}
	auth.Revision, e = access.Lock(ctx, tx, false)
	if e != nil {
		return auth, e
	}
	if e = a.Check(ctx, tx, false); e != nil {
		return auth, e
	}
	project, e := access.RequireActorProjectTx(ctx, tx, a, access.MonitoringOperation("project", "selections.create"), scope.Tenant, scope.Project)
	if e != nil {
		return auth, e
	}
	auth.ProjectStatus = project.ProjectStatus
	e = tx.QueryRow(ctx, `SELECT r.revision::text,COALESCE(m.revision,0)::text FROM project_resource_policies r LEFT JOIN monitoring_access_states m USING(tenant_id,project_id) WHERE r.tenant_id=$1 AND r.project_id=$2`, scope.Tenant, scope.Project).Scan(&auth.ResourceRevision, &auth.DataRevision)
	if e != nil {
		return auth, e
	}
	if _, e = tx.Exec(ctx, `SELECT id FROM engineering_catalog_state WHERE id=1 FOR SHARE`); e != nil {
		return auth, e
	}
	if e = devicecoord.Catalog(ctx, tx, false); e != nil {
		return auth, e
	}
	if e = devicecoord.Tenant(ctx, tx, scope.Tenant, false); e != nil {
		return auth, e
	}
	var mode string
	if e = tx.QueryRow(ctx, `SELECT mode FROM monitoring_environment_state WHERE id=1`).Scan(&mode); e != nil {
		return auth, e
	}
	if mode == "restoring" {
		return auth, problem("monitoring_restoring", "监测环境正在恢复。", 503)
	}
	return auth, nil
}
func displayScope(p dr.Plan) Scope {
	return ProjectScope(p.Authority.Owner.Tenant, p.Authority.Owner.Project)
}
func displayIDs(p dr.Plan) []string {
	ids := []string{}
	for _, scope := range p.Scopes {
		if scope.Domain == "channels" {
			ids = append(ids, scope.IDs("channel")...)
		}
	}
	slices.Sort(ids)
	return slices.Compact(ids)
}
func (s *Service) DisplayChannelsTx(ctx context.Context, tx pgx.Tx, p dr.Plan, ids []string, filter dr.ChannelFilters, search string) ([]dr.ChannelSummary, map[string][]Window, error) {
	a := p.ReadSubject().ReadActor()
	auth, e := s.displayAuthorizationTx(ctx, tx, a, displayScope(p))
	if e != nil {
		return nil, nil, e
	}
	if len(ids) == 0 {
		return []dr.ChannelSummary{}, map[string][]Window{}, nil
	}
	windows, e := authorizedWindowsTx(ctx, tx, auth, ids, "read")
	if e != nil {
		return nil, nil, e
	}
	legacy := []string{}
	for _, scope := range p.Scopes {
		if scope.Domain == "channels" && scope.StructureModel != 2 {
			for _, id := range scope.IDs("channel") {
				if slices.Contains(ids, id) {
					legacy = append(legacy, id)
				}
			}
		}
	}
	if len(legacy) > 0 {
		var bad bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM engineering_channels WHERE tenant_id=$1 AND id=ANY($2::uuid[]) AND owner_kind IN ('monitoring_section','borehole') AND deleted_at IS NULL)`, auth.Tenant, legacy).Scan(&bad); e != nil {
			return nil, nil, e
		}
		if bad {
			return nil, nil, dr.StructureUpgradeRequired()
		}
	}
	rows, e := tx.Query(ctx, `SELECT jsonb_build_object('id',c.id,'code',c.code,'name',c.name,'status',c.status,'revision',c.revision::text,'source_kind',c.source_kind,
 'facility_ref',jsonb_build_object('kind','facility','id',c.facility_id),'owner_ref',jsonb_build_object('kind',c.owner_kind,'id',CASE c.owner_kind WHEN 'facility' THEN c.facility_id WHEN 'component' THEN c.component_id WHEN 'monitoring_section' THEN c.monitoring_section_id WHEN 'borehole' THEN c.borehole_id ELSE c.point_id END),
 'point_ref',CASE WHEN c.point_id IS NULL THEN NULL ELSE jsonb_build_object('kind','point','id',c.point_id) END,
 'metric_ref',jsonb_build_object('kind',CASE WHEN c.source_kind='derived' THEN 'derived' ELSE 'engineering' END,'version_id',COALESCE(c.derived_metric_version_id,c.metric_version_id)),
 'metric_name',COALESCE(dm.name,m.content->>'name',''),'unit_version_id',c.unit_version_id,'unit',COALESCE(u.content->'unit'->>'symbol',''),'value_kind',CASE WHEN c.source_kind='derived' THEN 'decimal' ELSE m.content->'metric'->>'shape' END,
 'timestamp_meaning',c.timestamp_meaning,'aggregation_description',c.aggregation_description,'observation_direction',c.observation_direction,'reference_basis',c.reference_basis,'sampling',c.sampling,'quantity_spec',c.quantity_spec,'output_semantics',c.output_semantics)
 FROM engineering_channels c LEFT JOIN engineering_versions m ON m.id=c.metric_version_id LEFT JOIN derived_metric_versions dm ON dm.tenant_id=c.tenant_id AND dm.id=c.derived_metric_version_id LEFT JOIN engineering_versions u ON u.id=c.unit_version_id
 WHERE c.tenant_id=$1 AND c.id=ANY($2::uuid[]) AND c.deleted_at IS NULL
 AND ($3='' OR strpos(lower(c.name),lower($3))>0 OR strpos(lower(c.code),lower($3))>0)
 AND (COALESCE(cardinality($4::uuid[]),0)=0 OR c.facility_id=ANY($4)) AND (COALESCE(cardinality($5::text[]),0)=0 OR c.source_kind=ANY($5))
 AND (jsonb_array_length($6::jsonb)=0 OR EXISTS(SELECT 1 FROM jsonb_array_elements($6::jsonb) m WHERE (m->>'kind'='engineering' AND m->>'version_id'=c.metric_version_id::text) OR (m->>'kind'='derived' AND m->>'version_id'=c.derived_metric_version_id::text)))
 AND (jsonb_array_length($7::jsonb)=0 OR EXISTS(SELECT 1 FROM jsonb_array_elements($7::jsonb) o WHERE o->>'kind'=c.owner_kind AND o->>'id'=CASE c.owner_kind WHEN 'facility' THEN c.facility_id::text WHEN 'component' THEN c.component_id::text WHEN 'monitoring_section' THEN c.monitoring_section_id::text WHEN 'borehole' THEN c.borehole_id::text ELSE c.point_id::text END)) ORDER BY c.id`, auth.Tenant, ids, search, filter.FacilityIDs, filter.SourceKinds, jsonArray(filter.Metrics), jsonArray(filter.Owners))
	if e != nil {
		return nil, nil, e
	}
	out := []dr.ChannelSummary{}
	selected := map[string][]Window{}
	for rows.Next() {
		var raw []byte
		var item dr.ChannelSummary
		if e = rows.Scan(&raw); e != nil {
			rows.Close()
			return nil, nil, e
		}
		if e = json.Unmarshal(raw, &item); e != nil {
			rows.Close()
			return nil, nil, e
		}
		item.ReadRanges = []dr.Window{}
		for _, w := range windows[item.ID] {
			r := w.rangeDTO()
			item.ReadRanges = append(item.ReadRanges, dr.Window{From: r.From, To: r.To})
		}
		out = append(out, item)
		selected[item.ID] = windows[item.ID]
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, nil, e
	}
	if s.displayQualification != nil {
		if e = s.displayQualification(ctx, tx, a, displayScope(p), selected); e != nil {
			return nil, nil, e
		}
	}
	return out, selected, nil
}

type displayWork struct {
	StructureGroups map[string][]dr.StructureGroupRef `json:"structure_groups,omitempty"`
	Basis           string                            `json:"basis,omitempty"`
	Watermark       string                            `json:"watermark"`
	Route           string                            `json:"route_revision"`
	Input           QueryInput                        `json:"input"`
	Action          string                            `json:"action"`
	Channels        []dr.ChannelSummary               `json:"channels"`
	Resources       []dr.ResourceSummary              `json:"resources"`
	Windows         map[string][]Window               `json:"windows"`
	Heads           map[string]CalculationHead        `json:"heads"`
	Statistics      []ChannelStatistic                `json:"statistics,omitempty"`
	Rows            []json.RawMessage                 `json:"rows,omitempty"`
	Coverage        []CalculationCoverage             `json:"coverage,omitempty"`
	Proofs          []json.RawMessage                 `json:"proofs,omitempty"`
	Empty           bool                              `json:"empty,omitempty"`
	ScopeLimited    bool                              `json:"scope_limited"`
}

func (s *Service) DisplayDirectoryTx(ctx context.Context, tx pgx.Tx, p dr.Plan) (dr.Materialized, error) {
	q := p.Request.(*dr.ChannelsRequest)
	items, _, e := s.DisplayChannelsTx(ctx, tx, p, p.Scopes["target"].IDs("channel"), q.Filters, q.Search)
	out := dr.Materialized{Rows: []json.RawMessage{}, Proof: dr.Bytes(map[string]bool{"directory": true})}
	if e != nil {
		return out, e
	}
	for _, item := range items {
		if p.Method == "channels.get" {
			if item.ID == q.ID {
				out.Single = item
			}
		} else {
			out.Rows = append(out.Rows, dr.Bytes(item))
		}
	}
	if p.Method == "channels.get" && out.Single == nil {
		return out, dr.Hidden()
	}
	return out, nil
}
func (s *Service) DisplayPrepareTx(ctx context.Context, tx pgx.Tx, p dr.Plan, resources []dr.ResourceSummary, structureGroups ...map[string][]dr.StructureGroupRef) (dr.Materialized, error) {
	out := dr.Materialized{Rows: []json.RawMessage{}}
	if e := s.requireStorage(); e != nil {
		return out, e
	}
	var timing dr.MonitoringTime
	var filter dr.ChannelFilters
	action := strings.TrimPrefix(p.Method, "monitoring.")
	query := QueryInput{Limit: 1000}
	switch q := p.Request.(type) {
	case *dr.MonitoringStatisticsRequest:
		timing = q.MonitoringTime
		filter.SourceKinds = q.SourceKinds
		action = "statistics"
	case *dr.MonitoringRequest:
		timing = q.MonitoringTime
		filter = q.Filters
		query.PlotWidth = q.PlotWidth
		query.PlotBudget = q.PlotBudget
		query.Period = q.Period
		if q.History {
			query.Status = "history"
		}
	}
	items, windows, e := s.DisplayChannelsTx(ctx, tx, p, displayIDs(p), filter, "")
	if e != nil {
		return out, e
	}
	work := displayWork{Action: action, Channels: items, Windows: windows, Resources: resources, Rows: []json.RawMessage{}, Statistics: []ChannelStatistic{}, Coverage: []CalculationCoverage{}, Proofs: []json.RawMessage{}}
	if len(structureGroups) > 0 {
		work.StructureGroups = structureGroups[0]
	}
	query.AllHistory = timing.AllHistory
	query.Qualities = timing.Qualities
	query.Timezone = timing.Timezone
	if timing.Range != nil {
		query.Range = &Range{From: timing.Range.From, To: timing.Range.To}
	}
	if timing.FollowSeconds > 0 {
		to := time.Now().UTC()
		from := to.Add(-time.Duration(timing.FollowSeconds) * time.Second)
		f, t := from.Format(time.RFC3339Nano), to.Format(time.RFC3339Nano)
		query.Range = &Range{From: &f, To: &t}
	}
	if action == "latest" && query.Range == nil && !query.AllHistory {
		query.AllHistory = true
	}
	if action == "latest" && len(query.Qualities) == 0 {
		query.Qualities = slices.Clone(allQualities)
	}
	if len(items) == 0 {
		work.Empty = true
		work.Input = query
		out.Work = dr.Bytes(work)
		out.Proof = out.Work
		return out, nil
	}
	ids := []string{}
	for _, v := range items {
		ids = append(ids, v.ID)
	}
	actor := p.ReadSubject().ReadActor()
	auth, e := s.displayAuthorizationTx(ctx, tx, actor, displayScope(p))
	if e != nil {
		return out, e
	}
	selection, e := s.selectTx(ctx, tx, actor, displayScope(p), SelectionInput{Channels: ids}, auth)
	if e != nil {
		return out, e
	}
	query.Selection = selection.ID
	query, e = normalizedQuery(query)
	if e != nil {
		return out, e
	}
	requested := Window{}
	if query.Range != nil {
		requested, e = query.Range.window()
		if e != nil {
			return out, e
		}
	}
	for id, values := range windows {
		selected := []Window{}
		for _, w := range values {
			if v, ok := w.Intersect(requested); ok {
				selected = append(selected, v)
			}
		}
		windows[id] = UnionWindows(selected)
		covered := false
		for _, w := range windows[id] {
			if w.Covers(requested) {
				covered = true
			}
		}
		if !covered {
			work.ScopeLimited = true
		}
	}
	if _, e = tx.Exec(ctx, `UPDATE monitoring_selections SET read_windows=$2 WHERE id=$1`, selection.ID, encodeJSON(windows)); e != nil {
		return out, e
	}
	plan, e := s.queryPlanTx(ctx, tx, actor, displayScope(p), query, action, "read", false, auth)
	if e != nil {
		return out, e
	}
	query.Snapshot = plan.snapshot.ID
	work.Input = query
	work.Heads = plan.vector.Calculated
	work.Watermark = plan.snapshot.Watermark
	work.Route = plan.snapshot.RouteRevision
	work.Windows = plan.windows
	out.Work = dr.Bytes(work)
	out.Proof = out.Work
	return out, nil
}
func (s *Service) DisplayCheckTx(ctx context.Context, tx pgx.Tx, p dr.Plan, raw json.RawMessage) error {
	if p.Method == "channels.list" || p.Method == "channels.get" {
		q := p.Request.(*dr.ChannelsRequest)
		_, _, e := s.DisplayChannelsTx(ctx, tx, p, p.Scopes["target"].IDs("channel"), q.Filters, q.Search)
		return e
	}
	if len(raw) == 0 {
		return dr.Problem("query_failed", "读取快照依据缺失。", 409)
	}
	var work displayWork
	if e := json.Unmarshal(raw, &work); e != nil {
		return e
	}
	if work.Empty {
		return nil
	}
	actor := p.ReadSubject().ReadActor()
	auth, e := s.displayAuthorizationTx(ctx, tx, actor, displayScope(p))
	if e != nil {
		return e
	}
	plan, e := s.queryPlanTx(ctx, tx, actor, displayScope(p), work.Input, "overview", "read", false, auth)
	if e != nil {
		return e
	}
	if s.displayQualification != nil {
		if e = s.displayQualification(ctx, tx, actor, displayScope(p), plan.windows); e != nil {
			return e
		}
	}
	if len(work.Proofs) > 0 && s.calculationReader != nil {
		return s.calculationReader.CheckCalculationProofsTx(ctx, tx, actor, displayScope(p), work.Proofs, "read")
	}
	return nil
}
