package spatial

import (
	"context"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"slices"
)

func rowsJSON[T any](ctx context.Context, tx pgx.Tx, query string, args ...any) ([]T, error) {
	out := []T{}
	rows, e := tx.Query(ctx, query, args...)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		var v T
		if e = rows.Scan(&raw); e != nil {
			return out, e
		}
		if e = json.Unmarshal(raw, &v); e != nil {
			return out, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func draftTx(ctx context.Context, tx pgx.Tx, asset Asset, v Version) (Draft, error) {
	out := Draft{Asset: asset, Version: v}
	var e error
	out.Placements, e = rowsJSON[Placement](ctx, tx, `SELECT to_jsonb(p)||CASE WHEN target_kind='point' THEN '{}'::jsonb ELSE jsonb_build_object('target_ref',jsonb_build_object('kind',target_kind,'id',target_id)) END FROM spatial_point_placements p WHERE version_id=$1 ORDER BY id`, v.ID)
	if e != nil {
		return out, e
	}
	out.ComponentLinks, e = rowsJSON[ComponentLink](ctx, tx, `SELECT to_jsonb(c) FROM spatial_component_links c WHERE version_id=$1 ORDER BY id`, v.ID)
	if e != nil {
		return out, e
	}
	out.FeatureLinks, e = rowsJSON[FeatureLink](ctx, tx, `SELECT to_jsonb(f) FROM spatial_feature_links f WHERE version_id=$1 ORDER BY feature_key`, v.ID)
	return out, e
}
func (s *Service) Draft(ctx context.Context, a access.Actor, scope Scope, id string) (Draft, error) {
	out := Draft{}
	e := s.transact(ctx, a, scope, "versions.draft", false, func(tx pgx.Tx, auth k.Authorization) error {
		asset, v, e := s.readableTx(ctx, tx, auth, id, false)
		if e != nil {
			return e
		}
		out, e = draftTx(ctx, tx, asset, v)
		if e != nil {
			return e
		}
		return s.projectDraftTx(ctx, tx, a, auth, &out)
	})
	return out, e
}
func hasPage(v Version, id string) bool {
	for _, p := range v.Manifest.Pages {
		if p.ID == id {
			return true
		}
	}
	return false
}
func hasElement(v Version, id string) bool {
	if id == "" {
		return true
	}
	for _, e := range v.Manifest.Elements {
		if e.Key == id && len(e.Fragments) > 0 {
			return true
		}
	}
	return false
}
func placementRef(p Placement) ObjectRef {
	if p.TargetRef != nil {
		return *p.TargetRef
	}
	return ObjectRef{"point", p.PointID}
}
func validatePlacement(a Asset, v Version, p Placement) error {
	ref := placementRef(p)
	if !slices.Contains([]string{"point", "monitoring_section", "borehole"}, ref.Kind) || p.TargetRef != nil && (ref.Kind == "point" && p.PointID != "" && p.PointID != ref.ID || ref.Kind != "point" && p.PointID != "") {
		return invalid("/placements/target_ref", "挂载目标须为测点、断面或孔位，不能同时指定冲突身份。")
	}

	if !access.ValidID(ref.ID) || p.ID != "" && !access.ValidID(p.ID) {
		return invalid("/placements", "测点或挂载 ID 无效。")
	}
	if !vector(p.LabelOffset, 2, false) || slices.ContainsFunc(p.LabelOffset, func(x float64) bool { return x < -2000 || x > 2000 }) {
		return invalid("/label_offset", "标签偏移须为两个 -2000–2000 的有限像素值。")
	}
	if a.Kind == "layout" {
		if !hasPage(v, p.PageID) || !vector(p.Anchor, 2, true) || p.ElementKey != "" {
			return invalid("/placements", "图纸位置必须引用本版图页，坐标位于 [0,1]。")
		}
	} else if a.Kind == "model" {
		if p.PageID != "" || !vector(p.Anchor, 3, false) || !hasElement(v, p.ElementKey) {
			return invalid("/placements", "模型位置须为三个有限本地坐标，元素必须属于本版本。")
		}
	} else {
		return invalid("/placements", "地图位置来自工程资源，不能拖动或另建覆盖坐标。")
	}
	return validateAssociationGeometry(a, p)
}
func validateLink(a Asset, v Version, l ComponentLink) error {
	if !access.ValidID(l.ComponentID) || l.ID != "" && !access.ValidID(l.ID) {
		return invalid("/component_links", "构件或关联 ID 无效。")
	}
	if a.Kind == "model" {
		if l.ElementKey == "" || !hasElement(v, l.ElementKey) || l.PageID != "" || len(l.Region) > 0 {
			return invalid("/component_links", "请选择当前模型的明确元素实例。")
		}
	} else if a.Kind == "layout" {
		if l.ElementKey != "" || !hasPage(v, l.PageID) || len(l.Region) < 3 || len(l.Region) > 64 {
			return invalid("/component_links", "图纸区域须在本版图页中，含 3–64 个顶点。")
		}
		for _, p := range l.Region {
			if !vector(p, 2, true) {
				return invalid("/region", "区域坐标须位于 [0,1]。")
			}
		}
		area := 0.
		for i, p := range l.Region {
			q := l.Region[(i+1)%len(l.Region)]
			area += p[0]*q[1] - q[0]*p[1]
		}
		if area == 0 {
			return invalid("/region", "区域不能退化为一条线。")
		}
	} else {
		return invalid("/component_links", "地理图层使用设施关联。")
	}
	return nil
}
func (s *Service) channelTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, id string, owner ObjectRef, facility string) error {
	if id == "" {
		return nil
	}
	c, e := s.sameFacility(ctx, tx, a, scope, ObjectRef{"channel", id}, facility, false)
	if e != nil {
		return e
	}
	if c.OwnerKind != owner.Kind || c.OwnerID != owner.ID {
		return invalid("/main_channel_id", "主展示通道必须直接属于当前对象。")
	}
	return nil
}
func (s *Service) settingsTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, asset Asset, v Version, in Settings) error {
	if in.PageID != "" && !hasPage(v, in.PageID) {
		return invalid("/settings/page_id", "图页不属于当前版本。")
	}
	if in.Camera != nil && (!vector(in.Camera.Position, 3, false) || !vector(in.Camera.Target, 3, false)) {
		return invalid("/settings/camera", "视角坐标无效。")
	}
	if !s.knownBasemap(in.Basemap) {
		return invalid("/settings/basemap", "底图必须来自受控目录。")
	}
	if len(in.HiddenElements) > 1000 || len(in.MainChannels) > 1000 {
		return invalid("/settings", "配置超出单次 1000 个对象边界。")
	}
	for _, key := range in.HiddenElements {
		if !hasElement(v, key) {
			return invalid("/settings/hidden_elements", "元素不属于当前版本。")
		}
	}
	for owner, id := range in.MainChannels {
		parts := splitOwner(owner)
		if parts.ID == "" {
			return invalid("/settings/main_channels", "对象键无效。")
		}
		if _, e := s.sameFacility(ctx, tx, a, scope, parts, asset.FacilityID, false); e != nil {
			return e
		}
		if e := s.channelTx(ctx, tx, a, scope, id, parts, asset.FacilityID); e != nil {
			return e
		}
	}
	return nil
}
func splitOwner(key string) ObjectRef {
	for _, kind := range []string{"point", "component", "monitoring_section", "borehole", "facility"} {
		prefix := kind + ":"
		if len(key) > len(prefix) && key[:len(prefix)] == prefix && access.ValidID(key[len(prefix):]) {
			return ObjectRef{kind, key[len(prefix):]}
		}
	}
	return ObjectRef{}
}
func (s *Service) EditDraft(ctx context.Context, a access.Actor, scope Scope, id string, in DraftInput) (Result, error) {
	out := Result{}
	e := s.transact(ctx, a, scope, "versions.edit", true, func(tx pgx.Tx, auth k.Authorization) error {
		if e := checkMeta(&in.Meta); e != nil {
			return e
		}
		input := commandInput(id, in)
		var e error
		var replay bool
		out, replay, e = receipt(ctx, tx, auth, in.Meta, "versions.edit", input)
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
		if v.ManifestHash == "" {
			return k.Problem("spatial_not_processed", "来源文件尚未处理成功。", 409)
		}
		if len(in.Placements)+len(in.ComponentLinks)+len(in.FeatureLinks)+len(in.RemovePlacements)+len(in.RemoveComponentLinks)+len(in.RemoveFeatureLinks) > 1000 {
			return invalid("/changes", "单次最多修改 1000 个关系。")
		}
		readSameFacility := s.sameFacilityReaderTx(ctx, tx, a, scope)
		for table, ids := range map[string][]string{"spatial_point_placements": in.RemovePlacements, "spatial_component_links": in.RemoveComponentLinks} {
			for _, rid := range ids {
				if !access.ValidID(rid) {
					return invalid("/remove", "关系 ID 无效。")
				}
				tag, e := tx.Exec(ctx, `DELETE FROM `+table+` WHERE tenant_id=$1 AND version_id=$2 AND id=$3`, scope.Tenant, id, rid)
				if e != nil {
					return e
				}
				if tag.RowsAffected() != 1 {
					return notFound()
				}
			}
		}
		for _, key := range in.RemoveFeatureLinks {
			if _, e = tx.Exec(ctx, `DELETE FROM spatial_feature_links WHERE tenant_id=$1 AND version_id=$2 AND feature_key=$3`, scope.Tenant, id, key); e != nil {
				return e
			}
		}
		for _, p := range in.Placements {
			if e = validatePlacement(asset, v, p); e != nil {
				return e
			}
			newUse := p.ID == ""
			o, e := readSameFacility(placementRef(p), asset.FacilityID, newUse)
			if e != nil {
				return e
			}
			if e = s.channelTx(ctx, tx, a, scope, p.MainChannelID, placementRef(p), asset.FacilityID); e != nil {
				return e
			}
			if newUse {
				p.ID = uuid.NewString()
				_, e = tx.Exec(ctx, `INSERT INTO spatial_point_placements(id,tenant_id,asset_id,facility_id,version_id,point_id,page_id,anchor,element_key,label_offset,main_channel_id,basis_hash,target_kind,target_id,geometry) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,NULLIF($15::jsonb,'null'::jsonb))`, p.ID, scope.Tenant, asset.ID, asset.FacilityID, id, k.Nullable(p.PointID), p.PageID, k.JSON(p.Anchor), p.ElementKey, k.JSON(p.LabelOffset), k.Nullable(p.MainChannelID), o.BasisHash, placementRef(p).Kind, placementRef(p).ID, k.JSON(p.Geometry))
			} else {
				tag, err := tx.Exec(ctx, `UPDATE spatial_point_placements SET page_id=$4,anchor=$5,element_key=$6,label_offset=$7,main_channel_id=$8,basis_hash=$9,geometry=NULLIF($12::jsonb,'null'::jsonb) WHERE tenant_id=$1 AND version_id=$2 AND id=$3 AND target_kind=$10 AND target_id=$11`, scope.Tenant, id, p.ID, p.PageID, k.JSON(p.Anchor), p.ElementKey, k.JSON(p.LabelOffset), k.Nullable(p.MainChannelID), o.BasisHash, placementRef(p).Kind, placementRef(p).ID, k.JSON(p.Geometry))
				e = err
				if e == nil && tag.RowsAffected() != 1 {
					return notFound()
				}
			}
			if e != nil {
				return e
			}
		}
		for _, l := range in.ComponentLinks {
			if e = validateLink(asset, v, l); e != nil {
				return e
			}
			newUse := l.ID == ""
			o, e := readSameFacility(ObjectRef{"component", l.ComponentID}, asset.FacilityID, newUse)
			if e != nil {
				return e
			}
			if e = s.channelTx(ctx, tx, a, scope, l.MainChannelID, ObjectRef{"component", l.ComponentID}, asset.FacilityID); e != nil {
				return e
			}
			if newUse {
				l.ID = uuid.NewString()
				_, e = tx.Exec(ctx, `INSERT INTO spatial_component_links(id,tenant_id,asset_id,facility_id,version_id,component_id,element_key,page_id,region,basis_hash,main_channel_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, l.ID, scope.Tenant, asset.ID, asset.FacilityID, id, l.ComponentID, l.ElementKey, l.PageID, k.JSON(l.Region), o.BasisHash, k.Nullable(l.MainChannelID))
			} else {
				tag, err := tx.Exec(ctx, `UPDATE spatial_component_links SET element_key=$4,page_id=$5,region=$6,basis_hash=$7,main_channel_id=$8 WHERE tenant_id=$1 AND version_id=$2 AND id=$3 AND component_id=$9`, scope.Tenant, id, l.ID, l.ElementKey, l.PageID, k.JSON(l.Region), o.BasisHash, k.Nullable(l.MainChannelID), l.ComponentID)
				e = err
				if e == nil && tag.RowsAffected() != 1 {
					return notFound()
				}
			}
			if e != nil {
				return e
			}
		}
		for _, l := range in.FeatureLinks {
			if asset.Kind != "geolayer" || !slices.ContainsFunc(v.Manifest.Features, func(f Feature) bool { return f.ID == l.FeatureKey }) {
				return invalid("/feature_links", "图形不属于当前地理图层。")
			}
			if _, e = readSameFacility(ObjectRef{"facility", l.FacilityID}, l.FacilityID, true); e != nil {
				return e
			}
			_, e = tx.Exec(ctx, `INSERT INTO spatial_feature_links(tenant_id,version_id,feature_key,facility_id) VALUES($1,$2,$3,$4) ON CONFLICT(version_id,feature_key) DO UPDATE SET facility_id=EXCLUDED.facility_id`, scope.Tenant, id, l.FeatureKey, l.FacilityID)
			if e != nil {
				return e
			}
		}
		if in.Settings != nil {
			if e = s.settingsTx(ctx, tx, a, scope, asset, v, *in.Settings); e != nil {
				return e
			}
			if _, e = tx.Exec(ctx, `UPDATE spatial_asset_versions SET settings=$2 WHERE id=$1`, id, k.JSON(in.Settings)); e != nil {
				return e
			}
		}
		if in.ClearCandidates {
			if _, e = tx.Exec(ctx, `UPDATE spatial_asset_versions SET candidates='[]' WHERE id=$1`, id); e != nil {
				return e
			}
		}
		out.ID = id
		out.Revision, e = bumpVersion(ctx, tx, id)
		if e != nil {
			return e
		}
		out.Changed = true
		return complete(ctx, tx, auth, in.Meta, "versions.edit", input, out)
	})
	return out, e
}
func (s *Service) projectDraftTx(ctx context.Context, tx pgx.Tx, a access.Actor, auth k.Authorization, out *Draft) error {
	readObject := s.objectReaderTx(ctx, tx, a, auth.Scope)
	placements := []Placement{}
	for _, p := range out.Placements {
		o, e := readObject(placementRef(p))
		if restricted(e) {
			continue
		}
		if e != nil {
			return e
		}
		p.Object = &o
		p.BasisChanged = p.BasisHash != o.BasisHash
		if e = s.channelTx(ctx, tx, a, auth.Scope, p.MainChannelID, o.ObjectRef, o.FacilityID); restricted(e) {
			p.MainChannelID = ""
		} else if e != nil {
			return e
		}
		placements = append(placements, p)
	}
	out.Placements = placements
	links := []ComponentLink{}
	for _, l := range out.ComponentLinks {
		o, e := readObject(ObjectRef{"component", l.ComponentID})
		if restricted(e) {
			continue
		}
		if e != nil {
			return e
		}
		l.Object = &o
		l.BasisChanged = l.BasisHash != o.BasisHash
		if e = s.channelTx(ctx, tx, a, auth.Scope, l.MainChannelID, o.ObjectRef, o.FacilityID); restricted(e) {
			l.MainChannelID = ""
		} else if e != nil {
			return e
		}
		links = append(links, l)
	}
	out.ComponentLinks = links
	features := []FeatureLink{}
	for _, l := range out.FeatureLinks {
		_, e := readObject(ObjectRef{"facility", l.FacilityID})
		if restricted(e) {
			continue
		}
		if e != nil {
			return e
		}
		features = append(features, l)
	}
	out.FeatureLinks = features
	if auth.Entry == "project" {
		out.Version.Candidates = []Candidate{}
	} else {
		candidates := []Candidate{}
		for _, candidate := range out.Version.Candidates {
			temp := Draft{Asset: out.Asset, Version: Version{Settings: Settings{}}}
			if candidate.Placement != nil {
				temp.Placements = []Placement{*candidate.Placement}
			}
			if candidate.ComponentLink != nil {
				temp.ComponentLinks = []ComponentLink{*candidate.ComponentLink}
			}
			if e := s.projectDraftTx(ctx, tx, a, auth, &temp); e != nil {
				return e
			}
			if len(temp.Placements) > 0 {
				candidate.Placement = &temp.Placements[0]
				candidates = append(candidates, candidate)
			} else if len(temp.ComponentLinks) > 0 {
				candidate.ComponentLink = &temp.ComponentLinks[0]
				candidates = append(candidates, candidate)
			}
		}
		out.Version.Candidates = candidates
	}
	for key, id := range out.Version.Settings.MainChannels {
		owner := splitOwner(key)
		_, e := readObject(owner)
		if e == nil {
			e = s.channelTx(ctx, tx, a, auth.Scope, id, owner, out.Asset.FacilityID)
		}
		if restricted(e) {
			delete(out.Version.Settings.MainChannels, key)
		} else if e != nil {
			return e
		}
	}
	return nil
}
