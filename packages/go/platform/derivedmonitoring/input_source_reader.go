package derivedmonitoring

import (
	"container/heap"
	"context"
	"fmt"
	"slices"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/planning"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/resultstore"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoringvalue"
	"github.com/jackc/pgx/v5"
)

func splitInputKindsTx(ctx context.Context, tx pgx.Tx, tenant string, needs []monitoring.RawNeed) (direct, derived []monitoring.RawNeed, err error) {
	ids := []string{}
	for _, n := range needs {
		ids = append(ids, n.Channel)
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)
	rows, e := tx.Query(ctx, `SELECT id::text,source_kind FROM engineering_channels WHERE tenant_id=$1 AND id=ANY($2::uuid[]) AND deleted_at IS NULL`, tenant, ids)
	if e != nil {
		return nil, nil, e
	}
	kinds := map[string]string{}
	for rows.Next() {
		var id, kind string
		if e = rows.Scan(&id, &kind); e != nil {
			rows.Close()
			return nil, nil, e
		}
		kinds[id] = kind
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, nil, e
	}
	for _, n := range needs {
		switch kinds[n.Channel] {
		case "direct":
			direct = append(direct, n)
		case "derived":
			derived = append(derived, n)
		default:
			return nil, nil, notFound()
		}
	}
	return direct, derived, nil
}
func (s *Service) authorizeSourceOutputTx(ctx context.Context, tx pgx.Tx, auth inputAccess, channel string, r planning.Range) error {
	if auth.Permit != nil {
		if auth.History {
			return fmt.Errorf("system_history_not_allowed")
		}
		return s.AuthorizeRawSystemTx(ctx, tx, *auth.Permit, []monitoring.RawNeed{{Channel: channel, From: &r.From, To: r.To}}, false)
	}
	if auth.History {
		if e := requireAnyDerivedTx(ctx, tx, auth.Actor, Scope{Entry: auth.Scope.Entry, Tenant: auth.Scope.Tenant, Project: auth.Scope.Project}, "results.history"); e != nil {
			return e
		}
	}
	return s.monitoring.AuthorizeCalculationOutputTx(ctx, tx, auth.Actor, auth.Scope, channel, []monitoring.Window{rangeWindow(r)}, auth.History, auth.Capability)
}
func (s *Service) sourceManifest(ctx context.Context, tenant string, proof inputSourceProof) (resultManifest, error) {
	var manifest resultManifest
	if s.results == nil {
		return manifest, problem("calculation_storage_unavailable", "派生输入存储尚未配置。", 503)
	}
	if e := s.results.ReadDocument(ctx, tenant, "manifests", proof.Manifest, &manifest); e != nil {
		return manifest, e
	}
	if manifest.Schema != "derived-result-manifest/1" || manifest.Generation != proof.Generation || manifest.Channel != proof.Channel {
		return manifest, problem("result_integrity_unavailable", "上游正式清单身份不一致。", 503)
	}
	return manifest, nil
}

// resolveSourceProjection uses the original upstream roots from each manifest.
// It never substitutes today's upstream data while explaining/replaying an old
// generation. Only qualification uses today's authority and source changes.
func (s *Service) resolveSourceProjection(ctx context.Context, auth inputAccess, source inputSourceProof, part planning.Range, depth int) (inputSourceProof, error) {
	if depth > 8 {
		return source, problem("dependency_depth_exceeded", "上游来源深度超过上限。", 422)
	}
	source.Range = rangeDTO(part.From, part.To)
	if e := s.access.Transact(ctx, func(tx pgx.Tx) error { return s.authorizeSourceOutputTx(ctx, tx, auth, source.Channel, part) }); e != nil {
		return source, e
	}
	manifest, e := s.sourceManifest(ctx, auth.Scope.Tenant, source)
	if e != nil {
		return source, e
	}
	source.Config = manifest.Config
	source.Snapshot = manifest.Input.ID
	source.Baselines = manifest.Baselines
	source.Inputs = []inputSourceProof{}
	source.RawNeeds = []monitoring.RawNeed{}
	var direct, derived []monitoring.RawNeed
	var prefixes []monitoring.RawPrefix
	e = s.access.Transact(ctx, func(tx pgx.Tx) error {
		cfg, e := scanConfig(tx.QueryRow(ctx, configSelect+` WHERE tenant_id=$1 AND channel_id=$2 AND id=$3`, auth.Scope.Tenant, source.Channel, source.Config))
		if e != nil {
			return e
		}
		p, e := s.compile(cfg.Rule, "channel", cfg.Parameters)
		if e != nil {
			return e
		}
		if p.PlanHash != manifest.Plan {
			return problem("engine_version_unavailable", "上游固定计划无法由当前引擎解释。", 409)
		}
		needs, e := immediateInputNeeds(cfg, p, part)
		if e != nil {
			return e
		}
		prefixes = inputPrefixes(p, cfg, needs, part)
		direct, derived, e = splitInputKindsTx(ctx, tx, auth.Scope.Tenant, needs)
		return e
	})
	if e != nil {
		return source, e
	}
	if len(direct) > 0 {
		request := monitoring.RawFreezeRequest{Needs: direct, History: auth.History, Prefixes: selectInputPrefixes(prefixes, direct)}
		var resolved monitoring.RawSnapshotDetails
		if auth.Permit != nil {
			resolved, e = s.raw.ResolveDependencySystem(ctx, *auth.Permit, manifest.Input.ID, request)
		} else {
			resolved, e = s.raw.ResolvePartUser(ctx, auth.Actor, auth.Scope, manifest.Input.ID, request)
		}
		if e != nil {
			return source, e
		}
		if !auth.History && !auth.AllowChanged && resolved.State.Changed {
			return source, problem("upstream_pending", "上游相关来源已变化，等待正式结果更新。", 409)
		}
		source.RawNeeds = resolved.Candidates
	}
	if len(derived) > 0 {
		source.Inputs, e = s.resolveFrozenSources(ctx, auth, manifest.Sources, derived, selectInputPrefixes(prefixes, derived), depth+1)
		if e != nil {
			return source, e
		}
	}
	e = s.access.Transact(ctx, func(tx pgx.Tx) error {
		state, e := s.checkInputSourcesTx(ctx, tx, auth, []inputSourceProof{source})
		if e != nil {
			return e
		}
		if !auth.History && !auth.AllowChanged && !state.Current {
			return problem("upstream_pending", "上游正式结果已失效，等待更新。", 409)
		}
		return nil
	})
	return source, e
}
func selectInputPrefixes(prefixes []monitoring.RawPrefix, needs []monitoring.RawNeed) []monitoring.RawPrefix {
	ids := map[string]bool{}
	for _, n := range needs {
		ids[n.Channel] = true
	}
	out := []monitoring.RawPrefix{}
	for _, p := range prefixes {
		if ids[p.Channel] {
			out = append(out, p)
		}
	}
	return out
}
func (s *Service) currentSourceReferences(ctx context.Context, auth inputAccess, needs []monitoring.RawNeed, prefixes []monitoring.RawPrefix) ([]inputSourceProof, error) {
	if s.results == nil {
		return nil, problem("calculation_storage_unavailable", "派生输入存储尚未配置。", 503)
	}
	roots := map[string]string{}
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		for _, n := range needs {
			from := monitoringvalue.MinTime.UnixNano()
			if n.From != nil {
				from = *n.From
			}
			if e := s.authorizeSourceOutputTx(ctx, tx, auth, n.Channel, planning.Range{From: from, To: n.To}); e != nil {
				return e
			}
			if _, exists := roots[n.Channel]; exists {
				continue
			}
			var root string
			if e := tx.QueryRow(ctx, `SELECT COALESCE(current_root,'') FROM derived_channel_heads WHERE tenant_id=$1 AND channel_id=$2`, auth.Scope.Tenant, n.Channel).Scan(&root); e != nil {
				return e
			}
			roots[n.Channel] = root
		}
		return nil
	})
	if e != nil {
		return nil, e
	}
	search := append([]monitoring.RawNeed{}, needs...)
	for _, p := range prefixes {
		minimum := monitoringvalue.MinTime.UnixNano()
		if p.Before > minimum {
			search = append(search, monitoring.RawNeed{Channel: p.Channel, From: &minimum, To: &p.Before})
		}
	}
	search, e = normalizedNeeds(search)
	if e != nil {
		return nil, e
	}
	refs := []inputSourceProof{}
	for _, n := range search {
		entries, e := s.results.ReadRanges(ctx, auth.Scope.Tenant, roots[n.Channel], planning.Range{From: *n.From, To: n.To}, 10000)
		if e != nil {
			return nil, e
		}
		assignments, e := s.currentAssignments(ctx, auth.Scope.Tenant, n.Channel)
		if e != nil {
			return nil, e
		}
		for _, entry := range splitAssignmentEntries(entries, assignments) {
			part, ok := planning.Intersection(entry.Interval(), planning.Range{From: *n.From, To: n.To})
			if ok {
				refs = append(refs, inputSourceProof{Channel: n.Channel, Range: rangeDTO(part.From, part.To), Generation: entry.Generation, Manifest: entry.Manifest})
			}
		}
		if len(refs) > 10000 {
			return nil, problem("plan_limit_exceeded", "上游正式区间数量超过规划预算。", 422)
		}
	}
	return refs, nil
}

type timestampHeap []int64

func (h timestampHeap) Len() int           { return len(h) }
func (h timestampHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h timestampHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *timestampHeap) Push(v any)        { *h = append(*h, v.(int64)) }
func (h *timestampHeap) Pop() any          { old := *h; v := old[len(old)-1]; *h = old[:len(old)-1]; return v }

// Prefix discovery is internal and returns no user-visible samples. The exact
// candidate/non-hit interval is authorized after discovery, before using it.
func (s *Service) sourcePrefixStart(ctx context.Context, tenant string, refs []inputSourceProof, prefix monitoring.RawPrefix) (int64, error) {
	minimum := monitoringvalue.MinTime.UnixNano()
	if prefix.WholeHistory || prefix.Before <= minimum {
		return minimum, nil
	}
	if prefix.Count < 1 || prefix.Count > 1_000_000 {
		return 0, invalid("/prefixes", "前驱数量超出预算。")
	}
	best := &timestampHeap{}
	heap.Init(best)
	ordered := append([]inputSourceProof{}, refs...)
	slices.SortFunc(ordered, func(a, b inputSourceProof) int {
		ar, _ := a.Range.rangeValue()
		br, _ := b.Range.rangeValue()
		if ar.From > br.From {
			return -1
		}
		if ar.From < br.From {
			return 1
		}
		return 0
	})
	var scanned uint64
	for _, ref := range ordered {
		if ref.Channel != prefix.Channel {
			continue
		}
		r, e := ref.Range.rangeValue()
		if e != nil {
			return 0, e
		}
		if r.From >= prefix.Before {
			continue
		}
		if best.Len() == prefix.Count && r.To != nil && *r.To <= (*best)[0] {
			continue
		}
		manifest, e := s.sourceManifest(ctx, tenant, ref)
		if e != nil {
			return 0, e
		}
		for _, block := range manifest.Blocks {
			e = s.results.ReadBlock(ctx, block, func(row resultstore.Row) error {
				scanned++
				if scanned > 10_000_000 {
					return problem("budget_exceeded", "上游前驱检索超过单次预算。", 422)
				}
				if row.Generation != ref.Generation || row.Config != manifest.Config || row.Series != ref.Channel || row.Run != manifest.Run || row.Lineage != manifest.Lineage {
					return problem("result_integrity_unavailable", "上游结果块身份不一致。", 503)
				}
				if row.At >= prefix.Before || !r.Contains(row.At) || len(prefix.Qualities) > 0 && (row.Status != "valid" || !slices.Contains(prefix.Qualities, row.Quality)) {
					return nil
				}
				if best.Len() < prefix.Count {
					heap.Push(best, row.At)
				} else if row.At > (*best)[0] {
					(*best)[0] = row.At
					heap.Fix(best, 0)
				}
				return ctx.Err()
			})
			if e != nil {
				return 0, e
			}
		}
	}
	if best.Len() < prefix.Count {
		return minimum, nil
	}
	return (*best)[0], nil
}
func (s *Service) resolveFrozenSources(ctx context.Context, auth inputAccess, refs []inputSourceProof, needs []monitoring.RawNeed, prefixes []monitoring.RawPrefix, depth int) ([]inputSourceProof, error) {
	selected := append([]monitoring.RawNeed{}, needs...)
	for _, prefix := range prefixes {
		start, e := s.sourcePrefixStart(ctx, auth.Scope.Tenant, refs, prefix)
		if e != nil {
			return nil, e
		}
		if start < prefix.Before {
			selected = append(selected, monitoring.RawNeed{Channel: prefix.Channel, From: &start, To: &prefix.Before})
		}
	}
	selected, e := normalizedNeeds(selected)
	if e != nil {
		return nil, e
	}
	out := []inputSourceProof{}
	for _, need := range selected {
		wanted := planning.Range{From: *need.From, To: need.To}
		covered := []planning.Range{}
		for _, ref := range refs {
			if ref.Channel != need.Channel {
				continue
			}
			r, e := ref.Range.rangeValue()
			if e != nil {
				return nil, e
			}
			part, ok := planning.Intersection(wanted, r)
			if !ok {
				continue
			}
			proof, e := s.resolveSourceProjection(ctx, auth, ref, part, depth)
			if e != nil {
				return nil, e
			}
			covered = append(covered, part)
			out = append(out, proof)
			if len(out) > 10000 {
				return nil, problem("plan_limit_exceeded", "固定上游来源数量超过预算。", 422)
			}
		}
		complete, e := planning.Covered([]planning.Range{wanted}, covered)
		if e != nil {
			return nil, e
		}
		if !complete {
			return nil, problem("upstream_pending", "上游尚未正式计算所需的输入或前驱范围，请明确补算该范围。", 409)
		}
	}
	return out, nil
}
func (s *Service) resolveCurrentSources(ctx context.Context, auth inputAccess, needs []monitoring.RawNeed, prefixes []monitoring.RawPrefix) ([]inputSourceProof, error) {
	refs, e := s.currentSourceReferences(ctx, auth, needs, prefixes)
	if e != nil {
		return nil, e
	}
	return s.resolveFrozenSources(ctx, auth, refs, needs, prefixes, 1)
}
