package spatial

import (
	"context"
	"encoding/json"
	"strings"

	ar "github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
)

// Capture management metadata under the source's existing write locks. Geometry,
// manifests, source notes and storage locations never enter the audit payload.
func auditSnapshotTx(ctx context.Context, tx pgx.Tx, scope Scope, kind, id string) (any, error) {
	if id == "" {
		return nil, nil
	}
	var query string
	args := []any{scope.Tenant, id}
	switch kind {
	case "spatial_asset":
		query = `SELECT jsonb_build_object('id',id,'name',name,'code',code,'status',status,'revision',revision::text,'facility_id',facility_id,'current_version_id',current_version_id) FROM spatial_assets WHERE tenant_id=$1 AND id::text=$2`
	case "spatial_version":
		query = `SELECT jsonb_build_object('id',id,'asset_id',asset_id,'version',number::text,'state',state,'revision',revision::text,'base_version_id',base_version_id,'format',format,'manifest_root',manifest_hash,'point_count',(SELECT count(*) FROM spatial_point_placements WHERE version_id=v.id),'component_count',(SELECT count(*) FROM spatial_component_links WHERE version_id=v.id),'feature_count',(SELECT count(*) FROM spatial_feature_links WHERE version_id=v.id)) FROM spatial_asset_versions v WHERE tenant_id=$1 AND id::text=$2 AND deleted_at IS NULL`
	case "spatial_upload":
		query = `SELECT jsonb_build_object('id',id,'version_id',version_id,'state',state,'revision',revision::text,'filename',filename,'size_bytes',size::text,'sha256',sha256) FROM spatial_uploads WHERE tenant_id=$1 AND id::text=$2`
	case "spatial_job":
		query = `SELECT jsonb_build_object('id',id,'version_id',version_id,'state',state,'revision',revision::text,'generation',notification_generation::text,'error_code',error_code) FROM spatial_processing_jobs WHERE tenant_id=$1 AND id::text=$2`
	case "spatial_access":
		query = `SELECT jsonb_build_object('id',$2::text,'revision',COALESCE((SELECT revision::text FROM project_spatial_states WHERE tenant_id=$1 AND project_id::text=$2),'1'),'grants',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',id,'asset_id',asset_id,'version_id',version_id,'source_download',source_download,'is_default',is_default) ORDER BY id) FROM project_spatial_grants WHERE tenant_id=$1 AND project_id::text=$2 AND ended_at IS NULL),'[]'::jsonb))`
	case "spatial_view":
		query = `SELECT jsonb_build_object('id',id,'revision',revision::text,'version_id',config->>'version_id') FROM project_spatial_views WHERE tenant_id=$1 AND id::text=$2 AND project_id=$3`
		args = append(args, scope.Project)
	default:
		return nil, nil
	}
	var raw []byte
	e := tx.QueryRow(ctx, query, args...).Scan(&raw)
	if e == pgx.ErrNoRows {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	var out any
	e = json.Unmarshal(raw, &out)
	return out, e
}

func auditKind(action string) string {
	switch {
	case strings.HasPrefix(action, "assets."):
		return "spatial_asset"
	case strings.HasPrefix(action, "uploads."):
		return "spatial_upload"
	case strings.HasPrefix(action, "jobs."), action == "versions.process":
		return "spatial_job"
	case action == "access.save":
		return "spatial_access"
	case action == "views.save":
		return "spatial_view"
	default:
		return "spatial_version"
	}
}

func auditBeforeTx(ctx context.Context, tx pgx.Tx, auth k.Authorization, action string, input any) error {
	var before any
	if !strings.HasSuffix(action, ".create") && action != "versions.process" {
		if parts, ok := input.([]any); ok && len(parts) > 0 {
			if id, ok := parts[0].(string); ok {
				var e error
				before, e = auditSnapshotTx(ctx, tx, auth.Scope, auditKind(action), id)
				if e != nil {
					return e
				}
			}
		}
	}
	_, e := tx.Exec(ctx, `SELECT set_config('infrapro.spatial_audit_before',$1,true)`, string(ar.JSON(before)))
	return e
}
