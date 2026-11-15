package spatial

import (
	"context"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
)

func (s *Service) validateTx(ctx context.Context, tx pgx.Tx, a access.Actor, auth k.Authorization, asset Asset, v Version) (Draft, error) {
	out, e := draftTx(ctx, tx, asset, v)
	if e != nil {
		return out, e
	}
	if e = draftable(asset, v); e != nil {
		return out, e
	}
	if v.SourceBlobID == "" || v.ManifestHash == "" || v.Manifest.Contract != "infrapro.spatial/1" || v.ManifestHash != k.Hash(v.Manifest) || len(v.Manifest.Files) == 0 {
		return out, k.Problem("spatial_not_processed", "版本尚无完整且一致的封存处理结果。", 422)
	}
	var busy bool
	e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM spatial_processing_jobs WHERE version_id=$1 AND state IN ('queued','running')) OR EXISTS(SELECT 1 FROM spatial_uploads WHERE version_id=$1 AND state='uploading')`, v.ID).Scan(&busy)
	if e != nil {
		return out, e
	}
	if busy {
		return out, k.Problem("spatial_processing", "上传或处理仍在进行。", 409)
	}
	for _, f := range v.Manifest.Files {
		var valid bool
		e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM spatial_version_files f JOIN spatial_blobs b ON b.tenant_id=f.tenant_id AND b.id=f.blob_id WHERE f.tenant_id=$1 AND f.version_id=$2 AND f.file_id=$3 AND b.sha256=$4 AND b.size=$5)`, auth.Tenant, v.ID, f.ID, f.SHA256, f.Size).Scan(&valid)
		if e != nil {
			return out, e
		}
		if !valid {
			return out, k.Problem("spatial_manifest_invalid", "封存清单与文件记录不一致，不能发布。", 422)
		}
	}
	if asset.FacilityID != "" {
		if _, e = s.sameFacility(ctx, tx, a, auth.Scope, ObjectRef{"facility", asset.FacilityID}, asset.FacilityID, true); e != nil {
			return out, e
		}
	}
	for _, p := range out.Placements {
		if e = validatePlacement(asset, v, p); e != nil {
			return out, e
		}
		if _, e = s.sameFacility(ctx, tx, a, auth.Scope, ObjectRef{"point", p.PointID}, asset.FacilityID, false); e != nil {
			return out, e
		}
		if e = s.channelTx(ctx, tx, a, auth.Scope, p.MainChannelID, ObjectRef{"point", p.PointID}, asset.FacilityID); e != nil {
			return out, e
		}
	}
	for _, l := range out.ComponentLinks {
		if e = validateLink(asset, v, l); e != nil {
			return out, e
		}
		if _, e = s.sameFacility(ctx, tx, a, auth.Scope, ObjectRef{"component", l.ComponentID}, asset.FacilityID, false); e != nil {
			return out, e
		}
		if e = s.channelTx(ctx, tx, a, auth.Scope, l.MainChannelID, ObjectRef{"component", l.ComponentID}, asset.FacilityID); e != nil {
			return out, e
		}
	}
	for _, l := range out.FeatureLinks {
		if _, e = s.sameFacility(ctx, tx, a, auth.Scope, ObjectRef{"facility", l.FacilityID}, l.FacilityID, false); e != nil {
			return out, e
		}
	}
	if e = s.settingsTx(ctx, tx, a, auth.Scope, asset, v, v.Settings); e != nil {
		return out, e
	}
	e = s.projectDraftTx(ctx, tx, a, auth, &out)
	return out, e
}
func (s *Service) Validate(ctx context.Context, a access.Actor, scope Scope, id string, in RevisionInput) (Draft, error) {
	out := Draft{}
	e := s.transact(ctx, a, scope, "versions.validate", false, func(tx pgx.Tx, auth k.Authorization) error {
		asset, v, e := s.readableTx(ctx, tx, auth, id, false)
		if e != nil {
			return e
		}
		if e = k.Expect(in.Revision, v.Revision); e != nil {
			return e
		}
		out, e = s.validateTx(ctx, tx, a, auth, asset, v)
		return e
	})
	return out, e
}
func (s *Service) controlTx(ctx context.Context, tx pgx.Tx, a access.Actor, auth k.Authorization, id, action string, in ControlInput) (Asset, Version, Preview, error) {
	asset := Asset{}
	v := Version{}
	var e error
	if action == "versions.publish" || action == "versions.withdraw" {
		v, e = versionTx(ctx, tx, auth.Tenant, id)
		if e == nil {
			asset, e = assetTx(ctx, tx, auth.Tenant, v.AssetID)
		}
		if e == nil {
			e = k.Expect(in.Revision, v.Revision)
		}
	} else {
		asset, e = assetTx(ctx, tx, auth.Tenant, id)
		if e == nil {
			e = k.Expect(in.Revision, asset.Revision)
		}
	}
	if e != nil {
		return asset, v, Preview{}, e
	}
	var detail any
	switch action {
	case "versions.publish":
		detail, e = s.validateTx(ctx, tx, a, auth, asset, v)
	case "versions.withdraw":
		if v.State != "published" {
			e = k.Problem("spatial_state", "只能撤回已发布版本。", 409)
		}
		if asset.CurrentVersionID != id && in.ReplacementVersionID != "" {
			e = invalid("/replacement_version_id", "该版本不是当前版，无须指定替代。")
		}
		if in.ReplacementVersionID != "" {
			var replacement Version
			replacement, e = versionTx(ctx, tx, auth.Tenant, in.ReplacementVersionID)
			if e == nil && (replacement.AssetID != asset.ID || replacement.ID == id || replacement.State != "published" || !replacement.ContentAvailable) {
				e = invalid("/replacement_version_id", "替代版本必须为同资料的另一可用已发布版本。")
			}
			detail = replacement
		}
	case "assets.current":
		v, e = versionTx(ctx, tx, auth.Tenant, in.VersionID)
		if e == nil && (v.AssetID != asset.ID || v.State != "published" || !v.ContentAvailable) {
			e = invalid("/version_id", "请选择同资料可用的已发布版本。")
		}
	case "assets.retire":
		if asset.Status != "normal" {
			e = k.Problem("spatial_state", "资料已经退役。", 409)
		}
	case "assets.restore":
		if asset.Status != "retired" {
			e = k.Problem("spatial_state", "资料无需恢复。", 409)
		}
	default:
		e = invalid("/action", "不支持该状态操作。")
	}
	if e != nil {
		return asset, v, Preview{}, e
	}
	var grants any
	e = tx.QueryRow(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(g) ORDER BY g.id),'[]') FROM project_spatial_grants g WHERE tenant_id=$1 AND asset_id=$2 AND ended_at IS NULL`, auth.Tenant, asset.ID).Scan(&grants)
	if e != nil {
		return asset, v, Preview{}, e
	}
	in.PreviewHash = ""
	state := []any{auth.Account, auth.Scope, auth.Revision, auth.TenantMembership, asset, v, detail, grants}
	changes := map[string]any{"operation": action, "asset_id": asset.ID, "version_id": v.ID, "source_content": "查看授权包括完整图面、几何和已发布的源属性；过滤业务标记不会隐藏背景内容。", "project_versions_unchanged": action != "versions.withdraw", "unadopted_candidates": len(v.Candidates), "replacement_version_id": in.ReplacementVersionID}
	p := k.NewPreview(commandInput(id, in), state, changes, action != "assets.restore", false)
	return asset, v, p, nil
}
func (s *Service) Preview(ctx context.Context, a access.Actor, scope Scope, id, action string, in ControlInput) (Preview, error) {
	out := Preview{}
	e := s.transact(ctx, a, scope, action+"-preview", false, func(tx pgx.Tx, auth k.Authorization) error {
		if e := checkMeta(&in.Meta); e != nil {
			return e
		}
		_, _, p, e := s.controlTx(ctx, tx, a, auth, id, action, in)
		out = p
		return e
	})
	return out, e
}
func (s *Service) Control(ctx context.Context, a access.Actor, scope Scope, id, action string, in ControlInput) (Result, error) {
	out := Result{}
	e := s.transact(ctx, a, scope, action, true, func(tx pgx.Tx, auth k.Authorization) error {
		if e := checkMeta(&in.Meta); e != nil {
			return e
		}
		input := commandInput(id, in)
		var e error
		var replay bool
		out, replay, e = receipt(ctx, tx, auth, in.Meta, action, input)
		if e != nil || replay {
			return e
		}
		asset, v, p, e := s.controlTx(ctx, tx, a, auth, id, action, in)
		if e != nil {
			return e
		}
		if e = p.Check(in.Meta); e != nil {
			return e
		}
		notificationProjects, e := publicationProjectsTx(ctx, tx, scope.Tenant, asset.ID, v.ID)
		if e != nil {
			return e
		}
		out.ID = id
		switch action {
		case "versions.publish":
			e = tx.QueryRow(ctx, `UPDATE spatial_asset_versions SET state='published',published_at=clock_timestamp(),revision=revision+1 WHERE id=$1 RETURNING revision::text`, id).Scan(&out.Revision)
			if e == nil {
				_, e = tx.Exec(ctx, `UPDATE spatial_assets SET current_version_id=$2,revision=revision+1 WHERE id=$1`, asset.ID, id)
			}
			out.VersionID = id
		case "versions.withdraw":
			if asset.CurrentVersionID == id {
				_, e = tx.Exec(ctx, `UPDATE spatial_assets SET current_version_id=$2,revision=revision+1 WHERE id=$1`, asset.ID, k.Nullable(in.ReplacementVersionID))
				if e != nil {
					return e
				}
			}
			_, e = tx.Exec(ctx, `INSERT INTO project_spatial_states(tenant_id,project_id) SELECT DISTINCT tenant_id,project_id FROM project_spatial_grants WHERE version_id=$1 AND ended_at IS NULL ON CONFLICT(tenant_id,project_id) DO UPDATE SET revision=project_spatial_states.revision+1`, id)
			if e != nil {
				return e
			}
			_, e = tx.Exec(ctx, `UPDATE project_spatial_grants SET ended_at=clock_timestamp(),is_default=false,revision=revision+1 WHERE version_id=$1 AND ended_at IS NULL`, id)
			if e != nil {
				return e
			}
			e = tx.QueryRow(ctx, `UPDATE spatial_asset_versions SET state='withdrawn',revision=revision+1 WHERE id=$1 RETURNING revision::text`, id).Scan(&out.Revision)
		case "assets.current":
			e = tx.QueryRow(ctx, `UPDATE spatial_assets SET current_version_id=$2,revision=revision+1 WHERE id=$1 RETURNING revision::text`, id, v.ID).Scan(&out.Revision)
			out.VersionID = v.ID
		case "assets.retire", "assets.restore":
			state := "retired"
			if action == "assets.restore" {
				state = "normal"
			}
			e = tx.QueryRow(ctx, `UPDATE spatial_assets SET status=$2,revision=revision+1 WHERE id=$1 RETURNING revision::text`, id, state).Scan(&out.Revision)
		}
		if e != nil {
			return e
		}
		out.Changed = true
		if e = complete(ctx, tx, auth, in.Meta, action, input, out); e != nil {
			return e
		}
		return publicationNotificationTx(ctx, tx, scope.Tenant, a.ID, v.ID, action, notificationProjects)
	})
	return out, e
}
func (s *Service) integrityError(id string) error {
	return k.Problem("spatial_content_unavailable", fmt.Sprintf("固定版本 %s 的文件校验失败。", id), 503)
}
