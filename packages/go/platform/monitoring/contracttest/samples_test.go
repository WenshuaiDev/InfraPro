//go:build monitoringintegration

package contracttest

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

func TestSyntheticSampleFiles(t *testing.T) {
	dir := os.Getenv("INFRAPRO_CONTRACT_ARTIFACTS")
	if dir == "" {
		dir = t.TempDir()
	}
	dir = filepath.Join(dir, "samples")
	must(t, os.MkdirAll(dir, 0700))
	manifest := map[string]any{"origin": "synthetic; no real tenant data", "seed": "20261112", "timezone": "Asia/Shanghai", "files": map[string]any{}}
	record := func(name string) {
		b, err := os.ReadFile(filepath.Join(dir, name))
		must(t, err)
		manifest["files"].(map[string]any)[name] = map[string]any{"sha256": fmt.Sprintf("%x", sha256.Sum256(b)), "bytes": len(b)}
	}
	writeCSV := func(name string, rows [][]string) {
		f, err := os.Create(filepath.Join(dir, name))
		must(t, err)
		w := csv.NewWriter(f)
		must(t, w.WriteAll(rows))
		must(t, f.Close())
		record(name)
	}
	header := []string{"channel_key", "observed_at", "value", "quality", "unit"}
	stats := [][]string{header, {"temperature", "2024-01-01T00:00:00+08:00", "-2", "good", "degC"}, {"temperature", "2024-01-01T00:00:01+08:00", "-1", "", "degC"}, {"temperature", "2024-01-01T00:00:02+08:00", "5", "suspect", "degC"}, {"temperature", "2024-01-01T00:00:03+08:00", "99", "bad", "degC"}}
	writeCSV("statistics-a.csv", stats)
	writeCSV("statistics-b.csv", [][]string{header, {"temperature", "2024-01-01T00:00:04+08:00", "-3", "good", "degC"}})
	base := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, kind := range []string{"a-100", "b-80-duplicates-20-new", "one-invalid-time", "two-outside-bounds", "different-value-conflict"} {
		rows := [][]string{header}
		for i := 0; i < 100; i++ {
			index := i
			if kind == "b-80-duplicates-20-new" {
				index += 20
			}
			tm := base.Add(time.Duration(index) * time.Second).Format(time.RFC3339Nano)
			value := fmt.Sprint(index%10 - 5)
			if kind == "one-invalid-time" && i == 99 {
				tm = "2020-99-99T00:00:00Z"
			}
			if kind == "two-outside-bounds" && i >= 98 {
				value = "99"
			}
			if kind == "different-value-conflict" && i == 0 {
				value = "-999"
			}
			rows = append(rows, []string{"temperature", tm, value, "", "degC"})
		}
		writeCSV(kind+".csv", rows)
	}
	for _, system := range []bool{false, true} {
		f := excelize.NewFile()
		must(t, f.SetWorkbookProps(&excelize.WorkbookPropsOptions{Date1904: &system}))
		must(t, f.SetSheetName("Sheet1", "observations"))
		rows := [][]string{{"observed_at", "temperature", "counter"}, {"2024-02-29T00:00:00.123456789+08:00", "10000000000000000000000000.000000000001", "9223372036854775807"}, {"2262-04-11T23:47:16.854775807Z", "10000000000000000000000000.000000000003", "-9223372036854775808"}}
		for y, row := range rows {
			for x, value := range row {
				axis, err := excelize.CoordinatesToCellName(x+1, y+1)
				must(t, err)
				must(t, f.SetCellStr("observations", axis, value))
			}
		}
		name := fmt.Sprintf("precision-wide-date1904-%t.xlsx", system)
		must(t, f.SaveAs(filepath.Join(dir, name)))
		must(t, f.Close())
		record(name)
		read, err := excelize.OpenFile(filepath.Join(dir, name), excelize.Options{RawCellValue: true, UnzipSizeLimit: 16 << 20, UnzipXMLSizeLimit: 1 << 20})
		must(t, err)
		props, err := read.GetWorkbookProps()
		must(t, err)
		if (props.Date1904 != nil && *props.Date1904) != system {
			t.Fatal("date system lost")
		}
		stream, err := read.Rows("observations")
		must(t, err)
		n := 0
		for stream.Next() {
			got, e := stream.Columns(excelize.Options{RawCellValue: true})
			must(t, e)
			if len(got) != len(rows[n]) {
				t.Fatal("column count")
			}
			for i, value := range got {
				if value != rows[n][i] {
					t.Fatal("XLSX text precision lost")
				}
			}
			n++
		}
		must(t, stream.Error())
		must(t, stream.Close())
		must(t, read.Close())
		if n != len(rows) {
			t.Fatal("row count")
		}
	}
	b, err := json.MarshalIndent(manifest, "", "  ")
	must(t, err)
	must(t, os.WriteFile(filepath.Join(dir, "manifest.json"), b, 0600))
	t.Log("SYNTHETIC_SAMPLES", dir)
}
