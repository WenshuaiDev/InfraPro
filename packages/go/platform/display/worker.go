package display

import (
	stdbytes "bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"io"
	"os"
	"time"
)

// ProcessOne only needs the package tables and the display object namespace.
// The standalone entrypoint never opens identity, monitoring or other providers.
func (s *Service) ProcessOne(ctx context.Context) (worked bool, resultErr error) {
	started := time.Now()
	defer func() {
		if worked {
			s.Measure("package_validation", started, resultErr != nil)
		}
	}()
	var job, upload, tenant, project, instance, sha string
	var fence, size int64
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		e := tx.QueryRow(ctx, `SELECT j.id::text,u.id::text,u.tenant_id::text,u.project_id::text,u.instance_id::text,u.sha256,u.size_bytes FROM display_package_jobs j JOIN display_uploads u ON u.id=j.upload_id WHERE u.state='uploaded' AND j.attempt<5 AND (j.state='queued' OR j.state='validating' AND j.lease_until<clock_timestamp()) ORDER BY j.created_at FOR UPDATE OF j SKIP LOCKED LIMIT 1`).Scan(&job, &upload, &tenant, &project, &instance, &sha, &size)
		if e != nil {
			return e
		}
		return tx.QueryRow(ctx, `UPDATE display_package_jobs SET state='validating',attempt=attempt+1,fence=fence+1,lease_until=clock_timestamp()+interval '60 seconds',updated_at=clock_timestamp() WHERE id=$1 RETURNING fence`, job).Scan(&fence)
	})
	if e == pgx.ErrNoRows {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	work, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	defer diagnosticreport.TrackDeadline(ctx, work)()
	pkg, files, err := s.processPackage(work, upload, sha, size)
	if err == nil {
		id := uuid.NewString()
		keys := map[string]string{}
		for _, f := range pkg.Manifest.Files {
			key := "display/sealed/" + project + "/" + id + "/" + f.SHA256
			keys[f.Path] = key
			if err = s.trackObject(work, key); err != nil {
				break
			}
			if err = s.putObject(work, key, files[f.Path], f.MediaType); err != nil {
				break
			}
		}
		if err == nil {
			err = s.access.Transact(work, func(tx pgx.Tx) error {
				var valid bool
				e := tx.QueryRow(work, `SELECT j.state='validating' AND j.fence=$2 AND j.lease_until>clock_timestamp() AND u.state='uploaded' FROM display_package_jobs j JOIN display_uploads u ON u.id=j.upload_id WHERE j.id=$1 FOR UPDATE OF j,u`, job, fence).Scan(&valid)
				if e != nil {
					return e
				}
				if !valid {
					return conflict()
				}
				_, e = tx.Exec(work, `INSERT INTO display_packages(id,tenant_id,project_id,instance_id,job_id,sha256,manifest) VALUES($1,$2,$3,$4,$5,$6,$7)`, id, tenant, project, instance, job, sha, bytesOfManifest(pkg.Manifest))
				if e != nil {
					return e
				}
				for _, f := range pkg.Manifest.Files {
					_, e = tx.Exec(work, `INSERT INTO display_package_files(package_id,path,sha256,size_bytes,media_type,object_key) VALUES($1,$2,$3,$4,$5,$6)`, id, f.Path, f.SHA256, f.Size, f.MediaType, keys[f.Path])
					if e != nil {
						return e
					}
				}
				_, e = tx.Exec(work, `UPDATE display_package_jobs SET state='ready',lease_until=NULL,updated_at=clock_timestamp() WHERE id=$1`, job)
				if e != nil {
					return e
				}
				return eventTx(work, tx, Scope{tenant, project}, instance, "", "package.ready", map[string]string{"package_id": id, "sha256": sha})
			})
		}
	}
	if err != nil {
		code := "package_processing_failed"
		issues := []Issue{}
		var pe *Error
		if errors.As(err, &pe) {
			code = pe.Code
			issues = pe.Issues
		}
		save, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		e = s.access.Transact(save, func(tx pgx.Tx) error {
			tag, e := tx.Exec(save, `UPDATE display_package_jobs SET state='failed',error_code=$3,issues=$4,lease_until=NULL,updated_at=clock_timestamp() WHERE id=$1 AND fence=$2 AND state='validating'`, job, fence, code, jsonBytes(issues))
			if e != nil || tag.RowsAffected() == 0 {
				return e
			}
			return eventTx(save, tx, Scope{tenant, project}, instance, "", "package.failed", map[string]any{"job_id": job, "code": code, "issues": issues})
		})
		if e != nil {
			return true, e
		}
	}
	return true, err
}
func bytesOfManifest(m Manifest) []byte { return jsonBytes(m) }
func (s *Service) processPackage(ctx context.Context, upload, sha string, size int64) (ValidatedPackage, map[string][]byte, error) {
	empty := ValidatedPackage{}
	if s.blobs == nil {
		return empty, nil, problem("dependency_unavailable", "包存储不可用。", 503)
	}
	type part struct {
		key, sha string
		size     int64
	}
	parts := []part{}
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `SELECT object_key,sha256,size_bytes FROM display_upload_parts WHERE upload_id=$1 ORDER BY part`, upload)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var p part
			if e = rows.Scan(&p.key, &p.sha, &p.size); e != nil {
				return e
			}
			parts = append(parts, p)
		}
		return rows.Err()
	})
	if e != nil {
		return empty, nil, e
	}
	f, e := os.CreateTemp("", "infrapro-display-*.zip")
	if e != nil {
		return empty, nil, e
	}
	defer os.Remove(f.Name())
	defer f.Close()
	h := sha256.New()
	var total int64
	for _, p := range parts {
		if e = ctx.Err(); e != nil {
			return empty, nil, e
		}
		r, e := s.blobs.Get(ctx, p.key)
		if e != nil {
			return empty, nil, e
		}
		b, e := io.ReadAll(io.LimitReader(r, p.size+1))
		r.Close()
		if e != nil || int64(len(b)) != p.size || hashBytes(b) != p.sha {
			return empty, nil, invalid("/upload", "分片内容完整性校验失败。")
		}
		n, e := io.Copy(io.MultiWriter(f, h), stdbytes.NewReader(b))
		if e != nil {
			return empty, nil, e
		}
		total += n
	}
	if total != size || hex.EncodeToString(h.Sum(nil)) != sha {
		return empty, nil, invalid("/upload", "整包长度或 SHA-256 校验失败。")
	}
	out, e := ValidateZIP(ctx, f, size)
	return out, out.Files, e
}
func (s *Service) RunWorker(ctx context.Context, report func(error), reporters ...diagnosticreport.Sink) {
	reporter := diagnosticreport.Optional(reporters)
	reporter.Idle("maintain", time.Now().Add(time.Minute))
	reporter.Idle("process", time.Now().Add(time.Second))
	ticker := time.NewTicker(time.Second)
	count := 0
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			count++
			if count%60 == 0 {
				reporter.Begin("maintain", diagnosticreport.Deadline(ctx))
				e := s.Maintain(ctx)
				reporter.End("maintain", diagnosticreport.Result(e), time.Now().Add(time.Minute))
				if e != nil && report != nil {
					report(e)
				}
			}
			for {
				reporter.Begin("process", diagnosticreport.Deadline(ctx))
				worked, e := s.ProcessOne(diagnosticreport.ObserveDeadlines(ctx, reporter, "process"))
				next := time.Now()
				if !worked {
					next = next.Add(time.Second)
				}
				reporter.End("process", diagnosticreport.Result(e), next)
				if e != nil && report != nil {
					report(e)
				}
				if !worked || ctx.Err() != nil {
					break
				}
			}
		}
	}
}
func (s *Service) trackObject(ctx context.Context, key string) error {
	return s.access.Transact(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `INSERT INTO display_object_intents(object_key) VALUES($1) ON CONFLICT DO NOTHING`, key)
		return e
	})
}
