package derivedmonitoring

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/compute"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/resultstore"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/jackc/pgx/v5"
)

type AnalysisArtifact struct {
	ID            string    `json:"id"`
	ConfigID      string    `json:"config_id"`
	Name          string    `json:"name"`
	Status        string    `json:"status"`
	Revision      string    `json:"revision"`
	LatestVersion string    `json:"latest_version"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
type AnalysisArtifactVersion struct {
	Partial         bool            `json:"partial,omitempty"`
	ID              string          `json:"id"`
	ArtifactID      string          `json:"artifact_id"`
	Number          string          `json:"version_no"`
	RunID           string          `json:"run_id"`
	ConfigVersionID string          `json:"config_version_id"`
	OutputRange     NanosecondRange `json:"output_range"`
	InputSnapshotID string          `json:"input_snapshot_id"`
	SourcePosition  string          `json:"source_position"`
	OutputRows      string          `json:"output_rows,omitempty"`
	Stale           bool            `json:"stale"`
	InputState      string          `json:"input_state"`
	CreatedAt       time.Time       `json:"created_at"`
}
type AnalysisArtifactDetail struct {
	LatestOutputs    []AnalysisArtifactRow   `json:"latest_outputs,omitempty"`
	Run              AnalysisRunInfo         `json:"run"`
	Artifact         AnalysisArtifact        `json:"artifact"`
	Version          AnalysisArtifactVersion `json:"version"`
	Config           AnalysisVersion         `json:"config"`
	Outputs          []Output                `json:"outputs"`
	Changes          []AnalysisChange        `json:"changes"`
	ChangesTruncated bool                    `json:"changes_truncated"`
}
type AnalysisChange struct {
	Kind      string          `json:"kind"`
	ChannelID string          `json:"channel_id"`
	Range     NanosecondRange `json:"affected_range"`
	Position  string          `json:"source_position"`
	At        time.Time       `json:"changed_at"`
}
type AnalysisArtifactPage struct {
	Items      []AnalysisArtifactDetail `json:"items"`
	NextCursor string                   `json:"next_cursor"`
}
type AnalysisArtifactVersions struct {
	Items      []AnalysisArtifactVersion `json:"items"`
	NextCursor string                    `json:"next_cursor"`
}
type AnalysisArtifactQuery struct {
	IncludeSamples bool             `json:"include_samples,omitempty"`
	VersionID      string           `json:"version_id"`
	Range          *NanosecondRange `json:"output_range,omitempty"`
	Outputs        []string         `json:"outputs,omitempty"`
	Cursor         string           `json:"cursor,omitempty"`
	Limit          int              `json:"limit,omitempty"`
}
type AnalysisArtifactRow struct {
	InputFirst *int64 `json:"input_first_ns,string,omitempty"`
	InputLast  *int64 `json:"input_last_ns,string,omitempty"`
	ResultID   string `json:"result_id"`
	TrialValue
	Lineage string `json:"lineage_ref"`
}
type AnalysisArtifactRows struct {
	Detail     *AnalysisArtifactDetail `json:"detail,omitempty"`
	Version    AnalysisArtifactVersion `json:"version"`
	Items      []AnalysisArtifactRow   `json:"items"`
	NextCursor string                  `json:"next_cursor"`
}
type analysisArtifactRecord struct {
	AnalysisArtifactVersion
	Manifest, Lineage string
}
type analysisArtifactProof struct {
	PreviewPlan *compute.Plan
	Selection   *analysisSelection
	WholeRange  bool
	Detail      AnalysisArtifactDetail
	Version     analysisArtifactRecord
	Manifest    resultManifest
}

const analysisArtifactSelect = `SELECT id::text,config_id::text,name,status,revision::text,latest_version::text,created_at,updated_at FROM analysis_artifacts`
const analysisArtifactVersionSelect = `SELECT v.id::text,v.artifact_id::text,v.version_no::text,v.run_id::text,v.config_version_id::text,lower(v.output_range),upper(v.output_range),v.raw_snapshot_id::text,v.source_position::text,v.output_rows::text,v.stale_at_publication,v.created_at,v.manifest_root,v.lineage_root FROM analysis_artifact_versions v JOIN analysis_artifacts a ON a.tenant_id=v.tenant_id AND a.id=v.artifact_id`

func analysisArtifactTx(ctx context.Context, tx pgx.Tx, scope Scope, id string) (AnalysisArtifact, error) {
	var out AnalysisArtifact
	if !access.ValidID(id) {
		return out, notFound()
	}
	e := tx.QueryRow(ctx, analysisArtifactSelect+` WHERE tenant_id=$1 AND scope_key=$2 AND id=$3 AND deleted_at IS NULL`, scope.Tenant, scope.key(), id).Scan(&out.ID, &out.ConfigID, &out.Name, &out.Status, &out.Revision, &out.LatestVersion, &out.CreatedAt, &out.UpdatedAt)
	if e == pgx.ErrNoRows {
		e = notFound()
	}
	return out, e
}
func scanAnalysisArtifactVersion(row pgx.Row) (analysisArtifactRecord, error) {
	var out analysisArtifactRecord
	var from int64
	var to *int64
	e := row.Scan(&out.ID, &out.ArtifactID, &out.Number, &out.RunID, &out.ConfigVersionID, &from, &to, &out.InputSnapshotID, &out.SourcePosition, &out.OutputRows, &out.Stale, &out.CreatedAt, &out.Manifest, &out.Lineage)
	if e == pgx.ErrNoRows {
		e = notFound()
	}
	out.OutputRange = rangeDTO(from, to)
	return out, e
}
func (s *Service) analysisArtifactIdentityTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, artifact, version string) (analysisArtifactProof, error) {
	var out analysisArtifactProof
	var e error
	out.Detail.Artifact, e = analysisArtifactTx(ctx, tx, scope, artifact)
	if e != nil {
		return out, e
	}
	if version != "" && !access.ValidID(version) {
		return out, notFound()
	}
	out.Version, e = scanAnalysisArtifactVersion(tx.QueryRow(ctx, analysisArtifactVersionSelect+` WHERE v.tenant_id=$1 AND a.scope_key=$2 AND a.deleted_at IS NULL AND v.artifact_id=$3 AND ($4::uuid IS NULL AND v.version_no=a.latest_version OR v.id=$4)`, scope.Tenant, scope.key(), artifact, nullable(version)))
	if e != nil {
		return out, e
	}
	out.Detail.Config, e = analysisVersionTx(ctx, tx, scope, out.Version.ConfigVersionID)
	if e != nil {
		return out, e
	}
	target, e := analysisRunTargetTx(ctx, tx, scope.Tenant, out.Version.RunID)
	if e != nil {
		return out, e
	}
	if target.ConfigVersionID != out.Version.ConfigVersionID || target.ArtifactID != artifact || target.ArtifactVersionID != out.Version.ID {
		return out, problem("result_integrity_unavailable", "成果发布身份与固定任务不一致。", 503)
	}
	out.Detail.Run = target.AnalysisRunInfo
	var runRule Rule
	if e = json.Unmarshal(target.Rule, &runRule); e != nil {
		return out, e
	}
	out.WholeRange = runRule.Clock.Kind == "whole_range"
	if e = s.checkAnalysisBindingsTx(ctx, tx, a, scope, out.Detail.Config.Body); e != nil {
		return out, e
	}
	if e = requireResultAvailableTx(ctx, tx, scope.Tenant, "analysis_artifact_versions", out.Version.ID); e != nil {
		return out, e
	}
	return out, nil
}

func (s *Service) analysisArtifactProofTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, artifact, version string, capabilities ...string) (analysisArtifactProof, error) {
	return s.analysisArtifactSelectedTx(ctx, tx, a, scope, artifact, version, nil, capabilities...)
}

func (s *Service) analysisArtifactSelectedTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, artifact, version string, selection *analysisSelection, capabilities ...string) (analysisArtifactProof, error) {
	out, e := s.analysisArtifactIdentityTx(ctx, tx, a, scope, artifact, version)
	if e != nil {
		return out, e
	}
	out.Selection = selection
	if selection != nil && selection.Partial {
		out.Version.Stale = false
	}
	check := func(history bool) (monitoring.RawSnapshotState, error) {
		if selection == nil {
			return s.checkAnalysisInputsTx(ctx, tx, a, monitoringScope(scope), out.Version.InputSnapshotID, history, capabilities...)
		}
		auth := inputAccess{Actor: a, Scope: monitoringScope(scope), History: history, AllowChanged: true}
		if len(capabilities) > 0 {
			auth.Capability = capabilities[0]
		}
		state, e := s.checkInputPartTx(ctx, tx, auth, out.Version.InputSnapshotID, selection.Direct, selection.Sources)
		if e == nil && state.Revoked && !history {
			e = problem("snapshot_stale", "固定成果输入已撤销。", 409)
		}
		return state, e
	}
	state, e := check(false)
	var raw *monitoring.Error
	var de *Error
	if errors.As(e, &raw) && raw.Code == "snapshot_stale" || errors.As(e, &de) && de.Code == "snapshot_stale" {
		state, e = check(true)
	}
	if e != nil {
		return out, e
	}
	for key := range out.Detail.Config.Body.BaselineBindings {
		if selection != nil && !slices.Contains(selection.Baselines, key) {
			continue
		}
		fixed, found, e := loadBaselineMetadataTx(ctx, tx, scope.Tenant, out.Detail.Config.ID, key, nil)
		if e != nil {
			return out, e
		}
		if !found {
			return out, problem("result_integrity_unavailable", "成果固定基准依据缺失。", 503)
		}
		auth := inputAccess{Actor: a, Scope: monitoringScope(scope)}
		if len(capabilities) > 0 {
			auth.Capability = capabilities[0]
		}
		if e = s.authorizeFixedBaselineTx(ctx, tx, auth, fixed); e != nil {
			return out, e
		}
		basis, e := s.checkFixedInputsTx(ctx, tx, auth, fixed.Snapshot.ID)
		if e != nil {
			return out, e
		}
		if basis.Revoked {
			if _, e = s.checkFixedBaselineHistoryTx(ctx, tx, auth, fixed); e != nil {
				return out, e
			}
			state.Revoked = true
			state.Changed = true
		}
	}
	out.Version.Stale = out.Version.Stale || state.Changed
	out.Version.InputState = "current"
	if out.Version.Stale {
		out.Version.InputState = "affected"
	}
	if state.Revoked {
		out.Version.InputState = "revoked"
	}
	out.Detail.Changes = []AnalysisChange{}
	query := `SELECT e.change_type,r.channel_id::text,lower(r.affected*i.required_range),upper(r.affected*i.required_range),e.source_position::text,e.created_at FROM analysis_artifact_input_ranges i JOIN monitoring_change_ranges r ON r.tenant_id=i.tenant_id AND r.channel_id=i.channel_id AND r.affected&&i.required_range JOIN monitoring_changes e ON e.tenant_id=r.tenant_id AND e.id=r.event_id WHERE i.tenant_id=$1 AND i.version_id=$2 AND e.source_position>$3 ORDER BY e.source_position,r.ordinal LIMIT 101`
	args := []any{scope.Tenant, out.Version.ID, out.Version.SourcePosition}
	if selection != nil {
		query = `SELECT e.change_type,r.channel_id::text,lower(r.affected*int8range(n.from_ns,n.to_ns,'[)')),upper(r.affected*int8range(n.from_ns,n.to_ns,'[)')),e.source_position::text,e.created_at FROM jsonb_to_recordset($3::jsonb) n(channel_id uuid,from_ns bigint,to_ns bigint) JOIN monitoring_change_ranges r ON r.tenant_id=$1 AND r.channel_id=n.channel_id AND r.affected&&int8range(n.from_ns,n.to_ns,'[)') JOIN monitoring_changes e ON e.tenant_id=r.tenant_id AND e.id=r.event_id WHERE e.source_position>$2 ORDER BY e.source_position,r.ordinal LIMIT 101`
		args = []any{scope.Tenant, out.Version.SourcePosition, jsonBytes(selection.Requirements)}
	}
	rows, e := tx.Query(ctx, query, args...)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var change AnalysisChange
		var from, to *int64
		if e = rows.Scan(&change.Kind, &change.ChannelID, &from, &to, &change.Position, &change.At); e != nil {
			rows.Close()
			return out, e
		}
		need := monitoring.RawNeed{Channel: change.ChannelID, From: from, To: to}
		r, e := rawNeedRange(need)
		if e != nil {
			rows.Close()
			return out, e
		}
		change.Range = rangeDTO(r.From, r.To)
		out.Detail.Changes = append(out.Detail.Changes, change)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	if len(out.Detail.Changes) > 0 {
		out.Version.Stale = true
		if out.Version.InputState == "current" {
			out.Version.InputState = "affected"
		}
	}
	if len(out.Detail.Changes) > 100 {
		out.Detail.Changes = out.Detail.Changes[:100]
		out.Detail.ChangesTruncated = true
	}
	out.Detail.Version = out.Version.AnalysisArtifactVersion
	if selection != nil && selection.Partial {
		out.Detail.Version.Partial = true
		out.Detail.Version.OutputRange = selection.Range
		out.Detail.Version.OutputRows = ""
	}
	return out, nil
}
func (s *Service) loadAnalysisArtifact(ctx context.Context, a access.Actor, scope Scope, artifact, version, action string) (analysisArtifactProof, error) {
	var out analysisArtifactProof
	e := s.transact(ctx, a, scope, action, false, func(tx pgx.Tx) error {
		var e error
		out, e = s.analysisArtifactProofTx(ctx, tx, a, scope, artifact, version)
		return e
	})
	if e != nil {
		return out, e
	}
	e = s.loadAnalysisManifest(ctx, scope, artifact, &out)
	return out, e
}

func (s *Service) loadAnalysisManifest(ctx context.Context, scope Scope, artifact string, out *analysisArtifactProof) error {
	if s.results == nil {
		return problem("calculation_storage_unavailable", "分析存储不可用。", 503)
	}
	if e := s.results.ReadDocument(ctx, scope.Tenant, "manifests", out.Version.Manifest, &out.Manifest); e != nil {
		return e
	}
	m := out.Manifest
	if m.Schema != "analysis-result-manifest/1" || m.Generation != out.Version.ID || m.Artifact != artifact || m.Run != out.Version.RunID || m.Config != out.Version.ConfigVersionID || m.Input.ID != out.Version.InputSnapshotID || m.Lineage != out.Version.Lineage || strconv.FormatUint(m.Rows, 10) != out.Version.OutputRows || fingerprint(m.Range) != fingerprint(out.Version.OutputRange) {
		return problem("result_integrity_unavailable", "成果清单与发布依据不一致。", 503)
	}
	out.Detail.Outputs = m.Outputs
	return nil
}
func (s *Service) AnalysisArtifact(ctx context.Context, a access.Actor, scope Scope, id, version string) (AnalysisArtifactDetail, error) {
	proof, e := s.loadAnalysisArtifact(ctx, a, scope, id, version, "analysis.artifacts.detail")
	if e != nil {
		return AnalysisArtifactDetail{}, e
	}
	latest := map[string]resultstore.Row{}
	consider := func(row resultstore.Row) error {
		if row.Artifact == nil || *row.Artifact != id || row.Generation != proof.Version.ID || row.Config != proof.Version.ConfigVersionID || row.Run != proof.Version.RunID || row.Lineage != proof.Version.Lineage {
			return problem("result_integrity_unavailable", "固定成果行的身份不一致。", 503)
		}
		if previous, ok := latest[row.Output]; !ok || row.At > previous.At {
			latest[row.Output] = row
		}
		return nil
	}
	for _, block := range proof.Manifest.Blocks {
		if block.Endpoints != nil {
			e = consider(block.Endpoints.Last)
		} else {
			// Older sealed blocks predate endpoint indexes.
			e = s.results.ReadBlock(ctx, block, consider)
		}
		if e != nil {
			return AnalysisArtifactDetail{}, e
		}
	}
	e = s.transact(ctx, a, scope, "analysis.artifacts.detail", false, func(tx pgx.Tx) error {
		current, e := s.analysisArtifactProofTx(ctx, tx, a, scope, id, proof.Version.ID)
		if e == nil {
			current.Detail.Outputs = proof.Detail.Outputs
			for _, output := range current.Detail.Outputs {
				if row, ok := latest[output.Key]; ok {
					current.Detail.LatestOutputs = append(current.Detail.LatestOutputs, *artifactRow(&row))
				}
			}
			proof.Detail = current.Detail
		}
		return e
	})
	return proof.Detail, e
}

type artifactCursor struct {
	Hash   string `json:"h"`
	Offset uint64 `json:"o"`
}

func (s *Service) AnalysisArtifactRows(ctx context.Context, a access.Actor, scope Scope, id string, in AnalysisArtifactQuery) (AnalysisArtifactRows, error) {
	out := AnalysisArtifactRows{Items: []AnalysisArtifactRow{}}
	proof, e := s.loadSelectedAnalysis(ctx, a, scope, id, in, "analysis.artifacts.query")
	if e != nil {
		return out, e
	}
	out.Version = proof.Detail.Version
	limit := in.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		return out, invalid("/limit", "每页最多1000行。")
	}
	known := map[string]bool{}
	for _, o := range proof.Manifest.Outputs {
		known[o.Key] = true
	}
	for _, key := range in.Outputs {
		if !known[key] {
			return out, invalid("/outputs", "所选输出不属于该固定成果。")
		}
	}
	var from, to *int64
	if in.Range != nil {
		r, e := in.Range.rangeValue()
		if e != nil {
			return out, e
		}
		from = &r.From
		to = r.To
	}
	includeEnd := analysisIncludesWholeEnd(proof, from, to)
	hash := fingerprint([]any{proof.Version.ID, proof.Version.Manifest, in.Range, in.Outputs})
	var offset uint64
	if in.Cursor != "" {
		raw, e := base64.RawURLEncoding.DecodeString(in.Cursor)
		var c artifactCursor
		if e != nil || json.Unmarshal(raw, &c) != nil || c.Hash != hash {
			return out, invalid("/cursor", "分页必须继续使用同一成果版本及范围。")
		}
		offset = c.Offset
	}
	var scanned, matched uint64
	more := false
	for _, block := range proof.Manifest.Blocks {
		// Indexed blocks let a later page skip complete preceding files without
		// rescanning values. Edge blocks are still fully verified before use.
		if block.Output != "" && block.First != nil && block.Last != nil {
			if len(in.Outputs) > 0 && !slices.Contains(in.Outputs, block.Output) || from != nil && *block.Last < *from || to != nil && (*block.First > *to || *block.First == *to && !includeEnd) {
				continue
			}
			whole := (from == nil || *block.First >= *from) && (to == nil || *block.Last < *to || *block.Last == *to && includeEnd)
			if whole && offset >= matched && offset-matched >= block.Rows {
				matched += block.Rows
				continue
			}
		}
		if e = s.transact(ctx, a, scope, "analysis.artifacts.query", false, func(tx pgx.Tx) error {
			_, e := s.analysisArtifactSelectedTx(ctx, tx, a, scope, id, proof.Version.ID, proof.Selection)
			return e
		}); e != nil {
			return out, e
		}
		e = s.results.ReadBlock(ctx, block, func(row resultstore.Row) error {
			scanned++
			if scanned > 10_000_000 {
				return problem("budget_exceeded", "成果查询超过扫描预算，请缩小输出范围。", 422)
			}
			if row.Artifact == nil || *row.Artifact != id || row.Generation != proof.Version.ID || row.Config != proof.Version.ConfigVersionID || row.Run != proof.Version.RunID || row.Lineage != proof.Version.Lineage || !known[row.Output] {
				return problem("result_integrity_unavailable", "成果结果身份不一致。", 503)
			}
			if from != nil && row.At < *from || to != nil && (row.At > *to || row.At == *to && !includeEnd) || len(in.Outputs) > 0 && !slices.Contains(in.Outputs, row.Output) {
				return nil
			}
			if matched < offset {
				matched++
				return nil
			}
			matched++
			if len(out.Items) >= limit {
				more = true
				return nil
			}
			out.Items = append(out.Items, *artifactRow(&row))
			return nil
		})
		if e != nil {
			return AnalysisArtifactRows{}, e
		}
		if more {
			break
		}
	}
	if more {
		out.NextCursor = base64.RawURLEncoding.EncodeToString(jsonBytes(artifactCursor{hash, offset + uint64(len(out.Items))}))
	}
	e = s.transact(ctx, a, scope, "analysis.artifacts.query", false, func(tx pgx.Tx) error {
		current, e := s.analysisArtifactSelectedTx(ctx, tx, a, scope, id, proof.Version.ID, proof.Selection)
		if e == nil {
			out.Version = current.Detail.Version
			current.Detail.Outputs = proof.Detail.Outputs
			out.Detail = &current.Detail
		}
		return e
	})
	if e != nil {
		return AnalysisArtifactRows{}, e
	}
	return out, nil
}

func analysisIncludesWholeEnd(proof analysisArtifactProof, from, to *int64) bool {
	if !proof.WholeRange || to == nil {
		return false
	}
	r, e := proof.Version.OutputRange.rangeValue()
	return e == nil && r.To != nil && *to == *r.To && (from == nil || *from <= r.From)
}
