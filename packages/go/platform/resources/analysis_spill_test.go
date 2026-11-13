//go:build monitoringintegration

package resources_test

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func TestAnalysisExternalWindowExactAndCheckpointRecovery(t *testing.T) {
	f := monitoringSetup(t)
	s, _, cfg := derivedChannelSetup(t, f)
	s.WithStorage(f.storage)
	must(t, dm.InitializeStorage(ctx, f.storage))
	opts := config.DefaultDerived()
	opts.WriteRows = 1000
	opts.ReadBytes = 1 // Force the external path independently of output batch size.
	opts.WindowSamples = 10
	opts.TemporaryTTL = time.Millisecond
	opts.TrialTTL = time.Millisecond
	s.WithOptions(opts)
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	const count = 40000
	var csv strings.Builder
	csv.WriteString("observed_at,value,quality\n")
	for i := 0; i < count; i++ {
		fmt.Fprintf(&csv, "%s,1000000000000000000000000.%012d,good\n", base.Add(time.Duration(i)*time.Second).Format(time.RFC3339Nano), count-1-i)
	}
	f.publish(t, f.validate(t, f.upload(t, csv.String())))
	// This test runs selected tasks directly. Complete the queued raw summary
	// before the maintenance scan, as the shared worker normally would.
	f.analytic(t, "overview", f.query(t))
	var rule dm.Rule
	must(t, json.Unmarshal(cfg.Rule, &rule))
	rule.Clock = dm.Clock{Kind: "whole_range"}
	rule.Nodes = nil
	output := rule.Outputs[0]
	rule.Outputs = nil
	for _, name := range []string{"mean", "median", "quantile", "variance"} {
		params := dm.NodeParams{Window: &dm.Window{Kind: "whole_range"}}
		if name == "quantile" {
			params.Percentile = "95"
		}
		rule.Nodes = append(rule.Nodes, dm.Node{ID: name, Op: "window_" + name, Inputs: []string{"input:x"}, Params: params})
		o := output
		o.Key = name
		o.Node = name
		o.SemanticKey = "exact_" + name
		o.Quantity = nil
		o.Numeric.Scale = 36
		rule.Outputs = append(rule.Outputs, o)
	}
	raw, e := json.Marshal(rule)
	must(t, e)
	scope := dm.TenantScope(f.tenant)
	created, e := s.CreateAnalysis(ctx, f.actor, scope, dm.CreateAnalysisInput{WriteMeta: restoreMeta(""), Code: "EXTERNAL", Name: "精确大窗口", Body: dm.AnalysisBody{Rule: raw, InputBindings: cfg.InputBindings, Parameters: cfg.Parameters}}, access.Meta{})
	must(t, e)
	validation, e := s.ValidateAnalysis(ctx, f.actor, scope, created.Config.ID, dm.ValidateInput{DraftRevision: created.Config.Draft.Revision})
	must(t, e)
	published, e := s.PublishAnalysis(ctx, f.actor, scope, created.Config.ID, dm.PublishAnalysisInput{PublishMetricInput: dm.PublishMetricInput{WriteMeta: restoreMeta(created.Config.Revision), DraftRevision: created.Config.Draft.Revision, ValidationHash: validation.ValidationHash}}, access.Meta{})
	must(t, e)
	to := strconv.FormatInt(base.Add(count*time.Second).UnixNano(), 10)
	interval := dm.NanosecondRange{From: strconv.FormatInt(base.UnixNano(), 10), To: &to}
	request := func() dm.AnalysisRunInput {
		return dm.AnalysisRunInput{WriteMeta: restoreMeta(""), ConfigVersionID: published.Version.ID, ConfigHash: published.Version.ConfigHash, OutputRange: interval, SourceRange: &interval}
	}
	trial, e := s.StartAnalysis(ctx, f.actor, scope, request(), true, access.Meta{})
	must(t, e)
	_, e = f.service.RunTask(ctx, "external-window-trial", f.tenant, trial.Job.ID)
	must(t, e)
	trial, e = s.Run(ctx, f.actor, scope, trial.ID)
	must(t, e)
	if trial.Trial == nil || trial.Trial.OutputCount != "4" || trial.Trial.InputCounts.Rows != count {
		t.Fatal("large fixed trial was truncated", trial)
	}
	run, e := s.StartAnalysis(ctx, f.actor, scope, request(), false, access.Meta{})
	must(t, e)
	_, e = f.env.Pool.Exec(ctx, `CREATE FUNCTION fail_during_sort() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.step_key LIKE 'sort:%' AND EXISTS(SELECT 1 FROM monitoring_job_steps WHERE job_id=NEW.job_id AND step_key LIKE 'window-source:%') THEN RAISE EXCEPTION 'injected during external sort' USING ERRCODE='40001'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_during_sort BEFORE INSERT OR UPDATE ON monitoring_job_steps FOR EACH ROW EXECUTE FUNCTION fail_during_sort()`)
	must(t, e)
	_, e = f.service.RunTask(ctx, "external-mid-sort", f.tenant, run.Job.ID)
	if e == nil || !strings.Contains(e.Error(), "injected during external sort") {
		t.Fatal("mid-sort fault was not exercised", e)
	}
	var retainedKeys []string
	var originalFence int64
	var processed string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT array_agg(step_key ORDER BY step_key),max(fence) FROM monitoring_job_steps WHERE job_id=$1 AND step_key LIKE 'sort:%'`, run.Job.ID).Scan(&retainedKeys, &originalFence))
	must(t, f.env.Pool.QueryRow(ctx, `SELECT result->>'processed_rows' FROM monitoring_job_steps WHERE job_id=$1 AND step_key LIKE 'window-source:%'`, run.Job.ID).Scan(&processed))
	if len(retainedKeys) != 2 || processed != "16384" {
		t.Fatal("mid-sort source cursor and exact runs were not committed together", len(retainedKeys), processed)
	}
	var midWatermark string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT commit_sequence::text FROM monitoring_tenant_states WHERE tenant_id=$1`, f.tenant).Scan(&midWatermark))
	maintenance, e := f.service.QueueVerification(ctx, f.tenant, midWatermark, "重试中的封存排序段必须保留")
	must(t, e)
	_, e = f.service.RunTask(ctx, "external-active-retention", f.tenant, maintenance)
	must(t, e)
	maintenanceReport, e := f.service.OperatorJobReport(ctx, f.tenant, maintenance)
	must(t, e)
	if maintenanceReport.State != "passed" {
		t.Fatal("maintenance could not verify the completed raw source", maintenanceReport)
	}
	_, e = f.env.Pool.Exec(ctx, `DROP TRIGGER fail_during_sort ON monitoring_job_steps; DROP FUNCTION fail_during_sort(); UPDATE monitoring_jobs SET available_at=clock_timestamp() WHERE state='retry_wait'`)
	must(t, e)
	_, e = f.env.Pool.Exec(ctx, `CREATE FUNCTION fail_after_sort() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected after external sort' USING ERRCODE='40001'; END $$; CREATE TRIGGER fail_after_sort BEFORE INSERT ON derived_write_attempts FOR EACH ROW EXECUTE FUNCTION fail_after_sort()`)
	must(t, e)
	_, e = f.service.RunTask(ctx, "external-before-write", f.tenant, run.Job.ID)
	if e == nil || !strings.Contains(e.Error(), "injected after external sort") {
		t.Fatal("external checkpoint fault not exercised", e)
	}
	var retainedFence int64
	must(t, f.env.Pool.QueryRow(ctx, `SELECT max(fence) FROM monitoring_job_steps WHERE job_id=$1 AND step_key=ANY($2)`, run.Job.ID, retainedKeys).Scan(&retainedFence))
	if retainedFence != originalFence {
		t.Fatal("resumed source prefix was sorted and sealed again")
	}
	var segments int
	var fence int64
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*),max(fence) FROM monitoring_job_steps WHERE job_id=$1 AND step_key LIKE 'sort:%'`, run.Job.ID).Scan(&segments, &fence))
	if segments < 6 {
		t.Fatal("large sort did not seal bounded runs", segments)
	}
	_, e = f.env.Pool.Exec(ctx, `DROP TRIGGER fail_after_sort ON derived_write_attempts; DROP FUNCTION fail_after_sort(); UPDATE monitoring_jobs SET available_at=clock_timestamp() WHERE state='retry_wait'`)
	must(t, e)
	_, e = f.service.RunTask(ctx, "external-resume", f.tenant, run.Job.ID)
	must(t, e)
	var after int64
	must(t, f.env.Pool.QueryRow(ctx, `SELECT max(fence) FROM monitoring_job_steps WHERE job_id=$1 AND step_key LIKE 'sort:%'`, run.Job.ID).Scan(&after))
	if after != fence {
		t.Fatal("complete external checkpoint was sorted again")
	}
	page, e := s.AnalysisArtifactRows(ctx, f.actor, scope, run.Analysis.ArtifactID, dm.AnalysisArtifactQuery{VersionID: run.Analysis.ArtifactVersionID, Range: &interval, Limit: 10})
	must(t, e)
	if len(page.Items) != 4 {
		t.Fatal(page)
	}
	large, _ := new(big.Rat).SetString("1000000000000000000000000")
	mean := new(big.Rat).Add(large, new(big.Rat).SetFrac64(count-1, 2*1000000000000))
	p95 := new(big.Rat).Add(large, new(big.Rat).SetFrac64((count-1)*95, 100*1000000000000))
	den := new(big.Int).Exp(big.NewInt(10), big.NewInt(24), nil)
	den.Mul(den, big.NewInt(12))
	variance := new(big.Rat).SetFrac(big.NewInt(count*count-1), den)
	want := map[string]string{"mean": mean.FloatString(36), "median": mean.FloatString(36), "quantile": p95.FloatString(36), "variance": variance.FloatString(36)}
	for _, row := range page.Items {
		if row.Value == nil || *row.Value != want[row.Output] || row.Count != "40000" {
			t.Fatal("external exact result differs from independent rational oracle", row, want)
		}
	}
	for _, row := range trial.Trial.Examples {
		if row.Value == nil || *row.Value != want[row.Output] {
			t.Fatal("trial and formal external window disagree", row)
		}
	}
	f.service.WithResultExportTTL(10 * time.Second)
	export, e := s.CreateAnalysisExport(ctx, f.actor, scope, run.Analysis.ArtifactID, dm.AnalysisExportInput{WriteMeta: restoreMeta(""), VersionID: run.Analysis.ArtifactVersionID, Range: &interval, Format: "csv"}, access.Meta{})
	must(t, e)
	_, e = f.service.RunTask(ctx, "external-expiring-export", f.tenant, export.Job.ID)
	must(t, e)
	_, e = s.AnalysisExportFiles(ctx, f.actor, scope, export.ID)
	must(t, e)
	if remaining := time.Until(export.ExpiresAt); remaining > 0 {
		time.Sleep(remaining + 20*time.Millisecond)
	}
	var watermark string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT commit_sequence::text FROM monitoring_tenant_states WHERE tenant_id=$1`, f.tenant).Scan(&watermark))
	verification, e := f.service.QueueVerification(ctx, f.tenant, watermark, "完成大窗口后的临时排序与试算回收")
	must(t, e)
	_, e = f.service.RunTask(ctx, "external-retention", f.tenant, verification)
	must(t, e)
	finalReport, e := f.service.OperatorJobReport(ctx, f.tenant, verification)
	must(t, e)
	if finalReport.State != "passed" {
		t.Fatalf("final maintenance: %s", finalReport.Calculations)
	}
	var temporary, checkpoints, receipts int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM monitoring_retention_receipts WHERE tenant_id=$1 AND kind='derived_temp' AND state='completed'),(SELECT count(*) FROM monitoring_retention_receipts WHERE tenant_id=$1 AND kind='derived_checkpoint' AND state='completed'),(SELECT count(*) FROM analysis_trial_receipts WHERE tenant_id=$1)`, f.tenant).Scan(&temporary, &checkpoints, &receipts))
	if temporary != 2 || checkpoints < 6 || receipts != 0 {
		t.Fatal("external temporary lifecycle incomplete", temporary, checkpoints, receipts)
	}
	var expiredExports int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM monitoring_retention_receipts WHERE tenant_id=$1 AND kind='export' AND target=$2 AND state='completed'`, f.tenant, export.ID).Scan(&expiredExports))
	if expiredExports != 1 {
		t.Fatal("expired managed export was not collected")
	}
	objects, e := minio.New("127.0.0.1:58333", &minio.Options{Creds: credentials.NewStaticV4("contract-admin", "contract-admin-test-only", "")})
	must(t, e)
	for _, prefix := range []string{f.tenant + "/derived/temp/", f.tenant + "/exports/" + export.ID + "/"} {
		for object := range objects.ListObjects(ctx, "contract-source", minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
			must(t, object.Err)
			t.Fatal("retention left a temporary object", object.Key)
		}
	}
	if _, e = s.AnalysisExportDownload(ctx, f.actor, scope, export.ID, "1"); e == nil {
		t.Fatal("expired export can still be downloaded")
	}
	page, e = s.AnalysisArtifactRows(ctx, f.actor, scope, run.Analysis.ArtifactID, dm.AnalysisArtifactQuery{VersionID: run.Analysis.ArtifactVersionID, Range: &interval, Limit: 10})
	must(t, e)
	if len(page.Items) != 4 {
		t.Fatal("temporary cleanup removed permanent analysis rows")
	}
	reconciled, e := dm.NewRecovery(f.storage).Reconcile(ctx, f.env.Pool, f.tenant)
	must(t, e)
	if reconciled.State != "passed" {
		t.Fatal("temporary cleanup damaged recoverable evidence", reconciled)
	}
	t.Logf("40000 exact inputs, %d sealed sort segments, retained checkpoint after write failure; mean/median/R7 P95/variance match independent rational oracle", segments)
}
