package sources

import (
	"context"
	"encoding/json"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
)

// Directory is a tenant resource projection. Device information follows the
// same visibility rules as Current; it does not grant source management.
type DirectoryItem struct {
	Target
	Name         string     `json:"name"`
	Code         string     `json:"code"`
	FacilityName string     `json:"facility_name"`
	FacilityCode string     `json:"facility_code"`
	PointName    string     `json:"point_name"`
	PointCode    string     `json:"point_code"`
	MetricName   string     `json:"metric_name"`
	UnitSymbol   string     `json:"unit_symbol"`
	State        string     `json:"state"`
	Source       Projection `json:"source"`
	CreatedAt    time.Time  `json:"-"`
}

type DirectoryPage struct {
	k.Page[DirectoryItem]
	Counts map[string]int `json:"counts"`
}

// parent_id, when present, is a point. All predicates precede pagination.
// Only names from engineering resources participate in search, so a restricted
// device cannot be discovered indirectly by searching its name or code.
const directorySet = `WITH channels AS (
 SELECT c.id,c.facility_id,c.point_id,c.name,c.code,c.created_at,
 f.name facility_name,f.code facility_code,p.name point_name,p.code point_code,
 v.content->>'name' metric_name,COALESCE(u.content->'unit'->>'symbol','') unit_symbol,
 CASE WHEN lineage.retired THEN 'retired'
      WHEN b.id IS NULL THEN 'unbound'
      WHEN NOT $4 THEN 'restricted'
      WHEN d.status<>'enabled' OR o.ended_at IS NOT NULL THEN 'attention'
      ELSE 'bound' END state,
 CASE WHEN b.id IS NOT NULL THEN to_jsonb(b)||jsonb_build_object('revision',b.revision::text) END binding
 FROM engineering_channels c
 JOIN engineering_facilities f ON f.tenant_id=c.tenant_id AND f.id=c.facility_id
 JOIN engineering_points p ON p.tenant_id=c.tenant_id AND p.id=c.point_id
 JOIN engineering_versions v ON v.id=c.metric_version_id
 LEFT JOIN engineering_versions u ON u.id=c.unit_version_id
 LEFT JOIN monitoring_source_bindings b ON b.tenant_id=c.tenant_id AND b.channel_id=c.id AND b.ended_at IS NULL
 LEFT JOIN device_outputs o ON o.tenant_id=b.tenant_id AND o.id=b.output_id
 LEFT JOIN devices d ON d.tenant_id=o.tenant_id AND d.id=o.device_id
 CROSS JOIN LATERAL (
   WITH RECURSIVE ancestors AS (
     SELECT * FROM engineering_resource_nodes WHERE tenant_id=c.tenant_id AND kind='channel' AND id=c.id
     UNION ALL SELECT n.* FROM engineering_resource_nodes n JOIN ancestors a ON n.tenant_id=a.tenant_id AND n.kind=a.parent_kind AND n.id=a.parent_id
   ) SELECT bool_or(status<>'normal') retired,bool_or(deleted_at IS NOT NULL) deleted FROM ancestors
 ) lineage
 WHERE c.tenant_id=$1 AND c.source_kind='direct' AND NOT lineage.deleted
 AND ($2::uuid IS NULL OR c.facility_id=$2) AND ($3::uuid IS NULL OR c.point_id=$3)
 AND ($6::uuid IS NULL OR c.id=$6)
 AND ($5='' OR c.name ILIKE '%'||$5||'%' OR c.code ILIKE '%'||$5||'%' OR p.name ILIKE '%'||$5||'%' OR p.code ILIKE '%'||$5||'%' OR f.name ILIKE '%'||$5||'%' OR f.code ILIKE '%'||$5||'%')
 ) `

func (s *Service) Directory(ctx context.Context, a access.Actor, scope Scope, f Filter) (DirectoryPage, error) {
	out := DirectoryPage{Page: k.Page[DirectoryItem]{Items: []DirectoryItem{}}, Counts: map[string]int{"all": 0, "unbound": 0, "bound": 0, "attention": 0, "restricted": 0, "retired": 0}}
	if scope.Entry != "tenant" {
		return out, k.NotFound("source")
	}
	err := s.transact(ctx, a, scope, "source", "list", false, func(tx pgx.Tx, auth k.Authorization) error {
		if err := f.Validate(); err != nil {
			return err
		}
		if f.State != "" {
			if _, ok := out.Counts[f.State]; !ok || f.State == "all" {
				return k.Problem("invalid_request", "绑定状态筛选无效。", 400)
			}
		}
		cursor, err := k.DecodeCursor(f.Cursor, k.CursorContext(auth, f, "source-directory"))
		if err != nil {
			return err
		}
		args := []any{scope.Tenant, k.Nullable(f.FacilityID), k.Nullable(f.ParentID), auth.Has(access.TenantDeviceRead), f.Query, k.Nullable(f.ChannelID)}
		counts, err := tx.Query(ctx, directorySet+`SELECT state,count(*) FROM channels GROUP BY state`, args...)
		if err != nil {
			return err
		}
		for counts.Next() {
			var state string
			var count int
			if err = counts.Scan(&state, &count); err != nil {
				counts.Close()
				return err
			}
			out.Counts[state] = count
			out.Counts["all"] += count
		}
		counts.Close()
		if err = counts.Err(); err != nil {
			return err
		}
		args = append(args, f.State, cursor.TimeValue(), k.Nullable(cursor.ID), f.Limit+1)
		rows, err := tx.Query(ctx, directorySet+`SELECT id::text,facility_id::text,point_id::text,name,code,facility_name,facility_code,point_name,point_code,metric_name,unit_symbol,state,created_at,binding FROM channels WHERE ($7='' OR state=$7) AND ($8::timestamptz IS NULL OR (created_at,id)>($8,$9::uuid)) ORDER BY created_at,id LIMIT $10`, args...)
		if err != nil {
			return err
		}
		bindings := []Binding{}
		indices := []int{}
		for rows.Next() {
			var v DirectoryItem
			var raw []byte
			if err = rows.Scan(&v.ChannelID, &v.FacilityID, &v.PointID, &v.Name, &v.Code, &v.FacilityName, &v.FacilityCode, &v.PointName, &v.PointCode, &v.MetricName, &v.UnitSymbol, &v.State, &v.CreatedAt, &raw); err != nil {
				rows.Close()
				return err
			}
			v.Source = Projection{Visibility: "full"}
			if len(raw) > 0 {
				var b Binding
				if err = json.Unmarshal(raw, &b); err != nil {
					rows.Close()
					return err
				}
				bindings = append(bindings, b)
				indices = append(indices, len(out.Items))
			}
			out.Items = append(out.Items, v)
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return err
		}
		projected, err := s.projectBindings(ctx, tx, a, auth, bindings, false)
		if err != nil {
			return err
		}
		for i, source := range projected {
			out.Items[indices[i]].Source = source
		}
		if len(out.Items) > f.Limit {
			out.Items = out.Items[:f.Limit]
			last := out.Items[f.Limit-1]
			cursor.ID = last.ChannelID
			cursor.At = last.CreatedAt
			out.NextCursor = cursor.Encode()
		}
		return nil
	})
	return out, err
}
