package derivedmonitoring

import (
	"context"
	"encoding/json"
	ar "github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	"slices"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type AnalysisExportInput struct {
	WriteMeta
	Content   string           `json:"content,omitempty"`
	VersionID string           `json:"version_id"`
	Range     *NanosecondRange `json:"output_range,omitempty"`
	Outputs   []string         `json:"outputs,omitempty"`
	Format    string           `json:"format"`
	CSVMode   string           `json:"csv_mode"`
	History   bool             `json:"history"`
}
type analysisExportTarget struct {
	Selection      *analysisSelection `json:"selection,omitempty"`
	Content        string             `json:"content,omitempty"`
	Evidence       []evidenceGroup    `json:"input_evidence,omitempty"`
	Mode           string             `json:"mode"`
	Artifact       string             `json:"artifact_id"`
	Version        string             `json:"artifact_version_id"`
	Manifest       string             `json:"manifest_root"`
	RevokePosition string             `json:"revoke_position"`
	Range          *NanosecondRange   `json:"output_range,omitempty"`
	Outputs        []string           `json:"outputs"`
	CSVMode        string             `json:"csv_mode"`
	History        bool               `json:"history"`
}

func analysisExportSelection(proof analysisArtifactProof, in AnalysisExportInput) ([]string, error) {
	if in.Range != nil {
		if _, e := in.Range.rangeValue(); e != nil {
			return nil, e
		}
	}
	known := map[string]bool{}
	all := []string{}
	for _, o := range proof.Manifest.Outputs {
		known[o.Key] = true
		all = append(all, o.Key)
	}
	if len(in.Outputs) == 0 {
		return all, nil
	}
	selected := slices.Clone(in.Outputs)
	slices.Sort(selected)
	if len(selected) > 16 || len(slices.Compact(slices.Clone(selected))) != len(selected) {
		return nil, invalid("/outputs", "输出项必须唯一且最多16项。")
	}
	for _, key := range selected {
		if !known[key] {
			return nil, invalid("/outputs", "输出项不属于该固定成果。")
		}
	}
	return selected, nil
}
func analysisRevokePositionTx(ctx context.Context, tx pgx.Tx, scope Scope, version string) (string, error) {
	var position string
	e := tx.QueryRow(ctx, `SELECT COALESCE(MAX(e.source_position),0)::text FROM analysis_artifact_input_ranges i JOIN monitoring_change_ranges r ON r.tenant_id=i.tenant_id AND r.channel_id=i.channel_id AND r.affected&&i.required_range JOIN monitoring_changes e ON e.tenant_id=r.tenant_id AND e.id=r.event_id WHERE i.tenant_id=$1 AND i.version_id=$2 AND e.change_type='raw_revoke'`, scope.Tenant, version).Scan(&position)
	return position, e
}
func (s *Service) CreateAnalysisExport(ctx context.Context, a access.Actor, scope Scope, id string, in AnalysisExportInput, audit access.Meta) (monitoring.ManagedExportRecord, error) {
	var out monitoring.ManagedExportRecord
	if e := checkMeta(in.WriteMeta, false, false); e != nil {
		return out, e
	}
	if !access.ValidID(in.VersionID) {
		return out, invalid("/version_id", "请选择固定成果版本。")
	}
	if in.CSVMode == "" {
		in.CSVMode = "exact"
	}
	if e := monitoring.ValidateResultExport(in.Format, in.CSVMode); e != nil {
		return out, e
	}
	if in.Content == "" {
		in.Content = "results"
	}
	if in.Content != "results" && in.Content != "inputs" {
		return out, invalid("/content", "请选择结果或完整输入依据。")
	}
	proof, e := s.loadSelectedAnalysis(ctx, a, scope, id, AnalysisArtifactQuery{VersionID: in.VersionID, Range: in.Range, Outputs: in.Outputs}, "analysis.artifacts.exports")
	if e != nil {
		return out, e
	}
	selected, e := analysisExportSelection(proof, in)
	if e != nil {
		return out, e
	}
	var evidence []evidenceGroup
	if in.Content == "inputs" {
		evidence, e = s.analysisEvidenceGroups(ctx, inputAccess{Actor: a, Scope: monitoringScope(scope), History: true, Capability: "export"}, proof, analysisExportTarget{Range: in.Range, Outputs: selected})
		if e != nil {
			return out, e
		}
	}
	e = s.transact(ctx, a, scope, "analysis.artifacts.exports", false, func(tx pgx.Tx) error {
		found, e := analysisReceiptTx(ctx, tx, a, scope, "analysis.artifacts.exports", in.WriteMeta, in, &out)
		if e != nil {
			return e
		}
		if found {
			out, _, e = s.analysisExportTx(ctx, tx, a, scope, out.ID, true)
			return e
		}
		current, e := s.analysisArtifactSelectedTx(ctx, tx, a, scope, id, in.VersionID, proof.Selection, "export")
		if e != nil {
			return e
		}
		if current.Version.InputState == "revoked" && !in.History {
			return problem("export_invalidated", "成果依据已撤销，请显式选择历史导出。", 409)
		}
		position, e := s.selectedAnalysisRevokeTx(ctx, tx, scope, in.VersionID, proof.Selection)
		if e != nil {
			return e
		}
		if in.Content == "inputs" {
			if e = s.checkEvidenceGroupsTx(ctx, tx, inputAccess{Actor: a, Scope: monitoringScope(scope), History: true, Capability: "export"}, evidence); e != nil {
				return e
			}
		}
		target := analysisExportTarget{Selection: proof.Selection, Content: in.Content, Evidence: evidence, Mode: "analysis_export", Artifact: id, Version: in.VersionID, Manifest: current.Version.Manifest, RevokePosition: position, Range: in.Range, Outputs: selected, CSVMode: in.CSVMode, History: in.History}
		out, e = s.monitoring.CreateResultExportTx(ctx, tx, monitoring.EnqueueTask{Scope: monitoringScope(scope), ActorID: a.ID, Kind: "analysis_export", Target: uuid.NewString(), TargetRevision: "1", Input: jsonBytes(target)}, in.VersionID, in.Format)
		if e != nil {
			return e
		}
		return analysisCompleteTx(ctx, tx, a, scope, "analysis.artifacts.exports", "monitoring_export", out.ID, in.WriteMeta, in, nil, out, audit)
	})
	return out, e
}
func (s *Service) analysisExportTargetTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, record monitoring.ManagedExportRecord, available bool) (analysisExportTarget, error) {
	var target analysisExportTarget
	if record.SourceKind != "analysis" {
		return target, notFound()
	}
	if e := tx.QueryRow(ctx, `SELECT input FROM monitoring_jobs WHERE tenant_id=$1 AND id=$2 AND target_id=$3 AND actor_id=$4 AND kind='analysis_export'`, scope.Tenant, record.Job.ID, record.ID, a.ID).Scan(&target); e != nil {
		return target, e
	}
	for _, permission := range []string{"analysis.results.read", "analysis.export"} {
		if e := requireAnyDerivedTx(ctx, tx, a, scope, permission); e != nil {
			return target, e
		}
	}
	if !access.ValidID(target.Artifact) || target.Mode != "analysis_export" || target.Version != record.SourceVersion {
		return target, problem("result_integrity_unavailable", "导出固定版本不一致。", 503)
	}
	proof, e := s.analysisArtifactSelectedTx(ctx, tx, a, scope, target.Artifact, target.Version, target.Selection, "export")
	if e != nil {
		return target, e
	}
	if proof.Version.Manifest != target.Manifest {
		return target, problem("result_integrity_unavailable", "导出清单依据不一致。", 503)
	}
	if target.Content == "inputs" {
		if e = s.checkEvidenceGroupsTx(ctx, tx, inputAccess{Actor: a, Scope: monitoringScope(scope), History: true, Capability: "export"}, target.Evidence); e != nil {
			return target, e
		}
	}
	if available {
		if e = record.CheckAvailable(); e != nil {
			return target, e
		}
		if proof.Version.InputState == "revoked" && !target.History {
			return target, problem("export_invalidated", "成果依据已撤销，请重新申请历史导出。", 409)
		}
		position, e := s.selectedAnalysisRevokeTx(ctx, tx, scope, target.Version, target.Selection)
		if e != nil {
			return target, e
		}
		if position != target.RevokePosition {
			return target, problem("export_invalidated", "导出申请后相关输入发生撤销，请重新申请。", 409)
		}
	}
	return target, nil
}
func (s *Service) analysisExportTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, id string, available bool) (monitoring.ManagedExportRecord, analysisExportTarget, error) {
	record, e := s.monitoring.ResultExportTx(ctx, tx, monitoringScope(scope), id, a.ID)
	if e != nil {
		return record, analysisExportTarget{}, e
	}
	target, e := s.analysisExportTargetTx(ctx, tx, a, scope, record, available)
	return record, target, e
}
func (s *Service) authorizeAnalysisExportTaskTx(ctx context.Context, tx pgx.Tx, task monitoring.Task, a access.Actor, scope monitoring.Scope, execute, manage bool) error {
	if task.Scope != scope || a.ID != task.ActorID {
		return notFound()
	}
	view := Scope{Tenant: scope.Tenant, Project: scope.Project, Entry: scope.Entry}
	op := access.DerivedOperation(view.Entry, "analysis.exports.detail")
	if view.Entry == "project" {
		if _, e := access.RequireProjectTx(ctx, tx, a.ID, op, view.Tenant, view.Project); e != nil {
			return e
		}
	} else if e := access.RequireScopedTx(ctx, tx, a.ID, op, view.Tenant); e != nil {
		return e
	}
	if manage {
		if e := requireAnyDerivedTx(ctx, tx, a, view, "runs.manage"); e != nil {
			return e
		}
	}
	record, _, e := s.analysisExportTx(ctx, tx, a, view, task.Job.Target, execute)
	if e != nil {
		return e
	}
	if record.Job.ID != task.Job.ID {
		return notFound()
	}
	return nil
}
func (s *Service) AnalysisExport(ctx context.Context, a access.Actor, scope Scope, id string) (monitoring.ManagedExportRecord, error) {
	var out monitoring.ManagedExportRecord
	e := s.transact(ctx, a, scope, "analysis.exports.detail", false, func(tx pgx.Tx) error {
		var e error
		out, _, e = s.analysisExportTx(ctx, tx, a, scope, id, false)
		return e
	})
	return out, e
}
func (s *Service) analysisExportLoad(ctx context.Context, a access.Actor, scope Scope, id, action string) (monitoring.ManagedExportRecord, func(context.Context) error, error) {
	var record monitoring.ManagedExportRecord
	check := func(ctx context.Context) error {
		return s.transact(ctx, a, scope, action, false, func(tx pgx.Tx) error {
			var e error
			record, _, e = s.analysisExportTx(ctx, tx, a, scope, id, true)
			return e
		})
	}
	e := check(ctx)
	return record, check, e
}
func (s *Service) AnalysisExportFiles(ctx context.Context, a access.Actor, scope Scope, id string) (monitoring.ResultExportManifest, error) {
	r, guard, e := s.analysisExportLoad(ctx, a, scope, id, "analysis.exports.files")
	if e != nil {
		return monitoring.ResultExportManifest{}, e
	}
	return s.monitoring.ResultExportFiles(ctx, r, guard)
}
func (s *Service) AnalysisExportDownload(ctx context.Context, a access.Actor, scope Scope, id, part string) (monitoring.Download, error) {
	r, guard, e := s.analysisExportLoad(ctx, a, scope, id, "analysis.exports.download")
	if e != nil {
		return monitoring.Download{}, e
	}
	out, e := s.monitoring.ResultExportDownload(ctx, r, part, guard)
	if e != nil {
		return out, e
	}
	e = s.transact(ctx, a, scope, "analysis.exports.download", false, func(tx pgx.Tx) error {
		if _, _, e := s.analysisExportTx(ctx, tx, a, scope, id, true); e != nil {
			return e
		}
		var e error
		out.Observer, e = s.access.BeginTransferTx(ctx, tx, a.ID, "", ar.Scope{Kind: scope.Entry, Tenant: scope.Tenant, Project: scope.Project}, "derived", ar.Link{Kind: "domain_file", Module: "derived", TargetKind: "export", TargetID: id, Generation: monitoring.JobGeneration(r.Job), Relation: "output"})
		return e
	})
	if e != nil {
		return out, e
	}
	out.Write = out.Observer.Wrap(out.Write)
	return out, nil
}
func (s *Service) ControlAnalysisExport(ctx context.Context, a access.Actor, scope Scope, id, action string, in WriteMeta, audit access.Meta) (monitoring.ManagedExportRecord, error) {
	var out monitoring.ManagedExportRecord
	if e := checkMeta(in, true, true); e != nil {
		return out, e
	}
	if action != "cancel" && action != "retry" {
		return out, notFound()
	}
	e := s.transact(ctx, a, scope, "analysis.exports."+action, false, func(tx pgx.Tx) error {
		found, e := analysisReceiptTx(ctx, tx, a, scope, "analysis.exports."+action, in, in, &out)
		if e != nil {
			return e
		}
		if found {
			out, _, e = s.analysisExportTx(ctx, tx, a, scope, out.ID, false)
			return e
		}
		record, _, e := s.analysisExportTx(ctx, tx, a, scope, id, action == "retry")
		if e != nil {
			return e
		}
		if _, e = s.monitoring.ControlTaskTx(ctx, tx, a, monitoringScope(scope), record.Job.ID, in.Revision, action); e != nil {
			return e
		}
		out, _, e = s.analysisExportTx(ctx, tx, a, scope, id, false)
		if e != nil {
			return e
		}
		return analysisCompleteTx(ctx, tx, a, scope, "analysis.exports."+action, "monitoring_export", id, in, in, record, out, audit)
	})
	return out, e
}
func decodeAnalysisExport(raw json.RawMessage) (analysisExportTarget, error) {
	var target analysisExportTarget
	e := json.Unmarshal(raw, &target)
	return target, e
}

type AnalysisExportPage struct {
	Items      []monitoring.ManagedExportRecord `json:"items"`
	NextCursor string                           `json:"next_cursor"`
}

func (s *Service) AnalysisExports(ctx context.Context, a access.Actor, scope Scope, filter MetricFilter) (AnalysisExportPage, error) {
	out := AnalysisExportPage{Items: []monitoring.ManagedExportRecord{}}
	limit := filter.Limit
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > 100 || filter.Cursor != "" && !access.ValidID(filter.Cursor) {
		return out, invalid("/cursor", "导出分页参数无效。")
	}
	e := s.transact(ctx, a, scope, "analysis.exports.list", false, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `SELECT e.id::text FROM monitoring_exports e JOIN monitoring_jobs j ON j.tenant_id=e.tenant_id AND j.id=e.job_id WHERE e.tenant_id=$1 AND j.project_id IS NOT DISTINCT FROM $2::uuid AND j.actor_id=$3 AND j.kind='analysis_export' AND ($4::uuid IS NULL OR e.id>$4) ORDER BY e.id LIMIT $5`, scope.Tenant, nullable(scope.Project), a.ID, nullable(filter.Cursor), limit+1)
		if e != nil {
			return e
		}
		ids := []string{}
		for rows.Next() {
			var id string
			if e = rows.Scan(&id); e != nil {
				rows.Close()
				return e
			}
			ids = append(ids, id)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if len(ids) > limit {
			out.NextCursor = ids[limit-1]
			ids = ids[:limit]
		}
		for _, id := range ids {
			record, _, e := s.analysisExportTx(ctx, tx, a, scope, id, false)
			if analysisRestricted(e) {
				continue
			}
			if e != nil {
				return e
			}
			out.Items = append(out.Items, record)
		}
		return nil
	})
	return out, e
}
