//go:build monitoringintegration

package resources_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
)

func TestDerivedExternalWindowReplaysRevokedOriginalSnapshot(t *testing.T) {
	f := monitoringSetup(t)
	batch := f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-01T00:00:00Z,10,good\n2024-01-01T00:00:01Z,14,good\n2024-01-01T00:00:02Z,20,good\n2024-01-01T00:00:03Z,26,good\n2024-01-01T00:00:04Z,30,good\n2024-01-01T00:00:05Z,34,good\n")))
	step := strconv.FormatInt(int64(20*time.Second), 10)
	s, channel, cfg := derivedChannelSetup(t, f, func(rule *dm.Rule) {
		rule.Clock = dm.Clock{Kind: "fixed_grid", AnchorNS: "0", StepNS: step}
		rule.Nodes = []dm.Node{{ID: "result", Op: "window_median", Inputs: []string{"input:x"}, Params: dm.NodeParams{Window: &dm.Window{Kind: "fixed_duration", AnchorNS: "0", DurationNS: step}, MaxAgeNS: step, MaxGapNS: step}}}
	})
	s.WithStorage(f.storage)
	must(t, dm.InitializeStorage(ctx, f.storage))
	options := config.DefaultDerived()
	options.WriteRows = 2
	options.ReadBytes = 1 // Force a bounded external input path.
	options.TrialRows = 2
	s.WithOptions(options)
	at := time.Date(2024, 1, 1, 0, 0, 20, 0, time.UTC).UnixNano()
	from, to := strconv.FormatInt(at, 10), strconv.FormatInt(at+1, 10)
	scope := dm.TenantScope(f.tenant)
	trial, e := s.ChannelTrial(ctx, f.actor, scope, channel.Resource.ID, dm.TrialInput{WriteMeta: restoreMeta(channel.ConfigRevision), ConfigID: cfg.ID, ConfigHash: cfg.ConfigHash, OutputRange: dm.NanosecondRange{From: from, To: &to}}, access.Meta{})
	must(t, e)
	_, e = f.service.RunTask(ctx, "external-window-trial", f.tenant, trial.Job.ID)
	must(t, e)
	trial, e = s.Run(ctx, f.actor, scope, trial.ID)
	must(t, e)
	if trial.Trial == nil || len(trial.Trial.Examples) != 1 || trial.Trial.Examples[0].Value == nil || *trial.Trial.Examples[0].Value != "23.000000000000000000" {
		t.Fatal("fixed window external trial disagrees with median", trial)
	}
	preview, e := s.PreviewControl(ctx, f.actor, scope, channel.Resource.ID, dm.ControlPreviewInput{Revision: channel.ControlRevision, ControlProposal: dm.ControlProposal{Action: "enable", AutoStartNS: &from, StopNS: &to, TrialReceiptID: trial.Trial.ReceiptID}})
	must(t, e)
	_, e = s.ControlChannel(ctx, f.actor, scope, channel.Resource.ID, dm.ControlInput{WriteMeta: restoreMeta(channel.ControlRevision), PreviewID: preview.ID, PreviewHash: preview.Hash}, access.Meta{})
	must(t, e)
	_, e = s.ScheduleTenant(ctx, f.tenant)
	must(t, e)
	var job, generation string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT job_id::text FROM derived_runs WHERE tenant_id=$1 AND mode='automatic'`, f.tenant).Scan(&job))
	_, e = f.service.RunTask(ctx, "external-window-formal", f.tenant, job)
	must(t, e)
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM derived_generations WHERE tenant_id=$1`, f.tenant).Scan(&generation))
	revoke, e := f.service.PreviewRevokeBatch(ctx, f.actor, f.scope, batch.ID, batch.Revision)
	must(t, e)
	_, e = f.service.RevokeBatch(ctx, f.actor, f.scope, batch.ID, md.RevokeInput{WriteMeta: f.meta(batch.Revision), PreviewHash: revoke.Hash})
	must(t, e)
	f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-01T00:00:06Z,900,good\n")))
	replay, e := s.ReplayGeneration(ctx, f.actor, scope, generation, dm.ReplayInput{WriteMeta: restoreMeta("")}, access.Meta{})
	must(t, e)
	_, e = f.service.RunTask(ctx, "external-window-replay", f.tenant, replay.Job.ID)
	must(t, e)
	replay, e = s.Run(ctx, f.actor, scope, replay.ID)
	must(t, e)
	if replay.Replay == nil || !replay.Replay.Matched || replay.Replay.Actual != 1 || replay.Replay.Examples[0].Value == nil || *replay.Replay.Examples[0].Value != "23.000000000000000000" {
		t.Fatal("external replay used current input or lost original payload", replay)
	}
	var checkpoints, generations int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM monitoring_job_steps WHERE job_id=$1 AND step_key LIKE 'window-stream:%'),(SELECT count(*) FROM derived_generations WHERE tenant_id=$2)`, replay.Job.ID, f.tenant).Scan(&checkpoints, &generations))
	if checkpoints != 1 || generations != 1 {
		t.Fatal("replay did not use bounded external path or published a new generation", checkpoints, generations)
	}
}
