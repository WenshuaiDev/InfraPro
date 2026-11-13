package resources

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

// CalculationTarget is resource metadata and maintenance authority only. It
// never confers observation read, history or export privileges.
type CalculationTarget struct {
	Resource                      Resource
	Retired, CanCreate, CanManage bool
	AccessRevision, LinkRevision  string
}

func targetKey(t Target) string { return string(t.Kind) + ":" + t.ID }

// CalculationTargetsTx resolves a bounded configuration's owners and inputs.
// The caller holds authorization/catalog/tenant/graph locks. All facilities are
// acquired in sorted order before any channel/extension lock, even across sites.
func (s *Service) CalculationTargetsTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, targets []Target, write bool) (map[string]CalculationTarget, error) {
	out := map[string]CalculationTarget{}
	if !scope.valid() || len(targets) > 65 {
		return out, notFound()
	}
	auth, e := s.authorizeTx(ctx, tx, a, scope.operation("channel", "detail"), scope)
	if e != nil {
		return out, e
	}
	resolved := make([]Target, 0, len(targets))
	facilities := []string{}
	for _, t := range targets {
		if tables[t.Kind] == "" || !access.ValidID(t.ID) {
			return out, notFound()
		}
		var fid string
		e := tx.QueryRow(ctx, `SELECT facility_id::text FROM engineering_resource_nodes WHERE tenant_id=$1 AND kind=$2 AND id=$3 AND deleted_at IS NULL`, scope.Tenant, t.Kind, t.ID).Scan(&fid)
		if e == pgx.ErrNoRows {
			return out, notFound()
		}
		if e != nil {
			return out, e
		}
		if t.Facility != "" && t.Facility != fid {
			return out, notFound()
		}
		t.Facility = fid
		resolved = append(resolved, t)
		facilities = append(facilities, fid)
	}
	slices.Sort(facilities)
	facilities = slices.Compact(facilities)
	lock := " FOR SHARE"
	if write {
		lock = " FOR UPDATE"
	}
	rows, e := tx.Query(ctx, `SELECT id FROM engineering_facilities WHERE tenant_id=$1 AND id=ANY($2::uuid[]) ORDER BY id`+lock, scope.Tenant, facilities)
	if e != nil {
		return out, e
	}
	for rows.Next() {
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	for _, t := range resolved {
		if _, ok := out[targetKey(t)]; ok {
			continue
		}
		st := scoped(scope, t)
		v := CalculationTarget{AccessRevision: auth.AccessRevision}
		if scope.Entry == "project" {
			g, link, e := projectGrantTx(ctx, tx, auth, st, true)
			if e != nil {
				return out, e
			}
			v.LinkRevision = link.Revision
			v.CanCreate = slices.Contains(g.Capabilities, "create_derived_channel")
			v.CanManage = slices.Contains(g.Capabilities, "manage_calculation")
		} else {
			v.CanCreate = true
			v.CanManage = true
		}
		v.Resource, e = load(ctx, tx, st, false)
		if e != nil {
			return out, e
		}
		f, e := load(ctx, tx, scoped(scope, Target{Kind: Facility, ID: t.Facility}), false)
		if e != nil {
			return out, e
		}
		ancestors, e := ancestorsTx(ctx, tx, st, v.Resource, f)
		if e != nil {
			return out, e
		}
		v.Retired = v.Resource.Status != "normal"
		for _, r := range ancestors {
			v.Retired = v.Retired || r.Status != "normal"
		}
		out[targetKey(t)] = v
	}
	return out, nil
}

// CreateCalculationChannelTx inserts the established engineering resource and
// its resource audit. The caller must insert the deferred calculation extension
// in the same transaction. Project data grants are intentionally not created.
func (s *Service) CreateCalculationChannelTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, owner Target, r Resource, request, reason string, meta access.Meta) (Resource, error) {
	auth, e := s.authorizeTx(ctx, tx, a, scope.operation("channel", "create"), scope)
	if e != nil {
		return r, e
	}
	if owner.Kind == Channel || r.SourceKind != "derived" || r.Kind != Channel || r.TenantID != scope.Tenant || r.FacilityID != owner.Facility || !access.ValidID(r.ID) {
		return r, notFound()
	}
	targets, e := s.CalculationTargetsTx(ctx, tx, a, scope, []Target{owner}, true)
	if e != nil {
		return r, e
	}
	parent := targets[targetKey(owner)]
	if parent.Retired {
		return r, problem("ancestor_retired", "所属对象或祖先已退役。", 409)
	}
	if !parent.CanCreate {
		return r, access.ErrDenied
	}
	if e = validateBasic(r); e != nil {
		return r, e
	}
	var link *Link
	if scope.Entry == "project" {
		link, e = linkTx(ctx, tx, scope.Tenant, scope.Project, r.FacilityID)
		if e != nil {
			return r, e
		}
		r.CreatedProjectID = &scope.Project
	}
	r.CreatedBy = a.ID
	r.UpdatedBy = a.ID
	r.Revision = 1
	r.Status = "normal"
	columns := `id,tenant_id,code,name,status,revision,created_by,updated_by,facility_id,point_id,component_id,source_kind,owner_kind,derived_metric_version_id,quantity_spec,output_contract_hash,output_semantics,observation_direction,reference_basis,timestamp_meaning,aggregation_description,description,created_project_id`
	var raw []byte
	e = tx.QueryRow(ctx, `INSERT INTO engineering_channels(`+columns+`) SELECT `+columns+` FROM jsonb_populate_record(NULL::engineering_channels,$1::jsonb) RETURNING to_jsonb(engineering_channels)`, jsonBytes(r)).Scan(&raw)
	if e != nil {
		return r, e
	}
	if e = json.Unmarshal(raw, &r); e != nil {
		return r, e
	}
	linkID := ""
	if scope.Entry == "project" {
		linkID, _, _, e = projectCreateGrant(ctx, tx, auth, r, link, request, meta)
		if e != nil {
			return r, e
		}
	}
	e = recordChange(ctx, tx, a, auth, scoped(scope, Target{Kind: Channel, ID: r.ID, Facility: r.FacilityID, Point: r.PointID}), "create", WriteMeta{RequestID: request, Reason: reason}, nil, r, r.FacilityID, linkID, meta)
	return r, e
}
