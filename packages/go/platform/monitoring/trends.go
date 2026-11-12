package monitoring

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
)

type TrendBucket struct {
	Range         Range           `json:"range"`
	Count         string          `json:"sample_count"`
	Points        []Observation   `json:"points"`
	States        []EnumStatistic `json:"states"`
	Discontinuous bool            `json:"discontinuous"`
}
type TrendSeries struct {
	Channel        string        `json:"channel_id"`
	Range          Range         `json:"range"`
	Resolution     string        `json:"resolution"`
	Summary        bool          `json:"summary"`
	Samples        string        `json:"sample_count"`
	Buckets        []TrendBucket `json:"buckets"`
	Coverage       string        `json:"coverage"`
	Connection     string        `json:"connection"`
	GapExplanation string        `json:"gap_explanation"`
}

func (r *summaryReader) trend(ctx context.Context, channel string, ref summaryRef, total summaryTotals, in QueryInput, curves int) (TrendSeries, error) {
	samples := total.samples(in.Qualities, in.Status == "history")
	active := total.samples(allQualities, in.Status == "history")
	out := TrendSeries{Channel: channel, Samples: decimalCount(samples.N), Buckets: []TrendBucket{}, Coverage: "complete", Connection: "observed_points", GapExplanation: "仅显示真实观测；没有历史采样计划，不推断缺测率或状态持续时间。"}
	w := queryWindow(in)
	if w == nil && active.First != nil {
		lo := active.First.At
		var hi *int64
		if active.Last.At < 1<<63-1 {
			v := active.Last.At + 1
			hi = &v
		}
		w = &Window{&lo, hi}
	}
	if w != nil {
		out.Range = w.rangeDTO()
	}
	if samples.N == 0 {
		out.Resolution = "raw"
		return out, nil
	}
	width := in.PlotWidth
	if width == 0 {
		width = 960
	}
	budget := min(2000, 20000/curves, width*2)
	if in.PlotBudget != 0 {
		budget = min(budget, in.PlotBudget)
	}
	if samples.N <= uint64(budget) {
		watermark, _ := revisionNumber(r.plan.snapshot.Watermark)
		manifest := []selectedChunk{}
		e := r.service.db(ctx, func(tx pgx.Tx) error {
			var e error
			manifest, e = manifestTx(ctx, tx, r.tenant, map[string][]Window{channel: r.plan.windows[channel]}, watermark, in.Status)
			return e
		})
		if e != nil {
			return out, e
		}
		bucket := TrendBucket{Range: out.Range, Count: decimalCount(samples.N), Points: []Observation{}, States: []EnumStatistic{}, Discontinuous: active.N != samples.N || len(r.plan.windows[channel]) > 1}
		revoked := map[string]bool{}
		for _, entry := range manifest {
			revoked[entry.Batch] = entry.Revoked
		}
		e = r.service.storage.read(ctx, rawRead{Manifest: manifest, Tenant: r.tenant, Windows: map[string][]Window{channel: r.plan.windows[channel]}, Range: w, Qualities: in.Qualities, Limit: budget + 1}, func(row rawObservation) error {
			bucket.Points = append(bucket.Points, observationDTO(row, revoked[row.Batch]))
			return nil
		})
		if e != nil {
			return out, e
		}
		if uint64(len(bucket.Points)) != samples.N {
			return out, fmt.Errorf("raw trend and immutable summary disagree")
		}
		out.Resolution = "raw"
		out.Buckets = append(out.Buckets, bucket)
		return out, nil
	}
	out.Summary = true
	out.Connection = "extrema_envelope"
	out.GapExplanation = "摘要保留真实首点、极值点和末点；桶内连续性未知，按包络或事件计数显示，放大后查看原始观测。"
	first, last := minuteKey(samples.First.At), minuteKey(samples.Last.At)
	if w.From != nil {
		first = minuteKey(*w.From)
	}
	if w.To != nil {
		last = minuteKey(*w.To - 1)
	}
	cost := max(4, len(samples.States))
	if cost > budget {
		return out, invalid("plot_point_budget", "绘图预算不足以完整展示所有枚举状态，请增加宽度或减少比较通道。")
	}
	maxBuckets := max(1, budget/cost)
	resolution := int64(1)
	for _, candidate := range []int64{1, 15, 60, 1440} {
		resolution = candidate
		if last/resolution-first/resolution+1 <= int64(maxBuckets) {
			break
		}
	}
	for last/resolution-first/resolution+1 > int64(maxBuckets) {
		resolution *= 2
	}
	// Align to fixed UTC minutes; subtracting the domain origin before division
	// keeps tile boundaries stable across separate viewport requests.
	start := floorMultiple(first, resolution)
	for start <= last {
		loKey, hiKey := start, start+resolution
		lo := (loKey + minimumMinute) * minuteNS
		var hi *int64
		if hiKey <= minuteKey(1<<63-1) {
			v := (hiKey + minimumMinute) * minuteNS
			hi = &v
		}
		window := Window{&lo, hi}
		var ok bool
		window, ok = window.Intersect(*w)
		if !ok {
			start += resolution
			continue
		}
		v, e := r.collect(ctx, channel, ref, &window)
		if e != nil {
			return out, e
		}
		state := v.samples(in.Qualities, in.Status == "history")
		bucket := TrendBucket{Range: window.rangeDTO(), Count: decimalCount(state.N), Points: []Observation{}, States: []EnumStatistic{}, Discontinuous: true}
		if len(state.States) > 0 {
			stat, e := statisticOf(channel, v, in)
			if e != nil {
				return out, e
			}
			bucket.States = stat.States
		} else {
			for _, p := range state.representatives() {
				bucket.Points = append(bucket.Points, *p.dto(channel))
			}
		}
		out.Buckets = append(out.Buckets, bucket)
		start += resolution
	}
	out.Resolution = strconv.FormatInt(resolution, 10) + "m"
	return out, nil
}
func floorMultiple(n, width int64) int64 {
	q := n / width
	if n < 0 && n%width != 0 {
		q--
	}
	return q * width
}
