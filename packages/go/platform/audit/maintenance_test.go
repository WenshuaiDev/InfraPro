package audit_test

import (
	"errors"
	"io/fs"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/audit"
	ar "github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestAuditRetentionBackfillProtectedCleanupAndRoles(t *testing.T) {
	f := setup(t)
	s := f.business.Audit
	tenant := f.tenant(t, "audit-retain")
	project := f.project(t, tenant, "audit-retain-project")
	retention := 30 * 24 * time.Hour
	plan, e := s.Plan(ctx, "policy", "导入实际 30 天旧配置", &retention)
	must(t, e)
	_, e = s.ApplyPlan(ctx, plan.ID, plan.Revision, uuid.NewString(), "核对配置后追加策略版本")
	must(t, e)
	var platform, tenantDays string
	var projectKeep *string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT keep_seconds::text FROM audit_retention_policies WHERE class='platform' ORDER BY version DESC LIMIT 1`).Scan(&platform))
	must(t, f.env.Pool.QueryRow(ctx, `SELECT keep_seconds::text FROM audit_retention_policies WHERE class='tenant' ORDER BY version DESC LIMIT 1`).Scan(&tenantDays))
	must(t, f.env.Pool.QueryRow(ctx, `SELECT keep_seconds::text FROM audit_retention_policies WHERE class='project' ORDER BY version DESC LIMIT 1`).Scan(&projectKeep))
	if platform != "2592000" || tenantDays != platform || projectKeep != nil {
		t.Fatal("historical policy not preserved or project acquired expiry")
	}
	var old, projectEvent, unknown string
	must(t, f.env.Pool.QueryRow(ctx, `INSERT INTO account_security_events(event_type,actor_id,object_type,object_id,outcome,reason,created_at,details) VALUES('role_created',$1,'role',$2,'success','旧原因不得变更',clock_timestamp()-interval '120 days','{"after":{"name":"旧名称"},"private_legacy":"unchanged"}') RETURNING id::text`, f.admin.Account.ID, uuid.NewString()).Scan(&old))
	must(t, f.env.Pool.QueryRow(ctx, `INSERT INTO account_security_events(event_type,actor_id,tenant_id,project_id,actor_scope,object_type,object_id,outcome,created_at) VALUES('project.ProjectUpdate',$1,$2,$3::uuid,'project','project',($3::uuid)::text,'success',clock_timestamp()-interval '120 days') RETURNING id::text`, f.admin.Account.ID, tenant, project).Scan(&projectEvent))
	must(t, f.env.Pool.QueryRow(ctx, `INSERT INTO account_security_events(event_type,outcome,reason) VALUES('unknown.historical','success','不可猜测归属') RETURNING id::text`).Scan(&unknown))
	var before string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT jsonb_build_object('id',id,'reason',reason,'created_at',created_at,'details',details)::text FROM account_security_events WHERE id=$1`, old).Scan(&before))
	plan, e = s.Plan(ctx, "backfill", "有界旧事实分类", nil)
	must(t, e)
	request := uuid.NewString()
	summary, e := s.ApplyPlan(ctx, plan.ID, plan.Revision, request, "保留原始字段进行分类")
	must(t, e)
	if summary["unclassified"] != 1 {
		t.Fatal("unknown history not counted", summary)
	}
	_, e = s.ApplyPlan(ctx, plan.ID, plan.Revision, request, "同一维护命令重放")
	must(t, e)
	var after string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT jsonb_build_object('id',id,'reason',reason,'created_at',created_at,'details',details)::text FROM account_security_events WHERE id=$1`, old).Scan(&after))
	if before != after {
		t.Fatal("backfill changed original evidence")
	}
	var expiration *time.Time
	must(t, f.env.Pool.QueryRow(ctx, `SELECT retain_until FROM account_security_events WHERE id=$1`, projectEvent).Scan(&expiration))
	if expiration != nil {
		t.Fatal("project history gained automatic retention")
	}
	var visibility string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT visibility_class FROM account_security_events WHERE id=$1`, unknown).Scan(&visibility))
	if visibility != "legacy_unclassified" {
		t.Fatal("unknown history guessed as platform")
	}
	var native ar.EventRef
	must(t, f.env.Service.Access().Transact(ctx, func(tx pgx.Tx) error {
		var e error
		native, e = ar.AppendTx(ctx, tx, ar.Fact{Module: "access", Event: "role_created", SourceKey: "expired-native:" + uuid.NewString(), Effect: "created", Scope: audit.Scope{Kind: "platform"}, ActorType: "account", ActorID: f.admin.Account.ID, ObjectType: "role", ObjectID: uuid.NewString(), OccurredAt: time.Now().Add(-120 * 24 * time.Hour), Outcome: "success"})
		return e
	}))
	plan, e = s.Plan(ctx, "facts", "预览到期平台记录", nil)
	must(t, e)
	hold, e := s.Hold(ctx, "event", old, "计划生成后新增保护", time.Now().Add(time.Hour))
	must(t, e)
	// Setting an arbitrary session variable cannot bypass the trigger.
	e = f.env.Service.Access().Transact(ctx, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `SELECT set_config('infrapro.audit_cleanup','true',true)`); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, `DELETE FROM account_security_events WHERE id=$1`, native.ID)
		return e
	})
	if e == nil {
		t.Fatal("session flag bypassed immutable history")
	}
	result, e := s.ApplyPlan(ctx, plan.ID, plan.Revision, uuid.NewString(), "执行时重新核验保护项")
	must(t, e)
	if result["deleted"] != float64(1) || result["protected"] != float64(1) {
		t.Fatal("cleanup did not revalidate hold", result)
	}
	var exists bool
	must(t, f.env.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM account_security_events WHERE id=$1)`, old).Scan(&exists))
	if !exists {
		t.Fatal("protected history removed")
	}
	must(t, s.ReleaseHold(ctx, hold.ID, uuid.NewString(), "验证后释放测试保护"))
	for _, sql := range []string{`UPDATE account_security_events SET reason='tampered' WHERE id=` + old, `DELETE FROM account_security_events WHERE id=` + old, `INSERT INTO audit_cleanup_inflight(transaction_id,event_id) VALUES(pg_current_xact_id(),` + old + `)`} {
		e = f.env.Service.Access().Transact(ctx, func(tx pgx.Tx) error {
			if _, e := tx.Exec(ctx, `SET LOCAL ROLE infrapro_audit_api`); e != nil {
				return e
			}
			_, e := tx.Exec(ctx, sql)
			return e
		})
		if e == nil {
			t.Fatal("ordinary API role changed audit history")
		}
	}
	_, e = s.Plan(ctx, "policy", "未知部署只生成待核对计划", nil)
	must(t, e)
}
func TestAuditObjectCleanupHonorsReadLease(t *testing.T) {
	f := setup(t)
	store := exportStorage(t, f)
	s := f.business.Audit
	scope := audit.Scope{Kind: "platform"}
	export, e := s.CreateExport(ctx, f.actor, scope, uuid.NewString(), exportInput(t, s, f.actor, scope, audit.Filter{}))
	must(t, e)
	_, e = s.RunExportOnce(ctx)
	must(t, e)
	read, e := s.OpenRead(ctx, f.actor, scope, export.ID, "1", "GET", "cleanup-live-read")
	must(t, e)
	var key string
	must(t, f.env.Pool.QueryRow(ctx, `UPDATE audit_export_objects SET expires_at=clock_timestamp()-interval '1 second' WHERE export_id=$1 RETURNING object_key`, export.ID).Scan(&key))
	plan, e := s.Plan(ctx, "objects", "到期成果预览", nil)
	must(t, e)
	result, e := s.ApplyObjects(ctx, plan.ID, plan.Revision, uuid.NewString(), "活跃读取必须保护正文")
	must(t, e)
	if result["protected"] != 1 {
		t.Fatal("read lease did not protect object", result)
	}
	body, e := store.Get(ctx, key)
	must(t, e)
	must(t, body.Close())
	must(t, read.Close())
	plan, e = s.Plan(ctx, "objects", "读取结束后再次预览", nil)
	must(t, e)
	result, e = s.ApplyObjects(ctx, plan.ID, plan.Revision, uuid.NewString(), "删除已到期且未读取的注册成果")
	must(t, e)
	if result["deleted"] != 1 {
		t.Fatal("expired object not deleted", result)
	}
	_, e = store.Get(ctx, key)
	if !errors.Is(e, fs.ErrNotExist) {
		t.Fatal("object still present", e)
	}
	v, e := s.Export(ctx, f.actor, scope, export.ID, "1")
	must(t, e)
	if v.State != "succeeded" || v.ResultState != "expired" {
		t.Fatal("cleanup changed terminal run", v)
	}
	var rows int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_export_rows WHERE export_id=$1`, export.ID).Scan(&rows))
	if rows != 0 {
		t.Fatal("expired materialized rows retained")
	}
}
func TestAuditRestoreFencesWithoutReissuingNotifications(t *testing.T) {
	f := setup(t)
	exportStorage(t, f)
	s := f.business.Audit
	scope := audit.Scope{Kind: "platform"}
	success, e := s.CreateExport(ctx, f.actor, scope, uuid.NewString(), exportInput(t, s, f.actor, scope, audit.Filter{}))
	must(t, e)
	_, e = s.RunExportOnce(ctx)
	must(t, e)
	active, e := s.CreateExport(ctx, f.actor, scope, uuid.NewString(), exportInput(t, s, f.actor, scope, audit.Filter{}))
	must(t, e)
	page, e := s.Events(ctx, f.actor, scope, audit.Filter{Limit: 1})
	must(t, e)
	read, e := s.OpenRead(ctx, f.actor, scope, success.ID, "1", "GET", "restore-open-read")
	must(t, e)
	defer read.Close()
	var notificationCount int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM notification_events`).Scan(&notificationCount))
	must(t, s.SetMaintenanceMode(ctx, "reconciling", uuid.NewString(), "恢复门禁自验"))
	request := uuid.NewString()
	summary, e := s.RestoreReconcile(ctx, request, "核对一致恢复点并轮换协议")
	must(t, e)
	if summary["interrupted_exports"] != 1 || summary["notifications_reissued"] != 0 {
		t.Fatal("restore result incorrect", summary)
	}
	_, e = s.RestoreReconcile(ctx, request, "重复维护命令")
	must(t, e)
	var count int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM notification_events`).Scan(&count))
	if count != notificationCount {
		t.Fatal("restore reissued terminal notifications")
	}
	var outcome string
	var sent *string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT outcome,sent_bytes::text FROM audit_transfer_receipts WHERE request_id='restore-open-read'`).Scan(&outcome, &sent))
	if outcome != "interrupted_unknown" || sent != nil {
		t.Fatal("restore fabricated final transfer count")
	}
	_, e = s.Events(ctx, f.actor, scope, audit.Filter{Limit: 1, Cursor: page.NextCursor})
	errorCode(t, e, "audit_cursor_stale")
	if e = read.Authorize(ctx); e == nil {
		t.Fatal("old read lease survived restore")
	}
	v, e := s.Export(ctx, f.actor, scope, active.ID, "1")
	must(t, e)
	if v.State != "failed" || v.ErrorCode != "restore_interrupted" {
		t.Fatal("unfinished generation not terminalized")
	}
	v, e = s.Export(ctx, f.actor, scope, success.ID, "1")
	must(t, e)
	if v.State != "succeeded" || v.ResultState != "invalidated" {
		t.Fatal("success history changed or old epoch still downloads")
	}
	must(t, s.SetMaintenanceMode(ctx, "ready", uuid.NewString(), "只读核对完成后开放新审计工作"))
}
