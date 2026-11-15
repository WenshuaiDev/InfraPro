package display

import (
	"context"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
	"io"
	"time"
)

type BackupFile struct {
	PackageID string `json:"package_id"`
	Path      string `json:"path"`
	ObjectKey string `json:"object_key"`
	SHA256    string `json:"sha256"`
	Size      int64  `json:"size_bytes"`
	State     string `json:"state"`
}
type BackupManifest struct {
	Environment string       `json:"environment"`
	Files       []BackupFile `json:"files"`
}

// BackupManifest is inventory, not a substitute for a transaction-consistent PG
// backup plus private object storage backup. The two must share this cut point.
func (s *Service) BackupManifest(ctx context.Context) (BackupManifest, error) {
	out := BackupManifest{Files: []BackupFile{}}
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, true); e != nil {
			return e
		}
		if e := tx.QueryRow(ctx, `SELECT generation::text FROM display_environment WHERE id=1`).Scan(&out.Environment); e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT f.package_id::text,f.path,f.object_key,f.sha256,f.size_bytes,p.state FROM display_package_files f JOIN display_packages p ON p.id=f.package_id WHERE p.state<>'reclaiming' ORDER BY f.package_id,f.path`)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var v BackupFile
			if e = rows.Scan(&v.PackageID, &v.Path, &v.ObjectKey, &v.SHA256, &v.Size, &v.State); e != nil {
				return e
			}
			out.Files = append(out.Files, v)
		}
		return rows.Err()
	})
	return out, e
}
func (s *Service) VerifyStorage(ctx context.Context) ([]BackupFile, error) {
	inventory, e := s.BackupManifest(ctx)
	if e != nil {
		return nil, e
	}
	if s.blobs == nil {
		return nil, problem("dependency_unavailable", "包存储未配置。", 503)
	}
	missing := []BackupFile{}
	validPackages := map[string]bool{}
	for _, f := range inventory.Files {
		validPackages[f.PackageID] = true
	}
	for _, f := range inventory.Files {
		r, e := s.blobs.Get(ctx, f.ObjectKey)
		valid := false
		if e == nil {
			b, re := io.ReadAll(io.LimitReader(r, f.Size+1))
			r.Close()
			valid = re == nil && int64(len(b)) == f.Size && hashBytes(b) == f.SHA256
		}
		if !valid {
			validPackages[f.PackageID] = false
			missing = append(missing, f)
			e = s.access.Transact(ctx, func(tx pgx.Tx) error {
				return PackageAvailabilityTx(ctx, tx, f.PackageID, true)
			})
			if e != nil {
				return missing, e
			}
		}
	}
	// Restore availability only after every immutable file of the package was
	// verified. A transient outage never silently replaces its contents.
	for id, valid := range validPackages {
		if !valid {
			continue
		}
		if e = s.access.Transact(ctx, func(tx pgx.Tx) error {
			return PackageAvailabilityTx(ctx, tx, id, false)
		}); e != nil {
			return missing, e
		}
	}
	return missing, nil
}
func (s *Service) InvalidateAfterRestore(ctx context.Context) (string, error) {
	var generation string
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, true); e != nil {
			return e
		}
		if e := tx.QueryRow(ctx, `UPDATE display_environment SET generation=gen_random_uuid() WHERE id=1 RETURNING generation::text`).Scan(&generation); e != nil {
			return e
		}
		for _, q := range []string{`UPDATE display_sessions SET state='revoked',lease_until=clock_timestamp(),last_error='environment_restored'`, `UPDATE display_subscriptions SET state='canceled',generation=generation+1`, `UPDATE display_terminals SET state='unpaired',generation=generation+1,token_hash=NULL,pair_hash=NULL,pair_expires_at=NULL,actual_release_id=NULL,environment=(SELECT generation FROM display_environment WHERE id=1)`} {
			if _, e := tx.Exec(ctx, q); e != nil {
				return e
			}
		}
		return nil
	})
	return generation, e
}

// Maintenance only reclaims durable intents older than any upload/job lease.
// Immutable package metadata remains as a tombstone for operation history.
func (s *Service) Maintain(ctx context.Context) error {
	if s.blobs == nil {
		return nil
	}
	keys := []string{}
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		// A worker can die during its final attempt. Expired fifth leases must
		// become visible failures instead of remaining validating indefinitely.
		if _, e := tx.Exec(ctx, `WITH failed AS (UPDATE display_package_jobs SET state='failed',error_code='attempts_exhausted',lease_until=NULL,updated_at=clock_timestamp() WHERE state='validating' AND attempt>=5 AND lease_until<=clock_timestamp() RETURNING id,upload_id), events AS (INSERT INTO display_events(tenant_id,project_id,instance_id,kind,details) SELECT u.tenant_id,u.project_id,u.instance_id,'package.failed',jsonb_build_object('job_id',f.id::text,'code','attempts_exhausted') FROM failed f JOIN display_uploads u ON u.id=f.upload_id RETURNING *) INSERT INTO display_notification_handoffs(event_id,tenant_id,project_id,instance_id,kind,target_id,outcome,recipients) SELECT e.id,e.tenant_id,e.project_id,e.instance_id,'package',f.id,'failed',ARRAY[u.actor_id] FROM events e JOIN failed f ON f.id::text=e.details->>'job_id' JOIN display_uploads u ON u.id=f.upload_id`); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `UPDATE display_uploads SET state='expired' WHERE state='receiving' AND expires_at<=clock_timestamp()`); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `UPDATE display_sessions SET state='expired' WHERE state IN ('starting','ready','degraded') AND lease_until<=clock_timestamp()`); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `UPDATE display_subscriptions SET state='canceled' WHERE state IN ('active','invalid') AND session_id IN (SELECT id FROM display_sessions WHERE state IN ('expired','revoked','closed'))`); e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT i.object_key FROM display_object_intents i WHERE i.expires_at<clock_timestamp() AND NOT EXISTS(SELECT 1 FROM display_package_files f JOIN display_packages p ON p.id=f.package_id WHERE f.object_key=i.object_key AND p.state<>'reclaiming') AND NOT EXISTS(SELECT 1 FROM display_upload_parts p JOIN display_uploads u ON u.id=p.upload_id LEFT JOIN display_package_jobs j ON j.upload_id=u.id WHERE p.object_key=i.object_key AND (u.state='receiving' AND u.expires_at>clock_timestamp() OR j.state IN ('queued','validating','failed') AND u.created_at>clock_timestamp()-interval '7 days')) ORDER BY i.expires_at LIMIT 100`)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var key string
			if e = rows.Scan(&key); e != nil {
				return e
			}
			keys = append(keys, key)
		}
		return rows.Err()
	})
	if e != nil {
		return e
	}
	for _, key := range keys {
		if e = s.blobs.Delete(ctx, key); e != nil {
			return e
		}
		if e = s.access.Transact(ctx, func(tx pgx.Tx) error {
			_, e := tx.Exec(ctx, `DELETE FROM display_object_intents WHERE object_key=$1`, key)
			return e
		}); e != nil {
			return e
		}
	}
	return s.access.Transact(ctx, func(tx pgx.Tx) error {
		for _, q := range []string{`DELETE FROM display_pairings WHERE expires_at<clock_timestamp()-interval '1 day'`, `DELETE FROM display_tool_sessions WHERE expires_at<clock_timestamp()-interval '1 day'`, `DELETE FROM display_subscription_events WHERE created_at<clock_timestamp()-interval '10 minutes'`} {
			if _, e := tx.Exec(ctx, q); e != nil {
				return e
			}
		}
		retention := s.options.EventRetention
		if retention <= 0 {
			retention = 90 * 24 * time.Hour
		}
		for _, table := range []string{"display_events", "display_receipts"} {
			if _, e := tx.Exec(ctx, "DELETE FROM "+table+" WHERE created_at<clock_timestamp()-$1::interval"+func() string {
				if table == "display_events" {
					return " AND NOT EXISTS(SELECT 1 FROM display_notification_handoffs h WHERE h.event_id=display_events.id)"
				}
				return ""
			}(), fmt.Sprintf("%d seconds", int64(retention.Seconds()))); e != nil {
				return e
			}
		}
		return nil
	})
}
