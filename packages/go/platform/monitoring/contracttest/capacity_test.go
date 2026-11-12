//go:build monitoringintegration

package contracttest

import (
	"fmt"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/google/uuid"
)

// A repeatable P-0 layout measurement. Full identity/source/quality columns are
// written; these are synthetic distributions, never a P-1/P-3 qualification.
func TestInitialCapacity(t *testing.T) {
	for _, distribution := range []string{"dense", "sparse"} {
		t.Run(distribution, func(t *testing.T) {
			l := newLab(t)
			const count = 100000
			data := make([]sample, count)
			rng := rand.New(rand.NewPCG(20261112, 7))
			base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			for i := range data {
				offset := i / 128
				if distribution == "sparse" {
					offset = i * 61
				}
				kind := "decimal"
				v := fmt.Sprintf("%d.%012d", int64(rng.Uint64()%200000)-100000, rng.Uint64()%1000000000000)
				switch i % 5 {
				case 0:
					kind = "integer"
					v = fmt.Sprint(int64(rng.Uint64()))
				case 1:
					kind = "enum"
					v = fmt.Sprintf("state-%d", rng.Uint32()%7)
				}
				data[i] = sample{base.Add(time.Duration(offset)*time.Second + time.Duration(i%1000)), kind, v, []string{"good", "unmarked", "suspect", "bad"}[i%4]}
			}
			chunk, attempt := uuid.New(), uuid.New()
			start := time.Now()
			must(t, l.insert(t, l.ch[0], chunk, attempt, data, 2, nil))
			must(t, l.verify(l.ch[1], chunk, attempt, data))
			elapsed := time.Since(start)
			must(t, l.ch[0].Exec(l.ctx, "OPTIMIZE TABLE "+l.db+".observations FINAL"))
			var rows, compressed, uncompressed, parts uint64
			must(t, l.ch[0].QueryRow(l.ctx, "SELECT sum(rows),sum(data_compressed_bytes),sum(data_uncompressed_bytes),count() FROM system.parts WHERE active AND database=? AND table='observations'", l.db).Scan(&rows, &compressed, &uncompressed, &parts))
			if rows != count {
				t.Fatal("capacity row count", rows)
			}
			// Materialize exact per-quality numeric states using wide integer
			// accumulation only within this bounded pilot (coefficient ranges here
			// fit Int256). Production uses unbounded math/big states.
			must(t, l.ch[0].Exec(l.ctx, "CREATE TABLE "+l.db+".summary (channel UUID,bucket DateTime64(9,'UTC'),quality String,n UInt64,s1 String,s2 String) ENGINE=MergeTree ORDER BY (channel,bucket,quality)"))
			must(t, l.ch[0].Exec(l.ctx, "INSERT INTO "+l.db+".summary SELECT channel_id,toStartOfMinute(observed_at),quality,count(),toString(sum(c)),toString(sum(c*c)) FROM (SELECT *,if(value_kind='integer',toInt256(ifNull(integer_value,0))*toInt256(1000000000000),toInt256(ifNull(decimal_value,0)*toDecimal256(1000000000000,12))) AS c FROM "+l.db+".observations WHERE value_kind!='enum') GROUP BY channel_id,toStartOfMinute(observed_at),quality"))
			var summaryRows, summaryBytes uint64
			must(t, l.ch[0].QueryRow(l.ctx, "SELECT sum(rows),sum(data_compressed_bytes) FROM system.parts WHERE active AND database=? AND table='summary'", l.db).Scan(&summaryRows, &summaryBytes))
			t.Logf("CAPACITY distribution=%s seed=20261112/7 raw_rows=%d channels=128 compressed_bytes=%d bytes_per_row=%.3f uncompressed_bytes=%d parts=%d summary_rows=%d summary_bytes=%d summary_to_raw=%.4f insert_and_verify_ms=%d replicas=2", distribution, rows, compressed, float64(compressed)/float64(rows), uncompressed, parts, summaryRows, summaryBytes, float64(summaryBytes)/float64(compressed), elapsed.Milliseconds())
		})
	}
}
