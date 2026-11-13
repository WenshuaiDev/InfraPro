package derivedmonitoring

import (
	"context"
	"math"
	"strconv"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/compute"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/planning"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoringvalue"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// stageSchedulesTx retains due coverage until the task transaction advances
// next_due. Both dirty coverage and a queued task survive process termination.
func (s *Service) stageSchedulesTx(ctx context.Context, tx pgx.Tx, tenant, graph string, now int64) error {
	rows, e := tx.Query(ctx, `SELECT sch.channel_id::text,sch.config_id::text,sch.next_due_ns,c.config_revision::text,a.effective_range * allowed.r FROM derived_schedules sch JOIN derived_channels c USING(tenant_id,channel_id) JOIN derived_execution_mandates m ON m.tenant_id=c.tenant_id AND m.id=c.active_mandate_id JOIN derived_config_assignments a ON a.tenant_id=sch.tenant_id AND a.channel_id=sch.channel_id AND a.config_id=sch.config_id AND a.superseded_revision IS NULL JOIN LATERAL unnest(m.output_ranges) allowed(r) ON allowed.r && a.effective_range WHERE sch.tenant_id=$1 AND c.control_state='enabled' AND sch.clock_kind IN ('fixed_grid','calendar') AND sch.next_due_ns<=$2 AND NOT EXISTS(SELECT 1 FROM derived_runs r JOIN monitoring_jobs j ON j.id=r.job_id WHERE r.tenant_id=sch.tenant_id AND r.channel_id=sch.channel_id AND r.mode='automatic' AND j.state IN ('queued','running','retry_wait')) ORDER BY sch.next_due_ns,sch.channel_id LIMIT 100`, tenant, now)
	if e != nil {
		return e
	}
	type schedule struct {
		channel, config, revision string
		next                      int64
		output                    planning.Range
	}
	items := []schedule{}
	for rows.Next() {
		var v schedule
		var r pgtype.Range[int64]
		if e = rows.Scan(&v.channel, &v.config, &v.next, &v.revision, &r); e != nil {
			rows.Close()
			return e
		}
		v.output = pgRange(r)
		items = append(items, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	var position string
	if e = tx.QueryRow(ctx, `SELECT commit_sequence::text FROM monitoring_tenant_states WHERE tenant_id=$1`, tenant).Scan(&position); e != nil {
		return e
	}
	for _, v := range items {
		cfg, e := scanConfig(tx.QueryRow(ctx, configSelect+` WHERE tenant_id=$1 AND id=$2`, tenant, v.config))
		if e != nil {
			return e
		}
		p, e := s.compile(cfg.Rule, "channel", cfg.Parameters)
		if e != nil {
			return e
		}
		first, e := compute.NextClockTime(p.Rule.Clock, max(v.next, v.output.From))
		if e != nil {
			return e
		}
		if first == nil {
			continue
		}
		delay, _ := strconv.ParseInt(p.Rule.Policies.AllowedLatenessNS, 10, 64)
		if delay > 0 && now < monitoringvalue.MinTime.UnixNano()+delay {
			continue
		}
		due := now - delay
		if *first > due || !v.output.Contains(*first) {
			continue
		}
		to := planning.End(due + 1)
		if due == math.MaxInt64 {
			to = nil
		}
		if *first <= math.MaxInt64-int64(24*time.Hour) {
			end := *first + int64(24*time.Hour)
			if to == nil || end < *to {
				to = &end
			}
		}
		part, ok := planning.Intersection(v.output, planning.Range{From: *first, To: to})
		if !ok {
			continue
		}
		if e = dirtyTx(ctx, tx, tenant, v.channel, part, position, v.revision, graph, "clock_due"); e != nil {
			return e
		}
	}
	return nil
}

func advanceScheduleTx(ctx context.Context, tx pgx.Tx, tenant, channel, config string, p *compute.Plan, output planning.Range) error {
	if p.Rule.Clock.Kind != "fixed_grid" && p.Rule.Clock.Kind != "calendar" {
		return nil
	}
	var next *int64
	var e error
	if output.To != nil {
		next, e = compute.NextClockTime(p.Rule.Clock, *output.To)
		if e != nil {
			return e
		}
	}
	if next == nil {
		_, e = tx.Exec(ctx, `DELETE FROM derived_schedules WHERE tenant_id=$1 AND channel_id=$2 AND config_id=$3`, tenant, channel, config)
	} else {
		_, e = tx.Exec(ctx, `UPDATE derived_schedules SET next_due_ns=$4,revision=revision+1 WHERE tenant_id=$1 AND channel_id=$2 AND config_id=$3 AND next_due_ns<$4`, tenant, channel, config, *next)
	}
	return e
}

// Tick is invoked by the shared monitoring worker scheduler. The rotating UUID
// cursor is only a fairness hint; all correctness progress lives in PostgreSQL.
func (s *Service) Tick(ctx context.Context, now time.Time) error {
	s.schedulerMu.Lock()
	defer s.schedulerMu.Unlock()
	ids := []string{}
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `SELECT t.id::text FROM tenants t WHERE t.status='active' AND EXISTS(SELECT 1 FROM derived_channels c WHERE c.tenant_id=t.id) ORDER BY (t.id::text>$1) DESC,t.id LIMIT $2`, s.schedulerCursor, s.options.TenantBatch)
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
		return e
	}
	var firstError error
	for _, tenant := range ids {
		s.schedulerCursor = tenant
		if _, e = s.ConsumeChanges(ctx, tenant, now); e != nil {
			if firstError == nil {
				firstError = e
			}
			continue
		}
		if _, e = s.ScheduleTenantAt(ctx, tenant, now); e != nil && firstError == nil {
			firstError = e
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return firstError
}
