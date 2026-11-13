package monitoring

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
)

// Export creation already freezes the authorized output snapshot. A short
// preflight reports immediately discoverable input errors, but a long history
// is qualified completely by the queued worker before any file is published.
// A deadline is never a successful proof and is never reused by that worker.
func (s *Service) preflightCalculatedExport(ctx context.Context, a access.Actor, scope Scope, plan queryPlan, in QueryInput) error {
	if len(plan.vector.Calculated) == 0 {
		return nil
	}
	check, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	_, e := s.checkCalculatedExport(check, a, scope, plan, in)
	expired := errors.Is(check.Err(), context.DeadlineExceeded)
	cancel()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if expired {
		return nil
	}
	return e
}

var calculationDetailFields = []ExportField{
	{"source_kind", "text", false}, {"owner_kind", "text", false}, {"owner_id", "text", false}, {"owner_name", "text", false},
	{"result_id", "text", true}, {"generation_id", "text", true}, {"config_id", "text", true}, {"run_id", "text", true},
	{"interval_end", "timestamp_ns_utc", true}, {"scale", "int64", true}, {"rounding", "text", true}, {"rounded", "boolean", true},
	{"calculation_status", "text", true}, {"freshness", "text", true}, {"sample_count", "int64", true}, {"excluded_count", "int64", true}, {"reason_flags", "text", true}, {"lineage_id", "text", true},
}
var calculationStatisticFields = []ExportField{
	{"source_kind", "text", false}, {"owner_kind", "text", false}, {"owner_id", "text", false}, {"owner_name", "text", false},
	{"logical_output_count", "int64", true}, {"valid_result_count", "int64", true}, {"no_result_count", "int64", true}, {"calculation_status_counts", "text", true}, {"numeric_issues", "text", true},
}

func calculationExportFields(kind string) []ExportField {
	if kind == "statistics" {
		return append(slices.Clone(statisticsExportFields), calculationStatisticFields...)
	}
	fields := slices.Clone(detailExportFields)
	for i := range fields {
		switch fields[i].Name {
		case "point_id", "point_name", "metric_version_id", "observation_id", "adoption_id", "batch_id", "value":
			fields[i].Nullable = true
		}
	}
	// Mixed value is exact UTF-8 for every scale; raw typed numeric columns keep
	// their existing physical representation and are null on derived rows.
	fields[12].Type = "computed_decimal_text"
	return append(fields, calculationDetailFields...)
}
func extendRawExport(cells []exportCell, c exportChannel) []exportCell {
	out := append(cells, make([]exportCell, len(calculationDetailFields))...)
	for i := len(cells); i < len(out); i++ {
		out[i].Null = calculationDetailFields[i-len(cells)].Nullable
	}
	out[len(cells)] = exportCell{Text: "direct"}
	out[len(cells)+1] = exportCell{Text: c.OwnerKind}
	out[len(cells)+2] = exportCell{Text: c.OwnerID}
	out[len(cells)+3] = exportCell{Text: c.OwnerName}
	return out
}
func exportCalculated(row CalculatedObservation, c exportChannel) []exportCell {
	out := make([]exportCell, len(detailExportFields)+len(calculationDetailFields))
	for i, f := range calculationExportFields("details") {
		out[i].Null = f.Nullable
	}
	set := func(i int, v string) { out[i] = exportCell{Text: v} }
	set(0, row.Channel)
	if c.Point != "" {
		set(1, c.Point)
		set(2, c.PointName)
	}
	set(3, c.Name)
	set(5, c.Unit)
	set(9, strconv.FormatInt(row.AtNS, 10))
	set(11, "decimal")
	if row.Value != nil {
		set(12, *row.Value)
	}
	set(16, row.Quality)
	set(17, "")
	set(18, "false")
	if row.Start != nil {
		at, _ := time.Parse(time.RFC3339Nano, *row.Start)
		set(10, strconv.FormatInt(at.UnixNano(), 10))
	}
	values := []string{"derived", c.OwnerKind, c.OwnerID, c.OwnerName, row.Result, row.Generation, row.Config, row.Run, "", strconv.Itoa(row.Scale), row.Rounding, strconv.FormatBool(row.Rounded), row.Status, row.Freshness, row.Count, row.Excluded, string(encodeJSON(row.Reasons)), row.Lineage}
	for i, v := range values {
		if i == 8 {
			if row.End != nil {
				at, _ := time.Parse(time.RFC3339Nano, *row.End)
				set(len(detailExportFields)+i, strconv.FormatInt(at.UnixNano(), 10))
			}
			continue
		}
		set(len(detailExportFields)+i, v)
	}
	return out
}
func extendStatisticExport(cells []exportCell, v ChannelStatistic, c exportChannel) []exportCell {
	source := v.SourceKind
	if source == "" {
		source = "direct"
	}
	out := append(cells, []exportCell{{Text: source}, {Text: c.OwnerKind}, {Text: c.OwnerID}, {Text: c.OwnerName}, {Null: true}, {Null: true}, {Null: true}, {Null: true}, {Null: true}}...)
	if v.Derived != nil {
		values := []string{v.Derived.LogicalOutputs, v.Derived.ValidResults, v.Derived.NoResults, string(encodeJSON(v.Derived.Statuses)), string(encodeJSON(v.NumericIssues))}
		for i, value := range values {
			out[len(cells)+4+i] = exportCell{Text: value}
		}
	}
	return out
}
func (s *Service) checkCalculatedExport(ctx context.Context, a access.Actor, scope Scope, plan queryPlan, in QueryInput) ([]CalculationCoverage, error) {
	if len(plan.vector.Calculated) == 0 {
		return nil, nil
	}
	coverage, e := s.calculationReader.CheckCalculated(ctx, a, scope, CalculationQuery{Heads: plan.vector.Calculated, Windows: plan.windows, Range: queryWindow(in), Qualities: allQualities, History: in.Status == "history", Capability: "export"})
	if e != nil {
		return nil, e
	}
	for _, part := range coverage {
		if part.State == "pending" {
			return nil, problem("export_invalidated", "派生结果的输入已变化，请重算完成后重新申请导出。", 409)
		}
	}
	return coverage, nil
}
