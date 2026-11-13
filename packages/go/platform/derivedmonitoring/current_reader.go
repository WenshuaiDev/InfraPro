package derivedmonitoring

import (
	"container/heap"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/planning"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/resultstore"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoringvalue"
	"github.com/jackc/pgx/v5"
)

func (s *Service) FreezeCalculatedTx(ctx context.Context, tx pgx.Tx, tenant string, channels []string) (map[string]monitoring.CalculationHead, error) {
	out := map[string]monitoring.CalculationHead{}
	rows, e := tx.Query(ctx, `SELECT c.channel_id::text,COALESCE(h.current_root,''),h.head_revision::text,c.config_revision::text,c.control_revision::text,c.control_state,c.health FROM derived_channels c JOIN derived_channel_heads h USING(tenant_id,channel_id) WHERE c.tenant_id=$1 AND c.channel_id=ANY($2::uuid[]) ORDER BY c.channel_id`, tenant, channels)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var h monitoring.CalculationHead
		if e = rows.Scan(&id, &h.Root, &h.Revision, &h.ConfigRevision, &h.ControlRevision, &h.ControlState, &h.Health); e != nil {
			return nil, e
		}
		out[id] = h
	}
	return out, rows.Err()
}
func outputPoint(row resultstore.Row) monitoring.CalculatedObservation { return row.Observation() }

func compareResult(a, b monitoring.CalculationBoundary) int {
	if a.At < b.At {
		return -1
	}
	if a.At > b.At {
		return 1
	}
	for _, pair := range [][2]string{{a.Channel, b.Channel}, {a.Result, b.Result}, {a.Generation, b.Generation}} {
		if n := strings.Compare(pair[0], pair[1]); n != 0 {
			return n
		}
	}
	return 0
}
func resultBoundary(p monitoring.CalculatedObservation) monitoring.CalculationBoundary {
	return monitoring.CalculationBoundary{At: p.AtNS, Channel: p.Channel, Result: p.Result, Generation: p.Generation}
}

type pointHeap struct {
	points  []monitoring.CalculatedObservation
	reverse bool
}

func (h pointHeap) Len() int { return len(h.points) }
func (h pointHeap) Less(i, j int) bool {
	n := compareResult(resultBoundary(h.points[i]), resultBoundary(h.points[j]))
	if h.reverse {
		return n < 0
	}
	return n > 0
}
func (h pointHeap) Swap(i, j int) { h.points[i], h.points[j] = h.points[j], h.points[i] }
func (h *pointHeap) Push(v any)   { h.points = append(h.points, v.(monitoring.CalculatedObservation)) }
func (h *pointHeap) Pop() any {
	v := h.points[len(h.points)-1]
	h.points = h.points[:len(h.points)-1]
	return v
}
func rangeWindow(r planning.Range) monitoring.Window {
	return monitoring.Window{From: &r.From, To: r.To}
}
func coverage(channel string, r planning.Range, state, generation string) monitoring.CalculationCoverage {
	from := time.Unix(0, r.From).UTC().Format(time.RFC3339Nano)
	var to *string
	if r.To != nil {
		v := time.Unix(0, *r.To).UTC().Format(time.RFC3339Nano)
		to = &v
	}
	return monitoring.CalculationCoverage{Channel: channel, Range: monitoring.Range{From: &from, To: to}, State: state, Generation: generation}
}

type readProof struct {
	coverageIndex int
	sources       []inputSourceProof
	prepared      bool
	channel       string
	rangeValue    planning.Range
	manifest      resultManifest
	needs         []monitoring.RawNeed
	current       bool
	capability    string
}

func (s *Service) prepareResultPart(ctx context.Context, a access.Actor, scope monitoring.Scope, proof *readProof) (bool, error) {
	started := time.Now()
	defer func() { monitoring.RecordCalculationReadDuration(ctx, "read_qualify", time.Since(started)) }()
	if !proof.prepared {
		var request monitoring.RawFreezeRequest
		e := s.access.Transact(ctx, func(tx pgx.Tx) error {
			// The complete selected output ranges were authorized together by
			// readCalculated before reading any immutable manifests. Current
			// output and recursive input authority is checked again below.
			cfg, e := scanConfig(tx.QueryRow(ctx, configSelect+` WHERE tenant_id=$1 AND channel_id=$2 AND id=$3`, scope.Tenant, proof.channel, proof.manifest.Config))
			if e != nil {
				return e
			}
			p, e := s.compile(cfg.Rule, "channel", cfg.Parameters)
			if e != nil {
				return e
			}
			if p.PlanHash != proof.manifest.Plan {
				return problem("engine_version_unavailable", "固定计划无法由当前引擎解释。", 409)
			}
			request.Needs, e = s.plannedInputsTx(ctx, tx, scope.Tenant, cfg, p, proof.rangeValue)
			if e != nil {
				return e
			}
			request.Prefixes = inputPrefixes(p, cfg, request.Needs, proof.rangeValue)
			return nil
		})
		if e != nil {
			return false, e
		}
		resolved, e := s.resolveInputs(ctx, inputAccess{Actor: a, Scope: scope, Capability: proof.capability}, proof.manifest.Input.ID, request)
		var raw *monitoring.Error
		if errors.As(e, &raw) && raw.Code == "snapshot_stale" {
			return false, nil
		}
		var upstream *Error
		if errors.As(e, &upstream) && upstream.Code == "upstream_pending" {
			return false, nil
		}
		if e != nil {
			return false, e
		}
		proof.needs = resolved.Direct
		proof.sources = resolved.Sources
		proof.prepared = true
		if resolved.State.Changed {
			return false, nil
		}
	}
	return true, nil
}

func calculationProofChanged(e error) bool {
	var raw *monitoring.Error
	var derived *Error
	return errors.As(e, &raw) && (raw.Code == "snapshot_stale" || raw.Code == "export_invalidated") || errors.As(e, &derived) && derived.Code == "export_invalidated"
}

func (s *Service) qualifyResultPart(ctx context.Context, a access.Actor, scope monitoring.Scope, proof *readProof) (bool, error) {
	current, e := s.prepareResultPart(ctx, a, scope, proof)
	if e != nil || !current {
		return current, e
	}
	e = s.access.Transact(ctx, func(tx pgx.Tx) error {
		return s.CheckCalculationProofsTx(ctx, tx, a, scope, []json.RawMessage{jsonBytes(proof.ticket())}, proof.capability)
	})
	if calculationProofChanged(e) {
		return false, nil
	}
	return e == nil, e
}

func (proof readProof) ticket() calculationReadProof {
	return calculationReadProof{Schema: "calculation-read-proof/1", Channel: proof.channel, Range: rangeDTO(proof.rangeValue.From, proof.rangeValue.To), State: "current", Generation: proof.manifest.Generation, Config: proof.manifest.Config, Snapshot: proof.manifest.Input.ID, Needs: proof.needs, Sources: proof.sources, Baselines: proof.manifest.Baselines}
}
func (s *Service) qualifyUncomputed(ctx context.Context, a access.Actor, scope monitoring.Scope, channel string, r planning.Range, capability string) error {
	return s.access.Transact(ctx, func(tx pgx.Tx) error { return s.qualifyUncomputedTx(ctx, tx, a, scope, channel, r, capability) })
}
func (s *Service) qualifyUncomputedTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope monitoring.Scope, channel string, r planning.Range, capability string) error {
	if e := s.monitoring.AuthorizeCalculationOutputTx(ctx, tx, a, scope, channel, []monitoring.Window{rangeWindow(r)}, false, capability); e != nil {
		return e
	}
	assignments, e := activeAssignmentsTx(ctx, tx, scope.Tenant, channel)
	if e != nil {
		return e
	}
	missing := []planning.Range{r}
	for _, assignment := range assignments {
		part, _ := assignment.Range.rangeValue()
		if selected, ok := planning.Intersection(r, part); ok {
			if e = s.authorizeUncomputedConfigTx(ctx, tx, a, scope, channel, assignment.ConfigID, selected, capability); e != nil {
				return e
			}
			next := []planning.Range{}
			for _, old := range missing {
				next = append(next, planning.Subtract(old, selected)...)
			}
			missing = next
		}
	}
	if len(missing) == 0 {
		return nil
	}
	var config string
	if e = tx.QueryRow(ctx, `SELECT draft_config_id::text FROM derived_channels WHERE tenant_id=$1 AND channel_id=$2`, scope.Tenant, channel).Scan(&config); e != nil {
		return e
	}
	for _, part := range missing {
		if e = s.authorizeUncomputedConfigTx(ctx, tx, a, scope, channel, config, part, capability); e != nil {
			return e
		}
	}
	return nil
}
func (s *Service) authorizeUncomputedConfigTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope monitoring.Scope, channel, config string, r planning.Range, capability string) error {
	cfg, e := scanConfig(tx.QueryRow(ctx, configSelect+` WHERE tenant_id=$1 AND channel_id=$2 AND id=$3`, scope.Tenant, channel, config))
	if e != nil {
		return e
	}
	p, e := s.compile(cfg.Rule, "channel", cfg.Parameters)
	if e != nil {
		return e
	}
	needs, e := s.plannedInputsTx(ctx, tx, scope.Tenant, cfg, p, r)
	if e != nil {
		return e
	}
	baselineNeeds, e := s.baselineNeedsTx(ctx, tx, scope.Tenant, cfg, p)
	if e != nil {
		return e
	}
	needs = append(needs, baselineNeeds...)
	return s.authorizeInputsTx(ctx, tx, a, scope, needs, false, capability)
}
func (s *Service) ReadCalculated(ctx context.Context, a access.Actor, scope monitoring.Scope, in monitoring.CalculationQuery) (monitoring.CalculationPage, error) {
	return s.readCalculated(ctx, a, scope, in, 1_000_000, nil, false)
}
func (s *Service) ScanCalculated(ctx context.Context, a access.Actor, scope monitoring.Scope, in monitoring.CalculationQuery, budget uint64, consume func(monitoring.CalculatedObservation) error) ([]monitoring.CalculationCoverage, error) {
	if consume == nil || budget == 0 {
		return nil, invalid("/budget", "读取预算与消费函数不能为空。")
	}
	out, e := s.readCalculated(ctx, a, scope, in, budget, consume, false)
	return out.Coverage, e
}
func (s *Service) CheckCalculated(ctx context.Context, a access.Actor, scope monitoring.Scope, in monitoring.CalculationQuery) ([]monitoring.CalculationCoverage, error) {
	out, e := s.readCalculated(ctx, a, scope, in, 0, func(monitoring.CalculatedObservation) error { return nil }, true)
	return out.Coverage, e
}
func (s *Service) readCalculated(ctx context.Context, a access.Actor, scope monitoring.Scope, in monitoring.CalculationQuery, budget uint64, consume func(monitoring.CalculatedObservation) error, checkOnly bool) (monitoring.CalculationPage, error) {
	out := monitoring.CalculationPage{Items: []monitoring.CalculatedObservation{}, Coverage: []monitoring.CalculationCoverage{}}
	if s.results == nil {
		return out, problem("calculation_storage_unavailable", "派生结果存储尚未配置。", 503)
	}
	if in.History {
		return out, invalid("/generation_id", "派生历史读取需要明确的历史代次。")
	}
	if consume == nil && (in.Limit < 1 || in.Limit > 1001) {
		return out, invalid("/limit", "派生结果分页预算无效。")
	}
	best := &pointHeap{points: []monitoring.CalculatedObservation{}, reverse: in.Reverse}
	heap.Init(best)
	proofs := []readProof{}
	manifestCache := map[string]resultManifest{}
	prepared := map[string]calculationReadProof{}
	for _, raw := range in.PreparedProofs {
		var proof calculationReadProof
		if e := json.Unmarshal(raw, &proof); e != nil || proof.Schema != "calculation-read-proof/1" {
			return out, problem("result_integrity_unavailable", "计算结果读取证明不完整。", 503)
		}
		prepared[fingerprint([]any{proof.Channel, proof.Range, proof.Generation, proof.Config, proof.Snapshot})] = proof
	}
	ids := []string{}
	for id := range in.Heads {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	requestedWindows := map[string][]monitoring.Window{}
	for _, channel := range ids {
		for _, allowed := range in.Windows[channel] {
			requested := allowed
			if in.Range != nil {
				var ok bool
				requested, ok = requested.Intersect(*in.Range)
				if !ok {
					continue
				}
			}
			requestedWindows[channel] = append(requestedWindows[channel], requested)
		}
	}
	assignmentsByChannel := map[string][]ConfigAssignment{}
	if e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		if e := s.monitoring.AuthorizeCalculationOutputsTx(ctx, tx, a, scope, requestedWindows, false, in.Capability); e != nil {
			return e
		}
		for _, channel := range ids {
			var e error
			assignmentsByChannel[channel], e = activeAssignmentsTx(ctx, tx, scope.Tenant, channel)
			if e != nil {
				return e
			}
		}
		return nil
	}); e != nil {
		return out, e
	}
	var scanned uint64
	for _, channel := range ids {
		head := in.Heads[channel]
		assignments := assignmentsByChannel[channel]
		for _, requested := range requestedWindows[channel] {
			from := monitoringvalue.MinTime.UnixNano()
			if requested.From != nil {
				from = *requested.From
			}
			wanted := planning.Range{From: from, To: requested.To}
			entries, e := s.results.ReadRanges(ctx, scope.Tenant, head.Root, wanted, 10000)
			if e != nil {
				return out, e
			}
			entries = splitAssignmentEntries(entries, assignments)
			if in.Reverse {
				slices.Reverse(entries)
			}
			missing := []planning.Range{wanted}
			for _, entry := range entries {
				part, ok := planning.Intersection(wanted, entry.Interval())
				if !ok {
					continue
				}
				next := []planning.Range{}
				for _, r := range missing {
					next = append(next, planning.Subtract(r, part)...)
				}
				missing = next
				manifest, ok := manifestCache[entry.Manifest]
				if !ok {
					if e = s.results.ReadDocument(ctx, scope.Tenant, "manifests", entry.Manifest, &manifest); e != nil {
						return out, e
					}
					manifestCache[entry.Manifest] = manifest
				}
				if manifest.Schema != "derived-result-manifest/1" || manifest.Channel != channel || manifest.Generation != entry.Generation || manifest.Input.SourcePosition != entry.SourcePosition {
					return out, problem("result_integrity_unavailable", "正式结果清单不完整。", 503)
				}
				proof := readProof{channel: channel, rangeValue: part, manifest: manifest, capability: in.Capability}
				if fixed, ok := prepared[fingerprint([]any{channel, rangeDTO(part.From, part.To), manifest.Generation, manifest.Config, manifest.Input.ID})]; ok && fixed.State == "current" {
					proof.prepared = true
					proof.needs = fixed.Needs
					proof.sources = fixed.Sources
				}
				current, e := s.prepareResultPart(ctx, a, scope, &proof)
				if e != nil {
					return out, e
				}
				proof.current = current
				proof.coverageIndex = len(out.Coverage)
				proofs = append(proofs, proof)
				out.Coverage = append(out.Coverage, coverage(channel, part, "pending", entry.Generation))
			}
			for _, r := range missing {
				if e = s.qualifyUncomputed(ctx, a, scope, channel, r, in.Capability); e != nil {
					return out, e
				}
				missing := coverage(channel, r, "not_computed", "")
				missing.Proof = jsonBytes(calculationReadProof{Schema: "calculation-read-proof/1", Channel: channel, Range: rangeDTO(r.From, r.To), State: "not_computed"})
				out.Coverage = append(out.Coverage, missing)
			}
		}
	}
	// Immutable dependency discovery may perform physical prefix searches.
	// Qualify all prepared generations together before reading any payload.
	// Only a concurrently changed set falls back to per-part pending status.
	candidates := []json.RawMessage{}
	for _, proof := range proofs {
		if proof.current {
			candidates = append(candidates, jsonBytes(proof.ticket()))
		}
	}
	if len(candidates) > 0 {
		started := time.Now()
		e := s.access.Transact(ctx, func(tx pgx.Tx) error {
			return s.CheckCalculationProofsTx(ctx, tx, a, scope, candidates, in.Capability)
		})
		monitoring.RecordCalculationReadDuration(ctx, "read_current", time.Since(started))
		if e != nil {
			if !calculationProofChanged(e) {
				return out, e
			}
			for i := range proofs {
				if proofs[i].current {
					proofs[i].current, e = s.qualifyResultPart(ctx, a, scope, &proofs[i])
					if e != nil {
						return out, e
					}
				}
			}
		}
	}
	for _, proof := range proofs {
		if !proof.current {
			continue
		}
		channel, part, manifest := proof.channel, proof.rangeValue, proof.manifest
		qualified := coverage(channel, part, "current", manifest.Generation)
		qualified.Proof = jsonBytes(proof.ticket())
		out.Coverage[proof.coverageIndex] = qualified
		if checkOnly {
			continue
		}
		var e error
		blocks := slices.Clone(manifest.Blocks)
		if in.Reverse {
			slices.Reverse(blocks)
		}
		for _, block := range blocks {
			// The index is part of the content-addressed manifest, sealed only after
			// both replicas and the object payload have passed full verification.
			if block.First != nil && block.Last != nil {
				if *block.Last < part.From || part.To != nil && *block.First >= *part.To {
					continue
				}
				if in.Boundary != nil && (in.Reverse && *block.First > in.Boundary.At || !in.Reverse && *block.Last < in.Boundary.At) {
					continue
				}
				if consume == nil && best.Len() == in.Limit && (in.Reverse && *block.Last < best.points[0].AtNS || !in.Reverse && *block.First > best.points[0].AtNS) {
					continue
				}
			}
			if consume != nil && in.Summary != nil && block.Summary != nil && block.First != nil && block.Last != nil && part.Contains(*block.First) && part.Contains(*block.Last) {
				if e = in.Summary(block.Summary); e != nil {
					return out, e
				}
				continue
			}
			accept := func(row resultstore.Row) error {
				scanned++
				if scanned > budget {
					if consume != nil {
						return monitoring.ErrCalculationBudget
					}
					return problem("query_budget_exceeded", "结果读取超过交互预算，请缩小范围或使用后台分析。", 422)
				}
				if row.Generation != manifest.Generation || row.Config != manifest.Config || row.Series != channel || row.Run != manifest.Run || row.Lineage != manifest.Lineage {
					return fmt.Errorf("result_identity_mismatch")
				}
				if !part.Contains(row.At) || !slices.Contains(in.Qualities, row.Quality) || in.ValidOnly && row.Status != "valid" {
					return nil
				}
				point := outputPoint(row)
				if consume != nil {
					return consume(point)
				}
				key := resultBoundary(point)
				if in.Boundary != nil {
					n := compareResult(key, *in.Boundary)
					if in.Reverse && n >= 0 || !in.Reverse && n <= 0 {
						return nil
					}
				}
				if best.Len() < in.Limit {
					heap.Push(best, point)
				} else {
					n := compareResult(key, resultBoundary(best.points[0]))
					if in.Reverse && n > 0 || !in.Reverse && n < 0 {
						best.points[0] = point
						heap.Fix(best, 0)
					}
				}
				return nil
			}
			if in.EndpointsOnly && in.Reverse && in.Limit == 1 && consume == nil && in.Boundary == nil && block.Endpoints != nil && block.First != nil && block.Last != nil && part.Contains(*block.First) && part.Contains(*block.Last) {
				if in.ValidOnly {
					for _, point := range block.Endpoints.LastValid {
						if e = accept(point); e != nil {
							break
						}
					}
				} else {
					e = accept(block.Endpoints.Last)
				}
			} else {
				started := time.Now()
				e = s.results.ReadBlock(ctx, block, accept)
				monitoring.RecordCalculationReadDuration(ctx, "read_payload", time.Since(started))
			}
			if e != nil {
				return out, e
			}
		}
	}
	// No values or counts are returned after a concurrent revocation. A changed
	// proof is retried by the caller from a fresh view, never relabelled current.
	qualified := []json.RawMessage{}
	for _, part := range out.Coverage {
		if part.State == "current" {
			qualified = append(qualified, part.Proof)
		}
	}
	if len(qualified) > 0 {
		started := time.Now()
		e := s.access.Transact(ctx, func(tx pgx.Tx) error {
			return s.CheckCalculationProofsTx(ctx, tx, a, scope, qualified, in.Capability)
		})
		monitoring.RecordCalculationReadDuration(ctx, "read_recheck", time.Since(started))
		if e != nil {
			var changed *Error
			var raw *monitoring.Error
			if consume == nil && (errors.As(e, &changed) && changed.Code == "export_invalidated" || errors.As(e, &raw) && raw.Code == "export_invalidated") {
				e = problem("snapshot_stale", "相关输入已变化，请刷新当前结果。", 409)
			}
			return monitoring.CalculationPage{}, e
		}
	}
	if consume != nil {
		return out, nil
	}
	for _, proof := range proofs {
		// Pending coverage still repeats authorization, even though it carries
		// no output values. Current proofs were checked together above.
		if !proof.current {
			if _, e := s.qualifyResultPart(ctx, a, scope, &proof); e != nil {
				return monitoring.CalculationPage{}, e
			}
		}
	}
	out.Items = best.points
	slices.SortFunc(out.Items, func(a, b monitoring.CalculatedObservation) int {
		n := compareResult(resultBoundary(a), resultBoundary(b))
		if in.Reverse {
			return -n
		}
		return n
	})
	return out, nil
}
