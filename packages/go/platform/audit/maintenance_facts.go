package audit

import (
	"context"
	ar "github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	"github.com/jackc/pgx/v5"
)

func maintenanceFactTx(ctx context.Context, tx pgx.Tx, action, key, reason string, summary any) error {
	safe, e := ar.Snapshot(summary, []string{"kind", "mode", "plan_id", "hold_id", "deleted", "protected", "failed", "classified", "unclassified", "candidates", "active_holds", "policy_applied", "source", "platform_seconds", "tenant_seconds", "project_seconds"})
	if e != nil {
		return e
	}
	_, e = ar.AppendTx(ctx, tx, ar.Fact{Module: "audit", Event: "audit.maintenance." + action, SourceKey: key, Effect: action, Scope: Scope{Kind: "platform"}, ActorType: "maintenance", ExecutorKind: "maintenance", ExecutorRef: "infrapro-audit", ObjectType: "audit_maintenance", ObjectID: key, RequestID: key, RequestKind: "cli", Reason: safeReason(reason), Outcome: "success", Changes: ar.Diff(nil, safe, false, true)})
	return e
}
