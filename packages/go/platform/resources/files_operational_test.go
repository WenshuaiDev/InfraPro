package resources_test

import (
	"bytes"
	"context"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/files"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/notifications"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tasks"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"testing"
	"time"
)

func fileMember(t *testing.T, f filesFixture) (access.Actor, string, string) {
	t.Helper()
	a, role, member := f.reader(t)
	must(t, f.env.Service.Access().Transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, true); e != nil {
			return e
		}
		for _, code := range []access.Code{access.TenantProfileRead, access.TenantProfileUpdate, access.TenantAttachmentsRead, access.TenantAttachmentsDownload, access.TenantAttachmentsManage} {
			if _, e := tx.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_code,binding_scope) VALUES($1,$2,'tenant')`, role, code); e != nil {
				return e
			}
		}
		return access.Bump(ctx, tx)
	}))
	return a, role, member
}
func TestFilesConcurrentPartsMaintainBudgetLockOrder(t *testing.T) {
	f := newFilesFixture(t)
	f.s.WithStorage(&displayMemoryStore{files: map[string][]byte{}})
	for n := 0; n < 6; n++ {
		data := []byte("parallel")
		_, u := f.start(t, data)
		results := make(chan error, 2)
		for i := 0; i < 2; i++ {
			go func() {
				_, e := f.s.PutPart(ctx, f.actor, f.scope, *u.UploadID, files.PartInput{Part: 1, Size: int64(len(data)), SHA256: fileDigest(data), Epoch: f.epoch}, bytes.NewReader(data))
				results <- e
			}()
		}
		ok := 0
		for range 2 {
			e := <-results
			if e == nil {
				ok++
				continue
			}
			var fe *files.Error
			if !errors.As(e, &fe) || fe.Code != "part_in_progress" {
				t.Fatal(e)
			}
		}
		if ok == 0 {
			t.Fatal("no winning receipt")
		}
		_, e := f.s.CancelUpload(ctx, f.actor, f.scope, *u.UploadID, files.RevisionCommand{WriteMeta: f.meta(), ExpectedRevision: u.Revision})
		must(t, e)
	}
}
func TestFilesWorkerObserverAndBackupDatabaseRoles(t *testing.T) {
	f := newFilesFixture(t)
	store := &displayMemoryStore{files: map[string][]byte{}}
	f.s.WithStorage(store)
	u := f.upload(t, []byte("least privilege"), nil, nil)
	rolePool := func(role string) *pgxpool.Pool {
		pc, e := pgxpool.ParseConfig(f.env.Config.DatabaseURL)
		must(t, e)
		pc.ConnConfig.RuntimeParams["role"] = role
		p, e := pgxpool.NewWithConfig(ctx, pc)
		must(t, e)
		t.Cleanup(p.Close)
		return p
	}
	pool := rolePool("infrapro_files_worker")
	for _, q := range []string{`SELECT password_hash FROM accounts`, `SELECT * FROM account_sessions`, `SELECT * FROM account_notifications`, `SELECT * FROM notification_preferences`} {
		if _, e := pool.Exec(ctx, q); e == nil {
			t.Fatal("worker overprivileged", q)
		}
	}
	worker := runtime.Compose(access.New(pool, slog.Default())).Files.WithOptions(f.options).WithStorage(store).WithChecker(fileLocalChecker{f.options.TempDirectory})
	worked, e := worker.RunOnce(ctx)
	must(t, e)
	if !worked {
		t.Fatal("worker idle")
	}
	j, e := f.s.Job(ctx, f.actor, f.scope, *u.JobID, "")
	must(t, e)
	if j.State != "succeeded" {
		t.Fatal(j)
	}
	must(t, worker.Sweep(ctx))
	observerPool := rolePool("infrapro_files_observer")
	observer := runtime.Compose(access.New(observerPool, slog.Default())).Files
	_, e = observer.Status(ctx)
	must(t, e)
	for _, q := range []string{`SELECT filename FROM file_uploads`, `SELECT object_key FROM file_objects`, `SELECT * FROM file_jobs`, `UPDATE file_protocol SET mode='read_only'`} {
		if _, e = observerPool.Exec(ctx, q); e == nil {
			t.Fatal("observer revealed private metadata", q)
		}
	}
	backup := runtime.Compose(access.New(rolePool("infrapro_files_backup"), slog.Default())).Files.WithOptions(f.options).WithStorage(store)
	manifest, e := backup.BackupPrepare(ctx, "isolated-role-test", "验证备份角色")
	must(t, e)
	if manifest.State != "preparing" {
		t.Fatal(manifest)
	}
}
func TestFilesNotificationPreferencesFixedGenerationAndAdministratorActor(t *testing.T) {
	f := newFilesFixture(t)
	actor, role, _ := fileMember(t, f)
	memberFixture := f
	memberFixture.actor = actor
	u := memberFixture.upload(t, []byte("member cancellation"), nil, nil)
	j, e := f.s.Job(ctx, f.actor, f.scope, *u.JobID, "")
	must(t, e)
	center := runtime.ComposeTasks(f.env.Service.Access(), f.b)
	_, e = center.Command(ctx, f.actor, j.TaskRef.SourceRef.ID(), tasks.Command{RequestID: uuid.NewString(), Action: "cancel", ExpectedGeneration: j.Generation, ExpectedRevision: j.ControlRevision, Reason: "管理员代为取消"})
	must(t, e)
	var requester, eventActor string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT requested_by::text FROM task_executions WHERE task_id=$1`, j.TaskRef.SourceRef.ID()).Scan(&requester))
	must(t, f.env.Pool.QueryRow(ctx, `SELECT actor_id::text FROM task_events WHERE task_id=$1 ORDER BY source_revision DESC LIMIT 1`, j.TaskRef.SourceRef.ID()).Scan(&eventActor))
	if requester != actor.ID || eventActor != f.actor.ID {
		t.Fatal("event actor changed execution mandate", requester, eventActor)
	}
	inbox := runtime.ComposeNotifications(f.env.Service.Access())
	for i := 0; i < 2; i++ {
		_, e = inbox.Tick(ctx)
		must(t, e)
	}
	page, e := inbox.List(ctx, actor, notifications.Filter{Category: "tasks"})
	must(t, e)
	if len(page.Items) != 1 {
		t.Fatal("recipient delivery", page)
	}
	target, e := inbox.Target(ctx, actor, page.Items[0].ID)
	must(t, e)
	if target.Generation != "1" {
		t.Fatal(target)
	}
	var n int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM account_notifications n JOIN notification_events e ON e.id=n.event_id WHERE e.source_module='files' AND n.account_id<>$1`, actor.ID).Scan(&n))
	if n != 0 {
		t.Fatal("admin cancellation broadcast")
	}
	pref, e := inbox.Preferences(ctx, actor)
	must(t, e)
	pref.Tasks = false
	pref, e = inbox.SavePreferences(ctx, actor, pref)
	must(t, e)
	done := memberFixture.run(t, memberFixture.upload(t, []byte("notifications disabled"), nil, nil))
	if done.State != "succeeded" {
		t.Fatal(done)
	}
	_, e = inbox.Tick(ctx)
	must(t, e)
	page, e = inbox.List(ctx, actor, notifications.Filter{Category: "tasks"})
	must(t, e)
	if len(page.Items) != 1 {
		t.Fatal("preference ignored")
	}
	pref.Tasks = true
	_, e = inbox.SavePreferences(ctx, actor, pref)
	must(t, e)
	old := memberFixture.run(t, memberFixture.upload(t, []byte("original result"), nil, nil))
	_, e = inbox.Tick(ctx)
	must(t, e)
	page, e = inbox.List(ctx, actor, notifications.Filter{Category: "tasks"})
	must(t, e)
	var notification string
	for _, v := range page.Items {
		if v.TaskID == old.TaskRef.SourceRef.ID() {
			notification = v.ID
		}
	}
	if notification == "" {
		t.Fatal("missing original notification")
	}
	a, e := f.s.Attachment(ctx, actor, f.scope, *old.ResultAttachmentID)
	must(t, e)
	memberFixture.run(t, memberFixture.upload(t, []byte("replacement result"), &a.ID, &a.Revision))
	source, e := center.Target(ctx, actor, old.TaskRef.SourceRef.ID(), "1", "result")
	must(t, e)
	if source.Parameters["revision"] != *old.ResultRevisionID {
		t.Fatal("historical result moved", source)
	}
	must(t, f.env.Service.Access().Transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, true); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `DELETE FROM role_permissions WHERE role_id=$1 AND permission_code=$2`, role, access.TenantAttachmentsRead); e != nil {
			return e
		}
		return access.Bump(ctx, tx)
	}))
	redacted, e := inbox.Get(ctx, actor, notification)
	var denied *access.Error
	if e != nil && (!errors.As(e, &denied) || denied.Status != 404) {
		t.Fatal(e)
	}
	if e == nil && (!redacted.Redacted || redacted.ObjectName != "" || redacted.TargetAvailable) {
		t.Fatal("revoked notification leaked", redacted)
	}
}

type filesAfterChecker struct {
	base  files.Checker
	after func()
}

func (c filesAfterChecker) Check(ctx context.Context, in files.CheckRequest) (files.CheckResult, error) {
	v, e := c.base.Check(ctx, in)
	if e == nil {
		c.after()
	}
	return v, e
}
func TestFilesBindingUsesOriginalMembershipAfterRejoinAndIgnoresLogout(t *testing.T) {
	f := newFilesFixture(t)
	actor, role, member := fileMember(t, f)
	mf := f
	mf.actor = actor
	u := mf.upload(t, []byte("revoked at bind"), nil, nil)
	f.s.WithChecker(filesAfterChecker{fileLocalChecker{f.options.TempDirectory}, func() {
		must(t, f.env.Service.Access().Transact(ctx, func(tx pgx.Tx) error {
			if _, e := access.Lock(ctx, tx, true); e != nil {
				return e
			}
			if _, e := tx.Exec(ctx, `UPDATE tenant_member_roles SET revoked_at=now(),revoked_by=$2 WHERE membership_id=$1 AND revoked_at IS NULL`, member, f.actor.ID); e != nil {
				return e
			}
			if _, e := tx.Exec(ctx, `UPDATE tenant_memberships SET status='ended',ended_at=now(),ended_by=$2,end_reason='合成退出',status_changed_at=now(),status_changed_by=$2,status_reason='合成退出' WHERE id=$1`, member, f.actor.ID); e != nil {
				return e
			}
			var replacement string
			if e := tx.QueryRow(ctx, `INSERT INTO tenant_memberships(tenant_id,account_id,joined_by,status_changed_by) VALUES($1,$2,$3,$3) RETURNING id::text`, f.tenant, actor.ID, f.actor.ID).Scan(&replacement); e != nil {
				return e
			}
			if _, e := tx.Exec(ctx, `INSERT INTO tenant_member_roles(tenant_id,membership_id,role_id,assigned_by) SELECT $1,$2,id,$3 FROM roles WHERE builtin_key='tenant_member' OR id=$4`, f.tenant, replacement, f.actor.ID, role); e != nil {
				return e
			}
			return access.Bump(ctx, tx)
		}))
	}})
	j := mf.run(t, u)
	if j.State != "needs_action" {
		t.Fatal("old membership revived", j)
	}
	var count int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM file_attachments`).Scan(&count))
	if count != 0 {
		t.Fatal("bound after membership ended")
	}
	f.s.WithChecker(fileLocalChecker{f.options.TempDirectory})
	next := mf.upload(t, []byte("accepted delegation survives logout"), nil, nil)
	_, e := f.env.Pool.Exec(ctx, `UPDATE account_sessions SET revoked_at=now() WHERE account_id=$1`, actor.ID)
	must(t, e)
	worked, e := f.s.RunOnce(ctx)
	must(t, e)
	if !worked {
		t.Fatal("worker idle")
	}
	result, e := f.s.Job(ctx, f.actor, f.scope, *next.JobID, "")
	must(t, e)
	if result.State != "succeeded" {
		t.Fatal("logout cancelled delegation", result)
	}
}

type filesLostDelete struct {
	files.BlobStore
	lost bool
}

func (s *filesLostDelete) Delete(ctx context.Context, key string) error {
	e := s.BlobStore.Delete(ctx, key)
	if e == nil && !s.lost {
		s.lost = true
		return errors.New("synthetic lost deletion response")
	}
	return e
}
func TestFilesSharedQueueDownloadBudgetsAndLostDeletion(t *testing.T) {
	f := newFilesFixture(t)
	store := &displayMemoryStore{files: map[string][]byte{}}
	f.s.WithStorage(store)
	options := f.options
	options.MaxQueuedPerTenant = 1
	options.MaxQueuedPerInstance = 1
	f.s.WithOptions(options)
	first := f.upload(t, []byte("queue one"), nil, nil)
	body := []byte("queue two")
	_, up := f.start(t, body)
	_, e := f.s.PutPart(ctx, f.actor, f.scope, *up.UploadID, files.PartInput{Part: 1, Size: int64(len(body)), SHA256: fileDigest(body), Epoch: f.epoch}, bytes.NewReader(body))
	must(t, e)
	second := runtime.Compose(f.env.Service.Access()).Files.WithOptions(options).WithStorage(store)
	_, e = second.CompleteUpload(ctx, f.actor, f.scope, *up.UploadID, files.RevisionCommand{WriteMeta: f.meta(), ExpectedRevision: up.Revision})
	fileError(t, e, "queue_limit")
	j := f.run(t, first)
	if j.State != "succeeded" {
		t.Fatal(j)
	}
	options.MaxDownloadsPerAccount = 1
	f.s.WithOptions(options)
	r, e := f.s.OpenRead(ctx, f.actor, f.scope, *j.ResultAttachmentID, *j.ResultRevisionID, "source")
	must(t, e)
	second.WithOptions(options)
	_, e = second.OpenRead(ctx, f.actor, f.scope, *j.ResultAttachmentID, *j.ResultRevisionID, "source")
	fileError(t, e, "rate_limited")
	r.Close()
	r, e = second.OpenRead(ctx, f.actor, f.scope, *j.ResultAttachmentID, *j.ResultRevisionID, "source")
	must(t, e)
	r.Close()
	_, e = f.s.CancelUpload(ctx, f.actor, f.scope, *up.UploadID, files.RevisionCommand{WriteMeta: f.meta(), ExpectedRevision: up.Revision})
	must(t, e)
	_, e = f.s.ChangeAttachment(ctx, f.actor, f.scope, *j.ResultAttachmentID, files.AttachmentCommand{RevisionCommand: files.RevisionCommand{WriteMeta: f.meta(), ExpectedRevision: "1"}, Action: "trash"})
	must(t, e)
	clock := time.Now().Add(31 * 24 * time.Hour)
	f.s.WithClock(func() time.Time { return clock })
	lost := &filesLostDelete{BlobStore: store}
	f.s.WithStorage(lost)
	unknown := "files/v1/unknown-diagnostic-only"
	store.files[unknown] = []byte("never registered")
	plan, e := f.s.CleanupPlan(ctx, "functional-test", "到期清理与丢失响应")
	must(t, e)
	if len(plan.Items) < 2 {
		t.Fatal(plan)
	}
	_, e = f.s.CleanupApply(ctx, plan.ID, "首次删除响应丢失")
	must(t, e)
	var pending int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM file_objects WHERE state='delete_pending'`).Scan(&pending))
	if pending != 1 {
		t.Fatal("unknown deletion released early", pending)
	}
	_, e = f.s.CleanupApply(ctx, plan.ID, "重查同一清理计划")
	must(t, e)
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM file_objects WHERE state<>'deleted'`).Scan(&pending))
	if pending != 0 {
		t.Fatal("deletion retry did not reconcile", pending)
	}
	if string(store.files[unknown]) != "never registered" {
		t.Fatal("unknown object deleted")
	}
	var held string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT COALESCE(sum(reserved_bytes),0)::text FROM file_budget_reservations WHERE kind='temporary_object' AND state<>'released'`).Scan(&held))
	if held != "0" {
		t.Fatal("confirmed deletion did not release budget", held)
	}
}

func TestFilesCleanupRechecksIndependentReadAndLateConsumerProtection(t *testing.T) {
	f := newFilesFixture(t)
	store := &displayMemoryStore{files: map[string][]byte{}}
	f.s.WithStorage(store)
	first := f.run(t, f.upload(t, []byte("history protected bytes"), nil, nil))
	detail, e := f.s.Attachment(ctx, f.actor, f.scope, *first.ResultAttachmentID)
	must(t, e)
	replacement := f.run(t, f.upload(t, []byte("replacement bytes"), &detail.ID, &detail.Revision))
	if replacement.State != "succeeded" {
		t.Fatal(replacement)
	}
	clock := time.Now().UTC().Add(31 * 24 * time.Hour)
	maintenance := runtime.Compose(f.env.Service.Access()).Files.WithOptions(f.options).WithClock(func() time.Time { return clock }).WithStorage(store)
	var source, space string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT source_object_id::text,space_id::text FROM file_records WHERE id=$1`, first.ResultFileID).Scan(&source, &space))
	plan, e := maintenance.CleanupPlan(ctx, "functional-test", "旧修订独立保留")
	must(t, e)
	for _, item := range plan.Items {
		if item.ObjectID == source {
			t.Fatal("active historical revision became a candidate")
		}
	}
	reader, e := f.s.OpenRead(ctx, f.actor, f.scope, *first.ResultAttachmentID, *first.ResultRevisionID, "source")
	must(t, e)
	defer reader.Close()
	detail, e = f.s.Attachment(ctx, f.actor, f.scope, detail.ID)
	must(t, e)
	_, e = f.s.ChangeAttachment(ctx, f.actor, f.scope, detail.ID, files.AttachmentCommand{RevisionCommand: files.RevisionCommand{WriteMeta: f.meta(), ExpectedRevision: detail.Revision}, Action: "trash"})
	must(t, e)
	// The controlled-clock fixture keeps this persisted lease active at the purge boundary.
	_, e = f.env.Pool.Exec(ctx, `UPDATE file_read_leases SET expires_at=$2 WHERE file_id=$1 AND closed_at IS NULL`, first.ResultFileID, clock.Add(time.Hour))
	must(t, e)
	plan, e = maintenance.CleanupPlan(ctx, "functional-test", "读取租约独立保护")
	must(t, e)
	for _, item := range plan.Items {
		if item.ObjectID == source {
			t.Fatal("live read lease became a candidate")
		}
	}
	reader.Close()
	plan, e = maintenance.CleanupPlan(ctx, "functional-test", "记录候选后新增保护")
	must(t, e)
	found := false
	for _, item := range plan.Items {
		if item.ObjectID == source {
			found = true
		}
	}
	if !found {
		t.Fatal("expired unprotected body missing from plan")
	}
	// There is no external consumer registered in this release. Seed an internal
	// durable consumer fact to exercise cleanup's forward-compatible DB protection.
	_, e = f.env.Pool.Exec(ctx, `INSERT INTO file_protection_refs(space_id,file_id,owner_module,owner_kind,owner_id,owner_revision,kind) VALUES($1,$2,'fixture','fixed-input',$3,'1','consumer')`, space, first.ResultFileID, uuid.NewString())
	must(t, e)
	_, e = maintenance.CleanupApply(ctx, plan.ID, "重新核对晚到引用")
	must(t, e)
	var state, lifecycle string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT o.state,f.lifecycle FROM file_records f JOIN file_objects o ON o.id=f.source_object_id WHERE f.id=$1`, first.ResultFileID).Scan(&state, &lifecycle))
	if state != "available" || lifecycle != "ready" {
		t.Fatal("late protection ignored", state, lifecycle)
	}
	var dangling int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM file_protection_refs r JOIN file_records f ON f.id=r.file_id WHERE r.released_at IS NULL AND f.lifecycle IN ('reclaiming','deleted')`).Scan(&dangling))
	if dangling != 0 {
		t.Fatal("live protection points at reclaimed body")
	}
	must(t, f.env.Service.Access().Transact(ctx, func(tx pgx.Tx) error {
		e := f.s.ProtectTx(ctx, tx, files.Protection{SpaceID: space, FileID: *first.ResultFileID, Module: "unregistered", Kind: "consumer", OwnerID: uuid.NewString(), OwnerRevision: "1"})
		if !errors.Is(e, access.ErrDenied) {
			t.Fatal("unregistered consumer accepted", e)
		}
		return nil
	}))
}
