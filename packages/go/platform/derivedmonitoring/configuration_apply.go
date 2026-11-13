package derivedmonitoring

import (
	"context"
	"encoding/json"
	"math"
	"strconv"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/compute"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/planning"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func activeAssignmentsTx(ctx context.Context, tx pgx.Tx, tenant, channel string) ([]ConfigAssignment, error) {
	rows, e := tx.Query(ctx, assignmentSelect+` WHERE tenant_id=$1 AND channel_id=$2 AND superseded_revision IS NULL ORDER BY lower(effective_range),id LIMIT 10001`, tenant, channel)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []ConfigAssignment{}
	for rows.Next() {
		v, e := scanAssignment(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	if len(out) > 10000 {
		return nil, problem("plan_limit_exceeded", "配置分配数量超过单次操作预算。", 422)
	}
	return out, rows.Err()
}
func replacedAssignments(all []ConfigAssignment, selected planning.Range, config string) ([]ConfigAssignment, []ConfigAssignment) {
	replaced, next := []ConfigAssignment{}, []ConfigAssignment{}
	for _, v := range all {
		r, _ := v.Range.rangeValue()
		if _, ok := planning.Intersection(r, selected); !ok {
			next = append(next, v)
			continue
		}
		replaced = append(replaced, v)
		for _, keep := range planning.Subtract(r, selected) {
			next = append(next, ConfigAssignment{ConfigID: v.ConfigID, Range: rangeDTO(keep.From, keep.To)})
		}
	}
	next = append(next, ConfigAssignment{ConfigID: config, Range: rangeDTO(selected.From, selected.To)})
	return replaced, next
}

// Each assignment keeps its own plan and baseline. The new tenant mandate
// covers their union; a task must also match an active assignment and all fences.
func (s *Service) assignedRequirementsTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, assignments []ConfigAssignment, outputs []planning.Range, proofs []monitoring.RawNeed) ([]monitoring.RawNeed, error) {
	needs := []monitoring.RawNeed{}
	prefixChannels := []string{}
	for _, assignment := range assignments {
		r, e := assignment.Range.rangeValue()
		if e != nil {
			return nil, e
		}
		cfg, e := scanConfig(tx.QueryRow(ctx, configSelect+` WHERE tenant_id=$1 AND id=$2`, scope.Tenant, assignment.ConfigID))
		if e != nil {
			return nil, e
		}
		p, e := s.compile(cfg.Rule, "channel", cfg.Parameters)
		if e != nil {
			return nil, e
		}
		for _, allowed := range outputs {
			part, ok := planning.Intersection(r, allowed)
			if !ok {
				continue
			}
			items, e := s.plannedInputsTx(ctx, tx, scope.Tenant, cfg, p, part)
			if e != nil {
				return nil, e
			}
			for _, prefix := range inputPrefixes(p, cfg, items, part) {
				prefixChannels = append(prefixChannels, prefix.Channel)
			}
			needs = append(needs, items...)
			baselines, e := s.baselineNeedsTx(ctx, tx, scope.Tenant, cfg, p)
			if e != nil {
				return nil, e
			}
			needs = append(needs, baselines...)
		}
	}
	for i, n := range needs {
		for _, proof := range proofs {
			if proof.Channel == n.Channel && proof.From != nil && n.From != nil && *proof.From < *n.From {
				needs[i].From = proof.From
				n = needs[i]
			}
		}
	}
	var e error
	needs, e = normalizedNeeds(needs)
	if e != nil {
		return nil, e
	}
	if len(needs) == 0 {
		return needs, nil
	}
	if e = s.authorizeInputsTx(ctx, tx, a, monitoringScope(scope), needs, false); e != nil {
		return nil, e
	}
	if len(prefixChannels) > 0 {
		needs, e = s.monitoring.CalculationPrefixAuthorityTx(ctx, tx, a, monitoringScope(scope), needs, prefixChannels)
		if e != nil {
			return nil, e
		}
	}
	return s.expandInputMandateTx(ctx, tx, a, monitoringScope(scope), needs)
}
func configPreviewHash(v ConfigPreview) string {
	v.ID = ""
	v.Hash = ""
	v.ExpiresAt = time.Time{}
	return fingerprint(v)
}

func (s *Service) prepareConfigTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, id string, in ConfigPreviewInput, effective string) (ConfigPreview, []ConfigAssignment, *compute.Plan, error) {
	out := ConfigPreview{Proposal: in.ConfigProposal, ChannelID: id, EffectiveAtNS: effective, Assignments: []ConfigAssignment{}, InputRanges: []monitoring.RawNeed{}, OutputRanges: []NanosecondRange{}}
	if in.Mode != "ordinary" && in.Mode != "correction" {
		return out, nil, nil, invalid("/mode", "请选择普通变更或明确历史修正。")
	}
	channel, e := s.maintenanceTargetTx(ctx, tx, a, scope, id, in.Mode == "correction")
	if e != nil {
		return out, nil, nil, e
	}
	if channel.ConfigRevision != in.Revision {
		return out, nil, nil, conflict()
	}
	if channel.ControlState == "draft" {
		return out, nil, nil, problem("invalid_control_transition", "草稿通道保存配置后先试算并启用，无需生效分配操作。", 409)
	}
	if in.Mode == "correction" {
		if e = requireAnyDerivedTx(ctx, tx, a, scope, "channels.backfill"); e != nil {
			return out, nil, nil, e
		}
	}
	_, cfg, p, e := s.calculationConfigTx(ctx, tx, a, scope, id, in.ConfigID)
	if e != nil {
		return out, nil, nil, e
	}
	if in.Mode == "ordinary" && cfg.ChangeKind == "correction" {
		return out, nil, nil, invalid("/mode", "修正配置必须使用明确修正操作。")
	}
	selected, e := in.Range.rangeValue()
	if e != nil {
		return out, nil, nil, e
	}
	if e = checkRetiredRange(channel, selected); e != nil {
		return out, nil, nil, e
	}
	now, _ := strconv.ParseInt(effective, 10, 64)
	if in.Mode == "ordinary" && (selected.From < now || selected.To != nil) {
		return out, nil, nil, invalid("/output_range", "普通变更从当前或未来时刻生效；修改历史须使用修正并限定区间。")
	}
	if in.Mode == "correction" && selected.To == nil {
		return out, nil, nil, invalid("/output_range/to_ns", "历史修正需要明确结束边界。")
	}
	assignments, e := activeAssignmentsTx(ctx, tx, scope.Tenant, id)
	if e != nil {
		return out, nil, nil, e
	}
	if in.Mode == "correction" {
		ranges := []planning.Range{}
		for _, v := range assignments {
			r, _ := v.Range.rangeValue()
			ranges = append(ranges, r)
		}
		covered, e := planning.Covered([]planning.Range{selected}, ranges)
		if e != nil {
			return out, nil, nil, e
		}
		if !covered {
			return out, nil, nil, invalid("/output_range", "修正区间必须完全属于已有配置分配。")
		}
	}
	var next []ConfigAssignment
	out.Assignments, next = replacedAssignments(assignments, selected, cfg.ID)
	ids := []string{}
	for _, b := range cfg.InputBindings {
		ids = append(ids, b.ChannelID)
	}
	if e = s.graphTx(ctx, tx, scope.Tenant, id, ids); e != nil {
		return out, nil, nil, e
	}
	if e = tx.QueryRow(ctx, `SELECT revision::text FROM derived_graph_states WHERE tenant_id=$1`, scope.Tenant).Scan(&out.GraphRevision); e != nil {
		return out, nil, nil, e
	}
	out.ConfigRevision = channel.ConfigRevision
	out.ControlRevision = channel.ControlRevision
	out.ConfigHash = cfg.ConfigHash
	var trial TrialResult
	e = tx.QueryRow(ctx, `SELECT result FROM derived_trial_receipts WHERE tenant_id=$1 AND project_id IS NOT DISTINCT FROM $2::uuid AND actor_id=$3 AND id=$4 AND channel_id=$5 AND config_id=$6 AND config_hash=$7 AND expires_at>clock_timestamp()`, scope.Tenant, nullable(scope.Project), a.ID, in.TrialReceiptID, id, cfg.ID, cfg.ConfigHash).Scan(&trial)
	if e == pgx.ErrNoRows {
		return out, nil, nil, problem("trial_required", "需要此配置、空间和当前用户的有效试算收据。", 409)
	}
	if e != nil {
		return out, nil, nil, e
	}
	state, e := s.checkUserInputsTx(ctx, tx, a, monitoringScope(scope), trial.InputSnapshot.ID, false)
	if e != nil {
		return out, nil, nil, e
	}
	if state.Changed || trial.Stale {
		return out, nil, nil, problem("trial_stale", "试算输入已经变化。", 409)
	}
	_, baselineProofs, e := s.baselinesTx(ctx, tx, a, monitoringScope(scope), nil, cfg.ID)
	if e != nil {
		return out, nil, nil, e
	}
	if fingerprint(baselineProofs) != fingerprint(trial.Baselines) {
		return out, nil, nil, problem("trial_stale", "固定基准依据已变化。", 409)
	}
	out.WaitingForInput = trial.WaitingForInput
	if trial.WaitingForInput && !in.AllowWaiting {
		return out, nil, nil, problem("waiting_confirmation_required", "零输入配置需要明确选择等待输入。", 409)
	}
	trialRange, e := trial.OutputRange.rangeValue()
	if e != nil {
		return out, nil, nil, e
	}
	probe, e := s.plannedInputsTx(ctx, tx, scope.Tenant, cfg, p, selected)
	if e != nil {
		return out, nil, nil, e
	}
	if len(inputPrefixes(p, cfg, probe, selected)) > 0 && selected.From < trialRange.From {
		return out, nil, nil, problem("trial_range_required", "请先试算新生效起点，以验证实际前驱范围。", 409)
	}
	outputs, e := mandateOutputRangesTx(ctx, tx, scope.Tenant, id)
	if e != nil {
		return out, nil, nil, e
	}
	if channel.ControlState == "retired" {
		outputs = []planning.Range{selected}
	}
	for _, r := range outputs {
		out.OutputRanges = append(out.OutputRanges, rangeDTO(r.From, r.To))
	}
	proofs := append([]monitoring.RawNeed{}, trial.InputRanges...)
	var previous []monitoring.RawNeed
	e = tx.QueryRow(ctx, `SELECT m.input_requirements FROM derived_channels c JOIN derived_execution_mandates m ON m.tenant_id=c.tenant_id AND m.id=c.active_mandate_id WHERE c.tenant_id=$1 AND c.channel_id=$2`, scope.Tenant, id).Scan(&previous)
	if e != nil {
		return out, nil, nil, e
	}
	if channel.ControlState != "retired" {
		proofs = append(proofs, previous...)
	}
	out.InputRanges, e = s.assignedRequirementsTx(ctx, tx, a, scope, next, outputs, proofs)
	if e != nil {
		return out, nil, nil, e
	}
	// Count only visible downstream resources. Hidden resource identities and
	// counts must not leak through an otherwise authorized configuration preview.
	e = tx.QueryRow(ctx, `WITH RECURSIVE downstream(id) AS(SELECT $2::uuid UNION SELECT c.channel_id FROM downstream parent JOIN derived_dependencies d ON d.tenant_id=$1 AND d.input_channel_id=parent.id JOIN derived_channels c ON c.tenant_id=d.tenant_id AND (c.draft_config_id=d.config_id OR EXISTS(SELECT 1 FROM derived_config_assignments a WHERE a.tenant_id=c.tenant_id AND a.channel_id=c.channel_id AND a.config_id=d.config_id AND a.superseded_revision IS NULL))) SELECT count(*)::text FROM downstream WHERE id<>$2 AND ($3='' OR EXISTS(SELECT 1 FROM project_resource_visibility($1,NULLIF($3,'')::uuid,NULL) v WHERE v.kind='channel' AND v.id=downstream.id AND v.full_access))`, scope.Tenant, id, scope.Project).Scan(&out.DownstreamCount)
	return out, next, p, e
}

func (s *Service) PreviewConfig(ctx context.Context, a access.Actor, scope Scope, id string, in ConfigPreviewInput) (ConfigPreview, error) {
	var out ConfigPreview
	if s.monitoring == nil {
		return out, problem("dependency_unavailable", "监测任务宿主尚未配置。", 503)
	}
	if _, e := revisionNumber(in.Revision); e != nil {
		return out, e
	}
	if !access.ValidID(id) || !access.ValidID(in.ConfigID) || !access.ValidID(in.TrialReceiptID) {
		return out, invalid("/config_id", "需要有效配置和试算收据。")
	}
	effective := strconv.FormatInt(time.Now().UnixNano(), 10)
	if in.Range.From == "" && in.Mode == "ordinary" {
		in.Range.From = effective
	}
	e := s.transact(ctx, a, scope, "channels.config-preview", true, func(tx pgx.Tx) error {
		var e error
		out, _, _, e = s.prepareConfigTx(ctx, tx, a, scope, id, in, effective)
		if e != nil {
			return e
		}
		out.Hash = configPreviewHash(out)
		out.ID = uuid.NewString()
		return tx.QueryRow(ctx, `INSERT INTO derived_config_previews(id,tenant_id,project_id,actor_id,channel_id,proposal,content_hash) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING expires_at`, out.ID, scope.Tenant, nullable(scope.Project), a.ID, id, jsonBytes(out), out.Hash).Scan(&out.ExpiresAt)
	})
	return out, e
}

func (s *Service) ApplyConfig(ctx context.Context, a access.Actor, scope Scope, id string, in ControlInput, audit access.Meta) (ChannelResult, error) {
	var out ChannelResult
	if e := s.requireReady(ctx); e != nil {
		return out, e
	}
	if e := checkMeta(in.WriteMeta, true, true); e != nil {
		return out, e
	}
	if !access.ValidID(id) || !access.ValidID(in.PreviewID) || len(in.PreviewHash) != 64 {
		return out, invalid("/preview_id", "需要完整预览凭据。")
	}
	e := s.transact(ctx, a, scope, "channels.apply-config", true, func(tx pgx.Tx) error {
		before, e := s.maintenanceTargetTx(ctx, tx, a, scope, id, true)
		if e != nil {
			return e
		}
		hash := fingerprint([]any{id, in})
		var oldHash, action string
		var old []byte
		e = tx.QueryRow(ctx, `SELECT operation,payload_hash,result FROM derived_operation_receipts WHERE tenant_id=$1 AND scope_key=$2 AND actor_id=$3 AND request_id=$4`, scope.Tenant, scope.key(), a.ID, in.RequestID).Scan(&action, &oldHash, &old)
		if e == nil {
			if action != "channels.apply-config" || hash != oldHash {
				return problem("idempotency_conflict", "请求编号已用于不同内容。", 409)
			}
			if e = json.Unmarshal(old, &out); e != nil {
				return e
			}
			out.Replayed = true
			return nil
		}
		if e != pgx.ErrNoRows {
			return e
		}
		var saved ConfigPreview
		var previewHash string
		var fresh bool
		e = tx.QueryRow(ctx, `SELECT proposal,content_hash,expires_at>clock_timestamp() FROM derived_config_previews WHERE tenant_id=$1 AND project_id IS NOT DISTINCT FROM $2::uuid AND actor_id=$3 AND channel_id=$4 AND id=$5`, scope.Tenant, nullable(scope.Project), a.ID, id, in.PreviewID).Scan(&saved, &previewHash, &fresh)
		if e == pgx.ErrNoRows {
			return notFound()
		}
		if e != nil {
			return e
		}
		if !fresh {
			return problem("preview_expired", "配置预览已过期。", 409)
		}
		if in.PreviewHash != previewHash {
			return problem("preview_changed", "配置预览依据不一致。", 409)
		}
		preview, next, p, e := s.prepareConfigTx(ctx, tx, a, scope, id, ConfigPreviewInput{ConfigProposal: saved.Proposal, Revision: in.Revision}, saved.EffectiveAtNS)
		if e != nil {
			return e
		}
		if configPreviewHash(preview) != previewHash {
			return problem("preview_changed", "图、权限或配置分配已变化，请重新预览。", 409)
		}
		revision, _ := strconv.ParseInt(before.ConfigRevision, 10, 64)
		revision++
		mandateRevision, _ := strconv.ParseInt(before.MandateRevision, 10, 64)
		mandateRevision++
		for _, v := range preview.Assignments {
			if _, e = tx.Exec(ctx, `UPDATE derived_config_assignments SET superseded_revision=$3 WHERE tenant_id=$1 AND id=$2 AND superseded_revision IS NULL`, scope.Tenant, v.ID, revision); e != nil {
				return e
			}
		}
		for _, v := range next {
			if v.ID != "" {
				continue
			}
			r, _ := v.Range.rangeValue()
			if _, e = tx.Exec(ctx, `INSERT INTO derived_config_assignments(tenant_id,channel_id,config_id,effective_range,created_revision,request_id) VALUES($1,$2,$3,$4::int8range,$5,$6)`, scope.Tenant, id, v.ConfigID, rangeSQL(r), revision, in.RequestID); e != nil {
				return e
			}
		}
		var graph string
		if e = tx.QueryRow(ctx, `UPDATE derived_graph_states SET revision=revision+1 WHERE tenant_id=$1 RETURNING revision::text`, scope.Tenant).Scan(&graph); e != nil {
			return e
		}
		outputs := []planning.Range{}
		for _, dto := range preview.OutputRanges {
			r, _ := dto.rangeValue()
			outputs = append(outputs, r)
		}
		mandate := uuid.NewString()
		basis := map[string]any{"scope": scope, "actor_id": a.ID, "preview_hash": previewHash, "trial_receipt_id": saved.Proposal.TrialReceiptID, "config_revision": strconv.FormatInt(revision, 10)}
		_, e = tx.Exec(ctx, `INSERT INTO derived_execution_mandates(id,tenant_id,channel_id,config_id,revision,output_ranges,input_requirements,content_hash,created_by,project_id,basis,reason) VALUES($1,$2,$3,$4,$5,$6::int8multirange,$7,$8,$9,$10,$11,$12)`, mandate, scope.Tenant, id, saved.Proposal.ConfigID, mandateRevision, rangeListSQL(outputs), jsonBytes(preview.InputRanges), fingerprint([]any{next, outputs, preview.InputRanges, basis}), a.ID, nullable(scope.Project), jsonBytes(basis), in.Reason)
		if e != nil {
			return e
		}
		// A bounded historical correction does not change the default future
		// configuration. Ordinary effective changes become the default draft.
		_, e = tx.Exec(ctx, `UPDATE derived_channels SET draft_config_id=CASE WHEN $6='ordinary' THEN $3::uuid ELSE draft_config_id END,config_revision=$4,control_revision=control_revision+1,mandate_revision=$5,active_mandate_id=$7,execution_scope_root=$8,health='waiting',blocked_reason='' WHERE tenant_id=$1 AND channel_id=$2`, scope.Tenant, id, saved.Proposal.ConfigID, revision, mandateRevision, saved.Proposal.Mode, mandate, previewHash)
		if e != nil {
			return e
		}
		var position string
		if e = tx.QueryRow(ctx, `UPDATE monitoring_tenant_states SET commit_sequence=commit_sequence+1 WHERE tenant_id=$1 RETURNING commit_sequence::text`, scope.Tenant).Scan(&position); e != nil {
			return e
		}
		selected, _ := saved.Proposal.Range.rangeValue()
		now := time.Now().UnixNano()
		end := planning.End(now + 1)
		if now == math.MaxInt64 {
			end = nil
		}
		for _, allowed := range outputs {
			part, ok := planning.Intersection(selected, allowed)
			if !ok {
				continue
			}
			if due, ok := planning.Intersection(part, planning.Range{From: part.From, To: end}); ok {
				if e = dirtyTx(ctx, tx, scope.Tenant, id, due, position, strconv.FormatInt(revision, 10), graph, "config."+saved.Proposal.Mode); e != nil {
					return e
				}
			}
		}
		// Existing future adoptions must not be lost when the original publication
		// event was consumed before this configuration was applied.
		if p.Rule.Clock.Kind == "primary_input" {
			cfg, e := scanConfig(tx.QueryRow(ctx, configSelect+` WHERE tenant_id=$1 AND id=$2`, scope.Tenant, saved.Proposal.ConfigID))
			if e != nil {
				return e
			}
			coverage, e := s.monitoring.CurrentRawCoverageTx(ctx, tx, scope.Tenant, cfg.InputBindings[p.Rule.Clock.Input].ChannelID)
			if e != nil {
				return e
			}
			for _, item := range coverage {
				future := planning.Range{From: max(*item.From, now), To: item.To}
				if !future.Valid() {
					continue
				}
				part, ok := planning.Intersection(selected, future)
				if !ok {
					continue
				}
				for _, allowed := range outputs {
					if part, ok := planning.Intersection(part, allowed); ok {
						if e = dirtyTx(ctx, tx, scope.Tenant, id, part, position, strconv.FormatInt(revision, 10), graph, "config.future_input"); e != nil {
							return e
						}
					}
				}
			}
		}
		_, e = tx.Exec(ctx, `INSERT INTO derived_schedules(tenant_id,channel_id,config_id,clock_kind,next_due_ns,revision) VALUES($1,$2,$3,$4,$5,1) ON CONFLICT(tenant_id,channel_id,config_id) DO UPDATE SET next_due_ns=LEAST(derived_schedules.next_due_ns,EXCLUDED.next_due_ns),revision=derived_schedules.revision+1`, scope.Tenant, id, saved.Proposal.ConfigID, p.Rule.Clock.Kind, selected.From)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE monitoring_jobs j SET state='cancelled',cancel_requested=true,fence=fence+1,lease_owner=NULL,lease_until=NULL,revision=j.revision+1,updated_at=clock_timestamp() FROM derived_runs r WHERE j.tenant_id=$1 AND r.tenant_id=j.tenant_id AND r.job_id=j.id AND r.channel_id=$2 AND r.mode='automatic' AND j.state IN ('queued','running','retry_wait','needs_action')`, scope.Tenant, id); e != nil {
			return e
		}
		pos, _ := strconv.ParseInt(position, 10, 64)
		if _, e = monitoring.AppendChangeTx(ctx, tx, scope.Tenant, pos, "config_assignment_change", "derived", id, []monitoring.ChangeRange{{Channel: id, From: &selected.From, To: selected.To}}); e != nil {
			return e
		}
		out.Channel, e = s.channelTx(ctx, tx, a, scope, id)
		if e != nil {
			return e
		}
		return configOperationTx(ctx, tx, a, scope, id, "channels.apply-config", in.WriteMeta, hash, before, out, audit)
	})
	return out, e
}
