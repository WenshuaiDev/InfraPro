package derivedmonitoring

import (
	"context"
	"math"
	"strconv"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/planning"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicecoord"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ScheduleTenant is a bounded system scheduler entry point. It creates tasks
// only from persisted dirty coverage and current tenant execution mandates.
func (s *Service) ScheduleTenant(ctx context.Context, tenant string) (int, error) {
	return s.ScheduleTenantAt(ctx, tenant, time.Now())
}

func (s *Service) ScheduleTenantAt(ctx context.Context, tenant string, clock time.Time) (int, error) {
	if !access.ValidID(tenant) {
		return 0, notFound()
	}
	if s.results == nil || s.monitoring == nil {
		return 0, nil
	}
	queued := 0
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, false); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `SELECT id FROM engineering_catalog_state WHERE id=1 FOR SHARE`); e != nil {
			return e
		}
		if e := devicecoord.Catalog(ctx, tx, false); e != nil {
			return e
		}
		if e := devicecoord.Tenant(ctx, tx, tenant, true); e != nil {
			return e
		}
		var active, ready bool
		if e := tx.QueryRow(ctx, `SELECT t.status='active',e.mode='ready' FROM tenants t CROSS JOIN monitoring_environment_state e WHERE t.id=$1 AND e.id=1`, tenant).Scan(&active, &ready); e != nil {
			return e
		}
		if !active || !ready {
			return nil
		}
		var graph string
		if e := tx.QueryRow(ctx, `SELECT revision::text FROM derived_graph_states WHERE tenant_id=$1 FOR SHARE`, tenant).Scan(&graph); e != nil {
			return e
		}
		now := clock.UnixNano()
		if e := s.stageSchedulesTx(ctx, tx, tenant, graph, now); e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT DISTINCT d.channel_id::text FROM derived_dirty_ranges d JOIN derived_channels c USING(tenant_id,channel_id) WHERE d.tenant_id=$1 AND c.control_state='enabled' AND lower(d.affected)<=$2 AND NOT EXISTS(SELECT 1 FROM derived_runs r JOIN monitoring_jobs j ON j.id=r.job_id WHERE r.tenant_id=d.tenant_id AND r.channel_id=d.channel_id AND r.mode='automatic' AND j.state IN ('queued','running','retry_wait')) ORDER BY d.channel_id::text LIMIT $3`, tenant, now, s.options.ScheduleBatch)
		if e != nil {
			return e
		}
		ids := []string{}
		for rows.Next() {
			var id string
			if e = rows.Scan(&id); e != nil {
				rows.Close()
				return e
			}
			ids = append(ids, id)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		for _, channel := range ids {
			var interval pgtype.Range[int64]
			var config, configRevision, controlRevision, mandateRevision, position string
			var assignedFrom int64
			e = tx.QueryRow(ctx, `SELECT (d.affected * a.effective_range * allowed.r),a.config_id::text,c.config_revision::text,c.control_revision::text,c.mandate_revision::text,d.required_position::text,lower(a.effective_range * allowed.r) FROM derived_dirty_ranges d JOIN derived_channels c USING(tenant_id,channel_id) JOIN derived_execution_mandates m ON m.tenant_id=c.tenant_id AND m.id=c.active_mandate_id JOIN LATERAL unnest(m.output_ranges) allowed(r) ON allowed.r && d.affected JOIN derived_config_assignments a ON a.tenant_id=c.tenant_id AND a.channel_id=c.channel_id AND a.superseded_revision IS NULL AND a.effective_range && d.affected AND a.effective_range && allowed.r WHERE d.tenant_id=$1 AND d.channel_id=$2 AND lower(d.affected * a.effective_range * allowed.r)<=$3 ORDER BY lower(d.affected * a.effective_range * allowed.r),d.required_position DESC LIMIT 1`, tenant, channel, now).Scan(&interval, &config, &configRevision, &controlRevision, &mandateRevision, &position, &assignedFrom)
			if e == pgx.ErrNoRows {
				continue
			}
			if e != nil {
				return e
			}
			cfg, e := scanConfig(tx.QueryRow(ctx, configSelect+` WHERE tenant_id=$1 AND channel_id=$2 AND id=$3`, tenant, channel, config))
			if e != nil {
				return e
			}
			p, e := s.compile(cfg.Rule, "channel", cfg.Parameters)
			if e != nil {
				return e
			}
			delay, _ := strconv.ParseInt(p.Rule.Policies.AllowedLatenessNS, 10, 64)
			due := now - delay
			if due < interval.Lower {
				continue
			}
			to := due
			if to < math.MaxInt64 {
				to++
			}
			if interval.UpperType != pgtype.Unbounded {
				to = min(to, interval.Upper)
			}
			if interval.Lower <= math.MaxInt64-int64(24*time.Hour) {
				to = min(to, interval.Lower+int64(24*time.Hour))
			}
			output := planning.Range{From: interval.Lower, To: &to}
			// A derived primary clock cannot provide event times beyond its
			// published horizon. Activation can end later than its parent's last
			// publication; retain that tail as dirty instead of blocking the
			// already-computed prefix in one permanently waiting target.
			if p.Rule.Clock.Kind == "primary_input" {
				output, e = primaryClockGapTx(ctx, tx, tenant, channel, config, assignedFrom, output)
				if e != nil {
					return e
				}
				output, e = primaryClockHorizonTx(ctx, tx, tenant, cfg.InputBindings[p.Rule.Clock.Input].ChannelID, output)
				if e != nil {
					return e
				}
			}
			if output.To != nil && output.From <= math.MaxInt64-int64(24*time.Hour) && *output.To > output.From+int64(24*time.Hour) {
				end := output.From + int64(24*time.Hour)
				output.To = &end
			}
			if !output.Valid() {
				continue
			}
			needs, e := s.plannedInputsTx(ctx, tx, tenant, cfg, p, output)
			if e != nil {
				return e
			}
			pending, e := s.upstreamWorkPendingTx(ctx, tx, tenant, needs, inputPrefixes(p, cfg, needs, output))
			if e != nil {
				return e
			}
			if pending {
				if _, e = tx.Exec(ctx, `UPDATE derived_channels SET health='waiting',blocked_reason='upstream_pending' WHERE tenant_id=$1 AND channel_id=$2`, tenant, channel); e != nil {
					return e
				}
				continue
			}
			hash := fingerprint([]any{tenant, channel, config, configRevision, controlRevision, mandateRevision, rangeDTO(output.From, output.To), position, p.PlanHash, "automatic"})
			var exists bool
			if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM derived_run_publication_targets WHERE tenant_id=$1 AND target_hash=$2)`, tenant, hash).Scan(&exists); e != nil {
				return e
			}
			if exists {
				// Cancelling an automatic run retains its dirty coverage. A current
				// mandate may requeue that same target; pausing fences its mandate
				// and prevents this path. No duplicate generation identity is made.
				tag, e := tx.Exec(ctx, `UPDATE monitoring_jobs j SET state='queued',attempts=0,cancel_requested=false,fence=fence+1,lease_owner=NULL,lease_until=NULL,error_code=NULL,available_at=clock_timestamp(),revision=j.revision+1,updated_at=clock_timestamp() FROM derived_runs r JOIN derived_run_publication_targets t ON t.tenant_id=r.tenant_id AND t.run_id=r.id WHERE j.id=r.job_id AND t.tenant_id=$1 AND t.target_hash=$2 AND j.state='cancelled'`, tenant, hash)
				if e != nil {
					return e
				}
				queued += int(tag.RowsAffected())
				continue
			}
			runID, jobID, generation := uuid.NewString(), uuid.NewString(), uuid.NewString()
			_, e = tx.Exec(ctx, `INSERT INTO derived_runs(id,tenant_id,channel_id,config_id,mode,job_id,request_id,request_hash,output_range,config_revision,control_revision,mandate_revision,plan_hash,input_requirements,requested_at_ns,reason) VALUES($1,$2,$3,$4,'automatic',$5,$6,$7,$8::int8range,$9,$10,$11,$12,$13,$14,'自动更新固定范围')`, runID, tenant, channel, config, jobID, uuid.NewString(), hash, rangeSQL(output), configRevision, controlRevision, mandateRevision, p.PlanHash, jsonBytes(needs), now)
			if e != nil {
				return e
			}
			if _, e = tx.Exec(ctx, `INSERT INTO derived_run_publication_targets(tenant_id,run_id,channel_id,generation_id,target_hash,required_position,graph_revision) VALUES($1,$2,$3,$4,$5,$6,$7)`, tenant, runID, channel, generation, hash, position, graph); e != nil {
				return e
			}
			if _, e = s.monitoring.EnqueueTaskTx(ctx, tx, monitoring.EnqueueTask{ID: jobID, Scope: monitoring.TenantScope(tenant), Kind: "derive", Target: runID, TargetRevision: configRevision, Input: jsonBytes(map[string]string{"mode": "automatic", "run_id": runID})}); e != nil {
				return e
			}
			if e = advanceScheduleTx(ctx, tx, tenant, channel, config, p, output); e != nil {
				return e
			}
			queued++
		}
		return nil
	})
	return queued, e
}
