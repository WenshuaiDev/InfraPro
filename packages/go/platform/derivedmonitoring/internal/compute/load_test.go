//go:build derivedload

package compute

import (
	"fmt"
	"os"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoringvalue"
)

// Pure calculator workload: one million logical input rows, ten bounded
// partitions with complete halos. This does not qualify storage/worker/UI
// performance or replace the required mixed-load and 24-hour observation.
func TestMillionInputCalculator(t *testing.T) {
	const total = 1_000_000
	const partition = 100_000
	const width = 1000
	for _, op := range []string{"rate", "mean", "variance", "quantile"} {
		t.Run(op, func(t *testing.T) {
			var peak atomic.Uint64
			stop := make(chan struct{})
			done := make(chan struct{})
			go func() {
				defer close(done)
				ticker := time.NewTicker(10 * time.Millisecond)
				defer ticker.Stop()
				for {
					select {
					case <-stop:
						return
					case <-ticker.C:
						var m runtime.MemStats
						runtime.ReadMemStats(&m)
						if m.HeapAlloc > peak.Load() {
							peak.Store(m.HeapAlloc)
						}
					}
				}
			}()
			defer func() { close(stop); <-done }()
			n := rolling(op, "result", "input:A")
			if op == "rate" {
				n = difference("rate", "result", "input:A")
				n.Params.RateTimeUnit = "second"
			} else {
				n.Params.Window.DurationNS = fmt.Sprint(int64(width) * int64(time.Second))
				if op == "quantile" {
					n.Params.Percentile = "95"
				}
			}
			plan := compiled(t, baseRule(n), "channel")
			start := time.Now()
			count := 0
			maxChunk := time.Duration(0)
			for from := 0; from < total; from += partition {
				chunkStart := time.Now()
				left := from - width
				if left < 0 {
					left = 0
				}
				to := from + partition
				d := Data{Complete: true, Inputs: map[string][]Sample{"A": make([]Sample, 0, to-left), "B": nil}}
				for i := left; i < to; i++ {
					d.Inputs["A"] = append(d.Inputs["A"], Sample{At: int64(i) * int64(time.Second), Value: monitoringvalue.Integer(int64(i)), Quality: "good"})
				}
				err := Run(t.Context(), plan, d, RunOptions{OutputRange: TimeRange{int64(from) * int64(time.Second), int64(to) * int64(time.Second)}, NowNS: int64(total) * int64(time.Second)}, func(r Result) error {
					index := r.At / int64(time.Second)
					count++
					if index == 0 {
						if r.Status != "missing_input" && r.Status != "insufficient_samples" {
							return fmt.Errorf("first result status %s", r.Status)
						}
						return nil
					}
					if r.Value == nil {
						return fmt.Errorf("unexpected no result at %d: %s", index, r.Status)
					}
					if index >= width {
						want := ""
						switch op {
						case "rate":
							want = "1.000000000000000000"
						case "mean":
							want = fmt.Sprintf("%d.500000000000000000", index-500)
						case "variance":
							want = "83333.250000000000000000"
						case "quantile":
							want = fmt.Sprintf("%d.050000000000000000", index-50)
						}
						if *r.Value != want {
							return fmt.Errorf("row %d got %s want %s", index, *r.Value, want)
						}
					}
					return nil
				})
				if err != nil {
					t.Fatal(err)
				}
				if elapsed := time.Since(chunkStart); elapsed > maxChunk {
					maxChunk = elapsed
				}
			}
			elapsed := time.Since(start)
			if count != total {
				t.Fatalf("outputs %d want %d", count, total)
			}
			minimum := float64(1000)
			if op == "rate" {
				minimum = 20000
			} else if op == "mean" || op == "variance" {
				minimum = 2000
			}
			throughput := float64(total) / elapsed.Seconds()
			t.Logf("CALCULATOR_BASELINE op=%s input=%d outputs=%d partitions=%d duration=%s rows_per_second=%.0f min_rows_per_second=%.0f max_partition=%s peak_heap_bytes=%d GOMAXPROCS=%d GOMEMLIMIT=%s", op, total, count, total/partition, elapsed, throughput, minimum, maxChunk, peak.Load(), runtime.GOMAXPROCS(0), os.Getenv("GOMEMLIMIT"))
			if throughput < minimum || maxChunk > 60*time.Second || peak.Load() > 1<<30 {
				t.Fatal("calculator reference envelope failed")
			}
		})
	}
}
