package derivedmonitoring

import (
	"context"
	"encoding/json"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type GenerationExportInput struct {
	WriteMeta
	Range   *NanosecondRange `json:"output_range,omitempty"`
	Format  string           `json:"format"`
	CSVMode string           `json:"csv_mode"`
}
type generationExportTarget struct {
	Mode           string          `json:"mode"`
	Generation     string          `json:"generation_id"`
	Channel        string          `json:"channel_id"`
	Manifest       string          `json:"manifest_root"`
	Lineage        string          `json:"lineage_ref"`
	Range          NanosecondRange `json:"output_range"`
	Evidence       []evidenceGroup `json:"input_evidence"`
	RevokePosition string          `json:"revoke_position"`
	CSVMode        string          `json:"csv_mode"`
}

func evidenceRevokePositionTx(ctx context.Context, tx pgx.Tx, tenant string, groups []evidenceGroup) (string, error) {
	needs := []monitoring.RawNeed{}
	for _, group := range groups {
		needs = append(needs, group.Direct...)
	}
	var e error
	needs, e = normalizedNeeds(needs)
	if e != nil {
		return "", e
	}
	var position string
	e = tx.QueryRow(ctx, `SELECT COALESCE(max(e.source_position),0)::text FROM monitoring_changes e JOIN monitoring_change_ranges r ON r.tenant_id=e.tenant_id AND r.event_id=e.id JOIN jsonb_to_recordset($2::jsonb) n(channel_id uuid,from_ns bigint,to_ns bigint) ON n.channel_id=r.channel_id AND r.affected&&int8range(n.from_ns,n.to_ns,'[)') WHERE e.tenant_id=$1 AND e.change_type='raw_revoke'`, tenant, jsonBytes(needs)).Scan(&position)
	return position, e
}
func (s *Service) CreateGenerationExport(ctx context.Context, a access.Actor, scope Scope, id string, in GenerationExportInput, audit access.Meta) (monitoring.ManagedExportRecord, error) {
	var out monitoring.ManagedExportRecord
	if e := checkMeta(in.WriteMeta, false, false); e != nil {
		return out, e
	}
	if in.CSVMode == "" {
		in.CSVMode = "exact"
	}
	if e := monitoring.ValidateResultExport(in.Format, in.CSVMode); e != nil {
		return out, e
	}
	proof, e := s.historyGeneration(ctx, a, scope, id, in.Range, "generations.exports")
	if e != nil {
		return out, e
	}
	auth := inputAccess{Actor: a, Scope: monitoringScope(scope), History: true, Capability: "export"}
	versions, e := s.evidenceVersions(ctx, scope.Tenant, proof.generation.lineage)
	if e != nil {
		return out, e
	}
	root := evidenceGroup{Configuration: jsonBytes(proof.config), Versions: versions, Path: "root", Role: "candidate", Config: proof.config.ID, Rule: proof.config.Rule, Snapshot: proof.manifest.Input, Direct: proof.needs, Sources: proof.sources}
	groups, e := s.evidenceGroups(ctx, auth, root, proof.manifest.Baselines)
	if e != nil {
		return out, e
	}
	e = s.transact(ctx, a, scope, "generations.exports", false, func(tx pgx.Tx) error {
		found, e := analysisReceiptTx(ctx, tx, a, scope, "generations.exports", in.WriteMeta, in, &out)
		if e != nil {
			return e
		}
		if found {
			out, _, e = s.generationExportTx(ctx, tx, a, scope, out.ID, true)
			return e
		}
		if e = s.recheckHistoryTx(ctx, tx, a, scope, &proof); e != nil {
			return e
		}
		if e = s.monitoring.AuthorizeCalculationOutputTx(ctx, tx, a, monitoringScope(scope), proof.generation.ChannelID, []monitoring.Window{rangeWindow(proof.output)}, true, "export"); e != nil {
			return e
		}
		if e = s.checkEvidenceGroupsTx(ctx, tx, auth, groups); e != nil {
			return e
		}
		position, e := evidenceRevokePositionTx(ctx, tx, scope.Tenant, groups)
		if e != nil {
			return e
		}
		target := generationExportTarget{Mode: "derived_evidence_export", Generation: id, Channel: proof.generation.ChannelID, Manifest: proof.generation.manifest, Lineage: proof.generation.lineage, Range: rangeDTO(proof.output.From, proof.output.To), Evidence: groups, RevokePosition: position, CSVMode: in.CSVMode}
		out, e = s.monitoring.CreateResultExportTx(ctx, tx, monitoring.EnqueueTask{Scope: monitoringScope(scope), ActorID: a.ID, Kind: "derived_evidence_export", Target: uuid.NewString(), TargetRevision: "1", Input: jsonBytes(target)}, id, in.Format)
		if e != nil {
			return e
		}
		return analysisCompleteTx(ctx, tx, a, scope, "generations.exports", "monitoring_export", out.ID, in.WriteMeta, in, nil, out, audit)
	})
	return out, e
}
func (s *Service) generationExportTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, id string, available bool) (monitoring.ManagedExportRecord, generationExportTarget, error) {
	var target generationExportTarget
	record, e := s.monitoring.ResultExportTx(ctx, tx, monitoringScope(scope), id, a.ID)
	if e != nil {
		return record, target, e
	}
	if record.SourceKind != "derived_evidence" {
		return record, target, notFound()
	}
	if e = tx.QueryRow(ctx, `SELECT input FROM monitoring_jobs WHERE tenant_id=$1 AND id=$2 AND target_id=$3 AND actor_id=$4 AND kind='derived_evidence_export'`, scope.Tenant, record.Job.ID, id, a.ID).Scan(&target); e != nil {
		return record, target, e
	}
	for _, capability := range []string{"channels.read", "results.history"} {
		if e = requireAnyDerivedTx(ctx, tx, a, scope, capability); e != nil {
			return record, target, e
		}
	}
	if target.Mode != "derived_evidence_export" || target.Generation != record.SourceVersion {
		return record, target, problem("result_integrity_unavailable", "输入依据导出版本不一致。", 503)
	}
	generation, e := scanGeneration(tx.QueryRow(ctx, generationSelect+` WHERE tenant_id=$1 AND id=$2`, scope.Tenant, target.Generation))
	if e != nil {
		return record, target, e
	}
	if generation.manifest != target.Manifest || generation.lineage != target.Lineage || generation.ChannelID != target.Channel {
		return record, target, problem("result_integrity_unavailable", "输入依据导出身份不一致。", 503)
	}
	r, e := target.Range.rangeValue()
	if e != nil {
		return record, target, e
	}
	if _, e = s.channelTx(ctx, tx, a, scope, target.Channel); e != nil {
		return record, target, e
	}
	if e = s.monitoring.AuthorizeCalculationOutputTx(ctx, tx, a, monitoringScope(scope), target.Channel, []monitoring.Window{rangeWindow(r)}, true, "export"); e != nil {
		return record, target, e
	}
	if e = requireResultAvailableTx(ctx, tx, scope.Tenant, "derived_generations", target.Generation); e != nil {
		return record, target, e
	}
	if e = s.checkEvidenceGroupsTx(ctx, tx, inputAccess{Actor: a, Scope: monitoringScope(scope), History: true, Capability: "export"}, target.Evidence); e != nil {
		return record, target, e
	}
	if available {
		if e = record.CheckAvailable(); e != nil {
			return record, target, e
		}
		position, e := evidenceRevokePositionTx(ctx, tx, scope.Tenant, target.Evidence)
		if e != nil {
			return record, target, e
		}
		if position != target.RevokePosition {
			return record, target, problem("export_invalidated", "导出申请后相关依据发生撤销，请重新申请历史依据导出。", 409)
		}
	}
	return record, target, nil
}
func (s *Service) authorizeGenerationExportTaskTx(ctx context.Context, tx pgx.Tx, task monitoring.Task, a access.Actor, scope monitoring.Scope, execute, manage bool) error {
	if task.Scope != scope || a.ID != task.ActorID {
		return notFound()
	}
	view := Scope{Tenant: scope.Tenant, Project: scope.Project, Entry: scope.Entry}
	op := access.DerivedOperation(view.Entry, "evidence.exports.detail")
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
	record, _, e := s.generationExportTx(ctx, tx, a, view, task.Job.Target, execute)
	if e != nil {
		return e
	}
	if record.Job.ID != task.Job.ID {
		return notFound()
	}
	return nil
}
func (s *Service) executeGenerationExport(ctx context.Context, lease monitoring.TaskLease) error {
	var target generationExportTarget
	if e := json.Unmarshal(lease.Task().Input, &target); e != nil {
		return e
	}
	return s.generateInputEvidence(ctx, lease, inputAccess{Actor: lease.Actor(), Scope: lease.Task().Scope, History: true, Capability: "export"}, target.Evidence, target.CSVMode, map[string]any{"generation_id": target.Generation, "channel_id": target.Channel, "output_range": target.Range, "manifest_root": target.Manifest, "lineage_ref": target.Lineage})
}
func (s *Service) GenerationExport(ctx context.Context, a access.Actor, scope Scope, id string) (monitoring.ManagedExportRecord, error) {
	var out monitoring.ManagedExportRecord
	e := s.transact(ctx, a, scope, "evidence.exports.detail", false, func(tx pgx.Tx) error {
		var e error
		out, _, e = s.generationExportTx(ctx, tx, a, scope, id, false)
		return e
	})
	return out, e
}
func (s *Service) generationExportLoad(ctx context.Context, a access.Actor, scope Scope, id, action string) (monitoring.ManagedExportRecord, func(context.Context) error, error) {
	var record monitoring.ManagedExportRecord
	guard := func(ctx context.Context) error {
		return s.transact(ctx, a, scope, action, false, func(tx pgx.Tx) error {
			var e error
			record, _, e = s.generationExportTx(ctx, tx, a, scope, id, true)
			return e
		})
	}
	e := guard(ctx)
	return record, guard, e
}
func (s *Service) GenerationExportFiles(ctx context.Context, a access.Actor, scope Scope, id string) (monitoring.ResultExportManifest, error) {
	r, guard, e := s.generationExportLoad(ctx, a, scope, id, "evidence.exports.files")
	if e != nil {
		return monitoring.ResultExportManifest{}, e
	}
	return s.monitoring.ResultExportFiles(ctx, r, guard)
}
func (s *Service) GenerationExportDownload(ctx context.Context, a access.Actor, scope Scope, id, part string) (monitoring.Download, error) {
	r, guard, e := s.generationExportLoad(ctx, a, scope, id, "evidence.exports.download")
	if e != nil {
		return monitoring.Download{}, e
	}
	return s.monitoring.ResultExportDownload(ctx, r, part, guard)
}
func (s *Service) ControlGenerationExport(ctx context.Context, a access.Actor, scope Scope, id, action string, in WriteMeta, audit access.Meta) (monitoring.ManagedExportRecord, error) {
	var out monitoring.ManagedExportRecord
	if e := checkMeta(in, true, true); e != nil {
		return out, e
	}
	if action != "cancel" && action != "retry" {
		return out, notFound()
	}
	op := "evidence.exports." + action
	e := s.transact(ctx, a, scope, op, false, func(tx pgx.Tx) error {
		found, e := analysisReceiptTx(ctx, tx, a, scope, op, in, in, &out)
		if e != nil {
			return e
		}
		if found {
			out, _, e = s.generationExportTx(ctx, tx, a, scope, out.ID, false)
			return e
		}
		before, _, e := s.generationExportTx(ctx, tx, a, scope, id, action == "retry")
		if e != nil {
			return e
		}
		if _, e = s.monitoring.ControlTaskTx(ctx, tx, a, monitoringScope(scope), before.Job.ID, in.Revision, action); e != nil {
			return e
		}
		out, _, e = s.generationExportTx(ctx, tx, a, scope, id, false)
		if e != nil {
			return e
		}
		return analysisCompleteTx(ctx, tx, a, scope, op, "monitoring_export", id, in, in, before, out, audit)
	})
	return out, e
}

func (s *Service) GenerationExports(ctx context.Context, a access.Actor, scope Scope, filter MetricFilter) (AnalysisExportPage, error) {
	out := AnalysisExportPage{Items: []monitoring.ManagedExportRecord{}}
	limit := filter.Limit
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > 100 || filter.Cursor != "" && !access.ValidID(filter.Cursor) {
		return out, invalid("/cursor", "导出分页参数无效。")
	}
	e := s.transact(ctx, a, scope, "evidence.exports.list", false, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `SELECT e.id::text FROM monitoring_exports e JOIN monitoring_jobs j ON j.tenant_id=e.tenant_id AND j.id=e.job_id WHERE e.tenant_id=$1 AND j.project_id IS NOT DISTINCT FROM $2::uuid AND j.actor_id=$3 AND j.kind='derived_evidence_export' AND ($4::uuid IS NULL OR e.id>$4) ORDER BY e.id LIMIT $5`, scope.Tenant, nullable(scope.Project), a.ID, nullable(filter.Cursor), limit+1)
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
			record, _, e := s.generationExportTx(ctx, tx, a, scope, id, false)
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
