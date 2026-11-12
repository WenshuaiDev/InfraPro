package monitoring

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

type NavigationQuery struct{ Cursor, Search, Facility, Point string }
type MonitoringPoint struct {
	ID           string          `json:"id"`
	Code         string          `json:"code"`
	Name         string          `json:"name"`
	Facility     string          `json:"facility_id"`
	FacilityName string          `json:"facility_name"`
	Component    string          `json:"component_id"`
	Status       string          `json:"status"`
	Position     json.RawMessage `json:"position"`
	Direction    string          `json:"direction_description"`
	Reference    string          `json:"reference_description"`
}
type MonitoringChannel struct {
	ID          string          `json:"id"`
	Point       string          `json:"point_id"`
	Code        string          `json:"code"`
	Name        string          `json:"name"`
	Status      string          `json:"status"`
	Metric      string          `json:"metric_version_id"`
	MetricName  string          `json:"metric_name"`
	Unit        string          `json:"unit"`
	Kind        string          `json:"value_kind"`
	Direction   string          `json:"observation_direction"`
	Reference   string          `json:"reference_basis"`
	TimeMeaning string          `json:"timestamp_meaning"`
	Sampling    json.RawMessage `json:"sampling"`
	Ranges      []Range         `json:"read_ranges"`
}
type PointPage struct {
	Items         []MonitoringPoint `json:"items"`
	NextCursor    string            `json:"next_cursor"`
	Authorization string            `json:"authorization_revision"`
}
type ChannelPage struct {
	Items         []MonitoringChannel `json:"items"`
	NextCursor    string              `json:"next_cursor"`
	Authorization string              `json:"authorization_revision"`
}

const visibleDataChannel = `c.deleted_at IS NULL AND ($2='' OR EXISTS(SELECT 1 FROM monitoring_data_grants d JOIN project_resource_grants g ON g.id=d.resource_grant_id JOIN project_facility_links l ON l.id=g.link_id WHERE d.tenant_id=c.tenant_id AND d.channel_id=c.id AND d.project_id::text=$2 AND d.revoked_at IS NULL AND 'read'=ANY(d.capabilities) AND g.ended_at IS NULL AND l.status='active'))`

func navigationPermission(ctx context.Context, tx pgx.Tx, auth authorization) error {
	if auth.Entry == "project" {
		_, e := access.RequireProjectTx(ctx, tx, auth.Account, access.ProjectResourceOperation("channel", "detail"), auth.Tenant, auth.Project)
		return e
	}
	return access.RequireScopedTx(ctx, tx, auth.Account, access.ResourceOperation("channel", "detail"), auth.Tenant)
}
func (s *Service) Points(ctx context.Context, a access.Actor, scope Scope, q NavigationQuery) (PointPage, error) {
	out := PointPage{Items: []MonitoringPoint{}}
	if q.Cursor != "" && !access.ValidID(q.Cursor) || q.Facility != "" && !access.ValidID(q.Facility) || len(q.Search) > 120 || q.Point != "" && !access.ValidID(q.Point) {
		return out, invalid("query", "导航筛选或游标无效。")
	}
	search := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(strings.TrimSpace(q.Search))
	e := s.transact(ctx, a, scope, "points.list", false, false, func(tx pgx.Tx, auth authorization) error {
		if e := navigationPermission(ctx, tx, auth); e != nil {
			return e
		}
		out.Authorization = authorizationVersion(auth)
		rows, e := tx.Query(ctx, `SELECT p.id::text,p.code,p.name,p.facility_id::text,f.name,COALESCE(p.component_id::text,''),p.status,p.position,p.direction_description,p.reference_description FROM engineering_points p JOIN engineering_facilities f ON f.tenant_id=p.tenant_id AND f.id=p.facility_id WHERE p.tenant_id=$1 AND p.deleted_at IS NULL AND f.deleted_at IS NULL AND ($3='' OR p.facility_id::text=$3) AND ($6='' OR p.id::text=$6) AND ($4='' OR p.id::text>$4) AND ($5='' OR p.name ILIKE '%'||$5||'%' OR p.code ILIKE '%'||$5||'%' OR f.name ILIKE '%'||$5||'%') AND EXISTS(SELECT 1 FROM engineering_channels c WHERE c.tenant_id=p.tenant_id AND c.point_id=p.id AND `+visibleDataChannel+`) ORDER BY p.id LIMIT 51`, scope.Tenant, scope.Project, q.Facility, q.Cursor, search, q.Point)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var p MonitoringPoint
			if e = rows.Scan(&p.ID, &p.Code, &p.Name, &p.Facility, &p.FacilityName, &p.Component, &p.Status, &p.Position, &p.Direction, &p.Reference); e != nil {
				return e
			}
			out.Items = append(out.Items, p)
		}
		if e = rows.Err(); e != nil {
			return e
		}
		if len(out.Items) > 50 {
			out.Items = out.Items[:50]
			out.NextCursor = out.Items[49].ID
		}
		return nil
	})
	return out, e
}
func (s *Service) Channels(ctx context.Context, a access.Actor, scope Scope, point, cursor string, selected ...string) (ChannelPage, error) {
	out := ChannelPage{Items: []MonitoringChannel{}}
	channel := ""
	if len(selected) > 0 {
		channel = selected[0]
	}
	if channel != "" && !access.ValidID(channel) {
		return out, notFound()
	}
	if !access.ValidID(point) || cursor != "" && !access.ValidID(cursor) {
		return out, notFound()
	}
	e := s.transact(ctx, a, scope, "channels.list", false, false, func(tx pgx.Tx, auth authorization) error {
		if e := navigationPermission(ctx, tx, auth); e != nil {
			return e
		}
		out.Authorization = authorizationVersion(auth)
		rows, e := tx.Query(ctx, `SELECT c.id::text,c.point_id::text,c.code,c.name,c.status,c.metric_version_id::text,COALESCE(m.content->>'name',''),COALESCE(u.content->'unit'->>'symbol',''),m.content->'metric'->>'shape',c.observation_direction,c.reference_basis,c.timestamp_meaning,COALESCE(c.sampling,'null'::jsonb) FROM engineering_channels c JOIN engineering_versions m ON m.id=c.metric_version_id LEFT JOIN engineering_versions u ON u.id=c.unit_version_id WHERE c.tenant_id=$1 AND c.point_id=$3 AND ($4='' OR c.id::text>$4) AND ($5='' OR c.id::text=$5) AND `+visibleDataChannel+` ORDER BY c.id LIMIT 101`, scope.Tenant, scope.Project, point, cursor, channel)
		if e != nil {
			return e
		}
		ids := []string{}
		for rows.Next() {
			var c MonitoringChannel
			if e = rows.Scan(&c.ID, &c.Point, &c.Code, &c.Name, &c.Status, &c.Metric, &c.MetricName, &c.Unit, &c.Kind, &c.Direction, &c.Reference, &c.TimeMeaning, &c.Sampling); e != nil {
				rows.Close()
				return e
			}
			out.Items = append(out.Items, c)
			ids = append(ids, c.ID)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if len(ids) == 0 {
			return nil
		}
		windows, e := authorizedWindowsTx(ctx, tx, auth, ids, "read")
		if e != nil {
			return e
		}
		for i := range out.Items {
			out.Items[i].Ranges = []Range{}
			for _, w := range windows[out.Items[i].ID] {
				out.Items[i].Ranges = append(out.Items[i].Ranges, w.rangeDTO())
			}
		}
		if len(out.Items) > 100 {
			out.Items = out.Items[:100]
			out.NextCursor = out.Items[99].ID
		}
		return nil
	})
	return out, e
}
