package monitoring

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"io"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const batchSelect = `SELECT id::text,state,revision::text,mapping_revision::text,original_name,expected_size::text,COALESCE(source_file_id::text,''),created_by::text,created_at,updated_at,counts,COALESCE(validation_hash,''),COALESCE(error_code,''),COALESCE(readopt_batch_id::text,'') FROM monitoring_import_batches`

func scanBatch(row pgx.Row) (ImportBatch, error) {
	b := ImportBatch{Parts: []UploadPart{}, Counts: &ImportCounts{"0", "0", "0", "0", "0", "0", "0", "0", "0"}}
	var counts []byte
	e := row.Scan(&b.ID, &b.State, &b.Revision, &b.MappingRevision, &b.Name, &b.Size, &b.SourceFile, &b.Creator, &b.CreatedAt, &b.UpdatedAt, &counts, &b.ValidationHash, &b.ErrorCode, &b.ReadoptBatch)
	if e == pgx.ErrNoRows {
		return b, notFound()
	}
	if e == nil {
		e = json.Unmarshal(counts, &b.Counts)
	}
	return b, e
}
func batchTx(ctx context.Context, tx pgx.Tx, auth authorization, id string, lock bool) (ImportBatch, error) {
	if !access.ValidID(id) {
		return ImportBatch{}, notFound()
	}
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}
	b, e := scanBatch(tx.QueryRow(ctx, batchSelect+` WHERE tenant_id=$1 AND id=$2 AND ($3='' OR project_id::text=$3 OR state IN ('published','revoked'))`+suffix, auth.Tenant, id, auth.Project))
	if e != nil {
		return b, e
	}
	if e = batchVisibilityTx(ctx, tx, auth, b); e != nil {
		return b, e
	}
	// Do not expose a tenant-owned job through a clipped project batch.
	j, e := scanJob(tx.QueryRow(ctx, jobSelect+` WHERE tenant_id=$1 AND target_id=$2 AND kind IN ('validate','publish') AND project_id IS NOT DISTINCT FROM $3::uuid ORDER BY created_at DESC,id DESC LIMIT 1`, auth.Tenant, id, nullable(auth.Project)))
	if e == nil {
		b.JobID = j.ID
		b.TaskRef = j.TaskRef
	} else if !taskHidden(e) {
		return b, e
	}
	return b, nil
}
func batchVisibilityTx(ctx context.Context, tx pgx.Tx, auth authorization, b ImportBatch) error {
	if auth.Entry != "project" {
		return nil
	}
	if b.MappingRevision == "0" {
		if b.Creator != auth.Account {
			return notFound()
		}
		return nil
	}
	var raw, snapshot []byte
	e := tx.QueryRow(ctx, `SELECT m.mapping,b.semantic_snapshot FROM monitoring_import_batches b JOIN monitoring_import_mappings m ON m.tenant_id=b.tenant_id AND m.batch_id=b.id AND m.revision=b.mapping_revision WHERE b.tenant_id=$1 AND b.id=$2`, auth.Tenant, b.ID).Scan(&raw, &snapshot)
	if e != nil {
		return e
	}
	var mapping Mapping
	if e = json.Unmarshal(raw, &mapping); e != nil {
		return e
	}
	capability := "import"
	if b.State == "published" || b.State == "revoked" || b.State == "completed_noop" {
		capability = "read"
	}
	var prior validationSnapshot
	if e = json.Unmarshal(snapshot, &prior); e != nil {
		return e
	}
	ids := channelsOf(mapping)
	if b.State == "published" || b.State == "revoked" {
		ids = actualChannels(prior)
	}
	windows, e := authorizedWindowsTx(ctx, tx, auth, ids, capability)
	if e != nil {
		return e
	}
	if b.State == "published" || b.State == "revoked" {
		return coversBatch(prior.ActualCoverage, windows)
	}
	for id, old := range prior.Windows {
		for _, w := range old {
			covered := false
			for _, current := range windows[id] {
				if current.Covers(w) {
					covered = true
					break
				}
			}
			if !covered {
				return notFound()
			}
		}
	}
	return nil
}
func importEditable(b ImportBatch) bool {
	return slices.Contains([]string{"received", "needs_resolution", "ready", "failed"}, b.State)
}
func channelsOf(m Mapping) []string {
	ids := []string{}
	for _, c := range m.Channels {
		ids = append(ids, c.Channel)
	}
	slices.Sort(ids)
	return slices.Compact(ids)
}
func auditTx(ctx context.Context, tx pgx.Tx, auth authorization, op, id, reason, trace string, summary any) error {
	if v := TraceID(ctx); v != "" {
		trace = v
	}
	b, e := json.Marshal(summary)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `INSERT INTO monitoring_audit(tenant_id,project_id,actor_id,operation,target_id,reason,trace_id,summary) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, auth.Tenant, nullable(auth.Project), auth.Account, op, id, reason, trace, b)
	return e
}

func (s *Service) CreateImport(ctx context.Context, a access.Actor, scope Scope, in ImportInput) (ImportBatch, error) {
	out := ImportBatch{}
	if e := s.requireStorage(); e != nil {
		return out, e
	}
	if e := in.validate(); e != nil {
		return out, e
	}
	n, e := revisionNumber(in.Size)
	if e != nil || n <= 0 {
		return out, invalid("size_bytes", "需要正整数字符串文件大小。")
	}
	ext := strings.ToLower(filepath.Ext(in.Name))
	limit := s.maxCSVBytes
	if ext == ".xlsx" {
		limit = s.maxXLSXBytes
	}
	if (ext != ".csv" && ext != ".xlsx") || len(in.Name) > 255 || strings.ContainsAny(in.Name, "\x00\r\n") {
		return out, invalid("name", "文件必须是 CSV 或 XLSX，且名称有效。")
	}
	e = s.transact(ctx, a, scope, "imports.create", false, true, func(tx pgx.Tx, auth authorization) error {
		hit, e := receiptTx(ctx, tx, auth, "imports.create", in.RequestID, in, &out)
		if e != nil || hit {
			return e
		}
		if n > limit {
			return invalid("size_bytes", "文件超过当前部署配置的整文件大小上限。")
		}
		if in.ExpectedRevision != "0" {
			return revisionChanged()
		}
		var active int
		if e = tx.QueryRow(ctx, `SELECT count(*) FROM monitoring_import_batches WHERE tenant_id=$1 AND created_by=$2 AND state='uploading'`, scope.Tenant, a.ID).Scan(&active); e != nil {
			return e
		}
		if active >= 3 {
			return problem("upload_concurrency_limit", "最多同时上传 3 个文件，请完成或取消已有上传。", 409)
		}
		out, e = scanBatch(tx.QueryRow(ctx, `INSERT INTO monitoring_import_batches(tenant_id,project_id,created_by,original_name,expected_size) VALUES($1,$2,$3,$4,$5) RETURNING `+strings.TrimPrefix(strings.Split(batchSelect, " FROM ")[0], "SELECT "), scope.Tenant, nullable(scope.Project), a.ID, in.Name, n))
		if e != nil {
			return e
		}
		if e = auditTx(ctx, tx, auth, "imports.create", out.ID, in.Reason, in.RequestID, map[string]string{"size_bytes": in.Size}); e != nil {
			return e
		}
		return saveReceiptTx(ctx, tx, auth, "imports.create", in.RequestID, in, out)
	})
	return out, e
}
func (s *Service) Import(ctx context.Context, a access.Actor, scope Scope, id string) (ImportBatch, error) {
	out := ImportBatch{}
	e := s.transact(ctx, a, scope, "imports.detail", false, false, func(tx pgx.Tx, auth authorization) error {
		var e error
		out, e = batchTx(ctx, tx, auth, id, false)
		if e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT part,size_bytes::text,sha256 FROM monitoring_upload_parts WHERE tenant_id=$1 AND batch_id=$2 ORDER BY part`, scope.Tenant, id)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var p UploadPart
			if e = rows.Scan(&p.Part, &p.Size, &p.SHA256); e != nil {
				return e
			}
			out.Parts = append(out.Parts, p)
		}
		if e = rows.Err(); e != nil {
			return e
		}
		rows.Close()
		if out.MappingRevision != "0" {
			var raw []byte
			if e = tx.QueryRow(ctx, `SELECT mapping FROM monitoring_import_mappings WHERE tenant_id=$1 AND batch_id=$2 AND revision=$3`, scope.Tenant, id, out.MappingRevision).Scan(&raw); e != nil {
				return e
			}
			out.Mapping = &Mapping{}
			if e = json.Unmarshal(raw, out.Mapping); e != nil {
				return e
			}
		}
		return nil
	})
	if isHidden(e) && scope.Entry == "project" {
		return s.partialImport(ctx, a, scope, id)
	}
	if e == nil && out.State == "received" && out.Creator == a.ID && out.SourceFile != "" && out.ReadoptBatch == "" {
		out.Preview = s.sourcePreview(ctx, a, scope, out)
	}
	return out, e
}
func (s *Service) Imports(ctx context.Context, a access.Actor, scope Scope, cursor string) (ImportPage, error) {
	return s.QueryImports(ctx, a, scope, ImportQuery{Cursor: cursor})
}

func (s *Service) UploadPart(ctx context.Context, a access.Actor, scope Scope, id, revision string, part int, hash string, body io.Reader) (UploadPart, error) {
	out := UploadPart{}
	if e := s.requireStorage(); e != nil {
		return out, e
	}
	if part < 1 || int64(part) > 2147483647 || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(hash) {
		return out, invalid("part", "需要有效分片及 SHA-256。")
	}
	var batch ImportBatch
	check := func(tx pgx.Tx, auth authorization) error {
		var e error
		batch, e = batchTx(ctx, tx, auth, id, true)
		if e != nil {
			return e
		}
		if batch.State != "uploading" || batch.Revision != revision {
			return revisionChanged()
		}
		return nil
	}
	if e := s.transact(ctx, a, scope, "upload.part", false, true, check); e != nil {
		return out, e
	}
	size, _ := strconv.ParseInt(batch.Size, 10, 64)
	expected := min(UploadPartSize, size-int64(part-1)*UploadPartSize)
	if expected <= 0 {
		return out, invalid("part", "分片不属于此文件。")
	}
	raw, e := io.ReadAll(io.LimitReader(body, expected+1))
	if e != nil {
		return out, e
	}
	sum := sha256.Sum256(raw)
	if int64(len(raw)) != expected || hex.EncodeToString(sum[:]) != hash {
		return out, invalid("part", "分片大小或 SHA-256 不匹配。")
	}
	key := scope.Tenant + "/uploads/" + id + "/" + strconv.Itoa(part) + "/" + uuid.NewString()
	if e = s.storage.put(ctx, key, bytes.NewReader(raw), int64(len(raw)), "application/octet-stream"); e != nil {
		return out, e
	}
	out = UploadPart{part, strconv.FormatInt(expected, 10), hash}
	e = s.transact(ctx, a, scope, "upload.part", false, true, func(tx pgx.Tx, auth authorization) error {
		if e := check(tx, auth); e != nil {
			return e
		}
		var oldHash string
		e := tx.QueryRow(ctx, `SELECT sha256 FROM monitoring_upload_parts WHERE tenant_id=$1 AND batch_id=$2 AND part=$3`, scope.Tenant, id, part).Scan(&oldHash)
		if e == nil {
			if oldHash != hash {
				return problem("upload_part_conflict", "已接收分片内容不同，请创建新文件批次。", 409)
			}
			return nil
		}
		if e != pgx.ErrNoRows {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO monitoring_upload_parts(tenant_id,batch_id,part,size_bytes,sha256,object_key) VALUES($1,$2,$3,$4,$5,$6)`, scope.Tenant, id, part, expected, hash, key)
		return e
	})
	return out, e
}

func (s *Service) CompleteUpload(ctx context.Context, a access.Actor, scope Scope, id string, in UploadComplete) (ImportBatch, error) {
	out := ImportBatch{}
	if e := s.requireStorage(); e != nil {
		return out, e
	}
	if e := in.validate(); e != nil {
		return out, e
	}
	if !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(in.SHA256) {
		return out, invalid("sha256", "需要完整文件 SHA-256。")
	}
	var parts []string
	var batch ImportBatch
	var replay bool
	e := s.transact(ctx, a, scope, "upload.complete", false, true, func(tx pgx.Tx, auth authorization) error {
		var e error
		replay, e = receiptTx(ctx, tx, auth, "upload.complete:"+id, in.RequestID, in, &out)
		if e != nil || replay {
			return e
		}
		batch, e = batchTx(ctx, tx, auth, id, true)
		if e != nil {
			return e
		}
		if batch.State != "uploading" || batch.Revision != in.ExpectedRevision {
			return revisionChanged()
		}
		rows, e := tx.Query(ctx, `SELECT part,object_key FROM monitoring_upload_parts WHERE tenant_id=$1 AND batch_id=$2 ORDER BY part`, scope.Tenant, id)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var n int
			var key string
			if e = rows.Scan(&n, &key); e != nil {
				return e
			}
			if n != len(parts)+1 {
				return invalid("parts", "尚有分片未上传。")
			}
			parts = append(parts, key)
		}
		if e = rows.Err(); e != nil {
			return e
		}
		size, _ := strconv.ParseInt(batch.Size, 10, 64)
		if int64(len(parts)) != (size+UploadPartSize-1)/UploadPartSize {
			return invalid("parts", "尚有分片未上传。")
		}
		return nil
	})
	if e != nil || replay {
		return out, e
	}
	size, _ := strconv.ParseInt(batch.Size, 10, 64)
	key := scope.Tenant + "/sources/" + uuid.NewString()
	pr, pw := io.Pipe()
	copyDone := make(chan error, 1)
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		h := sha256.New()
		var total int64
		var failure error
		for _, part := range parts {
			object, e := s.storage.get(streamCtx, part)
			if e != nil {
				failure = e
				break
			}
			n, e := io.Copy(io.MultiWriter(pw, h), object)
			object.Close()
			total += n
			if e != nil {
				failure = e
				break
			}
		}
		if failure == nil && (total != size || hex.EncodeToString(h.Sum(nil)) != in.SHA256) {
			failure = invalid("sha256", "完整文件大小或 SHA-256 不匹配，请重新上传。")
		}
		pw.CloseWithError(failure)
		copyDone <- failure
	}()
	e = s.storage.put(streamCtx, key, pr, size, "application/octet-stream")
	pr.CloseWithError(e)
	if e != nil {
		cancel()
	}
	copyError := <-copyDone
	if copyError != nil {
		return out, copyError
	}
	if e != nil {
		return out, e
	}
	e = s.transact(ctx, a, scope, "upload.complete", false, true, func(tx pgx.Tx, auth authorization) error {
		hit, e := receiptTx(ctx, tx, auth, "upload.complete:"+id, in.RequestID, in, &out)
		if e != nil || hit {
			return e
		}
		b, e := batchTx(ctx, tx, auth, id, true)
		if e != nil {
			return e
		}
		if b.State != "uploading" || b.Revision != in.ExpectedRevision {
			return revisionChanged()
		}
		var file string
		e = tx.QueryRow(ctx, `INSERT INTO monitoring_source_files(tenant_id,object_key,original_name,size_bytes,sha256,created_by) VALUES($1,$2,$3,$4,$5,$6) RETURNING id::text`, scope.Tenant, key, b.Name, size, in.SHA256, a.ID).Scan(&file)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE monitoring_import_batches SET state='received',source_file_id=$3,revision=revision+1,updated_at=clock_timestamp() WHERE tenant_id=$1 AND id=$2`, scope.Tenant, id, file); e != nil {
			return e
		}
		out, e = batchTx(ctx, tx, auth, id, false)
		if e != nil {
			return e
		}
		return saveReceiptTx(ctx, tx, auth, "upload.complete:"+id, in.RequestID, in, out)
	})
	return out, e
}

func (s *Service) SetMapping(ctx context.Context, a access.Actor, scope Scope, id string, in MappingInput) (ImportBatch, error) {
	out := ImportBatch{}
	if e := in.validate(); e != nil {
		return out, e
	}
	if e := in.Mapping.validate(); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, scope, "imports.mapping", false, true, func(tx pgx.Tx, auth authorization) error {
		hit, e := receiptTx(ctx, tx, auth, "imports.mapping:"+id, in.RequestID, in, &out)
		if e != nil || hit {
			return e
		}
		b, e := batchTx(ctx, tx, auth, id, true)
		if e != nil {
			return e
		}
		if !importEditable(b) || b.Revision != in.ExpectedRevision {
			return revisionChanged()
		}
		if b.ReadoptBatch != "" {
			return invalid("mapping", "再次采用沿用原观测的固定映射；更正值或映射请上传新文件。")
		}
		if strings.TrimPrefix(strings.ToLower(filepath.Ext(b.Name)), ".") != in.Mapping.Format {
			return invalid("mapping.format", "映射格式与文件扩展名不一致。")
		}
		if _, e = authorizedWindowsTx(ctx, tx, auth, channelsOf(in.Mapping), "import"); e != nil {
			return e
		}
		raw, _ := json.Marshal(in.Mapping)
		rev, _ := revisionNumber(b.MappingRevision)
		rev++
		if _, e = tx.Exec(ctx, `INSERT INTO monitoring_import_mappings(tenant_id,batch_id,revision,mapping,sha256,created_by) VALUES($1,$2,$3,$4,$5,$6)`, scope.Tenant, id, rev, raw, digest(in.Mapping), a.ID); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE monitoring_import_batches SET state='received',mapping_revision=$3,revision=revision+1,validation_hash=NULL,validation_object=NULL,report_object=NULL,counts='{}',error_code=NULL,updated_at=clock_timestamp() WHERE tenant_id=$1 AND id=$2`, scope.Tenant, id, rev); e != nil {
			return e
		}
		out, e = batchTx(ctx, tx, auth, id, false)
		if e != nil {
			return e
		}
		return saveReceiptTx(ctx, tx, auth, "imports.mapping:"+id, in.RequestID, in, out)
	})
	return out, e
}

func (s *Service) QueueImport(ctx context.Context, a access.Actor, scope Scope, id, kind string, in WriteMeta) (ImportBatch, error) {
	out := ImportBatch{}
	if e := s.requireStorage(); e != nil {
		return out, e
	}
	if e := in.validate(); e != nil {
		return out, e
	}
	if kind != "validate" && kind != "publish" {
		return out, invalid("kind", "无效任务。")
	}
	action := "imports." + kind
	e := s.transact(ctx, a, scope, action, false, true, func(tx pgx.Tx, auth authorization) error {
		hit, e := receiptTx(ctx, tx, auth, action+":"+id, in.RequestID, in, &out)
		if e != nil || hit {
			return e
		}
		b, e := batchTx(ctx, tx, auth, id, true)
		if e != nil {
			return e
		}
		if b.Revision != in.ExpectedRevision || b.MappingRevision == "0" || kind == "validate" && !importEditable(b) || kind == "publish" && b.State != "ready" {
			return revisionChanged()
		}
		var raw []byte
		if e = tx.QueryRow(ctx, `SELECT mapping FROM monitoring_import_mappings WHERE tenant_id=$1 AND batch_id=$2 AND revision=$3`, scope.Tenant, id, b.MappingRevision).Scan(&raw); e != nil {
			return e
		}
		var m Mapping
		if e = json.Unmarshal(raw, &m); e != nil {
			return e
		}
		if _, e = authorizedWindowsTx(ctx, tx, auth, channelsOf(m), "import"); e != nil {
			return e
		}
		state := "validating"
		if kind == "publish" {
			state = "preparing"
		}
		rev, _ := revisionNumber(b.Revision)
		rev++
		if _, e = tx.Exec(ctx, `UPDATE monitoring_import_batches SET state=$3,revision=$4,error_code=NULL,updated_at=clock_timestamp() WHERE tenant_id=$1 AND id=$2`, scope.Tenant, id, state, rev); e != nil {
			return e
		}
		input, _ := json.Marshal(in)
		var job string
		e = tx.QueryRow(ctx, `INSERT INTO monitoring_jobs(tenant_id,project_id,actor_id,kind,target_id,target_revision,input_hash,input) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id::text`, scope.Tenant, nullable(scope.Project), a.ID, kind, id, rev, digest(in), input).Scan(&job)
		if e != nil {
			return e
		}
		out, e = batchTx(ctx, tx, auth, id, false)
		if e != nil {
			return e
		}
		out.JobID = job
		out.TaskRef = &taskrecord.Ref{SourceRef: taskrecord.SourceRef{Module: "monitoring", Kind: "job", RootID: job}, Generation: "1", TaskID: (taskrecord.SourceRef{Module: "monitoring", Kind: "job", RootID: job}).ID()}
		if b.JobID != "" {
			if e = taskrecord.FlushTx(ctx, tx); e != nil {
				return e
			}
			old, e := scanJob(tx.QueryRow(ctx, jobSelect+` WHERE id=$1`, b.JobID))
			if e != nil {
				return e
			}
			if old.TaskRef != nil {
				if _, e = tx.Exec(ctx, `INSERT INTO task_transaction_facts(change) SELECT snapshot FROM monitoring_task_metadata WHERE id=$1`, b.JobID); e != nil {
					return e
				}
				if e = taskrecord.FlushTx(ctx, tx); e != nil {
					return e
				}
				relation := "recreated_from"
				if kind == "publish" {
					relation = "dependency"
				}
				if e = taskrecord.LinkTx(ctx, tx, out.TaskRef.SourceRef, old.TaskRef.SourceRef, relation, "1"); e != nil {
					return e
				}
			}
		}
		if e = auditTx(ctx, tx, auth, action, id, in.Reason, in.RequestID, map[string]string{"job_id": job}); e != nil {
			return e
		}
		return saveReceiptTx(ctx, tx, auth, action+":"+id, in.RequestID, in, out)
	})
	return out, e
}

func (s *Service) ResolveImport(ctx context.Context, a access.Actor, scope Scope, id string, in ResolveInput) (ImportBatch, error) {
	out := ImportBatch{}
	if e := s.requireStorage(); e != nil {
		return out, e
	}
	if e := in.validate(); e != nil {
		return out, e
	}
	if !in.ConfirmSameObservation {
		return out, invalid("confirm_same_observation", "需要明确确认完整集合都是同一次观测。")
	}
	var reportObject, validationObject string
	var targetRevisions []byte
	var replay bool
	e := s.transact(ctx, a, scope, "imports.resolve", false, true, func(tx pgx.Tx, auth authorization) error {
		var e error
		replay, e = receiptTx(ctx, tx, auth, "imports.resolve:"+id, in.RequestID, in, &out)
		if e != nil || replay {
			return e
		}
		b, e := batchTx(ctx, tx, auth, id, true)
		if e != nil {
			return e
		}
		if b.State != "needs_resolution" || b.Revision != in.ExpectedRevision || b.ValidationHash != in.PreviewHash || b.Counts.Errors != "0" || b.Counts.Duplicates == "0" {
			return revisionChanged()
		}
		return tx.QueryRow(ctx, `SELECT report_object,validation_object,content_revisions FROM monitoring_import_batches WHERE tenant_id=$1 AND id=$2`, scope.Tenant, id).Scan(&reportObject, &validationObject, &targetRevisions)
	})
	if e != nil || replay {
		return out, e
	}
	// Evidence identifies the complete immutable issue set and target vector.
	evidence, _ := json.Marshal(map[string]any{"confirmation": in, "issues_object": reportObject, "validation_object": validationObject, "target_revisions": json.RawMessage(targetRevisions)})
	object := scope.Tenant + "/resolutions/" + uuid.NewString()
	if e := s.storage.put(ctx, object, bytes.NewReader(evidence), int64(len(evidence)), "application/json"); e != nil {
		return out, e
	}
	e = s.transact(ctx, a, scope, "imports.resolve", false, true, func(tx pgx.Tx, auth authorization) error {
		hit, e := receiptTx(ctx, tx, auth, "imports.resolve:"+id, in.RequestID, in, &out)
		if e != nil || hit {
			return e
		}
		b, e := batchTx(ctx, tx, auth, id, true)
		if e != nil {
			return e
		}
		if b.State != "needs_resolution" || b.Revision != in.ExpectedRevision || b.ValidationHash != in.PreviewHash || b.Counts.Errors != "0" || b.Counts.Duplicates == "0" {
			return revisionChanged()
		}
		var revisions []byte
		if e = tx.QueryRow(ctx, `SELECT content_revisions FROM monitoring_import_batches WHERE tenant_id=$1 AND id=$2`, scope.Tenant, id).Scan(&revisions); e != nil {
			return e
		}
		if e = checkContentTx(ctx, tx, scope.Tenant, revisions, false); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO monitoring_batch_resolutions(tenant_id,batch_id,mapping_revision,preview_hash,object_key,target_revisions,reason,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, scope.Tenant, id, b.MappingRevision, in.PreviewHash, object, revisions, in.Reason, a.ID); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE monitoring_import_batches SET state='ready',revision=revision+1,counts=jsonb_set(counts,'{ignored}',counts->'duplicates'),updated_at=clock_timestamp() WHERE tenant_id=$1 AND id=$2`, scope.Tenant, id); e != nil {
			return e
		}
		out, e = batchTx(ctx, tx, auth, id, false)
		if e != nil {
			return e
		}
		return saveReceiptTx(ctx, tx, auth, "imports.resolve:"+id, in.RequestID, in, out)
	})
	return out, e
}

func checkContentTx(ctx context.Context, tx pgx.Tx, tenant string, raw []byte, lock bool) error {
	var expected map[string]string
	if e := json.Unmarshal(raw, &expected); e != nil {
		return e
	}
	ids := []string{}
	for id := range expected {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}
	for _, id := range ids {
		var actual string
		var quarantined bool
		e := tx.QueryRow(ctx, `SELECT content_revision::text,quarantined FROM monitoring_channel_versions WHERE tenant_id=$1 AND channel_id=$2`+suffix, tenant, id).Scan(&actual, &quarantined)
		if e != nil {
			return e
		}
		if quarantined {
			return integrityUnavailable()
		}
		if actual != expected[id] {
			return problem("content_changed", "相关通道内容已变化，需要重新全量核对。", 409)
		}
	}
	return nil
}

func storageError(e error) error {
	if e == nil {
		return nil
	}
	if _, ok := e.(*Error); ok {
		return e
	}
	return problem("monitoring_storage_unavailable", "存储操作未完成，请保留任务并重试。", 503)
}
func encodeJSON(v any) []byte {
	raw, e := json.Marshal(v)
	if e != nil {
		panic(fmt.Sprintf("internal monitoring JSON: %v", e))
	}
	return raw
}
