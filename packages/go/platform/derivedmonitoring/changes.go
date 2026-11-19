package derivedmonitoring

import (
	"context"
	"math"
	"strconv"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/compute"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/planning"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicecoord"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoringvalue"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const changeConsumer = "derived-invalidation/1"

func (s *Service) systemTx(ctx context.Context, tenant string, fn func(pgx.Tx, string) error) error {
	if !access.ValidID(tenant) {
		return notFound()
	}
	return s.access.Transact(ctx, func(tx pgx.Tx) error {
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
		var ready bool
		if e := tx.QueryRow(ctx, `SELECT t.status='active' AND e.mode='ready' FROM tenants t CROSS JOIN monitoring_environment_state e WHERE t.id=$1 AND e.id=1`, tenant).Scan(&ready); e != nil {
			return e
		}
		if !ready {
			return nil
		}
		var graph string
		if e := tx.QueryRow(ctx, `SELECT revision::text FROM derived_graph_states WHERE tenant_id=$1 FOR SHARE`, tenant).Scan(&graph); e != nil {
			return e
		}
		return fn(tx, graph)
	})
}

func pgRange(r pgtype.Range[int64]) planning.Range {
	from := monitoringvalue.MinTime.UnixNano()
	if r.LowerType != pgtype.Unbounded {
		from = r.Lower
	}
	var to *int64
	if r.UpperType != pgtype.Unbounded {
		to = planning.End(r.Upper)
	}
	return planning.Range{From: from, To: to}
}

// dirtyTx keeps at most 128 coarse ranges for a channel. On fragmentation it
// widens to one conservative hull and retains the highest required position.
// No source fact is deleted; each consumer owns an independent durable cursor.
func dirtyTx(ctx context.Context, tx pgx.Tx, tenant, channel string, part planning.Range, position, config, graph, reason string) error {
	_, e := tx.Exec(ctx, `WITH old AS (DELETE FROM derived_dirty_ranges WHERE tenant_id=$1 AND channel_id=$2 AND (affected && $3::int8range OR affected -|- $3::int8range) RETURNING affected,required_position,created_at), merged AS(SELECT range_merge(range_agg(r)) AS affected,max(p) AS position,min(created_at) AS created_at FROM (SELECT affected AS r,required_position AS p,created_at FROM old UNION ALL SELECT $3::int8range,$4::bigint,clock_timestamp()) source) INSERT INTO derived_dirty_ranges(tenant_id,channel_id,affected,required_position,config_revision,graph_revision,reason,created_at) SELECT $1,$2,affected,position,$5,$6,$7,created_at FROM merged`, tenant, channel, rangeSQL(part), position, config, graph, reason)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `WITH count AS(SELECT count(*) AS n FROM derived_dirty_ranges WHERE tenant_id=$1 AND channel_id=$2), old AS(DELETE FROM derived_dirty_ranges WHERE tenant_id=$1 AND channel_id=$2 AND (SELECT n FROM count)>128 RETURNING affected,required_position,created_at), combined AS(SELECT range_merge(range_agg(affected)) AS affected,max(required_position) AS position,min(created_at) AS created_at FROM old HAVING count(*)>0) INSERT INTO derived_dirty_ranges(tenant_id,channel_id,affected,required_position,config_revision,graph_revision,reason,created_at) SELECT $1,$2,affected,position,$3,$4,'conservative_fragment_budget',created_at FROM combined`, tenant, channel, config, graph)
	return e
}

type impactAssignment struct {
	channel, config, revision string
	rangeValue                planning.Range
	bindings                  map[string]InputBinding
	plan                      *compute.Plan
	baselines                 []monitoring.RawNeed
}

func (s *Service) impactGraphTx(ctx context.Context, tx pgx.Tx, tenant string) ([]impactAssignment, []string, error) {
	rows, e := tx.Query(ctx, `SELECT a.channel_id::text,a.config_id::text,c.config_revision::text,a.effective_range * allowed.r FROM derived_config_assignments a JOIN derived_channels c USING(tenant_id,channel_id) JOIN derived_execution_mandates m ON m.tenant_id=c.tenant_id AND m.id=c.active_mandate_id JOIN LATERAL unnest(m.output_ranges) allowed(r) ON allowed.r && a.effective_range WHERE a.tenant_id=$1 AND a.superseded_revision IS NULL ORDER BY a.channel_id,lower(a.effective_range) LIMIT 10001`, tenant)
	if e != nil {
		return nil, nil, e
	}
	assignments := []impactAssignment{}
	for rows.Next() {
		var a impactAssignment
		var r pgtype.Range[int64]
		if e = rows.Scan(&a.channel, &a.config, &a.revision, &r); e != nil {
			rows.Close()
			return nil, nil, e
		}
		a.rangeValue = pgRange(r)
		assignments = append(assignments, a)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, nil, e
	}
	if len(assignments) > 10000 {
		return nil, nil, problem("graph_budget_exceeded", "依赖图超出单次规划预算。", 422)
	}
	graph := map[string][]string{}
	cache := map[string]impactAssignment{}
	for i := range assignments {
		a := &assignments[i]
		cached, ok := cache[a.config]
		if !ok {
			cfg, e := scanConfig(tx.QueryRow(ctx, configSelect+` WHERE tenant_id=$1 AND id=$2`, tenant, a.config))
			if e != nil {
				return nil, nil, e
			}
			p, e := s.compile(cfg.Rule, "channel", cfg.Parameters)
			if e != nil {
				return nil, nil, e
			}
			baselineNeeds, e := s.baselineNeedsTx(ctx, tx, tenant, cfg, p)
			if e != nil {
				return nil, nil, e
			}
			cached = impactAssignment{bindings: cfg.InputBindings, plan: p, baselines: baselineNeeds}
			cache[a.config] = cached
		}
		a.bindings, a.plan = cached.bindings, cached.plan
		a.baselines = cached.baselines
		if _, ok := graph[a.channel]; !ok {
			graph[a.channel] = []string{}
		}
		for _, input := range a.bindings {
			graph[a.channel] = append(graph[a.channel], input.ChannelID)
		}
	}
	order, e := planning.Order(graph, 8)
	return assignments, order, e
}

// ConsumeChanges commits invalidation and cursor advancement together. A
// rollback repeats the same bounded source batch; a restart cannot skip it.
func (s *Service) ConsumeChanges(ctx context.Context, tenant string, now time.Time) (int, error) {
	n, _, e := s.consumeThrough(ctx, tenant, now, math.MaxInt64)
	return n, e
}

// ConsumeThrough advances at most one bounded transaction. The caller can retry
// the same notice until the durable cursor reaches its requested position.
func (s *Service) ConsumeThrough(ctx context.Context, tenant string, position int64, now time.Time) (bool, error) {
	_, cursor, e := s.consumeThrough(ctx, tenant, now, position)
	return cursor >= position, e
}
func (s *Service) consumeThrough(ctx context.Context, tenant string, now time.Time, target int64) (int, int64, error) {
	processed := 0
	var cursor int64
	e := s.systemTx(ctx, tenant, func(tx pgx.Tx, graph string) error {
		if _, e := tx.Exec(ctx, `INSERT INTO monitoring_change_consumers(tenant_id,consumer) VALUES($1,$2) ON CONFLICT DO NOTHING`, tenant, changeConsumer); e != nil {
			return e
		}
		if e := tx.QueryRow(ctx, `SELECT greatest(durable_cursor,COALESCE((SELECT prior_position FROM monitoring_change_origins WHERE tenant_id=$1),0)) FROM monitoring_change_consumers WHERE tenant_id=$1 AND consumer=$2 FOR UPDATE`, tenant, changeConsumer).Scan(&cursor); e != nil {
			return e
		}
		type event struct {
			id       string
			kind     string
			position int64
		}
		rows, e := tx.Query(ctx, `SELECT id::text,change_type,source_position FROM monitoring_changes WHERE tenant_id=$1 AND source_position>$2 AND source_position<=$4 ORDER BY source_position LIMIT $3`, tenant, cursor, s.options.EventBatch, target)
		if e != nil {
			return e
		}
		events := []event{}
		for rows.Next() {
			var v event
			if e = rows.Scan(&v.id, &v.kind, &v.position); e != nil {
				rows.Close()
				return e
			}
			events = append(events, v)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if len(events) > 0 {
			assignments, order, e := s.impactGraphTx(ctx, tx, tenant)
			if e != nil {
				return e
			}
			for _, event := range events {
				if event.position != cursor+1 {
					break
				} // wait for the missing immutable fact
				changed := map[string][]planning.Range{}
				horizon := now.UnixNano()
				if horizon < math.MaxInt64 {
					horizon++
				}
				rows, e = tx.Query(ctx, `SELECT channel_id::text,affected FROM monitoring_change_ranges WHERE tenant_id=$1 AND event_id=$2 ORDER BY ordinal`, tenant, event.id)
				if e != nil {
					return e
				}
				for rows.Next() {
					var channel string
					var r pgtype.Range[int64]
					if e = rows.Scan(&channel, &r); e != nil {
						rows.Close()
						return e
					}
					part := pgRange(r)
					changed[channel] = append(changed[channel], part)
					if part.To != nil {
						horizon = max(horizon, *part.To)
					}
				}
				e = rows.Err()
				rows.Close()
				if e != nil {
					return e
				}
				for _, channel := range order {
					for _, assignment := range assignments {
						if assignment.channel != channel {
							continue
						}
						inputs := map[string][]planning.Range{}
						for slot, input := range assignment.bindings {
							inputs[slot] = changed[input.ChannelID]
						}
						bound, ok := planning.Intersection(assignment.rangeValue, planning.Range{From: monitoringvalue.MinTime.UnixNano(), To: &horizon})
						if !ok {
							continue
						}
						impacts, conservative, e := planning.Impact(assignment.plan, inputs, bound)
						if e != nil {
							return e
						}
						reason := "source_change"
						if event.kind == "raw_revoke" {
							for _, baseline := range assignment.baselines {
								from := monitoringvalue.MinTime.UnixNano()
								if baseline.From != nil {
									from = *baseline.From
								}
								for _, part := range changed[baseline.Channel] {
									if _, ok := planning.Intersection(planning.Range{From: from, To: baseline.To}, part); ok {
										impacts = []planning.Range{bound}
										reason = "fixed_baseline_revoked"
									}
								}
							}
						}
						if conservative {
							reason = "conservative_dependency_impact"
						}
						for _, part := range impacts {
							if e = dirtyTx(ctx, tx, tenant, channel, part, strconv.FormatInt(event.position, 10), assignment.revision, graph, reason); e != nil {
								return e
							}
							changed[channel] = append(changed[channel], part)
						}
					}
					changed[channel], e = planning.Union(changed[channel])
					if e != nil {
						return e
					}
					if len(changed[channel]) > 0 {
						if _, e = tx.Exec(ctx, `UPDATE monitoring_jobs j SET available_at=clock_timestamp() FROM derived_runs r WHERE r.tenant_id=$1 AND r.channel_id=$2 AND r.job_id=j.id AND r.mode='automatic' AND j.state='retry_wait' AND j.error_code='upstream_pending'`, tenant, channel); e != nil {
							return e
						}
					}
				}
				if e = markAnalysisChangesTx(ctx, tx, tenant, "", event.id); e != nil {
					return e
				}
				cursor = event.position
				processed++
			}
		}
		_, e = tx.Exec(ctx, `UPDATE monitoring_change_consumers SET durable_cursor=$3,updated_at=clock_timestamp() WHERE tenant_id=$1 AND consumer=$2`, tenant, changeConsumer, cursor)
		return e
	})
	return processed, cursor, e
}
