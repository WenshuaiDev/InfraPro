package audit

import (
	"context"
	ar "github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	"github.com/jackc/pgx/v5"
	"time"
)

// ApplyObjects only deletes registered objects from a reviewed bounded plan.
// A failed object-store deletion remains delete_pending and is never readable.
func (s *Service) ApplyObjects(ctx context.Context, id, revision, request, reason string) (map[string]any, error) {
	out := map[string]any{}
	if e := maintenanceInput(request, reason); e != nil {
		return out, e
	}
	if !ar.ID(id) || !ar.Number(revision, 1) {
		return out, Problem("audit_filter_invalid", 400)
	}
	if s.objects == nil {
		return out, Problem("audit_storage_unavailable", 503)
	}
	ids := []string{}
	replay := false
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		e := tx.QueryRow(ctx, `SELECT summary FROM audit_maintenance_receipts WHERE request_id=$1 AND plan_id=$2 AND operation='cleanup.objects'`, request, id).Scan(&out)
		if e == nil {
			replay = true
			return nil
		}
		if e != pgx.ErrNoRows {
			return e
		}
		p, e := loadPlanTx(ctx, tx, id)
		if e != nil {
			return e
		}
		var valid bool
		if e = tx.QueryRow(ctx, `SELECT mode='ready' AND restore_epoch=$1 FROM audit_protocol WHERE id=1`, p.Epoch).Scan(&valid); e != nil {
			return e
		}
		if !valid || p.Kind != "objects" || p.Revision != revision || p.State != "planned" && p.State != "partial" {
			return Problem("audit_plan_changed", 409)
		}
		tag, e := tx.Exec(ctx, `UPDATE audit_cleanup_plans SET apply_owner=$2,apply_until=clock_timestamp()+interval '30 seconds' WHERE id=$1 AND (apply_until IS NULL OR apply_until<=clock_timestamp())`, id, request)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return Problem("audit_cleanup_in_progress", 409)
		}
		rows, e := tx.Query(ctx, `SELECT object_id FROM audit_cleanup_items WHERE plan_id=$1 AND state<>'deleted' ORDER BY object_id`, id)
		if e != nil {
			return e
		}
		ids, e = pgx.CollectRows(rows, pgx.RowTo[string])
		if len(ids) > s.options.ObjectBatch {
			return Problem("audit_plan_changed", 409)
		}
		return e
	})
	if e != nil || replay {
		return out, e
	}
	deleted, protected, failed := 0, 0, 0
	for _, objectID := range ids {
		key := ""
		e = s.access.Transact(ctx, func(tx pgx.Tx) error {
			if _, e := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(834327122)`); e != nil {
				return e
			}
			p, e := loadPlanTx(ctx, tx, id)
			if e != nil {
				return e
			}
			if p.Revision != revision || p.State != "planned" && p.State != "partial" {
				return Problem("audit_plan_changed", 409)
			}
			tag, e := tx.Exec(ctx, `UPDATE audit_cleanup_plans SET apply_until=clock_timestamp()+interval '30 seconds' WHERE id=$1 AND apply_owner=$2 AND apply_until>clock_timestamp()`, id, request)
			if e != nil {
				return e
			}
			if tag.RowsAffected() != 1 {
				return Problem("audit_plan_changed", 409)
			}
			var state, run string
			var expires *time.Time
			var created time.Time
			e = tx.QueryRow(ctx, `SELECT o.object_key,o.state,o.expires_at,o.created_at,g.state FROM audit_export_objects o JOIN audit_export_generations g ON g.export_id=o.export_id AND g.generation=o.generation WHERE o.id=$1 FOR UPDATE OF o`, objectID).Scan(&key, &state, &expires, &created, &run)
			if e != nil {
				return e
			}
			var held, reading, ready bool
			e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM audit_retention_holds WHERE released_at IS NULL AND expires_at>clock_timestamp() AND (object_kind IN ('all','backup') OR object_kind='export_object' AND object_id=$1)),EXISTS(SELECT 1 FROM audit_export_read_leases WHERE object_id=$1::uuid AND closed_at IS NULL AND expires_at>clock_timestamp()),EXISTS(SELECT 1 FROM audit_protocol WHERE id=1 AND mode='ready' AND restore_epoch=$2)`, objectID, p.Epoch).Scan(&held, &reading, &ready)
			if e != nil {
				return e
			}
			expired := expires != nil && !expires.After(s.now()) && !expires.After(p.Cutoff)
			orphan := state == "intent" && (run == "succeeded" || run == "failed" || run == "cancelled") && created.Before(p.Cutoff.Add(-20*time.Minute))
			if held || reading || !ready || state != "delete_pending" && !expired && !orphan {
				key = ""
				protected++
				_, e = tx.Exec(ctx, `UPDATE audit_cleanup_items SET state='protected' WHERE plan_id=$1 AND object_id=$2`, id, objectID)
				return e
			}
			if state == "deleted" {
				key = ""
				deleted++
				_, e = tx.Exec(ctx, `UPDATE audit_cleanup_items SET state='deleted' WHERE plan_id=$1 AND object_id=$2`, id, objectID)
				return e
			}
			_, e = tx.Exec(ctx, `UPDATE audit_export_objects SET state='delete_pending' WHERE id=$1`, objectID)
			return e
		})
		if e != nil {
			return out, e
		}
		if key == "" {
			continue
		}
		deleteCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		removeErr := s.objects.Delete(deleteCtx, key)
		cancel()
		e = s.access.Transact(ctx, func(tx pgx.Tx) error {
			if removeErr != nil {
				failed++
				_, e := tx.Exec(ctx, `UPDATE audit_cleanup_items SET state='failed' WHERE plan_id=$1 AND object_id=$2`, id, objectID)
				return e
			}
			_, e := tx.Exec(ctx, `UPDATE audit_export_objects SET state='deleted',deleted_at=clock_timestamp() WHERE id=$1 AND state='delete_pending'`, objectID)
			if e != nil {
				return e
			}
			_, e = tx.Exec(ctx, `DELETE FROM audit_export_rows r USING audit_export_objects o WHERE o.id=$1 AND r.export_id=o.export_id AND r.generation=o.generation AND NOT EXISTS(SELECT 1 FROM audit_export_objects other WHERE other.export_id=o.export_id AND other.generation=o.generation AND other.state='available')`, objectID)
			if e != nil {
				return e
			}
			_, e = tx.Exec(ctx, `UPDATE audit_cleanup_items SET state='deleted' WHERE plan_id=$1 AND object_id=$2`, id, objectID)
			if e == nil {
				deleted++
			}
			return e
		})
		if e != nil {
			return out, e
		}
	}
	out = map[string]any{"plan_id": id, "deleted": deleted, "protected": protected, "failed": failed}
	e = s.access.Transact(ctx, func(tx pgx.Tx) error {
		state := "applied"
		if protected+failed > 0 {
			state = "partial"
		}
		tag, e := tx.Exec(ctx, `UPDATE audit_cleanup_plans SET state=$2,revision=revision+1,reason=$3,applied_at=clock_timestamp(),apply_owner=NULL,apply_until=NULL WHERE id=$1 AND revision=$4 AND apply_owner=$5`, id, state, safeReason(reason), revision, request)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return Problem("audit_plan_changed", 409)
		}
		_, e = tx.Exec(ctx, `INSERT INTO audit_maintenance_receipts(request_id,operation,plan_id,reason,summary) VALUES($1,'cleanup.objects',$2,$3,$4)`, request, id, safeReason(reason), ar.JSON(out))
		if e != nil {
			return e
		}
		return maintenanceFactTx(ctx, tx, "applied", request, reason, out)
	})
	return out, e
}

func (s *Service) ReconcileTransfers(ctx context.Context) (int, error) {
	count := 0
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `SELECT id::text,operation_id::text,source_module,actor_id::text,membership_key,request_id,epoch::text,scope_kind,COALESCE(tenant_id::text,''),COALESCE(project_id::text,''),file_ref FROM audit_transfer_receipts WHERE closed_at IS NULL AND last_seen_at<$1 ORDER BY last_seen_at LIMIT $2 FOR UPDATE SKIP LOCKED`, s.now().Add(-s.options.TransferStaleAfter), s.options.FactBatch)
		if e != nil {
			return e
		}
		items := []ar.Transfer{}
		for rows.Next() {
			var t ar.Transfer
			if e = rows.Scan(&t.ID, &t.Operation, &t.Module, &t.Actor, &t.Membership, &t.Request, &t.Epoch, &t.Scope.Kind, &t.Scope.Tenant, &t.Scope.Project, &t.File); e != nil {
				rows.Close()
				return e
			}
			items = append(items, t)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		for _, t := range items {
			if e = ar.FinishTransferTx(ctx, tx, t, "interrupted_unknown", nil); e != nil {
				return e
			}
			count++
		}
		return nil
	})
	return count, e
}
