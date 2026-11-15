package spatial

import (
	"context"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	ne "github.com/WenshuaiDev/InfraPro/packages/go/platform/notificationevent"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/jackc/pgx/v5"
	"strconv"
)

func jobNotificationTx(ctx context.Context, tx pgx.Tx, id string) error {
	var tenant, actor, originator, version, state, reason string
	var generation int64
	e := tx.QueryRow(ctx, `SELECT tenant_id::text,actor_id::text,version_id::text,state,error_code,notification_generation,COALESCE(originator_account_id::text,'') FROM spatial_processing_jobs WHERE id=$1`, id).Scan(&tenant, &actor, &version, &state, &reason, &generation, &originator)
	if e != nil {
		return e
	}
	if state != "succeeded" && state != "failed" && state != "canceled" {
		return nil
	}
	if state == "canceled" && reason == "" {
		reason = "job_cancelled"
	}
	return ne.EmitTx(ctx, tx, ne.Event{TaskReference: &taskrecord.Ref{SourceRef: taskrecord.SourceRef{Module: "spatial", Kind: "job", RootID: id}, Generation: strconv.FormatInt(generation, 10)}, Module: "spatial", Key: fmt.Sprintf("%s:%d:%s", id, generation, state), Type: "spatial.result", Scope: ne.BusinessScope(tenant, ""), Target: ne.Target{Kind: "spatial_version", ID: version}, Outcome: state, Reason: reason, Recipients: ne.Personal(actor, originator)})
}
func publicationProjectsTx(ctx context.Context, tx pgx.Tx, tenant, asset, version string) ([]string, error) {
	rows, e := tx.Query(ctx, `SELECT DISTINCT project_id::text FROM project_spatial_grants WHERE tenant_id=$1 AND asset_id=$2 AND ($3::uuid IS NULL OR version_id=$3) AND ended_at IS NULL ORDER BY project_id::text`, tenant, asset, k.Nullable(version))
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			return nil, e
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
func publicationNotificationTx(ctx context.Context, tx pgx.Tx, tenant, actor, version, action string, projects []string) error {
	for _, project := range projects {
		ids, e := access.NotificationRecipientsTx(ctx, tx, tenant, project, access.ProjectProfileUpdate)
		if e != nil {
			return e
		}
		target := ne.Target{Kind: "spatial_version", ID: version}
		if action == "versions.withdraw" || action == "assets.retire" || version == "" {
			target = ne.Target{Kind: "spatial_access", ID: project}
		}
		if e = access.NotificationReceiptTx(ctx, tx, "spatial", "spatial.published", action, actor, ne.BusinessScope(tenant, project), target, ne.Scoped(ids...)); e != nil {
			return e
		}
	}
	return nil
}
