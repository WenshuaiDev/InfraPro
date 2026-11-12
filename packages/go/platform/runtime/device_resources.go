package runtime

import (
	"context"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/devices"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/jackc/pgx/v5"
)

type installationResources struct{ resources *resources.Service }

func resourceScope(s devices.Scope) resources.Scope {
	if s.Entry == "project" {
		return resources.ProjectScope(s.Tenant, s.Project)
	}
	return resources.TenantScope(s.Tenant)
}
func resourceError(e error) error {
	var re *resources.Error
	if errors.As(e, &re) {
		out := k.Problem(re.Code, re.Message, re.Status)
		for _, i := range re.Issues {
			out.Issues = append(out.Issues, k.Issue{Path: i.Path, Code: i.Code, Message: i.Message})
		}
		return out
	}
	return e
}
func (r installationResources) ResolveInstallationTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope devices.Scope, target devices.Target, history bool) (devices.Installation, error) {
	var out devices.Installation
	resolved, e := r.resources.DeviceTargetTx(ctx, tx, a, resourceScope(scope), resources.Target{Kind: resources.Kind(target.Kind), ID: target.ID})
	if e != nil {
		return out, resourceError(e)
	}
	if !history && resolved.Retired {
		return out, k.Problem("target_retired", "安装目标或祖先已退役，仅可补录已结束历史。", 409)
	}
	v := resolved.Resource
	out.TargetKind = target.Kind
	out.TargetName = v.Name
	out.TargetStatus = v.Status
	out.Visibility = "full"
	fid := v.FacilityID
	if v.Kind == resources.Facility {
		fid = v.ID
	}
	out.FacilityID = &fid
	if v.Kind == resources.Component {
		out.ComponentID = &v.ID
	}
	if v.Kind == resources.Point {
		out.PointID = &v.ID
	}
	return out, nil
}

// ProjectInstallationsTx resolves the entire visible target page in one query;
// a full device grant never implies access to a structured resource target.
func (r installationResources) ProjectInstallationsTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope devices.Scope, items []devices.Installation) ([]devices.Installation, error) {
	allowed := false
	op := access.ResourceOperation("facility", "list")
	if scope.Entry == "project" {
		op = access.ProjectResourceOperation("facility", "list")
	}
	var e error
	if scope.Entry == "project" {
		_, e = access.RequireProjectTx(ctx, tx, a.ID, op, scope.Tenant, scope.Project)
	} else {
		e = access.RequireScopedTx(ctx, tx, a.ID, op, scope.Tenant)
	}
	if e == nil {
		allowed = true
	} else {
		var ae *access.Error
		if !errors.As(e, &ae) || (ae.Status != 403 && ae.Status != 404) {
			return nil, e
		}
	}
	ids := []string{}
	targetID := func(i devices.Installation) string {
		switch i.TargetKind {
		case "facility":
			if i.FacilityID != nil {
				return *i.FacilityID
			}
		case "component":
			if i.ComponentID != nil {
				return *i.ComponentID
			}
		case "point":
			if i.PointID != nil {
				return *i.PointID
			}
		}
		return ""
	}
	for _, i := range items {
		if id := targetID(i); id != "" {
			ids = append(ids, id)
		}
	}
	targets := map[string]struct{ Name, Status string }{}
	if allowed && len(ids) > 0 {
		rows, e := tx.Query(ctx, `SELECT n.id::text,n.name,n.status FROM engineering_resource_nodes n WHERE n.tenant_id=$1 AND n.id=ANY($2::uuid[]) AND n.deleted_at IS NULL AND ($3::uuid IS NULL OR EXISTS(SELECT 1 FROM project_resource_grants g JOIN project_facility_links l ON l.id=g.link_id AND l.status='active' WHERE g.tenant_id=n.tenant_id AND g.project_id=$3 AND g.resource_kind=n.kind AND COALESCE(g.component_id,g.point_id,g.channel_id,g.facility_id)=n.id AND g.ended_at IS NULL))`, scope.Tenant, ids, k.Nullable(scope.Project))
		if e != nil {
			return nil, e
		}
		for rows.Next() {
			var id, name, status string
			if e = rows.Scan(&id, &name, &status); e != nil {
				rows.Close()
				return nil, e
			}
			targets[id] = struct{ Name, Status string }{name, status}
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return nil, e
		}
	}
	out := make([]devices.Installation, 0, len(items))
	for _, i := range items {
		i.Visibility = "full"
		if i.TargetKind != "location" {
			if v, ok := targets[targetID(i)]; ok {
				i.TargetName = v.Name
				i.TargetStatus = v.Status
			} else {
				i = devices.Installation{ID: i.ID, DeviceID: i.DeviceID, Visibility: "restricted", InstalledAt: i.InstalledAt, UninstalledAt: i.UninstalledAt, Revision: i.Revision, RecordedAt: i.RecordedAt, UpdatedAt: i.UpdatedAt}
			}
		}
		out = append(out, i)
	}
	return out, nil
}
func (r installationResources) InstallationTargetsTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope devices.Scope, f devices.Filter, history bool) (k.Page[devices.InstallationTarget], error) {
	out := k.Page[devices.InstallationTarget]{Items: []devices.InstallationTarget{}}
	if scope.Entry != "tenant" {
		return out, access.ErrDenied
	}
	if e := access.RequireScopedTx(ctx, tx, a.ID, access.ResourceOperation("facility", "list"), scope.Tenant); e != nil {
		return out, e
	}
	if e := f.Validate(); e != nil {
		return out, e
	}
	if f.Kind != "facility" && f.Kind != "component" && f.Kind != "point" {
		return out, k.Problem("invalid_request", "请选择设施、构件或测点。", 400)
	}
	auth, e := k.AuthorizeTx(ctx, tx, a, scope, scope.Operation("installation", "targets"))
	if e != nil {
		return out, e
	}
	cursor, e := k.DecodeCursor(f.Cursor, k.CursorContext(auth, f, []any{"installation-targets", history}))
	if e != nil {
		return out, e
	}
	rows, e := tx.Query(ctx, `SELECT n.kind,n.id::text,n.facility_id::text,n.name,n.code,n.status,n.created_at FROM engineering_resource_nodes n WHERE n.tenant_id=$1 AND n.kind=$2 AND n.deleted_at IS NULL AND ($3 OR NOT EXISTS(WITH RECURSIVE ancestors AS (SELECT * FROM engineering_resource_nodes root WHERE root.tenant_id=n.tenant_id AND root.kind=n.kind AND root.id=n.id UNION ALL SELECT p.* FROM engineering_resource_nodes p JOIN ancestors a ON p.tenant_id=a.tenant_id AND p.kind=a.parent_kind AND p.id=a.parent_id) SELECT 1 FROM ancestors WHERE status<>'normal' OR deleted_at IS NOT NULL)) AND ($4='' OR n.name ILIKE '%'||$4||'%' OR n.code ILIKE '%'||$4||'%') AND ($5::timestamptz IS NULL OR (n.created_at,n.id)>($5,$6::uuid)) ORDER BY n.created_at,n.id LIMIT $7`, scope.Tenant, f.Kind, history, f.Query, cursor.TimeValue(), k.Nullable(cursor.ID), f.Limit+1)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var v devices.InstallationTarget
		if e = rows.Scan(&v.Kind, &v.ID, &v.FacilityID, &v.Name, &v.Code, &v.Status, &v.CreatedAt); e != nil {
			return out, e
		}
		out.Items = append(out.Items, v)
	}
	if e = rows.Err(); e != nil {
		return out, e
	}
	if len(out.Items) > f.Limit {
		out.Items = out.Items[:f.Limit]
		last := out.Items[f.Limit-1]
		cursor.ID = last.ID
		cursor.At = last.CreatedAt
		out.NextCursor = cursor.Encode()
	}
	return out, nil
}

type deviceReferences struct{ devices *devices.Service }

func (r deviceReferences) ResourceImpactTx(ctx context.Context, tx pgx.Tx, scope resources.Scope, target resources.Target) (resources.ExternalImpact, error) {
	v, e := r.devices.ResourceImpactTx(ctx, tx, scope.Tenant, string(target.Kind), target.ID)
	return resources.ExternalImpact{Hash: v.Hash, Referenced: v.Referenced, CurrentInstallation: v.CurrentInstallation, CurrentSource: v.CurrentSource}, e
}
