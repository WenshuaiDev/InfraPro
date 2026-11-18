package derivedmonitoring

import (
	"context"
	"errors"
	"slices"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/compute"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/planning"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/jackc/pgx/v5"
)

// This is a server-created projection of an immutable artifact, never accepted
// from an HTTP request. Jobs persist it with their immutable target so every
// execution and download boundary can requalify the same candidate intervals.
type analysisSelection struct {
	Range        NanosecondRange      `json:"range"`
	Outputs      []string             `json:"outputs"`
	Direct       []monitoring.RawNeed `json:"direct"`
	Sources      []inputSourceProof   `json:"sources"`
	Baselines    []string             `json:"baselines"`
	Requirements []monitoring.RawNeed `json:"requirements"`
	Partial      bool                 `json:"partial"`
}

func staleAnalysisInput(e error) bool {
	var raw *monitoring.Error
	var de *Error
	return errors.As(e, &raw) && raw.Code == "snapshot_stale" || errors.As(e, &de) && de.Code == "snapshot_stale"
}

func selectedBaselines(p *compute.Plan) []string {
	reachable := map[string]bool{}
	for _, o := range p.Rule.Outputs {
		reachable["node:"+o.Node] = true
	}
	refs := []string{}
	for i := len(p.Rule.Nodes) - 1; i >= 0; i-- {
		n := p.Rule.Nodes[i]
		if !reachable["node:"+n.ID] {
			continue
		}
		if b := n.Params.Baseline; b != nil && b.Kind != "constant" {
			refs = append(refs, b.Reference)
		}
		for _, ref := range n.Inputs {
			reachable[ref] = true
		}
		for _, ref := range n.Params.Variables {
			reachable[ref] = true
		}
	}
	slices.Sort(refs)
	return slices.Compact(refs)
}

func comparisonOutputKeys(mapping []AnalysisOutputMapping, left bool) []string {
	keys := []string{}
	for _, pair := range mapping {
		key := pair.Right
		if left {
			key = pair.Left
		}
		if key != "" {
			keys = append(keys, key)
		}
	}
	return keys
}

func (s *Service) loadSelectedAnalysis(ctx context.Context, a access.Actor, scope Scope, artifact string, in AnalysisArtifactQuery, action string) (analysisArtifactProof, error) {
	var out analysisArtifactProof
	var target analysisRunTarget
	e := s.transact(ctx, a, scope, action, false, func(tx pgx.Tx) error {
		var e error
		out, e = s.analysisArtifactIdentityTx(ctx, tx, a, scope, artifact, in.VersionID)
		if e != nil {
			return e
		}
		target, e = analysisRunTargetTx(ctx, tx, scope.Tenant, out.Version.RunID)
		return e
	})
	if e != nil {
		return out, e
	}
	if e = s.loadAnalysisManifest(ctx, scope, artifact, &out); e != nil {
		return out, e
	}
	keys, e := analysisExportSelection(out, AnalysisExportInput{Range: in.Range, Outputs: in.Outputs})
	if e != nil {
		return out, e
	}
	output, e := out.Version.OutputRange.rangeValue()
	if e != nil {
		return out, e
	}
	if in.Range != nil {
		requested, e := in.Range.rangeValue()
		if e != nil {
			return out, e
		}
		intersection, ok := planning.Intersection(output, requested)
		if !ok {
			// There is no result dependency to project. Preserve the ordinary
			// full-version qualification before allowing an empty query or the
			// absent side of a comparison; never use an empty interval as an
			// authorization shortcut to its metadata.
			return s.loadAnalysisArtifact(ctx, a, scope, artifact, out.Version.ID, action)
		}
		output = intersection
	}
	p, e := s.compile(target.Rule, "analysis", out.Detail.Config.Body.Parameters)
	if e != nil {
		return out, e
	}
	p.Rule.Outputs = slices.DeleteFunc(p.Rule.Outputs, func(o Output) bool { return !slices.Contains(keys, o.Key) })
	if in.IncludeSamples && p.PlanHash == out.Manifest.Plan {
		out.PreviewPlan = p
	}
	cfg := ChannelConfig{ID: out.Detail.Config.ID, Rule: target.Rule, InputBindings: out.Detail.Config.Body.InputBindings, Parameters: out.Detail.Config.Body.Parameters}
	needs, e := immediateInputNeeds(cfg, p, output, analysisSource(&target))
	if e != nil {
		return out, e
	}
	request := monitoring.RawFreezeRequest{Needs: needs, Prefixes: inputPrefixes(p, cfg, needs, output, analysisSource(&target))}
	auth := inputAccess{Actor: a, Scope: monitoringScope(scope), AllowChanged: true}
	resolved, e := s.resolveInputs(ctx, auth, out.Version.InputSnapshotID, request)
	if staleAnalysisInput(e) {
		auth.History = true
		resolved, e = s.resolveInputs(ctx, auth, out.Version.InputSnapshotID, request)
	}
	if e != nil {
		return out, e
	}
	part := &analysisSelection{Range: rangeDTO(output.From, output.To), Outputs: keys, Direct: resolved.Direct, Sources: resolved.Sources, Baselines: selectedBaselines(p)}
	part.Partial = fingerprint(part.Range) != fingerprint(out.Version.OutputRange) || len(keys) != len(out.Manifest.Outputs)
	e = s.transact(ctx, a, scope, action, false, func(tx pgx.Tx) error {
		baselines := slices.DeleteFunc(slices.Clone(out.Manifest.Baselines), func(b BaselineProof) bool { return !slices.Contains(part.Baselines, b.Reference) })
		if len(baselines) != len(part.Baselines) {
			return problem("result_integrity_unavailable", "所选输出的固定基准依据缺失。", 503)
		}
		selectionInput := resolved
		selectionInput.Needs = part.Direct
		part.Requirements, e = s.analysisAffectedNeedsTx(ctx, tx, scope.Tenant, selectionInput, cfg.ID, baselines)
		if e != nil {
			return e
		}
		current, e := s.analysisArtifactSelectedTx(ctx, tx, a, scope, artifact, out.Version.ID, part)
		if e == nil {
			out.Detail = current.Detail
			out.Detail.Outputs = slices.DeleteFunc(slices.Clone(out.Manifest.Outputs), func(o Output) bool { return !slices.Contains(keys, o.Key) })
			out.Selection = part
		}
		return e
	})
	return out, e
}

func (s *Service) selectedAnalysisRevokeTx(ctx context.Context, tx pgx.Tx, scope Scope, version string, selection *analysisSelection) (string, error) {
	if selection == nil {
		return analysisRevokePositionTx(ctx, tx, scope, version)
	}
	return evidenceRevokePositionTx(ctx, tx, scope.Tenant, []evidenceGroup{{Direct: selection.Requirements}})
}
