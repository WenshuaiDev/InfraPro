package derivedmonitoring

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/compute"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/planning"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func controlHash(p ControlPreview) string {
	p.ID = ""
	p.Hash = ""
	p.ExpiresAt = time.Time{}
	return fingerprint(p)
}
func rangeListSQL(ranges []planning.Range) string {
	parts := make([]string, len(ranges))
	for i, r := range ranges {
		parts[i] = rangeSQL(r)
	}
	return "{" + strings.Join(parts, ",") + "}"
}
func controlRange(p ControlProposal) (planning.Range, error) {
	if p.AutoStartNS == nil {
		return planning.Range{}, invalid("/auto_start_ns", "需要明确计算起点。")
	}
	return (NanosecondRange{*p.AutoStartNS, p.StopNS}).rangeValue()
}
func normalizedNeeds(needs []monitoring.RawNeed) ([]monitoring.RawNeed, error) {
	groups := map[string][]planning.Range{}
	for _, n := range needs {
		if n.From == nil {
			return nil, invalid("/input_ranges", "输入范围必须有起点。")
		}
		groups[n.Channel] = append(groups[n.Channel], planning.Range{From: *n.From, To: n.To})
	}
	ids := []string{}
	for id := range groups {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	out := []monitoring.RawNeed{}
	for _, id := range ids {
		ranges, e := planning.Union(groups[id])
		if e != nil {
			return nil, e
		}
		for _, r := range ranges {
			from := r.From
			out = append(out, monitoring.RawNeed{Channel: id, From: &from, To: r.To})
		}
	}
	return out, nil
}
func mandateOutputRangesTx(ctx context.Context, tx pgx.Tx, tenant, channel string) ([]planning.Range, error) {
	var stored pgtype.Multirange[pgtype.Range[int64]]
	e := tx.QueryRow(ctx, `SELECT m.output_ranges FROM derived_channels c JOIN derived_execution_mandates m ON m.tenant_id=c.tenant_id AND m.id=c.active_mandate_id WHERE c.tenant_id=$1 AND c.channel_id=$2`, tenant, channel).Scan(&stored)
	if e == pgx.ErrNoRows {
		return []planning.Range{}, nil
	}
	if e != nil {
		return nil, e
	}
	out := []planning.Range{}
	for _, r := range stored {
		v := planning.Range{From: r.Lower}
		if r.UpperType != pgtype.Unbounded {
			v.To = planning.End(r.Upper)
		}
		out = append(out, v)
	}
	return out, nil
}
func (s *Service) controlTargetTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, id string) (Channel, error) {
	return s.maintenanceTargetTx(ctx, tx, a, scope, id, false)
}
func (s *Service) maintenanceTargetTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, id string, allowRetired bool) (Channel, error) {
	targets, e := s.resources.CalculationTargetsTx(ctx, tx, a, resourceScope(scope), []resources.Target{{Kind: resources.Channel, ID: id}}, false)
	if e != nil {
		return Channel{}, e
	}
	target := targets["channel:"+id]
	if !target.CanManage {
		return Channel{}, access.ErrDenied
	}
	if target.Retired && !allowRetired {
		return Channel{}, problem("dependency_unavailable", "工程资源或其祖先已经退役。", 409)
	}
	return s.channelTx(ctx, tx, a, scope, id)
}
func (s *Service) prepareControlTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, id string, in ControlPreviewInput, effective string) (ControlPreview, *compute.Plan, error) {
	out := ControlPreview{Proposal: in.ControlProposal, ChannelID: id, EffectiveAtNS: effective, OutputRanges: []NanosecondRange{}, InputRanges: []monitoring.RawNeed{}}
	if !slices.Contains([]string{"enable", "pause", "resume"}, in.Action) {
		return out, nil, invalid("/action", "支持 enable、pause 和 resume。")
	}
	channel, e := s.controlTargetTx(ctx, tx, a, scope, id)
	if e != nil {
		return out, nil, e
	}
	if channel.ControlRevision != in.Revision {
		return out, nil, problem("revision_changed", "计算控制修订已变化。", 409)
	}
	if in.Action == "enable" && channel.ControlState != "draft" || in.Action == "pause" && channel.ControlState != "enabled" || in.Action == "resume" && channel.ControlState != "paused" {
		return out, nil, problem("invalid_control_transition", "当前计算状态不支持此操作。", 409)
	}
	out.ControlRevision = channel.ControlRevision
	out.ConfigRevision = channel.ConfigRevision
	out.ConfigID = channel.DraftConfigID
	if e = tx.QueryRow(ctx, `SELECT revision::text FROM derived_graph_states WHERE tenant_id=$1`, scope.Tenant).Scan(&out.GraphRevision); e != nil {
		return out, nil, e
	}
	if in.Action == "pause" {
		if in.AutoStartNS != nil || in.StopNS != nil || in.ResumeMode != "" || in.TrialReceiptID != "" || in.AllowWaiting {
			return out, nil, invalid("/action", "暂停不接受计算范围或试算选项。")
		}
		return out, nil, nil
	}
	if s.monitoring == nil {
		return out, nil, problem("dependency_unavailable", "监测任务宿主尚未配置。", 503)
	}
	_, cfg, p, e := s.calculationConfigTx(ctx, tx, a, scope, id, channel.DraftConfigID)
	if e != nil {
		return out, nil, e
	}
	out.ConfigHash = cfg.ConfigHash
	if in.Action == "enable" && in.ResumeMode != "" {
		return out, nil, invalid("/resume_mode", "首次启用不能指定恢复模式。")
	}
	if in.Action == "resume" && out.Proposal.ResumeMode == "" {
		out.Proposal.ResumeMode = "catch_up"
	}
	if in.Action == "resume" && !slices.Contains([]string{"catch_up", "new_start"}, out.Proposal.ResumeMode) {
		return out, nil, invalid("/resume_mode", "恢复方式必须是 catch_up 或 new_start。")
	}
	var paused *string
	if e = tx.QueryRow(ctx, `SELECT paused_at_ns::text FROM derived_channels WHERE tenant_id=$1 AND channel_id=$2`, scope.Tenant, id).Scan(&paused); e != nil {
		return out, nil, e
	}
	if out.Proposal.AutoStartNS == nil {
		start := effective
		if in.Action == "resume" && out.Proposal.ResumeMode == "catch_up" {
			if paused != nil {
				start = *paused
			} else if channel.AutoStartNS != nil {
				start = *channel.AutoStartNS
			}
		}
		out.Proposal.AutoStartNS = &start
	}
	selected, e := controlRange(out.Proposal)
	if e != nil {
		return out, nil, e
	}
	previous, e := mandateOutputRangesTx(ctx, tx, scope.Tenant, id)
	if e != nil {
		return out, nil, e
	}
	if in.Action == "resume" && out.Proposal.ResumeMode == "catch_up" && paused != nil && out.Proposal.AutoStartNS != nil && *out.Proposal.AutoStartNS != *paused {
		return out, nil, invalid("/auto_start_ns", "默认补算必须从暂停边界开始；另选起点请使用 new_start。")
	}
	allowed := []planning.Range{selected}
	if in.Action == "resume" {
		boundary := selected.From
		if paused != nil {
			boundary, _ = strconv.ParseInt(*paused, 10, 64)
		}
		if out.Proposal.ResumeMode == "new_start" && selected.From < boundary {
			return out, nil, invalid("/auto_start_ns", "新起点不能早于暂停边界。")
		}
		for _, r := range previous {
			if r.From < boundary {
				if r.To == nil || *r.To > boundary {
					r.To = planning.End(boundary)
				}
				allowed = append(allowed, r)
			}
		}
	}
	allowed, e = planning.Union(allowed)
	if e != nil {
		return out, nil, e
	}
	for _, r := range allowed {
		out.OutputRanges = append(out.OutputRanges, rangeDTO(r.From, r.To))
	}
	if in.Action == "resume" {
		assignments, e := activeAssignmentsTx(ctx, tx, scope.Tenant, id)
		if e != nil {
			return out, nil, e
		}
		out.InputRanges, e = s.assignedRequirementsTx(ctx, tx, a, scope, assignments, allowed, nil)
		if e != nil {
			return out, nil, e
		}
	} else {
		for _, r := range allowed {
			needs, e := s.plannedInputsTx(ctx, tx, scope.Tenant, cfg, p, r)
			if e != nil {
				return out, nil, e
			}
			out.InputRanges = append(out.InputRanges, needs...)
		}
		baselineNeeds, e := s.baselineNeedsTx(ctx, tx, scope.Tenant, cfg, p)
		if e != nil {
			return out, nil, e
		}
		out.InputRanges = append(out.InputRanges, baselineNeeds...)
		out.InputRanges, e = normalizedNeeds(out.InputRanges)
		if e != nil {
			return out, nil, e
		}
		if e = s.authorizeInputsTx(ctx, tx, a, monitoringScope(scope), out.InputRanges, false); e != nil {
			return out, nil, e
		}
	}
	inputIDs := []string{}
	inputTargets := []resources.Target{}
	for _, binding := range cfg.InputBindings {
		inputIDs = append(inputIDs, binding.ChannelID)
		inputTargets = append(inputTargets, resources.Target{Kind: resources.Channel, ID: binding.ChannelID})
	}
	activeInputs, e := s.resources.CalculationTargetsTx(ctx, tx, a, resourceScope(scope), inputTargets, false)
	if e != nil {
		return out, nil, e
	}
	for _, target := range activeInputs {
		if target.Retired {
			return out, nil, problem("dependency_unavailable", "启用与恢复要求可用的输入资源。", 409)
		}
	}
	if e = s.graphTx(ctx, tx, scope.Tenant, id, inputIDs); e != nil {
		return out, nil, e
	}
	if !access.ValidID(in.TrialReceiptID) {
		return out, nil, problem("trial_required", "启用或恢复需要当前配置的有效试算收据。", 409)
	}
	var trial TrialResult
	e = tx.QueryRow(ctx, `SELECT result FROM derived_trial_receipts WHERE tenant_id=$1 AND project_id IS NOT DISTINCT FROM $2::uuid AND actor_id=$3 AND id=$4 AND channel_id=$5 AND config_id=$6 AND config_hash=$7 AND expires_at>clock_timestamp()`, scope.Tenant, nullable(scope.Project), a.ID, in.TrialReceiptID, id, cfg.ID, cfg.ConfigHash).Scan(&trial)
	if e == pgx.ErrNoRows {
		return out, nil, problem("trial_required", "试算收据不存在、已过期或不属于当前配置与空间。", 409)
	}
	if e != nil {
		return out, nil, e
	}
	state, e := s.checkUserInputsTx(ctx, tx, a, monitoringScope(scope), trial.InputSnapshot.ID, false)
	if e != nil {
		return out, nil, e
	}
	if state.Changed || trial.Stale {
		return out, nil, problem("trial_stale", "试算输入已变化，需要重新试算。", 409)
	}
	_, baselineProofs, e := s.baselinesTx(ctx, tx, a, monitoringScope(scope), nil, cfg.ID)
	if e != nil {
		return out, nil, e
	}
	if fingerprint(baselineProofs) != fingerprint(trial.Baselines) {
		return out, nil, problem("trial_stale", "基准依据已变化，需要重新试算。", 409)
	}
	if prefixes := inputPrefixes(p, cfg, out.InputRanges, selected); len(prefixes) > 0 {
		trialRange, e := trial.OutputRange.rangeValue()
		if e != nil {
			return out, nil, e
		}
		if selected.From < trialRange.From {
			return out, nil, problem("trial_range_required", "请先试算自动起点，确认实际前驱与样本窗口的完整输入授权范围。", 409)
		}
		proofs := append([]monitoring.RawNeed{}, trial.InputRanges...)
		var previousInputs []monitoring.RawNeed
		e = tx.QueryRow(ctx, `SELECT m.input_requirements FROM derived_channels c JOIN derived_execution_mandates m ON m.tenant_id=c.tenant_id AND m.id=c.active_mandate_id WHERE c.tenant_id=$1 AND c.channel_id=$2`, scope.Tenant, id).Scan(&previousInputs)
		if e != nil && e != pgx.ErrNoRows {
			return out, nil, e
		}
		proofs = append(proofs, previousInputs...)
		for i, n := range out.InputRanges {
			for _, proof := range proofs {
				if proof.Channel == n.Channel && (proof.From == nil || n.From != nil && *proof.From < *n.From) {
					out.InputRanges[i].From = proof.From
					n = out.InputRanges[i]
				}
			}
		}
		out.InputRanges, e = normalizedNeeds(out.InputRanges)
		if e != nil {
			return out, nil, e
		}
		if e = s.authorizeInputsTx(ctx, tx, a, monitoringScope(scope), out.InputRanges, false); e != nil {
			return out, nil, e
		}
		prefixChannels := []string{}
		for _, prefix := range prefixes {
			prefixChannels = append(prefixChannels, prefix.Channel)
		}
		slices.Sort(prefixChannels)
		prefixChannels = slices.Compact(prefixChannels)
		out.InputRanges, e = s.monitoring.CalculationPrefixAuthorityTx(ctx, tx, a, monitoringScope(scope), out.InputRanges, prefixChannels)
		if e != nil {
			return out, nil, e
		}
		out.InputRanges, e = normalizedNeeds(out.InputRanges)
		if e != nil {
			return out, nil, e
		}
	}
	out.WaitingForInput = trial.WaitingForInput
	if out.WaitingForInput && !in.AllowWaiting {
		return out, nil, problem("waiting_confirmation_required", "当前试算没有输入；需要明确选择等待数据后启用。", 409)
	}
	out.InputRanges, e = s.expandInputMandateTx(ctx, tx, a, monitoringScope(scope), out.InputRanges)
	if e != nil {
		return out, nil, e
	}

	return out, p, nil
}
func (s *Service) PreviewControl(ctx context.Context, a access.Actor, scope Scope, id string, in ControlPreviewInput) (ControlPreview, error) {
	var out ControlPreview
	if in.Action == "enable" || in.Action == "resume" {
		if e := s.requireReady(ctx); e != nil {
			return out, e
		}
	}
	if _, e := revisionNumber(in.Revision); e != nil {
		return out, e
	}
	if !access.ValidID(id) {
		return out, notFound()
	}
	e := s.transact(ctx, a, scope, "channels.control-preview", true, func(tx pgx.Tx) error {
		var e error
		out, _, e = s.prepareControlTx(ctx, tx, a, scope, id, in, strconv.FormatInt(time.Now().UnixNano(), 10))
		if e != nil {
			return e
		}
		out.Hash = controlHash(out)
		return tx.QueryRow(ctx, `INSERT INTO derived_control_previews(tenant_id,project_id,actor_id,channel_id,proposal,content_hash) VALUES($1,$2,$3,$4,$5,$6) RETURNING id::text,expires_at`, scope.Tenant, nullable(scope.Project), a.ID, id, jsonBytes(out), out.Hash).Scan(&out.ID, &out.ExpiresAt)
	})
	return out, e
}
func (s *Service) ControlChannel(ctx context.Context, a access.Actor, scope Scope, id string, in ControlInput, audit access.Meta) (ChannelResult, error) {
	var out ChannelResult
	if e := checkMeta(in.WriteMeta, true, true); e != nil {
		return out, e
	}
	if !access.ValidID(id) || !access.ValidID(in.PreviewID) || len(in.PreviewHash) != 64 {
		return out, invalid("/preview_id", "需要有效操作预览。")
	}
	if s.readiness != nil {
		var action string
		e := s.transact(ctx, a, scope, "channels.control", false, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT proposal->'proposal'->>'action' FROM derived_control_previews WHERE tenant_id=$1 AND project_id IS NOT DISTINCT FROM $2::uuid AND actor_id=$3 AND channel_id=$4 AND id=$5`, scope.Tenant, nullable(scope.Project), a.ID, id, in.PreviewID).Scan(&action)
		})
		if e != nil {
			return out, e
		}
		if action != "pause" {
			if e = s.requireReady(ctx); e != nil {
				return out, e
			}
		}
	}
	e := s.transact(ctx, a, scope, "channels.control", true, func(tx pgx.Tx) error {
		before, e := s.controlTargetTx(ctx, tx, a, scope, id)
		if e != nil {
			return e
		}
		var saved ControlPreview
		var hash string
		var fresh bool
		e = tx.QueryRow(ctx, `SELECT proposal,content_hash,expires_at>clock_timestamp() FROM derived_control_previews WHERE id=$1 AND tenant_id=$2 AND project_id IS NOT DISTINCT FROM $3::uuid AND actor_id=$4 AND channel_id=$5`, in.PreviewID, scope.Tenant, nullable(scope.Project), a.ID, id).Scan(&saved, &hash, &fresh)
		if e == pgx.ErrNoRows {
			return notFound()
		}
		if e != nil {
			return e
		}
		if hash != in.PreviewHash {
			return problem("preview_changed", "预览内容不匹配。", 409)
		}
		requestHash := fingerprint([]any{id, in})
		var oldHash, oldAction string
		e = tx.QueryRow(ctx, `SELECT operation,payload_hash,result FROM derived_operation_receipts WHERE tenant_id=$1 AND scope_key=$2 AND actor_id=$3 AND request_id=$4`, scope.Tenant, scope.key(), a.ID, in.RequestID).Scan(&oldAction, &oldHash, &out)
		if e == nil {
			if oldAction != "channels.control" || oldHash != requestHash {
				return problem("request_id_reused", "请求编号已用于不同操作。", 409)
			}
			if saved.Proposal.Action != "pause" {
				if e = s.authorizeInputsTx(ctx, tx, a, monitoringScope(scope), saved.InputRanges, false); e != nil {
					return e
				}
			}
			out.Replayed = true
			return nil
		}
		if e != pgx.ErrNoRows {
			return e
		}
		if !fresh {
			return problem("preview_expired", "操作预览已过期。", 409)
		}
		preview, p, e := s.prepareControlTx(ctx, tx, a, scope, id, ControlPreviewInput{saved.Proposal, in.Revision}, saved.EffectiveAtNS)
		if e != nil {
			return e
		}
		if controlHash(preview) != hash {
			return problem("preview_changed", "配置、依赖或控制依据已经变化，请重新预览。", 409)
		}
		effective, _ := strconv.ParseInt(saved.EffectiveAtNS, 10, 64)
		var position int64
		if e = tx.QueryRow(ctx, `INSERT INTO monitoring_tenant_states(tenant_id,commit_sequence) VALUES($1,1) ON CONFLICT(tenant_id) DO UPDATE SET commit_sequence=monitoring_tenant_states.commit_sequence+1 RETURNING commit_sequence`, scope.Tenant).Scan(&position); e != nil {
			return e
		}
		if saved.Proposal.Action == "pause" {
			_, e = tx.Exec(ctx, `UPDATE derived_channels SET control_state='paused',control_revision=control_revision+1,mandate_revision=mandate_revision+1,paused_at_ns=$3,health='waiting',blocked_reason='' WHERE tenant_id=$1 AND channel_id=$2`, scope.Tenant, id, effective)
		} else {
			ranges := []planning.Range{}
			for _, dto := range preview.OutputRanges {
				r, err := dto.rangeValue()
				if err != nil {
					return err
				}
				ranges = append(ranges, r)
			}
			next, _ := revisionNumber(before.MandateRevision)
			next++
			mandate := uuid.NewString()
			basis := map[string]any{"scope": scope, "actor_id": a.ID, "trial_receipt_id": saved.Proposal.TrialReceiptID, "preview_hash": hash}
			_, e = tx.Exec(ctx, `INSERT INTO derived_execution_mandates(id,tenant_id,channel_id,config_id,revision,output_ranges,input_requirements,content_hash,created_by,project_id,basis,reason) VALUES($1,$2,$3,$4,$5,$6::int8multirange,$7,$8,$9,$10,$11,$12)`, mandate, scope.Tenant, id, preview.ConfigID, next, rangeListSQL(ranges), jsonBytes(preview.InputRanges), fingerprint([]any{preview.ConfigID, ranges, preview.InputRanges, basis}), a.ID, nullable(scope.Project), jsonBytes(basis), in.Reason)
			if e != nil {
				return e
			}
			selected, _ := controlRange(preview.Proposal)
			_, e = tx.Exec(ctx, `UPDATE derived_channels SET control_state='enabled',control_revision=control_revision+1,mandate_revision=$3,active_mandate_id=$4,execution_scope_root=$5,auto_start_ns=COALESCE(auto_start_ns,$6),stop_ns=$7,paused_at_ns=NULL,health='waiting',blocked_reason='' WHERE tenant_id=$1 AND channel_id=$2`, scope.Tenant, id, next, mandate, hash, selected.From, selected.To)
			if e != nil {
				return e
			}
			if before.ControlState == "draft" {
				_, e = tx.Exec(ctx, `INSERT INTO derived_config_assignments(tenant_id,channel_id,config_id,effective_range,created_revision,request_id) VALUES($1,$2,$3,$4::int8range,$5,$6)`, scope.Tenant, id, preview.ConfigID, rangeSQL(planning.Range{From: selected.From}), before.ConfigRevision, in.RequestID)
				if e != nil {
					return e
				}
			}
			until := time.Now().UnixNano()
			if until < 9223372036854775807 {
				until++
			}
			if part, ok := planning.Intersection(selected, planning.Range{From: selected.From, To: &until}); ok {
				_, e = tx.Exec(ctx, `INSERT INTO derived_dirty_ranges(tenant_id,channel_id,affected,required_position,config_revision,graph_revision,reason) VALUES($1,$2,$3::int8range,$4,$5,$6,$7)`, scope.Tenant, id, rangeSQL(part), position, before.ConfigRevision, preview.GraphRevision, "control."+saved.Proposal.Action)
				if e != nil {
					return e
				}
			}
			if p.Rule.Clock.Kind == "primary_input" {
				cfg, e := scanConfig(tx.QueryRow(ctx, configSelect+` WHERE tenant_id=$1 AND id=$2`, scope.Tenant, preview.ConfigID))
				if e != nil {
					return e
				}
				coverage, e := s.monitoring.CurrentRawCoverageTx(ctx, tx, scope.Tenant, cfg.InputBindings[p.Rule.Clock.Input].ChannelID)
				if e != nil {
					return e
				}
				for _, item := range coverage {
					future := planning.Range{From: max(*item.From, until), To: item.To}
					if !future.Valid() {
						continue
					}
					if part, ok := planning.Intersection(selected, future); ok {
						if e = dirtyTx(ctx, tx, scope.Tenant, id, part, strconv.FormatInt(position, 10), before.ConfigRevision, preview.GraphRevision, "future_input_at_activation"); e != nil {
							return e
						}
					}
				}
			}
			_, e = tx.Exec(ctx, `INSERT INTO derived_schedules(tenant_id,channel_id,config_id,clock_kind,next_due_ns,revision) VALUES($1,$2,$3,$4,$5,1) ON CONFLICT(tenant_id,channel_id,config_id) DO UPDATE SET next_due_ns=EXCLUDED.next_due_ns,revision=derived_schedules.revision+1`, scope.Tenant, id, preview.ConfigID, p.Rule.Clock.Kind, selected.From)
		}
		if e != nil {
			return e
		}
		// Any prior automatic lease is fenced, including paused jobs awaiting retry.
		_, e = tx.Exec(ctx, `UPDATE monitoring_jobs j SET state='cancelled',cancel_requested=true,fence=fence+1,lease_owner=NULL,lease_until=NULL,revision=j.revision+1,updated_at=clock_timestamp() FROM derived_runs r WHERE j.tenant_id=$1 AND r.tenant_id=j.tenant_id AND r.job_id=j.id AND r.channel_id=$2 AND r.mode='automatic' AND j.state IN ('queued','running','retry_wait','needs_action')`, scope.Tenant, id)
		if e != nil {
			return e
		}
		if _, e = monitoring.AppendChangeTx(ctx, tx, scope.Tenant, position, "channel_control_change", "derived", id, []monitoring.ChangeRange{{Channel: id, From: &effective}}); e != nil {
			return e
		}
		out.Channel, e = s.channelTx(ctx, tx, a, scope, id)
		if e != nil {
			return e
		}
		audit.ActorID = a.ID
		audit.ActorType = "account"
		audit.ActorScope = scope.Entry
		audit.TenantID = scope.Tenant
		audit.ProjectID = scope.Project
		audit.Reason = in.Reason
		if e = access.Audit(ctx, tx, "derived.channels.control", "derived_channel", id, "", before, out, audit); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO derived_changes(tenant_id,project_id,actor_kind,actor_id,operation,target_kind,target_id,request_id,reason,before,after) VALUES($1,$2,'user',$3,$4,'channel',$5,$6,$7,$8,$9)`, scope.Tenant, nullable(scope.Project), a.ID, "channels."+saved.Proposal.Action, id, in.RequestID, in.Reason, jsonBytes(before), jsonBytes(out)); e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO derived_operation_receipts(tenant_id,project_id,actor_id,request_id,operation,payload_hash,result) VALUES($1,$2,$3,$4,'channels.control',$5,$6)`, scope.Tenant, nullable(scope.Project), a.ID, in.RequestID, requestHash, jsonBytes(out))
		return e
	})
	return out, e
}
