//go:build monitoringintegration

package contracttest

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring/internal/observation"
	"github.com/apache/arrow-go/v18/parquet"
	"github.com/apache/arrow-go/v18/parquet/file"
	"github.com/apache/arrow-go/v18/parquet/schema"
)

func TestParquetPrecisionContract(t *testing.T) {
	ts, err := schema.NewPrimitiveNodeLogical("observed_at", parquet.Repetitions.Required, schema.NewTimestampLogicalType(true, schema.TimeUnitNanos), parquet.Types.Int64, -1, 1)
	must(t, err)
	dc, err := schema.NewPrimitiveNodeLogical("value", parquet.Repetitions.Required, schema.NewDecimalLogicalType(38, 12), parquet.Types.FixedLenByteArray, 16, 2)
	must(t, err)
	sc, err := schema.NewGroupNode("observations", parquet.Repetitions.Required, schema.FieldList{ts, dc}, 0)
	must(t, err)
	var buf bytes.Buffer
	writer, err := file.NewParquetWriterWithError(&buf, sc)
	must(t, err)
	nanos := []int64{}
	values := []parquet.FixedLenByteArray{}
	decimalText := []string{}
	for _, s := range boundarySamples(t) {
		if s.kind != "decimal" {
			continue
		}
		c, e := observation.Decimal(s.value)
		must(t, e)
		if c.Sign() < 0 {
			c.Add(c, new(big.Int).Lsh(big.NewInt(1), 128))
		}
		b := make([]byte, 16)
		c.FillBytes(b)
		values = append(values, b)
		nanos = append(nanos, s.at.UnixNano())
		decimalText = append(decimalText, s.value)
	}
	rg := writer.AppendBufferedRowGroup()
	cw, err := rg.Column(0)
	must(t, err)
	_, err = cw.(*file.Int64ColumnChunkWriter).WriteBatch(nanos, nil, nil)
	must(t, err)
	cw, err = rg.Column(1)
	must(t, err)
	_, err = cw.(*file.FixedLenByteArrayColumnChunkWriter).WriteBatch(values, nil, nil)
	must(t, err)
	must(t, rg.Close())
	must(t, writer.Close())
	reader, err := file.NewParquetReader(bytes.NewReader(buf.Bytes()))
	must(t, err)
	defer reader.Close()
	if !reader.MetaData().Schema.Column(0).LogicalType().Equals(ts.LogicalType()) || !reader.MetaData().Schema.Column(1).LogicalType().Equals(dc.LogicalType()) {
		t.Fatal("Parquet logical type changed")
	}
	readRG := reader.RowGroup(0)
	cr, err := readRG.Column(0)
	must(t, err)
	gotNS := make([]int64, len(nanos))
	total, n, err := cr.(*file.Int64ColumnChunkReader).ReadBatch(int64(len(nanos)), gotNS, nil, nil)
	must(t, err)
	if n != len(nanos) || total != int64(n) {
		t.Fatal("timestamp count")
	}
	cr, err = readRG.Column(1)
	must(t, err)
	got := make([]parquet.FixedLenByteArray, len(values))
	_, n, err = cr.(*file.FixedLenByteArrayColumnChunkReader).ReadBatch(int64(len(values)), got, nil, nil)
	must(t, err)
	if n != len(values) {
		t.Fatal("decimal count")
	}
	for i, b := range got {
		c := new(big.Int).SetBytes(b)
		if b[0]&128 != 0 {
			c.Sub(c, new(big.Int).Lsh(big.NewInt(1), 128))
		}
		if observation.Format(c, 12) != decimalText[i] || gotNS[i] != nanos[i] {
			t.Fatal("Parquet precision lost", i)
		}
	}
}
