package files

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	ar "github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/jackc/pgx/v5"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type Service struct {
	access  *access.Service
	hosts   HostAdapter
	objects BlobStore
	options config.Files
	now     func() time.Time
	checker Checker
}

func New(a *access.Service, hosts HostAdapter) *Service {
	return &Service{access: a, hosts: hosts, options: config.DefaultFiles(), now: func() time.Time { return time.Now().UTC() }}
}
func (s *Service) WithStorage(v BlobStore) *Service    { s.objects = v; return s }
func (s *Service) WithOptions(v config.Files) *Service { s.options = v; return s }

// WithClock permits deterministic expiry tests without changing the system clock.
func (s *Service) WithClock(v func() time.Time) *Service { s.now = v; return s }
func (s *Service) WithChecker(v Checker) *Service        { s.checker = v; return s }

type Error struct {
	Code       string            `json:"code"`
	Message    string            `json:"message"`
	Status     int               `json:"-"`
	Fields     map[string]string `json:"fields,omitempty"`
	Retryable  bool              `json:"retryable"`
	NextAction string            `json:"next_action"`
}

func (e *Error) Error() string   { return e.Code }
func (e *Error) StatusCode() int { return e.Status }
func fault(code string, status int) *Error {
	messages := map[string]string{"file_not_found": "文件不可访问。", "file_epoch_changed": "文件服务已恢复，请刷新后重新操作。", "file_protocol_unavailable": "文件服务正在维护，暂不接受此操作。", "file_storage_unavailable": "文件存储暂不可用，请稍后核对。", "file_missing": "文件正文缺失，请联系维护人员核对。", "file_corrupt": "文件正文完整性核对失败。", "file_integrity_mismatch": "实际字节或摘要与声明不符，请重新上传。", "part_conflict": "该分片已接收不同内容。", "part_in_progress": "该分片正在接收，请稍后核对。", "upload_incomplete": "尚有分片未接收，请继续上传。", "attachment_revision_changed": "附件已变化，请核对后重新确认。", "request_conflict": "此请求编号已用于不同操作。", "upload_expired": "上传会话已到期，请重新上传。", "prepared_file_expired": "待绑定文件已到期，请重新上传。", "attachment_purged": "附件保留期已结束。", "file_type_unsupported": "文件格式或声明类型不受支持。", "file_too_large": "文件大小超出允许范围。", "permission_denied": "没有执行此操作的权限。", "task_control_required": "文件已提交处理，请在任务中心操作。", "cursor_stale": "列表已变化，请从第一页刷新。", "file_worker_unavailable": "文件检查服务暂不可用。", "file_reclaiming": "文件正在回收，不能重新引用。", "queue_limit": "文件处理队列已满，请稍后提交。", "upload_limit": "正在上传的会话过多，请先完成或取消。", "temporary_budget_exceeded": "文件暂存空间暂不可用，请稍后重试。", "rate_limited": "请求过于频繁，请稍后重试。"}
	message := messages[code]
	if message == "" {
		message = "文件操作未完成，请核对当前状态。"
	}
	action := "none"
	retry := status == 503 || status == 429
	if retry {
		action = "wait"
	}
	if code == "upload_incomplete" {
		action = "resume_upload"
	}
	if code == "attachment_revision_changed" {
		action = "reconfirm"
	}
	if code == "task_control_required" {
		action = "open_task"
	}
	if status == 410 || code == "file_integrity_mismatch" {
		action = "reupload"
	}
	return &Error{Code: code, Message: message, Status: status, Retryable: retry, NextAction: action}
}

var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var formats = []string{"png", "jpeg", "pdf", "txt", "csv", "docx", "xlsx", "pptx"}
var mediaTypes = map[string]string{"png": "image/png", "jpeg": "image/jpeg", "pdf": "application/pdf", "txt": "text/plain", "csv": "text/csv", "docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document", "xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation"}

func number(n int64) string { return strconv.FormatInt(n, 10) }
func pointer(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
func value(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
func integer(v string, min int64) (int64, error) {
	n, e := taskrecord.Number(v, min)
	if e != nil {
		return 0, fault("invalid_request", 400)
	}
	return n, nil
}
func validText(v string, min, max int) bool {
	return utf8.ValidString(v) && utf8.RuneCountInString(v) >= min && utf8.RuneCountInString(v) <= max && !strings.ContainsRune(v, 0)
}
func validateMeta(v WriteMeta) error {
	if !access.ValidID(v.RequestID) || !validText(strings.TrimSpace(v.Reason), 1, 500) {
		return fault("invalid_request", 400)
	}
	return nil
}
func fileFormat(filename, declared string) (string, error) {
	if !validText(filename, 1, 255) || strings.TrimSpace(filename) != filename || strings.ContainsAny(filename, "/\\") || strings.IndexFunc(filename, unicode.IsControl) >= 0 {
		return "", fault("invalid_request", 400)
	}
	format := strings.ToLower(strings.TrimPrefix(filepath.Ext(filename), "."))
	if format == "jpg" {
		format = "jpeg"
	}
	if !slices.Contains(formats, format) || declared != mediaTypes[format] {
		return "", fault("file_type_unsupported", 415)
	}
	return format, nil
}
func validScope(v Scope) bool {
	return access.ValidID(v.TenantID) && ((v.Kind == "tenant" && v.ProjectID == "") || (v.Kind == "project" && access.ValidID(v.ProjectID)))
}

type txState struct {
	Epoch, Mode, AuthRevision, SpaceID, HostID, ContentRevision string
	Auth                                                        HostAuthorization
}

func protocolTx(ctx context.Context, tx pgx.Tx) (epoch, mode string, e error) {
	e = tx.QueryRow(ctx, `SELECT restore_epoch::text,mode FROM file_protocol WHERE id=1 FOR SHARE`).Scan(&epoch, &mode)
	return
}
func (s *Service) transact(ctx context.Context, a access.Actor, scope Scope, write, create bool, epoch string, fn func(pgx.Tx, txState) error) error {
	return s.transactBudget(ctx, a, scope, write, create, write, epoch, fn)
}
func (s *Service) transactBudget(ctx context.Context, a access.Actor, scope Scope, write, create, budget bool, epoch string, fn func(pgx.Tx, txState) error) error {
	if !validScope(scope) || !access.ValidID(a.ID) || a.Check == nil || a.ReadSubject() != nil || s.hosts == nil {
		return access.ErrDenied
	}
	return s.access.Transact(ctx, func(tx pgx.Tx) error {
		var st txState
		var e error
		st.Epoch, st.Mode, e = protocolTx(ctx, tx)
		if e != nil {
			return e
		}
		st.AuthRevision, e = access.Lock(ctx, tx, false)
		if e != nil {
			return e
		}
		if write {
			if !access.ValidID(epoch) || epoch != st.Epoch {
				return fault("file_epoch_changed", 409)
			}
		}
		if budget {
			if _, e = tx.Exec(ctx, `SELECT key FROM file_budget_states WHERE key='instance' FOR UPDATE`); e != nil {
				return e
			}
		}
		// Budget precedes account/session and host locks. Stage reservations acquire
		// this same budget before their foreign-key account lock.
		if e = a.Check(ctx, tx, false); e != nil {
			return e
		}
		st.Auth, e = s.hosts.ResolveTx(ctx, tx, a, scope, scope.Host(), Metadata)
		if e != nil {
			return e
		}
		if !st.Auth.CanRead || (write && !st.Auth.WritePermission) {
			return access.ErrDenied
		}
		if create && st.Mode == "online" && st.Auth.CanManage {
			_, e = tx.Exec(ctx, `INSERT INTO file_spaces(tenant_id,project_id,scope_kind) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, scope.TenantID, taskrecord.Null(scope.ProjectID), scope.Kind)
			if e != nil {
				return e
			}
		}
		lock := " FOR SHARE"
		if write {
			lock = " FOR UPDATE"
		}
		e = tx.QueryRow(ctx, `SELECT id::text,content_revision::text FROM file_spaces WHERE tenant_id=$1 AND project_id IS NOT DISTINCT FROM $2::uuid`+lock, scope.TenantID, taskrecord.Null(scope.ProjectID)).Scan(&st.SpaceID, &st.ContentRevision)
		if e != nil && e != pgx.ErrNoRows {
			return e
		}
		if e == pgx.ErrNoRows {
			st.ContentRevision = "0"
			return fn(tx, st)
		}
		if create {
			_, e = tx.Exec(ctx, `INSERT INTO file_hosts(space_id,kind,object_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, st.SpaceID, scope.Host().Kind, scope.Host().ID)
			if e != nil {
				return e
			}
		}
		e = tx.QueryRow(ctx, `SELECT id::text FROM file_hosts WHERE space_id=$1 AND kind=$2 AND object_id=$3`, st.SpaceID, scope.Host().Kind, scope.Host().ID).Scan(&st.HostID)
		if e != nil && e != pgx.ErrNoRows {
			return e
		}
		return fn(tx, st)
	})
}
func (st txState) mutable() error {
	if st.Mode != "online" {
		return fault("file_protocol_unavailable", 503)
	}
	if !st.Auth.CanManage {
		if st.Auth.Status == "archived" {
			return &access.Error{Code: "project_archived", Message: "项目已归档，仅可读取当前获准附件。", Status: 409}
		}
		return access.ErrDenied
	}
	return nil
}
func (s *Service) Capabilities(ctx context.Context, a access.Actor, scope Scope) (Capabilities, error) {
	var out Capabilities
	e := s.transact(ctx, a, scope, false, false, "", func(tx pgx.Tx, st txState) error {
		out = Capabilities{CanViewTrash: st.Auth.ManagePermission, HostAuthorization: st.Auth, RestoreEpoch: st.Epoch, Mode: st.Mode, AcceptUploads: s.options.AcceptUploads, Formats: slices.Clone(formats), MaxFileBytes: number(s.options.MaxFileBytes), PartBytes: number(s.options.PartBytes), MaxConcurrency: 2}
		if st.Mode != "online" {
			out.CanManage = false
			out.Reason = "file_protocol_" + st.Mode
		}
		return nil
	})
	return out, e
}
func receiptTx(ctx context.Context, tx pgx.Tx, st txState, a access.Actor, meta WriteMeta, operation string, payload any) (CommandResult, bool, error) {
	var out CommandResult
	if st.SpaceID == "" {
		return out, false, nil
	}
	var hash, op string
	var raw []byte
	e := tx.QueryRow(ctx, `SELECT operation,payload_hash,result FROM file_command_receipts WHERE space_id=$1 AND actor_id=$2 AND request_id=$3`, st.SpaceID, a.ID, meta.RequestID).Scan(&op, &hash, &raw)
	if e == pgx.ErrNoRows {
		return out, false, nil
	}
	if e != nil {
		return out, false, e
	}
	if hash != taskrecord.Hash(payload) || op != operation {
		return out, false, fault("request_conflict", 409)
	}
	e = json.Unmarshal(raw, &out)
	out.Replayed = true
	return out, true, e
}
func (s *Service) commitReceipt(ctx context.Context, tx pgx.Tx, st txState, a access.Actor, meta WriteMeta, operation, target string, payload any, out CommandResult) error {
	_, e := tx.Exec(ctx, `INSERT INTO file_command_receipts(space_id,actor_id,request_id,operation,target_id,payload_hash,result) VALUES($1,$2,$3,$4,$5,$6,$7)`, st.SpaceID, a.ID, meta.RequestID, operation, taskrecord.Null(target), taskrecord.Hash(payload), taskrecord.JSON(out))
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `UPDATE file_spaces SET content_revision=content_revision+1 WHERE id=$1`, st.SpaceID)
	if e != nil {
		return e
	}
	return s.audit(ctx, tx, st, a.ID, meta, operation, target, out)
}
func (s *Service) audit(ctx context.Context, tx pgx.Tx, st txState, actor string, meta WriteMeta, operation, target string, result any) error {
	scope := ar.Scope{Kind: st.Auth.Scope.Kind, Tenant: st.Auth.Scope.TenantID, Project: st.Auth.Scope.ProjectID}
	before, after := meta.auditBefore, meta.auditAfter
	if after == nil {
		after = result
	}
	in := ar.LegacyInput{Event: "files." + operation, ObjectType: "file", ObjectID: target, Before: before, After: after, ActorID: actor, ActorType: "account", Entry: scope.Kind, Tenant: scope.Tenant, Project: scope.Project, Reason: meta.Reason, RequestID: meta.RequestID, Outcome: "success"}
	if v, ok := before.(AttachmentDetail); ok && v.CurrentRevision.ID != "" {
		in.Links = append(in.Links, ar.Link{Kind: "file_revision", Module: "files", TargetKind: "attachment", TargetID: target, Revision: v.CurrentRevision.ID, Relation: "input", Scope: scope})
	}
	if v, ok := after.(AttachmentDetail); ok && v.CurrentRevision.ID != "" {
		in.Links = append(in.Links, ar.Link{Kind: "file_revision", Module: "files", TargetKind: "attachment", TargetID: target, Revision: v.CurrentRevision.ID, Relation: "output", Scope: scope})
	}
	if v, ok := result.(CommandResult); ok && v.TaskRef != nil {
		in.Links = append(in.Links, ar.Link{Kind: "task_execution", Module: "files", TargetKind: "job", TargetID: v.TaskRef.RootID, Generation: v.TaskRef.Generation, Relation: "execution", Scope: scope})
	}
	if meta.auditWorker {
		in.ActorType = "worker"
		in.ExecutorKind = "file_worker"
		in.ExecutorRef = "files-prepare"
		if e := tx.QueryRow(ctx, `SELECT j.originator_id::text,r.requested_by::text FROM file_jobs j JOIN file_job_runs r ON r.job_id=j.id AND r.generation=$2 WHERE j.id=$1`, meta.auditJob, meta.auditGeneration).Scan(&in.Initiator, &in.RequestedBy); e != nil {
			return e
		}
		in.Links = append(in.Links, ar.Link{Kind: "task_execution", Module: "files", TargetKind: "job", TargetID: meta.auditJob, Generation: meta.auditGeneration, Relation: "execution", Scope: scope})
	}
	_, e := ar.AppendLegacyTx(ctx, tx, in)
	return e
}

func (s *Service) rateTx(ctx context.Context, tx pgx.Tx, account, kind string, limit int) error {
	var n int
	e := tx.QueryRow(ctx, `INSERT INTO file_rate_windows(account_id,kind,window_start,requests) VALUES($1,$2,$3,1) ON CONFLICT(account_id,kind,window_start) DO UPDATE SET requests=file_rate_windows.requests+1 RETURNING requests`, account, kind, s.now().Truncate(time.Minute)).Scan(&n)
	if e != nil {
		return e
	}
	if n > limit {
		return fault("rate_limited", 429)
	}
	return nil
}
func notFound(e error) error {
	if errors.Is(e, pgx.ErrNoRows) {
		return fault("file_not_found", 404)
	}
	return e
}
func sameVersion(actual, expected string) error {
	if _, e := integer(expected, 1); e != nil {
		return e
	}
	if actual != expected {
		return fault("attachment_revision_changed", 409)
	}
	return nil
}
func (s *Service) Request(ctx context.Context, a access.Actor, scope Scope, id string) (CommandResult, error) {
	var out CommandResult
	if !access.ValidID(id) {
		return out, fault("file_not_found", 404)
	}
	e := s.transact(ctx, a, scope, false, false, "", func(tx pgx.Tx, st txState) error {
		if !st.Auth.WritePermission {
			return access.ErrDenied
		}
		if st.SpaceID == "" {
			return fault("file_not_found", 404)
		}
		var raw []byte
		e := tx.QueryRow(ctx, `SELECT result FROM file_command_receipts WHERE space_id=$1 AND actor_id=$2 AND request_id=$3`, st.SpaceID, a.ID, id).Scan(&raw)
		if e != nil {
			return notFound(e)
		}
		e = json.Unmarshal(raw, &out)
		out.Replayed = true
		return e
	})
	return out, e
}
func contextError(e error) error {
	if e == nil {
		return nil
	}
	var fe *Error
	var ae *access.Error
	if errors.As(e, &fe) || errors.As(e, &ae) {
		return e
	}
	return fmt.Errorf("file dependency: %w", e)
}
