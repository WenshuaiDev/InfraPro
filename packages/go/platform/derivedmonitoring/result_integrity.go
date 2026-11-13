package derivedmonitoring

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/jackc/pgx/v5"
	"github.com/minio/minio-go/v7"
)

func requireResultAvailableTx(ctx context.Context, tx pgx.Tx, tenant, kind, id string) error {
	var unavailable bool
	if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM derived_result_integrity WHERE tenant_id=$1 AND result_kind=$2 AND result_id=$3 AND state='unavailable')`, tenant, kind, id).Scan(&unavailable); e != nil {
		return e
	}
	if unavailable {
		return resultUnavailableError()
	}
	return nil
}

// The shared monitoring reader preserves its own public error type across
// detail, analytic and export paths. Batch and single-result checks must use
// the same contract, including when an analytic cache has been invalidated.
func resultUnavailableError() *monitoring.Error {
	return &monitoring.Error{Code: "result_integrity_unavailable", Message: "该结果的正式载荷或依据不完整，已停止读取；请查看核验任务并重新计算发布。", Status: 503}
}

// Transport outages remain retryable. Only proven missing/corrupt evidence
// changes persisted availability; cancellation cannot quarantine a result.
func provenResultCorruption(e error) bool {
	if e == nil || errors.Is(e, context.Canceled) || errors.Is(e, context.DeadlineExceeded) {
		return false
	}
	if errors.Is(e, pgx.ErrNoRows) {
		return true
	}
	code := minio.ToErrorResponse(e).Code
	if code == "NoSuchKey" || code == "NoSuchObject" || code == "NoSuchBucket" {
		return true
	}
	var syntax *json.SyntaxError
	var value *json.UnmarshalTypeError
	if errors.As(e, &syntax) || errors.As(e, &value) {
		return true
	}
	return slices.Contains([]string{"manifest_hash_mismatch", "generation_manifest_mismatch", "manifest_version_mismatch", "run_plan_mismatch", "manifest_snapshot_hash_mismatch", "lineage_identity_mismatch", "result_attempt_mismatch", "result_payload_owner_mismatch", "result_totals_mismatch", "status_summary_mismatch", "result_payload_mismatch", "result_block_index_mismatch", "result_summary_mismatch", "result_endpoint_index_mismatch", "result_block_incomplete", "result_object_incomplete"}, e.Error()) || strings.HasPrefix(e.Error(), "invalid_result_")
}

type integrityCheckpoint struct {
	Table  int            `json:"table"`
	After  string         `json:"after_id"`
	Report Reconciliation `json:"report"`
}

func (s *Service) MaintainCalculations(ctx context.Context, lease monitoring.TaskLease) (json.RawMessage, error) {
	task := lease.Task()
	if task.Job.Kind != "verify" || task.ActorID != "" || task.Scope.Project != "" || task.Job.Target != task.Scope.Tenant {
		return nil, access.ErrDenied
	}
	state := integrityCheckpoint{Report: Reconciliation{State: "passed", Started: time.Now().UTC(), Findings: []ReconciliationFinding{}}}
	saved, found, e := lease.Checkpoint(ctx, "derived-integrity")
	if e != nil {
		return nil, e
	}
	if found {
		if e = json.Unmarshal(saved.Result, &state); e != nil {
			return nil, e
		}
		if state.Table < 0 || state.Table > 2 || state.After != "" && !access.ValidID(state.After) {
			return nil, problem("result_integrity_unavailable", "核验检查点不完整。", 503)
		}
	}
	r := &Recovery{store: s.results}
	tables := []string{"derived_generations", "analysis_artifact_versions"}
	for state.Table < len(tables) {
		table := tables[state.Table]
		var ids []string
		e = lease.Transaction(ctx, func(tx pgx.Tx) error {
			rows, e := tx.Query(ctx, `SELECT id::text FROM `+table+` WHERE tenant_id=$1 AND ($2='' OR id::text>$2) ORDER BY id::text LIMIT 100`, task.Scope.Tenant, state.After)
			if e != nil {
				return e
			}
			defer rows.Close()
			for rows.Next() {
				var id string
				if e = rows.Scan(&id); e != nil {
					return e
				}
				ids = append(ids, id)
			}
			return rows.Err()
		})
		if e != nil {
			return nil, e
		}
		for _, id := range ids {
			if e = lease.Check(ctx); e != nil {
				return nil, e
			}
			if s.results == nil {
				return nil, problem("calculation_storage_unavailable", "派生结果存储不可用。", 503)
			}
			var piece Reconciliation
			checkError := s.access.Transact(ctx, func(tx pgx.Tx) error { return r.verifyPublished(ctx, tx, task.Scope.Tenant, id, table, &piece) })
			if checkError != nil && !provenResultCorruption(checkError) {
				return nil, checkError
			}
			status := "available"
			if checkError != nil {
				status = "unavailable"
			}
			if e = lease.Transaction(ctx, func(tx pgx.Tx) error {
				var previous string
				e := tx.QueryRow(ctx, `SELECT state FROM derived_result_integrity WHERE tenant_id=$1 AND result_kind=$2 AND result_id=$3 FOR UPDATE`, task.Scope.Tenant, table, id).Scan(&previous)
				if e != nil && e != pgx.ErrNoRows {
					return e
				}
				if _, e = tx.Exec(ctx, `INSERT INTO derived_result_integrity(tenant_id,result_kind,result_id,state,job_id) VALUES($1,$2,$3,$4,$5) ON CONFLICT(tenant_id,result_kind,result_id) DO UPDATE SET state=EXCLUDED.state,job_id=EXCLUDED.job_id,checked_at=clock_timestamp()`, task.Scope.Tenant, table, id, status, task.Job.ID); e != nil {
					return e
				}
				if previous != status && (previous != "" || status == "unavailable") {
					return access.Audit(ctx, tx, "derived.results.integrity", table, id, "", map[string]string{"state": previous}, map[string]string{"state": status, "job_id": task.Job.ID}, access.Meta{ActorType: "system", ActorScope: "tenant", TenantID: task.Scope.Tenant, Reason: "共享监测核验任务确认不可变结果完整性"})
				}
				return nil
			}); e != nil {
				return nil, e
			}
			state.Report.Generations += piece.Generations
			state.Report.Artifacts += piece.Artifacts
			state.Report.Blocks += piece.Blocks
			state.Report.Rows += piece.Rows
			state.Report.Bytes += piece.Bytes
			if checkError != nil {
				state.Report.State = "failed"
				state.Report.FindingCount++
				if len(state.Report.Findings) < 100 {
					state.Report.Findings = append(state.Report.Findings, ReconciliationFinding{table + "_integrity:" + integrityReason(checkError), task.Scope.Tenant, id})
				}
			}
			state.After = id
			if e = lease.SaveCheckpoint(ctx, "derived-integrity", "running", state); e != nil {
				return nil, e
			}
		}
		if len(ids) < 100 {
			state.Table++
			state.After = ""
		}
	}
	state.Report.Completed = time.Now().UTC()
	if e = lease.SaveCheckpoint(ctx, "derived-integrity", "completed", state); e != nil {
		return nil, e
	}
	if e = s.retainCompletedCalculations(ctx, lease); e != nil {
		return nil, e
	}
	return jsonBytes(state.Report), nil
}

// Only a normalized code is recorded; transport messages can contain private
// endpoints or keys and must never become public diagnostics.
func integrityReason(e error) string {
	if errors.Is(e, pgx.ErrNoRows) {
		return "metadata_missing"
	}
	if code := minio.ToErrorResponse(e).Code; code != "" {
		return code
	}
	var syntax *json.SyntaxError
	var value *json.UnmarshalTypeError
	if errors.As(e, &syntax) || errors.As(e, &value) {
		return "invalid_json"
	}
	if provenResultCorruption(e) {
		return e.Error()
	}
	return "unknown"
}
