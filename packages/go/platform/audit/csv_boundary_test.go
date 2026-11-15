package audit

import (
	"bytes"
	"encoding/csv"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestCSVExactHundredMiBBoundary(t *testing.T) {
	const ceiling = int64(100 << 20)
	w := boundedWriter{w: io.Discard, max: ceiling}
	chunk := make([]byte, 1<<20)
	for i := 0; i < 100; i++ {
		n, e := w.Write(chunk)
		if e != nil || n != len(chunk) {
			t.Fatal("100 MiB boundary rejected early", i, n, e)
		}
	}
	if w.n != ceiling {
		t.Fatal("byte accounting drift")
	}
	n, e := w.Write([]byte{0})
	var problem *Error
	if n != 0 || !errors.As(e, &problem) || problem.Code != "audit_export_too_large" || w.n != ceiling {
		t.Fatal("byte above 100 MiB was accepted", n, e, w.n)
	}
}
func TestCSVFormulaAndQuotedNewlineCells(t *testing.T) {
	original := []string{"=1+1", " +2", "-3", "@SUM(1)", "\tformula", "\rformula", "中文,引号\"和\n换行", ""}
	cells := make([]string, len(original))
	for i, v := range original {
		cells[i] = csvCell(v)
	}
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	w.UseCRLF = true
	if e := w.Write(cells); e != nil {
		t.Fatal(e)
	}
	w.Flush()
	if e := w.Error(); e != nil {
		t.Fatal(e)
	}
	got, e := csv.NewReader(&b).Read()
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 6; i++ {
		if !strings.HasPrefix(got[i], "'") {
			t.Fatal("active spreadsheet formula", got[i])
		}
	}
	if got[6] != original[6] || got[7] != "" {
		t.Fatal("quoted/newline/empty cell changed", got)
	}
}
