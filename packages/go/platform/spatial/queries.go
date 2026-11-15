package spatial

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
	"strconv"
	"strings"
)

func (s *Service) Manifest(ctx context.Context, a access.Actor, scope Scope, id string) (Version, error) {
	out := Version{}
	e := s.transact(ctx, a, scope, "versions.manifest", false, func(tx pgx.Tx, auth k.Authorization) error {
		asset, v, e := s.readableTx(ctx, tx, auth, id, false)
		if e != nil {
			return e
		}
		d := Draft{Asset: asset, Version: v}
		if e = s.projectDraftTx(ctx, tx, a, auth, &d); e != nil {
			return e
		}
		out = d.Version
		if scope.Entry == "tenant" {
			if e = tx.QueryRow(ctx, `SELECT COALESCE((SELECT id::text FROM spatial_uploads WHERE tenant_id=$1 AND version_id=$2 AND state='uploading' ORDER BY created_at DESC,id DESC LIMIT 1),'')`, scope.Tenant, id).Scan(&out.LatestUploadID); e != nil {
				return e
			}
			if e = tx.QueryRow(ctx, `SELECT COALESCE((SELECT id::text FROM spatial_processing_jobs WHERE tenant_id=$1 AND version_id=$2 ORDER BY created_at DESC,id DESC LIMIT 1),'')`, scope.Tenant, id).Scan(&out.LatestJobID); e != nil {
				return e
			}
		}
		out.Candidates = []Candidate{}
		out.Manifest.Elements = []Element{}
		out.Manifest.Features = []Feature{}
		if out.SourceBlobID != "" {
			out.Source = &File{}
			e = tx.QueryRow(ctx, `SELECT name,size,sha256,media_type FROM spatial_blobs WHERE tenant_id=$1 AND id=$2`, scope.Tenant, out.SourceBlobID).Scan(&out.Source.Name, &out.Source.Size, &out.Source.SHA256, &out.Source.MediaType)
		}
		return e
	})
	return out, e
}

type sliceCursor struct {
	Scope  string `json:"scope"`
	Offset int    `json:"offset"`
}

func pageSlice[T any](items []T, auth k.Authorization, f Filter, extra any) (List[T], error) {
	out := List[T]{Items: []T{}}
	cursor := sliceCursor{Scope: cursorContext(auth, f, extra)}
	if f.Cursor != "" {
		raw, e := base64.RawURLEncoding.DecodeString(f.Cursor)
		if e != nil {
			return out, invalid("/cursor", "分页游标无效。")
		}
		var old sliceCursor
		if json.Unmarshal(raw, &old) != nil || old.Scope != cursor.Scope || old.Offset < 0 || old.Offset > len(items) {
			return out, k.Problem("invalid_cursor", "筛选、版本或授权已变化，请回到第一页。", 400)
		}
		cursor = old
	}
	end := cursor.Offset + f.Limit
	if end > len(items) {
		end = len(items)
	}
	out.Items = append(out.Items, items[cursor.Offset:end]...)
	if end < len(items) {
		cursor.Offset = end
		out.NextCursor = base64.RawURLEncoding.EncodeToString(k.JSON(cursor))
	}
	return out, nil
}
func (s *Service) Pages(ctx context.Context, a access.Actor, scope Scope, id string, f Filter) (List[Page], error) {
	out := List[Page]{Items: []Page{}}
	e := s.transact(ctx, a, scope, "versions.pages", false, func(tx pgx.Tx, auth k.Authorization) error {
		if e := normalizeFilter(&f); e != nil {
			return e
		}
		_, v, e := s.readableTx(ctx, tx, auth, id, false)
		if e != nil {
			return e
		}
		items := []Page{}
		for _, p := range v.Manifest.Pages {
			if f.PageID == "" || p.ID == f.PageID {
				items = append(items, p)
			}
		}
		out, e = pageSlice(items, auth, f, []string{id, v.Revision})
		return e
	})
	return out, e
}
func (s *Service) Elements(ctx context.Context, a access.Actor, scope Scope, id string, f Filter) (List[Element], error) {
	out := List[Element]{Items: []Element{}}
	action := "versions.elements"
	if f.ElementKey != "" {
		action = "versions.properties"
	}
	e := s.transact(ctx, a, scope, action, false, func(tx pgx.Tx, auth k.Authorization) error {
		if e := normalizeFilter(&f); e != nil {
			return e
		}
		_, v, e := s.readableTx(ctx, tx, auth, id, false)
		if e != nil {
			return e
		}
		items := []Element{}
		for _, item := range v.Manifest.Elements {
			if (f.ElementKey == "" || item.Key == f.ElementKey) && matched(item.Name+" "+item.Key, f.Query) {
				if !v.Settings.SourceProperties {
					item.Properties = map[string]any{}
					item.TypeProperties = map[string]any{}
				}
				items = append(items, item)
			}
		}
		out, e = pageSlice(items, auth, f, []string{id, v.Revision})
		return e
	})
	return out, e
}
func (s *Service) Placements(ctx context.Context, a access.Actor, scope Scope, id string, f Filter) (List[Placement], error) {
	out := List[Placement]{Items: []Placement{}}
	e := s.transact(ctx, a, scope, "versions.placements", false, func(tx pgx.Tx, auth k.Authorization) error {
		if e := normalizeFilter(&f); e != nil {
			return e
		}
		asset, v, e := s.readableTx(ctx, tx, auth, id, false)
		if e != nil {
			return e
		}
		d, e := draftTx(ctx, tx, asset, v)
		if e != nil {
			return e
		}
		if e = s.projectDraftTx(ctx, tx, a, auth, &d); e != nil {
			return e
		}
		items := []Placement{}
		for _, p := range d.Placements {
			if (f.OwnerID == "" || p.PointID == f.OwnerID) && (f.PageID == "" || p.PageID == f.PageID) && (f.State != "normal" || !p.Object.Retired) && matched(p.Object.Name+" "+p.Object.Code, f.Query) {
				items = append(items, p)
			}
		}
		out, e = pageSlice(items, auth, f, []string{id, v.Revision})
		return e
	})
	return out, e
}
func (s *Service) ComponentLinks(ctx context.Context, a access.Actor, scope Scope, id string, f Filter) (List[ComponentLink], error) {
	out := List[ComponentLink]{Items: []ComponentLink{}}
	e := s.transact(ctx, a, scope, "versions.component-links", false, func(tx pgx.Tx, auth k.Authorization) error {
		if e := normalizeFilter(&f); e != nil {
			return e
		}
		asset, v, e := s.readableTx(ctx, tx, auth, id, false)
		if e != nil {
			return e
		}
		d, e := draftTx(ctx, tx, asset, v)
		if e != nil {
			return e
		}
		if e = s.projectDraftTx(ctx, tx, a, auth, &d); e != nil {
			return e
		}
		items := []ComponentLink{}
		for _, l := range d.ComponentLinks {
			if (f.OwnerID == "" || l.ComponentID == f.OwnerID) && (f.PageID == "" || l.PageID == f.PageID) && (f.ElementKey == "" || l.ElementKey == f.ElementKey) && matched(l.Object.Name+" "+l.Object.Code, f.Query) {
				items = append(items, l)
			}
		}
		out, e = pageSlice(items, auth, f, []string{id, v.Revision})
		return e
	})
	return out, e
}

type FeatureView struct {
	Feature
	Facility *Object `json:"facility,omitempty"`
}

func (s *Service) Features(ctx context.Context, a access.Actor, scope Scope, id string, f Filter) (List[FeatureView], error) {
	out := List[FeatureView]{Items: []FeatureView{}}
	e := s.transact(ctx, a, scope, "versions.features", false, func(tx pgx.Tx, auth k.Authorization) error {
		if e := normalizeFilter(&f); e != nil {
			return e
		}
		asset, v, e := s.readableTx(ctx, tx, auth, id, false)
		if e != nil {
			return e
		}
		d, e := draftTx(ctx, tx, asset, v)
		if e != nil {
			return e
		}
		if e = s.projectDraftTx(ctx, tx, a, auth, &d); e != nil {
			return e
		}
		links := map[string]string{}
		for _, l := range d.FeatureLinks {
			links[l.FeatureKey] = l.FacilityID
		}
		items := []FeatureView{}
		for _, feature := range v.Manifest.Features {
			if !matched(feature.Properties["name"], f.Query) || !bboxIntersects(feature.BBox, f.BBox) {
				continue
			}
			item := FeatureView{Feature: feature}
			if fid := links[feature.ID]; fid != "" {
				o, e := s.objectTx(ctx, tx, a, scope, ObjectRef{"facility", fid})
				if e != nil {
					return e
				}
				item.Facility = &o
			}
			items = append(items, item)
		}
		out, e = pageSlice(items, auth, f, []string{id, v.Revision})
		return e
	})
	return out, e
}
func bboxIntersects(a, b []float64) bool {
	if len(b) == 0 {
		return true
	}
	if len(a) != 4 {
		return false
	}
	if a[3] < b[1] || a[1] > b[3] {
		return false
	}
	if b[0] > b[2] {
		return a[2] >= b[0] || a[0] <= b[2]
	}
	return a[2] >= b[0] && a[0] <= b[2]
}
func (s *Service) Objects(ctx context.Context, a access.Actor, scope Scope, f Filter) (List[Object], error) {
	out := List[Object]{Items: []Object{}}
	if e := normalizeFilter(&f); e != nil {
		return out, e
	}
	var revision, cursorScope string
	type objectCursor struct{ Scope, Inner string }
	e := s.transact(ctx, a, scope, "objects", false, func(tx pgx.Tx, auth k.Authorization) error {
		revision = auth.Revision
		cursorScope = cursorContext(auth, f, "objects")
		if f.Cursor != "" {
			var cur objectCursor
			raw, err := base64.RawURLEncoding.DecodeString(f.Cursor)
			if err != nil || json.Unmarshal(raw, &cur) != nil || cur.Scope != cursorScope {
				return k.Problem("invalid_cursor", "查询或授权已变化，请回到第一页。", 400)
			}
			f.Cursor = cur.Inner
		}
		return nil
	})
	if e != nil {
		return out, e
	}
	if s.objects == nil {
		return out, unavailable()
	}
	out, e = s.objects.Objects(ctx, a, scope, f)
	if out.NextCursor != "" {
		out.NextCursor = base64.RawURLEncoding.EncodeToString(k.JSON(objectCursor{cursorScope, out.NextCursor}))
	}
	if e != nil {
		return out, e
	}
	e = s.transact(ctx, a, scope, "objects", false, func(tx pgx.Tx, auth k.Authorization) error {
		if auth.Revision != revision {
			return k.Problem("authorization_changed", "授权已变化，请重新读取。", 409)
		}
		for _, o := range out.Items {
			if _, e := s.objectTx(ctx, tx, a, scope, o.ObjectRef); e != nil {
				return e
			}
		}
		return nil
	})
	return out, e
}
func (s *Service) checkObjects(ctx context.Context, a access.Actor, scope Scope, action string, objects []ObjectRef) (string, error) {
	var revision string
	if len(objects) < 1 || len(objects) > 100 {
		return "", invalid("/objects", "每次选择 1–100 个对象。")
	}
	e := s.transact(ctx, a, scope, action, false, func(tx pgx.Tx, auth k.Authorization) error {
		revision = auth.Revision + ":" + auth.ResourceAccessRevision
		for _, ref := range objects {
			if ref.Kind != "facility" && ref.Kind != "component" && ref.Kind != "point" {
				return invalid("/objects", "请选择设施、构件或测点。")
			}
			if _, e := s.objectTx(ctx, tx, a, scope, ref); e != nil {
				return e
			}
		}
		return nil
	})
	return revision, e
}
func (s *Service) Summaries(ctx context.Context, a access.Actor, scope Scope, in ObjectQuery) (json.RawMessage, error) {
	revision, e := s.checkObjects(ctx, a, scope, "summaries", in.Objects)
	if e != nil {
		return nil, e
	}
	if s.monitoring == nil {
		return nil, unavailable()
	}
	if len(in.MainChannels) > len(in.Objects) {
		return nil, invalid("/main_channels", "主通道只可指定当前查询对象。")
	}
	for key, channel := range in.MainChannels {
		found := false
		for _, o := range in.Objects {
			if key == o.Kind+":"+o.ID {
				found = true
				break
			}
		}
		if !found || !access.ValidID(channel) {
			return nil, invalid("/main_channels", "主通道对象或编号无效。")
		}
	}
	out, e := s.monitoring.Summaries(ctx, a, scope, in)
	if e != nil {
		return nil, e
	}
	after, e := s.checkObjects(ctx, a, scope, "summaries", in.Objects)
	if e != nil {
		return nil, e
	}
	if after != revision {
		return nil, k.Problem("authorization_changed", "授权已变化，请重新读取。", 409)
	}
	return out, nil
}
func (s *Service) RelatedArtifacts(ctx context.Context, a access.Actor, scope Scope, in RelatedInput) (json.RawMessage, error) {
	revision, e := s.checkObjects(ctx, a, scope, "related-artifacts", []ObjectRef{in.Owner})
	if e != nil {
		return nil, e
	}
	if s.artifacts == nil {
		return nil, unavailable()
	}
	out, e := s.artifacts.Related(ctx, a, scope, in)
	if e != nil {
		return nil, e
	}
	after, e := s.checkObjects(ctx, a, scope, "related-artifacts", []ObjectRef{in.Owner})
	if e != nil {
		return nil, e
	}
	if after != revision {
		return nil, k.Problem("authorization_changed", "授权已变化，请重新读取。", 409)
	}
	return out, nil
}

type Location struct {
	Asset      Asset     `json:"asset"`
	VersionID  string    `json:"version_id"`
	Number     int       `json:"number"`
	Owner      ObjectRef `json:"owner"`
	PageID     string    `json:"page_id"`
	Anchor     []float64 `json:"anchor,omitempty"`
	ElementKey string    `json:"element_key,omitempty"`
}

func (s *Service) Locate(ctx context.Context, a access.Actor, scope Scope, in ObjectQuery) (List[Location], error) {
	out := List[Location]{Items: []Location{}}
	if _, e := s.checkObjects(ctx, a, scope, "locate", in.Objects); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, scope, "locate", false, func(tx pgx.Tx, auth k.Authorization) error {
		for _, ref := range in.Objects {
			if _, e := s.objectTx(ctx, tx, a, scope, ref); e != nil {
				return e
			}
			query := `SELECT v.id::text FROM spatial_asset_versions v JOIN spatial_assets a ON a.id=v.asset_id WHERE v.tenant_id=$1 AND v.state='published' AND ` + `($2::uuid IS NULL OR EXISTS(SELECT 1 FROM project_spatial_grants g WHERE g.tenant_id=v.tenant_id AND g.project_id=$2 AND g.version_id=v.id AND g.ended_at IS NULL)) AND `
			switch ref.Kind {
			case "facility":
				query += `a.facility_id=$3`
			case "point":
				query += `EXISTS(SELECT 1 FROM spatial_point_placements WHERE version_id=v.id AND point_id=$3)`
			case "component":
				query += `EXISTS(SELECT 1 FROM spatial_component_links WHERE version_id=v.id AND component_id=$3)`
			}
			query += ` ORDER BY v.created_at DESC,v.id LIMIT 100`
			rows, e := tx.Query(ctx, query, scope.Tenant, k.Nullable(scope.Project), ref.ID)
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
				asset, v, e := s.readableTx(ctx, tx, auth, id, false)
				if restricted(e) {
					continue
				}
				if e != nil {
					return e
				}
				d, e := draftTx(ctx, tx, asset, v)
				if e != nil {
					return e
				}
				if scope.Project != "" {
					asset.CurrentVersionID = ""
				}
				base := Location{Asset: asset, VersionID: id, Number: v.Number, Owner: ref}
				if ref.Kind == "facility" {
					out.Items = append(out.Items, base)
				}
				for _, p := range d.Placements {
					if ref.Kind == "point" && p.PointID == ref.ID {
						item := base
						item.PageID = p.PageID
						item.Anchor = p.Anchor
						item.ElementKey = p.ElementKey
						out.Items = append(out.Items, item)
					}
				}
				for _, l := range d.ComponentLinks {
					if ref.Kind == "component" && l.ComponentID == ref.ID {
						item := base
						item.PageID = l.PageID
						item.ElementKey = l.ElementKey
						out.Items = append(out.Items, item)
					}
				}
				if len(out.Items) > 1000 {
					return invalid("/objects", "可用挂载超过交互预算，请缩小对象范围。")
				}
			}
		}
		return nil
	})
	return out, e
}
func ParseBBox(raw string) ([]float64, error) {
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	if len(parts) != 4 {
		return nil, invalid("/bbox", "请提供四个逗号分隔坐标。")
	}
	out := make([]float64, 4)
	for i, p := range parts {
		v, e := strconv.ParseFloat(p, 64)
		if e != nil {
			return nil, invalid("/bbox", "范围坐标无效。")
		}
		out[i] = v
	}
	return out, validateBBox(out)
}

func (s *Service) Object(ctx context.Context, a access.Actor, scope Scope, ref ObjectRef) (Object, error) {
	out := Object{}
	e := s.transact(ctx, a, scope, "objects.detail", false, func(tx pgx.Tx, _ k.Authorization) error {
		var e error
		out, e = s.objectTx(ctx, tx, a, scope, ref)
		return e
	})
	return out, e
}
