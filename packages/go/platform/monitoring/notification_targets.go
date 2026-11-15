package monitoring

import (
	"context"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	ne "github.com/WenshuaiDev/InfraPro/packages/go/platform/notificationevent"
	"github.com/jackc/pgx/v5"
)

func notificationRestricted(e error) bool {
	var ae *access.Error
	var me *Error
	return errors.As(e, &ae) && (ae.Status == 403 || ae.Status == 404) || errors.As(e, &me) && (me.Status == 403 || me.Status == 404 || me.Code == "snapshot_expired")
}

// NotificationTargetsTx batches metadata and reuses the job's original current
// scope/object authorization, including derived executors and export snapshots.
func (s *Service) NotificationTargetsTx(ctx context.Context, tx pgx.Tx, account string, scope Scope, kind string, ids []string) (map[string]ne.TargetContext, error) {
	out := map[string]ne.TargetContext{}
	var rev string
	if e := tx.QueryRow(ctx, `SELECT revision::text FROM authorization_state WHERE id=1`).Scan(&rev); e != nil {
		return nil, e
	}
	auth := authorization{Scope: scope, Account: account, Revision: rev, ResourceRevision: "0", DataRevision: "0"}
	if scope.Project != "" {
		e := tx.QueryRow(ctx, `SELECT r.revision::text,COALESCE(m.revision,0)::text FROM project_resource_policies r LEFT JOIN monitoring_access_states m USING(tenant_id,project_id) WHERE r.tenant_id=$1 AND r.project_id=$2`, scope.Tenant, scope.Project).Scan(&auth.ResourceRevision, &auth.DataRevision)
		if e != nil {
			return nil, e
		}
	}
	if kind == "monitoring_import" {
		for _, id := range ids {
			b, e := batchTx(ctx, tx, auth, id, false)
			if notificationRestricted(e) {
				continue
			}
			if e != nil {
				return nil, e
			}
			op := access.MonitoringOperation(scope.Entry, "imports.detail")
			if scope.Project != "" {
				_, e = access.RequireProjectTx(ctx, tx, account, op, scope.Tenant, scope.Project)
			} else {
				e = access.RequireScopedTx(ctx, tx, account, op, scope.Tenant)
			}
			if notificationRestricted(e) {
				continue
			}
			if e != nil {
				return nil, e
			}
			out[id] = ne.TargetContext{Allowed: true, Available: true, State: b.State, Name: b.Name}
		}
		return out, nil
	}
	rows, e := tx.Query(ctx, jobSelect+` WHERE tenant_id=$1 AND id::text=ANY($2::text[])`, scope.Tenant, ids)
	if e != nil {
		return nil, e
	}
	jobs := []Job{}
	for rows.Next() {
		j, e := scanJob(rows)
		if e != nil {
			rows.Close()
			return nil, e
		}
		jobs = append(jobs, j)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	for _, j := range jobs {
		if j.project != scope.Project {
			continue
		}
		e = s.jobPermissionTx(ctx, tx, auth, j)
		if notificationRestricted(e) {
			continue
		}
		if e != nil {
			return nil, e
		}
		if e = presentJobTx(ctx, tx, &j); e != nil {
			return nil, e
		}
		label := map[string]string{"export": "监测数据导出", "derive": "派生计算／试算", "analysis": "历史分析／试算", "analysis_compare": "成果比较", "derived_replay": "依据回放", "analysis_export": "分析成果导出", "derived_evidence_export": "派生依据导出"}[j.Kind]
		v := ne.TargetContext{Allowed: true, Available: true, State: j.State, Name: label + " · " + j.ID[:8]}
		if j.Kind == "export" {
			var expired bool
			if e = tx.QueryRow(ctx, `SELECT expires_at<=clock_timestamp() OR invalidated_at IS NOT NULL FROM monitoring_exports WHERE job_id=$1`, j.ID).Scan(&expired); e != nil {
				return nil, e
			}
			if expired {
				v.Available = false
				v.Reason = "result_expired"
				v.State = "expired"
			}
		}
		out[j.ID] = v
	}
	return out, nil
}
