package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

// The worker checks one due tenant per minute. Daily checks use durable jobs,
// not an in-memory timer as evidence of completion. New data can be verified
// immediately through the operator command; scheduled scans start after a day.
func (s *Service) scheduleVerification(ctx context.Context) error {
	var tenant, watermark string
	e := s.db(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT t.tenant_id::text,t.commit_sequence::text FROM monitoring_tenant_states t WHERE EXISTS(SELECT 1 FROM monitoring_environment_state WHERE id=1 AND mode='ready') AND EXISTS(SELECT 1 FROM monitoring_publications p WHERE p.tenant_id=t.tenant_id AND p.committed_at<clock_timestamp()-INTERVAL '24 hours') AND NOT EXISTS(SELECT 1 FROM monitoring_jobs j WHERE j.tenant_id=t.tenant_id AND j.kind='verify' AND (j.created_at>clock_timestamp()-INTERVAL '24 hours' OR j.state IN ('queued','running','retry_wait'))) ORDER BY t.tenant_id LIMIT 1`).Scan(&tenant, &watermark)
	})
	if e == pgx.ErrNoRows {
		return nil
	}
	if e != nil {
		return e
	}
	_, e = s.QueueVerification(ctx, tenant, watermark, "周期完整性对账 "+time.Now().UTC().Format("2006-01-02"))
	return e
}

func (s *Service) QueueVerification(ctx context.Context, tenant, watermark, reason string) (string, error) {
	if !access.ValidID(tenant) {
		return "", fmt.Errorf("tenant is required")
	}
	if e := access.ValidReason(reason); e != nil {
		return "", e
	}
	if _, e := revisionNumber(watermark); e != nil {
		return "", e
	}
	var job string
	e := s.operatorTransaction(ctx, tenant, func(tx pgx.Tx) error {
		var current string
		if e := tx.QueryRow(ctx, `SELECT commit_sequence::text FROM monitoring_tenant_states WHERE tenant_id=$1 FOR UPDATE`, tenant).Scan(&current); e != nil {
			return e
		}
		if current != watermark {
			return revisionChanged()
		}
		input := map[string]string{"watermark": watermark, "reason": reason}
		if e := tx.QueryRow(ctx, `INSERT INTO monitoring_jobs(tenant_id,kind,target_id,target_revision,input_hash,input) VALUES($1,'verify',$1,$2,$3,$4) ON CONFLICT(tenant_id,kind,target_id,target_revision,input_hash) DO UPDATE SET input_hash=EXCLUDED.input_hash RETURNING id::text`, tenant, watermark, digest(input), encodeJSON(input)).Scan(&job); e != nil {
			return e
		}
		return operatorAuditTx(ctx, tx, tenant, "operator.verify.queued", job, reason, input)
	})
	return job, e
}
func (s *Service) verifyJob(ctx context.Context, j Job) error {
	if j.actor != "" {
		return access.ErrDenied
	}
	var in map[string]string
	if e := json.Unmarshal(j.input, &in); e != nil {
		return e
	}
	report, e := s.Verify(ctx, j.tenant)
	if e != nil {
		return e
	}
	// Only a frozen affected channel can be quarantined by this result. A new
	// publication or another repair invalidates the finding before applying it.
	affected := map[string]bool{}
	for _, v := range report.Findings {
		if v.Channel != "" {
			affected[v.Channel] = true
		}
	}
	repairs := []string{}
	for _, p := range report.checkpoints {
		if !affected[p.Channel] {
			continue
		}
		if e = s.jobTransaction(ctx, j, func(tx pgx.Tx) error { return assertJobTx(ctx, tx, j) }); e != nil {
			return e
		}
		id, e := s.QueueSummaryRepair(ctx, p, "完整性对账修复："+in["reason"])
		if e != nil {
			return e
		}
		repairs = append(repairs, id)
	}
	if s.calculationMaintenance != nil {
		report.Calculations, e = s.calculationMaintenance.MaintainCalculations(ctx, TaskLease{s, j})
		if e != nil {
			return e
		}
		var calculations struct {
			State string `json:"state"`
		}
		if e = json.Unmarshal(report.Calculations, &calculations); e != nil {
			return e
		}
		if calculations.State == "failed" {
			report.State = "failed"
			report.Findings = append(report.Findings, VerificationFinding{Code: "calculation_result_integrity_unavailable"})
		}
	}
	if e = s.pruneExpiredExports(ctx, TaskLease{s, j}); e != nil {
		return e
	}
	key, e := s.storage.putSummary(ctx, j.tenant, report)
	if e != nil {
		return e
	}
	return s.jobTransaction(ctx, j, func(tx pgx.Tx) error {
		if e := assertJobTx(ctx, tx, j); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `INSERT INTO monitoring_job_steps(tenant_id,job_id,step_key,fence,state,result) VALUES($1,$2,'verification-result',$3,'completed',$4) ON CONFLICT(tenant_id,job_id,step_key) DO UPDATE SET fence=EXCLUDED.fence,state='completed',result=EXCLUDED.result,updated_at=clock_timestamp()`, j.tenant, j.ID, j.fence, encodeJSON(map[string]any{"object": key, "repair_jobs": repairs})); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, `UPDATE monitoring_jobs SET progress=$2 WHERE id=$1`, j.ID, encodeJSON(map[string]string{"phase": report.State, "channels": report.Channels, "chunks": report.Chunks, "rows": report.Rows, "findings": countText(int64(len(report.Findings)))}))
		return e
	})
}
func (s *Service) OperatorJobReport(ctx context.Context, tenant, id string) (VerificationReport, error) {
	var report VerificationReport
	var key string
	e := s.db(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT result->>'object' FROM monitoring_job_steps WHERE tenant_id=$1 AND job_id=$2 AND step_key='verification-result' AND state='completed'`, tenant, id).Scan(&key)
	})
	if e == nil {
		e = s.storage.summaryObject(ctx, key, &report, true)
	}
	return report, e
}
