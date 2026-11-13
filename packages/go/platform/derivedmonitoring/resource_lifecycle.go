package derivedmonitoring

import (
	"context"
	"strconv"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/jackc/pgx/v5"
)

const affectedResources = `WITH RECURSIVE affected AS(SELECT kind,id FROM engineering_resource_nodes WHERE tenant_id=$1 AND kind=$2 AND id=$3 UNION SELECT n.kind,n.id FROM engineering_resource_nodes n JOIN affected p ON n.parent_kind=p.kind AND n.parent_id=p.id WHERE n.tenant_id=$1)`

// Called under the host's common tenant coordination lock. This reads the full
// reference set but returns only a hash and qualified action flags, never hidden
// input names, IDs, counts or values to a project resource preview.
func (s *Service) ResourceImpactTx(ctx context.Context, tx pgx.Tx, scope resources.Scope, target resources.Target) (resources.ExternalImpact, error) {
	var out resources.ExternalImpact
	e := tx.QueryRow(ctx, affectedResources+`, deps AS(SELECT d.config_id,d.input_channel_id,c.channel_id,dc.control_state,dc.control_revision FROM derived_dependencies d JOIN derived_channel_configs c ON c.tenant_id=d.tenant_id AND c.id=d.config_id JOIN derived_channels dc ON dc.tenant_id=c.tenant_id AND dc.channel_id=c.channel_id WHERE d.tenant_id=$1 AND d.input_channel_id IN(SELECT id FROM affected WHERE kind='channel')), owned AS(SELECT dc.*,h.current_root,h.history_root FROM derived_channels dc JOIN derived_channel_heads h USING(tenant_id,channel_id) WHERE dc.tenant_id=$1 AND dc.channel_id IN(SELECT id FROM affected WHERE kind='channel')) SELECT md5(jsonb_build_object('dependencies',(SELECT jsonb_agg(to_jsonb(deps) ORDER BY config_id,input_channel_id) FROM deps),'owned',(SELECT jsonb_agg(to_jsonb(owned) ORDER BY channel_id) FROM owned))::text), EXISTS(SELECT 1 FROM deps) OR EXISTS(SELECT 1 FROM owned WHERE current_root IS NOT NULL OR history_root IS NOT NULL OR control_state<>'draft') OR ($2<>'channel' AND EXISTS(SELECT 1 FROM owned)), EXISTS(SELECT 1 FROM deps d WHERE d.control_state='enabled' AND (d.config_id=(SELECT draft_config_id FROM derived_channels WHERE tenant_id=$1 AND channel_id=d.channel_id) OR EXISTS(SELECT 1 FROM derived_config_assignments a WHERE a.tenant_id=$1 AND a.config_id=d.config_id AND a.superseded_revision IS NULL)))`, scope.Tenant, target.Kind, target.ID).Scan(&out.Hash, &out.Referenced, &out.ActiveDerivedDependents)
	if e != nil {
		return out, e
	}
	var analysisHash string
	var analysisReference bool
	e = tx.QueryRow(ctx, affectedResources+` SELECT md5(COALESCE(jsonb_agg(jsonb_build_array(version_id,input_key,channel_id) ORDER BY version_id,input_key)::text,'[]')),count(*)>0 FROM analysis_version_inputs WHERE tenant_id=$1 AND channel_id IN(SELECT id FROM affected WHERE kind='channel')`, scope.Tenant, target.Kind, target.ID).Scan(&analysisHash, &analysisReference)
	out.Hash = fingerprint([]string{out.Hash, analysisHash})
	out.Referenced = out.Referenced || analysisReference
	out.CalculationReferenced = out.Referenced
	return out, e
}

// Retirement is the existing engineering lifecycle. The calculation control
// fence advances atomically; resource restoration always leaves it paused.
func (s *Service) ResourceChangedTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope resources.Scope, before *resources.Resource, after resources.Resource, request, reason string, meta access.Meta) error {
	if before == nil || before.Status == after.Status && after.DeletedAt == nil {
		return nil
	}
	if after.SourceKind == "derived" {
		return s.recordResourceControlTx(ctx, tx, a, scope, after, request, reason, meta)
	}
	if after.Kind == resources.Channel {
		return nil
	}
	rows, e := tx.Query(ctx, affectedResources+` SELECT c.id::text,c.revision,c.status FROM engineering_channels c JOIN derived_channels d ON d.tenant_id=c.tenant_id AND d.channel_id=c.id WHERE c.tenant_id=$1 AND c.id IN(SELECT id FROM affected WHERE kind='channel') AND c.deleted_at IS NULL ORDER BY c.id`, scope.Tenant, after.Kind, after.ID)
	if e != nil {
		return e
	}
	children := []resources.Resource{}
	for rows.Next() {
		var child resources.Resource
		if e = rows.Scan(&child.ID, &child.Revision, &child.Status); e != nil {
			rows.Close()
			return e
		}
		child.SourceKind = "derived"
		children = append(children, child)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, child := range children {
		if after.Status == "retired" {
			child.Status = "retired"
		} else {
			var retired bool
			if e = tx.QueryRow(ctx, `WITH RECURSIVE ancestors AS(SELECT kind,id,parent_kind,parent_id,status FROM engineering_resource_nodes WHERE tenant_id=$1 AND kind='channel' AND id=$2 UNION ALL SELECT n.kind,n.id,n.parent_kind,n.parent_id,n.status FROM engineering_resource_nodes n JOIN ancestors a ON n.kind=a.parent_kind AND n.id=a.parent_id WHERE n.tenant_id=$1) SELECT EXISTS(SELECT 1 FROM ancestors WHERE status='retired')`, scope.Tenant, child.ID).Scan(&retired); e != nil {
				return e
			}
			if retired {
				continue
			}
			child.Status = "normal"
		}
		if e = s.recordResourceControlTx(ctx, tx, a, scope, child, request, reason, meta); e != nil {
			return e
		}
	}
	return nil
}
func (s *Service) recordResourceControlTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope resources.Scope, after resources.Resource, request, reason string, meta access.Meta) error {
	var oldState, revision, mandate string
	e := tx.QueryRow(ctx, `SELECT control_state,control_revision::text,mandate_revision::text FROM derived_channels WHERE tenant_id=$1 AND channel_id=$2 FOR UPDATE`, scope.Tenant, after.ID).Scan(&oldState, &revision, &mandate)
	if e != nil {
		return e
	}
	state, operation := "paused", "resource.restore"
	if after.Status == "retired" {
		state, operation = "retired", "resource.retire"
	}
	if after.DeletedAt != nil {
		if oldState != "draft" {
			return &resources.Error{Code: "calculation_in_use", Message: "仅从未启用的派生草稿可以删除。", Status: 409}
		}
		state, operation = "retired", "resource.delete"
	}
	var stop any
	effective := time.Now().UnixNano()
	if state == "retired" {
		stop = effective
	}
	var nextRevision, nextMandate string
	e = tx.QueryRow(ctx, `UPDATE derived_channels SET control_state=$3,control_revision=control_revision+1,mandate_revision=mandate_revision+1,stop_ns=CASE WHEN $3='retired' AND control_state='retired' THEN LEAST(stop_ns,$4) ELSE COALESCE($4,stop_ns) END,paused_at_ns=COALESCE(paused_at_ns,$5),health='waiting',blocked_reason='' WHERE tenant_id=$1 AND channel_id=$2 RETURNING control_revision::text,mandate_revision::text`, scope.Tenant, after.ID, state, stop, effective).Scan(&nextRevision, &nextMandate)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `UPDATE monitoring_jobs j SET state='cancelled',cancel_requested=true,fence=fence+1,lease_owner=NULL,lease_until=NULL,revision=j.revision+1,updated_at=clock_timestamp() FROM derived_runs r WHERE j.tenant_id=$1 AND r.tenant_id=j.tenant_id AND r.job_id=j.id AND r.channel_id=$2 AND r.mode='automatic' AND j.state IN ('queued','running','retry_wait','needs_action')`, scope.Tenant, after.ID)
	if e != nil {
		return e
	}
	var position int64
	if e = tx.QueryRow(ctx, `INSERT INTO monitoring_tenant_states(tenant_id,commit_sequence) VALUES($1,1) ON CONFLICT(tenant_id) DO UPDATE SET commit_sequence=monitoring_tenant_states.commit_sequence+1 RETURNING commit_sequence`, scope.Tenant).Scan(&position); e != nil {
		return e
	}
	if _, e = monitoring.AppendChangeTx(ctx, tx, scope.Tenant, position, "channel_control_change", "derived", after.ID, []monitoring.ChangeRange{{Channel: after.ID, From: &effective}}); e != nil {
		return e
	}
	prior := map[string]string{"control_state": oldState, "control_revision": revision, "mandate_revision": mandate}
	var stopText any
	if value, ok := stop.(int64); ok {
		stopText = strconv.FormatInt(value, 10)
	}
	next := map[string]any{"control_state": state, "control_revision": nextRevision, "mandate_revision": nextMandate, "resource_revision": strconv.Itoa(after.Revision), "stop_ns": stopText}
	_, e = tx.Exec(ctx, `INSERT INTO derived_changes(tenant_id,project_id,actor_kind,actor_id,operation,target_kind,target_id,request_id,reason,before,after) VALUES($1,$2,'user',$3,$4,'channel',$5,$6,$7,$8,$9)`, scope.Tenant, nullable(scope.Project), a.ID, operation, after.ID, request, reason, jsonBytes(prior), jsonBytes(next))
	if e != nil {
		return e
	}
	meta.ActorID = a.ID
	meta.ActorScope = scope.Entry
	meta.TenantID = scope.Tenant
	meta.ProjectID = scope.Project
	meta.Reason = reason
	return access.Audit(ctx, tx, "derived."+operation, "derived_channel", after.ID, "", prior, next, meta)
}
