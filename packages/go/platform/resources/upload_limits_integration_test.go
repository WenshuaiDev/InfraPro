//go:build monitoringintegration

package resources_test

import (
	"bytes"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/display"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/files"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	sp "github.com/WenshuaiDev/InfraPro/packages/go/platform/spatial"
	"github.com/google/uuid"
	"testing"
)

func TestUploadLimitsPreserveAcceptedFileAfterReduction(t *testing.T) {
	f := newFilesFixture(t)
	f.options.MaxFileBytes = 2
	f.s.WithOptions(f.options)
	data := []byte("AB")
	in, started := f.start(t, data)
	f.options.MaxFileBytes = 1
	f.s.WithOptions(f.options)
	replayed, e := f.s.StartUpload(ctx, f.actor, f.scope, in)
	must(t, e)
	if !replayed.Replayed || *replayed.UploadID != *started.UploadID {
		t.Fatal("quota reduction changed the accepted upload receipt")
	}
	caps, e := f.s.Capabilities(ctx, f.actor, f.scope)
	must(t, e)
	if caps.MaxFileBytes != "1" {
		t.Fatal(caps.MaxFileBytes)
	}
	_, e = f.s.PutPart(ctx, f.actor, f.scope, *started.UploadID, files.PartInput{Part: 1, Size: 2, SHA256: fileDigest(data), Epoch: f.epoch}, bytes.NewReader(data))
	must(t, e)
	complete, e := f.s.CompleteUpload(ctx, f.actor, f.scope, *started.UploadID, files.RevisionCommand{WriteMeta: f.meta(), ExpectedRevision: started.Revision})
	must(t, e)
	job := f.run(t, complete)
	if job.State != "succeeded" {
		t.Fatal(job)
	}
	r, e := f.s.OpenRead(ctx, f.actor, f.scope, *job.ResultAttachmentID, *job.ResultRevisionID, "source")
	must(t, e)
	defer r.Close()
	var content bytes.Buffer
	must(t, r.Write(ctx, 0, r.Size, content.Write))
	if !bytes.Equal(content.Bytes(), data) {
		t.Fatal("accepted content changed")
	}
	_, e = f.s.StartUpload(ctx, f.actor, f.scope, files.StartUploadInput{WriteMeta: f.meta(), Host: f.scope.Host(), Intent: "add", Filename: "new.txt", DisplayName: "new", SizeBytes: "2", SHA256: fileDigest(data), DeclaredMediaType: "text/plain"})
	fileError(t, e, "file_too_large")
}

func TestUploadLimitsSpatialBeyondOldPartCount(t *testing.T) {
	f := spatialSetup(t)
	facility := f.facility(t, "UPLOAD-LIMIT")
	asset := f.newAsset(t, facility.ID, "layout", "LIMIT")
	version := f.newVersion(t, asset.ID, "", false)
	size := sp.MaxSourceSize + 1
	f.s.WithMaxFileBytes(size)
	in := sp.UploadInput{Meta: spatialMeta(), Revision: version.Revision, VersionID: version.ID, Filename: "large.pdf", Size: size, SHA256: fileDigest([]byte("x"))}
	upload, e := f.s.StartUpload(ctx, f.actor, f.scope, in)
	must(t, e)
	f.s.WithMaxFileBytes(1)
	_, e = f.s.UploadPart(ctx, f.actor, f.scope, upload.ID, 33, fileDigest([]byte("x")), bytes.NewReader([]byte("x")))
	must(t, e)
	_, e = f.s.UploadPart(ctx, f.actor, f.scope, upload.ID, 34, fileDigest([]byte("x")), bytes.NewReader([]byte("x")))
	if e == nil {
		t.Fatal("out-of-range spatial part accepted")
	}
	in.Meta = spatialMeta()
	_, e = f.s.StartUpload(ctx, f.actor, f.scope, in)
	if e == nil {
		t.Fatal("new spatial upload ignored reduced quota")
	}
}

func TestUploadLimitsDisplayBeyondOldPartCount(t *testing.T) {
	f := resourcesSetup(t)
	p := f.project(t, "UploadLimits")
	s := runtime.Compose(f.env.Service.Access()).Displays.WithStorage(&displayMemoryStore{files: map[string][]byte{}})
	opts := display.DefaultOptions()
	opts.MaxPackageBytes = (128 << 20) + 1
	s.WithOptions(opts)
	scope := display.Scope{Tenant: f.tenant, Project: p.id}
	command := func(id, action, revision string, in display.Command) map[string]any {
		t.Helper()
		in.Meta = display.Meta{RequestID: uuid.NewString(), Revision: revision, Reason: "configurable upload limit"}
		out, e := s.Command(ctx, p.actor, scope, id, action, "", in)
		must(t, e)
		return out
	}
	command("", "settings", "0", display.Command{Enabled: true})
	instance := command("", "create", "0", display.Command{Name: "Upload limits"})
	upload := command(instance["id"].(string), "upload", instance["revision"].(string), display.Command{Size: opts.MaxPackageBytes, SHA256: fileDigest([]byte("x"))})
	opts.MaxPackageBytes = 1
	s.WithOptions(opts)
	must(t, s.UploadPart(ctx, p.actor, scope, instance["id"].(string), upload["upload_id"].(string), 32, fileDigest([]byte("x")), []byte("x")))
	if s.UploadPart(ctx, p.actor, scope, instance["id"].(string), upload["upload_id"].(string), 33, fileDigest([]byte("x")), []byte("x")) == nil {
		t.Fatal("out-of-range display part accepted")
	}
	_, e := s.Command(ctx, p.actor, scope, instance["id"].(string), "upload", "", display.Command{Meta: display.Meta{RequestID: uuid.NewString(), Revision: upload["revision"].(string), Reason: "reduced"}, Size: 2, SHA256: fileDigest([]byte("xx"))})
	if e == nil {
		t.Fatal("new display upload ignored reduced quota")
	}
}
