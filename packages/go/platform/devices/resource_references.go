package devices

import (
	"context"
	"github.com/jackc/pgx/v5"
)

// ResourceImpact reveals only existence; snapshots and hidden IDs stay in SQL.
// Retained correction targets protect their original resources indefinitely.
type ResourceImpact struct {
	Hash                                           string
	Referenced, CurrentInstallation, CurrentSource bool
}

func (s *Service) ResourceImpactTx(ctx context.Context, tx pgx.Tx, tenant, kind, id string) (ResourceImpact, error) {
	var out ResourceImpact
	e := tx.QueryRow(ctx, `WITH RECURSIVE affected(kind,id) AS (
 SELECT kind,id FROM engineering_resource_nodes WHERE tenant_id=$1 AND kind=$2 AND id=$3
 UNION SELECT n.kind,n.id FROM engineering_resource_nodes n JOIN affected a ON n.parent_kind=a.kind AND n.parent_id=a.id WHERE n.tenant_id=$1
 ), refs AS (
 SELECT r.* FROM device_installation_target_refs r WHERE r.tenant_id=$1 AND EXISTS(SELECT 1 FROM affected a WHERE (a.kind='facility' AND a.id=r.facility_id) OR (a.kind='component' AND a.id=r.component_id) OR (a.kind='point' AND a.id=r.point_id))
 ), binds AS (
 SELECT b.* FROM monitoring_source_bindings b WHERE b.tenant_id=$1 AND EXISTS(SELECT 1 FROM affected a WHERE a.kind='channel' AND a.id=b.channel_id)
 ) SELECT EXISTS(SELECT 1 FROM refs) OR EXISTS(SELECT 1 FROM binds),
 EXISTS(SELECT 1 FROM device_installations i WHERE i.tenant_id=$1 AND i.uninstalled_at IS NULL AND EXISTS(SELECT 1 FROM affected a WHERE (a.kind='facility' AND a.id=i.facility_id) OR (a.kind='component' AND a.id=i.component_id) OR (a.kind='point' AND a.id=i.point_id))),
 EXISTS(SELECT 1 FROM binds WHERE ended_at IS NULL),
 md5(jsonb_build_array((SELECT jsonb_agg(to_jsonb(r) ORDER BY r.id) FROM refs r),(SELECT jsonb_agg(to_jsonb(b) ORDER BY b.id) FROM binds b),
 (SELECT jsonb_agg(to_jsonb(i) ORDER BY i.id) FROM device_installations i WHERE i.id IN(SELECT installation_id FROM refs)),
 (SELECT jsonb_agg(jsonb_build_array(d.id,d.revision,d.status) ORDER BY d.id) FROM devices d WHERE d.id IN(SELECT device_id FROM binds) OR d.id IN(SELECT i.device_id FROM device_installations i JOIN refs r ON r.installation_id=i.id)))::text)`, tenant, kind, id).Scan(&out.Referenced, &out.CurrentInstallation, &out.CurrentSource, &out.Hash)
	return out, e
}
