package audit

import (
	"context"
	ar "github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	"github.com/jackc/pgx/v5"
	"time"
)

type ManifestObject struct {
	ID         string     `json:"id"`
	ExportID   string     `json:"export_id"`
	Generation string     `json:"generation"`
	Key        string     `json:"object_key"`
	Size       string     `json:"size_bytes"`
	SHA256     string     `json:"sha256"`
	ExpiresAt  *time.Time `json:"expires_at"`
	State      string     `json:"state"`
}
type Manifest struct {
	SnapshotRef string           `json:"snapshot_ref"`
	Epoch       string           `json:"epoch"`
	Objects     []ManifestObject `json:"objects"`
	Next        string           `json:"next_cursor"`
	ObservedAt  time.Time        `json:"observed_at"`
	Digest      string           `json:"sha256"`
}

func (s *Service) Manifest(ctx context.Context, snapshot, after string) (Manifest, error) {
	out := Manifest{SnapshotRef: snapshot, Objects: []ManifestObject{}, ObservedAt: s.now()}
	if snapshot == "" || len(snapshot) > 100 || !identifier.MatchString(snapshot) || after != "" && !ar.ID(after) {
		return out, Problem("audit_filter_invalid", 400)
	}
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `SET TRANSACTION ISOLATION LEVEL REPEATABLE READ`); e != nil {
			return e
		}
		var mode string
		if e := tx.QueryRow(ctx, `SELECT restore_epoch::text,mode FROM audit_protocol WHERE id=1`).Scan(&out.Epoch, &mode); e != nil {
			return e
		}
		if mode == "ready" {
			return Problem("audit_backup_freeze_required", 409)
		}
		rows, e := tx.Query(ctx, `SELECT id::text,export_id::text,generation::text,object_key,COALESCE(size_bytes,0)::text,COALESCE(sha256,''),expires_at,state FROM audit_export_objects WHERE ($1='' OR id>NULLIF($1,'')::uuid) AND state NOT IN ('deleted','delete_pending','intent') ORDER BY id LIMIT $2`, after, s.options.ObjectBatch+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var v ManifestObject
			if e = rows.Scan(&v.ID, &v.ExportID, &v.Generation, &v.Key, &v.Size, &v.SHA256, &v.ExpiresAt, &v.State); e != nil {
				return e
			}
			if len(out.Objects) == s.options.ObjectBatch {
				out.Next = out.Objects[len(out.Objects)-1].ID
				break
			}
			out.Objects = append(out.Objects, v)
		}
		return rows.Err()
	})
	out.Digest = ar.Hash(out.Objects)
	return out, e
}
func (s *Service) SetMaintenanceMode(ctx context.Context, mode, request, reason string) error {
	if mode != "ready" && mode != "read_only" && mode != "reconciling" {
		return Problem("audit_filter_invalid", 400)
	}
	if e := maintenanceInput(request, reason); e != nil {
		return e
	}
	return s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,834326))`, request); e != nil {
			return e
		}
		var prior, priorMode string
		e := tx.QueryRow(ctx, `SELECT operation,summary->>'mode' FROM audit_maintenance_receipts WHERE request_id=$1`, request).Scan(&prior, &priorMode)
		if e == nil {
			if prior != "mode" || priorMode != mode {
				return Problem("request_conflict", 409)
			}
			return nil
		}
		if e != pgx.ErrNoRows {
			return e
		}
		if _, e := tx.Exec(ctx, `UPDATE audit_protocol SET mode=$1 WHERE id=1`, mode); e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO audit_maintenance_receipts(request_id,operation,reason,summary) VALUES($1,'mode',$2,$3) ON CONFLICT(request_id) DO NOTHING`, request, safeReason(reason), ar.JSON(map[string]string{"mode": mode}))
		if e != nil {
			return e
		}
		if mode == "reconciling" {
			return nil
		} // Gate blocks native appends until reconcile establishes the new epoch.
		return maintenanceFactTx(ctx, tx, "mode", request, reason, map[string]string{"mode": mode})
	})
}

// RestoreReconcile requires the persisted recovery gate. It preserves command
// receipts, immutable terminal generations, notifications and source identity.
// It leaves read_only for verification; reopening is an explicit mode command.
func (s *Service) RestoreReconcile(ctx context.Context, request, reason string) (map[string]any, error) {
	out := map[string]any{}
	if e := maintenanceInput(request, reason); e != nil {
		return out, e
	}
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		e := tx.QueryRow(ctx, `SELECT summary FROM audit_maintenance_receipts WHERE request_id=$1 AND operation='restore.reconcile'`, request).Scan(&out)
		if e == nil {
			return nil
		}
		if e != pgx.ErrNoRows {
			return e
		}
		var mode string
		if e = tx.QueryRow(ctx, `SELECT mode FROM audit_protocol WHERE id=1 FOR UPDATE`).Scan(&mode); e != nil {
			return e
		}
		if mode != "reconciling" {
			return Problem("audit_restore_gate_required", 409)
		}
		if _, e = tx.Exec(ctx, `UPDATE audit_protocol SET restore_epoch=gen_random_uuid(),cursor_secret=gen_random_uuid()::text||gen_random_uuid()::text,cleanup_revision=cleanup_revision+1,mode='read_only' WHERE id=1`); e != nil {
			return e
		}
		if s.restore != nil {
			if e = s.restore(ctx, tx, reason); e != nil {
				return e
			}
		}
		if _, e = tx.Exec(ctx, `UPDATE audit_export_read_leases SET closed_at=clock_timestamp() WHERE closed_at IS NULL`); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE audit_temp_reservations SET released_at=clock_timestamp() WHERE released_at IS NULL`); e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT x.id::text FROM audit_exports x JOIN audit_export_generations g ON g.export_id=x.id AND g.generation=x.current_generation WHERE g.state IN ('queued','running','retry_wait','cancelling') ORDER BY x.id FOR UPDATE OF x,g`)
		if e != nil {
			return e
		}
		ids, e := pgx.CollectRows(rows, pgx.RowTo[string])
		if e != nil {
			return e
		}
		for _, id := range ids {
			v, e := loadExportTx(ctx, tx, id, "", true)
			if e != nil {
				return e
			}
			_, e = tx.Exec(ctx, `UPDATE audit_export_generations SET state='failed',phase='failed',error_code='restore_interrupted',fence=fence+1,lease_owner=NULL,lease_until=NULL,finished_at=clock_timestamp() WHERE export_id=$1 AND generation=$2`, id, v.Generation)
			if e != nil {
				return e
			}
			if _, e = tx.Exec(ctx, `UPDATE audit_exports SET source_revision=source_revision+1,control_revision=control_revision+1 WHERE id=$1`, id); e != nil {
				return e
			}
			v, e = loadExportTx(ctx, tx, id, "", false)
			if e != nil {
				return e
			}
			if e = s.exportFactTx(ctx, tx, v, "audit.export.restore_interrupted", "", "maintenance", reason); e != nil {
				return e
			}
			if e = s.indexExportTx(ctx, tx, v, "", "restore_interrupted"); e != nil {
				return e
			}
		}
		// The old stream may have stopped between any two writes. Its previous
		// heartbeat is not a final byte count, so recovery records unknown bytes.
		rows, e = tx.Query(ctx, `SELECT id::text,operation_id::text,source_module,actor_id::text,membership_key,request_id,epoch::text,scope_kind,COALESCE(tenant_id::text,''),COALESCE(project_id::text,''),file_ref FROM audit_transfer_receipts WHERE closed_at IS NULL FOR UPDATE`)
		if e != nil {
			return e
		}
		transfers := []ar.Transfer{}
		for rows.Next() {
			var t ar.Transfer
			if e = rows.Scan(&t.ID, &t.Operation, &t.Module, &t.Actor, &t.Membership, &t.Request, &t.Epoch, &t.Scope.Kind, &t.Scope.Tenant, &t.Scope.Project, &t.File); e != nil {
				rows.Close()
				return e
			}
			transfers = append(transfers, t)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		for _, t := range transfers {
			if e = ar.FinishTransferTx(ctx, tx, t, "interrupted_unknown", nil); e != nil {
				return e
			}
		}
		var epoch string
		if e = tx.QueryRow(ctx, `SELECT restore_epoch::text FROM audit_protocol WHERE id=1`).Scan(&epoch); e != nil {
			return e
		}
		out = map[string]any{"epoch": epoch, "interrupted_exports": len(ids), "interrupted_transfers": len(transfers), "notifications_reissued": 0, "mode": "read_only"}
		_, e = tx.Exec(ctx, `INSERT INTO audit_maintenance_receipts(request_id,operation,reason,summary) VALUES($1,'restore.reconcile',$2,$3)`, request, safeReason(reason), ar.JSON(out))
		return e
	})
	return out, e
}
