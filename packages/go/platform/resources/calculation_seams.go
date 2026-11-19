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
	resolved, e := lockCalculationTargetsTx(ctx, tx, scope.Tenant, targets, write)
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

// lockCalculationTargetsTx resolves each typed identity first and then locks all
// facilities in the same order for both metadata reads and configuration writes.
func lockCalculationTargetsTx(ctx context.Context, tx pgx.Tx, tenant string, targets []Target, write bool) ([]Target, error) {
	resolved := make([]Target, 0, len(targets))
	facilities := []string{}
	for _, t := range targets {
		if tables[t.Kind] == "" || !access.ValidID(t.ID) {
			return nil, notFound()
		}
		var fid string
		e := tx.QueryRow(ctx, `SELECT facility_id::text FROM engineering_resource_nodes WHERE tenant_id=$1 AND kind=$2 AND id=$3 AND deleted_at IS NULL`, tenant, t.Kind, t.ID).Scan(&fid)
		if e == pgx.ErrNoRows {
			return nil, notFound()
		}
		if e != nil {
			return nil, e
		}
		if t.Facility != "" && t.Facility != fid {
			return nil, notFound()
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
	rows, e := tx.Query(ctx, `SELECT id FROM engineering_facilities WHERE tenant_id=$1 AND id=ANY($2::uuid[]) ORDER BY id`+lock, tenant, facilities)
	if e != nil {
		return nil, e
	}
	for rows.Next() {
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	return resolved, nil
}

// CheckCalculationInputChannelsTx checks the resource proof for a published
// result without loading its input channels' engineering records. A retired
// channel or ancestor still permits historical reads while a deleted one does not.
func (s *Service) CheckCalculationInputChannelsTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, channelIDs []string) error {
	if !scope.valid() || len(channelIDs) > 16 {
		return notFound()
	}
	if _, err := s.authorizeTx(ctx, tx, a, scope.operation("channel", "detail"), scope); err != nil {
		return err
	}
	targets := make([]Target, 0, len(channelIDs))
	for _, id := range channelIDs {
		targets = append(targets, Target{Kind: Channel, ID: id})
	}
	targets, err := lockCalculationTargetsTx(ctx, tx, scope.Tenant, targets, false)
	if err != nil {
		return err
	}
	seen := make(map[string]bool, len(targets))
	for _, target := range targets {
		if seen[target.ID] {
			continue
		}
		seen[target.ID] = true
		if scope.Entry == "project" {
			var granted bool
			err := tx.QueryRow(ctx, `SELECT EXISTS (
				SELECT 1 FROM project_facility_links l JOIN project_resource_grants g ON g.link_id=l.id
				WHERE g.tenant_id=$1 AND g.project_id=$2 AND g.facility_id=$3 AND l.status='active'
				  AND g.resource_kind='channel' AND g.channel_id=$4 AND g.ended_at IS NULL
			)`, scope.Tenant, scope.Project, target.Facility, target.ID).Scan(&granted)
			if err != nil {
				return err
			}
			if !granted {
				return notFound()
			}
		}
		if err := checkCalculationInputPathTx(ctx, tx, scope.Tenant, target); err != nil {
			return err
		}
	}
	return nil
}

func checkCalculationInputPathTx(ctx context.Context, tx pgx.Tx, tenant string, channel Target) error {
	// A channel attached to a point may have one more ancestor than a structural
	// node. Use the same bound and typed identity as structureAncestorsTx.
	const query = `WITH RECURSIVE path(kind,id,parent_kind,parent_id,depth) AS (
		SELECT n.kind,n.id,n.parent_kind,n.parent_id,0 FROM engineering_resource_nodes n
		WHERE n.tenant_id=$1 AND n.facility_id=$2 AND n.kind='channel' AND n.id=$3 AND n.deleted_at IS NULL
		UNION ALL
		SELECT n.kind,n.id,n.parent_kind,n.parent_id,p.depth+1 FROM path p
		JOIN engineering_resource_nodes n ON n.tenant_id=$1 AND n.facility_id=$2 AND n.kind=p.parent_kind AND n.id=p.parent_id
		WHERE n.deleted_at IS NULL AND p.depth<$4
	) CYCLE kind,id SET is_cycle USING visited
	SELECT COALESCE(bool_or(depth=0 AND (parent_id IS NULL OR parent_kind IS NULL OR parent_kind NOT IN ('facility','component','monitoring_section','borehole','point'))),false),
	       COALESCE(bool_or(is_cycle AND kind='channel' AND id=$3),false),
	       COALESCE(bool_or(kind='facility' AND id=$2 AND parent_id IS NULL),false)
	FROM path`
	var invalidParent, selfCycle, reachedFacility bool
	if err := tx.QueryRow(ctx, query, tenant, channel.Facility, channel.ID, MaxStructureDepth+1).Scan(&invalidParent, &selfCycle, &reachedFacility); err != nil {
		return err
	}
	switch {
	case invalidParent:
		return invalid("/parent_ref", "parent_reference", "请选择合法的同设施父对象。")
	case selfCycle:
		return invalid("/parent_ref", "cycle", "不能指向自身或后代。")
	case !reachedFacility:
		return notFound()
	default:
		return nil
	}
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
	columns := `id,tenant_id,code,name,status,revision,created_by,updated_by,facility_id,point_id,component_id,monitoring_section_id,borehole_id,source_kind,owner_kind,derived_metric_version_id,quantity_spec,output_contract_hash,output_semantics,observation_direction,reference_basis,timestamp_meaning,aggregation_description,description,created_project_id`
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
