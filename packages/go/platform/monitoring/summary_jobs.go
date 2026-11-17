package monitoring

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicecoord"
	"github.com/jackc/pgx/v5"
)

type summaryJobInput struct {
	Repair    *SummaryRepairPlan `json:"repair,omitempty"`
	Reason    string             `json:"reason,omitempty"`
	Mode      string             `json:"mode"`
	Action    string             `json:"action,omitempty"`
	Query     QueryInput         `json:"query,omitempty"`
	Channel   string             `json:"channel,omitempty"`
	Content   string             `json:"content,omitempty"`
	Revoke    string             `json:"revoke,omitempty"`
	Watermark string             `json:"watermark,omitempty"`
}

func enqueueSummariesTx(ctx context.Context, tx pgx.Tx, tenant string, channels []string, watermark int64) error {
	rows, e := tx.Query(ctx, `SELECT channel_id::text,content_revision::text,revoke_revision::text FROM monitoring_channel_versions WHERE tenant_id=$1 AND channel_id=ANY($2::uuid[]) ORDER BY channel_id`, tenant, channels)
	if e != nil {
		return e
	}
	inputs := []summaryJobInput{}
	for rows.Next() {
		v := summaryJobInput{Mode: "maintenance", Watermark: fmt.Sprint(watermark)}
		if e = rows.Scan(&v.Channel, &v.Content, &v.Revoke); e != nil {
			rows.Close()
			return e
		}
		inputs = append(inputs, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, v := range inputs {
		if _, e = tx.Exec(ctx, `INSERT INTO monitoring_jobs(tenant_id,kind,target_id,target_revision,input_hash,input) VALUES($1,'summary',$2,$3,$4,$5) ON CONFLICT DO NOTHING`, tenant, v.Channel, v.Content, digest(v), encodeJSON(v)); e != nil {
			return e
		}
	}
	return nil
}
func (s *Service) jobTransaction(ctx context.Context, j Job, fn func(pgx.Tx) error) error {
	if executor := s.executors[j.Kind]; executor != nil {
		return s.executionTransaction(ctx, j, executor, fn)
	}
	if j.actor != "" || j.subject != "" {
		a, e := s.jobActor(ctx, j)
		if e != nil {
			return e
		}
		return s.transact(ctx, a, j.scope(), j.action(), false, false, func(tx pgx.Tx, _ authorization) error { return fn(tx) })
	}
	if j.Kind != "summary" && j.Kind != "relocate" && j.Kind != "verify" {
		return access.ErrDenied
	}
	// System derivation has no former user's authority to borrow. It can maintain
	// published data after membership changes, but cannot publish observations.
	return s.db(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, false); e != nil {
			return e
		}
		if e := devicecoord.Catalog(ctx, tx, false); e != nil {
			return e
		}
		if e := devicecoord.Tenant(ctx, tx, j.tenant, false); e != nil {
			return e
		}
		return fn(tx)
	})
}
func (s *Service) queueAnalytic(ctx context.Context, a access.Actor, scope Scope, action string, in QueryInput, plan queryPlan) (AnalyticResult, error) {
	v := summaryJobInput{Mode: "query", Action: action, Query: in}
	out := AnalyticResult{State: "pending", Snapshot: plan.snapshot, Computation: "background"}
	e := s.transact(ctx, a, scope, action, false, false, func(tx pgx.Tx, auth authorization) error {
		if authorizationVersion(auth) != plan.authorization {
			return revisionChanged()
		}
		if _, e := authorizedWindowsTx(ctx, tx, auth, plan.channels, "read"); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, `INSERT INTO monitoring_jobs(tenant_id,project_id,actor_id,display_subject_id,kind,target_id,target_revision,input_hash,input) VALUES($1,$2,$3,$8,'summary',$4,$5,$6,$7) ON CONFLICT DO NOTHING`, scope.Tenant, nullable(scope.Project), a.OwnerAccount(), plan.snapshot.ID, plan.snapshot.Watermark, digest(v), encodeJSON(v), a.OwnerSubject())
		if e != nil {
			return e
		}
		var state string
		e = tx.QueryRow(ctx, `SELECT id::text,state FROM monitoring_jobs WHERE tenant_id=$1 AND kind='summary' AND target_id=$2 AND target_revision=$3 AND input_hash=$4 AND COALESCE(actor_id,display_subject_id)=$5`, scope.Tenant, plan.snapshot.ID, plan.snapshot.Watermark, digest(v), a.ID).Scan(&out.JobID, &state)
		if e != nil {
			return e
		}
		if a.ReadSubject() == nil {
			j, err := scanJob(tx.QueryRow(ctx, jobSelect+` WHERE id=$1`, out.JobID))
			if err != nil {
				return err
			}
			out.TaskRef = j.TaskRef
		}
		if state == "failed" || state == "needs_action" || state == "cancelled" {
			out.State = state
		}
		_, e = tx.Exec(ctx, `UPDATE monitoring_snapshots SET lease_until=GREATEST(lease_until,clock_timestamp()+interval '1 hour') WHERE tenant_id=$1 AND id=$2`, scope.Tenant, plan.snapshot.ID)
		return e
	})
	return out, e
}
func (s *Service) cachedAnalytic(ctx context.Context, a access.Actor, scope Scope, action string, in QueryInput, plan queryPlan) (AnalyticResult, bool, error) {
	out := AnalyticResult{}
	v := summaryJobInput{Mode: "query", Action: action, Query: in}
	key := ""
	e := s.db(ctx, func(tx pgx.Tx) error {
		e := tx.QueryRow(ctx, `SELECT steps.result->>'object' FROM monitoring_jobs j JOIN monitoring_job_steps steps ON steps.tenant_id=j.tenant_id AND steps.job_id=j.id AND steps.step_key='analytic-result' AND steps.state='completed' WHERE j.tenant_id=$1 AND COALESCE(j.actor_id,j.display_subject_id)=$2 AND j.kind='summary' AND j.target_id=$3 AND j.input_hash=$4 AND j.state='succeeded'`, scope.Tenant, a.ID, plan.snapshot.ID, digest(v)).Scan(&key)
		if e == pgx.ErrNoRows {
			return nil
		}
		return e
	})
	if e != nil || key == "" {
		return out, false, e
	}
	if e = s.storage.summaryObject(ctx, key, &out, true); e != nil {
		return out, false, storageError(e)
	}
	for i := range out.Channels {
		if out.Channels[i].SourceKind == "" {
			out.Channels[i].SourceKind = "direct"
		}
	}
	for i := range out.Groups {
		if out.Groups[i].SourceKind == "" {
			out.Groups[i].SourceKind = "direct"
		}
	}
	if len(plan.vector.Calculated) > 0 {
		heads := map[string]CalculationHead{}
		for _, part := range out.Coverage {
			heads[part.Channel] = plan.vector.Calculated[part.Channel]
		}
		coverage, e := s.calculationReader.CheckCalculated(ctx, a, scope, CalculationQuery{Heads: heads, Windows: plan.windows, Range: queryWindow(in), Qualities: allQualities, History: in.Status == "history"})
		if e != nil {
			return AnalyticResult{}, false, e
		}
		if digest(coverage) != digest(out.Coverage) {
			return AnalyticResult{}, false, nil
		}
		// Retain the freshly qualified transaction-only tickets for host reads.
		// They are intentionally excluded from the public JSON representation.
		out.Coverage = coverage
	}
	if _, e = s.queryPlan(ctx, a, scope, in, action, "read"); e != nil {
		return AnalyticResult{}, false, e
	}
	out.Snapshot = plan.snapshot
	return out, true, nil
}
func (s *Service) summaryJob(ctx context.Context, j Job) error {
	var in summaryJobInput
	if e := json.Unmarshal(j.input, &in); e != nil {
		return e
	}
	if in.Mode == "rebuild" {
		return s.rebuildSummaryJob(ctx, j, in)
	}
	if in.Mode == "maintenance" && j.actor == "" && j.subject == "" {
		watermark, e := revisionNumber(in.Watermark)
		if e != nil {
			return e
		}
		if e = s.jobTransaction(ctx, j, func(tx pgx.Tx) error { return assertJobTx(ctx, tx, j) }); e != nil {
			return e
		}
		_, e = s.buildSummary(ctx, j.tenant, in.Channel, in.Content, in.Revoke, watermark)
		return e
	}
	if in.Mode != "query" || j.actor == "" && j.subject == "" {
		return fmt.Errorf("invalid summary task")
	}
	a, e := s.jobActor(ctx, j)
	if e != nil {
		return e
	}
	query, e := analyticInput(in.Query, in.Action)
	if e != nil {
		return e
	}
	plan, e := s.queryPlan(ctx, a, j.scope(), query, in.Action, "read", true)
	if e != nil {
		return e
	}
	watermark, _ := revisionNumber(plan.snapshot.Watermark)
	for _, channel := range plan.channels {
		if _, derived := plan.vector.Calculated[channel]; derived {
			continue
		}
		if e = s.jobTransaction(ctx, j, func(tx pgx.Tx) error { return assertJobTx(ctx, tx, j) }); e != nil {
			return e
		}
		if _, e = s.buildSummary(ctx, j.tenant, channel, plan.vector.Contents[channel], plan.vector.Revokes[channel], watermark); e != nil {
			return e
		}
	}
	out, e := s.computeAnalytic(ctx, a, j.scope(), in.Action, query, plan, true)
	if e != nil {
		return e
	}
	out.Computation = "background-immutable-minute-tree-v1"
	key, e := s.storage.putSummary(ctx, j.tenant, out)
	if e != nil {
		return e
	}
	if _, e = s.queryPlan(ctx, a, j.scope(), query, in.Action, "read", true); e != nil {
		return e
	}
	return s.jobTransaction(ctx, j, func(tx pgx.Tx) error {
		if e := assertJobTx(ctx, tx, j); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, `INSERT INTO monitoring_job_steps(tenant_id,job_id,step_key,fence,state,result) VALUES($1,$2,'analytic-result',$3,'completed',$4) ON CONFLICT(tenant_id,job_id,step_key) DO UPDATE SET fence=EXCLUDED.fence,state='completed',result=EXCLUDED.result,updated_at=clock_timestamp()`, j.tenant, j.ID, j.fence, encodeJSON(map[string]string{"object": key}))
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE monitoring_jobs SET progress='{"phase":"complete"}',updated_at=clock_timestamp() WHERE id=$1`, j.ID)
		return e
	})
}
