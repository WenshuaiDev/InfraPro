package display

import (
	"context"
	ar "github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	"github.com/jackc/pgx/v5"
	"time"
)

func RetryAuditHandoffTx(ctx context.Context, tx pgx.Tx, id string, attempts int) (bool, error) {
	tag, e := tx.Exec(ctx, `UPDATE display_audit_handoffs SET attempts=0,last_error_code='' WHERE id=$1 AND translated_at IS NULL AND attempts=$2 AND attempts>=8`, id, attempts)
	return tag.RowsAffected() == 1, e
}

// BridgeAuditTx runs in the trusted metadata process. Restricted packagers only
// write domain handoffs through their source transaction's trigger.
func BridgeAuditTx(ctx context.Context, tx pgx.Tx, limit int) (int, error) {
	if limit < 1 || limit > 100 {
		return 0, ar.ErrInvalid
	}
	rows, e := tx.Query(ctx, `SELECT h.id::text,h.source_key,h.job_id::text,h.generation::text,h.fence::text,h.tenant_id::text,h.project_id::text,h.instance_id::text,h.state,h.safe_error_code,h.occurred_at,u.actor_id::text,COALESCE((SELECT snapshot->>'requested_by' FROM display_task_handoffs t WHERE t.job_id=h.job_id AND t.execution_generation=h.generation ORDER BY task_revision LIMIT 1),u.actor_id::text) FROM display_audit_handoffs h JOIN display_package_jobs j ON j.id=h.job_id JOIN display_uploads u ON u.id=j.upload_id AND u.tenant_id=h.tenant_id AND u.project_id=h.project_id AND u.instance_id=h.instance_id WHERE h.translated_at IS NULL AND h.attempts<8 ORDER BY h.occurred_at,h.id LIMIT $1 FOR UPDATE OF h SKIP LOCKED`, limit)
	if e != nil {
		return 0, e
	}
	type item struct {
		id, key, job, generation, fence, tenant, project, instance, state, code, owner, requested string
		at                                                                                        time.Time
	}
	items := []item{}
	for rows.Next() {
		var v item
		if e = rows.Scan(&v.id, &v.key, &v.job, &v.generation, &v.fence, &v.tenant, &v.project, &v.instance, &v.state, &v.code, &v.at, &v.owner, &v.requested); e != nil {
			rows.Close()
			return 0, e
		}
		items = append(items, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return 0, e
	}
	count := 0
	for _, v := range items {
		fact := ar.Fact{Module: "display", SourceKey: "handoff:" + v.key, Effect: "execution", Event: "display.execution." + v.state, Scope: ar.Scope{Kind: "project", Tenant: v.tenant, Project: v.project}, ActorType: "worker", Initiator: v.owner, RequestedBy: v.requested, ExecutorKind: "package_worker", ExecutorRef: "display-package", RequestKind: "worker", ObjectType: "display", ObjectID: v.instance, ObjectRevision: v.generation, Outcome: "success", ErrorCode: v.code, OccurredAt: v.at, Quality: "source_handoff", Changes: ar.Diff(nil, map[string]string{"state": v.state, "generation": v.generation}, false, true), Links: []ar.Link{{Kind: "task_execution", Module: "display", TargetKind: "job", TargetID: v.job, Generation: v.generation, Relation: "execution", Scope: ar.Scope{Kind: "project", Tenant: v.tenant, Project: v.project}}}}
		if v.state == "succeeded" {
			var packageID string
			err := tx.QueryRow(ctx, `SELECT id::text FROM display_packages WHERE job_id=$1 AND tenant_id=$2 AND project_id=$3 AND instance_id=$4`, v.job, v.tenant, v.project, v.instance).Scan(&packageID)
			if err != nil && err != pgx.ErrNoRows {
				return count, err
			}
			if err == nil {
				fact.Links = append(fact.Links, ar.Link{Kind: "business", Relation: "output", Module: "display", TargetKind: "display_package", TargetID: packageID, Scope: fact.Scope})
			}
		}
		if v.state == "accepted" {
			fact.ActorType = "account"
			fact.ActorID = v.requested
			fact.RequestKind = "command_legacy"
			fact.ExecutorKind = ""
			fact.ExecutorRef = ""
		}
		if v.state == "failed" {
			fact.Outcome = "failure"
		}
		if v.state == "cancelled" {
			fact.Outcome = "cancelled"
		}
		save, e := tx.Begin(ctx)
		if e != nil {
			return count, e
		}
		_, e = ar.AppendTx(ctx, save, fact)
		if e == nil {
			_, e = save.Exec(ctx, `UPDATE display_audit_handoffs SET translated_at=clock_timestamp(),last_error_code='' WHERE id=$1`, v.id)
		}
		if e != nil {
			if rollback := save.Rollback(ctx); rollback != nil {
				return count, rollback
			}
			_, e = tx.Exec(ctx, `UPDATE display_audit_handoffs SET attempts=attempts+1,last_error_code='audit_handoff_unavailable' WHERE id=$1`, v.id)
			if e != nil {
				return count, e
			}
			continue
		}
		if e = save.Commit(ctx); e != nil {
			return count, e
		}
		count++
	}
	_, e = tx.Exec(ctx, `INSERT INTO audit_source_states(source_module,source_kind,complete,pending_count,observed_at) SELECT 'display','package',NOT EXISTS(SELECT 1 FROM display_audit_handoffs WHERE translated_at IS NULL),count(*),clock_timestamp() FROM display_audit_handoffs WHERE translated_at IS NULL ON CONFLICT(source_module,source_kind) DO UPDATE SET complete=EXCLUDED.complete,pending_count=EXCLUDED.pending_count,observed_at=EXCLUDED.observed_at,error_code=CASE WHEN EXCLUDED.complete THEN '' ELSE 'pending_handoff' END`)
	return count, e
}
