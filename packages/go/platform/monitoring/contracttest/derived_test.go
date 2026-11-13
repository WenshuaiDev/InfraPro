//go:build monitoringintegration

package contracttest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoringvalue"
	"github.com/apache/arrow-go/v18/parquet"
	"github.com/apache/arrow-go/v18/parquet/file"
	"github.com/apache/arrow-go/v18/parquet/schema"
	"github.com/jackc/pgx/v5/pgtype"
)

// This qualifies the derived storage protocol on the locked production driver
// versions. Table isolation belongs to the existing disposable laboratory.
func TestDerivedNumericStorageContract(t *testing.T) {
	l := newLab(t)
	must(t, l.ch[0].Exec(l.ctx, "CREATE TABLE "+l.db+".derived_contract (ordinal UInt32, at DateTime64(9,'UTC'), coefficient Nullable(Int256), scale UInt8) ENGINE=MergeTree ORDER BY ordinal"))
	type sample struct {
		At          int64
		Coefficient *big.Int
		Scale       uint8
	}
	largest, _ := new(big.Int).SetString(strings.Repeat("9", 76), 10)
	samples := []sample{
		{monitoringvalue.MinTime.UnixNano(), largest, 0},
		{-1, new(big.Int).Neg(largest), 60},
		{time.Date(2024, 2, 29, 12, 0, 0, 123456789, time.UTC).UnixNano(), big.NewInt(250000000000), 36},
		{math.MaxInt64, big.NewInt(1), 60},
		{0, big.NewInt(0), 18},
		{1, nil, 18},
	}
	batch, e := l.ch[0].PrepareBatch(l.ctx, "INSERT INTO "+l.db+".derived_contract")
	must(t, e)
	for i, s := range samples {
		must(t, batch.Append(uint32(i), time.Unix(0, s.At).UTC(), s.Coefficient, s.Scale))
	}
	must(t, batch.Send())
	must(t, batch.Close())
	rows, e := l.ch[0].Query(l.ctx, "SELECT ordinal,at,coefficient,scale FROM "+l.db+".derived_contract ORDER BY ordinal")
	must(t, e)
	defer rows.Close()
	count := 0
	for rows.Next() {
		var ordinal uint32
		var at time.Time
		var coefficient *big.Int
		var scale uint8
		must(t, rows.Scan(&ordinal, &at, &coefficient, &scale))
		s := samples[ordinal]
		if at.UnixNano() != s.At || scale != s.Scale || (coefficient == nil) != (s.Coefficient == nil) || coefficient != nil && coefficient.Cmp(s.Coefficient) != 0 {
			t.Fatal("Int256/scale/nanosecond roundtrip changed", ordinal)
		}
		if coefficient != nil {
			value := monitoringvalue.Format(coefficient, int(scale))
			body, e := json.Marshal(struct {
				Value string `json:"value"`
				Scale uint8  `json:"scale"`
				At    string `json:"at_ns"`
			}{value, scale, fmt.Sprint(s.At)})
			must(t, e)
			var restored struct {
				Value string `json:"value"`
				Scale uint8  `json:"scale"`
				At    string `json:"at_ns"`
			}
			must(t, json.Unmarshal(body, &restored))
			if restored.Value != value || restored.At != fmt.Sprint(s.At) {
				t.Fatal("JSON precision lost")
			}
		}
		count++
	}
	must(t, rows.Err())
	if count != len(samples) {
		t.Fatal("missing samples")
	}
	// The maximal inclusive ns bound is represented by an unbounded upper range,
	// never max+1. Containment is still restricted to the application time domain.
	for _, r := range []pgtype.Range[int64]{
		{Lower: monitoringvalue.MinTime.UnixNano(), Upper: math.MaxInt64, LowerType: pgtype.Inclusive, UpperType: pgtype.Exclusive, Valid: true},
		{Lower: math.MaxInt64, LowerType: pgtype.Inclusive, UpperType: pgtype.Unbounded, Valid: true},
	} {
		var got pgtype.Range[int64]
		must(t, l.pg.QueryRow(l.ctx, "SELECT $1::int8range", r).Scan(&got))
		if got.Lower != r.Lower || got.LowerType != r.LowerType || got.UpperType != r.UpperType || r.UpperType != pgtype.Unbounded && got.Upper != r.Upper {
			t.Fatal("PG range changed", r, got)
		}
	}
	t.Log("derived Int256(76 digits), scale 0/18/36/60, nullable values, UTC ns boundaries, JSON strings and PG int8range: exact")
}

func TestDerivedParquetTextContract(t *testing.T) {
	ts, e := schema.NewPrimitiveNodeLogical("output_at", parquet.Repetitions.Required, schema.NewTimestampLogicalType(true, schema.TimeUnitNanos), parquet.Types.Int64, -1, 1)
	must(t, e)
	text, e := schema.NewPrimitiveNodeLogical("value", parquet.Repetitions.Required, schema.StringLogicalType{}, parquet.Types.ByteArray, -1, 2)
	must(t, e)
	scale, e := schema.NewPrimitiveNode("scale", parquet.Repetitions.Required, parquet.Types.Int32, 3, -1)
	must(t, e)
	rounding, e := schema.NewPrimitiveNodeLogical("rounding", parquet.Repetitions.Required, schema.StringLogicalType{}, parquet.Types.ByteArray, -1, 4)
	must(t, e)
	root, e := schema.NewGroupNode("computed_decimal_text", parquet.Repetitions.Required, schema.FieldList{ts, text, scale, rounding}, 0)
	must(t, e)
	values := []string{strings.Repeat("9", 76), "-" + strings.Repeat("9", 16) + "." + strings.Repeat("9", 60), "0.000000000000000000000000250000000000", "0." + strings.Repeat("0", 59) + "1", "0.000000000000000000"}
	scales := []int32{0, 60, 36, 60, 18}
	nanos := []int64{monitoringvalue.MinTime.UnixNano(), -1, 123456789, math.MaxInt64, 0}
	var buf bytes.Buffer
	writer, e := file.NewParquetWriterWithError(&buf, root)
	must(t, e)
	rg := writer.AppendBufferedRowGroup()
	c, e := rg.Column(0)
	must(t, e)
	_, e = c.(*file.Int64ColumnChunkWriter).WriteBatch(nanos, nil, nil)
	must(t, e)
	texts := make([]parquet.ByteArray, len(values))
	roundings := make([]parquet.ByteArray, len(values))
	for i, v := range values {
		texts[i] = []byte(v)
		roundings[i] = []byte("half_even")
	}
	c, e = rg.Column(1)
	must(t, e)
	_, e = c.(*file.ByteArrayColumnChunkWriter).WriteBatch(texts, nil, nil)
	must(t, e)
	c, e = rg.Column(2)
	must(t, e)
	_, e = c.(*file.Int32ColumnChunkWriter).WriteBatch(scales, nil, nil)
	must(t, e)
	c, e = rg.Column(3)
	must(t, e)
	_, e = c.(*file.ByteArrayColumnChunkWriter).WriteBatch(roundings, nil, nil)
	must(t, e)
	must(t, rg.Close())
	must(t, writer.Close())
	reader, e := file.NewParquetReader(bytes.NewReader(buf.Bytes()))
	must(t, e)
	defer reader.Close()
	if reader.MetaData().Schema.Root().Name() != "computed_decimal_text" || !reader.MetaData().Schema.Column(1).LogicalType().Equals(schema.StringLogicalType{}) {
		t.Fatal("not computed_decimal_text")
	}
	rr := reader.RowGroup(0)
	times := make([]int64, len(values))
	vals := make([]parquet.ByteArray, len(values))
	gotScales := make([]int32, len(values))
	gotRoundings := make([]parquet.ByteArray, len(values))
	cr, e := rr.Column(0)
	must(t, e)
	_, n, e := cr.(*file.Int64ColumnChunkReader).ReadBatch(int64(len(values)), times, nil, nil)
	must(t, e)
	if n != len(values) {
		t.Fatal(n)
	}
	cr, e = rr.Column(1)
	must(t, e)
	_, n, e = cr.(*file.ByteArrayColumnChunkReader).ReadBatch(int64(len(values)), vals, nil, nil)
	must(t, e)
	if n != len(values) {
		t.Fatal(n)
	}
	cr, e = rr.Column(2)
	must(t, e)
	_, _, e = cr.(*file.Int32ColumnChunkReader).ReadBatch(int64(len(values)), gotScales, nil, nil)
	must(t, e)
	cr, e = rr.Column(3)
	must(t, e)
	_, _, e = cr.(*file.ByteArrayColumnChunkReader).ReadBatch(int64(len(values)), gotRoundings, nil, nil)
	must(t, e)
	for i, v := range values {
		if string(vals[i]) != v || times[i] != nanos[i] || gotScales[i] != scales[i] || string(gotRoundings[i]) != "half_even" {
			t.Fatal("Parquet loss", i)
		}
	}
}
