//go:build monitoringintegration

package resources_test

import (
	"bytes"
	"encoding/csv"
	"testing"

	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/apache/arrow-go/v18/parquet"
	"github.com/apache/arrow-go/v18/parquet/file"
	"github.com/apache/arrow-go/v18/parquet/schema"
)

func verifyDerivedExports(t *testing.T, f monitoringFixture, query md.QueryInput) []string {
	t.Helper()
	ids := []string{}
	for _, format := range []string{"csv", "parquet"} {
		manifest := f.completeExport(t, md.ExportInput{WriteMeta: f.meta("0"), Query: query, Format: format, Kind: "details"})
		ids = append(ids, manifest.ID)
		if manifest.Rows != "6" || len(manifest.CalculationHeads) != 1 || len(manifest.CalculationCoverage) != 1 {
			t.Fatal(manifest)
		}
		columns := map[string]int{}
		for i, field := range manifest.Fields {
			columns[field.Name] = i
		}
		derived, raw, nulls := 0, 0, 0
		values := []string{}
		for _, part := range manifest.Files {
			data := f.exportBytes(t, manifest.ID, part.Part)
			if format == "csv" {
				rows, e := csv.NewReader(bytes.NewReader(data)).ReadAll()
				must(t, e)
				for _, row := range rows[1:] {
					if row[columns["source_kind"]] == "derived" {
						derived++
						value := row[columns["value"]]
						values = append(values, value)
						if value == "" {
							nulls++
						}
						if row[columns["scale"]] != "18" || row[columns["generation_id"]] == "" || row[columns["lineage_id"]] == "" || row[columns["observation_id"]] != "" {
							t.Fatal(row)
						}
					} else {
						raw++
					}
				}
			} else {
				reader, e := file.NewParquetReader(bytes.NewReader(data))
				must(t, e)
				if !reader.MetaData().Schema.Column(columns["decimal_value"]).LogicalType().Equals(schema.NewDecimalLogicalType(38, 12)) {
					t.Fatal("raw DECIMAL contract changed")
				}
				if reader.MetaData().Schema.Column(columns["value"]).PhysicalType() != parquet.Types.ByteArray {
					t.Fatal("computed value was narrowed")
				}
				for n := 0; n < reader.NumRowGroups(); n++ {
					group := reader.RowGroup(n)
					kinds, e := group.Column(columns["source_kind"])
					must(t, e)
					kindValues := make([]parquet.ByteArray, group.NumRows())
					_, _, e = kinds.(*file.ByteArrayColumnChunkReader).ReadBatch(group.NumRows(), kindValues, nil, nil)
					must(t, e)
					column, e := group.Column(columns["value"])
					must(t, e)
					text := make([]parquet.ByteArray, group.NumRows())
					defs := make([]int16, group.NumRows())
					_, _, e = column.(*file.ByteArrayColumnChunkReader).ReadBatch(group.NumRows(), text, defs, nil)
					must(t, e)
					valueIndex := 0
					for i, kind := range kindValues {
						value := ""
						if defs[i] > 0 {
							value = string(text[valueIndex])
							valueIndex++
						}
						if string(kind) == "derived" {
							derived++
							values = append(values, value)
							if defs[i] == 0 {
								nulls++
							}
						} else {
							raw++
						}
					}
				}
				must(t, reader.Close())
			}
		}
		if derived != 3 || raw != 3 || nulls != 1 {
			t.Fatal(format, derived, raw, nulls, values)
		}
		hasThree, hasFour := false, false
		for _, v := range values {
			hasThree = hasThree || v == "3.000000000000000000"
			hasFour = hasFour || v == "4.000000000000000000"
		}
		if !hasThree || !hasFour {
			t.Fatal("canonical scale lost", format, values)
		}
	}
	return ids
}

func verifyDerivedOnlyExports(t *testing.T, f monitoringFixture, query md.QueryInput) []string {
	t.Helper()
	ids := []string{}
	for _, kind := range []string{"details", "statistics"} {
		manifest := f.completeExport(t, md.ExportInput{WriteMeta: f.meta("0"), Query: query, Format: "csv", Kind: kind})
		ids = append(ids, manifest.ID)
		expected := "3"
		if kind == "statistics" {
			expected = "1"
		}
		if manifest.Rows != expected || len(manifest.CalculationHeads) != 1 {
			t.Fatal(manifest)
		}
		rows, e := csv.NewReader(bytes.NewReader(f.exportBytes(t, manifest.ID, "1"))).ReadAll()
		must(t, e)
		cols := map[string]int{}
		for i, name := range rows[0] {
			cols[name] = i
		}
		if rows[1][cols["source_kind"]] != "derived" {
			t.Fatal(rows)
		}
		if kind == "statistics" {
			for name, want := range map[string]string{"n": "2", "mean": "3.500000000000", "population_stddev": "0.500000000000", "logical_output_count": "3", "valid_result_count": "2", "no_result_count": "1"} {
				i, ok := cols[name]
				if !ok || rows[1][i] != want {
					t.Fatalf("%s: %v", name, rows)
				}
			}
		}
	}
	return ids
}
