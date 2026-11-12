package monitoring

import (
	"archive/zip"
	"context"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"strings"
)

type xlsxCell struct {
	Ref     string `xml:"r,attr"`
	Type    string `xml:"t,attr"`
	Value   string `xml:"v"`
	Formula *struct {
		Ref string `xml:"ref,attr"`
	} `xml:"f"`
	Inline xlsxText `xml:"is"`
}
type xlsxText struct {
	Text string `xml:"t"`
	Runs []struct {
		Text string `xml:"t"`
	} `xml:"r"`
}

func (t xlsxText) text() (string, error) {
	var b strings.Builder
	b.WriteString(t.Text)
	for _, r := range t.Runs {
		b.WriteString(r.Text)
		if b.Len() > 1024*1024 {
			return "", invalid("cell", "单元格超过 1 MiB。")
		}
	}
	if b.Len() > 1024*1024 {
		return "", invalid("cell", "单元格超过 1 MiB。")
	}
	return b.String(), nil
}

type sharedStrings struct {
	file    *spoolFile
	offsets []int64
}

func (s *sharedStrings) get(index string) (string, error) {
	i, e := strconv.ParseUint(index, 10, 64)
	if e != nil || i >= uint64(len(s.offsets)) {
		return "", invalid("xlsx", "共享字符串索引无效。")
	}
	var size [4]byte
	if _, e = s.file.ReadAt(size[:], s.offsets[i]); e != nil {
		return "", e
	}
	n := binary.LittleEndian.Uint32(size[:])
	if n > 1024*1024 {
		return "", invalid("cell", "单元格超过 1 MiB。")
	}
	value := make([]byte, n)
	_, e = s.file.ReadAt(value, s.offsets[i]+4)
	return string(value), e
}
func xmlReader(file *zip.File) (io.ReadCloser, *xml.Decoder, *recordBudget, error) {
	r, e := file.Open()
	if e != nil {
		return nil, nil, nil, e
	}
	budget := &recordBudget{r: r, remaining: 8 * 1024 * 1024}
	return r, xml.NewDecoder(budget), budget, nil
}
func parseXLSX(ctx context.Context, source io.Reader, m Mapping, temp *spool, date1904 *bool, consume func(inputRow) error, wholeSource ...*bool) error {
	file, e := temp.file()
	if e != nil {
		return e
	}
	defer file.Close()
	n, e := io.Copy(file, io.LimitReader(source, MaxXLSXBytes+1))
	if e != nil {
		return e
	}
	if n > MaxXLSXBytes {
		return invalid("file", "XLSX 压缩文件超过 512 MiB。")
	}
	archive, e := zip.NewReader(file.File, n)
	if e != nil {
		return invalid("file", "无效 XLSX ZIP。")
	}
	entries := map[string]*zip.File{}
	var total uint64
	for _, entry := range archive.File {
		if path.Clean(entry.Name) != entry.Name || strings.HasPrefix(entry.Name, "/") || strings.Contains(entry.Name, "\\") || strings.Contains(entry.Name, "..") || entries[entry.Name] != nil {
			return invalid("file", "XLSX 包路径无效或重复。")
		}
		total += entry.UncompressedSize64
		if total > uint64(MaxTemporaryBytes) {
			return invalid("file", "XLSX 展开大小超过 8 GiB。")
		}
		if strings.HasSuffix(strings.ToLower(entry.Name), "vbaproject.bin") {
			return invalid("file", "首版不接受含宏的工作簿。")
		}
		entries[entry.Name] = entry
	}
	readXML := func(name string, out any) error {
		f := entries[name]
		if f == nil {
			return invalid("file", "XLSX 缺少必需部件。")
		}
		r, e := f.Open()
		if e != nil {
			return e
		}
		defer r.Close()
		return xml.NewDecoder(io.LimitReader(r, 8*1024*1024)).Decode(out)
	}
	var workbook struct {
		Properties struct {
			Date1904 string `xml:"date1904,attr"`
		} `xml:"workbookPr"`
		Sheets []struct {
			Name string `xml:"name,attr"`
			ID   string `xml:"id,attr"`
		} `xml:"sheets>sheet"`
	}
	if e = readXML("xl/workbook.xml", &workbook); e != nil {
		return e
	}
	*date1904 = workbook.Properties.Date1904 == "1" || workbook.Properties.Date1904 == "true"
	if len(wholeSource) > 0 && len(workbook.Sheets) != 1 {
		*wholeSource[0] = false
	}
	var rid string
	for _, sheet := range workbook.Sheets {
		if sheet.Name == m.Sheet {
			rid = sheet.ID
		}
	}
	if rid == "" {
		return invalid("sheet", "工作表不存在。")
	}
	var rels struct {
		Items []struct {
			ID     string `xml:"Id,attr"`
			Target string `xml:"Target,attr"`
			Mode   string `xml:"TargetMode,attr"`
		} `xml:"Relationship"`
	}
	if e = readXML("xl/_rels/workbook.xml.rels", &rels); e != nil {
		return e
	}
	target := ""
	for _, rel := range rels.Items {
		if rel.ID == rid {
			if rel.Mode == "External" {
				return invalid("sheet", "不允许外部工作表。")
			}
			target = path.Clean(path.Join("xl", rel.Target))
			if strings.HasPrefix(rel.Target, "/") {
				target = strings.TrimPrefix(rel.Target, "/")
			}
		}
	}
	sheet := entries[target]
	if sheet == nil || !strings.HasPrefix(target, "xl/worksheets/") {
		return invalid("sheet", "工作表关系无效。")
	}
	sharedFile, e := temp.file()
	if e != nil {
		return e
	}
	defer sharedFile.Close()
	shared := sharedStrings{file: sharedFile}
	if f := entries["xl/sharedStrings.xml"]; f != nil {
		r, d, b, e := xmlReader(f)
		if e != nil {
			return e
		}
		defer r.Close()
		for {
			if e = ctx.Err(); e != nil {
				return e
			}
			b.remaining = 8 * 1024 * 1024
			token, e := d.Token()
			if e == io.EOF {
				break
			}
			if e != nil {
				return e
			}
			start, ok := token.(xml.StartElement)
			if !ok || start.Name.Local != "si" {
				continue
			}
			var text xlsxText
			if e = d.DecodeElement(&text, &start); e != nil {
				return e
			}
			value, e := text.text()
			if e != nil {
				return e
			}
			if len(shared.offsets) >= 8000000 {
				return invalid("file", "共享字符串索引超过本机任务保护值。")
			}
			offset, e := sharedFile.Seek(0, io.SeekCurrent)
			if e != nil {
				return e
			}
			shared.offsets = append(shared.offsets, offset)
			var size [4]byte
			binary.LittleEndian.PutUint32(size[:], uint32(len(value)))
			if _, e = sharedFile.Write(size[:]); e != nil {
				return e
			}
			if _, e = io.WriteString(sharedFile, value); e != nil {
				return e
			}
		}
	}
	decode := func(c xlsxCell) (inputCell, error) {
		out := inputCell{Formula: c.Formula != nil, Error: c.Type == "e"}
		var e error
		switch c.Type {
		case "s":
			out.Text, e = shared.get(c.Value)
		case "inlineStr":
			out.Text, e = c.Inline.text()
		case "", "n":
			out.Text = c.Value
			out.Numeric = c.Value != ""
		case "str", "d", "b", "e":
			out.Text = c.Value
		default:
			return out, invalid("cell", "不支持的 XLSX 单元格类型。")
		}
		return out, e
	}
	// First pass identifies mapped columns and rejects direct, shared and array
	// formulas whose declared ranges intersect any mapped business cell.
	mapped := map[int]bool{}
	formulaRanges := [][4]int{}
	scan := func(first bool) error {
		r, d, b, e := xmlReader(sheet)
		if e != nil {
			return e
		}
		defer r.Close()
		lastRow := uint64(0)
		for {
			if e = ctx.Err(); e != nil {
				return e
			}
			b.remaining = 8 * 1024 * 1024
			token, e := d.Token()
			if e == io.EOF {
				break
			}
			if e != nil {
				return e
			}
			start, ok := token.(xml.StartElement)
			if !ok || start.Name.Local != "row" {
				continue
			}
			var row struct {
				Number uint64     `xml:"r,attr"`
				Cells  []xlsxCell `xml:"c"`
			}
			if e = d.DecodeElement(&row, &start); e != nil {
				return e
			}
			if row.Number == 0 || row.Number <= lastRow || row.Number > 1048576 {
				return invalid("sheet", "工作表行号无效。")
			}
			lastRow = row.Number
			out := inputRow{Line: row.Number}
			seen := map[int]bool{}
			for _, c := range row.Cells {
				column, rowIndex, e := cellAddress(c.Ref)
				if e != nil || rowIndex != int(row.Number) || seen[column] {
					return invalid("cell", "单元格地址无效或重复。")
				}
				seen[column] = true
				if column >= 16384 {
					return invalid("cell", "列数超过 XLSX 上限。")
				}
				for len(out.Cells) <= column {
					out.Cells = append(out.Cells, inputCell{})
				}
				out.Cells[column], e = decode(c)
				if e != nil {
					return e
				}
				if first && c.Formula != nil {
					rangeValue := [4]int{column, int(row.Number), column, int(row.Number)}
					if c.Formula.Ref != "" {
						rangeValue, e = cellRange(c.Formula.Ref)
						if e != nil {
							return e
						}
					}
					if len(formulaRanges) >= 100000 {
						return invalid("sheet", "公式范围超过扫描预算。")
					}
					formulaRanges = append(formulaRanges, rangeValue)
				}
			}
			if first && row.Number == uint64(m.HeaderRow) {
				names := map[string]bool{}
				for _, name := range []string{m.ChannelColumn, m.TimeColumn, m.ValueColumn, m.UnitColumn, m.QualityColumn, m.RecordColumn, m.IntervalColumn} {
					if name != "" {
						names[name] = true
					}
				}
				for _, binding := range m.Channels {
					if m.Mode == "wide" {
						names[binding.ValueColumn] = true
					}
				}
				for column, c := range out.Cells {
					if names[c.Text] {
						mapped[column] = true
						if c.Formula {
							return invalid("header", "映射表头不能使用公式缓存。")
						}
					}
				}
			}
			if !first {
				if e = consume(out); e != nil {
					return e
				}
			}
		}
		return nil
	}
	if e = scan(true); e != nil {
		return e
	}
	for _, ref := range formulaRanges {
		if ref[3] < m.HeaderRow {
			continue
		}
		for column := range mapped {
			if column >= ref[0] && column <= ref[2] {
				return invalid("sheet", fmt.Sprintf("映射区域含公式范围（行 %d–%d）；不接受公式缓存。", ref[1], ref[3]))
			}
		}
	}
	return scan(false)
}
func cellAddress(ref string) (int, int, error) {
	ref = strings.ReplaceAll(ref, "$", "")
	split := 0
	column := 0
	for split < len(ref) && ref[split] >= 'A' && ref[split] <= 'Z' {
		column = column*26 + int(ref[split]-'A'+1)
		split++
	}
	row, e := strconv.Atoi(ref[split:])
	if split == 0 || e != nil || column < 1 || column > 16384 || row < 1 || row > 1048576 {
		return 0, 0, invalid("cell", "无效单元格地址。")
	}
	return column - 1, row, nil
}
func cellRange(ref string) ([4]int, error) {
	parts := strings.Split(ref, ":")
	if len(parts) > 2 {
		return [4]int{}, invalid("formula", "无效公式范围。")
	}
	a, b, e := cellAddress(parts[0])
	if e != nil {
		return [4]int{}, e
	}
	c, d := a, b
	if len(parts) == 2 {
		c, d, e = cellAddress(parts[1])
	}
	if e != nil || c < a || d < b {
		return [4]int{}, invalid("formula", "无效公式范围。")
	}
	return [4]int{a, b, c, d}, nil
}

// Keep the source reader contract explicit for zip.ReaderAt callers.
var _ io.ReaderAt = (*os.File)(nil)
