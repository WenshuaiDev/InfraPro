package derivedmonitoring

import (
	"cmp"
	"context"
	"errors"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/compute"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/jackc/pgx/v5"
)

func analysisRestricted(e error) bool {
	var de *Error
	var raw *monitoring.Error
	return errors.Is(e, access.ErrDenied) || errors.As(e, &de) && (de.Status == 403 || de.Status == 404) || errors.As(e, &raw) && (raw.Status == 403 || raw.Status == 404)
}
func (s *Service) AnalysisArtifacts(ctx context.Context, a access.Actor, scope Scope, filter MetricFilter) (AnalysisArtifactPage, error) {
	out := AnalysisArtifactPage{Items: []AnalysisArtifactDetail{}}
	if filter.State != "" && filter.State != "active" && filter.State != "archived" {
		return out, invalid("/state", "不支持该成果状态。")
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	key := filterKey(a, scope, filter)
	legacy := access.ValidID(filter.Cursor)
	var cursor listCursor
	var e error
	if !legacy {
		cursor, e = decodeCursor(filter.Cursor, key)
		if e != nil {
			return out, e
		}
	}
	e = s.transact(ctx, a, scope, "analysis.artifacts.list", false, func(tx pgx.Tx) error {
		query := `SELECT a.id::text,v.created_at FROM analysis_artifacts a JOIN analysis_artifact_versions v
			ON v.tenant_id=a.tenant_id AND v.artifact_id=a.id AND v.version_no=a.latest_version
			WHERE a.tenant_id=$1 AND a.scope_key=$2 AND a.deleted_at IS NULL AND ($3='' OR a.name ILIKE '%'||$3||'%') AND ($4='' OR a.status=$4)`
		args := []any{scope.Tenant, scope.key(), strings.TrimSpace(filter.Query), filter.State}
		if legacy {
			// Finish old ID-ordered traversals without changing their ordering mid-page.
			query += ` AND a.id>$5 ORDER BY a.id LIMIT 201`
			args = append(args, filter.Cursor)
		} else {
			query += ` AND ($5::timestamptz IS NULL OR (v.created_at,a.id)<($5,$6::uuid)) ORDER BY v.created_at DESC,a.id DESC LIMIT 201`
			args = append(args, cursor.At, nullable(cursor.ID))
		}
		rows, e := tx.Query(ctx, query, args...)
		if e != nil {
			return e
		}
		type entry struct {
			id string
			at time.Time
		}
		entries := []entry{}
		for rows.Next() {
			var item entry
			if e = rows.Scan(&item.id, &item.at); e != nil {
				rows.Close()
				return e
			}
			entries = append(entries, item)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		pageCursor := func(item entry) string {
			if legacy {
				return item.id
			}
			return encodeCursor(item.at, item.id, key)
		}
		for i, item := range entries {
			if i == 200 {
				out.NextCursor = pageCursor(entries[i-1])
				break
			}
			proof, e := s.analysisArtifactProofTx(ctx, tx, a, scope, item.id, "")
			if analysisRestricted(e) {
				continue
			}
			if e != nil {
				return e
			}
			var rule Rule
			if e = compute.Decode(proof.Detail.Config.Body.Rule, &rule); e != nil {
				return e
			}
			proof.Detail.Outputs = []Output{}
			for _, output := range rule.Outputs {
				if slices.Contains(proof.Detail.Run.Outputs, output.Key) {
					proof.Detail.Outputs = append(proof.Detail.Outputs, output)
				}
			}
			out.Items = append(out.Items, proof.Detail)
			if len(out.Items) == limit {
				if i+1 < len(entries) {
					out.NextCursor = pageCursor(item)
				}
				break
			}
		}
		return nil
	})
	return out, e
}
func (s *Service) AnalysisArtifactVersions(ctx context.Context, a access.Actor, scope Scope, id, cursor string) (AnalysisArtifactVersions, error) {
	out := AnalysisArtifactVersions{Items: []AnalysisArtifactVersion{}}
	before := int64(1<<63 - 1)
	if cursor != "" {
		var e error
		before, e = revisionNumber(cursor)
		if e != nil {
			return out, e
		}
	}
	e := s.transact(ctx, a, scope, "analysis.artifacts.versions", false, func(tx pgx.Tx) error {
		if _, e := analysisArtifactTx(ctx, tx, scope, id); e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT v.id::text,v.version_no FROM analysis_artifact_versions v JOIN analysis_artifacts a ON a.tenant_id=v.tenant_id AND a.id=v.artifact_id WHERE v.tenant_id=$1 AND a.scope_key=$2 AND a.deleted_at IS NULL AND a.id=$3 AND v.version_no<$4 ORDER BY v.version_no DESC LIMIT 201`, scope.Tenant, scope.key(), id, before)
		if e != nil {
			return e
		}
		type item struct {
			id     string
			number int64
		}
		items := []item{}
		for rows.Next() {
			var v item
			if e = rows.Scan(&v.id, &v.number); e != nil {
				rows.Close()
				return e
			}
			items = append(items, v)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		for i, v := range items {
			if i == 200 {
				out.NextCursor = strconv.FormatInt(items[i-1].number, 10)
				break
			}
			proof, e := s.analysisArtifactProofTx(ctx, tx, a, scope, id, v.id)
			if analysisRestricted(e) {
				continue
			}
			if e != nil {
				return e
			}
			out.Items = append(out.Items, proof.Detail.Version)
			if len(out.Items) == 50 {
				if i+1 < len(items) {
					out.NextCursor = strconv.FormatInt(v.number, 10)
				}
				break
			}
		}
		return nil
	})
	return out, e
}

type AnalysisArtifactArchiveResult struct {
	Artifact AnalysisArtifact `json:"artifact"`
	Replayed bool             `json:"replayed"`
}

func (s *Service) ArchiveAnalysisArtifact(ctx context.Context, a access.Actor, scope Scope, id string, in WriteMeta, audit access.Meta) (AnalysisArtifactArchiveResult, error) {
	var out AnalysisArtifactArchiveResult
	if e := checkMeta(in, true, true); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, scope, "analysis.artifacts.archive", true, func(tx pgx.Tx) error {
		payload := []any{id, in}
		found, e := analysisReceiptTx(ctx, tx, a, scope, "analysis.artifacts.archive", in, payload, &out)
		if e != nil {
			return e
		}
		if found {
			out.Replayed = true
			_, e = analysisArtifactTx(ctx, tx, scope, id)
			return e
		}
		before, e := analysisArtifactTx(ctx, tx, scope, id)
		if e != nil {
			return e
		}
		if before.Revision != in.Revision {
			return conflict()
		}
		if before.Status != "archived" {
			tag, e := tx.Exec(ctx, `UPDATE analysis_artifacts SET status='archived',revision=revision+1,updated_at=clock_timestamp() WHERE tenant_id=$1 AND id=$2 AND revision=$3 AND deleted_at IS NULL`, scope.Tenant, id, in.Revision)
			if e != nil {
				return e
			}
			if tag.RowsAffected() != 1 {
				return conflict()
			}
		}
		out.Artifact, e = analysisArtifactTx(ctx, tx, scope, id)
		if e != nil {
			return e
		}
		return analysisCompleteTx(ctx, tx, a, scope, "analysis.artifacts.archive", "analysis_artifact", id, in, payload, before, out, audit)
	})
	return out, e
}
func (s *Service) AnalysisArtifactLineage(ctx context.Context, a access.Actor, scope Scope, id string, in AnalysisArtifactQuery) (map[string]any, error) {
	proof, e := s.loadSelectedAnalysis(ctx, a, scope, id, in, "analysis.artifacts.lineage")
	if e != nil {
		return nil, e
	}
	var document map[string]any
	if e = s.results.ReadDocument(ctx, scope.Tenant, "lineage", proof.Version.Lineage, &document); e != nil {
		return nil, e
	}
	if document["schema"] != "analysis-lineage/1" || document["run_id"] != proof.Version.RunID || document["config_version_id"] != proof.Version.ConfigVersionID {
		return nil, problem("result_integrity_unavailable", "分析来源清单身份不一致。", 503)
	}
	if in.IncludeSamples {
		if in.Range == nil || len(in.Outputs) != 1 || proof.Selection == nil {
			return nil, invalid("/output_range", "查看样本需要指定单项输出及其时段。")
		}
		samples, more, e := s.analysisSamplePreview(ctx, a, scope, proof)
		if e != nil {
			return nil, e
		}
		document["samples"], document["samples_truncated"] = samples, more
	}
	e = s.transact(ctx, a, scope, "analysis.artifacts.lineage", false, func(tx pgx.Tx) error {
		_, e := s.analysisArtifactSelectedTx(ctx, tx, a, scope, id, proof.Version.ID, proof.Selection)
		return e
	})
	if e != nil {
		return nil, e
	}
	if proof.Selection != nil {
		needs, e := sourceCandidates(proof.Selection.Direct, proof.Selection.Sources)
		if e != nil {
			return nil, e
		}
		document["input_ranges"] = needs
		document["upstream_inputs"] = proof.Selection.Sources
		document["output_range"] = proof.Selection.Range
		document["outputs"] = proof.Selection.Outputs
		document["baselines"] = slices.DeleteFunc(slices.Clone(proof.Manifest.Baselines), func(b BaselineProof) bool { return !slices.Contains(proof.Selection.Baselines, b.Reference) })
		document["partial"] = proof.Selection.Partial
		document["lineage_root"] = proof.Version.Lineage
	}
	return document, nil
}

type AnalysisArtifactDeleteResult struct {
	ArtifactID string `json:"artifact_id"`
	Replayed   bool   `json:"replayed"`
}

func (s *Service) DeleteAnalysisArtifact(ctx context.Context, a access.Actor, scope Scope, id string, in WriteMeta, audit access.Meta) (AnalysisArtifactDeleteResult, error) {
	var out AnalysisArtifactDeleteResult
	if e := checkMeta(in, true, true); e != nil {
		return out, e
	}
	if !access.ValidID(id) {
		return out, notFound()
	}
	const action = "analysis.artifacts.delete"
	e := s.transact(ctx, a, scope, action, true, func(tx pgx.Tx) error {
		// Taking the publication lock before checking the receipt also serializes
		// simultaneous retries. Deleted rows remain available for this check.
		var exists int
		e := tx.QueryRow(ctx, `SELECT 1 FROM analysis_artifacts WHERE tenant_id=$1 AND scope_key=$2 AND id=$3 FOR UPDATE`, scope.Tenant, scope.key(), id).Scan(&exists)
		if e == pgx.ErrNoRows {
			return notFound()
		}
		if e != nil {
			return e
		}
		payload := []any{id, in}
		found, e := analysisReceiptTx(ctx, tx, a, scope, action, in, payload, &out)
		if e != nil || found {
			out.Replayed = found
			return e
		}
		proof, e := s.analysisArtifactProofTx(ctx, tx, a, scope, id, "")
		if e != nil {
			return e
		}
		tag, e := tx.Exec(ctx, `UPDATE analysis_artifacts SET deleted_at=now(),updated_at=now(),revision=revision+1
			WHERE tenant_id=$1 AND scope_key=$2 AND id=$3 AND revision=$4 AND deleted_at IS NULL`, scope.Tenant, scope.key(), id, in.Revision)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return conflict()
		}
		out.ArtifactID = id
		return analysisCompleteTx(ctx, tx, a, scope, action, "analysis_artifact", id, in, payload, proof.Detail.Artifact, out, audit)
	})
	return out, e
}

// Preview the same immutable candidate set as an input-evidence export. A
// window can contain excluded observations, so the UI labels these as candidates.
type analysisInputSample struct {
	Matched *bool   `json:"matched,omitempty"`
	Channel string  `json:"channel_id"`
	At      string  `json:"at_ns"`
	Value   *string `json:"value"`
	Quality string  `json:"quality"`
	Kind    string  `json:"source_kind"`
	ID      string  `json:"sample_id"`
}

func (s *Service) analysisSamplePreview(ctx context.Context, a access.Actor, scope Scope, proof analysisArtifactProof) ([]analysisInputSample, bool, error) {
	samples := []analysisInputSample{}
	byChannel := map[string][]compute.Sample{}
	full := errors.New("sample preview complete")
	auth := inputAccess{Actor: a, Scope: monitoringScope(scope), History: proof.Detail.Version.InputState == "revoked", AllowChanged: true, SelectedPart: true}
	part := inputSnapshotDetails{Direct: proof.Selection.Direct, Sources: proof.Selection.Sources}
	_, e := s.readInputs(ctx, auth, proof.Version.InputSnapshotID, part, monitoring.RawReadBudget{Rows: 256, Bytes: 4 << 20}, func(row inputSample) error {
		if len(samples) == 25 {
			return full
		}
		sample := analysisInputSample{Channel: row.Channel, At: strconv.FormatInt(row.Sample.At, 10), Quality: row.Sample.Quality, ID: row.Sample.ID}
		if row.Raw != nil {
			sample.Kind, sample.Value = "direct", &row.Raw.Value
		} else {
			sample.Kind, sample.Value = "derived", row.Derived.Value
		}
		samples = append(samples, sample)
		byChannel[row.Channel] = append(byChannel[row.Channel], row.Sample)
		return nil
	})
	if errors.Is(e, full) {
		return samples, true, nil
	}
	if e == nil && len(samples) > 0 {
		e = s.explainAnalysisMatches(ctx, a, scope, proof, byChannel, samples)
	}
	return samples, false, e
}

// Reproduce direct operand matches only for a complete, bounded candidate set
// at one output time. Older compilers and aggregate windows retain the plain
// snapshot preview. The recorded result must agree before any labels are set.
func (s *Service) explainAnalysisMatches(ctx context.Context, a access.Actor, scope Scope, proof analysisArtifactProof, byChannel map[string][]compute.Sample, samples []analysisInputSample) error {
	p := proof.PreviewPlan
	r, e := proof.Selection.Range.rangeValue()
	if e != nil || p == nil || len(p.Rule.Outputs) != 1 || r.To == nil || *r.To-1 != r.From || p.Rule.Clock.Kind == "whole_range" {
		return e
	}
	selected := slices.IndexFunc(p.Rule.Nodes, func(n Node) bool { return n.ID == p.Rule.Outputs[0].Node })
	if selected < 0 {
		return nil
	}
	node := p.Rule.Nodes[selected]
	if node.Params.Window != nil || node.Params.Baseline != nil {
		return nil
	}
	for _, ref := range node.Inputs {
		if strings.HasPrefix(ref, "node:") {
			return nil
		}
	}
	for _, ref := range node.Params.Variables {
		if strings.HasPrefix(ref, "node:") {
			return nil
		}
	}
	data := compute.Data{Inputs: map[string][]compute.Sample{}, Complete: true}
	for slot, binding := range proof.Detail.Config.Body.InputBindings {
		inputs := byChannel[binding.ChannelID]
		slices.SortFunc(inputs, func(a, b compute.Sample) int { return cmp.Compare(a.At, b.At) })
		data.Inputs[slot] = inputs
	}
	p.Rule.Nodes = []Node{node}
	var result *TrialValue
	e = compute.Run(ctx, p, data, runOptions(r, nil, 0), func(row compute.Result) error {
		v := trialValue(row)
		result = &v
		return nil
	})
	if e != nil || result == nil {
		return ctx.Err()
	}
	stored, e := s.AnalysisArtifactRows(ctx, a, scope, proof.Detail.Artifact.ID, AnalysisArtifactQuery{VersionID: proof.Version.ID, Range: &proof.Selection.Range, Outputs: proof.Selection.Outputs, Limit: 1})
	if e != nil || len(stored.Items) != 1 {
		return e
	}
	row := stored.Items[0]
	if result.At != row.At || !reflect.DeepEqual(result.Value, row.Value) || result.Quality != row.Quality || result.Status != row.Status || result.Count != row.Count || result.Excluded != row.Excluded || !slices.Equal(result.Reasons, row.Reasons) {
		return nil
	}
	for i, sample := range samples {
		matched := slices.ContainsFunc(result.Matches, func(m TrialMatch) bool { return m.ID == sample.ID && m.At == sample.At })
		samples[i].Matched = &matched
	}
	return nil
}
