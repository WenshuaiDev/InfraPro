package resources_test

import (
	"bytes"
	"context"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/blobstore"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/files"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tasks"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

type filesFixture struct {
	resourceFixture
	s       *files.Service
	b       runtime.Business
	scope   files.Scope
	options config.Files
	epoch   string
}

func newFilesFixture(t *testing.T) filesFixture {
	t.Helper()
	return filesFixtureForResources(t, resourcesSetup(t))
}
func filesFixtureForResources(t *testing.T, f resourceFixture) filesFixture {
	t.Helper()
	b := runtime.Compose(f.env.Service.Access())
	o := config.DefaultFiles()
	o.TempDirectory = t.TempDir()
	s := b.Files.WithOptions(o)
	if os.Getenv("INFRAPRO_FILES_CONTRACT") == "1" {
		bucket := "files-" + uuid.NewString()
		client, e := minio.New("127.0.0.1:58333", &minio.Options{Creds: credentials.NewStaticV4("contract-admin", "contract-admin-test-only", ""), BucketLookup: minio.BucketLookupPath})
		must(t, e)
		must(t, client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}))
		objects, e := blobstore.Open("http://127.0.0.1:58333", "contract-admin", "contract-admin-test-only", bucket, "files/v1")
		must(t, e)
		s.WithStorage(objects)
		t.Cleanup(func() {
			for item := range client.ListObjects(ctx, bucket, minio.ListObjectsOptions{Recursive: true}) {
				if item.Err == nil {
					_ = client.RemoveObject(ctx, bucket, item.Key, minio.RemoveObjectOptions{})
				}
			}
			_ = client.RemoveBucket(ctx, bucket)
		})
	} else {
		s.WithStorage(&displayMemoryStore{files: map[string][]byte{}})
	}
	if executable := os.Getenv("INFRAPRO_FILES_CHECKER_EXECUTABLE"); executable != "" {
		short, e := os.MkdirTemp("", "fa-check-")
		must(t, e)
		t.Cleanup(func() { _ = os.RemoveAll(short) })
		o.TempDirectory = short
		o.CheckerSocket = filepath.Join(o.TempDirectory, "checker.sock")
		s.WithOptions(o)
		cmd := exec.Command(executable, "--directory", o.TempDirectory, "--socket", o.CheckerSocket)
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "GOMEMLIMIT=256MiB", "GOMAXPROCS=2"}
		must(t, cmd.Start())
		t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
		connected := false
		for i := 0; i < 100; i++ {
			conn, e := net.DialTimeout("unix", o.CheckerSocket, 50*time.Millisecond)
			if e == nil {
				conn.Close()
				connected = true
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if !connected {
			t.Fatal("checker did not start")
		}
		s.WithChecker(files.SocketChecker{Socket: o.CheckerSocket, Timeout: o.CheckerTimeout})
	} else {
		s.WithChecker(fileLocalChecker{o.TempDirectory})
	}
	scope := files.Scope{Kind: "tenant", TenantID: f.tenant}
	caps, e := s.Capabilities(ctx, f.actor, scope)
	must(t, e)
	return filesFixture{f, s, b, scope, o, caps.RestoreEpoch}
}
func (f filesFixture) meta() files.WriteMeta {
	return files.WriteMeta{RequestID: uuid.NewString(), Reason: "普通附件生命周期验收", Epoch: f.epoch}
}
func (f filesFixture) upload(t *testing.T, data []byte, attachment, revision *string) files.CommandResult {
	t.Helper()
	intent := "add"
	if attachment != nil {
		intent = "replace"
	}
	in := files.StartUploadInput{WriteMeta: f.meta(), Host: f.scope.Host(), Intent: intent, AttachmentID: attachment, ExpectedAttachmentRevision: revision, Filename: "中文资料.txt", DisplayName: "同名资料", Description: "验证固定身份", SizeBytes: strconv.Itoa(len(data)), SHA256: fileDigest(data), DeclaredMediaType: "text/plain"}
	u, e := f.s.StartUpload(ctx, f.actor, f.scope, in)
	must(t, e)
	for offset, part := 0, 1; offset < len(data); part++ {
		end := min(offset+int(f.options.PartBytes), len(data))
		chunk := data[offset:end]
		_, e = f.s.PutPart(ctx, f.actor, f.scope, *u.UploadID, files.PartInput{Part: part, Size: int64(len(chunk)), SHA256: fileDigest(chunk), Epoch: f.epoch}, bytes.NewReader(chunk))
		must(t, e)
		offset = end
	}
	complete := files.RevisionCommand{WriteMeta: f.meta(), ExpectedRevision: u.Revision}
	v, e := f.s.CompleteUpload(ctx, f.actor, f.scope, *u.UploadID, complete)
	must(t, e)
	again, e := f.s.CompleteUpload(ctx, f.actor, f.scope, *u.UploadID, complete)
	must(t, e)
	if !again.Replayed || valueID(again.JobID) != valueID(v.JobID) {
		t.Fatal("complete receipt lost")
	}
	return v
}
func valueID(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
func (f filesFixture) run(t *testing.T, v files.CommandResult) files.JobDetail {
	t.Helper()
	worked, e := f.s.RunOnce(ctx)
	must(t, e)
	if !worked {
		t.Fatal("worker did not claim job")
	}
	job, e := f.s.Job(ctx, f.actor, f.scope, *v.JobID, "")
	must(t, e)
	return job
}
func fileError(t *testing.T, e error, code string) {
	t.Helper()
	var fe *files.Error
	if !errors.As(e, &fe) || fe.Code != code {
		t.Fatalf("want %s, got %T %v", code, e, e)
	}
}
func TestFilesReplacementConflictReconfirmHistoricalTargetAndReads(t *testing.T) {
	f := newFilesFixture(t)
	first := f.run(t, f.upload(t, []byte("first-body"), nil, nil))
	if first.State != "succeeded" {
		t.Fatal(first)
	}
	attachment := valueID(first.ResultAttachmentID)
	detail, e := f.s.Attachment(ctx, f.actor, f.scope, attachment)
	must(t, e)
	a := f.upload(t, []byte("second-body"), &attachment, &detail.Revision)
	b := f.upload(t, []byte("third-body"), &attachment, &detail.Revision)
	second := f.run(t, a)
	conflict := f.run(t, b)
	if second.State != "succeeded" || conflict.State != "needs_action" {
		t.Fatal(second, conflict)
	}
	target, e := runtime.ComposeTasks(f.env.Service.Access(), f.b).Target(ctx, f.actor, taskrecord.SourceRef{Module: "files", Kind: "job", RootID: *a.JobID}.ID(), "1", "result")
	must(t, e)
	if target.Kind != "file_attachment" || target.Parameters["revision"] != valueID(second.ResultRevisionID) {
		t.Fatal(target)
	}
	current, e := f.s.Attachment(ctx, f.actor, f.scope, attachment)
	must(t, e)
	input := files.ReconfirmInput{WriteMeta: f.meta(), ExpectedGeneration: conflict.Generation, ExpectedControlRevision: conflict.ControlRevision, ExpectedAttachmentRevision: &current.Revision}
	confirm, e := f.s.Reconfirm(ctx, f.actor, f.scope, *b.UploadID, input)
	must(t, e)
	again, e := f.s.Reconfirm(ctx, f.actor, f.scope, *b.UploadID, input)
	must(t, e)
	if !again.Replayed {
		t.Fatal("reconfirm receipt missing")
	}
	third := f.run(t, confirm)
	if third.Generation != "2" || third.State != "succeeded" {
		t.Fatal(third)
	}
	old, e := runtime.ComposeTasks(f.env.Service.Access(), f.b).Get(ctx, f.actor, first.TaskRef.SourceRef.ID(), "1")
	must(t, e)
	if old.ResultState != "available" || !old.ResultReadable {
		t.Fatal(old)
	}
	current, e = f.s.Attachment(ctx, f.actor, f.scope, attachment)
	must(t, e)
	reverted, e := f.s.ChangeAttachment(ctx, f.actor, f.scope, attachment, files.AttachmentCommand{RevisionCommand: files.RevisionCommand{WriteMeta: f.meta(), ExpectedRevision: current.Revision}, Action: "revert", RevisionID: valueID(first.ResultRevisionID)})
	must(t, e)
	history, e := f.s.Revisions(ctx, f.actor, f.scope, attachment, "", 50)
	must(t, e)
	if len(history.Items) != 4 || history.Items[0].RevisionNo != "4" || history.Items[0].FileID != valueID(first.ResultFileID) {
		t.Fatal(history)
	}
	read, e := f.s.OpenRead(ctx, f.actor, f.scope, attachment, valueID(reverted.AttachmentRevisionID), "source")
	must(t, e)
	var body bytes.Buffer
	must(t, read.Write(ctx, 0, read.Size, body.Write))
	must(t, read.Close())
	if body.String() != "first-body" {
		t.Fatal(body.String())
	}
	// Recycle after headers were authorized; no new chunk can pass the boundary.
	read, e = f.s.OpenRead(ctx, f.actor, f.scope, attachment, valueID(reverted.AttachmentRevisionID), "source")
	must(t, e)
	_, e = f.s.ChangeAttachment(ctx, f.actor, f.scope, attachment, files.AttachmentCommand{RevisionCommand: files.RevisionCommand{WriteMeta: f.meta(), ExpectedRevision: reverted.Revision}, Action: "trash"})
	must(t, e)
	sent := false
	e = read.Write(ctx, 0, read.Size, func(b []byte) (int, error) { sent = true; return len(b), nil })
	if e == nil || sent {
		t.Fatal("recycled attachment escaped read guard")
	}
	read.Close()
}
func TestFilesRegisteredCleanupRespectsHistoryReadAndBackupHold(t *testing.T) {
	f := newFilesFixture(t)
	job := f.run(t, f.upload(t, []byte("retained-content"), nil, nil))
	if job.State != "succeeded" {
		t.Fatal(job)
	}
	attachment := valueID(job.ResultAttachmentID)
	detail, e := f.s.Attachment(ctx, f.actor, f.scope, attachment)
	must(t, e)
	trashed, e := f.s.ChangeAttachment(ctx, f.actor, f.scope, attachment, files.AttachmentCommand{RevisionCommand: files.RevisionCommand{WriteMeta: f.meta(), ExpectedRevision: detail.Revision}, Action: "trash"})
	must(t, e)
	clock := time.Now().UTC().Add(31 * 24 * time.Hour)
	f.s.WithClock(func() time.Time { return clock })
	var object, space string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT source_object_id::text,space_id::text FROM file_records WHERE id=$1`, job.ResultFileID).Scan(&object, &space))
	hold := uuid.NewString()
	_, e = f.env.Pool.Exec(ctx, `INSERT INTO file_backup_manifests(id,restore_epoch,state,database_snapshot_ref,hold_until) VALUES($1,$2,'preparing','cleanup-fixture',$3)`, hold, f.epoch, clock.Add(time.Hour))
	must(t, e)
	_, e = f.env.Pool.Exec(ctx, `INSERT INTO file_backup_items(manifest_id,space_id,object_id,size_bytes,sha256,copy_status) SELECT $1,space_id,id,expected_size,expected_sha256,'pending' FROM file_objects WHERE id=$2`, hold, object)
	must(t, e)
	plan, e := f.s.CleanupPlan(ctx, "fixture", "保留备份保护")
	must(t, e)
	for _, item := range plan.Items {
		if item.ObjectID == object {
			t.Fatal("backup hold not protecting body")
		}
	}
	_, e = f.s.ChangeAttachment(ctx, f.actor, f.scope, attachment, files.AttachmentCommand{RevisionCommand: files.RevisionCommand{WriteMeta: f.meta(), ExpectedRevision: trashed.Revision}, Action: "restore"})
	fileError(t, e, "attachment_purged")
	_, e = f.env.Pool.Exec(ctx, `UPDATE file_backup_manifests SET state='released' WHERE id=$1`, hold)
	must(t, e)
	plan, e = f.s.CleanupPlan(ctx, "fixture", "到期回收验证")
	must(t, e)
	result, e := f.s.CleanupApply(ctx, plan.ID, "执行已核对计划")
	must(t, e)
	if result.State != "completed" {
		t.Fatal(result)
	}
	var lifecycle, status string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT f.lifecycle,a.status FROM file_records f JOIN file_attachment_revisions r ON r.file_id=f.id JOIN file_attachments a ON a.id=r.attachment_id WHERE f.id=$1`, job.ResultFileID).Scan(&lifecycle, &status))
	if lifecycle != "deleted" || status != "purged" {
		t.Fatal(lifecycle, status)
	}
}
func TestFilesTaskCancellationKeepsExecutionRequesterAndRejectsOldCommands(t *testing.T) {
	f := newFilesFixture(t)
	submitted := f.upload(t, []byte("cancel-before-claim"), nil, nil)
	center := runtime.ComposeTasks(f.env.Service.Access(), f.b)
	command := tasks.Command{RequestID: uuid.NewString(), Action: "cancel", ExpectedGeneration: "1", ExpectedRevision: "1", Reason: "取消后台处理"}
	result, e := center.Command(ctx, f.actor, submitted.TaskRef.SourceRef.ID(), command)
	must(t, e)
	if result.State != "cancelled" {
		t.Fatal(result)
	}
	replay, e := center.Command(ctx, f.actor, submitted.TaskRef.SourceRef.ID(), command)
	must(t, e)
	if !replay.Replayed {
		t.Fatal("task receipt not replayed")
	}
	worked, e := f.s.RunOnce(ctx)
	must(t, e)
	if worked {
		t.Fatal("cancelled generation reopened")
	}
	detail, e := center.Get(ctx, f.actor, submitted.TaskRef.SourceRef.ID(), "1")
	must(t, e)
	if detail.SelectedExecution.RequestedBy != f.actor.ID || detail.State != "cancelled" {
		t.Fatal(detail)
	}
	var n int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM notification_events WHERE source_module='files'`).Scan(&n))
	if n != 1 {
		t.Fatal(n)
	}
	_, e = f.s.OpenRead(context.Background(), f.actor, f.scope, uuid.NewString(), uuid.NewString(), "source")
	if e == nil {
		t.Fatal("arbitrary reference readable")
	}
}
