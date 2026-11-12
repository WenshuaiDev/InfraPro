package monitoring

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

type PreviewSheet struct {
	Name      string `json:"name"`
	Dimension string `json:"dimension"`
}
type SourcePreview struct {
	Available bool           `json:"available"`
	Format    string         `json:"format"`
	Sheet     string         `json:"sheet"`
	Sheets    []PreviewSheet `json:"sheets"`
	Rows      [][]string     `json:"rows"`
	Truncated bool           `json:"truncated"`
	Notice    string         `json:"notice"`
}
type boundedReaderAt struct {
	reader    io.ReaderAt
	remaining int64
}

func (r *boundedReaderAt) ReadAt(p []byte, offset int64) (int, error) {
	if int64(len(p)) > r.remaining {
		return 0, io.ErrShortBuffer
	}
	n, e := r.reader.ReadAt(p, offset)
	r.remaining -= int64(n)
	return n, e
}

func (s *Service) sourcePreview(ctx context.Context, a access.Actor, scope Scope, b ImportBatch) *SourcePreview {
	out := &SourcePreview{Format: strings.TrimPrefix(strings.ToLower(filepath.Ext(b.Name)), "."), Sheets: []PreviewSheet{}, Rows: [][]string{}, Truncated: true, Notice: "预览仅用于配置映射，不能代替全部数据校验。"}
	if s.storage == nil {
		out.Notice = "预览存储暂不可用。"
		return out
	}
	var key, revision string
	e := s.transact(ctx, a, scope, "imports.mapping", false, false, func(tx pgx.Tx, auth authorization) error {
		current, e := batchTx(ctx, tx, auth, b.ID, false)
		if e != nil {
			return e
		}
		if current.Revision != b.Revision || current.Creator != a.ID || current.State != "received" || current.ReadoptBatch != "" {
			return notFound()
		}
		revision = authorizationVersion(auth)
		return tx.QueryRow(ctx, `SELECT object_key FROM monitoring_source_files WHERE tenant_id=$1 AND id=$2`, scope.Tenant, b.SourceFile).Scan(&key)
	})
	if e != nil {
		out.Notice = "当前资格或状态不允许配置原文件映射。"
		return out
	}
	object, e := s.storage.get(ctx, key)
	if e != nil {
		out.Notice = "原文件预览暂不可用，请稍后重试。"
		return out
	}
	defer object.Close()
	if out.Format == "csv" {
		mapping := Mapping{Encoding: "utf-8", Delimiter: ","}
		if b.Mapping != nil {
			mapping = *b.Mapping
		}
		stop := errors.New("preview budget reached")
		bytes := 0
		e = parseCSV(ctx, io.LimitReader(object, 512*1024), mapping, func(row inputRow) error {
			if row.EmptyRows > 0 {
				return nil
			}
			values := []string{}
			for index, c := range row.Cells {
				if index >= 64 {
					break
				}
				value := previewText(c.Text)
				bytes += len(value)
				if bytes > 256*1024 {
					return stop
				}
				values = append(values, value)
			}
			out.Rows = append(out.Rows, values)
			if len(out.Rows) >= 100 {
				return stop
			}
			return nil
		})
		out.Available = len(out.Rows) > 0
		if e != nil && !errors.Is(e, stop) {
			out.Notice = "样本未完整解析；请核对编码、分隔符和表头，全量结果以后台校验为准。"
		}
	} else {
		e = previewXLSX(ctx, object, b, out)
		if e != nil {
			out.Notice = "工作簿预览达到保护限制或文件结构异常；可手动指定映射后执行全量校验。"
		}
	}
	e = s.transact(ctx, a, scope, "imports.mapping", false, false, func(_ pgx.Tx, auth authorization) error {
		if authorizationVersion(auth) != revision {
			return revisionChanged()
		}
		return nil
	})
	if e != nil {
		out.Rows = [][]string{}
		out.Sheets = []PreviewSheet{}
		out.Available = false
		out.Notice = "授权已变化，请刷新。"
	}
	return out
}
func previewText(value string) string {
	if len(value) <= 2048 {
		return value
	}
	runes := []rune(value)
	if len(runes) > 512 {
		runes = runes[:512]
	}
	return string(runes) + "…"
}
func previewXLSX(ctx context.Context, object io.ReaderAt, b ImportBatch, out *SourcePreview) error {
	size, e := strconv.ParseInt(b.Size, 10, 64)
	if e != nil {
		return e
	}
	archive, e := zip.NewReader(&boundedReaderAt{object, 8 * 1024 * 1024}, size)
	if e != nil {
		return e
	}
	if len(archive.File) > 10000 {
		return io.ErrShortBuffer
	}
	files := map[string]*zip.File{}
	for _, file := range archive.File {
		if files[file.Name] != nil {
			return io.ErrUnexpectedEOF
		}
		files[file.Name] = file
	}
	readXML := func(name string, value any) error {
		file := files[name]
		if file == nil {
			return io.ErrUnexpectedEOF
		}
		r, e := file.Open()
		if e != nil {
			return e
		}
		defer r.Close()
		return xml.NewDecoder(io.LimitReader(r, 512*1024)).Decode(value)
	}
	var workbook struct {
		Sheets []struct {
			Name string `xml:"name,attr"`
			ID   string `xml:"id,attr"`
		} `xml:"sheets>sheet"`
	}
	if e = readXML("xl/workbook.xml", &workbook); e != nil {
		return e
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
	targets := map[string]string{}
	for _, rel := range rels.Items {
		if rel.Mode != "External" {
			target := path.Clean(path.Join("xl", rel.Target))
			if strings.HasPrefix(rel.Target, "/") {
				target = strings.TrimPrefix(rel.Target, "/")
			}
			if strings.HasPrefix(target, "xl/worksheets/") {
				targets[rel.ID] = target
			}
		}
	}
	if len(workbook.Sheets) == 0 {
		return io.ErrUnexpectedEOF
	}
	out.Sheet = workbook.Sheets[0].Name
	if b.Mapping != nil && b.Mapping.Sheet != "" {
		out.Sheet = b.Mapping.Sheet
	}
	var selected *zip.File
	for index, sheet := range workbook.Sheets {
		if index >= 100 {
			break
		}
		preview := PreviewSheet{Name: sheet.Name}
		file := files[targets[sheet.ID]]
		if file != nil {
			r, e := file.Open()
			if e != nil {
				return e
			}
			decoder := xml.NewDecoder(io.LimitReader(r, 16384))
			for {
				token, e := decoder.Token()
				if e != nil {
					break
				}
				if start, ok := token.(xml.StartElement); ok {
					if start.Name.Local == "dimension" {
						for _, attribute := range start.Attr {
							if attribute.Name.Local == "ref" {
								preview.Dimension = attribute.Value
							}
						}
						break
					}
					if start.Name.Local == "sheetData" {
						break
					}
				}
			}
			r.Close()
		}
		out.Sheets = append(out.Sheets, preview)
		if sheet.Name == out.Sheet {
			selected = file
		}
	}
	out.Available = len(out.Sheets) > 0
	if selected == nil {
		return io.ErrUnexpectedEOF
	}
	shared := []string{}
	if file := files["xl/sharedStrings.xml"]; file != nil {
		r, e := file.Open()
		if e != nil {
			return e
		}
		decoder := xml.NewDecoder(io.LimitReader(r, 512*1024))
		for len(shared) < 4096 {
			token, e := decoder.Token()
			if e != nil {
				break
			}
			start, ok := token.(xml.StartElement)
			if !ok || start.Name.Local != "si" {
				continue
			}
			var value xlsxText
			if e = decoder.DecodeElement(&value, &start); e != nil {
				break
			}
			text, e := value.text()
			if e != nil {
				break
			}
			shared = append(shared, previewText(text))
		}
		r.Close()
	}
	reader, e := selected.Open()
	if e != nil {
		return e
	}
	defer reader.Close()
	decoder := xml.NewDecoder(io.LimitReader(reader, 512*1024))
	bytes := 0
	for len(out.Rows) < 25 {
		if e = ctx.Err(); e != nil {
			return e
		}
		token, e := decoder.Token()
		if e == io.EOF {
			return nil
		}
		if e != nil {
			return e
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "row" {
			continue
		}
		var row struct {
			Cells []xlsxCell `xml:"c"`
		}
		if e = decoder.DecodeElement(&row, &start); e != nil {
			return e
		}
		values := []string{}
		for _, cell := range row.Cells {
			column, _, e := cellAddress(cell.Ref)
			if e != nil {
				return e
			}
			if column >= 64 {
				continue
			}
			for len(values) <= column {
				values = append(values, "")
			}
			value := cell.Value
			switch cell.Type {
			case "s":
				index, e := strconv.Atoi(cell.Value)
				value = "（共享文本未载入预览）"
				if e == nil && index >= 0 && index < len(shared) {
					value = shared[index]
				}
			case "inlineStr":
				value, e = cell.Inline.text()
				if e != nil {
					return e
				}
			}
			if cell.Formula != nil {
				value = "公式（不可导入）"
			}
			value = previewText(value)
			bytes += len(value)
			if bytes > 256*1024 {
				return io.ErrShortBuffer
			}
			values[column] = value
		}
		out.Rows = append(out.Rows, values)
	}
	return nil
}
