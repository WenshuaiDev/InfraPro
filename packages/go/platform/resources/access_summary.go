package resources

import (
	"context"
	"github.com/jackc/pgx/v5"
)

// Aggregate active grants independently of pagination. These counts are tenant
// governance data, never part of a project's restricted resource projection.
func accessSummaries(ctx context.Context, tx pgx.Tx, tenant, project, facility string) (AccessSummary, map[string]AccessSummary, error) {
	total := AccessSummary{}
	byLink := map[string]AccessSummary{}
	rows, err := tx.Query(ctx, `SELECT l.id::text,
 count(*) FILTER (WHERE g.resource_kind='facility'),
 count(*) FILTER (WHERE g.resource_kind='component'),
 count(*) FILTER (WHERE g.resource_kind='point'),
 count(*) FILTER (WHERE g.resource_kind='channel'),
 count(*) FILTER (WHERE g.resource_kind='monitoring_section'),
 count(*) FILTER (WHERE g.resource_kind='borehole'),
 count(*) FILTER (WHERE g.can_update OR g.can_create_component OR g.can_create_point OR g.can_create_channel OR g.can_lifecycle OR g.can_source_manage OR g.can_create_monitoring_section OR g.can_create_borehole)
 FROM project_facility_links l JOIN project_resource_grants g ON g.link_id=l.id AND g.ended_at IS NULL
 WHERE l.tenant_id=$1 AND l.project_id=$2 AND l.status='active' AND ($3='' OR l.facility_id=NULLIF($3,'')::uuid)
 GROUP BY l.id`, tenant, project, facility)
	if err != nil {
		return total, byLink, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		v := AccessSummary{}
		if err = rows.Scan(&id, &v.Facilities, &v.Components, &v.Points, &v.Channels, &v.MonitoringSections, &v.Boreholes, &v.Maintainable); err != nil {
			return total, byLink, err
		}
		byLink[id] = v
		total.Facilities += v.Facilities
		total.Components += v.Components
		total.Points += v.Points
		total.Channels += v.Channels
		total.MonitoringSections += v.MonitoringSections
		total.Boreholes += v.Boreholes
		total.Maintainable += v.Maintainable
	}
	return total, byLink, rows.Err()
}
