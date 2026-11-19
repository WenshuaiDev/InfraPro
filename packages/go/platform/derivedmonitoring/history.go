package derivedmonitoring

import (
	"container/heap"
	"context"
	"slices"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/planning"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/resultstore"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/jackc/pgx/v5"
)

type Generation struct {
	ID                          string            `json:"id"`
	ChannelID                   string            `json:"channel_id"`
	RunID                       string            `json:"run_id"`
	ConfigID                    string            `json:"config_id"`
	Range                       NanosecondRange   `json:"output_range"`
	Rows                        *string           `json:"row_count"`
	States                      map[string]string `json:"status_counts,omitempty"`
	SourcePosition              string            `json:"source_position"`
	PublishPosition             string            `json:"publish_position"`
	InputState                  string            `json:"input_state"`
	CreatedAt                   time.Time         `json:"created_at"`
	Partial                     bool              `json:"partial"`
	manifest, lineage, snapshot string
}
type GenerationPage struct {
	Items      []Generation `json:"items"`
	NextCursor string       `json:"next_cursor"`
}
type GenerationFilter struct {
	Range  NanosecondRange
	Cursor string
	Limit  int
}
type GenerationQuery struct {
	Range  *NanosecondRange `json:"output_range,omitempty"`
	Cursor string           `json:"cursor,omitempty"`
	Limit  int              `json:"limit"`
}
type GenerationDetails struct {
	Generation Generation                         `json:"generation"`
	Items      []monitoring.CalculatedObservation `json:"items"`
	NextCursor string                             `json:"next_cursor"`
	HasMore    bool                               `json:"has_more"`
}
type GenerationLineage struct {
	Sources       []inputSourceProof     `json:"upstream_inputs"`
	Generation    Generation             `json:"generation"`
	Config        ChannelConfig          `json:"config"`
	InputSnapshot monitoring.RawSnapshot `json:"input_snapshot"`
	InputRanges   []monitoring.RawNeed   `json:"input_ranges"`
	Baselines     []BaselineProof        `json:"baselines"`
	ManifestHash  string                 `json:"manifest_hash"`
	LineageHash   string                 `json:"lineage_hash"`
}

const generationSelect = `SELECT id::text,channel_id::text,run_id::text,config_id::text,lower(output_range),upper(output_range),row_count::text,status_counts,source_position::text,publish_position::text,created_at,manifest_root,lineage_root,raw_snapshot_id::text FROM derived_generations`

func scanGeneration(row pgx.Row) (Generation, error) {
	var g Generation
	var from int64
	var to *int64
	e := row.Scan(&g.ID, &g.ChannelID, &g.RunID, &g.ConfigID, &from, &to, &g.Rows, &g.States, &g.SourcePosition, &g.PublishPosition, &g.CreatedAt, &g.manifest, &g.lineage, &g.snapshot)
	g.Range = rangeDTO(from, to)
	if e == pgx.ErrNoRows {
		e = notFound()
	}
	return g, e
}

type historyProof struct {
	sources    []inputSourceProof
	generation Generation
	manifest   resultManifest
	config     ChannelConfig
	output     planning.Range
	needs      []monitoring.RawNeed
}

func (s *Service) historyGeneration(ctx context.Context, a access.Actor, scope Scope, id string, selected *NanosecondRange, action string) (historyProof, error) {
	var proof historyProof
	if !access.ValidID(id) {
		return proof, notFound()
	}
	if s.results == nil || s.raw == nil {
		return proof, problem("calculation_storage_unavailable", "派生历史存储尚未配置。", 503)
	}
	var request monitoring.RawFreezeRequest
	e := s.transact(ctx, a, scope, action, false, func(tx pgx.Tx) error {
		var e error
		proof.generation, e = scanGeneration(tx.QueryRow(ctx, generationSelect+` WHERE tenant_id=$1 AND id=$2`, scope.Tenant, id))
		if e != nil {
			return e
		}
		g := proof.generation
		if _, e = s.channelTx(ctx, tx, a, scope, g.ChannelID); e != nil {
			return e
		}
		output, e := g.Range.rangeValue()
		if e != nil {
			return e
		}
		proof.output = output
		if selected != nil {
			requested, e := selected.rangeValue()
			if e != nil {
				return e
			}
			part, ok := planning.Intersection(output, requested)
			if !ok {
				return invalid("/output_range", "此代次未发布所选区间。")
			}
			proof.output = part
			proof.generation.Partial = rangeSQL(output) != rangeSQL(part)
		}
		if e = s.monitoring.AuthorizeCalculationOutputTx(ctx, tx, a, monitoringScope(scope), g.ChannelID, []monitoring.Window{rangeWindow(proof.output)}, true); e != nil {
			return e
		}
		proof.config, e = scanConfig(tx.QueryRow(ctx, configSelect+` WHERE tenant_id=$1 AND channel_id=$2 AND id=$3`, scope.Tenant, g.ChannelID, g.ConfigID))
		if e != nil {
			return e
		}
		p, e := s.compile(proof.config.Rule, "channel", proof.config.Parameters)
		if e != nil {
			return e
		}
		request.Needs, e = s.plannedInputsTx(ctx, tx, scope.Tenant, proof.config, p, proof.output)
		if e != nil {
			return e
		}
		request.Prefixes = inputPrefixes(p, proof.config, request.Needs, proof.output)
		request.History = true
		return nil
	})
	if e != nil {
		return proof, e
	}
	resolved, e := s.resolveInputs(ctx, inputAccess{Actor: a, Scope: monitoringScope(scope), History: true}, proof.generation.snapshot, request)
	if e != nil {
		return proof, e
	}
	proof.needs = resolved.Direct
	proof.sources = resolved.Sources
	if e = s.results.ReadDocument(ctx, scope.Tenant, "manifests", proof.generation.manifest, &proof.manifest); e != nil {
		return proof, e
	}
	g, m := proof.generation, proof.manifest
	if m.Schema != "derived-result-manifest/1" || m.Generation != g.ID || m.Channel != g.ChannelID || m.Run != g.RunID || m.Config != g.ConfigID || m.Input.ID != g.snapshot || m.Lineage != g.lineage || m.Input.SourcePosition != g.SourcePosition {
		return proof, problem("result_integrity_unavailable", "历史代次与来源清单不一致。", 503)
	}
	if e = s.recheckHistory(ctx, a, scope, &proof, action); e != nil {
		return proof, e
	}
	proof.generation.Range = rangeDTO(proof.output.From, proof.output.To)
	if proof.generation.Partial {
		proof.generation.Rows = nil
		proof.generation.States = nil
	}
	return proof, nil
}
func (s *Service) recheckHistory(ctx context.Context, a access.Actor, scope Scope, proof *historyProof, action string) error {
	return s.transact(ctx, a, scope, action, false, func(tx pgx.Tx) error { return s.recheckHistoryTx(ctx, tx, a, scope, proof) })
}
func (s *Service) recheckHistoryTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, proof *historyProof) error {
	g := proof.generation
	if e := s.monitoring.AuthorizeCalculationOutputTx(ctx, tx, a, monitoringScope(scope), g.ChannelID, []monitoring.Window{rangeWindow(proof.output)}, true); e != nil {
		return e
	}
	if e := requireResultAvailableTx(ctx, tx, scope.Tenant, "derived_generations", g.ID); e != nil {
		return e
	}
	state, e := s.checkInputPartTx(ctx, tx, inputAccess{Actor: a, Scope: monitoringScope(scope), History: true}, g.snapshot, proof.needs, proof.sources)
	if e != nil {
		return e
	}
	proof.generation.InputState = "unchanged"
	if state.Changed {
		proof.generation.InputState = "changed"
	}
	if state.Revoked {
		proof.generation.InputState = "revoked"
	}
	for _, baseline := range proof.manifest.Baselines {
		fixed, exists, e := loadBaselineMetadataTx(ctx, tx, scope.Tenant, g.ConfigID, baseline.Reference, &baseline.Hash)
		if e != nil {
			return e
		}
		if !exists || fixed.Snapshot.ID != baseline.Snapshot {
			return problem("result_integrity_unavailable", "历史固定基准依据不完整。", 503)
		}
		baselineState, e := s.checkFixedBaselineHistoryTx(ctx, tx, inputAccess{Actor: a, Scope: monitoringScope(scope), History: true}, fixed)
		if e != nil {
			return e
		}
		if baselineState.Revoked {
			proof.generation.InputState = "revoked"
		} else if baselineState.Changed && proof.generation.InputState == "unchanged" {
			proof.generation.InputState = "changed"
		}
	}
	return nil
}

func (s *Service) ChannelGenerations(ctx context.Context, a access.Actor, scope Scope, channel string, in GenerationFilter) (GenerationPage, error) {
	out := GenerationPage{Items: []Generation{}}
	if !access.ValidID(channel) {
		return out, notFound()
	}
	r, e := in.Range.rangeValue()
	if e != nil {
		return out, e
	}
	if in.Limit == 0 {
		in.Limit = 30
	}
	if in.Limit < 1 || in.Limit > 100 {
		return out, invalid("/limit", "每页1–100个代次。")
	}
	key := fingerprint([]any{"generations/1", a.ID, scope, channel, in.Range, in.Limit})
	cursor, e := decodeCursor(in.Cursor, key)
	if e != nil {
		return out, e
	}
	type entry struct {
		id string
		at time.Time
	}
	ids := []entry{}
	e = s.transact(ctx, a, scope, "channels.generations", false, func(tx pgx.Tx) error {
		if _, e := s.channelTx(ctx, tx, a, scope, channel); e != nil {
			return e
		}
		if e := s.monitoring.AuthorizeCalculationOutputTx(ctx, tx, a, monitoringScope(scope), channel, []monitoring.Window{rangeWindow(r)}, true); e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT id::text,created_at FROM derived_generations WHERE tenant_id=$1 AND channel_id=$2 AND output_range && $3::int8range AND ($4::timestamptz IS NULL OR (created_at,id)<($4,$5::uuid)) ORDER BY created_at DESC,id DESC LIMIT $6`, scope.Tenant, channel, rangeSQL(r), cursor.At, nullable(cursor.ID), in.Limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var v entry
			if e = rows.Scan(&v.id, &v.at); e != nil {
				return e
			}
			ids = append(ids, v)
		}
		return rows.Err()
	})
	if e != nil {
		return out, e
	}
	if len(ids) > in.Limit {
		ids = ids[:in.Limit]
		out.NextCursor = encodeCursor(ids[len(ids)-1].at, ids[len(ids)-1].id, key)
	}
	for _, v := range ids {
		proof, e := s.historyGeneration(ctx, a, scope, v.id, &in.Range, "channels.generations")
		if e != nil {
			return GenerationPage{}, e
		}
		out.Items = append(out.Items, proof.generation)
	}
	return out, nil
}
func (s *Service) GenerationDetails(ctx context.Context, a access.Actor, scope Scope, id string, in GenerationQuery) (GenerationDetails, error) {
	out := GenerationDetails{Items: []monitoring.CalculatedObservation{}}
	if in.Limit == 0 {
		in.Limit = 100
	}
	if in.Limit < 1 || in.Limit > 1000 {
		return out, invalid("/limit", "每页1–1000条历史结果。")
	}
	proof, e := s.historyGeneration(ctx, a, scope, id, in.Range, "generations.query")
	if e != nil {
		return out, e
	}
	key := fingerprint([]any{"generation-details/1", a.ID, scope, id, proof.generation.Range, in.Limit})
	cursor, e := decodeCursor(in.Cursor, key)
	if e != nil {
		return out, e
	}
	best := &pointHeap{points: []monitoring.CalculatedObservation{}}
	heap.Init(best)
	scanned := uint64(0)
	for _, block := range proof.manifest.Blocks {
		if e = s.recheckHistory(ctx, a, scope, &proof, "generations.query"); e != nil {
			return out, e
		}
		e = s.results.ReadBlock(ctx, block, func(row resultstore.Row) error {
			scanned++
			if scanned > 1_000_000 {
				return problem("query_budget_exceeded", "历史结果读取超过交互预算，请缩小范围或使用后台导出。", 422)
			}
			if row.Generation != id || row.Config != proof.generation.ConfigID || row.Series != proof.generation.ChannelID || row.Run != proof.generation.RunID || row.Lineage != proof.generation.lineage {
				return problem("result_integrity_unavailable", "历史结果块身份不一致。", 503)
			}
			if !proof.output.Contains(row.At) || cursor.At != nil && (row.At < cursor.At.UnixNano() || row.At == cursor.At.UnixNano() && row.Result <= cursor.ID) {
				return nil
			}
			point := outputPoint(row)
			point.Freshness = "historical"
			if best.Len() < in.Limit+1 {
				heap.Push(best, point)
			} else if compareResult(resultBoundary(point), resultBoundary(best.points[0])) < 0 {
				best.points[0] = point
				heap.Fix(best, 0)
			}
			return nil
		})
		if e != nil {
			return GenerationDetails{}, e
		}
	}
	if e = s.recheckHistory(ctx, a, scope, &proof, "generations.query"); e != nil {
		return GenerationDetails{}, e
	}
	out.Generation = proof.generation
	out.Items = best.points
	slices.SortFunc(out.Items, func(a, b monitoring.CalculatedObservation) int {
		return compareResult(resultBoundary(a), resultBoundary(b))
	})
	out.HasMore = len(out.Items) > in.Limit
	if out.HasMore {
		out.Items = out.Items[:in.Limit]
		last := out.Items[len(out.Items)-1]
		out.NextCursor = encodeCursor(time.Unix(0, last.AtNS).UTC(), last.Result, key)
	}
	return out, nil
}
func (s *Service) GenerationLineage(ctx context.Context, a access.Actor, scope Scope, id string, in GenerationQuery) (GenerationLineage, error) {
	var out GenerationLineage
	if in.Cursor != "" {
		return out, invalid("/cursor", "来源依据不接受分页游标。")
	}
	proof, e := s.historyGeneration(ctx, a, scope, id, in.Range, "generations.lineage")
	if e != nil {
		return out, e
	}
	var lineage map[string]any
	if e = s.results.ReadDocument(ctx, scope.Tenant, "lineage", proof.generation.lineage, &lineage); e != nil {
		return out, e
	}
	if lineage["schema"] != "derived-lineage/1" || lineage["config_id"] != proof.config.ID || lineage["config_hash"] != proof.config.ConfigHash || lineage["run_id"] != proof.generation.RunID {
		return out, problem("result_integrity_unavailable", "历史来源依据与固定配置不一致。", 503)
	}
	if e = s.recheckHistory(ctx, a, scope, &proof, "generations.lineage"); e != nil {
		return out, e
	}
	out = GenerationLineage{Generation: proof.generation, Config: proof.config, InputSnapshot: proof.manifest.Input, InputRanges: proof.needs, Sources: proof.sources, Baselines: proof.manifest.Baselines, ManifestHash: proof.generation.manifest, LineageHash: proof.generation.lineage}
	return out, nil
}
