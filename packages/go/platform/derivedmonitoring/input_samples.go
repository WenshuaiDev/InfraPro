package derivedmonitoring

import (
	"context"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/compute"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/resultstore"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoringvalue"
	"github.com/jackc/pgx/v5"
)

type inputSample struct {
	Channel string
	Sample  compute.Sample
	Raw     *monitoring.RawSample
	Derived *monitoring.CalculatedObservation
}

func rawInputSample(row monitoring.RawSample) (inputSample, error) {
	out := inputSample{Channel: row.Channel, Raw: &row}
	if row.Kind != "decimal" && row.Kind != "integer" {
		return out, problem("input_type_mismatch", "计算输入必须为数值通道。", 422)
	}
	value, e := monitoringvalue.Decimal(row.Value)
	if e != nil {
		return out, e
	}
	out.Sample = compute.Sample{At: row.At, Value: value, Quality: row.Quality, Status: "valid", ID: row.Adoption}
	if row.Start != nil {
		out.Sample.Interval = &compute.TimeRange{From: *row.Start, To: row.At}
	}
	return out, nil
}
func derivedInputSample(row resultstore.Row) (inputSample, error) {
	point := outputPoint(row)
	out := inputSample{Channel: row.Series, Derived: &point, Sample: compute.Sample{At: row.At, Quality: row.Quality, Status: row.Status, ID: row.Result + ":" + row.Generation, ConfigID: row.Config, Reasons: append([]string{}, row.Reasons...), Count: row.Count, Excluded: row.Excluded}}
	if row.Coefficient != nil {
		var e error
		out.Sample.Value, e = monitoringvalue.Decimal(*point.Value)
		if e != nil {
			return out, e
		}
	}
	if row.Start != nil {
		out.Sample.Interval = &compute.TimeRange{From: *row.Start, To: row.At}
	}
	return out, nil
}
func (s *Service) readInputs(ctx context.Context, auth inputAccess, id string, part inputSnapshotDetails, budget monitoring.RawReadBudget, consume func(inputSample) error) (monitoring.RawReadCount, error) {
	var count monitoring.RawReadCount
	defer func() { monitoring.RecordCalculationRead(ctx, count) }()
	check := func() error {
		return s.access.Transact(ctx, func(tx pgx.Tx) error {
			state, e := s.checkInputPartTx(ctx, tx, auth, id, part.Direct, part.Sources)
			if e != nil {
				return e
			}
			if !auth.History && state.Revoked {
				return problem("snapshot_stale", "固定来源已撤销，未完成的分析不能发布。", 409)
			}
			if !auth.History && !auth.AllowChanged && state.Changed {
				return problem("upstream_pending", "固定输入已变化，等待重新规划。", 409)
			}
			return nil
		})
	}
	if e := check(); e != nil {
		return count, e
	}
	if len(part.Direct) > 0 {
		callback := func(row monitoring.RawSample) error {
			sample, e := rawInputSample(row)
			if e != nil {
				return e
			}
			return consume(sample)
		}
		var e error
		if auth.Permit != nil {
			count, e = s.raw.ReadPartSystem(ctx, *auth.Permit, id, part.Direct, budget, callback)
		} else if auth.History || auth.SelectedPart {
			count, e = s.raw.ReadSelectedPartUser(ctx, auth.Actor, auth.Scope, id, part.Direct, auth.History, budget, callback)
		} else {
			count, e = s.raw.ReadPartUser(ctx, auth.Actor, auth.Scope, id, part.Direct, budget, callback)
		}
		if e != nil {
			return count, e
		}
	}
	var scanned uint64
	for _, proof := range part.Sources {
		r, e := proof.Range.rangeValue()
		if e != nil {
			return count, e
		}
		manifest, e := s.sourceManifest(ctx, auth.Scope.Tenant, proof)
		if e != nil {
			return count, e
		}
		for _, block := range manifest.Blocks {
			e = s.results.ReadBlock(ctx, block, func(row resultstore.Row) error {
				scanned++
				if scanned > 10_000_000 {
					return problem("budget_exceeded", "上游输入扫描超过单次预算。", 422)
				}
				if row.Generation != proof.Generation || row.Config != manifest.Config || row.Series != proof.Channel || row.Run != manifest.Run || row.Lineage != manifest.Lineage {
					return problem("result_integrity_unavailable", "上游结果块身份不一致。", 503)
				}
				if !r.Contains(row.At) {
					return nil
				}
				count.Rows++
				count.Bytes += uint64(len(jsonBytes(row)))
				if count.Rows > budget.Rows || count.Bytes > budget.Bytes {
					return &monitoring.Error{Code: "budget_exceeded", Message: "完整计算输入超过工作区预算。", Status: 422}
				}
				sample, e := derivedInputSample(row)
				if e != nil {
					return e
				}
				if e = consume(sample); e != nil {
					return e
				}
				return ctx.Err()
			})
			if e != nil {
				return count, e
			}
		}
	}
	return count, check()
}
