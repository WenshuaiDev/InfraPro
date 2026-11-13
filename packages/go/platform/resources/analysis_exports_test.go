//go:build monitoringintegration

package resources_test

import (
	"bytes"
	"encoding/csv"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	"github.com/apache/arrow-go/v18/parquet"
	"github.com/apache/arrow-go/v18/parquet/file"
	"github.com/google/uuid"
)

func verifyAnalysisExports(t *testing.T, f monitoringFixture, s *dm.Service, a access.Actor, scope dm.Scope, artifact, version, count, firstValue string, history bool) []string {
	t.Helper()
	ids := []string{}
	for _, format := range []string{"csv", "parquet"} {
		in := dm.AnalysisExportInput{WriteMeta: dm.WriteMeta{RequestID: uuid.NewString(), Reason: "固定成果导出往返验证"}, VersionID: version, Format: format, History: history}
		record, e := s.CreateAnalysisExport(ctx, a, scope, artifact, in, access.Meta{})
		must(t, e)
		again, e := s.CreateAnalysisExport(ctx, a, scope, artifact, in, access.Meta{})
		must(t, e)
		if again.ID != record.ID {
			t.Fatal("idempotent export duplicated")
		}
		_, e = f.service.RunTask(ctx, "analysis-export", f.tenant, record.Job.ID)
		must(t, e)
		manifest, e := s.AnalysisExportFiles(ctx, a, scope, record.ID)
		must(t, e)
		if manifest.Rows != count || manifest.SourceVersion != version || manifest.SourceKind != "analysis" {
			t.Fatal(manifest)
		}
		columns := map[string]int{}
		for i, field := range manifest.Fields {
			columns[field.Name] = i
		}
		found := false
		for _, part := range manifest.Files {
			download, e := s.AnalysisExportDownload(ctx, a, scope, record.ID, part.Part)
			must(t, e)
			var buf bytes.Buffer
			must(t, download.Write(ctx, &buf))
			if format == "csv" {
				rows, e := csv.NewReader(bytes.NewReader(buf.Bytes())).ReadAll()
				must(t, e)
				for _, row := range rows[1:] {
					if row[columns["value"]] == firstValue {
						found = true
					}
					if row[columns["artifact_version_id"]] != version || row[columns["lineage_ref"]] == "" || row[columns["rounding"]] != "half_even" {
						t.Fatal(row)
					}
				}
			} else {
				reader, e := file.NewParquetReader(bytes.NewReader(buf.Bytes()))
				must(t, e)
				if reader.MetaData().Schema.Column(columns["value"]).PhysicalType() != parquet.Types.ByteArray {
					t.Fatal("analysis numeric value narrowed")
				}
				for n := 0; n < reader.NumRowGroups(); n++ {
					group := reader.RowGroup(n)
					col, e := group.Column(columns["value"])
					must(t, e)
					values := make([]parquet.ByteArray, group.NumRows())
					defs := make([]int16, group.NumRows())
					_, _, e = col.(*file.ByteArrayColumnChunkReader).ReadBatch(group.NumRows(), values, defs, nil)
					must(t, e)
					for _, value := range values {
						if string(value) == firstValue {
							found = true
						}
					}
				}
				must(t, reader.Close())
			}
		}
		if !found {
			t.Fatal("canonical exact value not preserved", format, firstValue)
		}
		ids = append(ids, record.ID)
	}
	page, e := s.AnalysisExports(ctx, a, scope, dm.MetricFilter{Limit: 1})
	must(t, e)
	found := map[string]bool{}
	for {
		for _, record := range page.Items {
			found[record.ID] = true
		}
		if page.NextCursor == "" {
			break
		}
		page, e = s.AnalysisExports(ctx, a, scope, dm.MetricFilter{Limit: 1, Cursor: page.NextCursor})
		must(t, e)
	}
	for _, id := range ids {
		if !found[id] {
			t.Fatal("export cannot be recovered from task list", id)
		}
	}
	return ids
}

func forbidAnalysisExport(t *testing.T, s *dm.Service, a access.Actor, scope dm.Scope, artifact, version string, history bool) {
	t.Helper()
	_, e := s.CreateAnalysisExport(ctx, a, scope, artifact, dm.AnalysisExportInput{WriteMeta: dm.WriteMeta{RequestID: uuid.NewString()}, VersionID: version, Format: "csv", History: history}, access.Meta{})
	if e == nil {
		t.Fatal("unauthorized analysis export accepted")
	}
}
