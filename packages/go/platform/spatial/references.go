package spatial

import (
	"context"
	"github.com/jackc/pgx/v5"
)

// ReferencesTx covers all retained versions, including withdrawn publications.
func (s *Service) ReferencesTx(ctx context.Context, tx pgx.Tx, tenant string, ids []string) (string, bool, error) {
	var hash string
	var used bool
	e := tx.QueryRow(ctx, `WITH refs AS(SELECT 'asset' AS kind,id FROM spatial_assets WHERE tenant_id=$1 AND facility_id=ANY($2::uuid[]) UNION ALL SELECT 'point',id FROM spatial_point_placements WHERE tenant_id=$1 AND (point_id=ANY($2::uuid[]) OR main_channel_id=ANY($2::uuid[])) UNION ALL SELECT 'component',id FROM spatial_component_links WHERE tenant_id=$1 AND (component_id=ANY($2::uuid[]) OR main_channel_id=ANY($2::uuid[])) UNION ALL SELECT 'feature',version_id FROM spatial_feature_links WHERE tenant_id=$1 AND facility_id=ANY($2::uuid[]) UNION ALL SELECT 'main_channel',v.id FROM spatial_asset_versions v WHERE v.tenant_id=$1 AND EXISTS(SELECT 1 FROM jsonb_each_text(COALESCE(v.settings->'main_channels','{}'::jsonb)) c WHERE c.value=ANY($2::text[])) UNION ALL SELECT 'view_channel',v.id FROM project_spatial_views v WHERE v.tenant_id=$1 AND EXISTS(SELECT 1 FROM jsonb_each_text(COALESCE(v.config->'settings'->'main_channels','{}'::jsonb)) c WHERE c.value=ANY($2::text[]))) SELECT md5(COALESCE(jsonb_agg(jsonb_build_array(kind,id) ORDER BY kind,id)::text,'[]')),count(*)>0 FROM refs`, tenant, ids).Scan(&hash, &used)
	return hash, used, e
}
