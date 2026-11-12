package monitoring

import (
	"context"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestXLSXMappedFormulaRangesAndTextPrecision(t *testing.T) {
	for _, kind := range []string{"unmapped", "mapped", "array"} {
		t.Run(kind, func(t *testing.T) {
			workbook := excelize.NewFile()
			defer workbook.Close()
			set := func(cell, value string) {
				t.Helper()
				if e := workbook.SetCellStr("Sheet1", cell, value); e != nil {
					t.Fatal(e)
				}
			}
			set("A1", "observed_at")
			set("B1", "value")
			set("C1", "ignored")
			set("A2", "2262-04-11T23:47:16.854775807Z")
			set("B2", "10000000000000000000000000.000000000001")
			cell := "C2"
			if kind == "mapped" {
				cell = "B2"
			}
			var opts []excelize.FormulaOpts
			if kind == "array" {
				typ, ref := "array", "B2:C4"
				opts = []excelize.FormulaOpts{{Type: &typ, Ref: &ref}}
			}
			if e := workbook.SetCellFormula("Sheet1", cell, "1+1", opts...); e != nil {
				t.Fatal(e)
			}
			data, e := workbook.WriteToBuffer()
			if e != nil {
				t.Fatal(e)
			}
			temp, e := newSpool(t.TempDir())
			if e != nil {
				t.Fatal(e)
			}
			defer temp.close()
			mapping := Mapping{Format: "xlsx", Mode: "single", Sheet: "Sheet1", HeaderRow: 1, TimeColumn: "observed_at", ValueColumn: "value"}
			var date1904 bool
			var rows []inputRow
			e = parseXLSX(t.Context(), data, mapping, temp, &date1904, func(row inputRow) error { rows = append(rows, row); return nil })
			if kind != "unmapped" {
				if e == nil {
					t.Fatal("mapped formula range accepted")
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			if len(rows) != 2 || rows[1].Cells[1].Numeric || rows[1].Cells[1].Text != "10000000000000000000000000.000000000001" || date1904 {
				t.Fatal(rows, date1904)
			}
		})
	}
}

func TestExternalSortMultipleRunsPreservesEveryRow(t *testing.T) {
	temp, e := newSpool(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer temp.close()
	sorter := rowSorter{s: temp, compare: compareRaw}
	const count = 33000
	for i := count - 1; i >= 0; i-- {
		if e = sorter.add(rawObservation{Channel: "channel", At: int64(i), Line: uint64(i + 1), Value: "0.000000000001"}); e != nil {
			t.Fatal(e)
		}
	}
	file, e := sorter.finish(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	it, e := openRows(file)
	if e != nil {
		t.Fatal(e)
	}
	defer it.close()
	n := int64(0)
	for it.next() {
		if it.row.At != n {
			t.Fatal("sort lost/duplicated an observation", n, it.row.At)
		}
		n++
	}
	if it.err != nil || n != count {
		t.Fatal(n, it.err)
	}
}

func TestCSVRejectsOversizeQuotedRecordAndInvalidEncoding(t *testing.T) {
	for _, input := range []string{"time,value\nnow,\xff\n", "time,value\nnow,\"" + strings.Repeat("a", 9*1024*1024) + "\"\n"} {
		if e := parseCSV(t.Context(), strings.NewReader(input), Mapping{Encoding: "utf-8", Delimiter: ","}, func(inputRow) error { return nil }); e == nil {
			t.Fatal("unbounded or invalid record accepted")
		}
	}
	var mapping MappingInput
	if e := DecodeRequest([]byte(`{"mapping":{"quality_map":{"from":null}}}`), &mapping); e == nil {
		t.Fatal("map null was confused with an unbounded range")
	}
	var query QueryInput
	if e := DecodeRequest([]byte(`{"snapshot_id":null,"all_history":true}`), &query); e != nil {
		t.Fatal("explicit initial snapshot rejected", e)
	}
}

func TestCSVBlankRowsDoNotCountQuotedNewlines(t *testing.T) {
	var blank, records uint64
	var lines []uint64
	e := parseCSV(t.Context(), strings.NewReader("time,value\n\n\"two\nlines\",1\n\n"), Mapping{Encoding: "utf-8", Delimiter: ","}, func(row inputRow) error {
		if row.EmptyRows > 0 {
			blank += row.EmptyRows
		} else {
			records++
			lines = append(lines, row.Line)
		}
		return nil
	})
	if e != nil || blank != 2 || records != 2 || len(lines) != 2 || lines[1] != 3 {
		t.Fatal(blank, records, lines, e)
	}
}
