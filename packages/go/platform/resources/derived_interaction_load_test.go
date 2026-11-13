//go:build monitoringintegration && derivedload

package resources_test

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The query delay is injected only to keep a real PostgreSQL operation active
// while the shared worker receives the real cancel command. Numeric loop
// cancellation is separately exercised in the calculator's bounded-sort tests.
type interactionCancellationExecutor struct {
	*dm.Service
	pool  *pgxpool.Pool
	query string
}

func (x *interactionCancellationExecutor) ExecuteTask(ctx context.Context, _ md.TaskLease) error {
	_, e := x.pool.Exec(ctx, x.query)
	return e
}

func interactionP95(v []float64) float64 {
	c := slices.Clone(v)
	slices.Sort(c)
	return c[(len(c)*95+99)/100-1]
}

func TestDerivedInteractionTargets(t *testing.T) {
	f := monitoringSetup(t, func(c *md.StorageConfig) { c.HotDays = 3650 })
	s, _, template := derivedChannelSetup(t, f)
	s.WithStorage(f.storage)
	must(t, dm.InitializeStorage(ctx, f.storage))
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	var csv strings.Builder
	csv.WriteString("observed_at,value,quality\n")
	for i := 0; i < 120; i++ {
		fmt.Fprintf(&csv, "%s,%d,good\n", base.Add(time.Duration(i)*time.Second).Format(time.RFC3339Nano), i)
	}
	f.publish(t, f.validate(t, f.upload(t, csv.String())))
	from, to := strconv.FormatInt(base.UnixNano(), 10), strconv.FormatInt(base.Add(2*time.Minute).UnixNano(), 10)
	interval := dm.NanosecondRange{From: from, To: &to}
	scope := dm.TenantScope(f.tenant)
	channels := []dm.Channel{}
	trialSeconds, submitSeconds := []float64{}, []float64{}
	for _, op := range []string{"difference", "rate", "mean", "variance", "median", "quantile", "sum", "min", "max", "count"} {
		var rule dm.Rule
		must(t, json.Unmarshal(template.Rule, &rule))
		if op == "difference" || op == "rate" {
			rule.Nodes[0].Op = op
			if op == "rate" {
				rule.Nodes[0].Params.RateTimeUnit = "second"
			}
		} else {
			p := dm.NodeParams{Window: &dm.Window{Kind: "trailing_duration", DurationNS: "60000000000"}, MinSamples: "1", MaxGapNS: "120000000000", MaxAgeNS: "120000000000"}
			if op == "variance" {
				p.MinSamples = "2"
			}
			if op == "count" {
				p.MinSamples = "0"
			}
			if op == "quantile" {
				p.Percentile = "95"
			}
			rule.Nodes = []dm.Node{{ID: "result", Op: "window_" + op, Inputs: []string{"input:x"}, Params: p}}
		}
		rule.Outputs[0].SemanticKey, rule.Outputs[0].Quantity = "interaction_"+op, nil
		if op == "count" {
			rule.Outputs[0].Numeric.Scale = 0
		}
		channel, config := createRuleChannel(t, f, s, "INTERACTION_"+strings.ToUpper(op), rule, map[string]dm.InputBinding{"x": {ChannelID: f.channel}})
		var trial dm.Run
		for i := 0; i < 2; i++ {
			start := time.Now()
			var e error
			trial, e = s.ChannelTrial(ctx, f.actor, scope, channel.Resource.ID, dm.TrialInput{WriteMeta: restoreMeta(channel.ConfigRevision), ConfigID: config.ID, ConfigHash: config.ConfigHash, OutputRange: interval}, access.Meta{})
			must(t, e)
			trialSeconds = append(trialSeconds, time.Since(start).Seconds())
			if trial.Job.State != "succeeded" || trial.Trial == nil || trial.Trial.OutputCount != "120" {
				t.Fatal("ready small trial did not finish its complete range", op, trial)
			}
		}
		preview, e := s.PreviewControl(ctx, f.actor, scope, channel.Resource.ID, dm.ControlPreviewInput{Revision: channel.ControlRevision, ControlProposal: dm.ControlProposal{Action: "enable", AutoStartNS: &from, StopNS: &to, TrialReceiptID: trial.Trial.ReceiptID}})
		must(t, e)
		_, e = s.ControlChannel(ctx, f.actor, scope, channel.Resource.ID, dm.ControlInput{WriteMeta: restoreMeta(channel.ControlRevision), PreviewID: preview.ID, PreviewHash: preview.Hash}, access.Meta{})
		must(t, e)
		_, e = s.ScheduleTenant(ctx, f.tenant)
		must(t, e)
		var job string
		must(t, f.env.Pool.QueryRow(ctx, `SELECT job_id::text FROM derived_runs WHERE tenant_id=$1 AND channel_id=$2 AND mode='automatic'`, f.tenant, channel.Resource.ID).Scan(&job))
		_, e = f.service.RunTask(ctx, "interaction-formal", f.tenant, job)
		must(t, e)
		channels = append(channels, channel)
	}
	ids := []string{}
	for _, c := range channels {
		ids = append(ids, c.Resource.ID)
	}
	selection, e := f.service.Select(ctx, f.actor, f.scope, md.SelectionInput{Channels: ids})
	must(t, e)
	lo, hi := base.Format(time.RFC3339Nano), base.Add(2*time.Minute).Format(time.RFC3339Nano)
	q := md.QueryInput{Selection: selection.ID, Range: &md.Range{From: &lo, To: &hi}, Qualities: []string{"good", "unmarked"}, Limit: 200}
	measure := func(action string) float64 {
		t.Helper()
		start := time.Now()
		result, e := f.service.Analytics(ctx, f.actor, f.scope, action, q)
		must(t, e)
		elapsed := time.Since(start).Seconds()
		if result.State != "ready" {
			t.Fatal("ready query unexpectedly deferred", action, result.State)
		}
		if action == "trends" {
			if len(result.Trends) != 10 {
				t.Fatal("ten curves lost a selected channel", len(result.Trends))
			}
			for _, trend := range result.Trends {
				rows := 0
				for _, bucket := range trend.Buckets {
					rows += len(bucket.Points)
				}
				if rows != 120 || trend.Summary {
					t.Fatal("bounded exact curve lost rows", trend.Channel, rows, trend.Summary)
				}
			}
		} else if result.Overview == nil || result.Overview.Derived == nil || result.Overview.Derived.LogicalOutputs != "1200" {
			t.Fatal("overview lost separate logical output count", result.Overview)
		}
		return elapsed
	}
	trends, overview := []float64{}, []float64{}
	for i := 0; i < 20; i++ {
		trends = append(trends, measure("trends"))
		overview = append(overview, measure("overview"))
	}
	group, e := f.storage.CalculationTransport().Group("local")
	must(t, e)
	for _, replica := range group.Replicas {
		must(t, replica.Exec(ctx, "SYSTEM STOP MERGES "+f.database+".derived_results_local"))
		defer replica.Exec(context.Background(), "SYSTEM START MERGES "+f.database+".derived_results_local")
		var local uint64
		must(t, replica.QueryRow(ctx, `SELECT count() FROM system.parts WHERE database=? AND table='derived_results_local' AND active AND disk_name='default'`, f.database).Scan(&local))
		if local > 0 {
			must(t, replica.Exec(ctx, "ALTER TABLE "+f.database+".derived_results_local MOVE PARTITION 202401 TO VOLUME 'online'"))
		}
		var remote uint64
		// monitoring_tiered exposes the S3 disk through monitoring_cache;
		// system.parts reports that wrapper, not its contract_s3 backend.
		must(t, replica.QueryRow(ctx, `SELECT count() FROM system.parts WHERE database=? AND table='derived_results_local' AND active AND disk_name='monitoring_cache'`, f.database).Scan(&remote))
		if remote == 0 {
			t.Fatal("cold read was not backed by remote ClickHouse parts")
		}
	}
	cold := measure("trends")
	var cancelRun dm.Run
	for _, c := range channels {
		preview, e := s.PreviewBackfill(ctx, f.actor, scope, c.Resource.ID, dm.BackfillPreviewInput{Revision: c.ConfigRevision, Range: interval})
		must(t, e)
		start := time.Now()
		backfill, e := s.BackfillChannel(ctx, f.actor, scope, c.Resource.ID, dm.ControlInput{WriteMeta: restoreMeta(c.ConfigRevision), PreviewID: preview.ID, PreviewHash: preview.Hash}, access.Meta{})
		must(t, e)
		submitSeconds = append(submitSeconds, time.Since(start).Seconds())
		if len(backfill.Runs) != 1 {
			t.Fatal("unexpected backfill subdivision", len(backfill.Runs))
		}
		cancelRun = backfill.Runs[0]
		if c.Resource.ID != channels[len(channels)-1].Resource.ID {
			_, e = s.ControlRun(ctx, f.actor, scope, cancelRun.ID, "cancel", restoreMeta(cancelRun.Job.Revision), access.Meta{})
			must(t, e)
		}
	}
	query := "SELECT pg_sleep(30) /* derived_interaction_cancel_" + f.tenant + " */"
	_, e = f.service.WithTaskExecutors(map[string]md.TaskExecutor{"derive": &interactionCancellationExecutor{Service: s, pool: f.env.Pool, query: query}})
	must(t, e)
	work, stop := context.WithCancel(t.Context())
	defer stop()
	done := make(chan error, 1)
	go func() {
		_, err := f.service.RunTask(work, "interaction-cancel", f.tenant, cancelRun.Job.ID)
		done <- err
	}()
	deadline := time.Now().Add(10 * time.Second)
	for {
		var active bool
		must(t, f.env.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE query=$1 AND wait_event='PgSleep')`, query).Scan(&active))
		if active {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("controlled query did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancelRun, e = s.Run(ctx, f.actor, scope, cancelRun.ID)
	must(t, e)
	start := time.Now()
	_, e = s.ControlRun(ctx, f.actor, scope, cancelRun.ID, "cancel", restoreMeta(cancelRun.Job.Revision), access.Meta{})
	must(t, e)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cancel did not reach active database query within two seconds")
	}
	cancelSeconds := time.Since(start).Seconds()
	cancelRun, e = s.Run(ctx, f.actor, scope, cancelRun.ID)
	must(t, e)
	if cancelRun.Job.State != "cancelled" {
		t.Fatal("cancelled query lost final task state", cancelRun.Job)
	}
	report := map[string]any{"channels": 10, "input_rows": 120, "logical_outputs": 1200, "trial_samples": len(trialSeconds), "trial_p95_seconds": interactionP95(trialSeconds), "trends_samples": len(trends), "trends_p95_seconds": interactionP95(trends), "overview_p95_seconds": interactionP95(overview), "remote_first_read_seconds": cold, "submit_samples": len(submitSeconds), "submit_max_seconds": slices.Max(submitSeconds), "cancel_database_query_seconds": cancelSeconds, "cache": "new business fixture; remote parts verified; host and object service caches retained"}
	encoded, e := json.Marshal(report)
	must(t, e)
	t.Logf("DP0_INTERACTIONS %s", encoded)
	if interactionP95(trialSeconds) > 3 || interactionP95(trends) > 2 || interactionP95(overview) > 2 || cold > 10 || slices.Max(submitSeconds) > 1 || cancelSeconds > 2 {
		t.Fatal("frozen interaction target failed")
	}
}
