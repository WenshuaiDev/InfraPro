package audit_test

import (
	"bytes"
	"encoding/csv"
	"io"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/audit"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/blobstore"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tasks"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func exportStorage(t *testing.T, f fixture) *blobstore.Store {
	t.Helper()
	client, e := minio.New("127.0.0.1:58333", &minio.Options{Creds: credentials.NewStaticV4("contract-admin", "contract-admin-test-only", ""), BucketLookup: minio.BucketLookupPath})
	must(t, e)
	bucket := "audit-" + uuid.NewString()
	must(t, client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}))
	t.Cleanup(func() {
		for o := range client.ListObjects(ctx, bucket, minio.ListObjectsOptions{Recursive: true}) {
			if o.Err == nil {
				_ = client.RemoveObject(ctx, bucket, o.Key, minio.RemoveObjectOptions{})
			}
		}
		_ = client.RemoveBucket(ctx, bucket)
	})
	store, e := blobstore.Open("http://127.0.0.1:58333", "contract-admin", "contract-admin-test-only", bucket, "audit-exports")
	must(t, e)
	opts := config.DefaultAudit()
	opts.TempDirectory = t.TempDir()
	opts.NodeID = "audit-test-" + uuid.NewString()
	opts.Bucket = bucket
	opts.ObjectEndpoint = "http://127.0.0.1:58333"
	opts.ObjectAccessKey = "contract-admin"
	opts.ObjectSecretKey = "contract-admin-test-only"
	f.env.Config.Audit = opts
	f.business.Audit.WithOptions(opts).WithStorage(store)
	return store
}
func exportInput(t *testing.T, s *audit.Service, a access.Actor, scope audit.Scope, filter audit.Filter) audit.ExportInput {
	t.Helper()
	caps, e := s.Capabilities(ctx, a, scope)
	must(t, e)
	if filter.From == "" {
		filter.From = time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)
	}
	if filter.To == "" {
		filter.To = time.Now().UTC().Format(time.RFC3339Nano)
	}
	return audit.ExportInput{Filter: filter, FieldProfile: "standard", Reason: "真实私有存储导出自验", Epoch: caps.Epoch, AuthorizationRevision: caps.AuthorizationRevision, ProjectionVersion: caps.ProjectionVersion}
}
func TestAuditExportSnapshotTaskNotificationAndDownload(t *testing.T) {
	f := setup(t)
	store := exportStorage(t, f)
	s := f.business.Audit
	scope := audit.Scope{Kind: "platform"}
	_, e := f.env.Service.Access().CreateRole(ctx, f.actor, "=1+1 中文,引号\"换行\n测试", "", access.Meta{Reason: "CSV 单元格转义验证"})
	must(t, e)
	input := exportInput(t, s, f.actor, scope, audit.Filter{})
	command := uuid.NewString()
	export, e := s.CreateExport(ctx, f.actor, scope, command, input)
	must(t, e)
	if export.State != "queued" || export.Generation != "1" {
		t.Fatalf("request not queued: %+v", export)
	}
	replay, e := s.CreateExport(ctx, f.actor, scope, command, input)
	must(t, e)
	if replay.ID != export.ID {
		t.Fatal("retry created a duplicate export")
	}
	changed := input
	changed.Reason = "不同的命令载荷"
	_, e = s.CreateExport(ctx, f.actor, scope, command, changed)
	errorCode(t, e, "request_conflict")
	_, e = s.CreateExport(ctx, f.actor, scope, "", input)
	errorCode(t, e, "command_id_required")
	center := runtime.ComposeTasks(f.env.Service.Access(), f.business)
	page, e := center.List(ctx, f.actor, tasks.Filter{ScopeKind: "platform", Type: "audit.export"})
	must(t, e)
	if len(page.Items) != 1 || page.Items[0].ID != export.TaskID {
		t.Fatal("platform export absent from task center", page)
	}
	ok, e := s.RunExportOnce(ctx)
	must(t, e)
	if !ok {
		t.Fatal("worker did not claim")
	}
	result, e := s.Export(ctx, f.actor, scope, export.ID, "1")
	must(t, e)
	if result.State != "succeeded" || result.ResultState != "available" || result.File == nil {
		t.Fatalf("worker not successful: %+v", result)
	}
	var key string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT object_key FROM audit_export_objects WHERE export_id=$1 AND state='available'`, export.ID).Scan(&key))
	body, e := store.Get(ctx, key)
	must(t, e)
	raw, e := io.ReadAll(body)
	must(t, e)
	must(t, body.Close())
	size, e := strconv.ParseInt(*result.SizeBytes, 10, 64)
	must(t, e)
	if !audit.VerifyCSV(raw, size, result.SHA256) {
		t.Fatal("CSV integrity mismatch")
	}
	records, e := csv.NewReader(bytes.NewReader(raw)).ReadAll()
	must(t, e)
	count, e := strconv.Atoi(*result.RowCount)
	must(t, e)
	if len(records) != count+1 {
		t.Fatal("snapshot row count differs from CSV")
	}
	if len(records[0]) != 19 {
		t.Fatal("standard profile includes technical columns")
	}
	if strings.Contains(string(raw), ",=1+1") {
		t.Fatal("formula text not escaped")
	}
	var notifications, links int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM notification_events WHERE source_module='audit' AND source_key=$1`, "export:"+export.ID+":generation:1:terminal").Scan(&notifications))
	if notifications != 1 {
		t.Fatal("missing unique terminal notification")
	}
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_event_links l JOIN account_security_events e ON e.id=l.event_id WHERE e.event_type='audit.export.succeeded' AND e.object_id=$1 AND l.link_kind='notification_event'`, export.ID).Scan(&links))
	if links != 1 {
		t.Fatal("notification fact not explicitly linked")
	}
	read, e := s.OpenRead(ctx, f.actor, scope, export.ID, "1", "GET", "audit-download-test")
	must(t, e)
	var received bytes.Buffer
	must(t, read.Respond(ctx, func() error { return nil }))
	must(t, read.Write(ctx, 0, read.Size, received.Write))
	must(t, read.Close())
	if !bytes.Equal(received.Bytes(), raw) {
		t.Fatal("download differs from frozen output")
	}
	var outcome, sent string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT outcome,sent_bytes::text FROM audit_transfer_receipts WHERE request_id='audit-download-test'`).Scan(&outcome, &sent))
	if outcome != "complete" || sent != strconv.Itoa(len(raw)) {
		t.Fatal("transfer not accurately closed", outcome, sent)
	}
	// A current authorization revision change invalidates download, while the
	// immutable execution remains a historical success.
	_, e = f.env.Service.Access().CreateRole(ctx, f.actor, "导出后权限修订", "", access.Meta{Reason: "验证既有快照失效"})
	must(t, e)
	_, e = s.OpenRead(ctx, f.actor, scope, export.ID, "1", "HEAD", "revoked-head")
	errorCode(t, e, "audit_export_invalidated")
	result, e = s.Export(ctx, f.actor, scope, export.ID, "1")
	must(t, e)
	if result.State != "succeeded" || result.ResultState != "invalidated" {
		t.Fatal("result invalidation rewrote execution")
	}
}
func TestAuditEmptyExportCancelRetryAndOwnerIsolation(t *testing.T) {
	f := setup(t)
	exportStorage(t, f)
	s := f.business.Audit
	scope := audit.Scope{Kind: "platform"}
	manager := f.member(t, "ExportManager", []access.Code{access.AuditRead, access.PlatformAuditManage})
	ma := f.env.Service.AccessActor(manager)
	input := exportInput(t, s, f.actor, scope, audit.Filter{EventType: "does.not.exist"})
	first, e := s.CreateExport(ctx, f.actor, scope, uuid.NewString(), input)
	must(t, e)
	second, e := s.CreateExport(ctx, f.actor, scope, uuid.NewString(), input)
	must(t, e)
	_, e = s.CreateExport(ctx, f.actor, scope, uuid.NewString(), input)
	errorCode(t, e, "audit_export_limit")
	managed, e := s.Exports(ctx, ma, scope, audit.ExportFilter{}, true)
	must(t, e)
	if len(managed.Items) != 2 || managed.Items[0].Filter != nil || managed.Items[0].File != nil {
		t.Fatal("manager saw creator-only filter or file")
	}
	center := runtime.ComposeTasks(f.env.Service.Access(), f.business)
	control := tasks.Command{RequestID: uuid.NewString(), Action: "cancel", ExpectedGeneration: first.Generation, ExpectedRevision: first.ControlRevision, Reason: "管理员取消活动导出"}
	cancelled, e := center.Command(ctx, ma, first.TaskID, control)
	must(t, e)
	if cancelled.State != "cancelled" {
		t.Fatal("queued cancellation not terminal")
	}
	replayed, e := center.Command(ctx, ma, first.TaskID, control)
	must(t, e)
	if !replayed.Replayed {
		t.Fatal("cancel receipt not replayed")
	}
	current, e := s.Export(ctx, ma, scope, first.ID, "")
	must(t, e)
	_, e = center.Command(ctx, ma, first.TaskID, tasks.Command{RequestID: uuid.NewString(), Action: "retry", ExpectedGeneration: "1", ExpectedRevision: current.ControlRevision, Reason: "管理员不可代替所有人重试"})
	if e == nil {
		t.Fatal("manager retried another owner's export")
	}
	current, e = s.Export(ctx, f.actor, scope, first.ID, "")
	must(t, e)
	retried, e := center.Command(ctx, f.actor, first.TaskID, tasks.Command{RequestID: uuid.NewString(), Action: "retry", ExpectedGeneration: "1", ExpectedRevision: current.ControlRevision, Reason: "本人重新生成当前授权快照"})
	must(t, e)
	if retried.Generation != "2" {
		t.Fatal("manual retry did not create a generation")
	}
	for i := 0; i < 2; i++ {
		ok, e := s.RunExportOnce(ctx)
		must(t, e)
		if !ok {
			t.Fatal("export not claimed")
		}
	}
	current, e = s.Export(ctx, f.actor, scope, first.ID, "2")
	must(t, e)
	if current.State != "succeeded" || current.RowCount == nil || *current.RowCount != "0" {
		t.Fatalf("empty export failed: %+v", current)
	}
	old, e := s.Export(ctx, f.actor, scope, first.ID, "1")
	must(t, e)
	if old.State != "cancelled" || old.File != nil {
		t.Fatal("old generation borrowed new output")
	}
	_, e = s.OpenRead(ctx, ma, scope, second.ID, "1", "GET", "manager-no-download")
	if e == nil {
		t.Fatal("manager downloaded creator output")
	}
	read, e := s.OpenRead(ctx, f.actor, scope, first.ID, "2", "HEAD", "head-only")
	must(t, e)
	must(t, read.Respond(ctx, func() error { return nil }))
	must(t, read.Close())
	var outcome string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT outcome FROM audit_transfer_receipts WHERE request_id='head-only'`).Scan(&outcome))
	if outcome != "headers_only" {
		t.Fatal("HEAD described as completed body transfer")
	}
}
