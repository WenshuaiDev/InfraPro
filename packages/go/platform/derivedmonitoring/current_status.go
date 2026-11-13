package derivedmonitoring

import (
	"context"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/planning"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoringvalue"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Service) CurrentCalculationStatus(ctx context.Context, a access.Actor, scope monitoring.Scope, in monitoring.CalculationQuery, coverage []monitoring.CalculationCoverage) (monitoring.CalculationStatus, error) {
	out := monitoring.CalculationStatus{PendingRanges: []monitoring.Range{}}
	needs := []monitoring.RawNeed{}
	var direct, derived []monitoring.RawNeed
	heads := map[string]monitoring.CalculationHead{}
	pending := []planning.Range{}
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		for id, head := range in.Heads {
			out.ControlState, out.Health = head.ControlState, head.Health
			assignments, e := activeAssignmentsTx(ctx, tx, scope.Tenant, id)
			if e != nil {
				return e
			}
			for _, allowed := range in.Windows[id] {
				requested := allowed
				if in.Range != nil {
					var ok bool
					requested, ok = requested.Intersect(*in.Range)
					if !ok {
						continue
					}
				}
				from := monitoringvalue.MinTime.UnixNano()
				if requested.From != nil {
					from = *requested.From
				}
				wanted := planning.Range{From: from, To: requested.To}
				if e = s.qualifyUncomputedTx(ctx, tx, a, scope, id, wanted, in.Capability); e != nil {
					return e
				}
				dirty, e := tx.Query(ctx, `SELECT affected * $3::int8range FROM derived_dirty_ranges WHERE tenant_id=$1 AND channel_id=$2 AND affected && $3::int8range ORDER BY lower(affected) LIMIT 1001`, scope.Tenant, id, rangeSQL(wanted))
				if e != nil {
					return e
				}
				count := 0
				for dirty.Next() {
					var r pgtype.Range[int64]
					if e = dirty.Scan(&r); e != nil {
						dirty.Close()
						return e
					}
					pending = append(pending, pgRange(r))
					count++
				}
				e = dirty.Err()
				dirty.Close()
				if e != nil {
					return e
				}
				if count > 1000 {
					return problem("query_budget_exceeded", "待计算范围超过交互预算，请缩小范围。", 422)
				}
				for _, assignment := range assignments {
					interval, e := assignment.Range.rangeValue()
					if e != nil {
						return e
					}
					part, ok := planning.Intersection(interval, wanted)
					if !ok {
						continue
					}
					cfg, e := scanConfig(tx.QueryRow(ctx, configSelect+` WHERE tenant_id=$1 AND channel_id=$2 AND id=$3`, scope.Tenant, id, assignment.ConfigID))
					if e != nil {
						return e
					}
					p, e := s.compile(cfg.Rule, "channel", cfg.Parameters)
					if e != nil {
						return e
					}
					input, e := s.plannedInputsTx(ctx, tx, scope.Tenant, cfg, p, part)
					if e != nil {
						return e
					}
					needs = append(needs, input...)
				}
			}
		}
		var e error
		direct, derived, e = splitInputKindsTx(ctx, tx, scope.Tenant, needs)
		if e != nil {
			return e
		}
		ids := []string{}
		for _, n := range derived {
			ids = append(ids, n.Channel)
		}
		heads, e = s.FreezeCalculatedTx(ctx, tx, scope.Tenant, ids)
		return e
	})
	if e != nil {
		return out, e
	}
	for _, part := range coverage {
		if part.State == "pending" || part.State == "recomputing" {
			from, _ := time.Parse(time.RFC3339Nano, *part.Range.From)
			var to *int64
			if part.Range.To != nil {
				at, _ := time.Parse(time.RFC3339Nano, *part.Range.To)
				n := at.UnixNano()
				to = &n
			}
			pending = append(pending, planning.Range{From: from.UnixNano(), To: to})
		}
		if part.State == "current" && part.Range.To != nil {
			if out.ComputedThrough == nil {
				out.ComputedThrough = part.Range.To
			} else {
				previous, _ := time.Parse(time.RFC3339Nano, *out.ComputedThrough)
				at, _ := time.Parse(time.RFC3339Nano, *part.Range.To)
				if at.After(previous) {
					out.ComputedThrough = part.Range.To
				}
			}
		}
	}
	joined, e := planning.Union(pending)
	if e != nil {
		return out, e
	}
	for _, r := range joined {
		out.PendingRanges = append(out.PendingRanges, coverageRange(r))
	}
	out.InputThrough, e = s.monitoring.LatestRawInput(ctx, a, scope, direct, in.Capability)
	if e != nil {
		return out, e
	}
	if len(derived) > 0 {
		windows := map[string][]monitoring.Window{}
		for _, n := range derived {
			windows[n.Channel] = append(windows[n.Channel], monitoring.Window{From: n.From, To: n.To})
		}
		page, e := s.ReadCalculated(ctx, a, scope, monitoring.CalculationQuery{Heads: heads, Windows: windows, Qualities: []string{"good", "unmarked", "suspect", "bad"}, Limit: 1, Reverse: true, EndpointsOnly: true, Capability: in.Capability})
		if e != nil {
			return out, e
		}
		for _, p := range page.Items {
			if out.InputThrough == nil {
				at := p.At
				out.InputThrough = &at
			} else {
				previous, _ := time.Parse(time.RFC3339Nano, *out.InputThrough)
				if p.AtNS > previous.UnixNano() {
					at := p.At
					out.InputThrough = &at
				}
			}
		}
	}
	return out, nil
}

func coverageRange(r planning.Range) monitoring.Range { return coverage("", r, "", "").Range }
