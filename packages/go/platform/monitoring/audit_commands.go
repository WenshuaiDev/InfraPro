package monitoring

import (
	"context"
	"encoding/json"
	"strings"

	ar "github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	"github.com/jackc/pgx/v5"
)

// The native receipt is the command boundary. Capture only source-owned
// management fields before it changes, never mappings, samples or raw input.
func auditBeforeCommandTx(ctx context.Context, tx pgx.Tx, auth authorization, operation string) error {
	action, id, _ := strings.Cut(operation, ":")
	var before any
	if id != "" && (strings.HasPrefix(action, "imports.") || action == "upload.complete" || action == "batches.revoke") {
		var raw []byte
		e := tx.QueryRow(ctx, `SELECT jsonb_build_object('id',id,'name',original_name,'state',state,'revision',revision::text,'mapping_revision',mapping_revision::text) FROM monitoring_import_batches WHERE tenant_id=$1 AND id::text=$2`, auth.Tenant, id).Scan(&raw)
		if e != nil && e != pgx.ErrNoRows {
			return e
		}
		if e == nil {
			if e = json.Unmarshal(raw, &before); e != nil {
				return e
			}
		}
	}
	_, e := tx.Exec(ctx, `SELECT set_config('infrapro.monitoring_audit_before',$1,true)`, string(ar.JSON(before)))
	return e
}

func auditCommandTx(ctx context.Context, tx pgx.Tx, auth authorization, operation, request string, payload, result any) error {
	action, _, _ := strings.Cut(operation, ":")
	if strings.HasPrefix(action, "access.") || action == "task.control" {
		return nil
	} // already emitted by their canonical producer
	var out, input map[string]any
	if e := json.Unmarshal(ar.JSON(result), &out); e != nil {
		return e
	}
	if e := json.Unmarshal(ar.JSON(payload), &input); e != nil {
		return e
	}
	id, _ := out["id"].(string)
	if id == "" {
		_, id, _ = strings.Cut(operation, ":")
	}
	kind := "monitoring_batch"
	if strings.HasPrefix(action, "exports.") {
		kind = "monitoring_export"
	}
	if strings.HasPrefix(action, "jobs.") {
		kind = "monitoring_job"
	}
	var before any
	var raw string
	if e := tx.QueryRow(ctx, `SELECT COALESCE(NULLIF(current_setting('infrapro.monitoring_audit_before',true),''),'null')`).Scan(&raw); e != nil {
		return e
	}
	if e := json.Unmarshal([]byte(raw), &before); e != nil {
		return e
	}
	reason, _ := input["reason"].(string)
	links := []ar.Link{}
	job, _ := out["job_id"].(string)
	if kind == "monitoring_job" {
		job = id
	}
	if job != "" {
		var generation string
		if e := tx.QueryRow(ctx, `SELECT notification_generation::text FROM monitoring_jobs WHERE tenant_id=$1 AND id=$2`, auth.Tenant, job).Scan(&generation); e != nil {
			return e
		}
		links = append(links, ar.Link{Kind: "task_execution", Module: "monitoring", TargetKind: "job", TargetID: job, Generation: generation, Relation: "execution", Scope: ar.Scope{Kind: auth.Entry, Tenant: auth.Tenant, Project: auth.Project}})
	}
	_, e := ar.AppendLegacyTx(ctx, tx, ar.LegacyInput{Event: "monitoring." + action, ObjectType: kind, ObjectID: id, Before: before, After: out, ActorID: auth.Account, ActorType: "account", Tenant: auth.Tenant, Project: auth.Project, Entry: auth.Entry, Reason: reason, RequestID: request, Links: links})
	return e
}

func auditPublishedTx(ctx context.Context, tx pgx.Tx, auth authorization, id, job, generation, reason string, summary any) error {
	_, e := ar.AppendLegacyTx(ctx, tx, ar.LegacyInput{Event: "monitoring.imports.published", ObjectType: "monitoring_batch", ObjectID: id, After: summary, ActorID: auth.Account, ActorType: "worker", Tenant: auth.Tenant, Project: auth.Project, Entry: auth.Entry, Reason: reason, ExecutorKind: "monitoring_worker", ExecutorRef: "monitoring", Links: []ar.Link{{Kind: "task_execution", Module: "monitoring", TargetKind: "job", TargetID: job, Generation: generation, Relation: "execution", Scope: ar.Scope{Kind: auth.Entry, Tenant: auth.Tenant, Project: auth.Project}}}})
	return e
}
