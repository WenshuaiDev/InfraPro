//go:build monitoringintegration

package resources_test

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func TestDerivedMissingBlockQuarantinesCachesAndRepairsByNewGeneration(t *testing.T) {
	f := monitoringSetup(t)
	s, ch, cfg := derivedChannelSetup(t, f)
	s.WithStorage(f.storage)
	options := config.DefaultDerived()
	options.TemporaryTTL = time.Millisecond
	options.TrialTTL = 5 * time.Second
	s.WithOptions(options)
	must(t, dm.InitializeStorage(ctx, f.storage))
	f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-01T00:00:00Z,2,good\n2024-01-01T00:01:00Z,5,good\n2024-01-01T00:02:00Z,9,good\n")))
	// Complete the raw summary explicitly before isolating a derived result
	// fault. Exporting derived values need not claim the raw summary job.
	f.analytic(t, "overview", f.query(t))
	rawReport, e := f.service.Verify(ctx, f.tenant)
	must(t, e)
	if rawReport.State != "passed" {
		t.Fatal("raw source was not ready before the derived integrity fault", rawReport)
	}
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	from, to := strconv.FormatInt(base.UnixNano(), 10), strconv.FormatInt(base.Add(3*time.Minute).UnixNano(), 10)
	interval := dm.NanosecondRange{From: from, To: &to}
	scope := dm.TenantScope(f.tenant)
	trial, e := s.ChannelTrial(ctx, f.actor, scope, ch.Resource.ID, dm.TrialInput{WriteMeta: restoreMeta(ch.ConfigRevision), ConfigID: cfg.ID, ConfigHash: cfg.ConfigHash, OutputRange: interval}, access.Meta{})
	must(t, e)
	preview, e := s.PreviewControl(ctx, f.actor, scope, ch.Resource.ID, dm.ControlPreviewInput{Revision: ch.ControlRevision, ControlProposal: dm.ControlProposal{Action: "enable", AutoStartNS: &from, StopNS: &to, TrialReceiptID: trial.Trial.ReceiptID}})
	must(t, e)
	enabled, e := s.ControlChannel(ctx, f.actor, scope, ch.Resource.ID, dm.ControlInput{WriteMeta: restoreMeta(ch.ControlRevision), PreviewID: preview.ID, PreviewHash: preview.Hash}, access.Meta{})
	must(t, e)
	ch = enabled.Channel
	// This receipt is deliberately short-lived; retention must not depend on
	// how long the subsequent storage fault operations happen to take.
	options.TrialTTL = time.Millisecond
	s.WithOptions(options)
	unusedTrial, e := s.ChannelTrial(ctx, f.actor, scope, ch.Resource.ID, dm.TrialInput{WriteMeta: restoreMeta(ch.ConfigRevision), ConfigID: cfg.ID, ConfigHash: cfg.ConfigHash, OutputRange: interval}, access.Meta{})
	must(t, e)
	options.TrialTTL = 5 * time.Second
	s.WithOptions(options)
	_, e = s.ScheduleTenant(ctx, f.tenant)
	must(t, e)
	var job, generation, object string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT job_id::text FROM derived_runs WHERE tenant_id=$1 AND mode='automatic'`, f.tenant).Scan(&job))
	_, e = f.env.Pool.Exec(ctx, `CREATE FUNCTION fail_result_seal() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.state='sealed' THEN RAISE EXCEPTION 'injected result seal lost' USING ERRCODE='40001'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_result_seal BEFORE UPDATE ON derived_write_attempts FOR EACH ROW EXECUTE FUNCTION fail_result_seal()`)
	must(t, e)
	_, e = f.service.RunTask(ctx, "integrity-uncertain-seal", f.tenant, job)
	if e == nil || !strings.Contains(e.Error(), "injected result seal lost") {
		t.Fatal("unselected physical attempt fault not exercised", e)
	}
	var unusedAttempt string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM derived_write_attempts WHERE tenant_id=$1 AND state='writing'`, f.tenant).Scan(&unusedAttempt))
	_, e = f.env.Pool.Exec(ctx, `DROP TRIGGER fail_result_seal ON derived_write_attempts; DROP FUNCTION fail_result_seal(); UPDATE monitoring_jobs SET available_at=clock_timestamp() WHERE state='retry_wait'`)
	must(t, e)
	_, e = f.service.RunTask(ctx, "integrity-original", f.tenant, job)
	must(t, e)
	must(t, f.env.Pool.QueryRow(ctx, `SELECT g.id::text,a.manifest->>'object_key' FROM derived_generations g JOIN derived_write_attempts a ON a.tenant_id=g.tenant_id AND a.run_id=g.run_id AND a.state='sealed' WHERE g.tenant_id=$1`, f.tenant).Scan(&generation, &object))
	selection, e := f.service.Select(ctx, f.actor, f.scope, md.SelectionInput{Channels: []string{ch.Resource.ID}})
	must(t, e)
	query := f.query(t)
	query.Selection = selection.ID
	query.Limit = 100
	for _, action := range []string{"latest", "overview", "statistics", "trends"} {
		_, e = f.service.Analytics(ctx, f.actor, f.scope, action, query)
		must(t, e)
	}
	exports := verifyDerivedOnlyExports(t, f, query)
	objects, e := minio.New("127.0.0.1:58333", &minio.Options{Creds: credentials.NewStaticV4("contract-admin", "contract-admin-test-only", "")})
	must(t, e)
	must(t, objects.RemoveObject(ctx, "contract-source", object, minio.RemoveObjectOptions{}))
	readonly, e := dm.NewRecovery(f.storage).Reconcile(ctx, f.env.Pool, f.tenant)
	must(t, e)
	if readonly.State != "failed" {
		t.Fatal("missing result escaped read-only reconciliation")
	}
	var recorded int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM derived_result_integrity WHERE tenant_id=$1`, f.tenant).Scan(&recorded))
	if recorded != 0 {
		t.Fatal("read-only reconciliation modified availability")
	}
	var watermark string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT commit_sequence::text FROM monitoring_tenant_states WHERE tenant_id=$1`, f.tenant).Scan(&watermark))
	verification, e := f.service.QueueVerification(ctx, f.tenant, watermark, "派生缺块维护隔离验证")
	must(t, e)
	_, e = f.service.RunTask(ctx, "integrity-quarantine", f.tenant, verification)
	must(t, e)
	report, e := f.service.OperatorJobReport(ctx, f.tenant, verification)
	must(t, e)
	var calculations dm.Reconciliation
	must(t, json.Unmarshal(report.Calculations, &calculations))
	if report.State != "failed" || calculations.FindingCount != 1 || len(report.Findings) != 1 || report.Findings[0].Code != "calculation_result_integrity_unavailable" {
		t.Fatal("maintenance failed to report quarantined generation", report)
	}
	var unused, adopted, retired int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM derived_trial_receipts WHERE tenant_id=$1 AND run_id=$2),(SELECT count(*) FROM derived_trial_receipts WHERE tenant_id=$1 AND run_id=$3),(SELECT count(*) FROM monitoring_retention_receipts WHERE tenant_id=$1 AND kind='derived_attempt' AND target=$4 AND state='completed')`, f.tenant, unusedTrial.ID, trial.ID, unusedAttempt).Scan(&unused, &adopted, &retired))
	if unused != 0 || adopted != 1 || retired != 1 {
		t.Fatal("retention lost adopted proof or kept unused artifacts", unused, adopted, retired)
	}
	unusedTrial, e = s.Run(ctx, f.actor, scope, unusedTrial.ID)
	must(t, e)
	if unusedTrial.Trial != nil || unusedTrial.Job.ResultState != "expired" {
		t.Fatal("expired trial appeared as a completed empty result", unusedTrial)
	}
	replicas, e := f.storage.CalculationTransport().Group("local")
	must(t, e)
	for _, replica := range replicas.Replicas {
		var remaining uint64
		must(t, replica.QueryRow(ctx, "SELECT count() FROM "+f.database+".derived_results_local WHERE toString(attempt_id)=?", unusedAttempt).Scan(&remaining))
		if remaining != 0 {
			t.Fatal("unselected attempt remained on replica", remaining)
		}
	}
	denied := func(operation string, e error) {
		t.Helper()
		var issue *dm.Error
		var host *md.Error
		if !(errors.As(e, &issue) && issue.Code == "result_integrity_unavailable") && !(errors.As(e, &host) && host.Code == "result_integrity_unavailable") {
			t.Fatal(operation, "known missing result appeared empty or remained cached", e)
		}
	}
	_, e = f.service.Details(ctx, f.actor, f.scope, query)
	denied("details", e)
	for _, action := range []string{"latest", "overview", "statistics", "trends"} {
		_, e = f.service.Analytics(ctx, f.actor, f.scope, action, query)
		denied(action, e)
	}
	_, e = s.GenerationDetails(ctx, f.actor, scope, generation, dm.GenerationQuery{})
	denied("generation history", e)
	_, e = f.service.ExportDownload(ctx, f.actor, f.scope, exports[0], "1")
	denied("export download", e)
	backfill, e := s.PreviewBackfill(ctx, f.actor, scope, ch.Resource.ID, dm.BackfillPreviewInput{Revision: ch.ConfigRevision, Range: interval})
	must(t, e)
	repair, e := s.BackfillChannel(ctx, f.actor, scope, ch.Resource.ID, dm.ControlInput{WriteMeta: restoreMeta(ch.ConfigRevision), PreviewID: backfill.ID, PreviewHash: backfill.Hash}, access.Meta{})
	must(t, e)
	for _, run := range repair.Runs {
		_, e = f.service.RunTask(ctx, "integrity-new-publication", f.tenant, run.Job.ID)
		must(t, e)
	}
	query.Snapshot = ""
	page, e := f.service.Details(ctx, f.actor, f.scope, query)
	must(t, e)
	if len(page.Items) != 3 || page.Items[0].Calculated.Generation == generation || page.Items[0].Calculated.Value == nil || *page.Items[0].Calculated.Value != "4.000000000000000000" {
		t.Fatal("repair did not publish a distinct complete generation", page)
	}
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM derived_result_integrity WHERE tenant_id=$1 AND result_id=$2 AND state='unavailable'`, f.tenant, generation).Scan(&recorded))
	if recorded != 1 {
		t.Fatal("new publication hid old missing history")
	}
}
