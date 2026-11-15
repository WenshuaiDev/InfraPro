//go:build monitoringintegration

package resources_test

import (
	"bytes"
	"encoding/csv"
	"errors"
	"io"
	"math/big"
	"strings"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/projects"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/apache/arrow-go/v18/parquet"
	"github.com/apache/arrow-go/v18/parquet/file"
	"github.com/apache/arrow-go/v18/parquet/schema"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func (f monitoringFixture) completeExport(t *testing.T, in md.ExportInput) md.ExportManifest {
	t.Helper()
	result, e := f.service.CreateExport(ctx, f.actor, f.scope, in)
	must(t, e)
	for n := 0; n < 10; n++ {
		job, e := f.service.Job(ctx, f.actor, f.scope, result.JobID)
		must(t, e)
		if job.State == "succeeded" {
			out, e := f.service.ExportFiles(ctx, f.actor, f.scope, result.ID)
			must(t, e)
			return out
		}
		if job.State != "queued" && job.State != "running" {
			t.Fatal(job)
		}
		worked, e := f.service.RunTask(ctx, "export-test-worker", f.tenant, result.JobID)
		must(t, e)
		if !worked {
			t.Fatal("export not claimed")
		}
	}
	t.Fatal("export not complete")
	return md.ExportManifest{}
}
func (f monitoringFixture) exportBytes(t *testing.T, id, part string) []byte {
	t.Helper()
	download, e := f.service.ExportDownload(ctx, f.actor, f.scope, id, part)
	must(t, e)
	var out bytes.Buffer
	must(t, download.Write(ctx, &out))
	if int64(out.Len()) != download.Size {
		t.Fatal("download size")
	}
	return out.Bytes()
}
func TestMonitoringExportFixedSnapshotAndTypedParquet(t *testing.T) {
	f := monitoringSetup(t, func(c *md.StorageConfig) { c.ExportRows = 2 })
	t.Cleanup(func() { verifyTerminalNotifications(t, f.env) })
	a := f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-01T00:00:00.000000001Z,-99999999999999999999999999.000000000001,good\n2024-01-01T00:00:00.000000003Z,99999999999999999999999999.000000000003,good\n2262-04-11T23:47:16.854775807Z,0.000000000001,unmarked\n")))
	input := md.ExportInput{WriteMeta: f.meta("0"), Query: f.query(t), Format: "parquet", Kind: "details"}
	queued, e := f.service.CreateExport(ctx, f.actor, f.scope, input)
	must(t, e)
	again, e := f.service.CreateExport(ctx, f.actor, f.scope, input)
	must(t, e)
	if again.ID != queued.ID {
		t.Fatal("export request duplicated")
	}
	_, e = f.service.ExportFiles(ctx, f.actor, f.scope, queued.ID)
	monitoringCode(t, e, "export_not_ready")
	f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2025-01-01T00:00:00Z,99,good\n")))
	worked, e := f.service.RunTask(ctx, "export-fixed-test", f.tenant, queued.JobID)
	must(t, e)
	if !worked {
		t.Fatal("no export worker")
	}
	manifest, e := f.service.ExportFiles(ctx, f.actor, f.scope, queued.ID)
	must(t, e)
	if manifest.Rows != "3" || len(manifest.Files) != 2 || manifest.ParquetVersion == "" {
		t.Fatal(manifest)
	}
	gotTimes := []int64{}
	gotValues := []*big.Int{}
	for _, part := range manifest.Files {
		r, e := file.NewParquetReader(bytes.NewReader(f.exportBytes(t, manifest.ID, part.Part)))
		must(t, e)
		if !r.MetaData().Schema.Column(9).LogicalType().Equals(schema.NewTimestampLogicalType(true, schema.TimeUnitNanos)) || !r.MetaData().Schema.Column(14).LogicalType().Equals(schema.NewDecimalLogicalType(38, 12)) {
			t.Fatal("native logical types missing")
		}
		for n := 0; n < r.NumRowGroups(); n++ {
			rg := r.RowGroup(n)
			ts, e := rg.Column(9)
			must(t, e)
			times := make([]int64, rg.NumRows())
			_, count, e := ts.(*file.Int64ColumnChunkReader).ReadBatch(rg.NumRows(), times, nil, nil)
			must(t, e)
			gotTimes = append(gotTimes, times[:count]...)
			dc, e := rg.Column(14)
			must(t, e)
			values := make([]parquet.FixedLenByteArray, rg.NumRows())
			defs := make([]int16, rg.NumRows())
			_, count, e = dc.(*file.FixedLenByteArrayColumnChunkReader).ReadBatch(rg.NumRows(), values, defs, nil)
			must(t, e)
			for _, b := range values[:count] {
				c := new(big.Int).SetBytes(b)
				if b[0]&128 != 0 {
					c.Sub(c, new(big.Int).Lsh(big.NewInt(1), 128))
				}
				gotValues = append(gotValues, c)
			}
		}
		must(t, r.Close())
	}
	if len(gotTimes) != 3 || gotTimes[0] != 1704067200000000001 || gotTimes[1] != 1704067200000000003 || gotTimes[2] != 1<<63-1 || gotValues[0].String() != "-99999999999999999999999999000000000001" || gotValues[1].String() != "99999999999999999999999999000000000003" || gotValues[2].String() != "1" {
		t.Fatal(gotTimes, gotValues)
	}
	if !bytes.Contains(f.exportBytes(t, manifest.ID, "manifest"), []byte(`"row_count":"3"`)) {
		t.Fatal("missing manifest")
	}
	preview, e := f.service.PreviewRevokeBatch(ctx, f.actor, f.scope, a.ID, a.Revision)
	must(t, e)
	_, e = f.service.RevokeBatch(ctx, f.actor, f.scope, a.ID, md.RevokeInput{WriteMeta: f.meta(a.Revision), PreviewHash: preview.Hash})
	must(t, e)
	_, e = f.service.ExportDownload(ctx, f.actor, f.scope, manifest.ID, "1")
	monitoringCode(t, e, "export_invalidated")
}
func TestMonitoringCSVSafeAndStatisticsExport(t *testing.T) {
	f := monitoringSetup(t, func(c *md.StorageConfig) { c.ExportRows = 2 })
	var point, facility string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT point_id::text,facility_id::text FROM engineering_channels WHERE id=$1`, f.channel).Scan(&point, &facility))
	_, e := f.write(t, res.Target{Kind: res.Point, Facility: facility, ID: point}, "update", map[string]any{"expected_revision": 1, "name": "=1+1"})
	must(t, e)
	f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-01T00:00:00.000000001Z,-2,good\n2024-01-02T00:00:00Z,4,good\n2024-01-03T00:00:00Z,8,good\n")))
	q := f.query(t)
	for _, mode := range []string{"exact", "spreadsheet_safe"} {
		manifest := f.completeExport(t, md.ExportInput{WriteMeta: f.meta("0"), Query: q, Format: "csv", CSVMode: mode})
		if manifest.Rows != "3" || len(manifest.Files) != 2 {
			t.Fatal(manifest)
		}
		read := csv.NewReader(bytes.NewReader(f.exportBytes(t, manifest.ID, "1")))
		_, e = read.Read()
		must(t, e)
		row, e := read.Read()
		must(t, e)
		want := "=1+1"
		if mode == "spreadsheet_safe" {
			want = "'=1+1"
		}
		if row[2] != want || row[12] != "-2.000000000000" || row[14] != "-2.000000000000" || row[9] != "2024-01-01T00:00:00.000000001Z" {
			t.Fatal(row)
		}
	}
	q.Period = "day"
	q.Limit = 1
	for _, format := range []string{"csv", "parquet"} {
		manifest := f.completeExport(t, md.ExportInput{WriteMeta: f.meta("0"), Query: q, Format: format, Kind: "statistics"})
		if manifest.Rows != "3" || len(manifest.Files) != 2 {
			t.Fatal(manifest)
		}
		data := f.exportBytes(t, manifest.ID, "1")
		if format == "csv" {
			reader := csv.NewReader(bytes.NewReader(data))
			rows, e := reader.ReadAll()
			must(t, e)
			if len(rows) != 3 || rows[1][22] != "-2.000000000000" || rows[1][23] != "0.000000000000" {
				t.Fatal(rows)
			}
		} else {
			r, e := file.NewParquetReader(bytes.NewReader(data))
			must(t, e)
			if r.NumRows() != 2 {
				t.Fatal(r.NumRows())
			}
			must(t, r.Close())
		}
	}
	q.Period = ""
	q.Qualities = []string{"bad"}
	empty := f.completeExport(t, md.ExportInput{WriteMeta: f.meta("0"), Query: q, Format: "csv"})
	reader := csv.NewReader(strings.NewReader(string(f.exportBytes(t, empty.ID, "1"))))
	_, e = reader.Read()
	must(t, e)
	_, e = reader.Read()
	if e != io.EOF || empty.Rows != "0" {
		t.Fatal(empty, e)
	}
}

func TestMonitoringExportCommitFailureAndArchivedProject(t *testing.T) {
	f := monitoringSetup(t)
	t.Cleanup(func() { verifyTerminalNotifications(t, f.env) })
	f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-01T00:00:00Z,2,good\n2024-01-02T00:00:00Z,4,good\n")))
	result, e := f.service.CreateExport(ctx, f.actor, f.scope, md.ExportInput{WriteMeta: f.meta("0"), Query: f.query(t), Format: "csv"})
	must(t, e)
	_, e = f.env.Pool.Exec(ctx, `CREATE FUNCTION test_export_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.manifest_object IS NOT NULL THEN RAISE EXCEPTION 'synthetic export manifest commit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER test_export_failure BEFORE UPDATE ON monitoring_exports FOR EACH ROW EXECUTE FUNCTION test_export_failure()`)
	must(t, e)
	worked, e := f.service.RunTask(ctx, "failed-export-commit", f.tenant, result.JobID)
	if !worked || e == nil {
		t.Fatal("fault did not block manifest publication", worked, e)
	}
	_, e = f.service.ExportFiles(ctx, f.actor, f.scope, result.ID)
	monitoringCode(t, e, "export_not_ready")
	_, e = f.env.Pool.Exec(ctx, `DROP TRIGGER test_export_failure ON monitoring_exports; DROP FUNCTION test_export_failure(); UPDATE monitoring_jobs SET available_at=clock_timestamp()-interval '1 second' WHERE kind='export' AND state='retry_wait'`)
	must(t, e)
	worked, e = f.service.RunTask(ctx, "recovered-export", f.tenant, result.JobID)
	must(t, e)
	if !worked {
		t.Fatal("export retry missing")
	}
	manifest, e := f.service.ExportFiles(ctx, f.actor, f.scope, result.ID)
	must(t, e)
	if manifest.Rows != "2" || len(manifest.Files) != 1 {
		t.Fatal(manifest)
	}
	project := f.project(t, "ArchivedExport")
	var facility string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT facility_id::text FROM engineering_channels WHERE id=$1`, f.channel).Scan(&facility))
	resource := f.accessInput(t, project, facility, "create_link")
	resource.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}, {Kind: res.Channel, ChannelID: &f.channel, Capabilities: []string{"read"}}}
	f.grant(t, project, facility, resource)
	var grant string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM project_resource_grants WHERE project_id=$1 AND channel_id=$2 AND ended_at IS NULL`, project.id, f.channel).Scan(&grant))
	_, e = f.service.GrantData(ctx, f.actor, f.tenant, project.id, md.GrantInput{WriteMeta: f.meta("0"), ChannelID: f.channel, ResourceGrantID: grant, Range: &md.Range{}, Capabilities: []string{"read", "export"}})
	must(t, e)
	ps := projects.New(f.env.Service.Access())
	archive := projects.Change{ChangeMeta: projects.ChangeMeta{WriteMeta: projects.WriteMeta{RequestID: f.wm(t).RequestID, Reason: "验证归档后台导出"}}}
	preview, e := ps.Preview(ctx, project.actor, f.tenant, project.id, "", access.OpProjectArchive, archive)
	must(t, e)
	if !preview.CanSubmit {
		t.Fatal(preview.BlockedReason)
	}
	archive.ProjectVersion, archive.Version, archive.Hash = preview.ProjectVersion, preview.Version, preview.Hash
	_, e = ps.Change(ctx, project.actor, f.tenant, project.id, "", access.OpProjectArchive, archive, access.Meta{})
	must(t, e)
	pf := f
	pf.actor = project.actor
	pf.scope = md.ProjectScope(f.tenant, project.id)
	archived := pf.completeExport(t, md.ExportInput{WriteMeta: pf.meta("0"), Query: pf.query(t), Format: "csv"})
	if archived.Rows != "2" {
		t.Fatal(archived)
	}
	if len(pf.exportBytes(t, archived.ID, "1")) == 0 {
		t.Fatal("archived export empty")
	}
	_, e = f.env.Pool.Exec(ctx, `UPDATE monitoring_exports SET expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`, archived.ID)
	var guard *pgconn.PgError
	if !errors.As(e, &guard) || guard.Code != "23514" {
		t.Fatal("export expiry identity was mutable", e)
	}
	// Simulate seven days passing only in this disposable schema. Restore the
	// exact guard in the same transaction; production never edits the expiry.
	must(t, pgx.BeginFunc(ctx, f.env.Pool, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `ALTER TABLE monitoring_exports DISABLE TRIGGER monitoring_export_source_guard`); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `UPDATE monitoring_exports SET expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`, archived.ID); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, `ALTER TABLE monitoring_exports ENABLE TRIGGER monitoring_export_source_guard`)
		return e
	}))
	_, e = pf.service.ExportFiles(ctx, pf.actor, pf.scope, archived.ID)
	monitoringCode(t, e, "export_expired")
}
