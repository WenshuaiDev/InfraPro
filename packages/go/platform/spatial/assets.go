package spatial

import (
	"context"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
	"slices"
	"strings"
)

const visibleAsset = `($2::uuid IS NULL OR EXISTS(SELECT 1 FROM project_spatial_grants g JOIN spatial_asset_versions v ON v.id=g.version_id LEFT JOIN project_facility_links l ON l.id=g.facility_link_id WHERE g.tenant_id=a.tenant_id AND g.project_id=$2 AND g.asset_id=a.id AND g.ended_at IS NULL AND v.state='published' AND (a.facility_id IS NULL OR l.status='active')))`

func (s *Service) Assets(ctx context.Context, actor access.Actor, scope Scope, f Filter) (List[Asset], error) {
	out := List[Asset]{Items: []Asset{}}
	e := s.transact(ctx, actor, scope, "assets.list", false, func(tx pgx.Tx, auth k.Authorization) error {
		if e := normalizeFilter(&f); e != nil {
			return e
		}
		if f.Kind != "" && !slices.Contains([]string{"layout", "model", "geolayer"}, f.Kind) {
			return invalid("/kind", "资料类型无效。")
		}
		var grantRev string
		if e := tx.QueryRow(ctx, `SELECT COALESCE((SELECT revision::text FROM project_spatial_states WHERE tenant_id=$1 AND project_id=$2),'1')`, scope.Tenant, k.Nullable(scope.Project)).Scan(&grantRev); e != nil {
			return e
		}
		cursor, e := k.DecodeCursor(f.Cursor, cursorContext(auth, f, grantRev))
		if e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT `+assetJSON+` FROM spatial_assets a WHERE a.tenant_id=$1 AND `+visibleAsset+` AND ($3='' OR a.kind=$3) AND ($4::uuid IS NULL OR a.facility_id=$4) AND ($5='' OR a.name ILIKE '%'||$5||'%' OR a.code ILIKE '%'||$5||'%') AND ($6='' OR a.status=$6) AND ($7::timestamptz IS NULL OR (a.created_at,a.id)>($7,$8::uuid)) ORDER BY a.created_at,a.id LIMIT $9`, scope.Tenant, k.Nullable(scope.Project), f.Kind, k.Nullable(f.FacilityID), f.Query, f.State, cursor.TimeValue(), k.Nullable(cursor.ID), f.Limit+1)
		if e != nil {
			return e
		}
		for rows.Next() {
			var raw []byte
			var a Asset
			if e = rows.Scan(&raw); e != nil {
				rows.Close()
				return e
			}
			if e = json.Unmarshal(raw, &a); e != nil {
				rows.Close()
				return e
			}
			out.Items = append(out.Items, a)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if len(out.Items) > f.Limit {
			out.Items = out.Items[:f.Limit]
			last := out.Items[f.Limit-1]
			cursor.ID = last.ID
			cursor.At = last.CreatedAt
			out.NextCursor = cursor.Encode()
		}
		if scope.Project != "" {
			for i := range out.Items {
				out.Items[i].CurrentVersionID = ""
				if e := tx.QueryRow(ctx, `SELECT COALESCE((SELECT version_id::text FROM project_spatial_grants WHERE tenant_id=$1 AND project_id=$2 AND asset_id=$3 AND ended_at IS NULL AND is_default),'')`, scope.Tenant, scope.Project, out.Items[i].ID).Scan(&out.Items[i].CurrentVersionID); e != nil {
					return e
				}
			}
		}
		return nil
	})
	return out, e
}
func (s *Service) Asset(ctx context.Context, a access.Actor, scope Scope, id string) (Asset, error) {
	out := Asset{}
	e := s.transact(ctx, a, scope, "assets.detail", false, func(tx pgx.Tx, auth k.Authorization) error {
		if !access.ValidID(id) {
			return notFound()
		}
		if e := readJSON(ctx, tx, &out, `SELECT `+assetJSON+` FROM spatial_assets a WHERE a.tenant_id=$1 AND `+visibleAsset+` AND a.id=$3`, scope.Tenant, k.Nullable(scope.Project), id); e != nil {
			return e
		}
		if scope.Project != "" {
			return tx.QueryRow(ctx, `SELECT COALESCE((SELECT version_id::text FROM project_spatial_grants WHERE tenant_id=$1 AND project_id=$2 AND asset_id=$3 AND ended_at IS NULL AND is_default),'')`, scope.Tenant, scope.Project, id).Scan(&out.CurrentVersionID)
		}
		return nil
	})
	return out, e
}
func (s *Service) SaveAsset(ctx context.Context, a access.Actor, scope Scope, id string, in AssetInput) (Result, error) {
	out := Result{}
	action := "assets.create"
	if id != "" {
		action = "assets.update"
	}
	e := s.transact(ctx, a, scope, action, true, func(tx pgx.Tx, auth k.Authorization) error {
		if e := checkMeta(&in.Meta); e != nil {
			return e
		}
		if e := k.String(in.Name, "/name", 200, true); e != nil {
			return e
		}
		if e := k.String(in.SourceNote, "/source_note", 4000, false); e != nil {
			return e
		}
		if !k.Code(in.Code) || !slices.Contains([]string{"layout", "model", "geolayer"}, in.Kind) || !slices.Contains([]string{"general", "plan", "elevation", "section", "monitoring_section", "other"}, in.Category) {
			return invalid("/asset", "编号、类型或分类无效。")
		}
		var e error
		var replay bool
		input := commandInput(id, in)
		out, replay, e = receipt(ctx, tx, auth, in.Meta, action, input)
		if e != nil || replay {
			return e
		}
		if in.FacilityID != "" {
			if _, e = s.sameFacility(ctx, tx, a, scope, ObjectRef{"facility", in.FacilityID}, in.FacilityID, true); e != nil {
				return e
			}
		} else if in.Kind != "geolayer" {
			return invalid("/facility_id", "图纸和模型必须属于一个设施。")
		}
		if id == "" {
			e = tx.QueryRow(ctx, `INSERT INTO spatial_assets(tenant_id,facility_id,kind,code,name,category,source_note,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id::text,revision::text`, scope.Tenant, k.Nullable(in.FacilityID), in.Kind, in.Code, k.Text(in.Name), in.Category, k.Text(in.SourceNote), a.ID).Scan(&out.ID, &out.Revision)
		} else {
			asset, err := assetTx(ctx, tx, scope.Tenant, id)
			if err != nil {
				return err
			}
			if err = k.Expect(in.Revision, asset.Revision); err != nil {
				return err
			}
			if asset.Kind != in.Kind || asset.Code != in.Code || asset.FacilityID != in.FacilityID {
				return invalid("/asset", "资料身份、编号与设施归属固定，误建资料请另建。")
			}
			out.ID = id
			e = tx.QueryRow(ctx, `UPDATE spatial_assets SET name=$2,category=$3,source_note=$4,revision=revision+1 WHERE id=$1 RETURNING revision::text`, id, k.Text(in.Name), in.Category, k.Text(in.SourceNote)).Scan(&out.Revision)
		}
		if e != nil {
			return e
		}
		out.Changed = true
		return complete(ctx, tx, auth, in.Meta, action, input, out)
	})
	return out, e
}
func (s *Service) Versions(ctx context.Context, a access.Actor, scope Scope, id string, f Filter) (List[Version], error) {
	out := List[Version]{Items: []Version{}}
	e := s.transact(ctx, a, scope, "assets.versions", false, func(tx pgx.Tx, auth k.Authorization) error {
		if !access.ValidID(id) {
			return notFound()
		}
		if e := normalizeFilter(&f); e != nil {
			return e
		}
		asset, e := assetTx(ctx, tx, scope.Tenant, id)
		if e != nil {
			return e
		}
		cursor, e := k.DecodeCursor(f.Cursor, cursorContext(auth, f, asset.Revision))
		if e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT v.id::text FROM spatial_asset_versions v WHERE v.tenant_id=$1 AND v.asset_id=$2 AND v.deleted_at IS NULL AND ($3::uuid IS NULL OR (v.state='published' AND EXISTS(SELECT 1 FROM project_spatial_grants g LEFT JOIN project_facility_links l ON l.id=g.facility_link_id WHERE g.tenant_id=v.tenant_id AND g.version_id=v.id AND g.project_id=$3 AND g.ended_at IS NULL AND (g.facility_id IS NULL OR l.status='active')))) AND ($7='' OR v.state=$7) AND ($4::timestamptz IS NULL OR (v.created_at,v.id)>($4,$5::uuid)) ORDER BY v.created_at,v.id LIMIT $6`, scope.Tenant, id, k.Nullable(scope.Project), cursor.TimeValue(), k.Nullable(cursor.ID), f.Limit+1, f.State)
		if e != nil {
			return e
		}
		ids := []string{}
		for rows.Next() {
			var id string
			if e = rows.Scan(&id); e != nil {
				rows.Close()
				return e
			}
			ids = append(ids, id)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		for _, id := range ids {
			v, e := versionTx(ctx, tx, scope.Tenant, id)
			if e != nil {
				return e
			}
			v.Manifest = Manifest{}
			v.Candidates = []Candidate{}
			v.Settings = Settings{}
			out.Items = append(out.Items, v)
		}
		if len(out.Items) > f.Limit {
			out.Items = out.Items[:f.Limit]
			last := out.Items[f.Limit-1]
			cursor.ID = last.ID
			cursor.At = last.CreatedAt
			out.NextCursor = cursor.Encode()
		}
		return nil
	})
	return out, e
}
func (s *Service) CreateVersion(ctx context.Context, a access.Actor, scope Scope, id string, in VersionInput) (Result, error) {
	out := Result{}
	e := s.transact(ctx, a, scope, "versions.create", true, func(tx pgx.Tx, auth k.Authorization) error {
		if e := checkMeta(&in.Meta); e != nil {
			return e
		}
		input := commandInput(id, in)
		var e error
		var replay bool
		out, replay, e = receipt(ctx, tx, auth, in.Meta, "versions.create", input)
		if e != nil || replay {
			return e
		}
		asset, e := assetTx(ctx, tx, scope.Tenant, id)
		if e != nil {
			return e
		}
		if e = k.Expect(in.Revision, asset.Revision); e != nil {
			return e
		}
		if asset.Status != "normal" {
			return k.Problem("spatial_retired", "请先恢复资料。", 409)
		}
		if e = k.String(in.ChangeNote, "/change_note", 4000, false); e != nil {
			return e
		}
		base := Version{Settings: Settings{SourceProperties: true}, Candidates: []Candidate{}}
		if in.BaseVersionID != "" {
			base, e = versionTx(ctx, tx, scope.Tenant, in.BaseVersionID)
			if e != nil {
				return e
			}
			if base.AssetID != id || base.State != "published" {
				return invalid("/base_version_id", "只能复制同一资料的已发布版本。")
			}
		}
		source := base.SourceBlobID
		manifest := base.Manifest
		hash := base.ManifestHash
		format := base.Format
		if in.NewSource {
			source = ""
			manifest = Manifest{}
			hash = ""
			format = ""
		}
		e = tx.QueryRow(ctx, `INSERT INTO spatial_asset_versions(tenant_id,asset_id,number,base_version_id,source_blob_id,manifest,manifest_hash,format,settings,change_note,created_by) SELECT $1,$2,COALESCE(max(number),0)+1,$3,$4,$5,$6,$7,$8,$9,$10 FROM spatial_asset_versions WHERE asset_id=$2 RETURNING id::text,number,revision::text`, scope.Tenant, id, k.Nullable(in.BaseVersionID), k.Nullable(source), k.JSON(manifest), hash, format, k.JSON(base.Settings), k.Text(in.ChangeNote), a.ID).Scan(&out.ID, &out.Number, &out.Revision)
		if e != nil {
			return e
		}
		out.VersionID = out.ID
		if in.BaseVersionID != "" {
			if !in.NewSource {
				for _, q := range []string{
					`INSERT INTO spatial_version_files SELECT tenant_id,$2,file_id,blob_id FROM spatial_version_files WHERE version_id=$1`,
					`INSERT INTO spatial_point_placements(tenant_id,asset_id,facility_id,version_id,point_id,page_id,anchor,element_key,label_offset,main_channel_id,basis_hash,copied_from,target_kind,target_id,geometry) SELECT tenant_id,asset_id,facility_id,$2,point_id,page_id,anchor,element_key,label_offset,main_channel_id,basis_hash,id,target_kind,target_id,geometry FROM spatial_point_placements WHERE version_id=$1`,
					`INSERT INTO spatial_component_links(tenant_id,asset_id,facility_id,version_id,component_id,element_key,page_id,region,basis_hash,main_channel_id) SELECT tenant_id,asset_id,facility_id,$2,component_id,element_key,page_id,region,basis_hash,main_channel_id FROM spatial_component_links WHERE version_id=$1`,
					`INSERT INTO spatial_feature_links SELECT tenant_id,$2,feature_key,facility_id FROM spatial_feature_links WHERE version_id=$1`,
				} {
					if _, e = tx.Exec(ctx, q, base.ID, out.ID); e != nil {
						return e
					}
				}
			} else {
				_, e = tx.Exec(ctx, `UPDATE spatial_asset_versions SET candidates=COALESCE((SELECT jsonb_agg(jsonb_build_object('kind','point','state','needs_review','reason','来源已更换，处理完成后核对图页、元素身份和几何。','placement',to_jsonb(p)||CASE WHEN p.target_kind='point' THEN '{}'::jsonb ELSE jsonb_build_object('target_ref',jsonb_build_object('kind',p.target_kind,'id',p.target_id)) END)) FROM spatial_point_placements p WHERE version_id=$1),'[]')||COALESCE((SELECT jsonb_agg(jsonb_build_object('kind','component','state','needs_review','reason','来源已更换，关联须重新核对。','component_link',to_jsonb(c))) FROM spatial_component_links c WHERE version_id=$1),'[]') WHERE id=$2`, base.ID, out.ID)
				if e != nil {
					return e
				}
			}
		}
		if _, e = bumpAsset(ctx, tx, id); e != nil {
			return e
		}
		out.Changed = true
		return complete(ctx, tx, auth, in.Meta, "versions.create", input, out)
	})
	return out, e
}
func (s *Service) DeleteDraft(ctx context.Context, a access.Actor, scope Scope, id string, in RevisionInput) (Result, error) {
	out := Result{}
	temporary := []string{}
	e := s.transact(ctx, a, scope, "versions.delete", true, func(tx pgx.Tx, auth k.Authorization) error {
		if e := checkMeta(&in.Meta); e != nil {
			return e
		}
		input := commandInput(id, in)
		var e error
		var replay bool
		out, replay, e = receipt(ctx, tx, auth, in.Meta, "versions.delete", input)
		if e != nil || replay {
			return e
		}
		asset, v, e := s.readableTx(ctx, tx, auth, id, false)
		if e != nil {
			return e
		}
		if e = draftable(asset, v); e != nil {
			return e
		}
		if e = k.Expect(in.Revision, v.Revision); e != nil {
			return e
		}
		var used bool
		e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM spatial_processing_jobs WHERE version_id=$1 AND state IN ('queued','running')) OR EXISTS(SELECT 1 FROM spatial_uploads WHERE version_id=$1 AND state='uploading' AND expires_at>clock_timestamp()) OR EXISTS(SELECT 1 FROM spatial_feature_links WHERE version_id=$1) OR EXISTS(SELECT 1 FROM spatial_point_placements WHERE version_id=$1) OR EXISTS(SELECT 1 FROM spatial_component_links WHERE version_id=$1) OR EXISTS(SELECT 1 FROM spatial_asset_versions WHERE base_version_id=$1)`, id).Scan(&used)
		if e != nil {
			return e
		}
		if used {
			return k.Problem("spatial_in_use", "请先结束上传或处理，并解除挂载与关联；被其他版本引用的草稿不能删除。", 409)
		}
		// Preserve processing jobs, execution evidence and sealed source references.
		// A tombstone removes this unused draft from business reads without deleting
		// the immutable history needed by task, file and audit references.
		if _, e = tx.Exec(ctx, `INSERT INTO spatial_changes(tenant_id,actor_id,operation,target_id,reason,details) VALUES($1,$2,'versions.deleted',$3,$4,$5)`, scope.Tenant, a.ID, asset.ID, in.Reason, k.JSON(map[string]any{"version_id": id, "source_blob_id": v.SourceBlobID, "manifest_hash": v.ManifestHash, "number": v.Number})); e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT object_key FROM spatial_upload_parts WHERE upload_id IN(SELECT id FROM spatial_uploads WHERE version_id=$1)`, id)
		if e != nil {
			return e
		}
		for rows.Next() {
			var key string
			if e = rows.Scan(&key); e != nil {
				rows.Close()
				return e
			}
			temporary = append(temporary, key)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `DELETE FROM spatial_upload_parts WHERE upload_id IN(SELECT id FROM spatial_uploads WHERE version_id=$1)`, id); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE spatial_asset_versions SET deleted_at=clock_timestamp(),revision=revision+1 WHERE id=$1`, id); e != nil {
			return e
		}
		out = Result{ID: id, Revision: v.Revision, Changed: true}
		return complete(ctx, tx, auth, in.Meta, "versions.delete", input, out)
	})
	if e == nil && s.blobs != nil {
		for _, key := range temporary {
			_ = s.blobs.Delete(context.WithoutCancel(ctx), key)
		}
	}
	return out, e
}
func (s *Service) History(ctx context.Context, a access.Actor, scope Scope, id string, f Filter) (List[json.RawMessage], error) {
	out := List[json.RawMessage]{Items: []json.RawMessage{}}
	e := s.transact(ctx, a, scope, "assets.history", false, func(tx pgx.Tx, auth k.Authorization) error {
		if _, e := assetTx(ctx, tx, scope.Tenant, id); e != nil {
			return e
		}
		if e := normalizeFilter(&f); e != nil {
			return e
		}
		cursor, e := k.DecodeCursor(f.Cursor, cursorContext(auth, f, id))
		if e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT to_jsonb(c),c.id::text,c.created_at FROM spatial_changes c WHERE tenant_id=$1 AND (target_id=$2 OR target_id IN(SELECT id FROM spatial_asset_versions WHERE asset_id=$2)) AND ($3::timestamptz IS NULL OR (created_at,id)>($3,$4::uuid)) ORDER BY created_at,id LIMIT $5`, scope.Tenant, id, cursor.TimeValue(), k.Nullable(cursor.ID), f.Limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var raw json.RawMessage
			var next k.Cursor
			if e = rows.Scan(&raw, &next.ID, &next.At); e != nil {
				return e
			}
			if len(out.Items) == f.Limit {
				out.NextCursor = cursor.Encode()
				break
			}
			cursor.ID = next.ID
			cursor.At = next.At
			out.Items = append(out.Items, raw)
		}
		return rows.Err()
	})
	return out, e
}
func matched(text, query string) bool {
	return strings.Contains(strings.ToLower(text), strings.ToLower(query))
}
