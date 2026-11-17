package ingestion

import (
	"context"
	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	rc "github.com/WenshuaiDev/InfraPro/packages/go/foundation/relaycontract"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"slices"
)

type Recovery struct {
	ID         string         `json:"id"`
	TenantID   *string        `json:"tenant_id"`
	ActorID    string         `json:"actor_id"`
	Kind       string         `json:"kind"`
	Input      RecoveryInput  `json:"input"`
	State      string         `json:"state"`
	Revision   string         `json:"revision"`
	Generation string         `json:"generation"`
	Items      []RecoveryItem `json:"items"`
	Reason     string         `json:"reason"`
}
type RecoveryItem struct {
	ReceiptID  string `json:"receipt_id"`
	DecisionID string `json:"decision_id,omitempty"`
	State      string `json:"state"`
	ErrorCode  string `json:"error_code,omitempty"`
}

const recoveryJSON = `to_jsonb(j)||jsonb_build_object('revision',j.revision::text,'generation',j.generation::text)`

func recoveryTx(ctx context.Context, tx pgx.Tx, id string) (Recovery, error) {
	return read[Recovery](tx.QueryRow(ctx, `SELECT `+recoveryJSON+` FROM ingestion_recovery_jobs j WHERE id=$1`, id))
}
func (s *Service) authorizeRecoveryTx(ctx context.Context, tx pgx.Tx, a k.Authorization, in RecoveryInput) ([]Receipt, error) {
	if a.Entry == "project" {
		return nil, access.ErrDenied
	}
	if a.Entry == "platform" {
		if e := a.Require(access.IngestionQuarantineRead, access.IngestionQuarantineManage); e != nil {
			return nil, e
		}
		if !slices.Contains([]string{"reconcile", "confirm_epoch"}, in.Kind) || len(in.UnitIDs) > 0 || in.AssociationID != "" || in.ConfigRevisionID != "" || in.Identification != nil {
			return nil, access.ErrDenied
		}
	} else if len(in.UnitIDs) > 0 {
		if e := a.Require(access.TenantIngestionMessageRead, access.TenantIngestionReprocess, access.TenantParsingMethodRead); e != nil {
			return nil, e
		}
		if in.Kind != "reparse" && in.Kind != "reidentify" {
			return nil, k.Invalid("/kind", "enum", "请选择重新识别或解析。")
		}
	} else {
		if e := a.Require(access.TenantIngestionUnmatchedRead, access.TenantIngestionUnmatchedProcess, access.TenantIngestionTerminalRead, access.TenantDeviceRead, access.TenantParsingMethodRead); e != nil {
			return nil, e
		}
		if in.Kind != "backfill" && in.Kind != "reidentify" {
			return nil, k.Invalid("/kind", "enum", "请选择补处理或重新识别。")
		}
	}
	if in.Identification != nil {
		m, e := s.parser.IdentificationTx(ctx, tx, *in.Identification, false)
		if e != nil {
			return nil, e
		}
		if in.Kind != "reidentify" || m.Purpose != "source_identify" {
			return nil, k.Invalid("/identification", "purpose", "重新识别须明确来源识别方法。")
		}
	}
	if in.Kind == "confirm_epoch" {
		if !access.ValidID(in.RelayID) || !access.ValidID(in.Epoch) {
			return nil, k.Invalid("/epoch", "id", "请提供准确云节点和时间线。")
		}
		return []Receipt{}, nil
	}
	if len(in.ReceiptIDs)+len(in.UnitIDs) < 1 || len(in.ReceiptIDs)+len(in.UnitIDs) > 1000 || len(in.ReceiptIDs) > 0 && len(in.UnitIDs) > 0 {
		return nil, k.Invalid("/items", "limit", "请固定 1—1000 份接收记录或单元清单。")
	}
	ids := slices.Clone(in.ReceiptIDs)
	for _, id := range in.UnitIDs {
		u, _, e := s.unitAuthorizedTx(ctx, tx, a, id)
		if e != nil {
			return nil, e
		}
		ids = append(ids, u.ReceiptID)
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)
	out := []Receipt{}
	var association *Association
	if in.AssociationID != "" {
		v, e := associationTx(ctx, tx, a.Tenant, in.AssociationID)
		if e != nil {
			return nil, e
		}
		if !v.Enabled {
			return nil, c.Fail("association_disabled")
		}
		association = &v
	}
	for _, id := range ids {
		receipt, e := receiptTx(ctx, tx, id)
		if e != nil {
			return nil, e
		}
		src, e := sourceTx(ctx, tx, a.Scope, receipt.SourceID)
		if e != nil {
			return nil, e
		}
		if a.Entry == "tenant" {
			if _, e = businessTx(ctx, tx, src, ""); e != nil {
				return nil, e
			}
			if association != nil && association.SourceID != src.ID {
				return nil, k.Problem("routing_scope_mismatch", "补处理不能改变原入口范围。", 409)
			}
			if in.Kind == "backfill" && association == nil {
				return nil, k.Invalid("/association_id", "required", "历史补处理须明确目标关联。")
			}
		}
		if receipt.Envelope.EvidenceKind != "complete_message" {
			return nil, k.Problem("invalid_fragment", "不完整诊断片段不能作为测量报文重解析。", 409)
		}
		out = append(out, receipt)
	}
	return out, nil
}
func (s *Service) Recover(ctx context.Context, a access.Actor, scope Scope, in RecoveryInput, preview bool) (Result, error) {
	var out Result
	if e := meta(&in.Meta); e != nil {
		return out, e
	}
	action := "create"
	if preview {
		action = "preview"
	}
	e := s.transact(ctx, a, scope, "recovery", action, !preview, func(tx pgx.Tx, auth k.Authorization) error {
		list, e := s.authorizeRecoveryTx(ctx, tx, auth, in)
		if e != nil {
			return e
		}
		out = Result{Revision: "0", State: "ready"}
		if preview {
			return nil
		}
		if prev, ok, e := replay(ctx, tx, auth, "recovery.create", in.Meta, in); e != nil || ok {
			out = prev
			return e
		}
		out.ID = uuid.NewString()
		out.Revision = "1"
		out.State = "queued"
		items := []RecoveryItem{}
		for _, v := range list {
			items = append(items, RecoveryItem{ReceiptID: v.ID, State: "queued"})
		}
		if _, e = tx.Exec(ctx, `INSERT INTO ingestion_recovery_jobs(id,tenant_id,actor_id,kind,input,items,reason) VALUES($1,$2,$3,$4,$5,$6,$7)`, out.ID, k.Nullable(scope.Tenant), a.ID, in.Kind, k.JSON(in), k.JSON(items), in.Reason); e != nil {
			return e
		}
		ref, e := publishTaskTx(ctx, tx, out.ID, "created", a.ID)
		if e != nil {
			return e
		}
		out.TaskRef = &ref
		return complete(ctx, tx, auth, "recovery.create", in.Meta, in, out, nil, map[string]any{"state": out.State, "count": len(items)})
	})
	return out, e
}
func (s *Service) Recoveries(ctx context.Context, a access.Actor, scope Scope) ([]Recovery, error) {
	out := []Recovery{}
	e := s.transact(ctx, a, scope, "recovery", "list", false, func(tx pgx.Tx, auth k.Authorization) error {
		rows, e := tx.Query(ctx, `SELECT `+recoveryJSON+` FROM ingestion_recovery_jobs j WHERE tenant_id IS NOT DISTINCT FROM $1::uuid ORDER BY created_at DESC LIMIT 100`, k.Nullable(scope.Tenant))
		if e != nil {
			return e
		}
		list := []Recovery{}
		for rows.Next() {
			v, e := read[Recovery](rows)
			if e != nil {
				rows.Close()
				return e
			}
			list = append(list, v)
		}
		rows.Close()
		if e = rows.Err(); e != nil {
			return e
		}
		for _, v := range list {
			if _, e = s.authorizeRecoveryTx(ctx, tx, auth, v.Input); e == nil {
				out = append(out, v)
			}
		}
		return nil
	})
	return out, e
}
func recoveryScope(r Recovery) Scope {
	if r.TenantID == nil {
		return k.Platform()
	}
	return k.Tenant(*r.TenantID)
}
func (s *Service) recoveryAllowedTx(ctx context.Context, tx pgx.Tx, id, generation string) error {
	job, e := recoveryTx(ctx, tx, id)
	if e != nil {
		return e
	}
	if job.State != "running" || job.Generation != generation {
		return c.Fail("recovery_not_active")
	}
	actor := machineCommandActor(job.ActorID)
	if e = actor.Check(ctx, tx, false); e != nil {
		return e
	}
	auth, e := k.AuthorizeTx(ctx, tx, actor, recoveryScope(job), op(recoveryScope(job), "recovery", "create"))
	if e != nil {
		return e
	}
	_, e = s.authorizeRecoveryTx(ctx, tx, auth, job.Input)
	return e
}
func (s *Service) RecoveryOne(ctx context.Context) (bool, error) {
	var id string
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id::text FROM ingestion_recovery_jobs WHERE state IN ('queued','running') ORDER BY updated_at LIMIT 1`).Scan(&id)
	})
	if e == pgx.ErrNoRows {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	var job Recovery
	e = s.access.Transact(ctx, func(tx pgx.Tx) error { var e error; job, e = recoveryTx(ctx, tx, id); return e })
	if e != nil {
		return true, e
	}
	e = s.transact(ctx, machineCommandActor(job.ActorID), recoveryScope(job), "recovery", "create", true, func(tx pgx.Tx, auth k.Authorization) error {
		if _, e := tx.Exec(ctx, `SELECT id FROM ingestion_recovery_jobs WHERE id=$1 FOR UPDATE`, id); e != nil {
			return e
		}
		current, e := recoveryTx(ctx, tx, id)
		if e != nil {
			return e
		}
		if current.State != "queued" && current.State != "running" {
			return nil
		}
		list, e := s.authorizeRecoveryTx(ctx, tx, auth, current.Input)
		if e != nil {
			return e
		}
		if current.State == "queued" {
			items := []RecoveryItem{}
			if current.Kind == "confirm_epoch" {
				tag, e := tx.Exec(ctx, `UPDATE ingestion_epochs SET confirmed=true WHERE relay_id=$1 AND epoch=$2`, current.Input.RelayID, current.Input.Epoch)
				if e != nil {
					return e
				}
				if tag.RowsAffected() != 1 {
					return k.NotFound("epoch")
				}
			}
			for _, receipt := range list {
				item := RecoveryItem{ReceiptID: receipt.ID, State: "queued"}
				rev, e := revisionTx(ctx, tx, receipt.SourceID, receipt.SourceRevision)
				if e != nil {
					item.State = "failed"
					item.ErrorCode = "source_revision_unknown"
					items = append(items, item)
					continue
				}
				if e = rc.Match(receipt.Envelope, rev.Snapshot); e != nil {
					item.State = "failed"
					item.ErrorCode = code(e)
					items = append(items, item)
					continue
				}
				if current.Kind == "reconcile" {
					var epochConfirmed bool
					if e = tx.QueryRow(ctx, `SELECT confirmed FROM ingestion_epochs WHERE relay_id=$1 AND epoch=$2`, receipt.RelayID, receipt.Epoch).Scan(&epochConfirmed); e != nil {
						return e
					}
					if !rev.Confirmed || !epochConfirmed {
						item.State = "failed"
						item.ErrorCode = "awaiting_source_confirmation"
					} else {
						_, e = tx.Exec(ctx, `UPDATE ingestion_receipts SET tenant_id=$2,project_id=$3,state='stored',error_code='' WHERE id=$1`, receipt.ID, rev.Snapshot.TenantID, rev.Snapshot.ProjectID)
						if e != nil {
							return e
						}
						item.State = "succeeded"
					}
					items = append(items, item)
					continue
				}
				protocol, e := s.parser.IngressProtocolTx(ctx, tx, rev.Snapshot.ProtocolID, rev.Snapshot.ProtocolVersion, false)
				if e != nil {
					item.State = "failed"
					item.ErrorCode = code(e)
					items = append(items, item)
					continue
				}
				target := protocol.Identification
				if current.Input.Identification != nil {
					target = *current.Input.Identification
				}
				method, e := s.parser.IdentificationTx(ctx, tx, target, false)
				if e != nil {
					return e
				}
				grant, e := grantTx(ctx, tx, receipt.SourceID, "")
				if e != nil {
					return e
				}
				if !grant.Enabled || !grant.Identify {
					return c.Fail("authorization_revoked")
				}
				var generation int64
				if e = tx.QueryRow(ctx, `UPDATE ingestion_receipts SET current_generation=current_generation+1,state='stored',error_code='' WHERE id=$1 RETURNING current_generation`, receipt.ID).Scan(&generation); e != nil {
					return e
				}
				item.DecisionID = uuid.NewString()
				if _, e = tx.Exec(ctx, `INSERT INTO ingestion_identity_decisions(id,receipt_id,generation,method,grant_id,grant_generation,recovery_id,recovery_generation) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, item.DecisionID, receipt.ID, generation, k.JSON(method), grant.ID, grant.Generation, id, current.Generation); e != nil {
					return e
				}
				items = append(items, item)
			}
			state := "running"
			if current.Kind == "reconcile" || current.Kind == "confirm_epoch" {
				state = "succeeded"
				for _, item := range items {
					if item.State == "failed" {
						state = "failed"
					}
				}
			}
			if _, e = tx.Exec(ctx, `UPDATE ingestion_recovery_jobs SET state=$2,items=$3,revision=revision+1,updated_at=clock_timestamp() WHERE id=$1`, id, state, k.JSON(items)); e != nil {
				return e
			}
			_, e = publishTaskTx(ctx, tx, id, "started", "")
			return e
		}
		items := current.Items
		active, failed := false, false
		for n, item := range items {
			if item.DecisionID == "" {
				failed = failed || item.State == "failed"
				continue
			}
			d, e := decisionTx(ctx, tx, item.DecisionID)
			if e != nil {
				return e
			}
			var pending, bad int
			if e = tx.QueryRow(ctx, `SELECT count(*) FILTER(WHERE state IN ('identified','demultiplexing','demultiplex_retry','planned','retry_wait','parsing')),count(*) FILTER(WHERE state NOT IN ('identified','demultiplexing','demultiplex_retry','planned','retry_wait','parsing','parsed','control','envelope','not_selected')) FROM ingestion_units WHERE decision_id=$1`, d.ID).Scan(&pending, &bad); e != nil {
				return e
			}
			if d.State == "queued" || d.State == "identifying" || d.State == "retry_wait" || pending > 0 {
				active = true
				items[n].State = "running"
			} else if d.State != "identified" || bad > 0 {
				failed = true
				items[n].State = "failed"
				items[n].ErrorCode = d.ErrorCode
				if bad > 0 {
					items[n].ErrorCode = "unit_processing_failed"
				}
			} else {
				items[n].State = "succeeded"
			}
		}
		state := "succeeded"
		if active {
			state = "running"
		} else if failed {
			state = "failed"
		}
		if _, e = tx.Exec(ctx, `UPDATE ingestion_recovery_jobs SET state=$2,items=$3,revision=revision+1,updated_at=clock_timestamp() WHERE id=$1`, id, state, k.JSON(items)); e != nil {
			return e
		}
		_, e = publishTaskTx(ctx, tx, id, "progress", "")
		return e
	})
	if e != nil {
		_ = s.access.Transact(ctx, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `UPDATE ingestion_recovery_jobs SET state='failed',revision=revision+1,updated_at=clock_timestamp() WHERE id=$1 AND state IN ('queued','running')`, id); err != nil {
				return err
			}
			_, err := publishTaskTx(ctx, tx, id, "failed", "")
			return err
		})
	}
	return true, e
}
func recoveryConfig(ctx context.Context, tx pgx.Tx, d Decision) (RecoveryInput, error) {
	if d.RecoveryID == nil {
		return RecoveryInput{}, nil
	}
	j, e := recoveryTx(ctx, tx, *d.RecoveryID)
	return j.Input, e
}

func (s *Service) RecoveryGeneration(ctx context.Context, a access.Actor, scope Scope, id, generation string) (Recovery, error) {
	var out Recovery
	e := s.transact(ctx, a, scope, "recovery", "detail", false, func(tx pgx.Tx, auth k.Authorization) error {
		job, e := recoveryTx(ctx, tx, id)
		if e != nil {
			return e
		}
		if recoveryScope(job) != scope {
			return k.NotFound("recovery")
		}
		if _, e = s.authorizeRecoveryTx(ctx, tx, auth, job.Input); e != nil {
			return e
		}
		out, e = read[Recovery](tx.QueryRow(ctx, `SELECT snapshot FROM ingestion_recovery_generations WHERE job_id=$1 AND generation=$2`, id, generation))
		return e
	})
	return out, e
}
