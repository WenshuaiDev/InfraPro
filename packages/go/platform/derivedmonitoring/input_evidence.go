package derivedmonitoring

import (
	"context"
	"encoding/json"
	"math"
	"slices"
	"strconv"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/compute"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/planning"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/jackc/pgx/v5"
)

// Each group names one immutable candidate set. Repeated use in different
// nodes remains explicit through path; rows are never truncated to previews.
type evidenceGroup struct {
	Configuration json.RawMessage        `json:"configuration"`
	Versions      map[string]string      `json:"calculation_versions"`
	Path          string                 `json:"path"`
	Role          string                 `json:"role"`
	Config        string                 `json:"config_id"`
	Rule          json.RawMessage        `json:"rule"`
	Snapshot      monitoring.RawSnapshot `json:"input_snapshot"`
	Direct        []monitoring.RawNeed   `json:"direct_input_ranges"`
	Sources       []inputSourceProof     `json:"upstream_inputs"`
	Baseline      *fixedBaseline         `json:"baseline,omitempty"`
}

func (s *Service) evidenceGroups(ctx context.Context, auth inputAccess, root evidenceGroup, baselines []BaselineProof) ([]evidenceGroup, error) {
	groups := []evidenceGroup{}
	path := map[string]bool{}
	var visit func(evidenceGroup, []BaselineProof, int) error
	visit = func(group evidenceGroup, basis []BaselineProof, depth int) error {
		if depth > 16 || len(groups) >= 10000 {
			return problem("plan_limit_exceeded", "输入依据层级或分组超过预算。", 422)
		}
		key := group.Config + ":" + group.Snapshot.ID
		if path[key] {
			return problem("result_integrity_unavailable", "输入依据出现循环。", 503)
		}
		path[key] = true
		defer delete(path, key)
		if e := s.access.Transact(ctx, func(tx pgx.Tx) error {
			_, e := s.checkInputPartTx(ctx, tx, auth, group.Snapshot.ID, group.Direct, group.Sources)
			return e
		}); e != nil {
			return e
		}
		groups = append(groups, group)
		for i, source := range group.Sources {
			manifest, e := s.sourceManifest(ctx, auth.Scope.Tenant, source)
			if e != nil {
				return e
			}
			var cfg ChannelConfig
			e = s.access.Transact(ctx, func(tx pgx.Tx) error {
				var e error
				cfg, e = scanConfig(tx.QueryRow(ctx, configSelect+` WHERE tenant_id=$1 AND channel_id=$2 AND id=$3`, auth.Scope.Tenant, source.Channel, source.Config))
				return e
			})
			if e != nil {
				return e
			}
			versions, e := s.evidenceVersions(ctx, auth.Scope.Tenant, manifest.Lineage)
			if e != nil {
				return e
			}
			child := evidenceGroup{Configuration: jsonBytes(cfg), Versions: versions, Path: group.Path + "/upstream/" + strconv.Itoa(i), Role: "candidate", Config: cfg.ID, Rule: cfg.Rule, Snapshot: manifest.Input, Direct: source.RawNeeds, Sources: source.Inputs}
			if e = visit(child, source.Baselines, depth+1); e != nil {
				return e
			}
		}
		for _, proof := range basis {
			var fixed fixedBaseline
			e := s.access.Transact(ctx, func(tx pgx.Tx) error {
				v, found, e := loadBaselineTx(ctx, tx, auth.Scope.Tenant, group.Config, proof.Reference)
				if e != nil {
					return e
				}
				if !found || v.Snapshot.ID != proof.Snapshot || fingerprint(v) != proof.Hash {
					return problem("result_integrity_unavailable", "固定基准依据不一致。", 503)
				}
				fixed = v
				_, e = s.checkFixedBaselineHistoryTx(ctx, tx, auth, baselineMetadata{Snapshot: fixed.Snapshot, Needs: fixed.Needs})
				return e
			})
			if e != nil {
				return e
			}
			part, e := s.inspectInputs(ctx, auth, fixed.Snapshot.ID)
			if e != nil {
				return e
			}
			child := evidenceGroup{Configuration: group.Configuration, Versions: group.Versions, Path: group.Path + "/baseline/" + proof.Reference, Role: "baseline", Config: group.Config, Rule: group.Rule, Snapshot: fixed.Snapshot, Direct: part.Direct, Sources: part.Sources, Baseline: &fixed}
			if e = visit(child, nil, depth+1); e != nil {
				return e
			}
		}
		return nil
	}
	e := visit(root, baselines, 0)
	return groups, e
}
func (s *Service) checkEvidenceGroupsTx(ctx context.Context, tx pgx.Tx, auth inputAccess, groups []evidenceGroup) error {
	if len(groups) == 0 || len(groups) > 10000 {
		return problem("result_integrity_unavailable", "输入依据分组不完整。", 503)
	}
	for _, group := range groups {
		if _, e := s.checkInputPartTx(ctx, tx, auth, group.Snapshot.ID, group.Direct, group.Sources); e != nil {
			return e
		}
	}
	return nil
}

var evidenceExportFields = []monitoring.ExportField{
	{Name: "evidence_path", Type: "text"}, {Name: "role", Type: "text"}, {Name: "config_id", Type: "text"}, {Name: "input_snapshot_id", Type: "text"}, {Name: "channel_id", Type: "text"}, {Name: "source_kind", Type: "text"},
	{Name: "observation_id", Type: "text", Nullable: true}, {Name: "adoption_id", Type: "text", Nullable: true}, {Name: "batch_id", Type: "text", Nullable: true}, {Name: "result_id", Type: "text", Nullable: true}, {Name: "generation_id", Type: "text", Nullable: true},
	{Name: "observed_at", Type: "timestamp_ns_utc"}, {Name: "interval_start", Type: "timestamp_ns_utc", Nullable: true}, {Name: "value", Type: "computed_decimal_text", Nullable: true}, {Name: "scale", Type: "int64"}, {Name: "rounding", Type: "text"}, {Name: "rounded", Type: "boolean"},
	{Name: "quality", Type: "text"}, {Name: "calculation_status", Type: "text"}, {Name: "eligible_quality", Type: "boolean"}, {Name: "reason_flags", Type: "text"}, {Name: "quality_reason", Type: "text"}, {Name: "value_kind", Type: "text"},
}

func evidenceCells(group evidenceGroup, row inputSample, qualities []string) []monitoring.ExportCell {
	cells := make([]monitoring.ExportCell, len(evidenceExportFields))
	for i, f := range evidenceExportFields {
		cells[i].Null = f.Nullable
	}
	set := func(i int, v string) { cells[i] = monitoring.ExportCell{Text: v} }
	set(0, group.Path)
	set(1, group.Role)
	set(2, group.Config)
	set(3, group.Snapshot.ID)
	set(4, row.Channel)
	set(11, strconv.FormatInt(row.Sample.At, 10))
	set(17, row.Sample.Quality)
	set(18, row.Sample.Status)
	set(19, strconv.FormatBool(slices.Contains(qualities, row.Sample.Quality)))
	set(20, string(jsonBytes(row.Sample.Reasons)))
	set(21, "")
	set(22, "decimal")
	if row.Sample.Interval != nil {
		set(12, strconv.FormatInt(row.Sample.Interval.From, 10))
	}
	if row.Raw != nil {
		r := row.Raw
		set(21, r.QualityReason)
		set(22, r.Kind)
		set(5, "direct")
		set(6, r.Observation)
		set(7, r.Adoption)
		set(8, r.Batch)
		set(13, r.Value)
		scale := "12"
		if r.Kind == "integer" {
			scale = "0"
		}
		set(14, scale)
		set(15, "source_exact")
		set(16, "false")
	} else {
		r := row.Derived
		set(5, "derived")
		set(9, r.Result)
		set(10, r.Generation)
		if r.Value != nil {
			set(13, *r.Value)
		}
		set(14, strconv.Itoa(r.Scale))
		set(15, r.Rounding)
		set(16, strconv.FormatBool(r.Rounded))
		set(20, string(jsonBytes(r.Reasons)))
	}
	return cells
}
func (s *Service) generateInputEvidence(ctx context.Context, lease monitoring.TaskLease, auth inputAccess, groups []evidenceGroup, csvMode string, source any) error {
	spec := monitoring.ResultExportSpec{Schema: "calculation-input-evidence/1", CSVMode: csvMode, Fields: evidenceExportFields, Ordering: "evidence group depth-first; direct channel/time then upstream immutable block order; identities include source snapshot and evidence_path", Source: jsonBytes(map[string]any{"target": source, "groups": groups, "row_semantics": "Complete immutable candidate sets, including rejected qualities and upstream no-result rows. A candidate is not necessarily a selected match; the original rule, time ranges and baseline identify the computation. Values are exact decimal text."})}
	var rows uint64
	return s.monitoring.GenerateResultExport(ctx, lease, spec, func(write func([]monitoring.ExportCell) error) error {
		for _, group := range groups {
			var rule Rule
			if e := compute.Decode(group.Rule, &rule); e != nil {
				return e
			}
			part := inputSnapshotDetails{Direct: group.Direct, Sources: group.Sources}
			_, e := s.readInputs(ctx, auth, group.Snapshot.ID, part, monitoring.RawReadBudget{Rows: math.MaxUint64, Bytes: math.MaxUint64}, func(sample inputSample) error {
				if rows%1024 == 0 {
					if e := lease.Check(ctx); e != nil {
						return e
					}
				}
				rows++
				return write(evidenceCells(group, sample, rule.Policies.Quality))
			})
			if e != nil {
				return e
			}
		}
		return nil
	})
}
func (s *Service) analysisEvidenceGroups(ctx context.Context, auth inputAccess, proof analysisArtifactProof, target analysisExportTarget) ([]evidenceGroup, error) {
	var run analysisRunTarget
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		var e error
		run, e = analysisRunTargetTx(ctx, tx, auth.Scope.Tenant, proof.Version.RunID)
		return e
	})
	if e != nil {
		return nil, e
	}
	if proof.Selection != nil {
		versions, e := s.evidenceVersions(ctx, auth.Scope.Tenant, proof.Version.Lineage)
		if e != nil {
			return nil, e
		}
		root := evidenceGroup{Configuration: jsonBytes(proof.Detail.Config), Versions: versions, Path: "root", Role: "candidate", Config: proof.Detail.Config.ID, Rule: run.Rule, Snapshot: proof.Manifest.Input, Direct: proof.Selection.Direct, Sources: proof.Selection.Sources}
		baselines := slices.DeleteFunc(slices.Clone(proof.Manifest.Baselines), func(b BaselineProof) bool { return !slices.Contains(proof.Selection.Baselines, b.Reference) })
		return s.evidenceGroups(ctx, auth, root, baselines)
	}
	plan, e := s.compile(run.Rule, "analysis", proof.Detail.Config.Body.Parameters)
	if e != nil {
		return nil, e
	}
	selected := []Output{}
	for _, output := range plan.Rule.Outputs {
		if slices.Contains(target.Outputs, output.Key) {
			selected = append(selected, output)
		}
	}
	plan.Rule.Outputs = selected
	output, e := proof.Version.OutputRange.rangeValue()
	if e != nil {
		return nil, e
	}
	if target.Range != nil {
		requested, e := target.Range.rangeValue()
		if e != nil {
			return nil, e
		}
		var ok bool
		output, ok = planning.Intersection(output, requested)
		if !ok {
			return nil, invalid("/output_range", "所选范围没有该成果输出。")
		}
	}
	cfg := ChannelConfig{ID: proof.Detail.Config.ID, Rule: run.Rule, InputBindings: proof.Detail.Config.Body.InputBindings, Parameters: proof.Detail.Config.Body.Parameters}
	needs, e := immediateInputNeeds(cfg, plan, output, analysisSource(&run))
	if e != nil {
		return nil, e
	}
	resolved, e := s.resolveInputs(ctx, auth, proof.Version.InputSnapshotID, monitoring.RawFreezeRequest{Needs: needs, History: true, Prefixes: inputPrefixes(plan, cfg, needs, output, analysisSource(&run))})
	if e != nil {
		return nil, e
	}
	versions, e := s.evidenceVersions(ctx, auth.Scope.Tenant, proof.Version.Lineage)
	if e != nil {
		return nil, e
	}
	root := evidenceGroup{Configuration: jsonBytes(proof.Detail.Config), Versions: versions, Path: "root", Role: "candidate", Config: cfg.ID, Rule: run.Rule, Snapshot: proof.Manifest.Input, Direct: resolved.Direct, Sources: resolved.Sources}
	return s.evidenceGroups(ctx, auth, root, proof.Manifest.Baselines)
}

func (s *Service) evidenceVersions(ctx context.Context, tenant, lineage string) (map[string]string, error) {
	var document map[string]json.RawMessage
	if e := s.results.ReadDocument(ctx, tenant, "lineage", lineage, &document); e != nil {
		return nil, e
	}
	out := map[string]string{}
	for _, key := range []string{"engine_version", "compiler_version", "plan_hash"} {
		var value string
		if e := json.Unmarshal(document[key], &value); e != nil || value == "" {
			return nil, problem("result_integrity_unavailable", "来源计算版本不完整。", 503)
		}
		out[key] = value
	}
	return out, nil
}
