package monitoring

import (
	"bufio"
	"context"
	"encoding/csv"
	"fmt"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
	"io"
	"strings"
	"unicode/utf8"
)

type inputCell struct {
	Text    string
	Numeric bool
	Formula bool
	Error   bool
}
type inputRow struct {
	Line      uint64
	Cells     []inputCell
	EmptyRows uint64
}
type lineCounter struct {
	reader io.Reader
	lines  uint64
	last   byte
	read   bool
}

func (r *lineCounter) Read(p []byte) (int, error) {
	n, e := r.reader.Read(p)
	if n > 0 {
		r.read = true
		r.last = p[n-1]
		for _, b := range p[:n] {
			if b == '\n' {
				r.lines++
			}
		}
	}
	return n, e
}
func (r *lineCounter) total() uint64 {
	if r.read && r.last != '\n' {
		return r.lines + 1
	}
	return r.lines
}

// csv.Reader may buffer part of the next record. The additional 4 KiB is fixed;
// a quoted record cannot grow its internal buffer beyond this record budget.
type recordBudget struct {
	r         io.Reader
	remaining int64
}
type csvRecordBudget struct {
	reader io.Reader
	bytes  int
	quoted bool
}

func (r *csvRecordBudget) Read(p []byte) (int, error) {
	n, e := r.reader.Read(p)
	for _, value := range p[:n] {
		r.bytes++
		if value == '"' {
			r.quoted = !r.quoted
		}
		if value == '\n' && !r.quoted {
			r.bytes = 0
		}
		if r.bytes > 8*1024*1024 {
			return n, problem("record_too_large", "单条记录超过 8 MiB。", 422)
		}
	}
	return n, e
}

func (r *recordBudget) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		return 0, problem("record_too_large", "单条记录超过 8 MiB。", 422)
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	n, e := r.r.Read(p)
	r.remaining -= int64(n)
	return n, e
}
func parseCSV(ctx context.Context, source io.Reader, m Mapping, consume func(inputRow) error) error {
	reader := bufio.NewReader(source)
	if bom, _ := reader.Peek(3); string(bom) == "\xef\xbb\xbf" {
		_, _ = reader.Discard(3)
	}
	var decoded io.Reader = reader
	if m.Encoding == "gb18030" {
		decoded = transform.NewReader(reader, simplifiedchinese.GB18030.NewDecoder())
	}
	physical := &lineCounter{reader: decoded}
	budget := &csvRecordBudget{reader: physical}
	csvReader := csv.NewReader(budget)
	csvReader.Comma = []rune(m.Delimiter)[0]
	csvReader.FieldsPerRecord = -1
	csvReader.ReuseRecord = true
	var logical uint64
	var lastLine uint64
	for {
		if e := ctx.Err(); e != nil {
			return e
		}
		record, e := csvReader.Read()
		if e == io.EOF {
			if total := physical.total(); total > lastLine {
				return consume(inputRow{Line: lastLine + 1, EmptyRows: total - lastLine})
			}
			return nil
		}
		if e != nil {
			return fmt.Errorf("CSV record %d: %w", logical+1, e)
		}
		logical++
		line, _ := csvReader.FieldPos(0)
		if uint64(line) > lastLine+1 {
			if e = consume(inputRow{Line: lastLine + 1, EmptyRows: uint64(line) - lastLine - 1}); e != nil {
				return e
			}
		}
		row := inputRow{Line: uint64(line), Cells: make([]inputCell, len(record))}
		lastLine = uint64(line)
		for i, value := range record {
			if len(value) > 1024*1024 || !utf8.ValidString(value) || strings.ContainsRune(value, '\ufffd') {
				return invalid("file", "单元格超过 1 MiB 或包含无效编码。")
			}
			row.Cells[i] = inputCell{Text: value}
			lastLine += uint64(strings.Count(value, "\n"))
		}
		if e = consume(row); e != nil {
			return e
		}
	}
}
