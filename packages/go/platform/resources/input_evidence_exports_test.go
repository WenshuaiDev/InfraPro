//go:build monitoringintegration

package resources_test

import (
	"bytes"
	"encoding/csv"
	"math/big"
	"strconv"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/apache/arrow-go/v18/parquet"
	"github.com/apache/arrow-go/v18/parquet/file"
)

func verifyInputEvidenceFile(t *testing.T, manifest md.ResultExportManifest, download func(string) (md.Download, error), values []string) {
	t.Helper()
	if manifest.Schema != "calculation-input-evidence/1" || manifest.Rows != strconv.Itoa(len(values)) {
		t.Fatal("full candidate evidence missing", manifest.Schema, manifest.Rows)
	}
	expected := map[string]int{}
	for _, v := range values {
		if v == "" {
			expected["null"]++
			continue
		}
		r, ok := new(big.Rat).SetString(v)
		if !ok {
			t.Fatal(v)
		}
		expected[r.RatString()]++
	}
	columns := map[string]int{}
	for i, f := range manifest.Fields {
		columns[f.Name] = i
	}
	actual := map[string]int{}
	add := func(v string) {
		if v == "" {
			actual["null"]++
			return
		}
		r, ok := new(big.Rat).SetString(v)
		if !ok {
			t.Fatal("exact evidence value invalid", v)
		}
		actual[r.RatString()]++
	}
	for _, part := range manifest.Files {
		d, e := download(part.Part)
		must(t, e)
		var buf bytes.Buffer
		must(t, d.Write(ctx, &buf))
		if manifest.Format == "csv" {
			rows, e := csv.NewReader(&buf).ReadAll()
			must(t, e)
			for _, row := range rows[1:] {
				if row[columns["evidence_path"]] == "" || row[columns["input_snapshot_id"]] == "" || row[columns["source_kind"]] == "direct" && (row[columns["adoption_id"]] == "" || row[columns["observation_id"]] == "") || row[columns["source_kind"]] == "derived" && (row[columns["result_id"]] == "" || row[columns["generation_id"]] == "") {
					t.Fatal("evidence lost source identities", row)
				}
				add(row[columns["value"]])
			}
		} else {
			r, e := file.NewParquetReader(bytes.NewReader(buf.Bytes()))
			must(t, e)
			if r.MetaData().Schema.Column(columns["value"]).PhysicalType() != parquet.Types.ByteArray {
				t.Fatal("input evidence narrowed decimal")
			}
			for i := 0; i < r.NumRowGroups(); i++ {
				g := r.RowGroup(i)
				col, e := g.Column(columns["value"])
				must(t, e)
				v := make([]parquet.ByteArray, g.NumRows())
				defs := make([]int16, g.NumRows())
				_, count, e := col.(*file.ByteArrayColumnChunkReader).ReadBatch(g.NumRows(), v, defs, nil)
				must(t, e)
				for n := int64(count); n < g.NumRows(); n++ {
					add("")
				}
				for _, text := range v[:count] {
					add(string(text))
				}
			}
			must(t, r.Close())
		}
	}
	if len(actual) != len(expected) {
		t.Fatal("evidence candidate values differ", actual, expected)
	}
	for v, n := range expected {
		if actual[v] != n {
			t.Fatal("evidence count/value mismatch", v, actual[v], n)
		}
	}
}
func verifyAnalysisInputEvidence(t *testing.T, f monitoringFixture, s *dm.Service, artifact, version string, history bool) []string {
	t.Helper()
	scope := dm.TenantScope(f.tenant)
	ids := []string{}
	for _, format := range []string{"csv", "parquet"} {
		in := dm.AnalysisExportInput{WriteMeta: restoreMeta(""), VersionID: version, Content: "inputs", Format: format, History: history}
		record, e := s.CreateAnalysisExport(ctx, f.actor, scope, artifact, in, access.Meta{})
		must(t, e)
		_, e = f.service.RunTask(ctx, "analysis-full-input-evidence", f.tenant, record.Job.ID)
		must(t, e)
		manifest, e := s.AnalysisExportFiles(ctx, f.actor, scope, record.ID)
		must(t, e)
		verifyInputEvidenceFile(t, manifest, func(part string) (md.Download, error) {
			return s.AnalysisExportDownload(ctx, f.actor, scope, record.ID, part)
		}, []string{"2", "5", "9"})
		ids = append(ids, record.ID)
	}
	return ids
}

func verifyGenerationInputEvidence(t *testing.T, f monitoringFixture, s *dm.Service, generation string, values []string) {
	t.Helper()
	scope := dm.TenantScope(f.tenant)
	for _, format := range []string{"csv", "parquet"} {
		record, e := s.CreateGenerationExport(ctx, f.actor, scope, generation, dm.GenerationExportInput{WriteMeta: restoreMeta(""), Format: format}, access.Meta{})
		must(t, e)
		_, e = f.service.RunTask(ctx, "complete-generation-inputs", f.tenant, record.Job.ID)
		must(t, e)
		manifest, e := s.GenerationExportFiles(ctx, f.actor, scope, record.ID)
		must(t, e)
		verifyInputEvidenceFile(t, manifest, func(part string) (md.Download, error) {
			return s.GenerationExportDownload(ctx, f.actor, scope, record.ID, part)
		}, values)
	}
}
