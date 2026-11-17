package displayread

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"slices"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

type DrilldownHandler func(context.Context, pgx.Tx, Plan, json.RawMessage, *DrilldownRequest) (Scope, json.RawMessage, error)
type SelectionHandler func(context.Context, pgx.Tx, Plan, json.RawMessage) (Materialized, error)
type fixedSelection struct {
	Scope Scope           `json:"scope"`
	Work  json.RawMessage `json:"work"`
}
type DrilldownSelection struct {
	Selection     string    `json:"selection"`
	ParentQueryID string    `json:"parent_query_id"`
	Basis         string    `json:"basis"`
	Purpose       string    `json:"purpose"`
	ExpiresAt     time.Time `json:"expires_at"`
}

func (s *Service) WithDrilldown(makeSelection DrilldownHandler, prepare SelectionHandler) {
	s.drilldown, s.selectionPrepare = makeSelection, prepare
}

func (s *Service) parentPlanTx(ctx context.Context, tx pgx.Tx, a Authority, id string) (Plan, json.RawMessage, error) {
	p := Plan{Authority: a}
	var proof, request json.RawMessage
	var state string
	e := tx.QueryRow(ctx, `SELECT method,request,scopes,source_vector,id::text,read_snapshot_id::text,metadata_as_of,created_at,expires_at,scope_hash,filter_hash,proof,state FROM display_read_queries WHERE id=$1 AND subject_id=$2 AND session_id=$3 AND config_id=$4 AND environment=$5`, id, a.Owner.Subject, a.Owner.Session, a.Owner.Config, a.Owner.Environment).Scan(&p.Method, &request, &p.Scopes, &p.Evidence, &p.Query.ID, &p.Query.SnapshotID, &p.Query.MetadataAsOf, &p.Query.CreatedAt, &p.Query.ExpiresAt, &p.Query.ScopeHash, &p.Query.FilterHash, &proof, &state)
	if e == pgx.ErrNoRows {
		return p, nil, Hidden()
	}
	if e != nil {
		return p, nil, e
	}
	if !slices.Contains([]string{"statistics.monitoring", "statistics.structure", "statistics.deviceCategories"}, p.Method) || !slices.Contains([]string{"ready", "empty", "partial"}, state) {
		return p, nil, Problem("query_invalid", "下钻需要已完成的统计查询。", 409)
	}
	if !p.Query.ExpiresAt.After(time.Now()) {
		return p, nil, Problem("query_expired", "父查询已过期。", 409)
	}
	p.Query.Method = p.Method
	p.Request, e = DecodeRequest(p.Method, request)
	if e != nil {
		return p, nil, e
	}
	current, e := a.Validate(ctx, tx, p.Method, &p.Evidence)
	if e != nil {
		return p, nil, e
	}
	if current.LeaseUntil.Before(p.Query.ExpiresAt) {
		p.Query.ExpiresAt = current.LeaseUntil
	}
	if validate := s.validators[p.Method]; validate != nil {
		if e = validate(ctx, tx, p, proof); e != nil {
			return p, nil, e
		}
	}
	return p, proof, nil
}
func (s *Service) selectionFieldsTx(ctx context.Context, tx pgx.Tx, p Plan, wanted Scope, requested []string) ([]string, error) {
	fields := []string(nil)
	minimum := map[string]string{"channels": "monitoring.values", "resources": "resource.metadata", "devices": "device.metadata"}[wanted.Domain]
	covered := []Member{}
	for _, n := range needs(p.Method, p.Request) {
		eligible := n.domain == wanted.Domain || wanted.Domain == "resources" && n.domain == "channels"
		if !eligible || len(wanted.Members) > 0 && len(IntersectMembers(p.Scopes[n.key].Members, wanted.Members)) == 0 {
			continue
		}
		v, e := p.Authority.Resolve(ctx, tx, n.target, n.domain, requested, []string{minimum})
		if e != nil {
			return nil, e
		}
		covered = append(covered, IntersectMembers(v.Members, wanted.Members)...)
		if fields == nil {
			fields = slices.Clone(v.FieldGroups)
		} else {
			fields = slices.DeleteFunc(fields, func(f string) bool { return !slices.Contains(v.FieldGroups, f) })
		}
	}
	if fields == nil {
		return nil, Problem("field_not_granted", "此选择未获明细字段资格。", 403)
	}
	if len(IntersectMembers(wanted.Members, covered)) != len(wanted.Members) {
		return nil, Hidden()
	}
	return fields, nil
}

func selectionMethod(purpose string) string {
	switch purpose {
	case "devices":
		return "devices.list"
	case "resources":
		return "resources.query"
	default:
		return "monitoring." + purpose
	}
}
func (s *Service) createSelection(ctx context.Context, a Authority, q *DrilldownRequest, generation string) (out Result, err error) {
	err = s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, false); e != nil {
			return e
		}
		if e := a.Require(ctx, tx, "queries.drilldown"); e != nil {
			return e
		}
		if e := a.Require(ctx, tx, selectionMethod(q.Purpose)); e != nil {
			return e
		}
		p, proof, e := s.parentPlanTx(ctx, tx, a, q.QueryID)
		if e != nil {
			return e
		}
		if q.GroupKey != "" {
			var found bool
			if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM display_read_query_rows WHERE query_id=$1 AND data->>'group_key'=$2)`, q.QueryID, q.GroupKey).Scan(&found); e != nil {
				return e
			}
			if !found {
				return Hidden()
			}
		}
		if s.drilldown == nil {
			return Problem("capability_unavailable", "下钻读取未安装。", 409)
		}
		scope, work, e := s.drilldown(ctx, tx, p, proof, q)
		if e != nil {
			return e
		}
		scope.FieldGroups, e = s.selectionFieldsTx(ctx, tx, p, scope, nil)
		if e != nil {
			return e
		}
		token := rand.Text() + rand.Text()
		_, e = tx.Exec(ctx, `INSERT INTO display_read_selections(token,query_id,subject_id,group_key,basis,purpose,selection,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, token, q.QueryID, a.Owner.Subject, q.GroupKey, q.Basis, q.Purpose, Bytes(fixedSelection{scope, work}), p.Query.ExpiresAt)
		if e != nil {
			return e
		}
		out = Result{State: "ready", Data: DrilldownSelection{token, q.QueryID, q.Basis, q.Purpose, p.Query.ExpiresAt}, ReadAt: time.Now().UTC().Format(time.RFC3339Nano), Authorization: p.Evidence.Authorization, Generation: generation, SourceVector: p.Evidence.Vector, IncludedFields: scope.FieldGroups}
		return nil
	})
	return
}
func (s *Service) preparePlanTx(ctx context.Context, tx pgx.Tx, a Authority, method string, request any) (Plan, *Materialized, error) {
	var target Target
	var fields []string
	switch q := request.(type) {
	case *MonitoringRequest:
		target, fields = q.Target, q.FieldGroups
	case *DevicesRequest:
		target, fields = q.Target, q.FieldGroups
	case *ResourcesRequest:
		target, fields = q.Target, q.FieldGroups
	}
	if target.Selection == "" {
		p, e := resolvePlan(ctx, tx, a, method, request)
		return p, nil, e
	}
	var selected fixedSelection
	var parent, purpose string
	var expires time.Time
	e := tx.QueryRow(ctx, `SELECT selection,query_id::text,purpose,expires_at FROM display_read_selections WHERE token=$1 AND subject_id=$2`, target.Selection, a.Owner.Subject).Scan(&selected, &parent, &purpose, &expires)
	if e == pgx.ErrNoRows {
		return Plan{}, nil, Hidden()
	}
	if e != nil {
		return Plan{}, nil, e
	}
	if method != selectionMethod(purpose) {
		return Plan{}, nil, Hidden()
	}
	if !expires.After(time.Now()) {
		return Plan{}, nil, Problem("query_expired", "下钻选择已过期。", 409)
	}
	p, _, e := s.parentPlanTx(ctx, tx, a, parent)
	if e != nil {
		return p, nil, e
	}
	if e = a.Require(ctx, tx, method); e != nil {
		return p, nil, e
	}
	selected.Scope.FieldGroups, e = s.selectionFieldsTx(ctx, tx, p, selected.Scope, fields)
	if e != nil {
		return p, nil, e
	}
	if q, ok := request.(*MonitoringRequest); ok && q.History {
		return p, nil, Invalid("/history", "下钻沿用父查询在用数据口径。")
	}
	if target.IDs != nil {
		wanted := map[string]bool{}
		for _, id := range target.IDs {
			wanted[id] = true
		}
		selected.Scope.Members = slices.DeleteFunc(selected.Scope.Members, func(m Member) bool {
			if wanted[m.ID] {
				delete(wanted, m.ID)
				return false
			}
			delete(selected.Scope.Windows, m.ID)
			return true
		})
		if len(wanted) > 0 {
			return p, nil, Hidden()
		}
	}
	p.ParentQuery = parent
	p.Method = method
	p.Request = request
	p.Scopes = map[string]Scope{"target": selected.Scope}
	if expires.Before(p.Query.ExpiresAt) {
		p.Query.ExpiresAt = expires
	}
	if s.selectionPrepare == nil {
		return p, nil, Problem("capability_unavailable", "下钻执行未安装。", 409)
	}
	data, e := s.selectionPrepare(ctx, tx, p, selected.Work)
	return p, &data, e
}
func (s *Service) checkParentTx(ctx context.Context, tx pgx.Tx, id string) error {
	var valid bool
	e := tx.QueryRow(ctx, `SELECT c.parent_query_id IS NULL OR p.state IN ('ready','partial','empty') AND p.expires_at>clock_timestamp() FROM display_read_queries c LEFT JOIN display_read_queries p ON p.id=c.parent_query_id WHERE c.id=$1`, id).Scan(&valid)
	if e != nil {
		return e
	}
	if !valid {
		return Problem("query_expired", "父查询已取消或过期。", 409)
	}
	return nil
}
