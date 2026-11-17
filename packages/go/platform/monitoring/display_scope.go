package monitoring

import (
	"context"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	"github.com/jackc/pgx/v5"
)

func (s *Service) DisplayMembersTx(ctx context.Context, tx pgx.Tx, a access.Actor, tenant, project string, selector displayread.Selector) ([]displayread.Member, error) {
	if _, e := access.RequireActorProjectTx(ctx, tx, a, access.MonitoringOperation("project", "selections.create"), tenant, project); e != nil {
		return nil, e
	}
	rows, e := tx.Query(ctx, `SELECT c.id::text,'channel' FROM engineering_channels c
 WHERE c.tenant_id=$1 AND c.deleted_at IS NULL
 AND EXISTS(SELECT 1 FROM monitoring_data_grants d JOIN project_resource_grants g ON g.id=d.resource_grant_id JOIN project_facility_links l ON l.id=g.link_id
 WHERE d.tenant_id=c.tenant_id AND d.project_id=$2 AND d.channel_id=c.id AND d.revoked_at IS NULL AND 'read'=ANY(d.capabilities) AND g.ended_at IS NULL AND l.status='active')
 AND (COALESCE(cardinality($3::uuid[]),0)=0 OR c.id=ANY($3)) AND (COALESCE(cardinality($4::uuid[]),0)=0 OR c.facility_id=ANY($4))
 AND (COALESCE(cardinality($5::text[]),0)=0 OR c.source_kind=ANY($5))
 AND (jsonb_array_length($6::jsonb)=0 OR EXISTS(SELECT 1 FROM jsonb_array_elements($6::jsonb) m WHERE (m->>'kind'='engineering' AND c.metric_version_id::text=m->>'version_id') OR (m->>'kind'='derived' AND c.derived_metric_version_id::text=m->>'version_id')))
 AND (jsonb_array_length($7::jsonb)=0 OR EXISTS(SELECT 1 FROM jsonb_array_elements($7::jsonb) o WHERE o->>'kind'=c.owner_kind AND o->>'id'=CASE c.owner_kind WHEN 'facility' THEN c.facility_id::text WHEN 'component' THEN c.component_id::text WHEN 'monitoring_section' THEN c.monitoring_section_id::text WHEN 'borehole' THEN c.borehole_id::text ELSE c.point_id::text END))
 ORDER BY c.id LIMIT 50001`, tenant, project, selector.IDs, selector.FacilityIDs, selector.SourceKinds, jsonArray(selector.Metrics), jsonArray(selector.Owners))
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []displayread.Member{}
	for rows.Next() {
		var m displayread.Member
		if e = rows.Scan(&m.ID, &m.Kind); e != nil {
			return nil, e
		}
		out = append(out, m)
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	return displayread.NormalizeMembers(out)
}
func jsonArray[T any](v []T) []byte {
	if v == nil {
		return []byte(`[]`)
	}
	return displayread.Bytes(v)
}
