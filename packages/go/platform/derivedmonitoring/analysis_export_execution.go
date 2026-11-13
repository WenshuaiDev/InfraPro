package derivedmonitoring

import (
	"context"
	"strconv"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/resultstore"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
)

var analysisExportFields = []monitoring.ExportField{
	{Name: "source_kind", Type: "text"}, {Name: "artifact_id", Type: "text"}, {Name: "artifact_version_id", Type: "text"}, {Name: "result_id", Type: "text"}, {Name: "output_key", Type: "text"},
	{Name: "config_version_id", Type: "text"}, {Name: "config_hash", Type: "text"}, {Name: "run_id", Type: "text"}, {Name: "input_snapshot_id", Type: "text"}, {Name: "source_position", Type: "int64"},
	{Name: "output_at", Type: "timestamp_ns_utc"}, {Name: "interval_start", Type: "timestamp_ns_utc", Nullable: true}, {Name: "interval_end", Type: "timestamp_ns_utc", Nullable: true},
	{Name: "value", Type: "computed_decimal_text", Nullable: true}, {Name: "scale", Type: "int64"}, {Name: "rounding", Type: "text"}, {Name: "rounded", Type: "boolean"},
	{Name: "calculation_status", Type: "text"}, {Name: "quality", Type: "text"}, {Name: "sample_count", Type: "int64"}, {Name: "excluded_count", Type: "int64"}, {Name: "reason_flags", Type: "text"},
	{Name: "quantity", Type: "text"}, {Name: "lineage_ref", Type: "text"}, {Name: "input_state", Type: "text"},
}

func analysisExportCells(proof analysisArtifactProof, row resultstore.Row) []monitoring.ExportCell {
	quantity := ""
	for _, output := range proof.Manifest.Outputs {
		if output.Key == row.Output {
			quantity = string(jsonBytes(output.Quantity))
			break
		}
	}
	texts := []string{"analysis", proof.Version.ArtifactID, proof.Version.ID, row.Result, row.Output, proof.Version.ConfigVersionID, proof.Detail.Config.ConfigHash, row.Run, proof.Version.InputSnapshotID, proof.Version.SourcePosition, strconv.FormatInt(row.At, 10), "", "", "", strconv.Itoa(int(row.Scale)), "half_even", strconv.FormatBool(row.Rounded), row.Status, row.Quality, strconv.FormatUint(row.Count, 10), strconv.FormatUint(row.Excluded, 10), string(jsonBytes(row.Reasons)), quantity, row.Lineage, proof.Detail.Version.InputState}
	cells := make([]monitoring.ExportCell, len(texts))
	for i, text := range texts {
		cells[i] = monitoring.ExportCell{Text: text}
	}
	cells[11].Null = row.Start == nil
	if row.Start != nil {
		cells[11].Text = strconv.FormatInt(*row.Start, 10)
	}
	cells[12].Null = row.End == nil
	if row.End != nil {
		cells[12].Text = strconv.FormatInt(*row.End, 10)
	}
	value := row.Decimal()
	cells[13].Null = value == nil
	if value != nil {
		cells[13].Text = *value
	}
	return cells
}
func (s *Service) executeAnalysisExport(ctx context.Context, lease monitoring.TaskLease) error {
	target, e := decodeAnalysisExport(lease.Task().Input)
	if e != nil {
		return e
	}
	scope := Scope{Tenant: lease.Task().Scope.Tenant, Project: lease.Task().Scope.Project, Entry: lease.Task().Scope.Entry}
	proof, e := s.loadSelectedAnalysis(ctx, lease.Actor(), scope, target.Artifact, AnalysisArtifactQuery{VersionID: target.Version, Range: target.Range, Outputs: target.Outputs}, "analysis.artifacts.exports")
	if e != nil {
		return e
	}
	if proof.Version.Manifest != target.Manifest {
		return problem("result_integrity_unavailable", "成果导出根不匹配。", 503)
	}
	if target.Content == "inputs" {
		return s.generateInputEvidence(ctx, lease, inputAccess{Actor: lease.Actor(), Scope: monitoringScope(scope), History: true, Capability: "export"}, target.Evidence, target.CSVMode, map[string]any{"artifact_version_id": target.Version, "artifact_id": target.Artifact, "output_range": target.Range, "outputs": target.Outputs, "manifest_root": target.Manifest, "lineage_ref": proof.Version.Lineage, "engine_version": proof.Detail.Config.EngineVersion, "plan_hash": proof.Manifest.Plan})
	}
	spec := monitoring.ResultExportSpec{Schema: "analysis-export/1", Source: jsonBytes(map[string]any{"artifact": proof.Detail.Artifact, "version": proof.Detail.Version, "config": proof.Detail.Config, "run": proof.Detail.Run, "outputs": proof.Manifest.Outputs, "selected_outputs": target.Outputs, "output_range": target.Range, "history": target.History, "manifest_root": target.Manifest, "lineage_ref": proof.Version.Lineage, "revoke_position": target.RevokePosition}), CSVMode: target.CSVMode, Fields: analysisExportFields, Ordering: "output_key in selected order, output_at ascending; fixed artifact_version_id and result_id"}
	return s.monitoring.GenerateResultExport(ctx, lease, spec, func(write func([]monitoring.ExportCell) error) error {
		for _, key := range target.Outputs {
			iterator, e := s.analysisIterator(ctx, lease, proof, key, target.Range)
			if e != nil {
				return e
			}
			for {
				row, e := iterator.next(ctx)
				if e != nil {
					return e
				}
				if row == nil {
					break
				}
				if e = write(analysisExportCells(proof, *row)); e != nil {
					return e
				}
			}
		}
		return nil
	})
}
