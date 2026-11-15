package derivedmonitoring

import (
	"context"
	"encoding/json"
	"math/big"
	"slices"
	"strconv"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/resultstore"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoringvalue"
	"github.com/jackc/pgx/v5"
)

type analysisIterator struct {
	service    *Service
	lease      monitoring.TaskLease
	proof      analysisArtifactProof
	blocks     []resultstore.Block
	rows       []resultstore.Row
	block, row int
	rangeValue *NanosecondRange
}

func (s *Service) analysisIterator(ctx context.Context, lease monitoring.TaskLease, proof analysisArtifactProof, key string, selected *NanosecondRange) (*analysisIterator, error) {
	out := &analysisIterator{service: s, lease: lease, proof: proof, rangeValue: selected, blocks: []resultstore.Block{}}
	if key == "" {
		return out, nil
	}
	for _, block := range proof.Manifest.Blocks {
		// Older manifests predate the bounded index. Derive that metadata from a
		// fully verified block once; never guess its output from ordering.
		if block.Output == "" {
			if e := lease.Check(ctx); e != nil {
				return nil, e
			}
			var first, last int64
			var output string
			e := s.results.ReadBlock(ctx, block, func(row resultstore.Row) error {
				if output == "" {
					output = row.Output
					first = row.At
				}
				last = row.At
				return nil
			})
			if e != nil {
				return nil, e
			}
			block.Output = output
			block.First = &first
			block.Last = &last
		}
		if block.Output != key {
			continue
		}
		if selected != nil {
			r, e := selected.rangeValue()
			if e != nil {
				return nil, e
			}
			includeEnd := analysisIncludesWholeEnd(proof, &r.From, r.To)
			if *block.Last < r.From || r.To != nil && (*block.First > *r.To || *block.First == *r.To && !includeEnd) {
				continue
			}
		}
		out.blocks = append(out.blocks, block)
	}
	slices.SortFunc(out.blocks, func(a, b resultstore.Block) int {
		if *a.First < *b.First {
			return -1
		}
		if *a.First > *b.First {
			return 1
		}
		return 0
	})
	for i := 1; i < len(out.blocks); i++ {
		if *out.blocks[i-1].Last >= *out.blocks[i].First {
			return nil, problem("result_integrity_unavailable", "比较输出块存在重复时刻。", 503)
		}
	}
	return out, nil
}
func (i *analysisIterator) next(ctx context.Context) (*resultstore.Row, error) {
	for {
		if i.row < len(i.rows) {
			row := i.rows[i.row]
			i.row++
			return &row, nil
		}
		if i.block >= len(i.blocks) {
			return nil, nil
		}
		if e := i.lease.Check(ctx); e != nil {
			return nil, e
		}
		block := i.blocks[i.block]
		i.block++
		i.rows = nil
		i.row = 0
		e := i.service.results.ReadBlock(ctx, block, func(row resultstore.Row) error {
			if row.Artifact == nil || *row.Artifact != i.proof.Version.ArtifactID || row.Generation != i.proof.Version.ID || row.Config != i.proof.Version.ConfigVersionID || row.Run != i.proof.Version.RunID || row.Lineage != i.proof.Version.Lineage {
				return problem("result_integrity_unavailable", "比较来源身份不一致。", 503)
			}
			if i.rangeValue != nil {
				r, _ := i.rangeValue.rangeValue()
				if !r.Contains(row.At) && !(r.To != nil && row.At == *r.To && analysisIncludesWholeEnd(i.proof, &r.From, r.To)) {
					return nil
				}
			}
			i.rows = append(i.rows, row)
			return nil
		})
		if e != nil {
			return nil, e
		}
		if e = i.lease.Check(ctx); e != nil {
			return nil, e
		}
	}
}
func artifactRow(row *resultstore.Row) *AnalysisArtifactRow {
	if row == nil {
		return nil
	}
	value := TrialValue{Output: row.Output, At: strconv.FormatInt(row.At, 10), Value: row.Decimal(), Scale: int(row.Scale), Rounded: row.Rounded, Status: row.Status, Quality: row.Quality, Reasons: row.Reasons, Count: strconv.FormatUint(row.Count, 10), Excluded: strconv.FormatUint(row.Excluded, 10), Matches: []TrialMatch{}}
	if row.Start != nil {
		r := rangeDTO(*row.Start, row.End)
		value.Interval = &r
	}
	return &AnalysisArtifactRow{ResultID: row.Result, TrialValue: value, Lineage: row.Lineage}
}
func exactAnalysisDifference(left, right *resultstore.Row) *string {
	if left == nil || right == nil || left.Coefficient == nil || right.Coefficient == nil {
		return nil
	}
	scale := max(int(left.Scale), int(right.Scale))
	a, _ := new(big.Int).SetString(*left.Coefficient, 10)
	b, _ := new(big.Int).SetString(*right.Coefficient, 10)
	a.Mul(a, monitoringvalue.Power10(scale-int(left.Scale)))
	b.Mul(b, monitoringvalue.Power10(scale-int(right.Scale)))
	value := monitoringvalue.Format(b.Sub(b, a), scale)
	return &value
}

type comparisonPart struct {
	Output   AnalysisComparisonOutput       `json:"output"`
	Count    uint64                         `json:"differences,string"`
	Examples []AnalysisComparisonDifference `json:"examples"`
	Pages    []comparisonPage               `json:"pages"`
}

func (s *Service) compareAnalysisOutput(ctx context.Context, lease monitoring.TaskLease, left, right analysisArtifactProof, pair AnalysisOutputMapping, selected *NanosecondRange) (comparisonPart, error) {
	part := comparisonPart{Output: AnalysisComparisonOutput{AnalysisOutputMapping: pair}, Examples: []AnalysisComparisonDifference{}}
	page := comparisonPageDocument{Schema: "analysis-comparison-page/1", Run: lease.Task().Job.Target, Items: []AnalysisComparisonDifference{}}
	pageBytes := 0
	flush := func() error {
		if len(page.Items) == 0 {
			return nil
		}
		if e := lease.Check(ctx); e != nil {
			return e
		}
		hash, e := s.results.SealDocument(ctx, lease.Task().Scope.Tenant, "checkpoints", page)
		if e != nil {
			return e
		}
		part.Pages = append(part.Pages, comparisonPage{Hash: hash, Rows: uint64(len(page.Items))})
		page.Items = nil
		pageBytes = 0
		return nil
	}
	var lo, ro Output
	for _, o := range left.Manifest.Outputs {
		if o.Key == pair.Left {
			lo = o
		}
	}
	for _, o := range right.Manifest.Outputs {
		if o.Key == pair.Right {
			ro = o
		}
	}
	part.Output.NumericComparable = lo.Key != "" && ro.Key != "" && fingerprint(lo.Quantity) == fingerprint(ro.Quantity)
	part.Output.SemanticChanged = lo.SemanticKey != ro.SemanticKey || !part.Output.NumericComparable
	l, e := s.analysisIterator(ctx, lease, left, pair.Left, selected)
	if e != nil {
		return part, e
	}
	r, e := s.analysisIterator(ctx, lease, right, pair.Right, selected)
	if e != nil {
		return part, e
	}
	a, e := l.next(ctx)
	if e != nil {
		return part, e
	}
	b, e := r.next(ctx)
	if e != nil {
		return part, e
	}
	for a != nil || b != nil {
		if e = ctx.Err(); e != nil {
			return part, e
		}
		difference := AnalysisComparisonDifference{LeftOutput: pair.Left, RightOutput: pair.Right, Kind: "changed", Changes: []string{}}
		var takeLeft, takeRight bool
		switch {
		case a == nil || b != nil && b.At < a.At:
			takeRight = true
			difference.At = strconv.FormatInt(b.At, 10)
			difference.Kind = "added"
			difference.Right = artifactRow(b)
			part.Output.Added++
		case b == nil || a.At < b.At:
			takeLeft = true
			difference.At = strconv.FormatInt(a.At, 10)
			difference.Kind = "removed"
			difference.Left = artifactRow(a)
			part.Output.Removed++
		default:
			takeLeft, takeRight = true, true
			difference.At = strconv.FormatInt(a.At, 10)
			difference.Left, difference.Right = artifactRow(a), artifactRow(b)
			delta := exactAnalysisDifference(a, b)
			if part.Output.NumericComparable {
				difference.Difference = delta
			}
			valueChanged := (a.Coefficient == nil) != (b.Coefficient == nil)
			if delta != nil {
				rat, ok := new(big.Rat).SetString(*delta)
				if !ok {
					return part, problem("result_integrity_unavailable", "比较数值编码无效。", 503)
				}
				valueChanged = rat.Sign() != 0
			}
			if valueChanged {
				part.Output.ValueChanged++
				difference.Changes = append(difference.Changes, "value")
			}
			if a.Status != b.Status || fingerprint(a.Reasons) != fingerprint(b.Reasons) {
				part.Output.StatusChanged++
				difference.Changes = append(difference.Changes, "status")
			}
			if a.Quality != b.Quality {
				part.Output.QualityChanged++
				difference.Changes = append(difference.Changes, "quality")
			}
			if a.Count != b.Count || a.Excluded != b.Excluded {
				part.Output.SamplesChanged++
				difference.Changes = append(difference.Changes, "samples")
			}
			if fingerprint([]any{a.Start, a.End}) != fingerprint([]any{b.Start, b.End}) {
				part.Output.IntervalChanged++
				difference.Changes = append(difference.Changes, "interval")
			}
			if len(difference.Changes) == 0 {
				part.Output.Unchanged++
			}
		}
		if takeLeft {
			part.Output.LeftRows++
		}
		if takeRight {
			part.Output.RightRows++
		}
		if difference.Kind != "changed" || len(difference.Changes) > 0 {
			part.Count++
			if len(part.Examples) < 100 {
				part.Examples = append(part.Examples, difference)
			}
			size := len(jsonBytes(difference))
			if len(page.Items) >= 1000 || pageBytes+size > 4<<20 {
				if e = flush(); e != nil {
					return part, e
				}
			}
			page.Items = append(page.Items, difference)
			pageBytes += size
		}
		if takeLeft {
			a, e = l.next(ctx)
			if e != nil {
				return part, e
			}
		}
		if takeRight {
			b, e = r.next(ctx)
			if e != nil {
				return part, e
			}
		}
	}
	return part, flush()
}
func (s *Service) executeComparison(ctx context.Context, lease monitoring.TaskLease) error {
	task := lease.Task()
	scope := Scope{Tenant: task.Scope.Tenant, Project: task.Scope.Project, Entry: task.Scope.Entry}
	var target analysisComparisonTarget
	if e := lease.Transaction(ctx, func(tx pgx.Tx) error {
		var e error
		target, e = comparisonTargetTx(ctx, tx, scope, task.Job.Target)
		return e
	}); e != nil {
		return e
	}
	left, e := s.loadSelectedAnalysis(ctx, lease.Actor(), scope, target.LeftArtifact, AnalysisArtifactQuery{VersionID: target.LeftVersion, Range: target.Range, Outputs: comparisonOutputKeys(target.Outputs, true)}, "analysis.comparisons")
	if e != nil {
		return e
	}
	right, e := s.loadSelectedAnalysis(ctx, lease.Actor(), scope, target.RightArtifact, AnalysisArtifactQuery{VersionID: target.RightVersion, Range: target.Range, Outputs: comparisonOutputKeys(target.Outputs, false)}, "analysis.comparisons")
	if e != nil {
		return e
	}
	result := AnalysisComparisonResult{Left: left.Detail.Version, Right: right.Detail.Version, LeftConfigHash: left.Detail.Config.ConfigHash, RightConfigHash: right.Detail.Config.ConfigHash, LeftInputHash: left.Manifest.Input.Hash, RightInputHash: right.Manifest.Input.Hash, ConfigurationChanged: left.Detail.Config.ConfigHash != right.Detail.Config.ConfigHash || left.Manifest.Plan != right.Manifest.Plan, InputsChanged: left.Manifest.Input.Hash != right.Manifest.Input.Hash || fingerprint(left.Manifest.Sources) != fingerprint(right.Manifest.Sources), RangeChanged: fingerprint(left.Manifest.Range) != fingerprint(right.Manifest.Range) || fingerprint(left.Manifest.SourceRange) != fingerprint(right.Manifest.SourceRange), Range: target.Range, Outputs: []AnalysisComparisonOutput{}, Examples: []AnalysisComparisonDifference{}}
	result.LeftInputHash = fingerprint([]any{left.Manifest.Input.Hash, left.Manifest.Sources, left.Manifest.Baselines})
	result.LeftQuality = left.Detail.Run.Quality
	result.RightQuality = right.Detail.Run.Quality
	result.RightInputHash = fingerprint([]any{right.Manifest.Input.Hash, right.Manifest.Sources, right.Manifest.Baselines})
	result.InputsChanged = result.LeftInputHash != result.RightInputHash
	if result.Left.Partial || result.Right.Partial {
		result.RangeChanged = fingerprint(result.Left.OutputRange) != fingerprint(result.Right.OutputRange)
	}
	pages := []comparisonPage{}
	for index, pair := range target.Outputs {
		if e = lease.Progress(ctx, map[string]string{"phase": "comparing", "completed_outputs": strconv.Itoa(index), "total_outputs": strconv.Itoa(len(target.Outputs))}); e != nil {
			return e
		}
		key := "comparison:" + target.Hash + ":" + fingerprint(pair)
		saved, found, e := lease.Checkpoint(ctx, key)
		if e != nil {
			return e
		}
		var part comparisonPart
		if !found || saved.State != "completed" || json.Unmarshal(saved.Result, &part) != nil {
			part, e = s.compareAnalysisOutput(ctx, lease, left, right, pair, target.Range)
			if e != nil {
				return e
			}
			if e = lease.SaveCheckpoint(ctx, key, "completed", part); e != nil {
				return e
			}
		}
		result.Outputs = append(result.Outputs, part.Output)
		pages = append(pages, part.Pages...)
		result.Differences += part.Count
		remaining := 100 - len(result.Examples)
		if remaining > 0 {
			result.Examples = append(result.Examples, part.Examples[:min(remaining, len(part.Examples))]...)
		}
	}
	result.ExamplesTruncated = result.Differences > uint64(len(result.Examples))
	receipt := comparisonReceipt{AnalysisComparisonResult: result, Pages: pages}
	if len(jsonBytes(receipt)) > 4<<20 {
		return problem("budget_exceeded", "比较索引超过单次预算，请缩小比较范围。", 422)
	}
	return lease.Transaction(ctx, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `INSERT INTO analysis_comparison_receipts(tenant_id,run_id,result) VALUES($1,$2,$3)`, scope.Tenant, task.Job.Target, jsonBytes(receipt)); e != nil {
			return e
		}
		e := lease.CompleteTx(ctx, tx, map[string]string{"phase": "completed", "difference_count": strconv.FormatUint(result.Differences, 10)})
		return e
	})
}
