package spatial

import (
	"context"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
)

func projectStateTx(ctx context.Context, tx pgx.Tx, tenant, project string) (string, string, error) {
	if !access.ValidID(project) {
		return "", "", notFound()
	}
	var status, revision string
	e := tx.QueryRow(ctx, `SELECT p.status,COALESCE(s.revision,1)::text FROM projects p LEFT JOIN project_spatial_states s ON s.tenant_id=p.tenant_id AND s.project_id=p.id WHERE p.tenant_id=$1 AND p.id=$2`, tenant, project).Scan(&status, &revision)
	if e == pgx.ErrNoRows {
		e = notFound()
	}
	return status, revision, e
}
func accessTx(ctx context.Context, tx pgx.Tx, tenant, project string) (AccessDetail, error) {
	out := AccessDetail{Grants: []Grant{}}
	_, rev, e := projectStateTx(ctx, tx, tenant, project)
	out.Revision = rev
	if e != nil {
		return out, e
	}
	out.Grants, e = rowsJSON[Grant](ctx, tx, `SELECT to_jsonb(g)||jsonb_build_object('revision',g.revision::text,'facility_link_id',COALESCE(g.facility_link_id::text,'')) FROM project_spatial_grants g WHERE tenant_id=$1 AND project_id=$2 ORDER BY created_at,id`, tenant, project)
	return out, e
}
func (s *Service) Access(ctx context.Context, a access.Actor, scope Scope, project string) (AccessDetail, error) {
	out := AccessDetail{}
	e := s.transact(ctx, a, scope, "access.detail", false, func(tx pgx.Tx, auth k.Authorization) error {
		var e error
		out, e = accessTx(ctx, tx, scope.Tenant, project)
		return e
	})
	return out, e
}

type grantPlan struct {
	Version              Version
	Asset                Asset
	LinkID, LinkRevision string
	Grant                GrantChange
}

func (s *Service) accessPreviewTx(ctx context.Context, tx pgx.Tx, a access.Actor, auth k.Authorization, project string, in AccessInput) (Preview, []grantPlan, error) {
	out := Preview{}
	plans := []grantPlan{}
	if len(in.Add)+len(in.End) > 1000 {
		return out, plans, invalid("/changes", "单次最多提交 1000 个授权变更。")
	}
	status, _, e := projectStateTx(ctx, tx, auth.Tenant, project)
	if e != nil {
		return out, plans, e
	}
	before, e := accessTx(ctx, tx, auth.Tenant, project)
	if e != nil {
		return out, plans, e
	}
	if e = k.Expect(in.Revision, before.Revision); e != nil {
		return out, plans, e
	}
	if status == "archived" && len(in.Add) > 0 {
		return out, plans, k.Problem("project_archived", "项目已归档，只能结束授权。", 409)
	}
	seen := map[string]bool{}
	for _, id := range in.End {
		if !access.ValidID(id) || seen[id] {
			return out, plans, invalid("/end", "授权 ID 无效或重复。")
		}
		seen[id] = true
		found := false
		for _, g := range before.Grants {
			if g.ID == id && g.EndedAt == nil {
				found = true
			}
		}
		if !found {
			return out, plans, notFound()
		}
	}
	versions := map[string]bool{}
	defaults := map[string]bool{}
	for _, g := range in.Add {
		if versions[g.VersionID] {
			return out, plans, invalid("/add", "不能重复选择版本。")
		}
		versions[g.VersionID] = true
		v, e := versionTx(ctx, tx, auth.Tenant, g.VersionID)
		if e != nil {
			return out, plans, e
		}
		asset, e := assetTx(ctx, tx, auth.Tenant, v.AssetID)
		if e != nil {
			return out, plans, e
		}
		if v.State != "published" || !v.ContentAvailable || asset.Status != "normal" {
			return out, plans, invalid("/add", "只能新增正常资料的可用已发布版本授权。")
		}
		plan := grantPlan{Version: v, Asset: asset, Grant: g}
		if asset.FacilityID != "" {
			e = tx.QueryRow(ctx, `SELECT id::text,revision::text FROM project_facility_links WHERE tenant_id=$1 AND project_id=$2 AND facility_id=$3 AND status='active'`, auth.Tenant, project, asset.FacilityID).Scan(&plan.LinkID, &plan.LinkRevision)
			if e == pgx.ErrNoRows {
				return out, plans, k.Problem("facility_link_required", "项目尚未关联此设施，请先在资源授权中明确关联。", 422)
			}
			if e != nil {
				return out, plans, e
			}
		}
		if g.Default {
			if defaults[asset.ID] {
				return out, plans, invalid("/add", "同一资料只能选一个项目默认版本。")
			}
			defaults[asset.ID] = true
		}
		plans = append(plans, plan)
	}
	in.PreviewHash = ""
	out = k.NewPreview([]any{project, in}, []any{auth.Account, auth.Scope, auth.Revision, before, status, plans}, map[string]any{"add": in.Add, "end": in.End, "view_scope": "允许查看整份图面、几何及发布的源属性；业务标记和监测另行授权。", "source_scope": "原 IFC／PDF 可能包含浏览属性之外的来源内容。", "business_access": "此次操作不新增工程资源、设备输出或监测时段权限。"}, true, false)
	return out, plans, nil
}
func (s *Service) PreviewAccess(ctx context.Context, a access.Actor, scope Scope, project string, in AccessInput) (Preview, error) {
	out := Preview{}
	e := s.transact(ctx, a, scope, "access.preview", false, func(tx pgx.Tx, auth k.Authorization) error {
		if e := checkMeta(&in.Meta); e != nil {
			return e
		}
		var e error
		out, _, e = s.accessPreviewTx(ctx, tx, a, auth, project, in)
		return e
	})
	return out, e
}
func (s *Service) SaveAccess(ctx context.Context, a access.Actor, scope Scope, project string, in AccessInput) (Result, error) {
	out := Result{}
	e := s.transact(ctx, a, scope, "access.save", true, func(tx pgx.Tx, auth k.Authorization) error {
		if e := checkMeta(&in.Meta); e != nil {
			return e
		}
		input := commandInput(project, in)
		var e error
		var replay bool
		out, replay, e = receipt(ctx, tx, auth, in.Meta, "access.save", input)
		if e != nil || replay {
			return e
		}
		p, plans, e := s.accessPreviewTx(ctx, tx, a, auth, project, in)
		if e != nil {
			return e
		}
		if e = p.Check(in.Meta); e != nil {
			return e
		}
		for _, id := range in.End {
			if _, e = tx.Exec(ctx, `UPDATE project_spatial_grants SET ended_at=clock_timestamp(),is_default=false,revision=revision+1 WHERE id=$1 AND tenant_id=$2 AND project_id=$3 AND ended_at IS NULL`, id, scope.Tenant, project); e != nil {
				return e
			}
		}
		for _, plan := range plans {
			if plan.Grant.Default {
				if _, e = tx.Exec(ctx, `UPDATE project_spatial_grants SET is_default=false,revision=revision+1 WHERE tenant_id=$1 AND project_id=$2 AND asset_id=$3 AND ended_at IS NULL AND is_default`, scope.Tenant, project, plan.Asset.ID); e != nil {
					return e
				}
			}
			_, e = tx.Exec(ctx, `INSERT INTO project_spatial_grants(tenant_id,project_id,asset_id,version_id,facility_id,facility_link_id,source_download,is_default,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT(tenant_id,project_id,version_id) WHERE ended_at IS NULL DO UPDATE SET source_download=EXCLUDED.source_download,is_default=EXCLUDED.is_default,revision=project_spatial_grants.revision+1`, scope.Tenant, project, plan.Asset.ID, plan.Version.ID, k.Nullable(plan.Asset.FacilityID), k.Nullable(plan.LinkID), plan.Grant.SourceDownload, plan.Grant.Default, a.ID)
			if e != nil {
				return e
			}
		}
		out.ID = project
		e = tx.QueryRow(ctx, `INSERT INTO project_spatial_states(tenant_id,project_id,revision) VALUES($1,$2,2) ON CONFLICT(tenant_id,project_id) DO UPDATE SET revision=project_spatial_states.revision+1 RETURNING revision::text`, scope.Tenant, project).Scan(&out.Revision)
		if e != nil {
			return e
		}
		out.Changed = true
		// The global authorization revision invalidates in-flight plans and browser caches.
		if e = access.Bump(ctx, tx); e != nil {
			return e
		}
		if e = complete(ctx, tx, auth, in.Meta, "access.save", input, out); e != nil {
			return e
		}
		return access.NotifyProjectGrantTx(ctx, tx, "spatial", scope.Tenant, project, a.ID)
	})
	return out, e
}
func (s *Service) View(ctx context.Context, a access.Actor, scope Scope, id string) (View, error) {
	out := View{}
	e := s.transact(ctx, a, scope, "views.detail", false, func(tx pgx.Tx, auth k.Authorization) error {
		if !access.ValidID(id) {
			return notFound()
		}
		var raw []byte
		e := tx.QueryRow(ctx, `SELECT config||jsonb_build_object('revision',revision::text) FROM project_spatial_views WHERE tenant_id=$1 AND project_id=$2 AND id=$3`, scope.Tenant, scope.Project, id).Scan(&raw)
		if e == pgx.ErrNoRows {
			// A fixed version ID is also the stable key for its project default.
			// Before the first save, inherit only the readable version settings.
			_, v, err := s.readableTx(ctx, tx, auth, id, false)
			if err != nil {
				return err
			}
			out = View{Revision: "0", VersionID: id, Settings: v.Settings}
		} else if e != nil {
			return e
		} else if e = json.Unmarshal(raw, &out); e != nil {
			return e
		}
		asset, v, e := s.readableTx(ctx, tx, auth, out.VersionID, false)
		if e != nil {
			return e
		}
		d := Draft{Asset: asset, Version: v}
		d.Version.Settings = out.Settings
		if e = s.projectDraftTx(ctx, tx, a, auth, &d); e != nil {
			return e
		}
		out.Settings = d.Version.Settings
		return nil
	})
	return out, e
}
func (s *Service) SaveView(ctx context.Context, a access.Actor, scope Scope, id string, in ViewInput) (Result, error) {
	out := Result{}
	e := s.transact(ctx, a, scope, "views.save", true, func(tx pgx.Tx, auth k.Authorization) error {
		if !access.ValidID(id) {
			return notFound()
		}
		if e := checkMeta(&in.Meta); e != nil {
			return e
		}
		input := commandInput(id, in)
		var e error
		var replay bool
		out, replay, e = receipt(ctx, tx, auth, in.Meta, "views.save", input)
		if e != nil || replay {
			return e
		}
		asset, v, e := s.readableTx(ctx, tx, auth, in.VersionID, false)
		if e != nil {
			return e
		}
		if e = s.settingsTx(ctx, tx, a, scope, asset, v, in.Settings); e != nil {
			return e
		}
		var rev string
		if e = tx.QueryRow(ctx, `SELECT COALESCE((SELECT revision::text FROM project_spatial_views WHERE tenant_id=$1 AND project_id=$2 AND id=$3),'0')`, scope.Tenant, scope.Project, id).Scan(&rev); e != nil {
			return e
		}
		if e = k.Expect(in.Revision, rev); e != nil {
			return e
		}
		out.ID = id
		e = tx.QueryRow(ctx, `INSERT INTO project_spatial_views(tenant_id,project_id,id,config,updated_by) VALUES($1,$2,$3,$4,$5) ON CONFLICT(tenant_id,project_id,id) DO UPDATE SET config=EXCLUDED.config,updated_by=EXCLUDED.updated_by,revision=project_spatial_views.revision+1 RETURNING revision::text`, scope.Tenant, scope.Project, id, k.JSON(View{VersionID: in.VersionID, Settings: in.Settings}), a.ID).Scan(&out.Revision)
		if e != nil {
			return e
		}
		out.Changed = true
		return complete(ctx, tx, auth, in.Meta, "views.save", input, out)
	})
	return out, e
}
