//go:build spatialintegration || monitoringintegration

package resources_test

import (
	"sync"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/blobstore"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/projects"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	sp "github.com/WenshuaiDev/InfraPro/packages/go/platform/spatial"
)

func TestSpatialLifecycleProjectDefaultsAndConcurrentDraft(t *testing.T) {
	f := spatialSetup(t)
	facility := f.facility(t, "LIFECYCLE")
	a := f.newAsset(t, facility.ID, "layout", "LIFECYCLE")
	v := f.newVersion(t, a.ID, "", false)
	f.process(t, v, "bridge-plan.png")
	d, e := f.s.Draft(ctx, f.actor, f.scope, v.ID)
	must(t, e)
	// Two real transactions start with the same revision: exactly one commits.
	results := make(chan error, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, e := f.s.EditDraft(ctx, f.actor, f.scope, v.ID, sp.DraftInput{Meta: spatialMeta(), Revision: d.Version.Revision})
			results <- e
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	success := 0
	for e := range results {
		if e == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("concurrent drafts committed %d times", success)
	}
	published := f.publish(t, v.ID)
	p := f.project(t, "Lifecycle")
	g := f.accessInput(t, p, facility.ID, "create_link")
	g.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}}
	f.grant(t, p, facility.ID, g)
	f.grantSpatial(t, p, v.ID, true)
	pscope := sp.Project(f.tenant, p.id)
	initial, e := f.s.View(ctx, p.actor, pscope, v.ID)
	must(t, e)
	if initial.Revision != "0" || initial.VersionID != v.ID {
		t.Fatal(initial)
	}
	settings := initial.Settings
	settings.PageID = "page-1"
	settings.Basemap = "blank"
	saved, e := f.s.SaveView(ctx, p.actor, pscope, v.ID, sp.ViewInput{Meta: spatialMeta(), Revision: "0", VersionID: v.ID, Settings: settings})
	must(t, e)
	restored, e := f.s.View(ctx, p.actor, pscope, v.ID)
	must(t, e)
	if restored.Revision != saved.Revision || restored.Settings.PageID != "page-1" {
		t.Fatal(restored)
	}
	asset, e := f.s.Asset(ctx, f.actor, f.scope, a.ID)
	must(t, e)
	in := sp.ControlInput{Meta: spatialMeta(), Revision: asset.Revision}
	preview, e := f.s.Preview(ctx, f.actor, f.scope, a.ID, "assets.retire", in)
	must(t, e)
	in.PreviewHash = preview.Hash
	_, e = f.s.Control(ctx, f.actor, f.scope, a.ID, "assets.retire", in)
	must(t, e)
	_, e = f.s.Manifest(ctx, p.actor, pscope, v.ID)
	must(t, e)
	asset, e = f.s.Asset(ctx, f.actor, f.scope, a.ID)
	must(t, e)
	_, e = f.s.CreateVersion(ctx, f.actor, f.scope, a.ID, sp.VersionInput{Meta: spatialMeta(), Revision: asset.Revision})
	if e == nil {
		t.Fatal("retired asset accepted new draft")
	}
	_, e = f.s.Control(ctx, f.actor, f.scope, a.ID, "assets.restore", sp.ControlInput{Meta: spatialMeta(), Revision: asset.Revision})
	must(t, e)
	projectService := projects.New(f.env.Service.Access())
	archive := projects.Change{ChangeMeta: projects.ChangeMeta{WriteMeta: projects.WriteMeta{RequestID: spatialMeta().RequestID, Reason: "空间归档自验"}}}
	pr, e := projectService.Preview(ctx, p.actor, f.tenant, p.id, "", access.OpProjectArchive, archive)
	must(t, e)
	archive.ProjectVersion, archive.Version, archive.Hash = pr.ProjectVersion, pr.Version, pr.Hash
	_, e = projectService.Change(ctx, p.actor, f.tenant, p.id, "", access.OpProjectArchive, archive, access.Meta{})
	must(t, e)
	_, e = f.s.Manifest(ctx, p.actor, pscope, v.ID)
	must(t, e)
	file, e := f.s.OpenFile(ctx, p.actor, pscope, v.ID, "", true)
	must(t, e)
	file.Close()
	_, e = f.s.SaveView(ctx, p.actor, pscope, v.ID, sp.ViewInput{Meta: spatialMeta(), Revision: saved.Revision, VersionID: v.ID, Settings: settings})
	if e == nil {
		t.Fatal("archived project saved shared view")
	}
	grants, e := f.s.Access(ctx, f.actor, f.scope, p.id)
	must(t, e)
	change := sp.AccessInput{Meta: spatialMeta(), Revision: grants.Revision, End: []string{grants.Grants[0].ID}}
	ap, e := f.s.PreviewAccess(ctx, f.actor, f.scope, p.id, change)
	must(t, e)
	change.PreviewHash = ap.Hash
	_, e = f.s.SaveAccess(ctx, f.actor, f.scope, p.id, change)
	must(t, e)
	// Withdrawal preserves provenance but ends readability; no substitute is granted.
	in = sp.ControlInput{Meta: spatialMeta(), Revision: published.Revision}
	preview, e = f.s.Preview(ctx, f.actor, f.scope, v.ID, "versions.withdraw", in)
	must(t, e)
	in.PreviewHash = preview.Hash
	_, e = f.s.Control(ctx, f.actor, f.scope, v.ID, "versions.withdraw", in)
	must(t, e)
	_, e = f.s.Manifest(ctx, f.actor, f.scope, v.ID)
	if e == nil {
		t.Fatal("withdrawn version still readable")
	}
	unused := f.newVersion(t, a.ID, "", false)
	f.process(t, unused, "bridge-plan.png")
	fresh, e := f.s.Manifest(ctx, f.actor, f.scope, unused.ID)
	must(t, e)
	var jobCount, evidenceCount int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(DISTINCT j.id),count(e.job_id) FROM spatial_processing_jobs j LEFT JOIN spatial_task_execution_evidence e ON e.job_id=j.id WHERE j.version_id=$1`, unused.ID).Scan(&jobCount, &evidenceCount))
	if jobCount == 0 || evidenceCount == 0 {
		t.Fatal("processed draft has no fixed task evidence")
	}
	_, e = f.s.DeleteDraft(ctx, f.actor, f.scope, unused.ID, sp.RevisionInput{Meta: spatialMeta(), Revision: fresh.Revision})
	must(t, e)
	_, e = f.s.Manifest(ctx, f.actor, f.scope, unused.ID)
	if e == nil {
		t.Fatal("unused processed draft survived deletion")
	}
	versions, e := f.s.Versions(ctx, f.actor, f.scope, a.ID, sp.Filter{})
	must(t, e)
	for _, version := range versions.Items {
		if version.ID == unused.ID {
			t.Fatal("deleted draft remained in version directory")
		}
	}
	var retainedJobs, retainedEvidence int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(DISTINCT j.id),count(e.job_id) FROM spatial_processing_jobs j LEFT JOIN spatial_task_execution_evidence e ON e.job_id=j.id WHERE j.version_id=$1`, unused.ID).Scan(&retainedJobs, &retainedEvidence))
	if retainedJobs != jobCount || retainedEvidence != evidenceCount {
		t.Fatalf("draft deletion removed task history: jobs %d/%d, evidence %d/%d", retainedJobs, jobCount, retainedEvidence, evidenceCount)
	}
}

func TestSpatialExpiredLeaseCannotReplaceWinner(t *testing.T) {
	f := spatialSetup(t)
	facility := f.facility(t, "LEASE")
	a := f.newAsset(t, facility.ID, "layout", "LEASE")
	v := f.newVersion(t, a.ID, "", false)
	j := f.upload(t, v, "bridge-plan.png")
	real := sp.PythonConverter{Python: f.root + "/.artifacts/spatial-venv/bin/python", Script: f.root + "/tools/spatial-converter/convert.py"}
	gate := &gatedSpatialConverter{real: real, entered: make(chan struct{}), release: make(chan struct{})}
	f.s.WithConverter(gate, t.TempDir())
	done := make(chan error, 1)
	go func() { _, e := f.s.WorkOne(ctx, "expired-worker"); done <- e }()
	select {
	case <-gate.entered:
	case <-time.After(20 * time.Second):
		t.Fatal("old worker did not reach gate")
	}
	_, e := f.env.Pool.Exec(ctx, `UPDATE spatial_processing_jobs SET lease_until=clock_timestamp()-interval '1 second' WHERE id=$1`, j.ID)
	must(t, e)
	// Separate service avoids replacing a converter while its old call is running.
	store, e := blobstore.Open("http://127.0.0.1:58333", "contract-admin", "contract-admin-test-only", "contract-spatial", "spatial")
	must(t, e)
	replacement := sp.New(f.env.Service.Access(), nil).WithStorage(store).WithConverter(real, t.TempDir())
	worked, e := replacement.WorkOne(ctx, "winning-worker")
	must(t, e)
	if !worked {
		t.Fatal("expired lease not reclaimed")
	}
	close(gate.release)
	if e = <-done; e == nil {
		t.Fatal("expired worker attached its stale output")
	}
	job, e := f.s.Job(ctx, f.actor, f.scope, j.ID)
	must(t, e)
	if job.State != "succeeded" || job.Attempt != 2 {
		t.Fatal(job)
	}
	f.publish(t, v.ID)
}
