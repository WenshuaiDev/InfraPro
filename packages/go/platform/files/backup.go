package files

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type BackupManifest struct {
	Schema        int       `json:"schema_version"`
	ID            string    `json:"id"`
	Epoch         string    `json:"restore_epoch"`
	State         string    `json:"state"`
	SnapshotRef   string    `json:"database_snapshot_ref"`
	PolicyVersion int       `json:"policy_version"`
	CreatedAt     time.Time `json:"created_at"`
	HoldUntil     time.Time `json:"hold_until"`
	ObjectsHash   string    `json:"objects_sha256"`
	Count         int       `json:"object_count"`
	DatabaseHash  string    `json:"database_sha256,omitempty"`
	DatabaseSize  string    `json:"database_size_bytes,omitempty"`
}
type BackupObject struct {
	SpaceID        string  `json:"space_id"`
	ID             string  `json:"object_id"`
	Kind           string  `json:"kind"`
	Size           string  `json:"size_bytes"`
	SHA256         string  `json:"sha256"`
	Key            string  `json:"object_key"`
	StorageVersion *string `json:"storage_version"`
}

// BackupPrepare erects a module barrier before fencing writers. The caller then
// takes pg_dump while this barrier remains held, copies objects, and seals.
func (s *Service) BackupPrepare(ctx context.Context, snapshotRef, reason string) (BackupManifest, error) {
	out := BackupManifest{Schema: 1, ID: uuid.NewString(), State: "preparing", SnapshotRef: snapshotRef, PolicyVersion: 1, CreatedAt: s.now(), HoldUntil: s.now().Add(7 * 24 * time.Hour)}
	if !validText(snapshotRef, 1, 500) || !validText(reason, 1, 500) {
		return out, fault("invalid_request", 400)
	}
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		var mode string
		e := tx.QueryRow(ctx, `SELECT restore_epoch::text,mode FROM file_protocol WHERE id=1 FOR UPDATE`).Scan(&out.Epoch, &mode)
		if e != nil {
			return e
		}
		if mode != "online" {
			return fault("file_protocol_unavailable", 503)
		}
		_, e = tx.Exec(ctx, `UPDATE file_protocol SET mode='read_only',updated_at=$1 WHERE id=1`, s.now())
		if e != nil {
			return e
		}
		// A source generation remains active on the original installation. Old IO
		// gets independent intent keys and cannot publish after this fence.
		rows, e := tx.Query(ctx, `SELECT job_id::text,generation::text FROM file_job_runs WHERE state='running' FOR UPDATE`)
		if e != nil {
			return e
		}
		type run struct{ id, generation string }
		runs := []run{}
		for rows.Next() {
			var r run
			if e = rows.Scan(&r.id, &r.generation); e != nil {
				rows.Close()
				return e
			}
			runs = append(runs, r)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		for _, r := range runs {
			_, e = tx.Exec(ctx, `UPDATE file_job_runs SET state='retry_wait',fence=fence+1,lease_owner=NULL,lease_until=NULL,available_at=$3 WHERE job_id=$1 AND generation=$2`, r.id, r.generation, s.now().Add(s.options.ObjectIOTimeout))
			if e != nil {
				return e
			}
			_, e = tx.Exec(ctx, `UPDATE file_jobs SET state='retry_wait',phase='waiting',source_revision=source_revision+1,error_code='backup_fenced',observed_at=$2 WHERE id=$1`, r.id, s.now())
			if e != nil {
				return e
			}
			_, e = tx.Exec(ctx, `UPDATE file_job_attempts SET finished_at=$3,outcome='superseded',error_code='backup_fenced' WHERE job_id=$1 AND generation=$2 AND finished_at IS NULL`, r.id, r.generation, s.now())
			if e != nil {
				return e
			}
			if _, e = s.projectJobTx(ctx, tx, r.id, "", "backup_fenced", reason); e != nil {
				return e
			}
		}
		_, e = tx.Exec(ctx, `INSERT INTO file_backup_manifests(id,restore_epoch,state,database_snapshot_ref,hold_until,created_at) VALUES($1,$2,'preparing',$3,$4,$5)`, out.ID, out.Epoch, snapshotRef, out.HoldUntil, out.CreatedAt)
		if e != nil {
			return e
		}
		return maintenanceAuditTx(ctx, tx, "backup.prepared", out.ID, reason, nil, map[string]string{"state": "preparing", "epoch": out.Epoch})
	})
	if e != nil {
		return out, e
	}
	// No database transaction is held while waiting for a registered IO deadline.
	var deadline time.Time
	e = s.access.Transact(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT COALESCE(max(io_deadline),$1) FROM file_objects WHERE state='writing'`, s.now()).Scan(&deadline)
	})
	if e != nil {
		return out, e
	}
	if delay := time.Until(deadline); delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-timer.C:
		}
	}
	objects, e := s.registered(ctx)
	if e != nil {
		return out, e
	}
	for _, o := range objects {
		if o.State == "deleted" || o.State == "delete_pending" {
			continue
		}
		state, e := s.inspectObject(ctx, o)
		if e != nil {
			return out, e
		}
		if state != "ok" {
			// An uncommitted writer may legitimately have no object. Its intent remains
			// in the DB snapshot; every accepted receipt and formal body is mandatory.
			var required bool
			e = s.access.Transact(ctx, func(tx pgx.Tx) error {
				return tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM file_upload_parts WHERE object_id=$1 AND state='received') OR EXISTS(SELECT 1 FROM file_records WHERE (source_object_id=$1 OR preview_object_id=$1) AND lifecycle='ready')`, o.ID).Scan(&required)
			})
			if e != nil {
				return out, e
			}
			if required || state == "corrupt" {
				_ = s.failBackup(ctx, out.ID)
				out.State = "failed"
				return out, fault("backup_incomplete", 409)
			}
			continue
		}
		e = s.access.Transact(ctx, func(tx pgx.Tx) error {
			_, e := tx.Exec(ctx, `INSERT INTO file_backup_items(manifest_id,space_id,object_id,size_bytes,sha256,storage_version,copy_status) SELECT $1,space_id,id,expected_size,expected_sha256,storage_version,'pending' FROM file_objects WHERE id=$2 ON CONFLICT DO NOTHING`, out.ID, o.ID)
			return e
		})
		if e != nil {
			return out, e
		}
		out.Count++
	}
	return out, nil
}
func (s *Service) failBackup(ctx context.Context, id string) error {
	return s.access.Transact(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `UPDATE file_backup_manifests SET state='failed' WHERE id=$1 AND state='preparing'`, id)
		return e
	})
}
func (s *Service) backupManifest(ctx context.Context, id string) (BackupManifest, []BackupObject, error) {
	out := BackupManifest{Schema: 1, PolicyVersion: 1}
	objects := []BackupObject{}
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		e := tx.QueryRow(ctx, `SELECT id::text,restore_epoch::text,state,database_snapshot_ref,created_at,hold_until,COALESCE(manifest_hash,'') FROM file_backup_manifests WHERE id=$1`, id).Scan(&out.ID, &out.Epoch, &out.State, &out.SnapshotRef, &out.CreatedAt, &out.HoldUntil, &out.ObjectsHash)
		if e != nil {
			return notFound(e)
		}
		rows, e := tx.Query(ctx, `SELECT i.space_id::text,i.object_id::text,o.kind,i.size_bytes::text,i.sha256,o.object_key,i.storage_version FROM file_backup_items i JOIN file_objects o ON o.id=i.object_id WHERE i.manifest_id=$1 ORDER BY i.object_id`, id)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var o BackupObject
			if e = rows.Scan(&o.SpaceID, &o.ID, &o.Kind, &o.Size, &o.SHA256, &o.Key, &o.StorageVersion); e != nil {
				return e
			}
			objects = append(objects, o)
		}
		out.Count = len(objects)
		return rows.Err()
	})
	return out, objects, e
}
func privateDirectory(dir string) error {
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	st, e := os.Lstat(dir)
	if e != nil {
		return e
	}
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("backup directory must be a real private directory")
	}
	return os.Chmod(dir, 0700)
}
func secureFile(path string) (*os.File, error) {
	st, e := os.Lstat(path)
	if e != nil {
		return nil, e
	}
	if !st.Mode().IsRegular() || st.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("backup entry must be a regular file")
	}
	return os.Open(path)
}
func verifyBackupFile(path string, size int64, expected string) error {
	f, e := secureFile(path)
	if e != nil {
		return e
	}
	defer f.Close()
	hash := sha256.New()
	n, e := io.Copy(hash, io.LimitReader(f, size+1))
	if e != nil {
		return e
	}
	if n != size || hex.EncodeToString(hash.Sum(nil)) != expected {
		return fault("backup_integrity_mismatch", 422)
	}
	return nil
}
func (s *Service) BackupCopy(ctx context.Context, id, directory string) (BackupManifest, error) {
	manifest, objects, e := s.backupManifest(ctx, id)
	if e != nil {
		return manifest, e
	}
	if manifest.State != "preparing" {
		return manifest, fault("backup_state_changed", 409)
	}
	if s.objects == nil {
		return manifest, fault("file_storage_unavailable", 503)
	}
	if e = privateDirectory(directory); e != nil {
		return manifest, e
	}
	objectDir := filepath.Join(directory, "objects")
	if e = privateDirectory(objectDir); e != nil {
		return manifest, e
	}
	for _, o := range objects {
		size, _ := integer(o.Size, 1)
		dest := filepath.Join(objectDir, o.ID+".bin")
		if _, err := os.Lstat(dest); err == nil {
			if e = verifyBackupFile(dest, size, o.SHA256); e != nil {
				return manifest, e
			}
		} else if errors.Is(err, os.ErrNotExist) {
			reader, err := s.objects.Get(ctx, o.Key)
			if err != nil {
				return manifest, fault("file_storage_unavailable", 503)
			}
			tmp, err := os.CreateTemp(objectDir, "copy-")
			if err != nil {
				reader.Close()
				return manifest, err
			}
			n, err := io.Copy(tmp, io.LimitReader(reader, size+1))
			closeErr := reader.Close()
			syncErr := tmp.Sync()
			tmp.Close()
			if err == nil && closeErr == nil && syncErr == nil && n == size {
				err = verifyBackupFile(tmp.Name(), size, o.SHA256)
			} else {
				err = fault("backup_integrity_mismatch", 422)
			}
			if err != nil {
				os.Remove(tmp.Name())
				return manifest, err
			}
			if err = os.Rename(tmp.Name(), dest); err != nil {
				return manifest, err
			}
		} else {
			return manifest, err
		}
		e = s.access.Transact(ctx, func(tx pgx.Tx) error {
			_, e := tx.Exec(ctx, `UPDATE file_backup_items SET copy_status='verified' WHERE manifest_id=$1 AND object_id=$2`, id, o.ID)
			return e
		})
		if e != nil {
			return manifest, e
		}
	}
	var content []byte
	for _, o := range objects {
		content = append(content, taskrecord.JSON(o)...)
		content = append(content, '\n')
	}
	sum := sha256.Sum256(content)
	manifest.ObjectsHash = hex.EncodeToString(sum[:])
	manifest.Count = len(objects)
	if e = os.WriteFile(filepath.Join(directory, "objects.jsonl"), content, 0600); e != nil {
		return manifest, e
	}
	if e = os.WriteFile(filepath.Join(directory, "manifest.json"), taskrecord.JSON(manifest), 0600); e != nil {
		return manifest, e
	}
	return manifest, nil
}
func loadBackup(directory string) (BackupManifest, []BackupObject, error) {
	var m BackupManifest
	f, e := secureFile(filepath.Join(directory, "manifest.json"))
	if e != nil {
		return m, nil, e
	}
	raw, e := io.ReadAll(io.LimitReader(f, 16385))
	f.Close()
	if e != nil || len(raw) > 16384 {
		return m, nil, fault("backup_invalid_manifest", 422)
	}
	if e = strictWire(raw, &m); e != nil || m.Schema != 1 || !access.ValidID(m.ID) || !access.ValidID(m.Epoch) || !digestPattern.MatchString(m.ObjectsHash) || m.Count < 0 || m.PolicyVersion != 1 {
		return m, nil, fault("backup_invalid_manifest", 422)
	}
	f, e = secureFile(filepath.Join(directory, "objects.jsonl"))
	if e != nil {
		return m, nil, e
	}
	defer f.Close()
	h := sha256.New()
	scanner := bufio.NewScanner(io.TeeReader(f, h))
	scanner.Buffer(make([]byte, 4096), 16384)
	objects := []BackupObject{}
	seen := map[string]bool{}
	for scanner.Scan() {
		var o BackupObject
		if e = strictWire(scanner.Bytes(), &o); e != nil {
			return m, nil, e
		}
		size, e := integer(o.Size, 1)
		if e != nil || size > 100<<20 || !access.ValidID(o.ID) || !access.ValidID(o.SpaceID) || seen[o.ID] || o.Key != "files/v1/"+o.SpaceID+"/"+o.ID || !digestPattern.MatchString(o.SHA256) || (o.Kind != "part" && o.Kind != "source" && o.Kind != "preview") {
			return m, nil, fault("backup_invalid_manifest", 422)
		}
		seen[o.ID] = true
		objects = append(objects, o)
	}
	if e = scanner.Err(); e != nil {
		return m, nil, e
	}
	if len(objects) != m.Count || hex.EncodeToString(h.Sum(nil)) != m.ObjectsHash || !sort.SliceIsSorted(objects, func(i, j int) bool { return objects[i].ID < objects[j].ID }) {
		return m, nil, fault("backup_integrity_mismatch", 422)
	}
	objectDir, err := os.Lstat(filepath.Join(directory, "objects"))
	if err != nil || !objectDir.IsDir() || objectDir.Mode()&os.ModeSymlink != 0 {
		return m, nil, fault("backup_invalid_manifest", 422)
	}
	for _, o := range objects {
		size, _ := integer(o.Size, 1)
		if e = verifyBackupFile(filepath.Join(directory, "objects", o.ID+".bin"), size, o.SHA256); e != nil {
			return m, nil, e
		}
	}
	if m.State == "sealed" {
		size, e := integer(m.DatabaseSize, 1)
		if e != nil || !digestPattern.MatchString(m.DatabaseHash) {
			return m, nil, fault("backup_invalid_database", 422)
		}
		if e = verifyBackupFile(filepath.Join(directory, "database.dump"), size, m.DatabaseHash); e != nil {
			return m, nil, e
		}
	}
	return m, objects, nil
}

func (s *Service) BackupSeal(ctx context.Context, id, directory, reason string) (BackupManifest, error) {
	manifest, objects, e := loadBackup(directory)
	if e != nil {
		return manifest, e
	}
	if manifest.ID != id || !validText(reason, 1, 500) {
		return manifest, fault("invalid_request", 400)
	}
	// A real consistent DB dump is part of the package, not a caller-supplied bool.
	dump, e := secureFile(filepath.Join(directory, "database.dump"))
	if e != nil {
		return manifest, e
	}
	info, e := dump.Stat()
	if e != nil {
		dump.Close()
		return manifest, e
	}
	header := make([]byte, 5)
	_, readErr := io.ReadFull(dump, header)
	if readErr != nil || string(header) != "PGDMP" {
		dump.Close()
		return manifest, fault("backup_invalid_database", 422)
	}
	if _, e = dump.Seek(0, 0); e != nil {
		dump.Close()
		return manifest, e
	}
	dbHash := sha256.New()
	_, hashErr := io.Copy(dbHash, dump)
	dump.Close()
	if hashErr != nil {
		return manifest, hashErr
	}
	manifest.DatabaseHash = hex.EncodeToString(dbHash.Sum(nil))
	manifest.DatabaseSize = number(info.Size())
	if e != nil || info.Size() == 0 {
		return manifest, fault("backup_incomplete", 409)
	}
	e = s.access.Transact(ctx, func(tx pgx.Tx) error {
		var epoch, mode string
		if e := tx.QueryRow(ctx, `SELECT restore_epoch::text,mode FROM file_protocol WHERE id=1 FOR UPDATE`).Scan(&epoch, &mode); e != nil {
			return e
		}
		if epoch != manifest.Epoch || mode != "read_only" {
			return fault("file_protocol_unavailable", 503)
		}
		actual, registered, e := s.backupManifestTx(ctx, tx, id)
		if e != nil {
			return e
		}
		if actual != "preparing" || registered != taskrecord.Hash(objects) {
			return fault("backup_manifest_changed", 409)
		}
		var incomplete bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM file_backup_items WHERE manifest_id=$1 AND copy_status<>'verified')`, id).Scan(&incomplete); e != nil {
			return e
		}
		if incomplete {
			return fault("backup_incomplete", 409)
		}
		_, e = tx.Exec(ctx, `UPDATE file_backup_manifests SET state='sealed',manifest_hash=$2 WHERE id=$1`, id, manifest.ObjectsHash)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE file_protocol SET mode='online',updated_at=$1 WHERE id=1`, s.now())
		if e != nil {
			return e
		}
		return maintenanceAuditTx(ctx, tx, "backup.sealed", id, reason, map[string]string{"state": "preparing"}, map[string]string{"state": "sealed"})
	})
	if e != nil {
		return manifest, e
	}
	manifest.State = "sealed"
	e = os.WriteFile(filepath.Join(directory, "manifest.json"), taskrecord.JSON(manifest), 0600)
	return manifest, e
}
func (s *Service) backupManifestTx(ctx context.Context, tx pgx.Tx, id string) (string, string, error) {
	var state string
	e := tx.QueryRow(ctx, `SELECT state FROM file_backup_manifests WHERE id=$1`, id).Scan(&state)
	if e != nil {
		return "", "", e
	}
	rows, e := tx.Query(ctx, `SELECT i.space_id::text,i.object_id::text,o.kind,i.size_bytes::text,i.sha256,o.object_key,i.storage_version FROM file_backup_items i JOIN file_objects o ON o.id=i.object_id WHERE manifest_id=$1 ORDER BY object_id`, id)
	if e != nil {
		return "", "", e
	}
	defer rows.Close()
	objects := []BackupObject{}
	for rows.Next() {
		var o BackupObject
		if e = rows.Scan(&o.SpaceID, &o.ID, &o.Kind, &o.Size, &o.SHA256, &o.Key, &o.StorageVersion); e != nil {
			return "", "", e
		}
		objects = append(objects, o)
	}
	return state, taskrecord.Hash(objects), rows.Err()
}
func (s *Service) BackupRelease(ctx context.Context, id, reason string) error {
	if !validText(reason, 1, 500) {
		return fault("invalid_request", 400)
	}
	return s.access.Transact(ctx, func(tx pgx.Tx) error {
		var state string
		e := tx.QueryRow(ctx, `SELECT state FROM file_backup_manifests WHERE id=$1 FOR UPDATE`, id).Scan(&state)
		if e != nil {
			return notFound(e)
		}
		if state == "preparing" {
			return fault("backup_incomplete", 409)
		}
		if state == "released" {
			return nil
		}
		_, e = tx.Exec(ctx, `UPDATE file_backup_manifests SET state='released' WHERE id=$1`, id)
		if e != nil {
			return e
		}
		if state == "failed" {
			_, e = tx.Exec(ctx, `UPDATE file_protocol SET mode='online',updated_at=$1 WHERE id=1 AND mode='read_only' AND NOT EXISTS(SELECT 1 FROM file_backup_manifests WHERE state='preparing')`, s.now())
		}
		if e != nil {
			return e
		}
		return maintenanceAuditTx(ctx, tx, "backup.released", id, reason, map[string]string{"state": state}, map[string]string{"state": "released"})
	})
}

// BeginRestore advances the file epoch once and records interrupted executions
// without producing notification events. Existing receipts and history survive.
func (s *Service) BeginRestore(ctx context.Context, reason string) error {
	return s.access.Transact(ctx, func(tx pgx.Tx) error { return s.BeginRestoreTx(ctx, tx, reason) })
}

// BeginRestoreTx permits the coordinator to fence related protocols atomically.
func (s *Service) BeginRestoreTx(ctx context.Context, tx pgx.Tx, reason string) error {
	if !validText(reason, 1, 500) {
		return fault("invalid_request", 400)
	}
	var mode string
	e := tx.QueryRow(ctx, `SELECT mode FROM file_protocol WHERE id=1 FOR UPDATE`).Scan(&mode)
	if e != nil {
		return e
	}
	if mode == "reconciling" {
		return nil
	}
	_, e = tx.Exec(ctx, `UPDATE file_protocol SET mode='reconciling',restore_epoch=gen_random_uuid(),updated_at=$1 WHERE id=1`, s.now())
	if e != nil {
		return e
	}
	if _, e = access.Lock(ctx, tx, false); e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `UPDATE file_uploads SET state='expired',revision=revision+1,updated_at=$1 WHERE state='receiving'`, s.now())
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `UPDATE file_read_leases SET closed_at=$1,outcome='restore_interrupted' WHERE closed_at IS NULL`, s.now())
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `UPDATE file_budget_reservations SET state='reconciling' WHERE kind IN ('download','local_temp') AND state<>'released'`)
	if e != nil {
		return e
	}

	// Terminal generations remain terminal, but old control commands are
	// invalidated as well. Their optional result references are unchanged.
	terminalRows, err := tx.Query(ctx, `UPDATE file_jobs SET control_revision=control_revision+1,source_revision=source_revision+1,observed_at=$1 WHERE state IN ('failed','succeeded','cancelled','needs_action') RETURNING id::text`, s.now())
	if err != nil {
		return err
	}
	terminalIDs := []string{}
	for terminalRows.Next() {
		var id string
		if err = terminalRows.Scan(&id); err != nil {
			terminalRows.Close()
			return err
		}
		terminalIDs = append(terminalIDs, id)
	}
	err = terminalRows.Err()
	terminalRows.Close()
	if err != nil {
		return err
	}
	for _, id := range terminalIDs {
		if _, err = s.projectJobTx(ctx, tx, id, "", "restore_epoch_changed", reason); err != nil {
			return err
		}
	}
	rows, e := tx.Query(ctx, `SELECT j.id::text,j.generation::text,r.state,r.requested_by::text FROM file_jobs j JOIN file_job_runs r ON r.job_id=j.id AND r.generation=j.generation WHERE r.finished_at IS NULL FOR UPDATE OF j,r`)
	if e != nil {
		return e
	}
	type run struct{ id, generation, state, actor string }
	runs := []run{}
	for rows.Next() {
		var r run
		if e = rows.Scan(&r.id, &r.generation, &r.state, &r.actor); e != nil {
			rows.Close()
			return e
		}
		runs = append(runs, r)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, r := range runs {
		_, e = tx.Exec(ctx, `UPDATE file_job_runs SET fence=fence+1,lease_owner=NULL WHERE job_id=$1 AND generation=$2`, r.id, r.generation)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE file_jobs SET control_revision=control_revision+1 WHERE id=$1`, r.id)
		if e != nil {
			return e
		}
		state := "failed"
		if r.state == "cancelling" {
			state = "cancelled"
		}
		if e = s.finishTx(ctx, tx, r.id, r.generation, state, "restore_interrupted", r.actor, "", "", "", false); e != nil {
			return e
		}
	}
	return access.Audit(ctx, tx, "files.restore_interrupted", "file_protocol", "1", "", nil, map[string]int{"interrupted": len(runs)}, access.Meta{ActorType: "maintenance", Reason: reason})
}

func (s *Service) RestoreObjects(ctx context.Context, directory, reason string) (BackupManifest, error) {
	manifest, objects, e := loadBackup(directory)
	if e != nil {
		return manifest, e
	}
	if manifest.State != "sealed" {
		return manifest, fault("backup_incomplete", 409)
	}
	if s.objects == nil {
		return manifest, fault("file_storage_unavailable", 503)
	}
	if e = s.BeginRestore(ctx, reason); e != nil {
		return manifest, e
	}
	// Validate every binding against the restored metadata before the first Put.
	e = s.access.Transact(ctx, func(tx pgx.Tx) error {
		for _, o := range objects {
			var valid bool
			e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM file_objects WHERE id=$1 AND space_id=$2 AND object_key=$3 AND expected_size=$4 AND expected_sha256=$5 AND kind=$6)`, o.ID, o.SpaceID, o.Key, o.Size, o.SHA256, o.Kind).Scan(&valid)
			if e != nil {
				return e
			}
			if !valid {
				return fault("backup_metadata_mismatch", 409)
			}
		}
		return nil
	})
	if e != nil {
		return manifest, e
	}
	for _, o := range objects {
		size, _ := integer(o.Size, 1)
		current, e := s.objects.Get(ctx, o.Key)
		if e == nil {
			hash := sha256.New()
			n, e := io.Copy(hash, io.LimitReader(current, size+1))
			current.Close()
			if e != nil || n != size || hex.EncodeToString(hash.Sum(nil)) != o.SHA256 {
				return manifest, fault("restore_object_conflict", 409)
			}
			continue
		}
		if !errors.Is(e, fs.ErrNotExist) {
			return manifest, fault("file_storage_unavailable", 503)
		}
		input, e := secureFile(filepath.Join(directory, "objects", o.ID+".bin"))
		if e != nil {
			return manifest, e
		}
		e = s.objects.Put(ctx, o.Key, input, size, "application/octet-stream")
		input.Close()
		if e != nil {
			return manifest, fault("file_storage_unavailable", 503)
		}
	}
	e = s.access.Transact(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `UPDATE file_backup_manifests SET state='sealed',manifest_hash=$2 WHERE id=$1 AND state IN ('preparing','sealed')`, manifest.ID, manifest.ObjectsHash)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE file_backup_items SET copy_status='verified' WHERE manifest_id=$1`, manifest.ID)
		return e
	})
	return manifest, e
}
func (s *Service) RestoreReconcile(ctx context.Context, reason string) (Verification, error) {
	if e := s.BeginRestore(ctx, reason); e != nil {
		return Verification{}, e
	}
	report, e := s.Verify(ctx, "content", true, reason)
	if e != nil || !report.Complete {
		return report, e
	}
	e = s.access.Transact(ctx, func(tx pgx.Tx) error {
		var mode string
		if e := tx.QueryRow(ctx, `SELECT mode FROM file_protocol WHERE id=1 FOR UPDATE`).Scan(&mode); e != nil {
			return e
		}
		if mode != "reconciling" {
			return fault("file_protocol_unavailable", 503)
		}
		_, e := tx.Exec(ctx, `UPDATE file_protocol SET mode='online',last_reconcile_at=$1,updated_at=$1 WHERE id=1`, s.now())
		return e
	})
	return report, e
}
