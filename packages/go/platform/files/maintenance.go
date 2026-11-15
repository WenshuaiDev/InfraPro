package files

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"io"
	"io/fs"
	"time"
)

type Status struct {
	Mode              string     `json:"mode"`
	RestoreEpoch      string     `json:"restore_epoch"`
	LastReconcileAt   *time.Time `json:"last_reconcile_at"`
	UploadsReceiving  string     `json:"uploads_receiving"`
	PreparedUnbound   string     `json:"prepared_unbound"`
	JobsQueued        string     `json:"jobs_queued"`
	JobsRunning       string     `json:"jobs_running"`
	JobsFailed        string     `json:"jobs_failed"`
	OldestQueuedAt    *time.Time `json:"oldest_queued_at"`
	TemporaryReserved string     `json:"temporary_reserved"`
	TemporaryActual   string     `json:"temporary_actual"`
	CleanupFailed     string     `json:"cleanup_failed"`
	ContentMissing    string     `json:"content_missing"`
	ContentCorrupt    string     `json:"content_corrupt"`
	CheckerAvailable  bool       `json:"checker_available"`
	WorkerLastSeen    *time.Time `json:"worker_last_seen"`
}

func (s *Service) Status(ctx context.Context) (Status, error) {
	var out Status
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		var checker *time.Time
		e := tx.QueryRow(ctx, `SELECT mode,restore_epoch,last_reconcile_at,uploads_receiving,prepared_unbound,jobs_queued,jobs_running,jobs_failed,oldest_queued_at,temporary_reserved,temporary_actual,cleanup_failed,content_missing,content_corrupt,worker_last_seen,checker_last_seen FROM file_operations_status`).Scan(&out.Mode, &out.RestoreEpoch, &out.LastReconcileAt, &out.UploadsReceiving, &out.PreparedUnbound, &out.JobsQueued, &out.JobsRunning, &out.JobsFailed, &out.OldestQueuedAt, &out.TemporaryReserved, &out.TemporaryActual, &out.CleanupFailed, &out.ContentMissing, &out.ContentCorrupt, &out.WorkerLastSeen, &checker)
		out.CheckerAvailable = checker != nil && checker.After(s.now().Add(-s.options.StaleAfter))
		return e
	})
	return out, e
}

type VerificationItem struct {
	ObjectID string `json:"object_id"`
	State    string `json:"state"`
}
type Verification struct {
	Mode     string             `json:"mode"`
	Complete bool               `json:"complete"`
	Checked  int                `json:"checked"`
	Items    []VerificationItem `json:"items"`
}
type object struct {
	ID, Space, Kind, Key, Hash, State, WriterKind, WriterID, Epoch string
	Size, Revision, Generation, Fence                              int64
	Deadline, Lease                                                time.Time
}

const objectColumns = `id::text,space_id::text,kind,object_key,expected_size,COALESCE(expected_sha256,''),state,writer_kind,writer_id::text,writer_generation,writer_fence,writer_epoch::text,io_deadline,lease_until,revision`

func objectScan(o *object) []any {
	return []any{&o.ID, &o.Space, &o.Kind, &o.Key, &o.Size, &o.Hash, &o.State, &o.WriterKind, &o.WriterID, &o.Generation, &o.Fence, &o.Epoch, &o.Deadline, &o.Lease, &o.Revision}
}
func (s *Service) registered(ctx context.Context) ([]object, error) {
	out := []object{}
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `SELECT `+objectColumns+` FROM file_objects ORDER BY id`)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var v object
			if e = rows.Scan(objectScan(&v)...); e != nil {
				return e
			}
			out = append(out, v)
		}
		return rows.Err()
	})
	return out, e
}
func (s *Service) inspectObject(ctx context.Context, o object) (string, error) {
	if s.objects == nil {
		return "unknown", fault("file_storage_unavailable", 503)
	}
	ioctx, cancel := context.WithTimeout(ctx, s.options.ObjectIOTimeout)
	defer cancel()
	reader, e := s.objects.Get(ioctx, o.Key)
	if errors.Is(e, fs.ErrNotExist) {
		return "missing", nil
	}
	if e != nil {
		return "unknown", fault("file_storage_unavailable", 503)
	}
	hash := sha256.New()
	n, e := io.Copy(hash, io.LimitReader(reader, o.Size+1))
	closeErr := reader.Close()
	if e != nil || closeErr != nil {
		return "unknown", fault("file_storage_unavailable", 503)
	}
	if n != o.Size || hex.EncodeToString(hash.Sum(nil)) != o.Hash {
		return "corrupt", nil
	}
	return "ok", nil
}
func (s *Service) Verify(ctx context.Context, mode string, apply bool, reason string) (Verification, error) {
	out := Verification{Mode: mode, Complete: true, Items: []VerificationItem{}}
	if mode != "metadata" && mode != "content" || apply && !validText(reason, 1, 500) {
		return out, fault("invalid_request", 400)
	}
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		var broken int
		e := tx.QueryRow(ctx, `SELECT count(*) FROM file_attachments a WHERE a.status IN ('active','trashed') AND EXISTS(SELECT 1 FROM file_attachment_revisions r WHERE r.attachment_id=a.id AND NOT EXISTS(SELECT 1 FROM file_protection_refs p WHERE p.file_id=r.file_id AND p.owner_id=r.id AND p.kind='attachment_revision' AND p.released_at IS NULL))`).Scan(&broken)
		if e != nil {
			return e
		}
		if broken > 0 {
			out.Complete = false
			out.Items = append(out.Items, VerificationItem{State: "reference_mismatch"})
		}
		return nil
	})
	if e != nil {
		return out, e
	}
	if mode == "metadata" {
		return out, nil
	}
	objects, e := s.registered(ctx)
	if e != nil {
		return out, e
	}
	for _, o := range objects {
		state, e := s.inspectObject(ctx, o)
		out.Checked++
		if e != nil {
			out.Complete = false
			out.Items = append(out.Items, VerificationItem{ObjectID: o.ID, State: "unknown"})
			return out, e
		}
		expectedAbsent := o.State == "deleted" || o.State == "delete_pending"
		if state == "missing" && expectedAbsent {
			continue
		}
		if expectedAbsent {
			out.Items = append(out.Items, VerificationItem{ObjectID: o.ID, State: "late_object"})
			out.Complete = false
			continue
		}
		// In-flight writers are unknown, including a temporarily absent key. Never
		// turn a lease timeout into an assertion that the remote operation stopped.
		if o.State == "writing" && s.now().Before(o.Deadline) {
			out.Items = append(out.Items, VerificationItem{ObjectID: o.ID, State: "writer_pending"})
			out.Complete = false
			continue
		}
		if state != "ok" {
			required := true
			if state == "missing" && (o.State == "writing" || o.State == "missing") {
				e = s.access.Transact(ctx, func(tx pgx.Tx) error {
					return tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM file_upload_parts WHERE object_id=$1 AND state='received') OR EXISTS(SELECT 1 FROM file_records WHERE source_object_id=$1 OR preview_object_id=$1)`, o.ID).Scan(&required)
				})
				if e != nil {
					return out, e
				}
			}
			if required {
				out.Complete = false
			}
			label := state
			if !required {
				label = "uncommitted_absent"
			}
			out.Items = append(out.Items, VerificationItem{ObjectID: o.ID, State: label})
		}
		if apply {
			e = s.access.Transact(ctx, func(tx pgx.Tx) error {
				if _, _, e := protocolTx(ctx, tx); e != nil {
					return e
				}
				// Lock the source file before its physical object, just like protect/purge.
				if _, e := tx.Exec(ctx, `SELECT id FROM file_records WHERE source_object_id=$1 OR preview_object_id=$1 ORDER BY id FOR UPDATE`, o.ID); e != nil {
					return e
				}
				newState := state
				if state == "ok" {
					newState = "available"
				}
				tag, e := tx.Exec(ctx, `UPDATE file_objects SET state=$3,actual_size=CASE WHEN $3='available' THEN expected_size ELSE actual_size END,actual_sha256=CASE WHEN $3='available' THEN expected_sha256 ELSE actual_sha256 END,last_verified_at=$4,revision=revision+1,last_error_code=CASE WHEN $3='available' THEN '' ELSE $3 END WHERE id=$1 AND revision=$2 AND state NOT IN ('deleted','delete_pending')`, o.ID, o.Revision, newState, s.now())
				if e != nil || tag.RowsAffected() == 0 {
					return e
				}
				_, e = tx.Exec(ctx, `UPDATE file_records SET integrity=$2,verified_at=$3 WHERE source_object_id=$1 AND lifecycle='ready'`, o.ID, state, s.now())
				if e != nil {
					return e
				}
				return maintenanceAuditTx(ctx, tx, "integrity", o.ID, reason, map[string]any{"state": o.State, "revision": number(o.Revision)}, map[string]any{"state": newState, "revision": number(o.Revision + 1)})
			})
			if e != nil {
				return out, e
			}
		}
	}
	if inventory, ok := s.objects.(interface {
		Inventory(context.Context, string, int) ([]string, string, error)
	}); ok {
		known := map[string]bool{}
		for _, o := range objects {
			known[o.Key] = true
		}
		after := ""
		for {
			keys, next, e := inventory.Inventory(ctx, after, 1000)
			if e != nil {
				return out, fault("file_storage_unavailable", 503)
			}
			for _, key := range keys {
				if !known[key] {
					out.Complete = false
					out.Items = append(out.Items, VerificationItem{ObjectID: taskrecord.Hash(key), State: "extra_unknown"})
				}
			}
			if next == "" {
				break
			}
			after = next
		}
	}
	return out, nil
}

type CleanupPlan struct {
	ID    string        `json:"id"`
	Epoch string        `json:"restore_epoch"`
	State string        `json:"state"`
	Items []CleanupItem `json:"items"`
}
type CleanupItem struct {
	ObjectID string `json:"object_id"`
	State    string `json:"state"`
	Reason   string `json:"reason_code"`
}

func (s *Service) cleanupEligibleTx(ctx context.Context, tx pgx.Tx, o object) (bool, string, string, error) {
	var file string
	var lifecycle string
	var until *time.Time
	e := tx.QueryRow(ctx, `SELECT id::text,lifecycle,unbound_until FROM file_records WHERE source_object_id=$1 OR preview_object_id=$1`, o.ID).Scan(&file, &lifecycle, &until)
	if e != nil && e != pgx.ErrNoRows {
		return false, "", "", e
	}
	var held bool
	if e == nil {
		e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM file_protection_refs p WHERE p.file_id=$1 AND p.released_at IS NULL AND (p.expires_at IS NULL OR p.expires_at>$2) AND NOT (p.kind='attachment_revision' AND EXISTS(SELECT 1 FROM file_attachment_revisions r JOIN file_attachments a ON a.id=r.attachment_id WHERE r.id=p.owner_id AND a.status IN ('trashed','purged') AND a.purge_after<=$2))) OR EXISTS(SELECT 1 FROM file_read_leases WHERE file_id=$1 AND closed_at IS NULL AND expires_at>$2)`, file, s.now()).Scan(&held)
		if e != nil {
			return false, "", file, e
		}
		if held {
			return false, "file_protected", file, nil
		}
		expired := until != nil && !s.now().Before(*until)
		if !expired && lifecycle == "ready" {
			e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM file_attachment_revisions r JOIN file_attachments a ON a.id=r.attachment_id WHERE r.file_id=$1 AND a.status IN ('trashed','purged') AND a.purge_after<=$2)`, file, s.now()).Scan(&expired)
			if e != nil {
				return false, "", file, e
			}
		}
		if !expired && lifecycle == "ready" {
			return false, "retention_active", file, nil
		}
	}
	e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM file_backup_items i JOIN file_backup_manifests m ON m.id=i.manifest_id WHERE i.object_id=$1 AND m.state<>'released')`, o.ID).Scan(&held)
	if e != nil {
		return false, "", file, e
	}
	if held {
		return false, "backup_protected", file, nil
	}
	if file != "" {
		return true, "file_retention_expired", file, nil
	}
	if s.now().Before(o.Deadline) || s.now().Before(o.Lease) {
		return false, "writer_pending", "", nil
	}
	if o.WriterKind == "upload" {
		var state string
		var expires time.Time
		e = tx.QueryRow(ctx, `SELECT state,LEAST(idle_expires_at,hard_expires_at) FROM file_uploads WHERE id=$1`, o.WriterID).Scan(&state, &expires)
		if e != nil {
			return false, "source_unknown", "", e
		}
		if state == "cancelled" || state == "expired" || state == "closed" || state == "receiving" && !s.now().Before(expires) {
			return true, "upload_retired", "", nil
		}
		if state == "submitted" {
			var done bool
			e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM file_jobs j JOIN file_job_runs r ON r.job_id=j.id AND r.generation=j.generation WHERE j.upload_id=$1 AND r.finished_at IS NOT NULL AND r.state<>'needs_action' AND r.input_retain_until<=$2)`, o.WriterID, s.now()).Scan(&done)
			return done, "input_retention_expired", "", e
		}
		// A replaced part writer uses an independent key; a confirmed new receipt is
		// proof this older key can never enter the frozen input.
		var current bool
		e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM file_upload_parts WHERE object_id=$1)`, o.ID).Scan(&current)
		return !current, "superseded_part", "", e
	}
	var live bool
	e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM file_job_runs WHERE job_id=$1 AND generation=$2 AND fence=$3 AND state='running' AND lease_until>$4 AND restore_epoch=$5)`, o.WriterID, o.Generation, o.Fence, s.now(), o.Epoch).Scan(&live)
	if e != nil {
		return false, "source_unknown", "", e
	}
	return !live, "fenced_output", "", nil
}
func (s *Service) CleanupPlan(ctx context.Context, operator, reason string) (CleanupPlan, error) {
	out := CleanupPlan{ID: uuid.NewString(), State: "planned", Items: []CleanupItem{}}
	if !validText(operator, 1, 100) || !validText(reason, 1, 500) {
		return out, fault("invalid_request", 400)
	}
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		var mode string
		var e error
		out.Epoch, mode, e = protocolTx(ctx, tx)
		if e != nil {
			return e
		}
		if mode != "online" {
			return fault("file_protocol_unavailable", 503)
		}
		_, e = tx.Exec(ctx, `INSERT INTO file_cleanup_runs(id,mode,operator,reason,restore_epoch,state,planned_at) VALUES($1,'plan',$2,$3,$4,'planned',$5)`, out.ID, operator, reason, out.Epoch, s.now())
		if e != nil {
			return e
		}
		var cursor *string
		if e = tx.QueryRow(ctx, `SELECT cleanup_cursor::text FROM file_budget_states WHERE key='instance' FOR UPDATE`).Scan(&cursor); e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT `+objectColumns+` FROM file_objects WHERE ($1::uuid IS NULL OR id>$1) ORDER BY id LIMIT $2`, cursor, s.options.CleanupBatch*10)
		if e != nil {
			return e
		}
		objects := []object{}
		for rows.Next() {
			var o object
			if e = rows.Scan(objectScan(&o)...); e != nil {
				rows.Close()
				return e
			}
			objects = append(objects, o)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		var last *string
		for _, o := range objects {
			last = pointer(o.ID)
			eligible, why, file, e := s.cleanupEligibleTx(ctx, tx, o)
			if e != nil {
				return e
			}
			if !eligible {
				continue
			}
			_, e = tx.Exec(ctx, `INSERT INTO file_cleanup_items(run_id,space_id,object_id,file_id,candidate_revision,reason_code,not_before,state) VALUES($1,$2,$3,$4,$5,$6,$7,'planned')`, out.ID, o.Space, o.ID, taskrecord.Null(file), o.Revision, why, s.now())
			if e != nil {
				return e
			}
			out.Items = append(out.Items, CleanupItem{ObjectID: o.ID, State: "planned", Reason: why})
			if len(out.Items) >= s.options.CleanupBatch {
				break
			}
		}
		if len(objects) < s.options.CleanupBatch*10 && (len(objects) == 0 || last != nil && *last == objects[len(objects)-1].ID) {
			last = nil
		}
		_, e = tx.Exec(ctx, `UPDATE file_budget_states SET cleanup_cursor=$1 WHERE key='instance'`, last)
		if e != nil {
			return e
		}
		return maintenanceAuditTx(ctx, tx, "cleanup.planned", out.ID, reason, nil, map[string]any{"state": out.State, "count": len(out.Items)})
	})
	return out, e
}
func (s *Service) purgeAttachmentsTx(ctx context.Context, tx pgx.Tx, objectID string) error {
	rows, e := tx.Query(ctx, `SELECT DISTINCT a.id::text FROM file_records f JOIN file_attachment_revisions r ON r.file_id=f.id JOIN file_attachments a ON a.id=r.attachment_id WHERE (f.source_object_id=$1 OR f.preview_object_id=$1) AND a.status='trashed' AND a.purge_after<=$2 ORDER BY a.id::text`, objectID, s.now())
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
	for _, id := range ids {
		var valid bool
		e = tx.QueryRow(ctx, `SELECT status='trashed' AND purge_after<=$2 FROM file_attachments WHERE id=$1 FOR UPDATE`, id, s.now()).Scan(&valid)
		if e != nil {
			return e
		}
		if !valid {
			continue
		}
		if _, e = tx.Exec(ctx, `SELECT id FROM file_records WHERE id IN(SELECT file_id FROM file_attachment_revisions WHERE attachment_id=$1) ORDER BY id FOR UPDATE`, id); e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE file_attachments SET status='purged',revision=revision+1,updated_at=$2 WHERE id=$1`, id, s.now())
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE file_protection_refs SET released_at=$2,release_reason='trash_retention_expired' WHERE kind='attachment_revision' AND owner_id IN(SELECT id FROM file_attachment_revisions WHERE attachment_id=$1) AND released_at IS NULL`, id, s.now())
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE file_spaces SET content_revision=content_revision+1 WHERE id=(SELECT space_id FROM file_attachments WHERE id=$1)`, id)
		if e != nil {
			return e
		}
	}
	return nil
}
func (s *Service) CleanupApply(ctx context.Context, id, reason string) (CleanupPlan, error) {
	out := CleanupPlan{ID: id, State: "completed", Items: []CleanupItem{}}
	if !validText(reason, 1, 500) {
		return out, fault("invalid_request", 400)
	}
	type item struct {
		id       string
		revision int64
	}
	items := []item{}
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		epoch, mode, e := protocolTx(ctx, tx)
		if e != nil {
			return e
		}
		if mode != "online" {
			return fault("file_protocol_unavailable", 503)
		}
		var planned time.Time
		e = tx.QueryRow(ctx, `SELECT restore_epoch::text,planned_at FROM file_cleanup_runs WHERE id=$1 FOR UPDATE`, id).Scan(&out.Epoch, &planned)
		if e != nil {
			return notFound(e)
		}
		if out.Epoch != epoch || s.now().Sub(planned) > 15*time.Minute {
			return fault("cleanup_plan_stale", 409)
		}
		rows, e := tx.Query(ctx, `SELECT object_id::text,candidate_revision FROM file_cleanup_items WHERE run_id=$1 AND state IN ('planned','failed') ORDER BY object_id`, id)
		if e != nil {
			return e
		}
		for rows.Next() {
			var i item
			if e = rows.Scan(&i.id, &i.revision); e != nil {
				rows.Close()
				return e
			}
			items = append(items, i)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE file_cleanup_runs SET mode='apply',state='running',reason=$2 WHERE id=$1`, id, reason)
		return e
	})
	if e != nil {
		return out, e
	}
	for _, it := range items {
		var o object
		deleteIt := false
		state, why := "protected", "reference_changed"
		e = s.access.Transact(ctx, func(tx pgx.Tx) error {
			epoch, mode, e := protocolTx(ctx, tx)
			if e != nil {
				return e
			}
			if mode != "online" || epoch != out.Epoch {
				return fault("file_protocol_unavailable", 503)
			}
			if _, e = tx.Exec(ctx, `SELECT key FROM file_budget_states WHERE key='instance' FOR UPDATE`); e != nil {
				return e
			}
			// Space locks serialize attachment restore/purge without borrowing a user.
			if _, e = tx.Exec(ctx, `SELECT id FROM file_spaces WHERE id=(SELECT space_id FROM file_objects WHERE id=$1) FOR UPDATE`, it.id); e != nil {
				return e
			}
			if e = s.purgeAttachmentsTx(ctx, tx, it.id); e != nil {
				return e
			}
			if _, e = tx.Exec(ctx, `SELECT id FROM file_records WHERE source_object_id=$1 OR preview_object_id=$1 ORDER BY id FOR UPDATE`, it.id); e != nil {
				return e
			}
			e = tx.QueryRow(ctx, `SELECT `+objectColumns+` FROM file_objects WHERE id=$1 FOR UPDATE`, it.id).Scan(objectScan(&o)...)
			if e != nil {
				return e
			}
			if o.Revision != it.revision && o.State != "delete_pending" {
				state = "stale"
				why = "candidate_changed"
				return nil
			}
			eligible, reason, file, e := s.cleanupEligibleTx(ctx, tx, o)
			if e != nil {
				return e
			}
			why = reason
			if !eligible {
				return nil
			}
			if file != "" {
				_, e = tx.Exec(ctx, `UPDATE file_records SET lifecycle='reclaiming' WHERE id=$1 AND lifecycle='ready'`, file)
				if e != nil {
					return e
				}
			}
			_, e = tx.Exec(ctx, `UPDATE file_objects SET state='delete_pending',delete_attempts=delete_attempts+1,revision=revision+1,delete_after=$2,updated_at=$2 WHERE id=$1`, o.ID, s.now())
			if e != nil {
				return e
			}
			deleteIt = true
			return nil
		})
		if e != nil {
			return out, e
		}
		if deleteIt {
			if s.objects == nil {
				e = fault("file_storage_unavailable", 503)
			} else {
				ioctx, cancel := context.WithTimeout(ctx, s.options.ObjectIOTimeout)
				e = s.objects.Delete(ioctx, o.Key)
				cancel()
			}
			if e != nil {
				state = "failed"
				why = "file_storage_unavailable"
				out.State = "failed"
			} else {
				state = "deleted"
				why = ""
			}
		}
		e = s.access.Transact(ctx, func(tx pgx.Tx) error {
			if state == "deleted" {
				if _, e := tx.Exec(ctx, `UPDATE file_objects SET state='deleted',revision=revision+1,last_error_code='',updated_at=$2 WHERE id=$1 AND state='delete_pending'`, it.id, s.now()); e != nil {
					return e
				}
				if _, e := tx.Exec(ctx, `UPDATE file_records f SET lifecycle='deleted',reclaimed_at=$2 WHERE (source_object_id=$1 OR preview_object_id=$1) AND lifecycle='reclaiming' AND NOT EXISTS(SELECT 1 FROM file_objects o WHERE o.id IN(f.source_object_id,f.preview_object_id) AND o.state<>'deleted')`, it.id, s.now()); e != nil {
					return e
				}
			}
			_, e := tx.Exec(ctx, `UPDATE file_cleanup_items SET state=$3,last_error_code=$4,attempt_no=attempt_no+1 WHERE run_id=$1 AND object_id=$2`, id, it.id, state, why)
			if e != nil {
				return e
			}
			return maintenanceAuditTx(ctx, tx, "cleanup.item", it.id, reason, nil, map[string]any{"state": state, "error_code": why, "run_id": id})
		})
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, CleanupItem{ObjectID: it.id, State: state, Reason: why})
	}
	e = s.access.Transact(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `UPDATE file_cleanup_runs SET state=$2,finished_at=$3 WHERE id=$1`, id, out.State, s.now())
		if e != nil {
			return e
		}
		return maintenanceAuditTx(ctx, tx, "cleanup.completed", id, reason, nil, map[string]any{"state": out.State, "count": len(out.Items)})
	})
	if e == nil {
		e = s.reconcileBudgets(ctx)
	}
	return out, e
}
func (s *Service) reconcileBudgets(ctx context.Context) error {
	return s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `SELECT key FROM file_budget_states WHERE key='instance' FOR UPDATE`); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, `UPDATE file_budget_reservations b SET actual_bytes=(SELECT COALESCE(sum(o.expected_size),0) FROM file_objects o WHERE o.state<>'deleted' AND ((o.writer_kind='upload' AND o.writer_id=b.owner_id) OR(o.writer_kind='job' AND o.writer_id IN(SELECT id FROM file_jobs WHERE upload_id=b.owner_id))) AND NOT EXISTS(SELECT 1 FROM file_records f WHERE (f.source_object_id=o.id OR f.preview_object_id=o.id) AND EXISTS(SELECT 1 FROM file_protection_refs p WHERE p.file_id=f.id AND p.kind='attachment_revision' AND p.released_at IS NULL))),reconciled_at=$1 WHERE b.kind='temporary_object' AND b.state<>'released'`, s.now())
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE file_budget_reservations b SET reserved_bytes=actual_bytes,state=CASE WHEN actual_bytes=0 THEN 'released' ELSE 'held' END WHERE kind='temporary_object' AND EXISTS(SELECT 1 FROM file_uploads u WHERE u.id=b.owner_id AND (u.state IN('closed','cancelled','expired') OR(u.state='receiving' AND LEAST(u.idle_expires_at,u.hard_expires_at)<=$1)))`, s.now())
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE file_budget_states SET reserved_bytes=(SELECT COALESCE(sum(reserved_bytes),0) FROM file_budget_reservations WHERE kind='temporary_object' AND state<>'released'),revision=revision+1 WHERE key='instance'`)
		return e
	})
}

// CheckDependencies is explicit execution diagnostics, never called by Compose.
func (s *Service) CheckDependencies(ctx context.Context) error {
	if s.objects == nil {
		return fault("file_storage_unavailable", 503)
	}
	if probe, ok := s.objects.(interface{ Probe(context.Context) error }); ok {
		if e := probe.Probe(ctx); e != nil {
			return fault("file_storage_unavailable", 503)
		}
	}
	if !s.checkerAvailable(ctx) {
		return fault("file_worker_unavailable", 503)
	}
	status, e := s.Status(ctx)
	if e != nil {
		return e
	}
	if status.WorkerLastSeen == nil || s.now().Sub(*status.WorkerLastSeen) > s.options.StaleAfter {
		return fault("file_worker_unavailable", 503)
	}
	return nil
}
