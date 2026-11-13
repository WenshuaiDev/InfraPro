//go:build monitoringintegration

package resources_test

import (
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
)

func TestDerivedPrimaryClockAdvancesPublishedPrefixBeforeUncomputedTail(t *testing.T) {
	f := monitoringSetup(t)
	s, parent, parentConfig := derivedChannelSetup(t, f, func(r *dm.Rule) {
		r.Nodes = []dm.Node{{ID: "result", Op: "formula", Params: dm.NodeParams{Expression: "x * 2", Variables: map[string]string{"x": "input:x"}}}}
		r.Outputs[0].SemanticKey = "double_value"
	})
	s.WithStorage(f.storage)
	must(t, dm.InitializeStorage(ctx, f.storage))
	scope := dm.TenantScope(f.tenant)
	base := time.Now().UTC().Add(-time.Minute)
	from, trialTo := strconv.FormatInt(base.UnixNano(), 10), strconv.FormatInt(base.Add(time.Second).UnixNano(), 10)
	f.publish(t, f.validate(t, f.upload(t, fmt.Sprintf("observed_at,value,quality\n%s,2,good\n", base.Format(time.RFC3339Nano)))))
	enable := func(channel dm.Channel, cfg dm.ChannelConfig) {
		t.Helper()
		trial, e := s.ChannelTrial(ctx, f.actor, scope, channel.Resource.ID, dm.TrialInput{WriteMeta: restoreMeta(channel.ConfigRevision), ConfigID: cfg.ID, ConfigHash: cfg.ConfigHash, OutputRange: dm.NanosecondRange{From: from, To: &trialTo}}, access.Meta{})
		must(t, e)
		if trial.Trial == nil {
			t.Fatal("expected completed bounded trial", trial.Job)
		}
		preview, e := s.PreviewControl(ctx, f.actor, scope, channel.Resource.ID, dm.ControlPreviewInput{Revision: channel.ControlRevision, ControlProposal: dm.ControlProposal{Action: "enable", AutoStartNS: &from, TrialReceiptID: trial.Trial.ReceiptID}})
		must(t, e)
		_, e = s.ControlChannel(ctx, f.actor, scope, channel.Resource.ID, dm.ControlInput{WriteMeta: restoreMeta(channel.ControlRevision), PreviewID: preview.ID, PreviewHash: preview.Hash}, access.Meta{})
		must(t, e)
	}
	drain := func() {
		t.Helper()
		for i := 0; i < 12; i++ {
			_, e := s.ConsumeChanges(ctx, f.tenant, time.Now())
			must(t, e)
			n, e := s.ScheduleTenant(ctx, f.tenant)
			must(t, e)
			if n == 0 {
				return
			}
			rows, e := f.env.Pool.Query(ctx, `SELECT j.id::text FROM derived_runs r JOIN monitoring_jobs j ON j.id=r.job_id WHERE r.tenant_id=$1 AND r.mode='automatic' AND j.state='queued' ORDER BY j.created_at`, f.tenant)
			must(t, e)
			jobs := []string{}
			for rows.Next() {
				var id string
				must(t, rows.Scan(&id))
				jobs = append(jobs, id)
			}
			must(t, rows.Err())
			rows.Close()
			for _, id := range jobs {
				_, e = f.service.RunTask(ctx, "clock-horizon-regression", f.tenant, id)
				must(t, e)
			}
		}
		t.Fatal("automatic targets did not converge")
	}
	enable(parent, parentConfig)
	drain()
	var horizon int64
	must(t, f.env.Pool.QueryRow(ctx, `SELECT max(upper(output_range)) FROM derived_generations WHERE tenant_id=$1 AND channel_id=$2`, f.tenant, parent.Resource.ID).Scan(&horizon))
	var rule dm.Rule
	must(t, json.Unmarshal(parentConfig.Rule, &rule))
	rule.Inputs[0].SourceKinds, rule.Inputs[0].MetricVersionID = []string{"derived"}, ""
	child, cfg := createRuleChannel(t, f, s, "CHILD_CLOCK", rule, map[string]dm.InputBinding{"x": {ChannelID: parent.Resource.ID}})
	enable(child, cfg)
	drain()
	var firstTo int64
	must(t, f.env.Pool.QueryRow(ctx, `SELECT upper(output_range) FROM derived_generations WHERE tenant_id=$1 AND channel_id=$2 ORDER BY created_at LIMIT 1`, f.tenant, child.Resource.ID).Scan(&firstTo))
	if firstTo != horizon {
		t.Fatal("downstream target did not stop at its published primary clock", firstTo, horizon)
	}
	var tail bool
	must(t, f.env.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM derived_dirty_ranges WHERE tenant_id=$1 AND channel_id=$2 AND lower(affected)=$3 AND upper(affected)>$3)`, f.tenant, child.Resource.ID, horizon).Scan(&tail))
	if !tail {
		t.Fatal("uncomputed activation tail was cleared")
	}
	next := time.Now().UTC()
	f.publish(t, f.validate(t, f.upload(t, fmt.Sprintf("observed_at,value,quality\n%s,7,good\n", next.Format(time.RFC3339Nano)))))
	drain()
	selection, e := f.service.Select(ctx, f.actor, f.scope, md.SelectionInput{Channels: []string{child.Resource.ID}})
	must(t, e)
	lo, hi := base.Format(time.RFC3339Nano), next.Add(time.Nanosecond).Format(time.RFC3339Nano)
	page, e := f.service.Details(ctx, f.actor, f.scope, md.QueryInput{Selection: selection.ID, Range: &md.Range{From: &lo, To: &hi}, Limit: 10})
	must(t, e)
	if len(page.Items) != 2 || page.Items[0].Calculated == nil || page.Items[0].Calculated.Value == nil || *page.Items[0].Calculated.Value != "28.000000000000000000" {
		t.Fatal("newly published upstream did not advance the same downstream clock", page)
	}
}
