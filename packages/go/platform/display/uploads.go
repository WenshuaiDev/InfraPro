package display

import (
	stdbytes "bytes"
	"context"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
	"io"
	"strconv"
	"time"
)

func (s *Service) createUploadTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, v Instance, in Command, out map[string]any) error {
	if s.blobs == nil {
		return problem("dependency_unavailable", "包存储尚未配置。", 503)
	}
	if in.Size < 1 || in.Size > s.options.MaxPackageBytes || !digestPattern.MatchString(in.SHA256) {
		return invalid("/upload", "需要有效 ZIP 长度和 SHA-256。")
	}
	var count int
	e := tx.QueryRow(ctx, `SELECT count(*) FROM display_uploads WHERE tenant_id=$1 AND state='receiving' AND expires_at>clock_timestamp()`, scope.Tenant).Scan(&count)
	if e != nil {
		return e
	}
	if count >= s.options.MaxPackageJobs {
		return problem("budget_exceeded", "待处理上传过多，请完成或取消旧上传。", 429)
	}
	var id string
	e = tx.QueryRow(ctx, `INSERT INTO display_uploads(tenant_id,project_id,instance_id,actor_id,size_bytes,sha256) VALUES($1,$2,$3,$4,$5,$6) RETURNING id::text`, scope.Tenant, scope.Project, v.ID, a.ID, in.Size, in.SHA256).Scan(&id)
	if e != nil {
		return e
	}
	out["upload_id"] = id
	out["part_size"] = strconv.FormatInt(PartSize, 10)
	return nil
}
func (s *Service) Upload(ctx context.Context, a access.Actor, scope Scope, id, upload string) (map[string]any, error) {
	out := map[string]any{}
	e := s.transact(ctx, a, scope, "upload.get", func(tx pgx.Tx, _ access.ProjectAuthorization) error {
		if !access.ValidID(upload) {
			return hidden()
		}
		var size int64
		var state, sha string
		var expires time.Time
		e := tx.QueryRow(ctx, `SELECT size_bytes,state,sha256,expires_at FROM display_uploads WHERE tenant_id=$1 AND project_id=$2 AND instance_id=$3 AND id=$4`, scope.Tenant, scope.Project, id, upload).Scan(&size, &state, &sha, &expires)
		if e != nil {
			return hidden()
		}
		parts, e := jsonRows(ctx, tx, `SELECT jsonb_build_object('part',part,'size',size_bytes::text,'sha256',sha256) FROM display_upload_parts WHERE upload_id=$1 ORDER BY part`, upload)
		if e != nil {
			return e
		}
		jobs, e := jsonRows(ctx, tx, `SELECT jsonb_build_object('id',id,'state',state,'error_code',error_code,'issues',issues,'attempt',attempt) FROM display_package_jobs WHERE upload_id=$1`, upload)
		if e != nil {
			return e
		}
		out = map[string]any{"id": upload, "size": strconv.FormatInt(size, 10), "state": state, "sha256": sha, "expires_at": expires, "parts": parts, "jobs": jobs}
		return nil
	})
	return out, e
}
func (s *Service) UploadPart(ctx context.Context, a access.Actor, scope Scope, id, upload string, part int, sha string, data []byte) error {
	if s.blobs == nil {
		return problem("dependency_unavailable", "包存储暂不可用。", 503)
	}
	if part < 0 || int64(part) > 2147483647 || len(data) < 1 || len(data) > int(PartSize) || hashBytes(data) != sha {
		return invalid("/part", "分片编号、大小或摘要不正确。")
	}
	if !access.ValidID(upload) || !access.ValidID(scope.Project) {
		return hidden()
	}
	key := fmt.Sprintf("display/uploads/%s/%s/%d-%s", scope.Project, upload, part, sha)
	if e := s.trackObject(ctx, key); e != nil {
		return e
	}
	return s.transact(ctx, a, scope, "upload.part", func(tx pgx.Tx, _ access.ProjectAuthorization) error {
		if !access.ValidID(upload) {
			return hidden()
		}
		var size int64
		var state string
		var expires time.Time
		e := tx.QueryRow(ctx, `SELECT size_bytes,state,expires_at FROM display_uploads WHERE tenant_id=$1 AND project_id=$2 AND instance_id=$3 AND id=$4 FOR UPDATE`, scope.Tenant, scope.Project, id, upload).Scan(&size, &state, &expires)
		if e != nil {
			return hidden()
		}
		if state != "receiving" || expires.Before(time.Now()) {
			return conflict()
		}
		if int64(part) >= (size-1)/PartSize+1 {
			return invalid("/part", "分片编号超出整包范围。")
		}
		expected := min(PartSize, size-int64(part)*PartSize)
		if int64(len(data)) != expected {
			return invalid("/part", "分片长度不符合整包长度。")
		}
		var prior string
		e = tx.QueryRow(ctx, `SELECT sha256 FROM display_upload_parts WHERE upload_id=$1 AND part=$2`, upload, part).Scan(&prior)
		if e == nil {
			if prior != sha {
				return conflict()
			}
			return nil
		}
		if e != pgx.ErrNoRows {
			return e
		}
		// An object intent survives process crashes; maintenance reclaims unreferenced
		// objects only after the bounded upload / worker lease has elapsed.
		if e = s.putObject(ctx, key, data, "application/octet-stream"); e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO display_upload_parts(upload_id,part,size_bytes,sha256,object_key) VALUES($1,$2,$3,$4,$5)`, upload, part, len(data), sha, key)
		return e
	})
}
func (s *Service) putObject(ctx context.Context, key string, b []byte, media string) error {
	// Do not start a nested metadata transaction while governance is locked.
	e := s.blobs.Put(ctx, key, stdbytes.NewReader(b), int64(len(b)), media)
	if e == nil {
		return nil
	}
	// An uncertain immutable PUT can be retried safely only after byte verification.
	r, re := s.blobs.Get(ctx, key)
	if re != nil {
		return problem("dependency_unavailable", "私有对象写入失败。", 503)
	}
	defer r.Close()
	existing, re := io.ReadAll(io.LimitReader(r, int64(len(b))+1))
	if re != nil || hashBytes(existing) != hashBytes(b) {
		return problem("dependency_unavailable", "私有对象完整性校验失败。", 503)
	}
	return nil
}
func (s *Service) uploadCommandTx(ctx context.Context, tx pgx.Tx, scope Scope, id, upload, action string, out map[string]any) error {
	if !access.ValidID(upload) {
		return hidden()
	}
	var state string
	var size int64
	var expires time.Time
	e := tx.QueryRow(ctx, `SELECT state,size_bytes,expires_at FROM display_uploads WHERE tenant_id=$1 AND project_id=$2 AND instance_id=$3 AND id=$4 FOR UPDATE`, scope.Tenant, scope.Project, id, upload).Scan(&state, &size, &expires)
	if e != nil {
		return hidden()
	}
	switch action {
	case "upload.cancel":
		if state == "canceled" {
			return nil
		}
		var ready bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM display_package_jobs WHERE upload_id=$1 AND state='ready')`, upload).Scan(&ready); e != nil {
			return e
		}
		if ready {
			return conflict()
		}
		_, e = tx.Exec(ctx, `UPDATE display_uploads SET state='canceled' WHERE id=$1`, upload)
		if e != nil {
			return e
		}
		var job string
		e = tx.QueryRow(ctx, `UPDATE display_package_jobs SET state='canceled',fence=fence+1,lease_until=NULL WHERE upload_id=$1 RETURNING id::text`, upload).Scan(&job)
		if e == pgx.ErrNoRows {
			return nil
		}
		if e != nil {
			return e
		}
		return eventTx(ctx, tx, scope, id, "", "package.canceled", map[string]string{"job_id": job})
	case "package.retry":
		if state != "uploaded" {
			return conflict()
		}
		tag, e := tx.Exec(ctx, `UPDATE display_package_jobs SET state='queued',execution_generation=execution_generation+1,fence=fence+1,lease_until=NULL,error_code='',issues='[]' WHERE upload_id=$1 AND state='failed' AND attempt<5`, upload)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return conflict()
		}
		return nil
	case "upload.complete":
		if state != "receiving" || expires.Before(time.Now()) {
			return conflict()
		}
		var count int
		var total int64
		e = tx.QueryRow(ctx, `SELECT count(*),COALESCE(sum(size_bytes),0)::bigint FROM display_upload_parts WHERE upload_id=$1`, upload).Scan(&count, &total)
		if e != nil {
			return e
		}
		if count != int((size+PartSize-1)/PartSize) || total != size {
			return invalid("/upload", "分片尚未全部上传。")
		}
		var job string
		e = tx.QueryRow(ctx, `INSERT INTO display_package_jobs(upload_id) VALUES($1) RETURNING id::text`, upload).Scan(&job)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE display_uploads SET state='uploaded' WHERE id=$1`, upload)
		out["job_id"] = job
		ref, err := TaskReferenceTx(ctx, tx, job)
		if err != nil {
			return err
		}
		out["task_ref"] = ref
		return e
	}
	return invalid("/action", "不支持的上传操作。")
}
func (s *Service) deletePackageTx(ctx context.Context, tx pgx.Tx, id, pkg string) error {
	if !access.ValidID(pkg) {
		return hidden()
	}
	var referenced bool
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM display_configs WHERE instance_id=$1 AND package_id=$2) OR EXISTS(SELECT 1 FROM display_sessions WHERE instance_id=$1 AND package_id=$2 AND state IN ('starting','ready','degraded') AND lease_until>clock_timestamp())`, id, pkg).Scan(&referenced)
	if e != nil {
		return e
	}
	if referenced {
		return problem("reference_conflict", "包仍被配置、发布或运行会话引用。", 409)
	}
	tag, e := tx.Exec(ctx, `UPDATE display_packages SET state='reclaiming' WHERE instance_id=$1 AND id=$2`, id, pkg)
	if e == nil && tag.RowsAffected() != 1 {
		return hidden()
	}
	return e
}
