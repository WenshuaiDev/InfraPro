package resources_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/files"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tasks"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

func (f filesFixture) start(t *testing.T, data []byte) (files.StartUploadInput, files.CommandResult) {
	t.Helper()
	in := files.StartUploadInput{WriteMeta: f.meta(), Host: f.scope.Host(), Intent: "add", Filename: "boundary.txt", DisplayName: "边界资料", SizeBytes: fmt.Sprint(len(data)), SHA256: fileDigest(data), DeclaredMediaType: "text/plain"}
	v, e := f.s.StartUpload(ctx, f.actor, f.scope, in)
	must(t, e)
	return in, v
}
func TestFilesByteBoundariesReceiptsAndSharedBudgets(t *testing.T) {
	f := newFilesFixture(t)
	for _, size := range []int{1, 8 << 20, (8 << 20) + 1, 100 << 20} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			data := bytes.Repeat([]byte("x"), size)
			j := f.run(t, f.upload(t, data, nil, nil))
			if j.State != "succeeded" {
				t.Fatal(j)
			}
			r, e := f.s.OpenRead(ctx, f.actor, f.scope, *j.ResultAttachmentID, *j.ResultRevisionID, "source")
			must(t, e)
			if r.Size != int64(size) {
				t.Fatal(r.Size)
			}
			r.Close()
		})
	}
	for _, size := range []string{"0", "104857601", "-1", "9007199254740993"} {
		in := files.StartUploadInput{WriteMeta: f.meta(), Host: f.scope.Host(), Intent: "add", Filename: "bad.txt", SizeBytes: size, SHA256: fileDigest([]byte("x"))}
		_, e := f.s.StartUpload(ctx, f.actor, f.scope, in)
		if e == nil {
			t.Fatal("size accepted", size)
		}
	}
	data := []byte("actual-body")
	in, v := f.start(t, data)
	for _, name := range []string{"../x.txt", "a\\b.txt", "a\r\n.txt"} {
		bad := in
		bad.Filename = name
		bad.WriteMeta = f.meta()
		_, e := f.s.StartUpload(ctx, f.actor, f.scope, bad)
		if e == nil {
			t.Fatal("unsafe filename")
		}
	}
	changed := in
	changed.DisplayName = "changed"
	_, e := f.s.StartUpload(ctx, f.actor, f.scope, changed)
	fileError(t, e, "request_conflict")
	part := files.PartInput{Part: 1, Size: int64(len(data)), SHA256: fileDigest(data), Epoch: f.epoch}
	for _, body := range [][]byte{data[:len(data)-1], append(append([]byte{}, data...), byte('x'))} {
		_, e = f.s.PutPart(ctx, f.actor, f.scope, *v.UploadID, part, bytes.NewReader(body))
		if e == nil {
			t.Fatal("actual length bypass")
		}
	}
	// A rejected part reserves its independent intent until the bounded writer lease expires.
	clock := time.Now().Add(3 * time.Minute)
	f.s.WithClock(func() time.Time { return clock })
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := f.s.PutPart(ctx, f.actor, f.scope, *v.UploadID, part, bytes.NewReader(data))
			results <- e
		}()
	}
	wg.Wait()
	close(results)
	success := 0
	for e := range results {
		if e == nil {
			success++
		} else {
			var fe *files.Error
			if !errors.As(e, &fe) || (fe.Code != "part_in_progress" && fe.Code != "worker_fenced") {
				t.Fatal(e)
			}
		}
	}
	if success < 1 {
		t.Fatal("no valid receipt")
	}
	_, e = f.s.PutPart(ctx, f.actor, f.scope, *v.UploadID, part, bytes.NewReader(data))
	must(t, e)
	wrong := part
	wrong.SHA256 = fileDigest([]byte("other-body!"))
	_, e = f.s.PutPart(ctx, f.actor, f.scope, *v.UploadID, wrong, bytes.NewReader([]byte("other-body!")))
	if e == nil {
		t.Fatal("different same part")
	}
	var receipts int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM file_upload_parts WHERE upload_id=$1 AND state='received'`, *v.UploadID).Scan(&receipts))
	if receipts != 1 {
		t.Fatal(receipts)
	}
	options := f.options
	options.MaxActivePerAccount = 1
	other := runtime.Compose(f.env.Service.Access()).Files.WithOptions(options)
	_, e = other.StartUpload(ctx, f.actor, f.scope, files.StartUploadInput{WriteMeta: f.meta(), Host: f.scope.Host(), Intent: "add", Filename: "more.txt", DisplayName: "更多资料", DeclaredMediaType: "text/plain", SizeBytes: "1", SHA256: fileDigest([]byte("x"))})
	fileError(t, e, "upload_limit")
}

// Wrapper blocks the first accepted Put to model an expired writer that finishes late.
type delayedFileStore struct {
	files.BlobStore
	once             sync.Once
	entered, release chan struct{}
}

func (s *delayedFileStore) Put(ctx context.Context, key string, r io.Reader, size int64, typ string) error {
	wait := false
	s.once.Do(func() { wait = true; close(s.entered) })
	if wait {
		<-s.release
	}
	return s.BlobStore.Put(context.Background(), key, r, size, typ)
}
func TestFilesLateWriterFenceAndProducerRollback(t *testing.T) {
	f := newFilesFixture(t)
	store := &displayMemoryStore{files: map[string][]byte{}}
	delayed := &delayedFileStore{BlobStore: store, entered: make(chan struct{}), release: make(chan struct{})}
	f.s.WithStorage(delayed)
	data := []byte("late-write-data")
	_, u := f.start(t, data)
	p := files.PartInput{Part: 1, Size: int64(len(data)), SHA256: fileDigest(data), Epoch: f.epoch}
	result := make(chan error, 1)
	go func() { _, e := f.s.PutPart(ctx, f.actor, f.scope, *u.UploadID, p, bytes.NewReader(data)); result <- e }()
	<-delayed.entered
	clock := time.Now().Add(3 * time.Minute)
	second := runtime.Compose(f.env.Service.Access()).Files.WithOptions(f.options).WithStorage(store).WithClock(func() time.Time { return clock })
	_, e := second.PutPart(ctx, f.actor, f.scope, *u.UploadID, p, bytes.NewReader(data))
	must(t, e)
	close(delayed.release)
	if e = <-result; e == nil {
		t.Fatal("stale writer confirmed")
	}
	f.s.WithStorage(store).WithClock(func() time.Time { return clock })
	submitted, e := f.s.CompleteUpload(ctx, f.actor, f.scope, *u.UploadID, files.RevisionCommand{WriteMeta: f.meta(), ExpectedRevision: u.Revision})
	must(t, e)
	_, e = f.env.Pool.Exec(ctx, `CREATE FUNCTION reject_file_terminal() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.source_module='files' THEN RAISE EXCEPTION 'synthetic producer failure'; END IF;RETURN NEW;END $$;CREATE TRIGGER reject_file_terminal BEFORE INSERT ON notification_events FOR EACH ROW EXECUTE FUNCTION reject_file_terminal()`)
	must(t, e)
	_, e = f.s.RunOnce(ctx)
	if e == nil {
		t.Fatal("producer failure swallowed")
	}
	var count int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM file_attachments`).Scan(&count))
	if count != 0 {
		t.Fatal("attachment committed without event")
	}
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM file_objects`).Scan(&count))
	if count < 3 {
		t.Fatal("lost intents")
	}
	_, e = f.env.Pool.Exec(ctx, `DROP TRIGGER reject_file_terminal ON notification_events`)
	must(t, e)
	_, e = f.env.Pool.Exec(ctx, `CREATE FUNCTION reject_file_task_terminal() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.source_module='files' AND NEW.state IN ('succeeded','failed','needs_action') THEN RAISE EXCEPTION 'synthetic task producer failure'; END IF;RETURN NEW;END $$;CREATE TRIGGER reject_file_task_terminal BEFORE INSERT OR UPDATE ON task_records FOR EACH ROW EXECUTE FUNCTION reject_file_task_terminal()`)
	must(t, e)
	clock = clock.Add(2 * time.Minute)
	_, e = f.s.RunOnce(ctx)
	if e == nil {
		t.Fatal("task producer failure swallowed")
	}
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM file_attachments`).Scan(&count))
	if count != 0 {
		t.Fatal("attachment committed without terminal task fact")
	}
	_, e = f.env.Pool.Exec(ctx, `DROP TRIGGER reject_file_task_terminal ON task_records`)
	must(t, e)
	clock = clock.Add(2 * time.Minute)
	worked, e := f.s.RunOnce(ctx)
	must(t, e)
	if !worked {
		t.Fatal("expired lease not reclaimed")
	}
	j, e := f.s.Job(ctx, f.actor, f.scope, *submitted.JobID, "")
	must(t, e)
	if j.State != "succeeded" || j.Generation != "1" || j.Attempt != 3 {
		t.Fatal(j)
	}
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM notification_events WHERE source_module='files'`).Scan(&count))
	if count != 1 {
		t.Fatal("terminal duplicates")
	}
}
func TestFilesTaskActorAndLegacyFingerprint(t *testing.T) {
	f := newFilesFixture(t)
	c := f.upload(t, []byte("cancel-requester"), nil, nil)
	// The platform-only account is explicitly not a file manager.
	platform := f.fixture.actor(f.p)
	_, e := f.s.Attachment(ctx, platform, f.scope, uuid.NewString())
	if e == nil {
		t.Fatal("platform implicit file access")
	}
	j, e := f.s.Job(ctx, f.actor, f.scope, *c.JobID, "")
	must(t, e)
	commands := runtime.ComposeTasks(f.env.Service.Access(), f.b)
	_, e = commands.Command(ctx, f.actor, taskrecord.SourceRef{Module: "files", Kind: "job", RootID: *c.JobID}.ID(), tasks.Command{RequestID: uuid.NewString(), Action: "cancel", ExpectedGeneration: j.Generation, ExpectedRevision: j.ControlRevision, Reason: "取消该执行"})
	must(t, e)
	var original, actor string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT requested_by::text FROM task_executions WHERE task_id=$1`, j.TaskRef.SourceRef.ID()).Scan(&original))
	must(t, f.env.Pool.QueryRow(ctx, `SELECT actor_id::text FROM task_events WHERE task_id=$1 ORDER BY source_revision DESC LIMIT 1`, j.TaskRef.SourceRef.ID()).Scan(&actor))
	if original != f.actor.ID || actor != f.actor.ID {
		t.Fatal(original, actor)
	}
	subject := (&access.ReadSubject{}).ReadActor()
	_, e = f.s.Capabilities(ctx, subject, f.scope)
	if e == nil {
		t.Fatal("ReadSubject allowed")
	}
	// No credential or object key reaches allowed diagnostics.
	st, e := f.s.Status(ctx)
	must(t, e)
	raw := fmt.Sprint(st)
	for _, v := range []string{"cancel-requester", "files/v1/", "contract-admin", "boundary.txt"} {
		if strings.Contains(raw, v) {
			t.Fatal("diagnostic leak")
		}
	}
}

var _ pgx.Tx

func TestFilesPermissionsCurrentReadBoundaryAndArchivedHost(t *testing.T) {
	f := newFilesFixture(t)
	j := f.run(t, f.upload(t, bytes.Repeat([]byte("r"), 150000), nil, nil))
	if j.State != "succeeded" {
		t.Fatal(j)
	}
	reader, role, member := f.reader(t)
	set := func(codes ...access.Code) {
		t.Helper()
		must(t, f.env.Service.Access().Transact(ctx, func(tx pgx.Tx) error {
			if _, e := access.Lock(ctx, tx, true); e != nil {
				return e
			}
			if _, e := tx.Exec(ctx, `DELETE FROM role_permissions WHERE role_id=$1`, role); e != nil {
				return e
			}
			for _, c := range codes {
				if _, e := tx.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_code,binding_scope) VALUES($1,$2,'tenant')`, role, c); e != nil {
					return e
				}
			}
			return access.Bump(ctx, tx)
		}))
	}
	set(access.TenantProfileRead)
	_, e := f.s.Attachments(ctx, reader, f.scope, files.AttachmentFilter{})
	if e == nil {
		t.Fatal("profile read leaked attachments")
	}
	set(access.TenantProfileRead, access.TenantAttachmentsRead)
	a, e := f.s.Attachment(ctx, reader, f.scope, *j.ResultAttachmentID)
	must(t, e)
	_, e = f.s.OpenRead(ctx, reader, f.scope, a.ID, a.CurrentRevision.ID, "source")
	if e == nil {
		t.Fatal("metadata read downloaded")
	}
	set(access.TenantProfileRead, access.TenantAttachmentsRead, access.TenantAttachmentsManage, access.TenantProfileUpdate)
	name := "管理不含下载"
	_, e = f.s.ChangeAttachment(ctx, reader, f.scope, a.ID, files.AttachmentCommand{RevisionCommand: files.RevisionCommand{WriteMeta: f.meta(), ExpectedRevision: a.Revision}, Action: "update", DisplayName: &name})
	must(t, e)
	_, e = f.s.OpenRead(ctx, reader, f.scope, a.ID, a.CurrentRevision.ID, "preview")
	if e == nil {
		t.Fatal("manager implicitly downloaded")
	}
	set(access.TenantProfileRead, access.TenantAttachmentsRead, access.TenantAttachmentsDownload)
	r, e := f.s.OpenRead(ctx, reader, f.scope, a.ID, a.CurrentRevision.ID, "source")
	must(t, e)
	chunks := 0
	e = r.Write(ctx, 0, r.Size, func(b []byte) (int, error) { chunks++; return len(b), nil })
	must(t, e)
	r.Close()
	if chunks != 3 {
		t.Fatal("read chunks exceed 64KiB", chunks)
	}
	r, e = f.s.OpenRead(ctx, reader, f.scope, a.ID, a.CurrentRevision.ID, "source")
	must(t, e)
	set(access.TenantProfileRead, access.TenantAttachmentsRead)
	sent := false
	e = r.Write(ctx, 0, r.Size, func(b []byte) (int, error) { sent = true; return len(b), nil })
	if e == nil || sent {
		t.Fatal("revoked grant emitted chunk")
	}
	r.Close()
	// A delayed write retains the authorization lock; revocation completes first
	// at the next block boundary and no subsequent block receives authorization.
	set(access.TenantProfileRead, access.TenantAttachmentsRead, access.TenantAttachmentsDownload)
	r, e = f.s.OpenRead(ctx, reader, f.scope, a.ID, a.CurrentRevision.ID, "source")
	must(t, e)
	must(t, f.env.Service.Access().Transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, true); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, `UPDATE account_sessions SET revoked_at=now() WHERE account_id=$1 AND revoked_at IS NULL`, reader.ID)
		return e
	}))
	sent = false
	e = r.Write(ctx, 0, r.Size, func(b []byte) (int, error) { sent = true; return len(b), nil })
	if e == nil || sent {
		t.Fatal("ended session emitted bytes")
	}
	r.Close()
	_ = member
	project := f.project(t, "FilesProject")
	pf := f
	pf.scope = files.Scope{Kind: "project", TenantID: f.tenant, ProjectID: project.id}
	pf.actor = project.actor
	pj := pf.run(t, pf.upload(t, []byte("project-only"), nil, nil))
	if pj.State != "succeeded" {
		t.Fatal(pj)
	}
	_, e = f.s.Attachment(ctx, f.actor, pf.scope, *pj.ResultAttachmentID)
	if e == nil {
		t.Fatal("tenant governance implicitly read project")
	}
	_, e = f.env.Pool.Exec(ctx, `UPDATE projects SET status='archived',archived_at=now(),archive_reason='功能验证',archived_by=$2,version=version+1 WHERE id=$1`, project.id, project.actor.ID)
	must(t, e)
	caps, e := f.s.Capabilities(ctx, project.actor, pf.scope)
	must(t, e)
	if caps.CanManage || !caps.CanRead {
		t.Fatal(caps)
	}
	_, e = f.s.Attachment(ctx, project.actor, pf.scope, *pj.ResultAttachmentID)
	must(t, e)
	_, e = f.s.ChangeAttachment(ctx, project.actor, pf.scope, *pj.ResultAttachmentID, files.AttachmentCommand{RevisionCommand: files.RevisionCommand{WriteMeta: f.meta(), ExpectedRevision: "1"}, Action: "trash"})
	if e == nil {
		t.Fatal("archived host mutated")
	}
	_, e = f.s.Attachment(ctx, project.actor, pf.scope, *j.ResultAttachmentID)
	if e == nil {
		t.Fatal("cross space attachment")
	}
}
