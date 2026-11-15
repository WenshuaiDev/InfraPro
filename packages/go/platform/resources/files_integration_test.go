package resources_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/files"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/contentcheck"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tasks"
	"github.com/google/uuid"
	"strconv"
	"testing"
)

type fileLocalChecker struct{ root string }

func (c fileLocalChecker) Check(ctx context.Context, r files.CheckRequest) (files.CheckResult, error) {
	return contentcheck.Check(ctx, c.root, r), nil
}
func fileDigest(v []byte) string { h := sha256.Sum256(v); return hex.EncodeToString(h[:]) }
func TestFilesUploadFactsAndImmutableAttachment(t *testing.T) {
	f := resourcesSetup(t)
	options := config.DefaultFiles()
	options.TempDirectory = t.TempDir()
	s := runtime.Compose(f.env.Service.Access()).Files.WithOptions(options).WithStorage(&displayMemoryStore{files: map[string][]byte{}}).WithChecker(fileLocalChecker{options.TempDirectory})
	scope := files.Scope{Kind: "tenant", TenantID: f.tenant}
	caps, e := s.Capabilities(ctx, f.actor, scope)
	must(t, e)
	meta := func() files.WriteMeta {
		return files.WriteMeta{RequestID: uuid.NewString(), Reason: "文件附件功能自验", Epoch: caps.RestoreEpoch}
	}
	data := []byte("真实的普通附件\n")
	in := files.StartUploadInput{WriteMeta: meta(), Host: scope.Host(), Intent: "add", Filename: "资料.txt", DisplayName: "资料", SizeBytes: "25", SHA256: fileDigest(data), DeclaredMediaType: "text/plain"}
	in.SizeBytes = strconv.Itoa(len(data))
	started, e := s.StartUpload(ctx, f.actor, scope, in)
	must(t, e)
	replay, e := s.StartUpload(ctx, f.actor, scope, in)
	must(t, e)
	if !replay.Replayed || *replay.UploadID != *started.UploadID {
		t.Fatal("upload receipt not stable")
	}
	part := files.PartInput{Part: 1, Size: int64(len(data)), SHA256: fileDigest(data), Epoch: caps.RestoreEpoch}
	_, e = s.PutPart(ctx, f.actor, scope, *started.UploadID, part, bytes.NewReader(data))
	must(t, e)
	_, e = s.PutPart(ctx, f.actor, scope, *started.UploadID, part, bytes.NewReader(data))
	must(t, e)
	submitted, e := s.CompleteUpload(ctx, f.actor, scope, *started.UploadID, files.RevisionCommand{WriteMeta: meta(), ExpectedRevision: started.Revision})
	must(t, e)
	worked, e := s.RunOnce(ctx)
	must(t, e)
	if !worked {
		t.Fatal("no job claimed")
	}
	var state, attachment, revision string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT state,result_attachment_id::text,result_revision_id::text FROM file_job_runs WHERE job_id=$1 AND generation=1`, *submitted.JobID).Scan(&state, &attachment, &revision))
	if state != "succeeded" {
		t.Fatalf("file job did not bind: %s", state)
	}
	detail, e := s.Attachment(ctx, f.actor, scope, attachment)
	must(t, e)
	if detail.CurrentRevision.SHA256 != fileDigest(data) || detail.CurrentRevision.ID != revision {
		t.Fatal("immutable result identity lost")
	}
	var facts, events int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM task_records WHERE source_module='files' AND source_root_id=$1`, *submitted.JobID).Scan(&facts))
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM notification_events WHERE source_module='files'`).Scan(&events))
	if facts != 1 || events != 1 {
		t.Fatalf("atomic public facts: tasks=%d events=%d", facts, events)
	}
	_, e = runtime.ComposeTaskIndexer(f.env.Service.Access()).Tick(ctx, "files-integration")
	must(t, e)
	var backfilled bool
	must(t, f.env.Pool.QueryRow(ctx, `SELECT backfill_complete FROM task_source_states WHERE source_module='files' AND source_kind='job'`).Scan(&backfilled))
	summary, e := runtime.ComposeTasks(f.env.Service.Access()).Summary(ctx, f.actor, tasks.Filter{Mode: "mine", Source: "files"})
	must(t, e)
	if !backfilled || summary.CountsState != "exact" || summary.Total == nil || *summary.Total != 1 {
		t.Fatal("file source was not reconciled by normal worker tick", backfilled, summary)
	}
	_, e = f.env.Pool.Exec(ctx, `UPDATE file_records SET sha256=$2 WHERE id=$1`, detail.CurrentRevision.FileID, fileDigest([]byte("different")))
	if e == nil {
		t.Fatal("body mutated")
	}
	_, e = f.env.Pool.Exec(ctx, `UPDATE file_attachment_revisions SET reason='rewrite' WHERE id=$1`, revision)
	if e == nil {
		t.Fatal("history mutated")
	}
	trashed, e := s.ChangeAttachment(ctx, f.actor, scope, attachment, files.AttachmentCommand{RevisionCommand: files.RevisionCommand{WriteMeta: meta(), ExpectedRevision: detail.Revision}, Action: "trash"})
	must(t, e)
	restored, e := s.ChangeAttachment(ctx, f.actor, scope, attachment, files.AttachmentCommand{RevisionCommand: files.RevisionCommand{WriteMeta: meta(), ExpectedRevision: trashed.Revision}, Action: "restore"})
	must(t, e)
	if restored.State != "active" {
		t.Fatal("restore did not restore identity")
	}
}
