//go:build monitoringintegration && derivedsoak

package resources_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
)

type soakChannel struct {
	channel dm.Channel
	config  dm.ChannelConfig
	op      string
}

// This is a real elapsed-time qualification. The explicit smoke mode is only
// a harness check and is never reported as the 2h + 24h stability result.
func TestDerivedContinuousMixedLoadAndObservation(t *testing.T) {
	smoke := os.Getenv("INFRAPRO_DERIVED_SOAK_SMOKE") == "1"
	mixed, observation := 2*time.Hour, 24*time.Hour
	if smoke {
		mixed, observation = 2*time.Minute, 2*time.Minute
	}
	var temporary string
	f := monitoringSetup(t, func(c *md.StorageConfig) { temporary = c.TempDirectory })
	s, _, cfg := derivedChannelSetup(t, f)
	s.WithStorage(f.storage)
	must(t, dm.InitializeStorage(ctx, f.storage))
	inputs := createInputChannels(t, f, 4)
	var template dm.Rule
	must(t, json.Unmarshal(cfg.Rule, &template))
	channels := []soakChannel{}
	for i, op := range []string{"difference", "rate", "difference", "rate", "mean", "variance", "median", "quantile", "four_sum", "chain"} {
		var rule dm.Rule
		must(t, json.Unmarshal(cfg.Rule, &rule))
		bindings := map[string]dm.InputBinding{"x": {ChannelID: inputs[0]}}
		switch op {
		case "difference", "rate":
			rule.Nodes[0].Op = op
			if op == "rate" {
				rule.Nodes[0].Params.RateTimeUnit = "second"
			}
		case "four_sum":
			rule.Inputs = nil
			bindings = map[string]dm.InputBinding{}
			refs := []string{}
			for j, id := range inputs {
				in := template.Inputs[0]
				in.Key = fmt.Sprintf("x%d", j)
				rule.Inputs = append(rule.Inputs, in)
				bindings[in.Key] = dm.InputBinding{ChannelID: id}
				refs = append(refs, "input:"+in.Key)
			}
			rule.Clock.Input = "x0"
			rule.Nodes = []dm.Node{{ID: "result", Op: "add", Inputs: refs, Params: dm.NodeParams{Alignment: "exact"}}}
		case "chain":
			rule.Inputs[0].MetricVersionID = ""
			rule.Inputs[0].SourceKinds = []string{"derived"}
			bindings["x"] = dm.InputBinding{ChannelID: channels[8].channel.Resource.ID}
			rule.Nodes = []dm.Node{{ID: "result", Op: "formula", Params: dm.NodeParams{Expression: "x * 2", Variables: map[string]string{"x": "input:x"}}}}
		default:
			p := dm.NodeParams{Window: &dm.Window{Kind: "trailing_duration", DurationNS: "60000000000"}, MinSamples: "1", MaxGapNS: "120000000000", MaxAgeNS: "120000000000"}
			if op == "variance" {
				p.MinSamples = "2"
			}
			if op == "quantile" {
				p.Percentile = "95"
			}
			rule.Nodes = []dm.Node{{ID: "result", Op: "window_" + op, Inputs: []string{"input:x"}, Params: p}}
		}
		rule.Outputs[0].SemanticKey, rule.Outputs[0].Quantity = "soak_"+op, nil
		channel, config := createRuleChannel(t, f, s, fmt.Sprintf("SOAK_%02d", i), rule, bindings)
		channels = append(channels, soakChannel{channel, config, op})
	}
	base := time.Now().UTC().Truncate(time.Second).Add(-2 * time.Minute)
	scope := dm.TenantScope(f.tenant)
	from := strconv.FormatInt(base.UnixNano(), 10)
	trialEnd := strconv.FormatInt(base.Add(time.Second).UnixNano(), 10)
	for i := range channels {
		c := &channels[i]
		if c.op == "chain" {
			// Establish explicit empty formal coverage for the upstream trial;
			// absence of an upstream generation is not fabricated empty input.
			parent := channels[8]
			preview, e := s.PreviewBackfill(ctx, f.actor, scope, parent.channel.Resource.ID, dm.BackfillPreviewInput{Revision: parent.channel.ConfigRevision, Range: dm.NanosecondRange{From: from, To: &trialEnd}})
			must(t, e)
			backfill, e := s.BackfillChannel(ctx, f.actor, scope, parent.channel.Resource.ID, dm.ControlInput{WriteMeta: restoreMeta(parent.channel.ConfigRevision), PreviewID: preview.ID, PreviewHash: preview.Hash}, access.Meta{})
			must(t, e)
			for _, run := range backfill.Runs {
				_, e = f.service.RunTask(ctx, "soak-empty-upstream", f.tenant, run.Job.ID)
				must(t, e)
			}
		}
		trial, e := s.ChannelTrial(ctx, f.actor, scope, c.channel.Resource.ID, dm.TrialInput{WriteMeta: restoreMeta(c.channel.ConfigRevision), ConfigID: c.config.ID, ConfigHash: c.config.ConfigHash, OutputRange: dm.NanosecondRange{From: from, To: &trialEnd}}, access.Meta{})
		must(t, e)
		if trial.Job.State != "succeeded" {
			_, e = f.service.RunTask(ctx, "soak-initial-trial", f.tenant, trial.Job.ID)
			must(t, e)
			trial, e = s.Run(ctx, f.actor, scope, trial.ID)
			must(t, e)
		}
		if trial.Trial == nil {
			t.Fatal("empty trial did not provide a waiting receipt", trial)
		}
		preview, e := s.PreviewControl(ctx, f.actor, scope, c.channel.Resource.ID, dm.ControlPreviewInput{Revision: c.channel.ControlRevision, ControlProposal: dm.ControlProposal{Action: "enable", AutoStartNS: &from, TrialReceiptID: trial.Trial.ReceiptID, AllowWaiting: true}})
		must(t, e)
		controlled, e := s.ControlChannel(ctx, f.actor, scope, c.channel.Resource.ID, dm.ControlInput{WriteMeta: restoreMeta(c.channel.ControlRevision), PreviewID: preview.ID, PreviewHash: preview.Hash}, access.Meta{})
		must(t, e)
		c.channel = controlled.Channel
	}
	work, stop := context.WithCancel(t.Context())
	var workers sync.WaitGroup
	var peakHeap, peakTemp atomic.Uint64
	workers.Go(func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		ticks := 0
		for {
			select {
			case <-work.Done():
				return
			case <-ticker.C:
				var memory runtime.MemStats
				runtime.ReadMemStats(&memory)
				if memory.HeapAlloc > peakHeap.Load() {
					peakHeap.Store(memory.HeapAlloc)
				}
				ticks++
				if ticks%10 == 0 {
					var bytes uint64
					_ = filepath.WalkDir(temporary, func(path string, entry os.DirEntry, e error) error {
						if e == nil && !entry.IsDir() {
							if info, e := entry.Info(); e == nil {
								bytes += uint64(info.Size())
							}
						}
						return nil
					})
					if bytes > peakTemp.Load() {
						peakTemp.Store(bytes)
					}
				}
			}
		}
	})
	workers.Go(func() { _ = f.service.RunWorker(work, slog.New(slog.NewTextHandler(os.Stdout, nil))) })
	defer func() { stop(); workers.Wait() }()
	var sampleMu sync.RWMutex
	ticks := []int64{}
	var latest atomic.Int64
	latest.Store(-1)
	ids := []string{}
	byID := map[string]string{}
	for _, c := range channels {
		ids = append(ids, c.channel.Resource.ID)
		byID[c.channel.Resource.ID] = c.op
	}
	waitJob := func(id string) {
		t.Helper()
		deadline := time.Now().Add(2 * time.Minute)
		for {
			j, e := f.service.Job(work, f.actor, f.scope, id)
			must(t, e)
			if j.State == "succeeded" {
				return
			}
			if j.State == "failed" || j.State == "needs_action" || j.State == "cancelled" || time.Now().After(deadline) {
				t.Fatal("continuous worker did not complete a submitted job", j)
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
	var latencyMu sync.Mutex
	readLatencies := []float64{}
	autoLatencies := []float64{}
	var queryCount atomic.Uint64
	errors := make(chan error, 8)
	query := func(c context.Context, lower, upper time.Time, selected []string) (md.QueryInput, error) {
		selection, e := f.service.Select(c, f.actor, f.scope, md.SelectionInput{Channels: selected})
		if e != nil {
			return md.QueryInput{}, e
		}
		lo, hi := lower.Format(time.RFC3339Nano), upper.Format(time.RFC3339Nano)
		return md.QueryInput{Selection: selection.ID, Range: &md.Range{From: &lo, To: &hi}, Limit: 200, Qualities: []string{"good", "unmarked"}}, nil
	}
	checkRows := func(rows []md.Observation) error {
		sampleMu.RLock()
		defer sampleMu.RUnlock()
		seen := map[string]bool{}
		for _, row := range rows {
			at, e := time.Parse(time.RFC3339Nano, row.At)
			if e != nil {
				return e
			}
			index := int64(at.Sub(base) / time.Second)
			value, status := soakExpected(byID[row.Channel], ticks, index)
			key := row.Channel + ":" + row.At
			if seen[key] || row.Calculated == nil || row.Calculated.Status != status || value != "" && (row.Calculated.Value == nil || *row.Calculated.Value != value) || value == "" && row.Calculated != nil && row.Calculated.Value != nil {
				return fmt.Errorf("current row lost exact value, status or uniqueness at %s", key)
			}
			seen[key] = true
		}
		return nil
	}
	for client := 0; client < 2; client++ {
		workers.Go(func() {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-work.Done():
					return
				case <-ticker.C:
					last := latest.Load()
					if last < 0 {
						continue
					}
					start := time.Now()
					traced := md.TraceContext(work, "")
					q, e := query(traced, base.Add(time.Duration(max(0, last-9))*time.Second), base.Add(time.Duration(last)*time.Second+time.Nanosecond), ids)
					selected := time.Now()
					if e == nil {
						var page md.DetailPage
						page, e = f.service.Details(traced, f.actor, f.scope, q)
						if e == nil {
							e = checkRows(page.Items)
						}
					}
					status := 200
					if e != nil {
						status = 500
					}
					md.LogOperation(traced, "soak.query", status, 0, work.Err() != nil)
					if time.Since(start) > 2*time.Second {
						t.Logf("SOAK_SLOW_QUERY client=%d select_seconds=%.6f details_seconds=%.6f", client, selected.Sub(start).Seconds(), time.Since(selected).Seconds())
					}
					if work.Err() != nil {
						return
					}
					if e != nil {
						select {
						case errors <- e:
						default:
						}
						return
					}
					latencyMu.Lock()
					readLatencies = append(readLatencies, time.Since(start).Seconds())
					latencyMu.Unlock()
					queryCount.Add(1)
				}
			}
		})
	}
	appendInputs := func(indices []int64) {
		t.Helper()
		if len(indices) == 0 {
			return
		}
		var content strings.Builder
		content.WriteString("observed_at,quality,x0,x1,x2,x3\n")
		for _, i := range indices {
			fmt.Fprintf(&content, "%s,good,%d,%d,%d,%d\n", base.Add(time.Duration(i)*time.Second).Format(time.RFC3339Nano), i, 2*i, 3*i, 4*i)
		}
		mapping := md.Mapping{Format: "csv", Mode: "wide", Encoding: "utf-8", Delimiter: ",", HeaderRow: 1, TimeColumn: "observed_at", QualityColumn: "quality", Timezone: "UTC", EmptyWide: "error"}
		for j, id := range inputs {
			mapping.Channels = append(mapping.Channels, md.ChannelMapping{Alias: fmt.Sprintf("x%d", j), Channel: id, ValueColumn: fmt.Sprintf("x%d", j), Unit: "mm"})
		}
		batch := f.uploadBytes(t, "continuous.csv", []byte(content.String()), mapping)
		queued, e := f.service.QueueImport(work, f.actor, f.scope, batch.ID, "validate", f.meta(batch.Revision))
		must(t, e)
		waitJob(queued.JobID)
		batch, e = f.service.Import(work, f.actor, f.scope, batch.ID)
		must(t, e)
		sampleMu.Lock()
		ticks = append(ticks, indices...)
		sampleMu.Unlock()
		queued, e = f.service.QueueImport(work, f.actor, f.scope, batch.ID, "publish", f.meta(batch.Revision))
		must(t, e)
		waitJob(queued.JobID)
		published := time.Now()
		last := indices[len(indices)-1]
		latest.Store(last)
		deadline := time.Now().Add(2 * time.Minute)
		for {
			q, e := query(work, base.Add(time.Duration(last)*time.Second), base.Add(time.Duration(last)*time.Second+time.Nanosecond), ids)
			must(t, e)
			page, e := f.service.Details(work, f.actor, f.scope, q)
			must(t, e)
			must(t, checkRows(page.Items))
			if len(page.Items) == len(ids) {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("automatic backlog did not converge to all ten latest outputs", len(page.Items))
			}
			time.Sleep(500 * time.Millisecond)
		}
		autoLatencies = append(autoLatencies, time.Since(published).Seconds())
	}
	seed := []int64{}
	for i := int64(0); i < int64(time.Since(base)/time.Second); i++ {
		seed = append(seed, i)
	}
	appendInputs(seed)
	backgroundJobs := []string{}
	backgroundCycles := 0
	queueBackground := func() {
		t.Helper()
		for _, id := range backgroundJobs {
			waitJob(id)
		}
		backgroundJobs = nil
		c := channels[0]
		end := strconv.FormatInt(base.Add(60*time.Second).UnixNano(), 10)
		preview, e := s.PreviewBackfill(work, f.actor, scope, c.channel.Resource.ID, dm.BackfillPreviewInput{Revision: c.channel.ConfigRevision, Range: dm.NanosecondRange{From: from, To: &end}})
		must(t, e)
		backfill, e := s.BackfillChannel(work, f.actor, scope, c.channel.Resource.ID, dm.ControlInput{WriteMeta: restoreMeta(c.channel.ConfigRevision), PreviewID: preview.ID, PreviewHash: preview.Hash}, access.Meta{})
		must(t, e)
		for _, run := range backfill.Runs {
			backgroundJobs = append(backgroundJobs, run.Job.ID)
		}
		for _, format := range []string{"csv", "parquet"} {
			q, e := query(work, base, base.Add(60*time.Second), ids[1:])
			must(t, e)
			export, e := f.service.CreateExport(work, f.actor, f.scope, md.ExportInput{WriteMeta: f.meta("0"), Query: q, Kind: "details", Format: format})
			must(t, e)
			backgroundJobs = append(backgroundJobs, export.JobID)
		}
		backgroundCycles++
	}
	started := time.Now()
	phaseStarted := started
	phase := "mixed"
	phaseDuration := mixed
	var actualMixed, actualObservation time.Duration
	appendEvery, backgroundEvery := 30*time.Second, 5*time.Minute
	if smoke {
		appendEvery, backgroundEvery = 10*time.Second, time.Minute
	}
	nextAppend, nextBackground := time.Now(), time.Now()
	nextSample := time.Now()
	for {
		select {
		case e := <-errors:
			t.Fatal("continuous reader failed", e)
		default:
		}
		now := time.Now()
		if now.Sub(phaseStarted) >= phaseDuration {
			for _, id := range backgroundJobs {
				waitJob(id)
			}
			finished := time.Now()
			elapsed := finished.Sub(phaseStarted)
			t.Logf("SOAK_PHASE phase=%s started=%s finished=%s elapsed_seconds=%.3f", phase, phaseStarted.UTC().Format(time.RFC3339Nano), finished.UTC().Format(time.RFC3339Nano), elapsed.Seconds())
			if phase == "observation" {
				actualObservation = elapsed
				break
			}
			actualMixed = elapsed
			phase, phaseStarted, phaseDuration = "observation", time.Now(), observation
			appendEvery, backgroundEvery = time.Minute, time.Hour
			if smoke {
				appendEvery, backgroundEvery = 15*time.Second, time.Minute
			}
			nextAppend, nextBackground = time.Now(), time.Now()
		}
		if !now.Before(nextAppend) {
			last := int64(time.Since(base)/time.Second) - 1
			indices := []int64{}
			if phase == "mixed" {
				for i := latest.Load() + 1; i <= last; i++ {
					indices = append(indices, i)
				}
			} else if last > latest.Load() {
				indices = append(indices, last)
			}
			appendInputs(indices)
			nextAppend = now.Add(appendEvery)
		}
		if !now.Before(nextBackground) {
			queueBackground()
			nextBackground = now.Add(backgroundEvery)
		}
		if !now.Before(nextSample) {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			var pending, failed int
			must(t, f.env.Pool.QueryRow(work, `SELECT count(*) FILTER(WHERE state IN ('queued','running','retry_wait')),count(*) FILTER(WHERE state='failed') FROM monitoring_jobs WHERE tenant_id=$1`, f.tenant).Scan(&pending, &failed))
			if failed > 0 || peakHeap.Load() > 1<<30 || peakTemp.Load() > 20<<30 {
				t.Fatal("continuous resource or task health threshold failed", failed, peakHeap.Load(), peakTemp.Load())
			}
			t.Logf("SOAK_SAMPLE phase=%s at=%s elapsed_seconds=%.3f raw_instants=%d queries=%d pending=%d heap_bytes=%d temporary_bytes=%d", phase, time.Now().UTC().Format(time.RFC3339Nano), time.Since(started).Seconds(), len(ticks), queryCount.Load(), pending, m.HeapAlloc, peakTemp.Load())
			nextSample = now.Add(time.Minute)
		}
		time.Sleep(200 * time.Millisecond)
	}
	for _, id := range backgroundJobs {
		waitJob(id)
	}
	// Freeze one query snapshot, then export contiguous views of that same
	// snapshot. This bounds proof/manifest work for the sparse 24-hour history
	// without skipping gaps, truncating rows or weakening final verification.
	q, e := query(work, base, base.Add(time.Duration(latest.Load())*time.Second+time.Nanosecond), ids)
	must(t, e)
	verification := []md.ExportRecord{}
	for offset := 0; offset < len(ticks); offset += 64 {
		lower := base.Add(time.Duration(ticks[offset]) * time.Second)
		upper := base.Add(time.Duration(latest.Load())*time.Second + time.Nanosecond)
		if offset+64 < len(ticks) {
			upper = base.Add(time.Duration(ticks[offset+64]) * time.Second)
		}
		if offset == 0 {
			lower = base
		}
		lo, hi := lower.UTC().Format(time.RFC3339Nano), upper.UTC().Format(time.RFC3339Nano)
		q.ViewRange = &md.Range{From: &lo, To: &hi}
		export, e := f.service.CreateExport(work, f.actor, f.scope, md.ExportInput{WriteMeta: f.meta("0"), Query: q, Kind: "details", Format: "csv"})
		must(t, e)
		if q.Snapshot != "" && q.Snapshot != export.Snapshot.ID {
			t.Fatal("final verification switched snapshots")
		}
		q.Snapshot = export.Snapshot.ID
		verification = append(verification, export)
	}
	seen := map[string]bool{}
	for _, export := range verification {
		waitJob(export.JobID)
		manifest, e := f.service.ExportFiles(work, f.actor, f.scope, export.ID)
		must(t, e)
		if manifest.Snapshot.ID != q.Snapshot {
			t.Fatal("verification manifest lost the fixed snapshot")
		}
		for _, part := range manifest.Files {
			download, e := f.service.ExportDownload(work, f.actor, f.scope, export.ID, part.Part)
			must(t, e)
			must(t, walkLoadCSV(download, func(columns map[string]int, row []string) error {
				at, e := time.Parse(time.RFC3339Nano, row[columns["observed_at"]])
				if e != nil {
					return e
				}
				id := row[columns["channel_id"]]
				index := int64(at.Sub(base) / time.Second)
				value, status := soakExpected(byID[id], ticks, index)
				key := id + ":" + row[columns["observed_at"]]
				if seen[key] || row[columns["value"]] != value || row[columns["calculation_status"]] != status {
					return fmt.Errorf("continuous final output mismatched %s", key)
				}
				seen[key] = true
				return nil
			}))
		}
	}
	if len(seen) != len(ticks)*len(ids) {
		t.Fatal("continuous final output was lost or duplicated", len(seen), len(ticks)*len(ids))
	}
	// Stop readers before reading their final sample slice.
	stop()
	workers.Wait()
	select {
	case e := <-errors:
		t.Fatal("continuous reader failed during final verification", e)
	default:
	}
	report := map[string]any{"smoke": smoke, "full_qualification": !smoke, "started_at": started.UTC(), "finished_at": time.Now().UTC(), "mixed_seconds": actualMixed.Seconds(), "observation_seconds": actualObservation.Seconds(), "mixed_target_seconds": mixed.Seconds(), "observation_target_seconds": observation.Seconds(), "elapsed_seconds": time.Since(started).Seconds(), "input_rows": len(ticks) * 4, "current_rows": len(seen), "queries": queryCount.Load(), "query_p95_seconds": soakP95(readLatencies), "automatic_p95_seconds": soakP95(autoLatencies), "background_cycles": backgroundCycles, "verification_exports": len(verification), "verification_snapshot": q.Snapshot, "peak_heap_bytes": peakHeap.Load(), "peak_temporary_bytes": peakTemp.Load()}
	encoded, e := json.Marshal(report)
	must(t, e)
	t.Logf("DP0_SOAK %s", encoded)
	if actualMixed < mixed || actualObservation < observation || soakP95(readLatencies) > 2 || soakP95(autoLatencies) > 10 || peakHeap.Load() > 1<<30 || peakTemp.Load() > 20<<30 {
		t.Fatal("continuous duration, latency or resource target failed")
	}
}

func soakP95(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	v := slices.Clone(values)
	slices.Sort(v)
	return v[(len(v)*95+99)/100-1]
}

func soakExpected(op string, times []int64, at int64) (string, string) {
	index, found := slices.BinarySearch(times, at)
	if !found {
		return "", "unexpected_time"
	}
	if op == "rate" || op == "difference" {
		if index == 0 {
			return "", "missing_input"
		}
		if op == "rate" {
			return "1.000000000000000000", "valid"
		}
		return new(big.Rat).SetInt64(at - times[index-1]).FloatString(18), "valid"
	}
	if op == "four_sum" || op == "chain" {
		factor := int64(10)
		if op == "chain" {
			factor = 20
		}
		return new(big.Rat).SetInt64(at * factor).FloatString(18), "valid"
	}
	first := index
	for first > 0 && times[first-1] > at-60 {
		first--
	}
	window := times[first : index+1]
	n := int64(len(window))
	if op == "variance" && n < 2 {
		return "", "insufficient_samples"
	}
	var value *big.Rat
	switch op {
	case "mean", "variance":
		sum, squares := new(big.Int), new(big.Int)
		for _, v := range window {
			sum.Add(sum, big.NewInt(v))
			squares.Add(squares, new(big.Int).Mul(big.NewInt(v), big.NewInt(v)))
		}
		value = new(big.Rat).SetFrac(sum, big.NewInt(n))
		if op == "variance" {
			value.Sub(new(big.Rat).SetFrac(squares, big.NewInt(n)), new(big.Rat).Mul(value, value))
		}
	case "median", "quantile":
		percent := int64(50)
		if op == "quantile" {
			percent = 95
		}
		rank := new(big.Rat).SetFrac64((n-1)*percent, 100)
		lo := new(big.Int).Quo(rank.Num(), rank.Denom()).Int64()
		hi := min(lo+1, n-1)
		fraction := new(big.Rat).Sub(rank, new(big.Rat).SetInt64(lo))
		value = new(big.Rat).Add(new(big.Rat).SetInt64(window[lo]), new(big.Rat).Mul(fraction, new(big.Rat).SetInt64(window[hi]-window[lo])))
	default:
		return "", "unexpected_operator"
	}
	return value.FloatString(18), "valid"
}
