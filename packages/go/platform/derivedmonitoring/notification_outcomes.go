package derivedmonitoring

import (
	"context"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	ne "github.com/WenshuaiDev/InfraPro/packages/go/platform/notificationevent"
	"github.com/jackc/pgx/v5"
	"slices"
)

func (s *Service) TaskOutcomeTx(ctx context.Context, tx pgx.Tx, task monitoring.Task, state, code string) error {
	if task.ActorID != "" || task.Job.Kind != "derive" || state == "cancelled" {
		return nil
	}
	var channel, config, control, mandate string
	var current bool
	e := tx.QueryRow(ctx, `SELECT r.channel_id::text,r.config_revision::text,r.control_revision::text,r.mandate_revision::text,c.control_state='enabled' AND c.config_revision=r.config_revision AND c.control_revision=r.control_revision AND c.mandate_revision=r.mandate_revision FROM derived_runs r JOIN derived_channels c ON c.tenant_id=r.tenant_id AND c.channel_id=r.channel_id WHERE r.tenant_id=$1 AND r.job_id=$2 AND r.mode='automatic'`, task.Scope.Tenant, task.Job.ID).Scan(&channel, &config, &control, &mandate, &current)
	if e == pgx.ErrNoRows {
		return nil
	}
	if e != nil || !current {
		return e
	}
	if state != "failed" && state != "needs_action" && state != "succeeded" {
		return nil
	}
	ids, e := access.NotificationRecipientsTx(ctx, tx, task.Scope.Tenant, "", access.TenantDerivedMetricsManage)
	if e != nil {
		return e
	}
	prefix := fmt.Sprintf("%s:%s:%s:%s:%s:", task.Scope.Tenant, channel, config, control, mandate)
	event := ne.Event{Module: "derivedmonitoring", Type: "derived.condition", Scope: ne.BusinessScope(task.Scope.Tenant, ""), Target: ne.Target{Kind: "derived_channel", ID: channel}, Recipients: ne.Scoped(ids...), Reason: code}
	if state != "succeeded" {
		if code == "" {
			code = "execution_failed"
		}
		return ne.ConditionTx(ctx, tx, prefix+code, true, event)
	}
	rows, e := tx.Query(ctx, `SELECT condition_key FROM notification_conditions WHERE source_module='derivedmonitoring' AND starts_with(condition_key,$1) AND active ORDER BY condition_key FOR UPDATE`, prefix)
	if e != nil {
		return e
	}
	keys := []string{}
	for rows.Next() {
		var key string
		if e = rows.Scan(&key); e != nil {
			rows.Close()
			return e
		}
		keys = append(keys, key)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, key := range keys {
		if e = ne.ConditionTx(ctx, tx, key, false, event); e != nil {
			return e
		}
	}
	return nil
}

func (s *Service) NotificationChannelsTx(ctx context.Context, tx pgx.Tx, account, tenant string, ids []string) (map[string]ne.TargetContext, error) {
	out := map[string]ne.TargetContext{}
	_, snap, e := access.TenantSnapshotTx(ctx, tx, account, tenant, "")
	if e != nil {
		return nil, e
	}
	if !slices.Contains(snap.Permissions, access.TenantDerivedMetricsManage) {
		return out, nil
	}
	rows, e := tx.Query(ctx, `SELECT channel_id::text,health FROM derived_channels WHERE tenant_id=$1 AND channel_id::text=ANY($2::text[])`, tenant, ids)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var id, state string
		if e = rows.Scan(&id, &state); e != nil {
			return nil, e
		}
		out[id] = ne.TargetContext{Allowed: true, Available: true, State: state}
	}
	return out, rows.Err()
}
