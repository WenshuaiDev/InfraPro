package resources

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
	"strings"
)

// SpatialTargetTx exposes the same full-object proof used by installation
// references. Context-only ancestors never expose positions or object links.
func (s *Service) SpatialTargetTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, target Target) (DeviceTarget, error) {
	return s.deviceTargetTx(ctx, tx, a, scope, target)
}

// SpatialTargets is a bounded, fully authorized directory across facilities.
// Spatial viewers must not reconstruct this directory from navigation context.
func (s *Service) SpatialTargets(ctx context.Context, a access.Actor, scope Scope, kind Kind, facility string, f Filter, bbox []float64) (Page, error) {
	out := Page{Items: []Item{}}
	if kind != Facility && kind != Component && kind != Point {
		return out, notFound()
	}
	if facility != "" && !access.ValidID(facility) {
		return out, notFound()
	}
	if e := normalizeFilter(&f); e != nil {
		return out, e
	}
	t := scoped(scope, Target{Kind: kind, Facility: facility})
	e := s.transact(ctx, a, scope.operation(string(kind), "list"), scope, false, func(tx pgx.Tx, auth authorization) error {
		cursor, e := readCursor(t, f, false)
		if e != nil {
			return e
		}
		var west, south, east, north any
		if len(bbox) == 4 {
			west, south, east, north = bbox[0], bbox[1], bbox[2], bbox[3]
		}
		query := strings.Replace(`WITH positioned AS(SELECT id,CASE WHEN position#>>'{geographic,crs}'='EPSG:4326' THEN (position#>>'{geographic,longitude}')::double precision WHEN position#>>'{projected,crs}'='EPSG:3857' AND position#>>'{projected,unit}'='m' THEN (position#>>'{projected,x}')::double precision/6378137*180/pi() END AS lon,CASE WHEN position#>>'{geographic,crs}'='EPSG:4326' THEN (position#>>'{geographic,latitude}')::double precision WHEN position#>>'{projected,crs}'='EPSG:3857' AND position#>>'{projected,unit}'='m' AND abs((position#>>'{projected,y}')::double precision)<=20037508.342789244 THEN (2*atan(exp((position#>>'{projected,y}')::double precision/6378137))-pi()/2)*180/pi() END AS lat FROM __TABLE__ WHERE tenant_id=$1) SELECT n.id::text,n.facility_id::text,n.created_at FROM engineering_resource_nodes n JOIN positioned pos ON pos.id=n.id WHERE n.tenant_id=$1 AND n.kind=$2 AND n.deleted_at IS NULL AND ($3::uuid IS NULL OR n.facility_id=$3) AND ($4='all' OR n.status=$4) AND ($5='' OR n.name ILIKE '%'||$5||'%' OR n.code ILIKE '%'||$5||'%') AND ($6::uuid IS NULL OR EXISTS(SELECT 1 FROM project_resource_grants g JOIN project_facility_links l ON l.id=g.link_id AND l.status='active' WHERE g.tenant_id=n.tenant_id AND g.project_id=$6 AND g.resource_kind=n.kind AND COALESCE(g.component_id,g.point_id,g.channel_id,g.facility_id)=n.id AND g.ended_at IS NULL)) AND ($10::double precision IS NULL OR (pos.lon BETWEEN -180 AND 180 AND pos.lat BETWEEN -85.0511287798066 AND 85.0511287798066 AND pos.lat BETWEEN $11 AND $13 AND (CASE WHEN $10<=$12 THEN pos.lon BETWEEN $10 AND $12 ELSE pos.lon>=$10 OR pos.lon<=$12 END))) AND ($7::timestamptz IS NULL OR (n.created_at,n.id)>($7,$8::uuid)) ORDER BY n.created_at,n.id LIMIT $9`, "__TABLE__", tables[kind], 1)
		rows, e := tx.Query(ctx, query, scope.Tenant, kind, nullableID(facility), f.Status, f.Query, nullableID(scope.Project), cursorTime(cursor), nullableID(cursor.ID), f.Limit+1, west, south, east, north)
		if e != nil {
			return e
		}
		targets := []Target{}
		for rows.Next() {
			var target Target
			var at any
			if e = rows.Scan(&target.ID, &target.Facility, &at); e != nil {
				rows.Close()
				return e
			}
			target.Kind = kind
			targets = append(targets, target)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		for _, target := range targets {
			proof, e := s.deviceTargetTx(ctx, tx, a, scope, target)
			if e != nil {
				return e
			}
			out.Items = append(out.Items, Item{Resource: proof.Resource})
		}
		if len(out.Items) > f.Limit {
			out.Items = out.Items[:f.Limit]
			last := out.Items[f.Limit-1]
			out.NextCursor = nextCursor(t, f, false, last.CreatedAt, last.ID)
		}
		out.AccessRevision = auth.AccessRevision
		return nil
	})
	return out, e
}
func nullableID(id string) any {
	if id == "" {
		return nil
	}
	return id
}
func cursorTime(c cursor) any {
	if c.ID == "" {
		return nil
	}
	return c.Time
}
