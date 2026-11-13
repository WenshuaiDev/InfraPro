//go:build monitoringintegration

package resources_test

import (
	"bytes"
	"strconv"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/google/uuid"
)

func TestDerivedFormalPublicationUsesSealedAttempts(t *testing.T) {
	f := monitoringSetup(t)
	service, channel, cfg := derivedChannelSetup(t, f)
	service.WithStorage(f.storage)
	must(t, dm.InitializeStorage(ctx, f.storage))
	scope := dm.TenantScope(f.tenant)
	meta := func(revision string) dm.WriteMeta {
		return dm.WriteMeta{RequestID: uuid.NewString(), Revision: revision, Reason: "正式结果原子发布验证"}
	}
	batch := f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-01T00:00:00Z,2,good\n2024-01-01T00:01:00Z,5,good\n2024-01-01T00:02:00Z,9,good\n")))
	start := strconv.FormatInt(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).UnixNano(), 10)
	end := strconv.FormatInt(time.Date(2024, 1, 1, 0, 3, 0, 0, time.UTC).UnixNano(), 10)
	trial, e := service.ChannelTrial(ctx, f.actor, scope, channel.Resource.ID, dm.TrialInput{WriteMeta: meta(channel.ConfigRevision), ConfigID: cfg.ID, ConfigHash: cfg.ConfigHash, OutputRange: dm.NanosecondRange{From: start, To: &end}}, access.Meta{})
	must(t, e)
	if trial.Trial == nil {
		t.Fatal(trial)
	}
	preview, e := service.PreviewControl(ctx, f.actor, scope, channel.Resource.ID, dm.ControlPreviewInput{Revision: channel.ControlRevision, ControlProposal: dm.ControlProposal{Action: "enable", AutoStartNS: &start, StopNS: &end, TrialReceiptID: trial.Trial.ReceiptID}})
	must(t, e)
	_, e = service.ControlChannel(ctx, f.actor, scope, channel.Resource.ID, dm.ControlInput{WriteMeta: meta(channel.ControlRevision), PreviewID: preview.ID, PreviewHash: preview.Hash}, access.Meta{})
	must(t, e)
	queued, e := service.ScheduleTenant(ctx, f.tenant)
	must(t, e)
	if queued != 1 {
		t.Fatal("enabled dirty range was not queued", queued)
	}
	queued, e = service.ScheduleTenant(ctx, f.tenant)
	must(t, e)
	if queued != 0 {
		t.Fatal("duplicate system target", queued)
	}
	var job, run string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT job_id::text,id::text FROM derived_runs WHERE tenant_id=$1 AND mode='automatic'`, f.tenant).Scan(&job, &run))
	runView, e := service.Run(ctx, f.actor, scope, run)
	must(t, e)
	if runView.Mode != "automatic" || runView.Job.State != "queued" {
		t.Fatal("formal run hidden from qualified viewer", runView)
	}
	_, e = service.ControlRun(ctx, f.actor, scope, run, "cancel", meta(runView.Job.Revision), access.Meta{})
	must(t, e)
	queued, e = service.ScheduleTenant(ctx, f.tenant)
	must(t, e)
	if queued != 1 {
		t.Fatal("cancelled enabled target lost its pending work", queued)
	}
	// A commit failure after physical sealing must expose no generation. The
	// next fence re-verifies the completed checkpoint rather than replaying its
	// uncertain insert into ClickHouse.
	_, e = f.env.Pool.Exec(ctx, `CREATE FUNCTION fail_derived_publication_test() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected publication commit failure' USING ERRCODE='40001'; END $$; CREATE TRIGGER fail_derived_publication_test BEFORE INSERT ON derived_generations FOR EACH ROW EXECUTE FUNCTION fail_derived_publication_test()`)
	must(t, e)
	_, e = f.service.RunTask(ctx, "formal-test-failure", f.tenant, job)
	if e == nil {
		t.Fatal("injected publication failure was ignored")
	}
	var visible, attempts int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM derived_generations WHERE tenant_id=$1),(SELECT count(*) FROM derived_write_attempts WHERE tenant_id=$1 AND run_id=$2 AND state='sealed')`, f.tenant, run).Scan(&visible, &attempts))
	if visible != 0 || attempts != 1 {
		t.Fatal("failed PG commit exposed or lost sealed payload", visible, attempts)
	}
	var premature *string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT current_root FROM derived_channel_heads WHERE tenant_id=$1 AND channel_id=$2`, f.tenant, channel.Resource.ID).Scan(&premature))
	if premature != nil {
		t.Fatal("uncommitted physical results became current")
	}
	_, e = f.env.Pool.Exec(ctx, `DROP TRIGGER fail_derived_publication_test ON derived_generations; DROP FUNCTION fail_derived_publication_test()`)
	must(t, e)
	_, e = f.env.Pool.Exec(ctx, `UPDATE monitoring_jobs SET available_at=clock_timestamp() WHERE id=$1`, job)
	must(t, e)
	worked, e := f.service.RunTask(ctx, "formal-test", f.tenant, job)
	must(t, e)
	if !worked {
		t.Fatal("formal task not claimed")
	}
	var state, code string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT state,COALESCE(error_code,'') FROM monitoring_jobs WHERE id=$1`, job).Scan(&state, &code))
	if state != "succeeded" {
		t.Fatal("formal task did not publish", state, code)
	}
	var generation, manifest, currentRoot string
	var rows, dirty, sealed, receipts int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT g.id::text,g.manifest_root,g.row_count,h.current_root,(SELECT count(*) FROM derived_dirty_ranges WHERE tenant_id=g.tenant_id AND channel_id=g.channel_id),(SELECT count(*) FROM derived_write_attempts WHERE tenant_id=g.tenant_id AND run_id=g.run_id AND state='sealed'),(SELECT count(*) FROM derived_publication_receipts WHERE tenant_id=g.tenant_id AND run_id=g.run_id) FROM derived_generations g JOIN derived_channel_heads h USING(tenant_id,channel_id) WHERE g.tenant_id=$1 AND g.run_id=$2`, f.tenant, run).Scan(&generation, &manifest, &rows, &currentRoot, &dirty, &sealed, &receipts))
	if rows != 3 || len(manifest) != 64 || len(currentRoot) != 64 || dirty != 0 || sealed != 1 || receipts != 1 {
		t.Fatal("partial publication metadata", rows, dirty, sealed, receipts)
	}
	replicas, e := f.storage.CalculationTransport().Group("local")
	must(t, e)
	for _, replica := range replicas.Replicas {
		var values, statuses []string
		must(t, replica.QueryRow(ctx, "SELECT groupArray(ifNull(toString(coefficient),'null')),groupArray(calculation_status) FROM (SELECT coefficient,calculation_status FROM "+f.database+".derived_results_local WHERE tenant_id=? AND generation_id=? ORDER BY output_at)", uuid.MustParse(f.tenant), uuid.MustParse(generation)).Scan(&values, &statuses))
		if len(values) != 3 || values[0] != "null" || values[1] != "3000000000000000000" || values[2] != "4000000000000000000" || statuses[0] != "missing_input" {
			t.Fatal("published payload differs from hand calculation", values, statuses)
		}
	}
	worked, e = f.service.RunTask(ctx, "formal-test-retry", f.tenant, job)
	must(t, e)
	if worked {
		t.Fatal("successful publication reran its physical attempt")
	}
	var events int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM monitoring_changes WHERE tenant_id=$1 AND change_type='derived_publish'`, f.tenant).Scan(&events))
	if events != 1 {
		t.Fatal("publication event lost idempotency", events)
	}
	_, e = f.env.Pool.Exec(ctx, `UPDATE derived_generations SET row_count=0 WHERE id=$1`, generation)
	if e == nil {
		t.Fatal("formal generation was mutable")
	}
	selection, e := f.service.Select(ctx, f.actor, f.scope, md.SelectionInput{Channels: []string{channel.Resource.ID, f.channel}})
	must(t, e)
	fromText, toText := "2024-01-01T00:00:00Z", "2024-01-01T00:03:00Z"
	query := md.QueryInput{Selection: selection.ID, Range: &md.Range{From: &fromText, To: &toText}, Qualities: []string{"good", "unmarked"}, Limit: 10}
	page, e := f.service.Details(ctx, f.actor, f.scope, query)
	must(t, e)
	derived, direct := 0, 0
	for _, item := range page.Items {
		if item.Calculated == nil {
			direct++
			continue
		}
		derived++
		point := item.Calculated
		if point.Freshness != "current" || point.Generation != generation || point.Scale != 18 {
			t.Fatal("unified result lost formal identity", point)
		}
	}
	if derived != 3 || direct != 3 || len(page.Coverage) != 1 || page.Coverage[0].State != "current" {
		t.Fatal("mixed monitoring details lost source kinds", derived, direct, page.Coverage)
	}
	exportIDs := verifyDerivedExports(t, f, query)
	// Check immediate source revocation with only the output selected. Its raw
	// leaf is deliberately absent from the query's ordinary raw revision vector.
	selection, e = f.service.Select(ctx, f.actor, f.scope, md.SelectionInput{Channels: []string{channel.Resource.ID}})
	must(t, e)
	query.Selection = selection.ID
	derivedExports := verifyDerivedOnlyExports(t, f, query)
	exportIDs = append(exportIDs, derivedExports...)
	pendingDownload, e := f.service.ExportDownload(ctx, f.actor, f.scope, derivedExports[0], "1")
	must(t, e)
	for _, action := range []string{"overview", "latest", "statistics", "trends"} {
		analytic, e := f.service.Analytics(ctx, f.actor, f.scope, action, query)
		must(t, e)
		if analytic.State != "ready" {
			t.Fatal(action, analytic)
		}
		switch action {
		case "overview":
			if analytic.Overview.Derived == nil || analytic.Overview.Derived.LogicalOutputs != "3" || analytic.Overview.Derived.ValidResults != "2" || analytic.Overview.Derived.NoResults != "1" || analytic.Overview.Active != "0" || analytic.Overview.Latest == nil {
				t.Fatal(analytic.Overview)
			}
		case "latest", "statistics":
			if len(analytic.Channels) != 1 {
				t.Fatal(analytic)
			}
			c := analytic.Channels[0]
			if c.SourceKind != "derived" || c.N != "2" || c.Mean == nil || *c.Mean != "3.500000000000" || c.Stddev == nil || *c.Stddev != "0.500000000000" || c.Last.Calculated == nil || c.LatestOutput == nil || *c.Last.Calculated.Value != "4.000000000000000000" {
				t.Fatal(c)
			}
			if action == "latest" && (c.CalculationStatus == nil || c.LastValidOutput == nil || c.InputThrough == nil || c.ComputedThrough == nil || c.ControlState != "enabled") {
				t.Fatal("actual freshness fields missing", c)
			}
		case "trends":
			if len(analytic.Trends) != 1 || len(analytic.Trends[0].Buckets) != 1 || len(analytic.Trends[0].Buckets[0].Points) != 3 || !analytic.Trends[0].Buckets[0].Discontinuous || analytic.Trends[0].Buckets[0].Points[0].Calculated.Value != nil {
				t.Fatal(analytic.Trends)
			}
		}
	}
	calendarQuery := query
	calendarQuery.Period = "day"
	calendar, e := f.service.Analytics(ctx, f.actor, f.scope, "statistics", calendarQuery)
	must(t, e)
	if len(calendar.Groups) != 1 || calendar.Groups[0].N != "2" || !calendar.Groups[0].Partial || calendar.Groups[0].Mean == nil || *calendar.Groups[0].Mean != "3.500000000000" {
		t.Fatal(calendar)
	}
	page, e = f.service.Details(ctx, f.actor, f.scope, query)
	must(t, e)
	query.Snapshot = page.Snapshot.ID
	revoke, e := f.service.PreviewRevokeBatch(ctx, f.actor, f.scope, batch.ID, batch.Revision)
	must(t, e)
	_, e = f.service.RevokeBatch(ctx, f.actor, f.scope, batch.ID, md.RevokeInput{WriteMeta: f.meta(batch.Revision), PreviewHash: revoke.Hash})
	must(t, e)
	var deniedDownload bytes.Buffer
	if e = pendingDownload.Write(ctx, &deniedDownload); e == nil || deniedDownload.Len() != 0 {
		t.Fatal("calculated leaf revocation not rechecked before download", e, deniedDownload.Len())
	}
	for _, id := range exportIDs {
		if _, e = f.service.ExportFiles(ctx, f.actor, f.scope, id); e == nil {
			t.Fatal("revoked calculation export remained downloadable", id)
		}
	}
	pending, e := f.service.Details(ctx, f.actor, f.scope, query)
	must(t, e)
	if len(pending.Items) != 0 || len(pending.Coverage) != 1 || pending.Coverage[0].State != "pending" {
		t.Fatal("revoked leaf remained current before consumer scheduling", pending)
	}
	history, e := service.GenerationDetails(ctx, f.actor, scope, generation, dm.GenerationQuery{Limit: 2})
	must(t, e)
	if len(history.Items) != 2 || !history.HasMore || history.NextCursor == "" || history.Generation.InputState != "revoked" || history.Generation.Rows == nil || *history.Generation.Rows != "3" || history.Items[0].Value != nil || history.Items[1].Value == nil || *history.Items[1].Value != "3.000000000000000000" || history.Items[1].Freshness != "historical" {
		t.Fatal("historical snapshot changed with source revocation", history)
	}
	lastHistory, e := service.GenerationDetails(ctx, f.actor, scope, generation, dm.GenerationQuery{Limit: 2, Cursor: history.NextCursor})
	must(t, e)
	if len(lastHistory.Items) != 1 || lastHistory.HasMore || lastHistory.Items[0].Value == nil || *lastHistory.Items[0].Value != "4.000000000000000000" || lastHistory.Items[0].Result == history.Items[1].Result {
		t.Fatal("historical paging omitted or duplicated a result", lastHistory)
	}
	middle := strconv.FormatInt(time.Date(2024, 1, 1, 0, 1, 0, 0, time.UTC).UnixNano(), 10)
	middleEnd := strconv.FormatInt(time.Date(2024, 1, 1, 0, 2, 0, 0, time.UTC).UnixNano(), 10)
	part := dm.NanosecondRange{From: middle, To: &middleEnd}
	partialHistory, e := service.GenerationDetails(ctx, f.actor, scope, generation, dm.GenerationQuery{Range: &part})
	must(t, e)
	if len(partialHistory.Items) != 1 || !partialHistory.Generation.Partial || partialHistory.Generation.Rows != nil || partialHistory.Generation.States != nil || partialHistory.Generation.Range.From != middle || *partialHistory.Generation.Range.To != middleEnd {
		t.Fatal("partial history exposed unrelated generation totals", partialHistory)
	}
	lineage, e := service.GenerationLineage(ctx, f.actor, scope, generation, dm.GenerationQuery{Range: &part})
	must(t, e)
	if lineage.Config.ID != cfg.ID || lineage.InputSnapshot.ID == "" || len(lineage.InputRanges) != 1 || lineage.InputRanges[0].Channel != f.channel || lineage.InputRanges[0].From == nil || *lineage.InputRanges[0].From > time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).UnixNano() || lineage.Generation.InputState != "revoked" || lineage.ManifestHash != manifest {
		t.Fatal("historical lineage lost actual predecessor or fixed configuration", lineage)
	}
	// Cursor advancement is inseparable from invalidation. Inject a failure at
	// the last write, restart the consumer, and verify the source fact survives.
	_, e = f.env.Pool.Exec(ctx, `CREATE FUNCTION fail_derived_consumer_test() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected cursor failure' USING ERRCODE='40001'; END $$; CREATE TRIGGER fail_derived_consumer_test BEFORE UPDATE ON monitoring_change_consumers FOR EACH ROW EXECUTE FUNCTION fail_derived_consumer_test()`)
	must(t, e)
	_, e = service.ConsumeChanges(ctx, f.tenant, time.Now())
	if e == nil {
		t.Fatal("injected consumer failure was ignored")
	}
	var cursor int64
	must(t, f.env.Pool.QueryRow(ctx, `SELECT COALESCE((SELECT durable_cursor FROM monitoring_change_consumers WHERE tenant_id=$1 AND consumer='derived-invalidation/1'),0),(SELECT count(*) FROM derived_dirty_ranges WHERE tenant_id=$1)`, f.tenant).Scan(&cursor, &dirty))
	if cursor != 0 || dirty != 0 {
		t.Fatal("consumer rollback left partial invalidation", cursor, dirty)
	}
	_, e = f.env.Pool.Exec(ctx, `DROP TRIGGER fail_derived_consumer_test ON monitoring_change_consumers; DROP FUNCTION fail_derived_consumer_test()`)
	must(t, e)
	processed, e := service.ConsumeChanges(ctx, f.tenant, time.Now())
	must(t, e)
	if processed < 4 {
		t.Fatal("consumer lost durable source changes", processed)
	}
	processed, e = service.ConsumeChanges(ctx, f.tenant, time.Now())
	must(t, e)
	if processed != 0 {
		t.Fatal("consumer replayed committed cursor", processed)
	}
	queued, e = service.ScheduleTenant(ctx, f.tenant)
	must(t, e)
	if queued != 1 {
		t.Fatal("revocation failed to schedule replacement", queued)
	}
	var replacementJob string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT j.id::text FROM derived_runs r JOIN monitoring_jobs j ON j.id=r.job_id WHERE r.tenant_id=$1 AND r.mode='automatic' AND j.state='queued'`, f.tenant).Scan(&replacementJob))
	_, e = f.service.RunTask(ctx, "revoke-recompute", f.tenant, replacementJob)
	must(t, e)
	var emptyGeneration string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text,row_count FROM derived_generations WHERE tenant_id=$1 AND run_id=(SELECT id FROM derived_runs WHERE job_id=$2)`, f.tenant, replacementJob).Scan(&emptyGeneration, &rows))
	if rows != 0 || emptyGeneration == generation {
		t.Fatal("revoked result did not publish an empty replacement", rows, emptyGeneration)
	}
	query.Snapshot = ""
	emptyPage, e := f.service.Details(ctx, f.actor, f.scope, query)
	must(t, e)
	if len(emptyPage.Items) != 0 || len(emptyPage.Coverage) != 1 || emptyPage.Coverage[0].State != "current" || emptyPage.Coverage[0].Generation != emptyGeneration {
		t.Fatal("empty generation resurrected old result rows", emptyPage)
	}
	var generations int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM derived_generations WHERE tenant_id=$1 AND channel_id=$2`, f.tenant, channel.Resource.ID).Scan(&generations))
	if generations != 2 {
		t.Fatal("replacement deleted historical generation", generations)
	}
	generationPage, e := service.ChannelGenerations(ctx, f.actor, scope, channel.Resource.ID, dm.GenerationFilter{Range: dm.NanosecondRange{From: start, To: &end}, Limit: 1})
	must(t, e)
	if len(generationPage.Items) != 1 || generationPage.Items[0].ID != emptyGeneration || generationPage.NextCursor == "" {
		t.Fatal("generation list did not retain newest empty replacement", generationPage)
	}
	generationPage, e = service.ChannelGenerations(ctx, f.actor, scope, channel.Resource.ID, dm.GenerationFilter{Range: dm.NanosecondRange{From: start, To: &end}, Limit: 1, Cursor: generationPage.NextCursor})
	must(t, e)
	if len(generationPage.Items) != 1 || generationPage.Items[0].ID != generation || generationPage.Items[0].InputState != "revoked" || generationPage.NextCursor != "" {
		t.Fatal("old generation missing from historical pages", generationPage)
	}
	var beforeReplayRoot string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT current_root FROM derived_channel_heads WHERE tenant_id=$1 AND channel_id=$2`, f.tenant, channel.Resource.ID).Scan(&beforeReplayRoot))
	replayInput := dm.ReplayInput{WriteMeta: meta("")}
	replay, e := service.ReplayGeneration(ctx, f.actor, scope, generation, replayInput, access.Meta{})
	must(t, e)
	if replay.Mode != "historical_replay" || replay.Job.Kind != "derived_replay" || replay.Job.State != "queued" {
		t.Fatal("replay did not use the shared task host", replay)
	}
	repeatedReplay, e := service.ReplayGeneration(ctx, f.actor, scope, generation, replayInput, access.Meta{})
	must(t, e)
	if !repeatedReplay.Replayed || repeatedReplay.ID != replay.ID {
		t.Fatal("replay request duplicated the immutable target", repeatedReplay)
	}
	_, e = f.service.RunTask(ctx, "history-replay", f.tenant, replay.Job.ID)
	must(t, e)
	replay, e = service.Run(ctx, f.actor, scope, replay.ID)
	must(t, e)
	if replay.Job.State != "succeeded" || replay.Replay == nil || !replay.Replay.Matched || replay.Replay.Expected != 3 || replay.Replay.Actual != 3 || replay.Replay.Differences != 0 || replay.Replay.InputState != "revoked" || len(replay.Replay.Examples) != 3 || replay.Replay.Examples[1].Value == nil || *replay.Replay.Examples[1].Value != "3.000000000000000000" || len(replay.Replay.Examples[1].Matches) != 2 {
		t.Fatal("revoked input did not replay its original numeric and matching evidence", replay)
	}
	var afterReplayRoot string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT current_root,(SELECT count(*) FROM derived_generations WHERE tenant_id=$1 AND channel_id=$2) FROM derived_channel_heads WHERE tenant_id=$1 AND channel_id=$2`, f.tenant, channel.Resource.ID).Scan(&afterReplayRoot, &generations))
	if afterReplayRoot != beforeReplayRoot || generations != 2 {
		t.Fatal("historical replay changed formal current or generations", beforeReplayRoot, afterReplayRoot, generations)
	}
	verifyDerivedProjectHistory(t, f, service, channel.Resource.ID, generation, part)
	reconciled, e := dm.NewRecovery(f.storage).Reconcile(ctx, f.env.Pool, f.tenant)
	must(t, e)
	if reconciled.State != "passed" || reconciled.Generations != 2 || reconciled.Rows != 3 || reconciled.CurrentRanges != 1 {
		t.Fatal("formal evidence reconciliation failed", reconciled)
	}
	t.Log("real PG/CH/S3 formal difference publication: 3 rows, both replicas, sealed manifest, current interval root, durable event, and shared-task success committed together")
}
