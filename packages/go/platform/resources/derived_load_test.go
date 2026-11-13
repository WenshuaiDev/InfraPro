//go:build monitoringintegration && derivedload

package resources_test

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// The before-run thresholds are documented in derived-monitoring-dp0-load.md.
// All rows go through the real CSV, worker, two replicas, object store, current
// query and controlled download paths. A pure calculator is not this test.
func TestMillionInputPipeline(t *testing.T) {
	for _, op := range []string{"rate", "mean", "variance", "quantile"} {
		t.Run(op, func(t *testing.T) { millionInputPipeline(t, op) })
	}
}

func millionInputPipeline(t *testing.T, op string) {
	const total = 1_000_000
	const width = 1000
	var temporary string
	f := monitoringSetup(t, func(c *md.StorageConfig) { temporary = c.TempDirectory })
	var peakHeap, peakTemp atomic.Uint64
	stop, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		ticks := 0
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				if m.HeapAlloc > peakHeap.Load() {
					peakHeap.Store(m.HeapAlloc)
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
	}()
	defer func() { close(stop); <-stopped }()
	s, channel, cfg := derivedChannelSetup(t, f, func(rule *dm.Rule) {
		n := &rule.Nodes[0]
		if op == "rate" {
			n.Op, n.Params.RateTimeUnit = "rate", "second"
		} else {
			n.Op = "window_" + op
			n.Params = dm.NodeParams{Window: &dm.Window{Kind: "trailing_duration", DurationNS: "1000000000"}, MinSamples: "1", MaxGapNS: "1000000000", MaxAgeNS: "1000000000"}
			if op == "variance" {
				n.Params.MinSamples = "2"
			}
			if op == "quantile" {
				n.Params.Percentile = "95"
			}
		}
		rule.Outputs[0].SemanticKey = "dp0_" + op
		rule.Outputs[0].Quantity = nil
	})
	s.WithStorage(f.storage)
	must(t, dm.InitializeStorage(ctx, f.storage))
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	var content strings.Builder
	content.Grow(total * 40)
	content.WriteString("observed_at,value,quality\n")
	for i := 0; i < total; i++ {
		fmt.Fprintf(&content, "%s,%d,good\n", base.Add(time.Duration(i)*time.Millisecond).Format(time.RFC3339Nano), i)
	}
	start := time.Now()
	f.publish(t, f.validate(t, f.upload(t, content.String())))
	content.Reset()
	importTime := time.Since(start)
	f.analytic(t, "overview", f.query(t))
	scope := dm.TenantScope(f.tenant)
	from, to, trialTo := strconv.FormatInt(base.UnixNano(), 10), strconv.FormatInt(base.Add(total*time.Millisecond).UnixNano(), 10), strconv.FormatInt(base.Add(3*time.Millisecond).UnixNano(), 10)
	trial, e := s.ChannelTrial(ctx, f.actor, scope, channel.Resource.ID, dm.TrialInput{WriteMeta: restoreMeta(channel.ConfigRevision), ConfigID: cfg.ID, ConfigHash: cfg.ConfigHash, OutputRange: dm.NanosecondRange{From: from, To: &trialTo}}, access.Meta{})
	must(t, e)
	if trial.Job.State != "succeeded" {
		_, e = f.service.RunTask(ctx, "dp0-trial", f.tenant, trial.Job.ID)
		must(t, e)
		trial, e = s.Run(ctx, f.actor, scope, trial.ID)
		must(t, e)
	}
	if trial.Trial == nil || trial.Trial.OutputCount != "3" {
		t.Fatal("small complete trial did not finish", trial)
	}
	preview, e := s.PreviewControl(ctx, f.actor, scope, channel.Resource.ID, dm.ControlPreviewInput{Revision: channel.ControlRevision, ControlProposal: dm.ControlProposal{Action: "enable", AutoStartNS: &from, StopNS: &to, TrialReceiptID: trial.Trial.ReceiptID}})
	must(t, e)
	_, e = s.ControlChannel(ctx, f.actor, scope, channel.Resource.ID, dm.ControlInput{WriteMeta: restoreMeta(channel.ControlRevision), PreviewID: preview.ID, PreviewHash: preview.Hash}, access.Meta{})
	must(t, e)
	n, e := s.ScheduleTenant(ctx, f.tenant)
	must(t, e)
	if n != 1 {
		t.Fatal("million target was not uniquely queued", n)
	}
	var job, run string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT job_id::text,id::text FROM derived_runs WHERE tenant_id=$1 AND channel_id=$2 AND mode='automatic'`, f.tenant, channel.Resource.ID).Scan(&job, &run))
	if op == "rate" {
		_, e = f.env.Pool.Exec(ctx, `CREATE FUNCTION fail_dp0_publish() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'dp0 publication interruption' USING ERRCODE='40001'; END $$; CREATE TRIGGER fail_dp0_publish BEFORE INSERT ON derived_generations FOR EACH ROW EXECUTE FUNCTION fail_dp0_publish()`)
		must(t, e)
	}
	start = time.Now()
	formalStarted := start
	_, e = f.service.RunTask(ctx, "dp0-formal", f.tenant, job)
	firstAttemptTime := time.Since(start)
	recoveryTime := time.Duration(0)
	if op == "rate" {
		if e == nil || !strings.Contains(e.Error(), "dp0 publication interruption") {
			t.Fatal("million recovery fault was not reached", e)
		}
		var visible, sealed int
		must(t, f.env.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM derived_generations WHERE tenant_id=$1),(SELECT count(*) FROM derived_write_attempts WHERE tenant_id=$1 AND run_id=$2 AND state='sealed')`, f.tenant, run).Scan(&visible, &sealed))
		if visible != 0 || sealed < 2 {
			t.Fatal("failure exposed partial results or did not partition", visible, sealed)
		}
		_, e = f.env.Pool.Exec(ctx, `DROP TRIGGER fail_dp0_publish ON derived_generations; DROP FUNCTION fail_dp0_publish(); UPDATE monitoring_jobs SET available_at=clock_timestamp() WHERE state='retry_wait'`)
		must(t, e)
		start = time.Now()
		_, e = f.service.RunTask(ctx, "dp0-resumed", f.tenant, job)
		must(t, e)
		recoveryTime = time.Since(start)
		var after int
		must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM derived_write_attempts WHERE tenant_id=$1 AND run_id=$2 AND state='sealed'`, f.tenant, run).Scan(&after))
		if after != sealed {
			t.Fatal("recovery rewrote sealed million-row attempts", sealed, after)
		}
	} else {
		must(t, e)
	}
	// Include the interrupted attempt and recovery through the successful PG
	// publication. The failed attempt alone is not a completed formal result.
	computeTime := time.Since(formalStarted)
	var generation string
	var formalRows uint64
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text,row_count FROM derived_generations WHERE tenant_id=$1 AND run_id=$2`, f.tenant, run).Scan(&generation, &formalRows))
	if formalRows != total {
		t.Fatal("formal million target was truncated", formalRows)
	}
	group, e := f.storage.CalculationTransport().Group("local")
	must(t, e)
	var physicalBytes uint64
	for _, replica := range group.Replicas {
		var rows, unique, bytes uint64
		must(t, replica.QueryRow(ctx, `SELECT count(),uniqExact(result_id) FROM `+f.database+`.derived_results_local WHERE tenant_id=? AND generation_id=?`, uuid.MustParse(f.tenant), uuid.MustParse(generation)).Scan(&rows, &unique))
		if rows != total || unique != total {
			t.Fatal("replica lost or duplicated logical results", rows, unique)
		}
		must(t, replica.QueryRow(ctx, `SELECT sum(bytes_on_disk) FROM system.parts WHERE active AND database=?`, f.database).Scan(&bytes))
		physicalBytes += bytes
	}
	selection, e := f.service.Select(ctx, f.actor, f.scope, md.SelectionInput{Channels: []string{channel.Resource.ID}})
	must(t, e)
	fromText, toText := base.Format(time.RFC3339Nano), base.Add(total*time.Millisecond).Format(time.RFC3339Nano)
	query := md.QueryInput{Selection: selection.ID, Range: &md.Range{From: &fromText, To: &toText}, Limit: 2}
	firstTo := base.Add(2 * time.Millisecond).Format(time.RFC3339Nano)
	firstQuery := query
	firstQuery.Range = &md.Range{From: &fromText, To: &firstTo}
	page, e := f.service.Details(ctx, f.actor, f.scope, firstQuery)
	must(t, e)
	if len(page.Items) != 2 || len(page.Coverage) != 1 || page.Coverage[0].State != "current" {
		t.Fatal("million current query lost complete coverage", page)
	}
	lastFrom := base.Add((total - 2) * time.Millisecond).Format(time.RFC3339Nano)
	lastQuery := query
	lastQuery.Range = &md.Range{From: &lastFrom, To: &toText}
	last, e := f.service.Details(ctx, f.actor, f.scope, lastQuery)
	must(t, e)
	if len(last.Items) != 2 || last.Items[0].At != base.Add((total-1)*time.Millisecond).Format(time.RFC3339Nano) {
		t.Fatal("million current tail query was incomplete", last)
	}
	for i, row := range last.Items {
		value, _ := loadExpected(op, total-1-i, width)
		if row.Calculated == nil || row.Calculated.Value == nil || *row.Calculated.Value != value {
			t.Fatalf("current tail %d did not match %s", i, value)
		}
	}
	start = time.Now()
	export := f.completeExport(t, md.ExportInput{WriteMeta: f.meta("0"), Query: query, Kind: "details", Format: "csv"})
	var count int
	for _, part := range export.Files {
		d, e := f.service.ExportDownload(ctx, f.actor, f.scope, export.ID, part.Part)
		must(t, e)
		must(t, walkLoadCSV(d, func(columns map[string]int, row []string) error {
			at, e := time.Parse(time.RFC3339Nano, row[columns["observed_at"]])
			if e != nil || at.UnixNano() != base.Add(time.Duration(count)*time.Millisecond).UnixNano() {
				return fmt.Errorf("result time/order mismatch at %d", count)
			}
			value, status := loadExpected(op, count, width)
			if row[columns["value"]] != value || row[columns["calculation_status"]] != status || row[columns["result_id"]] == "" || row[columns["generation_id"]] != generation {
				return fmt.Errorf("result %d: %s/%s want %s/%s", count, row[columns["value"]], row[columns["calculation_status"]], value, status)
			}
			count++
			return nil
		}))
	}
	if count != total || export.Rows != "1000000" {
		t.Fatal("full result export truncated", count, export.Rows)
	}
	exportTime := time.Since(start)
	start = time.Now()
	evidence, e := s.CreateGenerationExport(ctx, f.actor, scope, generation, dm.GenerationExportInput{WriteMeta: restoreMeta(""), Format: "csv"}, access.Meta{})
	must(t, e)
	_, e = f.service.RunTask(ctx, "dp0-input-evidence", f.tenant, evidence.Job.ID)
	must(t, e)
	manifest, e := s.GenerationExportFiles(ctx, f.actor, scope, evidence.ID)
	must(t, e)
	count = 0
	for _, part := range manifest.Files {
		d, e := s.GenerationExportDownload(ctx, f.actor, scope, evidence.ID, part.Part)
		must(t, e)
		must(t, walkLoadCSV(d, func(columns map[string]int, row []string) error {
			at, e := time.Parse(time.RFC3339Nano, row[columns["observed_at"]])
			if e != nil || at.UnixNano() != base.Add(time.Duration(count)*time.Millisecond).UnixNano() || row[columns["value"]] != fmt.Sprintf("%d.000000000000", count) || row[columns["adoption_id"]] == "" || row[columns["observation_id"]] == "" {
				return fmt.Errorf("input evidence %d changed or lost identity", count)
			}
			count++
			return nil
		}))
	}
	if count != total || manifest.Rows != "1000000" {
		t.Fatal("full input export truncated", count, manifest.Rows)
	}
	evidenceTime := time.Since(start)
	objects, e := minio.New("127.0.0.1:58333", &minio.Options{Creds: credentials.NewStaticV4("contract-admin", "contract-admin-test-only", ""), BucketLookup: minio.BucketLookupPath})
	must(t, e)
	var objectBytes uint64
	for object := range objects.ListObjects(ctx, "contract-source", minio.ListObjectsOptions{Recursive: true, Prefix: f.tenant + "/"}) {
		must(t, object.Err)
		objectBytes += uint64(object.Size)
	}
	var pgBytes uint64
	must(t, f.env.Pool.QueryRow(ctx, `SELECT COALESCE(sum(pg_total_relation_size(c.oid)),0)::bigint FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=current_schema() AND c.relkind='r'`).Scan(&pgBytes))
	minimum := 500.0
	if op == "rate" {
		minimum = 1000
	}
	report := map[string]any{"operator": op, "inputs": total, "outputs": formalRows, "import_seconds": importTime.Seconds(), "first_attempt_seconds": firstAttemptTime.Seconds(), "first_attempt_rows_per_second": total / firstAttemptTime.Seconds(), "formal_seconds": computeTime.Seconds(), "formal_rows_per_second": total / computeTime.Seconds(), "minimum_rows_per_second": minimum, "recovery_seconds": recoveryTime.Seconds(), "recovery_max_seconds": 600, "result_export_seconds": exportTime.Seconds(), "input_export_seconds": evidenceTime.Seconds(), "peak_heap_bytes": peakHeap.Load(), "peak_temporary_bytes": peakTemp.Load(), "postgres_bytes": pgBytes, "clickhouse_replicas_bytes": physicalBytes, "s3_tenant_bytes": objectBytes}
	encoded, e := json.Marshal(report)
	must(t, e)
	t.Logf("DP0_PIPELINE %s", encoded)
	if total/computeTime.Seconds() < minimum || recoveryTime > 600*time.Second || peakHeap.Load() > 1<<30 || peakTemp.Load() > 20<<30 {
		t.Fatal("DP-0 frozen throughput/recovery/resource threshold failed")
	}
	if op == "quantile" {
		verifyMillionActiveWindow(t, f, s, cfg, base)
		t.Logf("DP0_MAXIMUM_WINDOW_RESOURCES peak_heap_bytes=%d peak_temporary_bytes=%d", peakHeap.Load(), peakTemp.Load())
		if peakHeap.Load() > 1<<30 || peakTemp.Load() > 20<<30 {
			t.Fatal("maximum active window exceeded frozen resource bounds")
		}
	}
}

func verifyMillionActiveWindow(t *testing.T, f monitoringFixture, s *dm.Service, cfg dm.ChannelConfig, base time.Time) {
	t.Helper()
	var rule dm.Rule
	must(t, json.Unmarshal(cfg.Rule, &rule))
	endpoint := base.Add(1000 * time.Second)
	rule.Clock = dm.Clock{Kind: "fixed_grid", StepNS: "1000000000000", AnchorNS: strconv.FormatInt(base.UnixNano(), 10)}
	params := dm.NodeParams{Window: &dm.Window{Kind: "trailing_duration", DurationNS: "1001000000000"}, MaxGapNS: "1001000000000", MaxAgeNS: "1001000000000"}
	rule.Nodes = []dm.Node{{ID: "count", Op: "window_count", Inputs: []string{"input:x"}, Params: params}, {ID: "p95", Op: "window_quantile", Inputs: []string{"input:x"}, Params: params}}
	rule.Nodes[1].Params.Percentile = "95"
	output := rule.Outputs[0]
	rule.Outputs = nil
	for _, key := range []string{"count", "p95"} {
		o := output
		o.Key, o.Node, o.SemanticKey = key, key, "maximum_window_"+key
		o.Quantity = nil
		if key == "count" {
			o.Numeric.Scale = 0
		}
		rule.Outputs = append(rule.Outputs, o)
	}
	raw, e := json.Marshal(rule)
	must(t, e)
	scope := dm.TenantScope(f.tenant)
	created, e := s.CreateAnalysis(ctx, f.actor, scope, dm.CreateAnalysisInput{WriteMeta: restoreMeta(""), Code: "MILLION_WINDOW", Name: "百万活动窗口边界", Body: dm.AnalysisBody{Rule: raw, InputBindings: cfg.InputBindings, Parameters: cfg.Parameters}}, access.Meta{})
	must(t, e)
	validation, e := s.ValidateAnalysis(ctx, f.actor, scope, created.Config.ID, dm.ValidateInput{DraftRevision: created.Config.Draft.Revision})
	must(t, e)
	published, e := s.PublishAnalysis(ctx, f.actor, scope, created.Config.ID, dm.PublishAnalysisInput{PublishMetricInput: dm.PublishMetricInput{WriteMeta: restoreMeta(created.Config.Revision), DraftRevision: created.Config.Draft.Revision, ValidationHash: validation.ValidationHash}}, access.Meta{})
	must(t, e)
	end := strconv.FormatInt(endpoint.Add(time.Nanosecond).UnixNano(), 10)
	interval := dm.NanosecondRange{From: strconv.FormatInt(endpoint.UnixNano(), 10), To: &end}
	source := dm.NanosecondRange{From: strconv.FormatInt(base.Add(-time.Second).UnixNano(), 10), To: &end}
	startRun := func() dm.Run {
		run, e := s.StartAnalysis(ctx, f.actor, scope, dm.AnalysisRunInput{WriteMeta: restoreMeta(""), ConfigVersionID: published.Version.ID, ConfigHash: published.Version.ConfigHash, OutputRange: interval, SourceRange: &source}, false, access.Meta{})
		must(t, e)
		return run
	}
	start := time.Now()
	run := startRun()
	_, e = f.service.RunTask(ctx, "maximum-active-window", f.tenant, run.Job.ID)
	must(t, e)
	page, e := s.AnalysisArtifactRows(ctx, f.actor, scope, run.Analysis.ArtifactID, dm.AnalysisArtifactQuery{VersionID: run.Analysis.ArtifactVersionID, Range: &interval, Limit: 10})
	must(t, e)
	want := map[string]string{"count": "1000000", "p95": "949999.050000000000000000"}
	if len(page.Items) != 2 {
		t.Fatal("maximum active window lost an output", page)
	}
	for _, row := range page.Items {
		if row.Value == nil || *row.Value != want[row.Output] || row.Count != "1000000" || row.Interval != nil {
			t.Fatal("maximum active window changed exact value or instant semantics", row)
		}
	}
	validSeconds := time.Since(start).Seconds()
	f.publish(t, f.validate(t, f.upload(t, fmt.Sprintf("observed_at,value,quality\n%s,1000000,good\n", endpoint.Add(-500*time.Microsecond).Format(time.RFC3339Nano)))))
	tooLarge := startRun()
	_, e = f.service.RunTask(ctx, "oversized-active-window", f.tenant, tooLarge.Job.ID)
	if e == nil || !strings.Contains(e.Error(), "budget") {
		t.Fatal("million-plus-one active input did not stop with explicit budget error", e)
	}
	tooLarge, e = s.Run(ctx, f.actor, scope, tooLarge.ID)
	must(t, e)
	if tooLarge.Job.State != "needs_action" || tooLarge.Job.ErrorCode != "budget_exceeded" {
		t.Fatal("oversized active window became a successful partial artifact", tooLarge.Job)
	}
	t.Logf("DP0_MAXIMUM_WINDOW accepted=1000000 rejected=1000001 exact_p95=%s seconds=%.3f", want["p95"], validSeconds)
}

func loadExpected(op string, index, width int) (string, string) {
	n := min(index+1, width)
	if index == 0 && (op == "rate" || op == "variance") {
		if op == "rate" {
			return "", "missing_input"
		}
		return "", "insufficient_samples"
	}
	var value *big.Rat
	switch op {
	case "rate":
		return "1000.000000000000000000", "valid"
	case "mean":
		value = new(big.Rat).SetFrac64(int64(2*index-(n-1)), 2)
	case "variance":
		value = new(big.Rat).SetFrac64(int64(n*n-1), 12)
	case "quantile":
		value = new(big.Rat).SetFrac64(int64(20*index-(n-1)), 20)
	}
	return value.FloatString(18), "valid"
}
