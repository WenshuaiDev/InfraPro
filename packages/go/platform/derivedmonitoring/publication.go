package derivedmonitoring

import (
	"context"
	"errors"
	"strconv"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/planning"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/resultstore"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/jackc/pgx/v5"
)

var errHeadMoved = errors.New("result head advanced while sealing range map")

func (s *Service) publishFormal(ctx context.Context, lease monitoring.TaskLease, run runRecord, target publicationTarget, manifest resultManifest, manifestRoot string) error {
	output, e := run.OutputRange.rangeValue()
	if e != nil {
		return e
	}
	sourcePosition, _ := strconv.ParseInt(manifest.Input.SourcePosition, 10, 64)
	for iteration := 0; iteration < 8; iteration++ {
		var oldRoot, oldHistory string
		var headRevision int64
		var already bool
		e = lease.Transaction(ctx, func(tx pgx.Tx) error {
			if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM derived_publication_receipts WHERE tenant_id=$1 AND run_id=$2)`, run.Scope.Tenant, run.ID).Scan(&already); e != nil {
				return e
			}
			if already {
				return nil
			}
			return tx.QueryRow(ctx, `SELECT COALESCE(current_root,''),COALESCE(history_root,''),head_revision FROM derived_channel_heads WHERE tenant_id=$1 AND channel_id=$2`, run.Scope.Tenant, run.ChannelID).Scan(&oldRoot, &oldHistory, &headRevision)
		})
		if e != nil {
			return e
		}
		if already {
			return nil
		}
		entries, e := s.results.ReadRanges(ctx, run.Scope.Tenant, oldRoot, output, 10000)
		if e != nil {
			return e
		}
		for _, entry := range entries {
			position, e := strconv.ParseInt(entry.SourcePosition, 10, 64)
			if e != nil {
				return e
			}
			if position > sourcePosition {
				return &monitoring.TaskError{Code: "superseded", Message: "此范围已有更新输入版本的结果。", Disposition: "cancelled"}
			}
		}
		current, e := s.results.ReplaceRange(ctx, run.Scope.Tenant, oldRoot, resultstore.RangeEntry{From: output.From, To: output.To, Generation: target.Generation, Manifest: manifestRoot, SourcePosition: manifest.Input.SourcePosition})
		if e != nil {
			return e
		}
		history, e := s.results.SealDocument(ctx, run.Scope.Tenant, "history", map[string]any{"schema": "derived-history/1", "previous": oldHistory, "previous_current": oldRoot, "current": current, "generation_id": target.Generation, "manifest_root": manifestRoot, "range": run.OutputRange})
		if e != nil {
			return e
		}
		if e = lease.Progress(ctx, map[string]string{"phase": "publishing", "output_rows": strconv.FormatUint(manifest.Rows, 10)}); e != nil {
			return e
		}
		e = lease.Transaction(ctx, func(tx pgx.Tx) error {
			var revision int64
			if e := tx.QueryRow(ctx, `SELECT head_revision FROM derived_channel_heads WHERE tenant_id=$1 AND channel_id=$2 FOR UPDATE`, run.Scope.Tenant, run.ChannelID).Scan(&revision); e != nil {
				return e
			}
			if revision != headRevision {
				return errHeadMoved
			}
			state, e := s.checkRunInputsTx(ctx, tx, lease.Actor(), run, manifest.Input.ID)
			if e != nil {
				return e
			}
			if state.Changed {
				return &monitoring.TaskError{Code: "input_changed", Message: "相关输入已更新，需要按新的固定快照计算。", Disposition: "needs_action"}
			}
			_, baselines, e := s.runBaselinesTx(ctx, tx, lease.Actor(), run)
			if e != nil {
				return e
			}
			if fingerprint(baselines) != fingerprint(manifest.Baselines) {
				return &monitoring.TaskError{Code: "baseline_changed", Message: "固定基准状态已经变化，需要重新计算。", Disposition: "needs_action"}
			}
			// All immutable physical receipts were verified before this short commit.
			for _, block := range manifest.Blocks {
				var sealed bool
				e = tx.QueryRow(ctx, `SELECT state='sealed' AND manifest=$4::jsonb FROM derived_write_attempts WHERE tenant_id=$1 AND run_id=$2 AND id=$3`, run.Scope.Tenant, run.ID, block.Attempt, jsonBytes(block)).Scan(&sealed)
				if e != nil {
					return e
				}
				if !sealed {
					return problem("result_manifest_incomplete", "结果块尚未封存。", 409)
				}
			}
			var position int64
			if e = tx.QueryRow(ctx, `UPDATE monitoring_tenant_states SET commit_sequence=commit_sequence+1 WHERE tenant_id=$1 RETURNING commit_sequence`, run.Scope.Tenant).Scan(&position); e != nil {
				return e
			}
			_, e = tx.Exec(ctx, `INSERT INTO derived_generations(id,tenant_id,channel_id,run_id,config_id,output_range,manifest_root,lineage_root,raw_snapshot_id,source_position,publish_position,row_count,status_counts) VALUES($1,$2,$3,$4,$5,$6::int8range,$7,$8,$9,$10,$11,$12,$13)`, target.Generation, run.Scope.Tenant, run.ChannelID, run.ID, run.ConfigID, rangeSQL(output), manifestRoot, manifest.Lineage, manifest.Input.ID, sourcePosition, position, int64(manifest.Rows), jsonBytes(manifest.States))
			if e != nil {
				return e
			}
			_, e = tx.Exec(ctx, `UPDATE derived_channel_heads SET head_revision=head_revision+1,current_root=$3,history_root=$4,coverage_root=$3,summary_root=$5,checked_source_position=$6 WHERE tenant_id=$1 AND channel_id=$2`, run.Scope.Tenant, run.ChannelID, current, history, manifestRoot, position)
			if e != nil {
				return e
			}
			// Subtract only the completed range at or before this fixed source position.
			// A newer change, including one observed while an old task was running, survives.
			_, e = tx.Exec(ctx, `WITH removed AS(DELETE FROM derived_dirty_ranges WHERE tenant_id=$1 AND channel_id=$2 AND required_position<=$3 AND affected && $4::int8range RETURNING affected,required_position,config_revision,graph_revision,reason) INSERT INTO derived_dirty_ranges(tenant_id,channel_id,affected,required_position,config_revision,graph_revision,reason) SELECT $1,$2,unnest(int8multirange(affected)-int8multirange($4::int8range)),required_position,config_revision,graph_revision,reason FROM removed`, run.Scope.Tenant, run.ChannelID, sourcePosition, rangeSQL(output))
			if e != nil {
				return e
			}
			health, reason := "healthy", ""
			if manifest.Rows == 0 {
				health = "waiting"
			}
			if n, _ := strconv.ParseUint(manifest.States["baseline_invalid"], 10, 64); n > 0 {
				health, reason = "blocked", "baseline_invalid"
			}
			if _, e = tx.Exec(ctx, `UPDATE derived_channels SET health=$3,blocked_reason=$4 WHERE tenant_id=$1 AND channel_id=$2`, run.Scope.Tenant, run.ChannelID, health, reason); e != nil {
				return e
			}
			if _, e = tx.Exec(ctx, `INSERT INTO derived_publication_receipts(tenant_id,run_id,generation_id,head_revision,current_root,history_root) VALUES($1,$2,$3,$4,$5,$6)`, run.Scope.Tenant, run.ID, target.Generation, headRevision+1, current, history); e != nil {
				return e
			}
			if _, e = monitoring.AppendChangeTx(ctx, tx, run.Scope.Tenant, position, "derived_publish", "derived", run.ChannelID, []monitoring.ChangeRange{{Channel: run.ChannelID, From: &output.From, To: output.To}}); e != nil {
				return e
			}
			if e = access.Audit(ctx, tx, "derived.results.published", "derived_channel", run.ChannelID, "", map[string]any{"head_revision": headRevision}, map[string]any{"generation_id": target.Generation, "run_id": run.ID, "output_range": run.OutputRange, "row_count": strconv.FormatUint(manifest.Rows, 10), "manifest_root": manifestRoot}, access.Meta{ActorType: "system", ActorScope: "tenant", TenantID: run.Scope.Tenant, Reason: "固定输入计算结果发布"}); e != nil {
				return e
			}
			e = lease.CompleteTx(ctx, tx, map[string]string{"phase": "completed", "generation_id": target.Generation, "output_rows": strconv.FormatUint(manifest.Rows, 10)})
			if e != nil {
				return e
			}
			return s.wakeDependentRunsTx(ctx, tx, run.Scope.Tenant, run.ID)
		})
		if errors.Is(e, errHeadMoved) {
			continue
		}
		return e
	}
	return problem("head_changed", "结果头持续变化，任务将重新校验后重试。", 503)
}

func intersectBounded(r planning.Range, to int64) (planning.Range, bool) {
	return planning.Intersection(r, planning.Range{From: r.From, To: &to})
}
