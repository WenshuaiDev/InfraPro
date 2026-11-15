package spatial

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tasks"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

func jobTx(ctx context.Context, tx pgx.Tx, tenant, id string) (Job, error) {
	out := Job{}
	if !access.ValidID(id) {
		return out, notFound()
	}
	e := readJSON(ctx, tx, &out, `SELECT to_jsonb(j)||jsonb_build_object('revision',revision::text,'task_ref',jsonb_build_object('source_module','spatial','source_kind','job','source_root_id',j.id,'generation',j.notification_generation::text)) FROM spatial_processing_jobs j WHERE tenant_id=$1 AND id=$2`, tenant, id)
	if e == nil {
		e = tx.QueryRow(ctx, `SELECT source_blob_id::text,fence,version_revision::text FROM spatial_processing_jobs WHERE id=$1`, id).Scan(&out.SourceBlobID, &out.Fence, &out.VersionRevision)
	}
	return out, e
}
func (s *Service) Job(ctx context.Context, a access.Actor, scope Scope, id string) (Job, error) {
	out := Job{}
	e := s.transact(ctx, a, scope, "jobs.detail", false, func(tx pgx.Tx, auth k.Authorization) error {
		var e error
		out, e = jobTx(ctx, tx, scope.Tenant, id)
		return e
	})
	return out, e
}
func (s *Service) Process(ctx context.Context, a access.Actor, scope Scope, id string, in RevisionInput) (Result, error) {
	out := Result{}
	if s.converter == nil || s.blobs == nil {
		return out, unavailable()
	}
	if e := s.converter.Ready(ctx); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, scope, "versions.process", true, func(tx pgx.Tx, auth k.Authorization) error {
		if e := checkMeta(&in.Meta); e != nil {
			return e
		}
		input := commandInput(id, in)
		var e error
		var replay bool
		out, replay, e = receipt(ctx, tx, auth, in.Meta, "versions.process", input)
		if e != nil || replay {
			return e
		}
		asset, v, e := s.readableTx(ctx, tx, auth, id, false)
		if e != nil {
			return e
		}
		if e = draftable(asset, v); e != nil {
			return e
		}
		if e = k.Expect(in.Revision, v.Revision); e != nil {
			return e
		}
		if v.SourceBlobID == "" || v.ManifestHash != "" {
			return k.Problem("spatial_processing_state", "请先封存来源；成功产物通过复制版本复用。", 409)
		}
		rev, e := bumpVersion(ctx, tx, id)
		if e != nil {
			return e
		}
		e = tx.QueryRow(ctx, `INSERT INTO spatial_processing_jobs(tenant_id,version_id,source_blob_id,actor_id,membership_id,version_revision) VALUES($1,$2,$3,$4,$5,$6) RETURNING id::text,revision::text`, scope.Tenant, id, v.SourceBlobID, a.ID, auth.TenantMembership, rev).Scan(&out.ID, &out.Revision)
		if e != nil {
			return e
		}
		out.VersionID = id
		out.Changed = true
		return complete(ctx, tx, auth, in.Meta, "versions.process", input, out)
	})
	return out, e
}
func (s *Service) ControlJob(ctx context.Context, a access.Actor, scope Scope, id, action string, in RevisionInput, command ...tasks.Command) (Result, error) {
	out := Result{}
	if action == "jobs.retry" {
		if s.converter == nil || s.blobs == nil {
			return out, unavailable()
		}
		if e := s.converter.Ready(ctx); e != nil {
			return out, e
		}
	}
	op := access.SpatialOperation(scope.Entry, action)
	if len(command) > 0 {
		if scope.Entry != "tenant" {
			return out, notFound()
		}
		op = "TasksCommand"
	}
	e := k.Transact(ctx, s.access, a, scope, op, k.Locks{TenantWrite: true, Governance: true}, func(tx pgx.Tx, auth k.Authorization) error {
		if len(command) > 0 {
			if e := auth.Require(access.TenantSpatialManage); e != nil {
				return e
			}
		}
		if e := checkMeta(&in.Meta); e != nil {
			return e
		}
		input := commandInput(id, in)
		if len(command) > 0 {
			input = []any{id, in, command[0]}
		}
		var e error
		var replay bool
		out, replay, e = receipt(ctx, tx, auth, in.Meta, action, input)
		if e != nil || replay {
			return e
		}
		j, e := jobTx(ctx, tx, scope.Tenant, id)
		if e != nil {
			return e
		}
		if len(command) > 0 {
			views, err := s.TaskResolveTx(ctx, tx, a, []taskrecord.SourceRef{{Module: "spatial", Kind: "job", RootID: id}}, "detail")
			if err != nil {
				return err
			}
			if len(views) != 1 {
				return tasks.Hidden()
			}
			if err = tasks.ValidateCommand(command[0], views[0], a); err != nil {
				return err
			}
			if err = tasks.SetCommandTx(ctx, tx, a, command[0], views[0]); err != nil {
				return err
			}
		}
		if len(command) == 0 {
			var fact taskrecord.Change
			if e = tx.QueryRow(ctx, `SELECT snapshot FROM spatial_task_metadata WHERE id=$1`, id).Scan(&fact); e != nil {
				return e
			}
			kind := "cancel"
			if action == "jobs.retry" {
				kind = "retry"
			}
			if e = tasks.SetCommandTx(ctx, tx, a, tasks.Command{RequestID: in.RequestID, Action: kind, Reason: in.Reason}, tasks.Present(fact, "")); e != nil {
				return e
			}
		}
		if e = k.Expect(in.Revision, j.Revision); e != nil {
			return e
		}
		out.ID = id
		out.VersionID = j.VersionID
		if action == "jobs.cancel" {
			if j.State != "queued" && j.State != "running" {
				return k.Problem("job_state", "任务已经结束。", 409)
			}
			e = tx.QueryRow(ctx, `UPDATE spatial_processing_jobs SET state='canceled',stage='canceled',fence=fence+1,lease_owner=NULL,lease_until=NULL,revision=revision+1,updated_at=clock_timestamp() WHERE id=$1 RETURNING revision::text`, id).Scan(&out.Revision)
		} else if action == "jobs.retry" {
			if j.State != "failed" && j.State != "canceled" {
				return k.Problem("job_state", "只能重试失败或取消的任务。", 409)
			}
			asset, v, e := s.readableTx(ctx, tx, auth, j.VersionID, false)
			if e != nil {
				return e
			}
			if e = draftable(asset, v); e != nil {
				return e
			}
			if v.SourceBlobID != j.SourceBlobID || v.ManifestHash != "" {
				return k.Problem("job_obsolete", "来源已变化或已有成功产物，不能重试。", 409)
			}
			rev, e := bumpVersion(ctx, tx, v.ID)
			if e != nil {
				return e
			}
			e = tx.QueryRow(ctx, `UPDATE spatial_processing_jobs SET state='queued',notification_generation=notification_generation+1,stage='queued',attempt=1,fence=fence+1,lease_owner=NULL,lease_until=NULL,error_code='',report='{}',version_revision=$2,actor_id=$3,membership_id=$4,revision=revision+1,updated_at=clock_timestamp() WHERE id=$1 RETURNING revision::text`, id, rev, a.ID, auth.TenantMembership).Scan(&out.Revision)
			if e != nil {
				return e
			}
		} else {
			return invalid("/action", "无效任务操作。")
		}
		if e != nil {
			return e
		}
		out.Changed = true
		if e = complete(ctx, tx, auth, in.Meta, action, input, out); e != nil {
			return e
		}
		return jobNotificationTx(ctx, tx, id)
	})
	return out, e
}

// WorkOne claims one fenced lease. Parsing and object IO never hold a database transaction.
func (s *Service) WorkOne(ctx context.Context, worker string) (bool, error) {
	if s.converter == nil || s.blobs == nil {
		return false, unavailable()
	}
	job := Job{}
	var actorID, membership string
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		var id, tenant string
		e := tx.QueryRow(ctx, `SELECT id::text,tenant_id::text FROM spatial_processing_jobs WHERE state='queued' OR (state='running' AND lease_until<clock_timestamp()) ORDER BY created_at,id FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&id, &tenant)
		if e == pgx.ErrNoRows {
			return nil
		}
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE spatial_processing_jobs SET state='running',stage='validating',attempt=attempt+CASE WHEN state='running' THEN 1 ELSE 0 END,fence=fence+1,lease_owner=$2,lease_until=clock_timestamp()+interval '30 seconds',revision=revision+1,updated_at=clock_timestamp() WHERE id=$1`, id, worker)
		if e != nil {
			return e
		}
		job, e = jobTx(ctx, tx, tenant, id)
		if e != nil {
			return e
		}
		return tx.QueryRow(ctx, `SELECT actor_id::text,membership_id::text FROM spatial_processing_jobs WHERE id=$1`, id).Scan(&actorID, &membership)
	})
	if e != nil || job.ID == "" {
		return false, e
	}
	work, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-work.Done():
				return
			case <-ticker.C:
				e := s.access.Transact(work, func(tx pgx.Tx) error {
					tag, e := tx.Exec(work, `UPDATE spatial_processing_jobs SET lease_until=clock_timestamp()+interval '30 seconds' WHERE id=$1 AND state='running' AND fence=$2 AND lease_owner=$3 AND lease_until>clock_timestamp()`, job.ID, job.Fence, worker)
					if e == nil && tag.RowsAffected() != 1 {
						return errors.New("lease lost")
					}
					return e
				})
				if e != nil {
					cancel()
					return
				}
			}
		}
	}()
	e = s.executeJob(work, job, actorID, membership, worker)
	cancel()
	<-done
	if e != nil {
		failure := &Error{Code: "processing_failed", Message: "处理未完成，请重试或核对来源文件。", Status: 422}
		var se *Error
		if errors.As(e, &se) {
			failure = se
		}
		if errors.Is(e, context.Canceled) {
			failure = &Error{Code: "processing_canceled", Message: "任务已取消或租约失效。", Status: 422}
		}
		cleanup, cancelCleanup := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancelCleanup()
		_ = s.access.Transact(cleanup, func(tx pgx.Tx) error {
			tag, e := tx.Exec(cleanup, `UPDATE spatial_processing_jobs SET state='failed',stage='failed',error_code=$3,report=$4,lease_owner=NULL,lease_until=NULL,revision=revision+1,updated_at=clock_timestamp() WHERE id=$1 AND state='running' AND fence=$2`, job.ID, job.Fence, failure.Code, k.JSON(map[string]string{"message": failure.Message}))
			if e != nil || tag.RowsAffected() == 0 {
				return e
			}
			return jobNotificationTx(cleanup, tx, job.ID)
		})
	}
	return true, e
}
func workerActor(id string) access.Actor {
	return access.Actor{ID: id, Check: func(ctx context.Context, tx pgx.Tx, recent bool) error {
		var active bool
		if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM accounts WHERE id=$1 AND status='active')`, id).Scan(&active); e != nil {
			return e
		}
		if !active {
			return access.ErrDenied
		}
		return nil
	}}
}
func (s *Service) executeJob(ctx context.Context, j Job, actorID, membership, worker string) error {
	if j.Attempt > 5 {
		return kProblem("attempt_limit", "处理租约已重试五次，请人工核对后重试。")
	}
	a := workerActor(actorID)
	scope := Tenant(j.TenantID)
	v := Version{}
	source := File{}
	var key string
	e := s.transact(ctx, a, scope, "versions.process", false, func(tx pgx.Tx, auth k.Authorization) error {
		if auth.TenantMembership != membership {
			return access.ErrDenied
		}
		asset, version, e := s.readableTx(ctx, tx, auth, j.VersionID, false)
		if e != nil {
			return e
		}
		v = version
		if e = draftable(asset, v); e != nil {
			return e
		}
		if v.Revision != j.VersionRevision || v.SourceBlobID != j.SourceBlobID {
			return k.Problem("job_obsolete", "版本或来源已变化。", 409)
		}
		return tx.QueryRow(ctx, `SELECT object_key,sha256,size,name,media_type FROM spatial_blobs WHERE tenant_id=$1 AND id=$2`, scope.Tenant, j.SourceBlobID).Scan(&key, &source.SHA256, &source.Size, &source.Name, &source.MediaType)
	})
	if e != nil {
		return e
	}
	dir, e := os.MkdirTemp(s.temp, "spatial-job-*")
	if e != nil {
		return e
	}
	defer os.RemoveAll(dir)
	input := filepath.Join(dir, "input."+v.Format)
	f, e := os.OpenFile(input, os.O_CREATE|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	r, e := s.blobs.Get(ctx, key)
	if e != nil {
		f.Close()
		return unavailable()
	}
	h := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(f, h), io.LimitReader(r, source.Size+1))
	r.Close()
	f.Close()
	if copyErr != nil || n != source.Size || hex.EncodeToString(h.Sum(nil)) != source.SHA256 {
		return kProblem("source_integrity", "封存来源的大小或哈希校验失败。")
	}
	outDir := filepath.Join(dir, "output")
	if e = os.Mkdir(outDir, 0700); e != nil {
		return e
	}
	manifest, e := s.converter.Convert(ctx, v.Format, input, outDir)
	if e != nil {
		return e
	}
	if manifest.Contract != "infrapro.spatial/1" || manifest.SourceSHA256 != source.SHA256 || manifest.Format != v.Format || len(manifest.Files) == 0 || len(manifest.Files) > 1000 {
		return kProblem("manifest_invalid", "来源与处理清单不一致。")
	}
	type stored struct {
		File
		Key, ID string
	}
	files := []stored{}
	attached := false
	defer func() {
		if !attached {
			for _, f := range files {
				_ = s.blobs.Delete(context.WithoutCancel(ctx), f.Key)
			}
		}
	}()
	seen := map[string]bool{}
	var total int64
	for _, entry := range manifest.Files {
		if entry.ID != entry.Name || entry.ID == "" || filepath.Base(entry.Name) != entry.Name || seen[entry.ID] || entry.Size < 1 || entry.Size > 1024*1024*1024 || !shaPattern.MatchString(entry.SHA256) {
			return kProblem("manifest_invalid", "处理清单含非法文件引用。")
		}
		seen[entry.ID] = true
		total += entry.Size
		if total > 1024*1024*1024 {
			return kProblem("output_limit", "展开产物超过 1 GiB。")
		}
		path := filepath.Join(outDir, entry.Name)
		info, e := os.Lstat(path)
		if e != nil || !info.Mode().IsRegular() || info.Size() != entry.Size {
			return kProblem("output_integrity", "浏览产物大小或类型不一致。")
		}
		f, e := os.Open(path)
		if e != nil {
			return e
		}
		h := sha256.New()
		_, e = io.Copy(h, f)
		if e != nil || hex.EncodeToString(h.Sum(nil)) != entry.SHA256 {
			f.Close()
			return kProblem("output_integrity", "浏览产物哈希不一致。")
		}
		f.Seek(0, 0)
		blobID := uuid.NewString()
		key := fmt.Sprintf("spatial/%s/products/%s/%d/%s", scope.Tenant, j.ID, j.Fence, blobID)
		e = s.blobs.Put(ctx, key, f, entry.Size, entry.MediaType)
		f.Close()
		if e != nil {
			return unavailable()
		}
		files = append(files, stored{entry, key, blobID})
	}
	e = s.transact(ctx, a, scope, "versions.process", true, func(tx pgx.Tx, auth k.Authorization) error {
		if auth.TenantMembership != membership {
			return access.ErrDenied
		}
		asset, current, e := s.readableTx(ctx, tx, auth, v.ID, false)
		if e != nil {
			return e
		}
		if e = draftable(asset, current); e != nil {
			return e
		}
		if current.Revision != j.VersionRevision || current.SourceBlobID != j.SourceBlobID {
			return k.Problem("job_obsolete", "版本已变化，旧任务结果不会附着。", 409)
		}
		var live bool
		e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM spatial_processing_jobs WHERE id=$1 AND state='running' AND fence=$2 AND lease_owner=$3 AND lease_until>clock_timestamp())`, j.ID, j.Fence, worker).Scan(&live)
		if e != nil {
			return e
		}
		if !live {
			return k.Problem("job_fenced", "任务租约失效，旧结果已丢弃。", 409)
		}
		for _, f := range files {
			_, e = tx.Exec(ctx, `INSERT INTO spatial_blobs(id,tenant_id,object_key,sha256,size,media_type,name) VALUES($1,$2,$3,$4,$5,$6,$7)`, f.ID, scope.Tenant, f.Key, f.SHA256, f.Size, f.MediaType, f.Name)
			if e != nil {
				return e
			}
			_, e = tx.Exec(ctx, `INSERT INTO spatial_version_files(tenant_id,version_id,file_id,blob_id) VALUES($1,$2,$3,$4)`, scope.Tenant, v.ID, f.File.ID, f.ID)
			if e != nil {
				return e
			}
		}
		candidates := current.Candidates
		if current.BaseVersionID != "" {
			base, e := versionTx(ctx, tx, scope.Tenant, current.BaseVersionID)
			if e != nil {
				return e
			}
			candidates = classifyCandidates(candidates, base.Manifest, manifest)
		}
		_, e = tx.Exec(ctx, `UPDATE spatial_asset_versions SET manifest=$2,manifest_hash=$3,candidates=$4,revision=revision+1 WHERE id=$1`, v.ID, k.JSON(manifest), k.Hash(manifest), k.JSON(candidates))
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE spatial_processing_jobs SET state='succeeded',stage='sealed',report=$3,lease_owner=NULL,lease_until=NULL,revision=revision+1,updated_at=clock_timestamp() WHERE id=$1 AND fence=$2`, j.ID, j.Fence, k.JSON(map[string]any{"files": len(files), "elements": len(manifest.Elements), "pages": len(manifest.Pages), "manifest_hash": k.Hash(manifest), "attempt": j.Attempt}))
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO spatial_changes(tenant_id,actor_id,operation,target_id,details) VALUES($1,$2,'processing.sealed',$3,$4)`, scope.Tenant, actorID, v.ID, k.JSON(map[string]string{"job_id": j.ID, "fence": strconv.FormatInt(j.Fence, 10), "manifest_hash": k.Hash(manifest)}))
		if e != nil {
			return e
		}
		return jobNotificationTx(ctx, tx, j.ID)
	})
	attached = e == nil
	return e
}
func classifyCandidates(candidates []Candidate, old, new Manifest) []Candidate {
	before := map[string]Element{}
	after := map[string]Element{}
	for _, e := range old.Elements {
		before[e.Key] = e
	}
	for _, e := range new.Elements {
		after[e.Key] = e
	}
	for i, c := range candidates {
		key := ""
		if c.Placement != nil {
			key = c.Placement.ElementKey
		}
		if c.ComponentLink != nil {
			key = c.ComponentLink.ElementKey
		}
		candidates[i].State = "needs_review"
		candidates[i].Reason = "来源图页、裁剪或方向变化；旧位置仅供参考，须明确重新定位。"
		if key != "" {
			next, found := after[key]
			prior := before[key]
			if !found {
				candidates[i].State = "unmatched"
				candidates[i].Reason = "原元素不存在；同名元素不能视为同一身份。"
			} else if next.Properties["source_identity"] != true {
				candidates[i].State = "uncertain"
				candidates[i].Reason = "仅有版本内节点索引，不能证明跨版本身份。"
			} else if next.GeometryHash != prior.GeometryHash {
				candidates[i].Reason = "源 ID 延续，但几何或变换已变化；须核对锚点。"
			} else {
				candidates[i].Reason = "源 ID 与几何一致，可核对后采用；尚未自动生成标记。"
			}
		}
	}
	return candidates
}
func (s *Service) RunWorker(ctx context.Context, worker string, onError func(error), reporters ...diagnosticreport.Sink) {
	reporter := diagnosticreport.Optional(reporters)
	for {
		reporter.Begin("convert", diagnosticreport.Deadline(ctx))
		worked, e := s.WorkOne(diagnosticreport.ObserveDeadlines(ctx, reporter, "convert"), worker)
		next := time.Now()
		if !worked || e != nil {
			next = next.Add(time.Second)
		}
		reporter.End("convert", diagnosticreport.Result(e), next)
		if e != nil && ctx.Err() == nil && onError != nil {
			onError(e)
		}
		if ctx.Err() != nil {
			return
		}
		if !worked || e != nil {
			timer := time.NewTimer(time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}
}
