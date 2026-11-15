//go:build spatialintegration || monitoringintegration

package resources_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/blobstore"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	sp "github.com/WenshuaiDev/InfraPro/packages/go/platform/spatial"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type spatialFixture struct {
	resourceFixture
	s     *sp.Service
	scope sp.Scope
	root  string
}

func spatialSetup(t *testing.T) spatialFixture {
	t.Helper()
	if os.Getenv("INFRAPRO_SPATIAL_CONTRACT") != "1" {
		t.Skip("requires real spatial PG/S3/converter laboratory")
	}
	f := resourcesSetup(t)
	if os.Getenv("INFRAPRO_TASK_ASSERTIONS") == "1" {
		t.Cleanup(func() { verifyTaskSources(t, f) })
	}
	root, e := filepath.Abs("../../../..")
	must(t, e)
	store, e := blobstore.Open("http://127.0.0.1:58333", "contract-admin", "contract-admin-test-only", "contract-spatial", "spatial")
	must(t, e)
	client, e := minio.New("127.0.0.1:58333", &minio.Options{Creds: credentials.NewStaticV4("contract-admin", "contract-admin-test-only", ""), BucketLookup: minio.BucketLookupPath})
	must(t, e)
	exists, e := client.BucketExists(ctx, "contract-spatial")
	must(t, e)
	if !exists {
		must(t, client.MakeBucket(ctx, "contract-spatial", minio.MakeBucketOptions{}))
	}
	business := runtime.Compose(f.env.Service.Access())
	s := business.Spatial.WithStorage(store).WithConverter(sp.PythonConverter{Python: filepath.Join(root, ".artifacts/spatial-venv/bin/python"), Script: filepath.Join(root, "tools/spatial-converter/convert.py")}, t.TempDir())
	f.srv = business.Resources
	t.Cleanup(func() {
		for item := range client.ListObjects(context.Background(), "contract-spatial", minio.ListObjectsOptions{Prefix: "spatial/" + f.tenant + "/", Recursive: true}) {
			if item.Err == nil {
				_ = client.RemoveObject(context.Background(), "contract-spatial", item.Key, minio.RemoveObjectOptions{})
			}
		}
	})
	return spatialFixture{f, s, sp.Tenant(f.tenant), root}
}
func spatialMeta() sp.Meta {
	return sp.Meta{RequestID: uuid.NewString(), Reason: "合成空间资料自验"}
}
func spatialCode(t *testing.T, e error, want string) {
	t.Helper()
	var p *sp.Error
	if !errors.As(e, &p) || p.Code != want {
		t.Fatalf("want %s got %v", want, e)
	}
}
func (f spatialFixture) newAsset(t *testing.T, facility, kind, code string) sp.Result {
	t.Helper()
	v, e := f.s.SaveAsset(ctx, f.actor, f.scope, "", sp.AssetInput{Meta: spatialMeta(), FacilityID: facility, Kind: kind, Code: code, Name: "合成示例 / " + code, Category: "plan"})
	must(t, e)
	return v
}
func (f spatialFixture) newVersion(t *testing.T, asset, base string, newSource bool) sp.Result {
	t.Helper()
	a, e := f.s.Asset(ctx, f.actor, f.scope, asset)
	must(t, e)
	v, e := f.s.CreateVersion(ctx, f.actor, f.scope, asset, sp.VersionInput{Meta: spatialMeta(), Revision: a.Revision, BaseVersionID: base, NewSource: newSource})
	must(t, e)
	return v
}
func (f spatialFixture) upload(t *testing.T, v sp.Result, name string) sp.Result {
	t.Helper()
	data, e := os.ReadFile(filepath.Join(f.root, "samples/spatial", name))
	must(t, e)
	sum := sha256.Sum256(data)
	up, e := f.s.StartUpload(ctx, f.actor, f.scope, sp.UploadInput{Meta: spatialMeta(), Revision: v.Revision, VersionID: v.ID, Filename: filepath.Base(name), Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:])})
	must(t, e)
	var current sp.Result
	for offset := 0; offset < len(data); offset += int(sp.UploadPartSize) {
		end := offset + int(sp.UploadPartSize)
		if end > len(data) {
			end = len(data)
		}
		part := data[offset:end]
		hash := sha256.Sum256(part)
		current, e = f.s.UploadPart(ctx, f.actor, f.scope, up.ID, offset/int(sp.UploadPartSize)+1, hex.EncodeToString(hash[:]), bytes.NewReader(part))
		must(t, e)
	}
	_, e = f.s.CompleteUpload(ctx, f.actor, f.scope, up.ID, sp.RevisionInput{Meta: spatialMeta(), Revision: current.Revision})
	must(t, e)
	version, e := f.s.Manifest(ctx, f.actor, f.scope, v.ID)
	must(t, e)
	job, e := f.s.Process(ctx, f.actor, f.scope, v.ID, sp.RevisionInput{Meta: spatialMeta(), Revision: version.Revision})
	must(t, e)
	return job
}
func (f spatialFixture) process(t *testing.T, v sp.Result, name string) sp.Version {
	t.Helper()
	job := f.upload(t, v, name)
	worked, e := f.s.WorkOne(ctx, "spatial-test")
	must(t, e)
	if !worked {
		t.Fatal("worker did not claim")
	}
	j, e := f.s.Job(ctx, f.actor, f.scope, job.ID)
	must(t, e)
	if j.State != "succeeded" {
		t.Fatal(j)
	}
	result, e := f.s.Manifest(ctx, f.actor, f.scope, v.ID)
	must(t, e)
	return result
}
func (f spatialFixture) publish(t *testing.T, id string) sp.Version {
	t.Helper()
	v, e := f.s.Manifest(ctx, f.actor, f.scope, id)
	must(t, e)
	in := sp.ControlInput{Meta: spatialMeta(), Revision: v.Revision}
	p, e := f.s.Preview(ctx, f.actor, f.scope, id, "versions.publish", in)
	must(t, e)
	in.PreviewHash = p.Hash
	_, e = f.s.Control(ctx, f.actor, f.scope, id, "versions.publish", in)
	must(t, e)
	v, e = f.s.Manifest(ctx, f.actor, f.scope, id)
	must(t, e)
	return v
}
func (f spatialFixture) grantSpatial(t *testing.T, p projectFixture, v string, download bool) sp.Result {
	t.Helper()
	state, e := f.s.Access(ctx, f.actor, f.scope, p.id)
	must(t, e)
	in := sp.AccessInput{Meta: spatialMeta(), Revision: state.Revision, Add: []sp.GrantChange{{VersionID: v, SourceDownload: download, Default: true}}, End: []string{}}
	preview, e := f.s.PreviewAccess(ctx, f.actor, f.scope, p.id, in)
	must(t, e)
	in.PreviewHash = preview.Hash
	out, e := f.s.SaveAccess(ctx, f.actor, f.scope, p.id, in)
	must(t, e)
	again, e := f.s.SaveAccess(ctx, f.actor, f.scope, p.id, in)
	must(t, e)
	if !again.Replayed || again.Revision != out.Revision {
		t.Fatal("grant receipt replay", again)
	}
	return out
}
func TestSpatialRealFilesVersionsAndProjectBoundaries(t *testing.T) {
	f := spatialSetup(t)
	t.Cleanup(func() { verifyTerminalNotifications(t, f.env) })
	facility := f.facility(t, "SPATIAL")
	point, e := f.write(t, target(facility, res.Point, ""), "create", map[string]any{"code": "P-01", "name": "相同业务测点", "position": map[string]any{"geographic": map[string]any{"crs": "EPSG:4326", "longitude": 116.38, "latitude": 39.9}}})
	must(t, e)
	a := f.newAsset(t, facility.ID, "layout", "LAYOUT")
	v := f.newVersion(t, a.ID, "", false)
	v1 := f.process(t, v, "bridge-layouts-v1.pdf")
	if len(v1.Manifest.Pages) != 3 {
		t.Fatal("PDF pages", v1.Manifest)
	}
	edit := sp.DraftInput{Meta: spatialMeta(), Revision: v1.Revision, Placements: []sp.Placement{{PointID: point.ID, PageID: "page-1", Anchor: []float64{.5, .5}, LabelOffset: []float64{90, -20}}}}
	saved, e := f.s.EditDraft(ctx, f.actor, f.scope, v.ID, edit)
	must(t, e)
	_, e = f.s.EditDraft(ctx, f.actor, f.scope, v.ID, sp.DraftInput{Meta: spatialMeta(), Revision: v1.Revision, Placements: edit.Placements})
	spatialCode(t, e, "revision_conflict")
	v1 = f.publish(t, v.ID)
	_, e = f.s.EditDraft(ctx, f.actor, f.scope, v.ID, sp.DraftInput{Meta: spatialMeta(), Revision: v1.Revision})
	spatialCode(t, e, "spatial_immutable")
	// Database protections are independent of the HTTP or service validation.
	_, e = f.env.Pool.Exec(ctx, `UPDATE spatial_point_placements SET anchor='[0,0]' WHERE version_id=$1`, v.ID)
	if e == nil {
		t.Fatal("database allowed historical anchor mutation")
	}
	projectA := f.project(t, "SpatialA")
	projectB := f.project(t, "SpatialB")
	for _, project := range []projectFixture{projectA, projectB} {
		in := f.accessInput(t, project, facility.ID, "create_link")
		in.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}}
		if project.id == projectA.id {
			in.Add = append(in.Add, res.GrantInput{Kind: res.Point, PointID: &point.ID, Capabilities: []string{"read"}})
		}
		f.grant(t, project, facility.ID, in)
		f.grantSpatial(t, project, v.ID, false)
	}
	scopeA, scopeB := sp.Project(f.tenant, projectA.id), sp.Project(f.tenant, projectB.id)
	markers, e := f.s.Placements(ctx, projectA.actor, scopeA, v.ID, sp.Filter{})
	must(t, e)
	if len(markers.Items) != 1 || markers.Items[0].PointID != point.ID {
		t.Fatal(markers)
	}
	hidden, e := f.s.Placements(ctx, projectB.actor, scopeB, v.ID, sp.Filter{})
	must(t, e)
	if len(hidden.Items) != 0 {
		t.Fatal("context-only point position leaked", hidden)
	}
	_, e = f.s.OpenFile(ctx, projectA.actor, scopeA, v.ID, "", true)
	spatialCode(t, e, "spatial_not_found")
	file, e := f.s.OpenFile(ctx, projectA.actor, scopeA, v.ID, v1.Manifest.Pages[0].FileID, false)
	must(t, e)
	var content bytes.Buffer
	must(t, file.Stream(ctx, 0, file.Size, func(data []byte) error { _, e := content.Write(data); return e }))
	file.Close()
	if !bytes.HasPrefix(content.Bytes(), []byte{137, 80, 78, 71}) {
		t.Fatal("browser receives rendered page, not PDF")
	}
	v2 := f.newVersion(t, a.ID, v.ID, false)
	draft, e := f.s.Draft(ctx, f.actor, f.scope, v2.ID)
	must(t, e)
	if len(draft.Placements) != 1 || draft.Placements[0].Anchor[0] != .5 {
		t.Fatal(draft)
	}
	draft.Placements[0].LabelOffset = []float64{200, 100}
	_, e = f.s.EditDraft(ctx, f.actor, f.scope, v2.ID, sp.DraftInput{Meta: spatialMeta(), Revision: draft.Version.Revision, Placements: draft.Placements})
	must(t, e)
	f.publish(t, v2.ID)
	_, e = f.s.Manifest(ctx, projectA.actor, scopeA, v2.ID)
	spatialCode(t, e, "spatial_not_found")
	projectAsset, e := f.s.Asset(ctx, projectA.actor, scopeA, a.ID)
	must(t, e)
	if projectAsset.CurrentVersionID != v.ID {
		t.Fatal("new publication silently upgraded project", projectAsset)
	}
	f.grantSpatial(t, projectA, v2.ID, true)
	file, e = f.s.OpenFile(ctx, projectA.actor, scopeA, v2.ID, "", true)
	must(t, e)
	content.Reset()
	must(t, file.Stream(ctx, 0, 5, func(data []byte) error { _, e := content.Write(data); return e }))
	file.Close()
	if content.String() != "%PDF-" {
		t.Fatal(content.String())
	}
	old, e := f.s.Placements(ctx, f.actor, f.scope, v.ID, sp.Filter{})
	must(t, e)
	if old.Items[0].LabelOffset[0] != 90 || old.Items[0].Anchor[0] != .5 {
		t.Fatal("old anchor or label changed", old)
	}
	_, e = f.write(t, target(facility, res.Point, point.ID), "delete", map[string]any{"expected_revision": 1})
	if e == nil {
		t.Fatal("retained spatial history did not protect point")
	}
	changed := f.newVersion(t, a.ID, v.ID, true)
	v3 := f.process(t, changed, "bridge-layouts-v2.pdf")
	d3, e := f.s.Draft(ctx, f.actor, f.scope, changed.ID)
	must(t, e)
	if len(d3.Placements) != 0 || len(d3.Version.Candidates) != 1 || v3.Manifest.Pages[0].Rotation != 90 {
		t.Fatal("changed source auto-adopted anchors or lost rotation", d3)
	}
	// Ending the facility relation ends grants atomically and re-linking never revives them.
	in := f.accessInput(t, projectA, facility.ID, "revoke_link")
	f.grant(t, projectA, facility.ID, in)
	_, e = f.s.Manifest(ctx, projectA.actor, scopeA, v.ID)
	spatialCode(t, e, "spatial_not_found")
	state, e := f.s.Access(ctx, f.actor, f.scope, projectA.id)
	must(t, e)
	for _, grant := range state.Grants {
		if grant.EndedAt == nil {
			t.Fatal("link revocation left grant active", grant)
		}
	}
	_ = saved
}
func TestSpatialRealModelSemanticAndFailurePipeline(t *testing.T) {
	f := spatialSetup(t)
	t.Cleanup(func() { verifyTerminalNotifications(t, f.env) })
	facility := f.facility(t, "BIM")
	for _, sample := range []string{"bridge-v1.glb", "bridge-ifc2x3-v1.ifc", "bridge-ifc4-v1.ifc", "slope-ifc4-v1.ifc"} {
		a := f.newAsset(t, facility.ID, "model", "M-"+strings.ReplaceAll(sample, ".", "-"))
		v := f.newVersion(t, a.ID, "", false)
		result := f.process(t, v, sample)
		elements, e := f.s.Elements(ctx, f.actor, f.scope, v.ID, sp.Filter{})
		must(t, e)
		if len(elements.Items) == 0 || len(result.Manifest.Files) != 1 {
			t.Fatal(result, elements)
		}
		if strings.HasSuffix(sample, ".ifc") {
			found := false
			for _, el := range elements.Items {
				if _, ok := el.Properties["Pset_SyntheticExample"]; ok && len(el.Fragments) > 0 {
					found = true
				}
			}
			if !found {
				t.Fatal("IFC geometry and actual properties not bound", elements)
			}
		}
		f.publish(t, v.ID)
	}
	a := f.newAsset(t, facility.ID, "model", "BROKEN")
	v := f.newVersion(t, a.ID, "", false)
	job := f.upload(t, v, "invalid/required-extension.glb")
	_, e := f.s.WorkOne(ctx, "failure-test")
	if e == nil {
		t.Fatal("unsupported required extension accepted")
	}
	j, e := f.s.Job(ctx, f.actor, f.scope, job.ID)
	must(t, e)
	if j.State != "failed" || j.ErrorCode != "required_extension" {
		t.Fatal(j)
	}
	version, e := f.s.Manifest(ctx, f.actor, f.scope, v.ID)
	must(t, e)
	if version.ManifestHash != "" {
		t.Fatal("partial failed output attached")
	}
	_, e = f.s.Validate(ctx, f.actor, f.scope, v.ID, sp.RevisionInput{Revision: version.Revision})
	spatialCode(t, e, "spatial_not_processed")
	retry, e := f.s.ControlJob(ctx, f.actor, f.scope, j.ID, "jobs.retry", sp.RevisionInput{Meta: spatialMeta(), Revision: j.Revision})
	must(t, e)
	_, e = f.s.ControlJob(ctx, f.actor, f.scope, j.ID, "jobs.cancel", sp.RevisionInput{Meta: spatialMeta(), Revision: retry.Revision})
	must(t, e)
	worked, e := f.s.WorkOne(ctx, "canceled-test")
	must(t, e)
	if worked {
		t.Fatal("canceled task executed")
	}
}

var _ io.Reader
