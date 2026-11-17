package spatial

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
)

func displayVersion(v Version) displayread.SpatialVersion {
	return displayread.SpatialVersion{ID: v.ID, AssetID: v.AssetID, Number: strconv.Itoa(v.Number), Format: v.Format, State: v.State, Revision: v.Revision, ManifestHash: v.ManifestHash, PublishedAt: v.PublishedAt}
}
func displayAsset(a Asset, resources map[string]displayread.ResourceSummary) displayread.SpatialAsset {
	out := displayread.SpatialAsset{ID: a.ID, Code: a.Code, Name: a.Name, Kind: a.Kind, Category: a.Category, Status: a.Status, Revision: a.Revision, DefaultState: "unset", VersionCount: "0"}
	if r, ok := resources[displayread.RefKey(displayread.Ref{Kind: "facility", ID: a.FacilityID})]; ok {
		out.Facility = &displayread.Ref{Kind: "facility", ID: r.ID}
	}
	return out
}
func (s *Service) DisplayReadTx(ctx context.Context, tx pgx.Tx, p displayread.Plan, resources []displayread.ResourceSummary) (displayread.Materialized, error) {
	out := displayread.Materialized{Rows: []json.RawMessage{}}
	o := p.Authority.Owner
	a := p.Authority.Subject.ReadActor()
	scope := Project(o.Tenant, o.Project)
	auth, e := k.AuthorizeTx(ctx, tx, a, scope, access.SpatialOperation("project", "assets.list"))
	if e != nil {
		return out, e
	}
	refs := map[string]displayread.ResourceSummary{}
	for _, r := range resources {
		if r.Visibility == "full" {
			refs[displayread.RefKey(displayread.Ref{Kind: r.Kind, ID: r.ID})] = r
		}
	}
	switch q := p.Request.(type) {
	case *displayread.EmptyRequest:
		return s.displayBasemaps(), nil
	case *displayread.TileRequest:
		return s.displayTile(*q)
	case *displayread.SpatialAssetsRequest:
		return s.displayAssetsTx(ctx, tx, p, *q, refs)
	case *displayread.SpatialVersionsRequest:
		return s.displayVersionsTx(ctx, tx, p, *q)
	case *displayread.SpatialLocationsRequest:
		return s.displayLocationsTx(ctx, tx, p, *q, refs)
	case *displayread.SpatialFileRequest:
		return s.displayFileTx(ctx, tx, p, auth, *q)
	}
	var id, search string
	switch q := p.Request.(type) {
	case *displayread.SpatialVersionRequest:
		id, search = q.ID, q.Search
	case *displayread.MapFeaturesRequest:
		id, search = q.ID, q.Search
	case *displayread.SpatialSettingsRequest:
		id = q.ID
	}
	if !slices.Contains(p.Scopes["target"].IDs("version"), id) {
		return out, displayread.Hidden()
	}
	asset, v, e := s.readableTx(ctx, tx, auth, id, false)
	if e != nil {
		return out, e
	}
	properties := slices.Contains(p.Scopes["target"].FieldGroups, "spatial.properties") && v.Settings.SourceProperties
	if slices.Contains(p.Scopes["target"].FieldGroups, "spatial.properties") && !properties {
		return out, displayread.Problem("field_not_granted", "当前空间版本不允许源属性读取。", 403)
	}
	switch p.Method {
	case "spatial.manifest":
		m := displayread.SpatialManifest{SpatialVersion: displayVersion(v), Contract: v.Manifest.Contract, Files: []displayread.SpatialFile{}, Pages: []displayread.SpatialPage{}, ElementCount: strconv.Itoa(len(v.Manifest.Elements)), FeatureCount: strconv.Itoa(len(v.Manifest.Features)), Warnings: v.Manifest.Warnings}
		if slices.Contains(p.Scopes["target"].FieldGroups, "spatial.geometry") {
			_ = json.Unmarshal(k.JSON(v.Manifest.Coordinates), &m.Coordinates)
			for _, f := range v.Manifest.Files {
				m.Files = append(m.Files, displayread.SpatialFile{ID: f.ID, Name: f.Name, Size: strconv.FormatInt(f.Size, 10), SHA256: f.SHA256, MediaType: f.MediaType})
			}
			_ = json.Unmarshal(k.JSON(v.Manifest.Pages), &m.Pages)
		}
		out.Single = m
	case "spatial.pages":
		for _, page := range v.Manifest.Pages {
			var safe displayread.SpatialPage
			_ = json.Unmarshal(k.JSON(page), &safe)
			out.Rows = append(out.Rows, displayread.Bytes(safe))
		}
	case "spatial.objects":
		for _, el := range v.Manifest.Elements {
			if !matched(el.Name+" "+el.Key, search) {
				continue
			}
			var safe displayread.SpatialElement
			_ = json.Unmarshal(k.JSON(el), &safe)
			if !properties {
				safe.Properties = nil
				safe.TypeProperties = nil
			}
			out.Rows = append(out.Rows, displayread.Bytes(safe))
		}
		slices.SortFunc(out.Rows, func(a, b json.RawMessage) int {
			var x, y displayread.SpatialElement
			_ = json.Unmarshal(a, &x)
			_ = json.Unmarshal(b, &y)
			return strings.Compare(x.Key, y.Key)
		})
	case "spatial.settings":
		return s.displaySettingsTx(ctx, tx, p, v, refs)
	case "spatial.placements", "spatial.componentLinks", "map.features":
		d, e := draftTx(ctx, tx, asset, v)
		if e != nil {
			return out, e
		}
		switch p.Method {
		case "spatial.placements":
			for _, item := range d.Placements {
				target := placementRef(item)
				r, ok := refs[displayread.RefKey(displayread.Ref{Kind: target.Kind, ID: target.ID})]
				if !ok || !matched(r.Code+" "+r.Name, search) {
					continue
				}
				ref := displayread.Ref{Kind: r.Kind, ID: r.ID}
				value := displayread.SpatialPlacement{Geometry: item.Geometry, ID: item.ID, Target: &ref, Code: r.Code, Name: r.Name, Status: r.Status, PageID: item.PageID, Anchor: item.Anchor, ElementKey: item.ElementKey, LabelOffset: item.LabelOffset, BasisChanged: item.BasisHash != r.BasisHash}
				if r.Kind == "point" {
					value.Point = &ref
				}
				out.Rows = append(out.Rows, displayread.Bytes(value))
			}
		case "spatial.componentLinks":
			for _, item := range d.ComponentLinks {
				r, ok := refs[displayread.RefKey(displayread.Ref{Kind: "component", ID: item.ComponentID})]
				if !ok || r.Kind != "component" || !matched(r.Code+" "+r.Name, search) {
					continue
				}
				out.Rows = append(out.Rows, displayread.Bytes(displayread.SpatialComponentLink{ID: item.ID, Component: displayread.Ref{Kind: "component", ID: r.ID}, Code: r.Code, Name: r.Name, Status: r.Status, PageID: item.PageID, ElementKey: item.ElementKey, Region: item.Region, BasisChanged: item.BasisHash != r.BasisHash}))
			}
		case "map.features":
			q := p.Request.(*displayread.MapFeaturesRequest)
			if asset.Kind != "geolayer" {
				return out, displayread.Invalid("/id", "要素读取需要已发布地理图层版本。")
			}
			bbox := []float64{}
			if q.BBox != nil {
				bbox = []float64{q.BBox.West, q.BBox.South, q.BBox.East, q.BBox.North}
			}
			links := map[string]string{}
			for _, l := range d.FeatureLinks {
				links[l.FeatureKey] = l.FacilityID
			}
			for _, f := range v.Manifest.Features {
				if !bboxIntersects(f.BBox, bbox) {
					continue
				}
				if search != "" && (!properties || !matched(f.Properties["name"], search)) {
					continue
				}
				item := displayread.MapFeature{ID: f.ID, Type: f.Type, Geometry: f.Geometry, BBox: f.BBox, FacilityState: "unassigned"}
				if properties {
					item.Properties = f.Properties
				}
				if fid := links[f.ID]; fid != "" {
					item.FacilityState = "restricted"
					if _, ok := refs[displayread.RefKey(displayread.Ref{Kind: "facility", ID: fid})]; ok {
						item.FacilityState = "full"
						item.Facility = &displayread.Ref{Kind: "facility", ID: fid}
					}
				}
				out.Rows = append(out.Rows, displayread.Bytes(item))
			}
			slices.SortFunc(out.Rows, func(a, b json.RawMessage) int {
				var x, y displayread.MapFeature
				_ = json.Unmarshal(a, &x)
				_ = json.Unmarshal(b, &y)
				return strings.Compare(x.ID, y.ID)
			})
		}
	default:
		return out, displayread.Problem("capability_unavailable", "空间读取方法不可用。", 409)
	}
	return out, nil
}

func (s *Service) displayAssetsTx(ctx context.Context, tx pgx.Tx, p displayread.Plan, q displayread.SpatialAssetsRequest, refs map[string]displayread.ResourceSummary) (displayread.Materialized, error) {
	out := displayread.Materialized{Rows: []json.RawMessage{}}
	o := p.Authority.Owner
	for _, id := range q.Filters.FacilityIDs {
		if _, ok := refs[displayread.RefKey(displayread.Ref{Kind: "facility", ID: id})]; !ok {
			return out, displayread.Hidden()
		}
	}
	rows, e := tx.Query(ctx, `SELECT `+assetJSON+`,count(v.id)::text,COALESCE((SELECT g.version_id::text FROM project_spatial_grants g WHERE g.tenant_id=a.tenant_id AND g.project_id=$2 AND g.asset_id=a.id AND g.is_default AND g.ended_at IS NULL),'')
 FROM spatial_assets a JOIN spatial_asset_versions v ON v.tenant_id=a.tenant_id AND v.asset_id=a.id AND v.id=ANY($3::uuid[])
 WHERE a.tenant_id=$1 AND ($4='' OR strpos(lower(a.name),lower($4))>0 OR strpos(lower(a.code),lower($4))>0)
 AND (COALESCE(cardinality($5::uuid[]),0)=0 OR a.facility_id=ANY($5)) AND (COALESCE(cardinality($6::text[]),0)=0 OR a.kind=ANY($6)) AND (COALESCE(cardinality($7::text[]),0)=0 OR a.status=ANY($7)) GROUP BY a.id ORDER BY a.kind,a.code,a.id`, o.Tenant, o.Project, p.Scopes["target"].IDs("version"), q.Search, q.Filters.FacilityIDs, q.Filters.AssetKinds, q.Filters.Statuses)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		var count, defaultID string
		if e = rows.Scan(&raw, &count, &defaultID); e != nil {
			return out, e
		}
		var a Asset
		if e = json.Unmarshal(raw, &a); e != nil {
			return out, e
		}
		v := displayAsset(a, refs)
		v.VersionCount = count
		if defaultID != "" {
			v.DefaultState = "unavailable"
			if slices.Contains(p.Scopes["target"].IDs("version"), defaultID) {
				v.DefaultState = "available"
				v.VersionID = defaultID
			}
		}
		out.Rows = append(out.Rows, displayread.Bytes(v))
	}
	return out, rows.Err()
}
func (s *Service) displayVersionsTx(ctx context.Context, tx pgx.Tx, p displayread.Plan, q displayread.SpatialVersionsRequest) (displayread.Materialized, error) {
	out := displayread.Materialized{Rows: []json.RawMessage{}}
	rows, e := tx.Query(ctx, `SELECT id::text,asset_id::text,number::text,format,state,revision::text,manifest_hash,published_at FROM spatial_asset_versions WHERE tenant_id=$1 AND asset_id=$2 AND id=ANY($3::uuid[]) ORDER BY number,id`, p.Authority.Owner.Tenant, q.AssetID, p.Scopes["target"].IDs("version"))
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var v displayread.SpatialVersion
		if e = rows.Scan(&v.ID, &v.AssetID, &v.Number, &v.Format, &v.State, &v.Revision, &v.ManifestHash, &v.PublishedAt); e != nil {
			return out, e
		}
		out.Rows = append(out.Rows, displayread.Bytes(v))
	}
	if e = rows.Err(); e != nil {
		return out, e
	}
	if len(out.Rows) == 0 {
		return out, displayread.Hidden()
	}
	return out, nil
}
func (s *Service) displayLocationsTx(ctx context.Context, tx pgx.Tx, p displayread.Plan, q displayread.SpatialLocationsRequest, refs map[string]displayread.ResourceSummary) (displayread.Materialized, error) {
	out := displayread.Materialized{Rows: []json.RawMessage{}}
	var r displayread.ResourceSummary
	ok := false
	for _, candidate := range refs {
		if candidate.ID == q.ID && (q.Kind == "" || candidate.Kind == q.Kind) {
			if ok {
				return out, displayread.Invalid("/kind", "同一标识对应多个种类，请明确资源种类。")
			}
			r = candidate
			ok = true
		}
	}
	if !ok || !slices.Contains([]string{"facility", "component", "monitoring_section", "borehole", "point"}, r.Kind) {
		return out, displayread.Hidden()
	}
	rows, e := tx.Query(ctx, `SELECT `+assetJSON+`,v.id::text,v.number::text,COALESCE(p.page_id,l.page_id,''),p.anchor,COALESCE(p.element_key,l.element_key,''),p.geometry
 FROM spatial_asset_versions v JOIN spatial_assets a ON a.tenant_id=v.tenant_id AND a.id=v.asset_id
 LEFT JOIN spatial_point_placements p ON p.tenant_id=v.tenant_id AND p.version_id=v.id AND p.target_kind=$4 AND p.target_id=$3
 LEFT JOIN spatial_component_links l ON l.tenant_id=v.tenant_id AND l.version_id=v.id AND $4='component' AND l.component_id=$3
 WHERE v.tenant_id=$1 AND v.id=ANY($2::uuid[]) AND (($4='facility' AND a.facility_id=$3) OR p.id IS NOT NULL OR l.id IS NOT NULL)
 ORDER BY a.code,a.id,v.number,v.id,p.id,l.id`, p.Authority.Owner.Tenant, p.Scopes["spatial_target"].IDs("version"), q.ID, r.Kind)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var raw, anchor, geometry []byte
		var item displayread.SpatialLocation
		if e = rows.Scan(&raw, &item.VersionID, &item.Number, &item.PageID, &anchor, &item.ElementKey, &geometry); e != nil {
			return out, e
		}
		var asset Asset
		if e = json.Unmarshal(raw, &asset); e != nil {
			return out, e
		}
		if len(anchor) > 0 {
			if e = json.Unmarshal(anchor, &item.Anchor); e != nil {
				return out, e
			}
		}
		if len(geometry) > 0 {
			if e = json.Unmarshal(geometry, &item.Geometry); e != nil {
				return out, e
			}
		}
		item.Asset = displayAsset(asset, refs)
		item.Owner = displayread.Ref{Kind: r.Kind, ID: r.ID}
		out.Rows = append(out.Rows, displayread.Bytes(item))
	}
	return out, rows.Err()
}
func (s *Service) displaySettingsTx(ctx context.Context, tx pgx.Tx, p displayread.Plan, v Version, refs map[string]displayread.ResourceSummary) (displayread.Materialized, error) {
	out := displayread.Materialized{}
	settings := v.Settings
	revision := v.Revision
	var raw []byte
	e := tx.QueryRow(ctx, `SELECT config->'settings',revision::text FROM project_spatial_views WHERE tenant_id=$1 AND project_id=$2 AND id=$3 AND config->>'version_id'=id::text`, p.Authority.Owner.Tenant, p.Authority.Owner.Project, v.ID).Scan(&raw, &revision)
	if e != nil && e != pgx.ErrNoRows {
		return out, e
	}
	if e == nil {
		if e = json.Unmarshal(raw, &settings); e != nil {
			return out, e
		}
	}
	// Empty settings are valid. The public DTO promises an array even when
	// older stored versions contain a null or omitted hidden-elements list.
	safe := displayread.SpatialSettings{VersionID: v.ID, Revision: revision, PageID: settings.PageID, HiddenElements: append([]string{}, settings.HiddenElements...), MainChannels: []displayread.SpatialMainChannel{}, MainChannelState: "unset"}
	if settings.Camera != nil {
		safe.Camera = &displayread.SpatialCamera{Position: settings.Camera.Position, Target: settings.Camera.Target}
	}
	if len(settings.MainChannels) > 0 {
		safe.MainChannelState = "restricted"
	}
	keys := []string{}
	for key := range settings.MainChannels {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		parts := strings.SplitN(key, ":", 2)
		if len(parts) != 2 {
			continue
		}
		r, ok := refs[key]
		if !ok || r.Kind != parts[0] {
			continue
		}
		item := displayread.SpatialMainChannel{Owner: displayread.Ref{Kind: r.Kind, ID: r.ID}, State: "restricted"}
		id := settings.MainChannels[key]
		if slices.Contains(p.Scopes["channel_target"].IDs("channel"), id) {
			item.State = "available"
			item.Channel = &displayread.Ref{Kind: "channel", ID: id}
			safe.MainChannelState = "available"
		}
		safe.MainChannels = append(safe.MainChannels, item)
	}
	out.Single = safe
	return out, nil
}
