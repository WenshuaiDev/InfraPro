//go:build spatialintegration || monitoringintegration

package resources_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/blobstore"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	sp "github.com/WenshuaiDev/InfraPro/packages/go/platform/spatial"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func TestSpatialGeoJSONAndSpatialPaging(t *testing.T) {
	f := spatialSetup(t)
	facility := f.facility(t, "GEO")
	a := f.newAsset(t, "", "geolayer", "GEO")
	v := f.newVersion(t, a.ID, "", false)
	f.process(t, v, "reference.geojson")
	published := f.publish(t, v.ID)
	if len(published.Manifest.Features) != 0 {
		t.Fatal("manifest leaked unpaged GeoJSON")
	}
	features, e := f.s.Features(ctx, f.actor, f.scope, v.ID, sp.Filter{Limit: 1})
	must(t, e)
	if len(features.Items) != 1 || features.NextCursor == "" {
		t.Fatal("GeoJSON page missing", features)
	}
	more, e := f.s.Features(ctx, f.actor, f.scope, v.ID, sp.Filter{Limit: 1, Cursor: features.NextCursor})
	must(t, e)
	if len(more.Items) != 1 || more.Items[0].ID == features.Items[0].ID {
		t.Fatal("GeoJSON cursor did not advance")
	}
	_, e = f.s.Features(ctx, f.actor, f.scope, v.ID, sp.Filter{Limit: 1, Cursor: features.NextCursor, BBox: []float64{110, 30, 120, 45}})
	if e == nil {
		t.Fatal("cursor accepted altered bounds")
	}
	for i, lon := range []float64{116.38, 179.5, -179.5, 0} {
		_, e = f.write(t, target(facility, res.Point, ""), "create", map[string]any{"code": fmt.Sprintf("P%d", i), "name": "地理点", "position": map[string]any{"geographic": map[string]any{"crs": "EPSG:4326", "longitude": lon, "latitude": 39.9}}})
		must(t, e)
	}
	for _, tc := range []struct {
		bbox  []float64
		count int
	}{{[]float64{116, 39, 117, 40}, 1}, {[]float64{179, 39, -179, 40}, 2}, {[]float64{-1, 39, 1, 40}, 1}} {
		page, e := f.s.Objects(ctx, f.actor, f.scope, sp.Filter{Kind: "point", FacilityID: facility.ID, BBox: tc.bbox, Limit: 100})
		must(t, e)
		if len(page.Items) != tc.count {
			t.Fatalf("bbox %v returned %d", tc.bbox, len(page.Items))
		}
	}
	page, e := f.s.Objects(ctx, f.actor, f.scope, sp.Filter{Kind: "point", FacilityID: facility.ID, Limit: 1})
	must(t, e)
	_, e = f.s.Objects(ctx, f.actor, f.scope, sp.Filter{Kind: "point", FacilityID: facility.ID, Limit: 1, Cursor: page.NextCursor, BBox: []float64{116, 39, 117, 40}})
	if e == nil {
		t.Fatal("object cursor accepted altered bbox")
	}
}

type gatedSpatialConverter struct {
	real             sp.Converter
	entered, release chan struct{}
	once             sync.Once
}

func (g *gatedSpatialConverter) Ready(ctx context.Context) error { return g.real.Ready(ctx) }
func (g *gatedSpatialConverter) Convert(ctx context.Context, format, input, out string) (sp.Manifest, error) {
	m, e := g.real.Convert(ctx, format, input, out)
	if e != nil {
		return m, e
	}
	g.once.Do(func() { close(g.entered); <-g.release })
	return m, nil
}
func TestSpatialUploadCancelAndProcessingFences(t *testing.T) {
	f := spatialSetup(t)
	t.Cleanup(func() { verifyTerminalNotifications(t, f.env) })
	facility := f.facility(t, "FENCES")
	a := f.newAsset(t, facility.ID, "layout", "FENCES")
	v := f.newVersion(t, a.ID, "", false)
	data, e := os.ReadFile(filepath.Join(f.root, "samples/spatial/bridge-plan.png"))
	must(t, e)
	hash := sha256.Sum256(data)
	up, e := f.s.StartUpload(ctx, f.actor, f.scope, sp.UploadInput{Meta: spatialMeta(), Revision: v.Revision, VersionID: v.ID, Filename: "bridge-plan.png", Size: int64(len(data)), SHA256: hex.EncodeToString(hash[:])})
	must(t, e)
	_, e = f.s.CompleteUpload(ctx, f.actor, f.scope, up.ID, sp.RevisionInput{Meta: spatialMeta(), Revision: up.Revision})
	if e == nil {
		t.Fatal("incomplete upload sealed")
	}
	_, e = f.s.CancelUpload(ctx, f.actor, f.scope, up.ID, sp.RevisionInput{Meta: spatialMeta(), Revision: up.Revision})
	must(t, e)
	_, e = f.s.UploadPart(ctx, f.actor, f.scope, up.ID, 1, hex.EncodeToString(hash[:]), bytes.NewReader(data))
	if e == nil {
		t.Fatal("canceled upload accepted late part")
	}
	fresh, e := f.s.Manifest(ctx, f.actor, f.scope, v.ID)
	must(t, e)
	v.Revision = fresh.Revision
	job := f.upload(t, v, "bridge-plan.png")
	real := sp.PythonConverter{Python: filepath.Join(f.root, ".artifacts/spatial-venv/bin/python"), Script: filepath.Join(f.root, "tools/spatial-converter/convert.py")}
	gate := &gatedSpatialConverter{real: real, entered: make(chan struct{}), release: make(chan struct{})}
	f.s.WithConverter(gate, t.TempDir())
	done := make(chan error, 1)
	go func() { _, e := f.s.WorkOne(ctx, "old-worker"); done <- e }()
	select {
	case <-gate.entered:
	case <-time.After(20 * time.Second):
		t.Fatal("real converter did not finish")
	}
	running, e := f.s.Job(ctx, f.actor, f.scope, job.ID)
	must(t, e)
	_, e = f.s.ControlJob(ctx, f.actor, f.scope, job.ID, "jobs.cancel", sp.RevisionInput{Meta: spatialMeta(), Revision: running.Revision})
	must(t, e)
	close(gate.release)
	if e = <-done; e == nil {
		t.Fatal("canceled worker attached output")
	}
	result, e := f.s.Manifest(ctx, f.actor, f.scope, v.ID)
	must(t, e)
	if result.ManifestHash != "" {
		t.Fatal("partial output was published")
	}
	canceled, e := f.s.Job(ctx, f.actor, f.scope, job.ID)
	must(t, e)
	if canceled.State != "canceled" {
		t.Fatal(canceled)
	}
	_, e = f.s.ControlJob(ctx, f.actor, f.scope, job.ID, "jobs.retry", sp.RevisionInput{Meta: spatialMeta(), Revision: canceled.Revision})
	must(t, e)
	f.s.WithConverter(real, t.TempDir())
	worked, e := f.s.WorkOne(ctx, "replacement-worker")
	must(t, e)
	if !worked {
		t.Fatal("retry not claimed")
	}
	f.publish(t, v.ID)
}

func TestSpatialDownloadRevocationAndBasisChange(t *testing.T) {
	f := spatialSetup(t)
	facility := f.facility(t, "REVOKE")
	point, e := f.write(t, target(facility, res.Point, ""), "create", map[string]any{"code": "P", "name": "保持原锚点"})
	must(t, e)
	a := f.newAsset(t, facility.ID, "layout", "REVOKE")
	v := f.newVersion(t, a.ID, "", false)
	f.process(t, v, "bridge-layouts-v1.pdf")
	d, e := f.s.Draft(ctx, f.actor, f.scope, v.ID)
	must(t, e)
	_, e = f.s.EditDraft(ctx, f.actor, f.scope, v.ID, sp.DraftInput{Meta: spatialMeta(), Revision: d.Version.Revision, Placements: []sp.Placement{{PointID: point.ID, PageID: "page-1", Anchor: []float64{.3, .4}, LabelOffset: []float64{8, 8}}}})
	must(t, e)
	f.publish(t, v.ID)
	_, e = f.write(t, target(facility, res.Point, point.ID), "update", map[string]any{"expected_revision": point.Revision, "position": map[string]any{"geographic": map[string]any{"crs": "EPSG:4326", "longitude": 116.38, "latitude": 39.9}}})
	must(t, e)
	placements, e := f.s.Placements(ctx, f.actor, f.scope, v.ID, sp.Filter{})
	must(t, e)
	if len(placements.Items) != 1 || !placements.Items[0].BasisChanged || placements.Items[0].Anchor[0] != .3 {
		t.Fatal("basis correction moved historical anchor", placements)
	}
	project := f.project(t, "Download")
	in := f.accessInput(t, project, facility.ID, "create_link")
	in.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}}
	f.grant(t, project, facility.ID, in)
	f.grantSpatial(t, project, v.ID, true)
	scope := sp.Project(f.tenant, project.id)
	download, e := f.s.OpenFile(ctx, project.actor, scope, v.ID, "", true)
	must(t, e)
	defer download.Close()
	first := int64(1)
	must(t, download.Stream(ctx, 0, first, func(p []byte) error {
		if p[0] != '%' {
			t.Fatal("not PDF")
		}
		return nil
	}))
	grants, e := f.s.Access(ctx, f.actor, f.scope, project.id)
	must(t, e)
	change := sp.AccessInput{Meta: spatialMeta(), Revision: grants.Revision, End: []string{grants.Grants[0].ID}}
	preview, e := f.s.PreviewAccess(ctx, f.actor, f.scope, project.id, change)
	must(t, e)
	change.PreviewHash = preview.Hash
	_, e = f.s.SaveAccess(ctx, f.actor, f.scope, project.id, change)
	must(t, e)
	bytesAfter := 0
	e = download.Stream(ctx, first, download.Size-first, func(p []byte) error { bytesAfter += len(p); return nil })
	if e == nil || bytesAfter != 0 {
		t.Fatalf("revoked stream emitted %d bytes: %v", bytesAfter, e)
	}
	_, e = f.s.OpenFile(ctx, project.actor, scope, v.ID, "", true)
	if e == nil {
		t.Fatal("old file URL reopened after revoke")
	}
}

func TestSpatialBackupRestoreAndMissingFileIsolation(t *testing.T) {
	f := spatialSetup(t)
	facility := f.facility(t, "RESTORE")
	a := f.newAsset(t, facility.ID, "model", "RESTORE")
	v := f.newVersion(t, a.ID, "", false)
	f.process(t, v, "bridge-ifc4-v1.ifc")
	f.publish(t, v.ID)
	p := f.project(t, "Restore")
	grant := f.accessInput(t, p, facility.ID, "create_link")
	grant.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}}
	f.grant(t, p, facility.ID, grant)
	f.grantSpatial(t, p, v.ID, false)
	source, e := blobstore.Open("http://127.0.0.1:58333", "contract-admin", "contract-admin-test-only", "contract-spatial", "spatial")
	must(t, e)
	saved := map[string][]byte{}
	rows, e := f.env.Pool.Query(ctx, `SELECT object_key FROM spatial_blobs WHERE tenant_id=$1`, f.tenant)
	must(t, e)
	keys := []string{}
	for rows.Next() {
		var key string
		must(t, rows.Scan(&key))
		keys = append(keys, key)
	}
	rows.Close()
	for _, key := range keys {
		r, e := source.Get(ctx, key)
		must(t, e)
		saved[key], e = io.ReadAll(r)
		r.Close()
		must(t, e)
	}
	u, e := url.Parse(f.env.Config.DatabaseURL)
	must(t, e)
	schema := u.Query().Get("search_path")
	db := "spatial_restore_" + fmt.Sprintf("%d", time.Now().UnixNano())
	compose := filepath.Join(f.root, "deploy/compose/test.yaml")
	run := func(input []byte, args ...string) []byte {
		t.Helper()
		cmd := exec.Command("docker", append([]string{"compose", "-f", compose, "exec", "-T", "postgres"}, args...)...)
		cmd.Stdin = bytes.NewReader(input)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, e := cmd.Output()
		if e != nil {
			t.Fatalf("restore command: %v %s", e, stderr.String())
		}
		return out
	}
	dump := run(nil, "pg_dump", "-U", "infrapro_test", "-d", "infrapro_test", "-n", schema, "-Fc")
	run(nil, "createdb", "-U", "infrapro_test", db)
	defer run(nil, "dropdb", "-U", "infrapro_test", db)
	run(nil, "psql", "-v", "ON_ERROR_STOP=1", "-U", "infrapro_test", "-d", db, "-c", "CREATE EXTENSION IF NOT EXISTS btree_gist WITH SCHEMA public")
	run(dump, "pg_restore", "--exit-on-error", "--no-owner", "-U", "infrapro_test", "-d", db)
	u.Path = "/" + db
	pool, e := pgxpool.New(ctx, u.String())
	must(t, e)
	defer pool.Close()
	for _, table := range []string{"spatial_assets", "spatial_asset_versions", "spatial_blobs", "spatial_version_files", "spatial_point_placements", "spatial_component_links", "project_spatial_grants", "project_spatial_views", "spatial_changes", "spatial_receipts", "spatial_processing_jobs"} {
		query := `SELECT md5(COALESCE(jsonb_agg(to_jsonb(r) ORDER BY to_jsonb(r)::text)::text,'[]')) FROM ` + table + ` r`
		var before, after string
		must(t, f.env.Pool.QueryRow(ctx, query).Scan(&before))
		must(t, pool.QueryRow(ctx, query).Scan(&after))
		if before != after {
			t.Fatal("restore metadata mismatch", table)
		}
	}
	// Restore the original objects into a separate bucket; the source remains intact.
	client, e := minio.New("127.0.0.1:58333", &minio.Options{Creds: credentials.NewStaticV4("contract-admin", "contract-admin-test-only", ""), BucketLookup: minio.BucketLookupPath})
	must(t, e)
	bucket := "spatial-restore-" + fmt.Sprintf("%d", time.Now().UnixNano())
	must(t, client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}))
	defer func() {
		for key := range saved {
			_ = client.RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{})
		}
		_ = client.RemoveBucket(ctx, bucket)
	}()
	restored, e := blobstore.Open("http://127.0.0.1:58333", "contract-admin", "contract-admin-test-only", bucket, "spatial")
	must(t, e)
	for key, data := range saved {
		must(t, restored.Put(ctx, key, bytes.NewReader(data), int64(len(data)), "application/octet-stream"))
	}
	check := func(want bool) {
		t.Helper()
		count := 0
		must(t, sp.VerifyContent(ctx, pool, restored, f.tenant, true, "隔离恢复核验", func(r sp.IntegrityResult) error {
			count++
			if r.Available != want || !r.Applied {
				t.Fatalf("integrity %#v", r)
			}
			return nil
		}))
		if count != 1 {
			t.Fatal(count)
		}
	}
	check(true)
	key := keys[0]
	must(t, restored.Delete(ctx, key))
	check(false)
	var available bool
	must(t, pool.QueryRow(ctx, `SELECT content_available FROM spatial_asset_versions WHERE id=$1`, v.ID).Scan(&available))
	if available {
		t.Fatal("missing restored file left readable")
	}
	data := saved[key]
	must(t, restored.Put(ctx, key, bytes.NewReader(data), int64(len(data)), "application/octet-stream"))
	check(true)
	// Temporary network errors do not change a previously verified flag.
	transient := failingSpatialStore{BlobStore: restored}
	must(t, sp.VerifyContent(ctx, pool, transient, f.tenant, true, "网络错误核验", func(r sp.IntegrityResult) error {
		if r.Applied || r.Available {
			t.Fatal(r)
		}
		return nil
	}))
	must(t, pool.QueryRow(ctx, `SELECT content_available FROM spatial_asset_versions WHERE id=$1`, v.ID).Scan(&available))
	if !available {
		t.Fatal("temporary outage quarantined a valid version")
	}
}

type failingSpatialStore struct{ sp.BlobStore }

func (f failingSpatialStore) Get(context.Context, string) (io.ReadCloser, error) {
	return nil, errors.New("temporary network failure")
}
