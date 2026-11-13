package monitoring

import (
	"encoding/csv"
	"fmt"
	"io"
	"math/big"
	"os"
	"strconv"
	"strings"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring/internal/observation"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoringvalue"
	"github.com/apache/arrow-go/v18/parquet"
	"github.com/apache/arrow-go/v18/parquet/file"
	"github.com/apache/arrow-go/v18/parquet/schema"
)

type ExportField struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
}
type exportCell struct {
	Text string
	Null bool
}
type exportWriter struct {
	file         *os.File
	fields       []ExportField
	format, mode string
	csv          *csv.Writer
	parquet      *file.Writer
	group        [][]exportCell
	groupBytes   int64
	rows         int64
}

var detailExportFields = []ExportField{
	{"channel_id", "text", false}, {"point_id", "text", false}, {"point_name", "text", false}, {"channel_name", "text", false}, {"metric_version_id", "text", false}, {"unit", "text", false},
	{"observation_id", "text", false}, {"adoption_id", "text", false}, {"batch_id", "text", false}, {"observed_at", "timestamp_ns_utc", false}, {"interval_start", "timestamp_ns_utc", true},
	{"value_kind", "text", false}, {"value", "exact_text", false}, {"integer_value", "int64", true}, {"decimal_value", "decimal128_38_12", true}, {"enum_value", "text", true}, {"quality", "text", false}, {"quality_reason", "text", false}, {"revoked", "boolean", false},
}
var statisticsExportFields = []ExportField{
	{"channel_id", "text", false}, {"point_name", "text", false}, {"channel_name", "text", false}, {"unit", "text", false}, {"period", "text", false}, {"timezone", "text", false}, {"calendar_start", "text", true}, {"calendar_end", "text", true}, {"actual_range", "text", true}, {"partial", "boolean", false},
	{"ever_published_count", "int64", false}, {"active_count", "int64", false}, {"sample_count", "int64", false}, {"n", "int64", false}, {"first_at", "timestamp_ns_utc", true}, {"last_at", "timestamp_ns_utc", true}, {"first_value", "exact_text", true}, {"last_value", "exact_text", true}, {"minimum", "exact_text", true}, {"maximum", "exact_text", true}, {"minimum_ties", "int64", false}, {"maximum_ties", "int64", false}, {"mean", "computed_decimal_text", true}, {"population_stddev", "computed_decimal_text", true}, {"scale", "int64", false}, {"rounding", "text", false}, {"state_counts", "text", false},
}

func newExportWriter(path, format, mode string, fields []ExportField) (*exportWriter, error) {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	w := &exportWriter{file: f, fields: fields, format: format, mode: mode}
	if format == "csv" {
		w.csv = csv.NewWriter(f)
		header := []string{}
		for _, field := range fields {
			header = append(header, field.Name)
		}
		if e = w.csv.Write(header); e != nil {
			f.Close()
			return nil, e
		}
		return w, nil
	}
	nodes := schema.FieldList{}
	for i, field := range fields {
		repetition := parquet.Repetitions.Required
		if field.Nullable {
			repetition = parquet.Repetitions.Optional
		}
		physical := parquet.Types.ByteArray
		var logical schema.LogicalType = schema.StringLogicalType{}
		length := -1
		switch field.Type {
		case "int64":
			physical = parquet.Types.Int64
			logical = schema.NewIntLogicalType(64, true)
		case "timestamp_ns_utc":
			physical = parquet.Types.Int64
			logical = schema.NewTimestampLogicalType(true, schema.TimeUnitNanos)
		case "decimal128_38_12":
			physical = parquet.Types.FixedLenByteArray
			logical = schema.NewDecimalLogicalType(38, 12)
			length = 16
		case "boolean":
			physical = parquet.Types.Boolean
			logical = schema.NoLogicalType{}
		}
		node, e := schema.NewPrimitiveNodeLogical(field.Name, repetition, logical, physical, length, int32(i+1))
		if e != nil {
			f.Close()
			return nil, e
		}
		nodes = append(nodes, node)
	}
	root, e := schema.NewGroupNode("monitoring_export", parquet.Repetitions.Required, nodes, 0)
	if e != nil {
		f.Close()
		return nil, e
	}
	w.parquet, e = file.NewParquetWriterWithError(struct{ io.Writer }{f}, root, file.WithWriterProps(parquet.NewWriterProperties(parquet.WithVersion(parquet.V2_6))))
	if e != nil {
		f.Close()
		return nil, e
	}
	return w, nil
}
func spreadsheetCell(v string) string {
	trimmed := strings.TrimLeft(v, " \t\r\n")
	if strings.HasPrefix(v, "'") || strings.HasPrefix(v, "\t") || strings.HasPrefix(v, "\r") || strings.HasPrefix(v, "\n") || len(trimmed) > 0 && strings.ContainsAny(trimmed[:1], "=+-@") {
		return "'" + v
	}
	return v
}
func exportNumeric(field ExportField, cells []exportCell) bool {
	if field.Name == "value" && field.Type == "exact_text" && len(cells) >= len(detailExportFields) {
		return cells[11].Text == "integer" || cells[11].Text == "decimal"
	}
	if field.Type == "int64" || field.Type == "decimal128_38_12" || field.Type == "computed_decimal_text" {
		return true
	}
	if field.Type == "exact_text" {
		// Detail's generic value inherits its discriminated raw type. Statistics
		// exact text is numeric unless its contents fail the lossless grammar.
		for i, f := range detailExportFields {
			if f.Name == "value_kind" && len(cells) == len(detailExportFields) {
				return cells[i].Text == "integer" || cells[i].Text == "decimal"
			}
		}
	}
	return false
}
func (w *exportWriter) write(cells []exportCell) error {
	if len(cells) != len(w.fields) {
		return fmt.Errorf("export column count")
	}
	if w.csv != nil {
		row := make([]string, len(cells))
		for i, c := range cells {
			if c.Null {
				continue
			}
			value := c.Text
			if w.fields[i].Type == "timestamp_ns_utc" {
				n, e := strconv.ParseInt(value, 10, 64)
				if e != nil {
					return e
				}
				value = *nsText(&n)
			}
			numeric := exportNumeric(w.fields[i], cells)
			if w.fields[i].Type == "exact_text" && len(cells) != len(detailExportFields) {
				_, e := monitoringvalue.ParseDecimal(value)
				numeric = e == nil
			}
			if w.mode == "spreadsheet_safe" && !numeric {
				value = spreadsheetCell(value)
			}
			row[i] = value
		}
		if e := w.csv.Write(row); e != nil {
			return e
		}
		w.csv.Flush()
		if e := w.csv.Error(); e != nil {
			return e
		}
	} else {
		w.group = append(w.group, cells)
		for _, cell := range cells {
			w.groupBytes += int64(len(cell.Text) + 8)
		}
		if len(w.group) >= 8192 || w.groupBytes >= 8<<20 {
			if e := w.flushGroup(); e != nil {
				return e
			}
		}
	}
	w.rows++
	return nil
}
func (w *exportWriter) flushGroup() error {
	if len(w.group) == 0 {
		return nil
	}
	rg := w.parquet.AppendBufferedRowGroup()
	for col, field := range w.fields {
		cw, e := rg.Column(col)
		if e != nil {
			return e
		}
		defs := []int16(nil)
		if field.Nullable {
			defs = make([]int16, len(w.group))
		}
		integers := []int64{}
		text := []parquet.ByteArray{}
		decimals := []parquet.FixedLenByteArray{}
		booleans := []bool{}
		for row, cells := range w.group {
			c := cells[col]
			if c.Null {
				if !field.Nullable {
					return fmt.Errorf("required export value missing")
				}
				continue
			}
			if field.Nullable {
				defs[row] = 1
			}
			switch field.Type {
			case "int64", "timestamp_ns_utc":
				v, e := strconv.ParseInt(c.Text, 10, 64)
				if e != nil {
					return e
				}
				integers = append(integers, v)
			case "decimal128_38_12":
				v, e := observation.Decimal(c.Text)
				if e != nil {
					return e
				}
				if v.Sign() < 0 {
					v.Add(v, new(big.Int).Lsh(big.NewInt(1), 128))
				}
				bytes := make([]byte, 16)
				v.FillBytes(bytes)
				decimals = append(decimals, bytes)
			case "boolean":
				v, e := strconv.ParseBool(c.Text)
				if e != nil {
					return e
				}
				booleans = append(booleans, v)
			default:
				text = append(text, []byte(c.Text))
			}
		}
		switch writer := cw.(type) {
		case *file.Int64ColumnChunkWriter:
			_, e = writer.WriteBatch(integers, defs, nil)
		case *file.ByteArrayColumnChunkWriter:
			_, e = writer.WriteBatch(text, defs, nil)
		case *file.FixedLenByteArrayColumnChunkWriter:
			_, e = writer.WriteBatch(decimals, defs, nil)
		case *file.BooleanColumnChunkWriter:
			_, e = writer.WriteBatch(booleans, defs, nil)
		default:
			e = fmt.Errorf("unsupported export writer")
		}
		if e != nil {
			return e
		}
	}
	if e := rg.Close(); e != nil {
		return e
	}
	w.group = nil
	w.groupBytes = 0
	return nil
}
func (w *exportWriter) size() (int64, error) {
	if w.csv != nil {
		w.csv.Flush()
		if e := w.csv.Error(); e != nil {
			return 0, e
		}
	}
	n, e := w.file.Seek(0, io.SeekCurrent)
	return n + w.groupBytes, e
}
func (w *exportWriter) close() error {
	if w.parquet != nil {
		if e := w.flushGroup(); e != nil {
			w.file.Close()
			return e
		}
		if e := w.parquet.Close(); e != nil {
			w.file.Close()
			return e
		}
	}
	if w.csv != nil {
		w.csv.Flush()
		if e := w.csv.Error(); e != nil {
			w.file.Close()
			return e
		}
	}
	if e := w.file.Sync(); e != nil {
		w.file.Close()
		return e
	}
	return w.file.Close()
}
