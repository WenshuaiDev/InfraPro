//go:build monitoringintegration

package resources_test

import (
	"encoding/json"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	sp "github.com/WenshuaiDev/InfraPro/packages/go/platform/spatial"
	"github.com/google/uuid"
)

func TestSpatialRealPendingSummaryAndFixedArtifact(t *testing.T) {
	f := monitoringSetup(t)
	service, ch, cfg := derivedChannelSetup(t, f)
	service.WithStorage(f.storage)
	must(t, dm.InitializeStorage(ctx, f.storage))
	batch := f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-01T00:00:00Z,2,good\n2024-01-01T00:01:00Z,5,good\n2024-01-01T00:02:00Z,9,good\n")))
	meta := func(rev string) dm.WriteMeta {
		return dm.WriteMeta{RequestID: uuid.NewString(), Revision: rev, Reason: "空间待更新与固定成果自验"}
	}
	scope := dm.TenantScope(f.tenant)
	start, end := "1704067200000000000", "1704067380000000000"
	trial, e := service.ChannelTrial(ctx, f.actor, scope, ch.Resource.ID, dm.TrialInput{WriteMeta: meta(ch.ConfigRevision), ConfigID: cfg.ID, ConfigHash: cfg.ConfigHash, OutputRange: dm.NanosecondRange{From: start, To: &end}}, access.Meta{})
	must(t, e)
	preview, e := service.PreviewControl(ctx, f.actor, scope, ch.Resource.ID, dm.ControlPreviewInput{Revision: ch.ControlRevision, ControlProposal: dm.ControlProposal{Action: "enable", AutoStartNS: &start, StopNS: &end, TrialReceiptID: trial.Trial.ReceiptID}})
	must(t, e)
	_, e = service.ControlChannel(ctx, f.actor, scope, ch.Resource.ID, dm.ControlInput{WriteMeta: meta(ch.ControlRevision), PreviewID: preview.ID, PreviewHash: preview.Hash}, access.Meta{})
	must(t, e)
	_, e = service.ScheduleTenant(ctx, f.tenant)
	must(t, e)
	var job string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT job_id::text FROM derived_runs WHERE tenant_id=$1 AND mode='automatic'`, f.tenant).Scan(&job))
	_, e = f.service.RunTask(ctx, "spatial-summary", f.tenant, job)
	must(t, e)
	var point string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT point_id::text FROM engineering_channels WHERE id=$1`, ch.Resource.ID).Scan(&point))
	business := runtime.Compose(f.env.Service.Access(), f.storage)
	owner := sp.ObjectRef{Kind: "point", ID: point}
	sscope := sp.Tenant(f.tenant)
	read := func() map[string]any {
		t.Helper()
		raw, e := business.Spatial.Summaries(ctx, f.actor, sscope, sp.ObjectQuery{Objects: []sp.ObjectRef{owner}, MainChannels: map[string]string{"point:" + point: ch.Resource.ID}})
		must(t, e)
		var out struct {
			Items []struct {
				Channels []struct {
					Channel struct {
						ID string `json:"id"`
					} `json:"channel"`
					Latest struct {
						Channels []map[string]any `json:"channels"`
					} `json:"latest"`
				} `json:"channels"`
			} `json:"items"`
		}
		must(t, json.Unmarshal(raw, &out))
		if len(out.Items) != 1 || len(out.Items[0].Channels) == 0 || out.Items[0].Channels[0].Channel.ID != ch.Resource.ID {
			t.Fatal("preferred direct owner lost", string(raw))
		}
		return out.Items[0].Channels[0].Latest.Channels[0]
	}
	before := read()
	if before["latest_output"].(map[string]any)["value"] != "4.000000000000000000" {
		t.Fatal(before)
	}
	created, e := service.CreateAnalysis(ctx, f.actor, scope, dm.CreateAnalysisInput{WriteMeta: meta(""), Code: "SPATIAL-FIXED", Name: "空间固定输入成果", Body: dm.AnalysisBody{Rule: cfg.Rule, InputBindings: cfg.InputBindings, Parameters: cfg.Parameters}}, access.Meta{})
	must(t, e)
	valid, e := service.ValidateAnalysis(ctx, f.actor, scope, created.Config.ID, dm.ValidateInput{DraftRevision: created.Config.Draft.Revision})
	must(t, e)
	published, e := service.PublishAnalysis(ctx, f.actor, scope, created.Config.ID, dm.PublishAnalysisInput{PublishMetricInput: dm.PublishMetricInput{WriteMeta: meta(created.Config.Revision), DraftRevision: created.Config.Draft.Revision, ValidationHash: valid.ValidationHash}}, access.Meta{})
	must(t, e)
	run, e := service.StartAnalysis(ctx, f.actor, scope, dm.AnalysisRunInput{WriteMeta: meta(""), ConfigVersionID: published.Version.ID, ConfigHash: published.Version.ConfigHash, OutputRange: dm.NanosecondRange{From: start, To: &end}}, false, access.Meta{})
	must(t, e)
	_, e = f.service.RunTask(ctx, "spatial-fixed", f.tenant, run.Job.ID)
	must(t, e)
	related := func() dm.AnalysisArtifactPage {
		t.Helper()
		raw, e := business.Spatial.RelatedArtifacts(ctx, f.actor, sscope, sp.RelatedInput{Owner: owner, Limit: 20})
		must(t, e)
		var page dm.AnalysisArtifactPage
		must(t, json.Unmarshal(raw, &page))
		if len(page.Items) != 1 {
			t.Fatal(string(raw))
		}
		return page
	}
	prior := related()
	revoke, e := f.service.PreviewRevokeBatch(ctx, f.actor, f.scope, batch.ID, batch.Revision)
	must(t, e)
	_, e = f.service.RevokeBatch(ctx, f.actor, f.scope, batch.ID, md.RevokeInput{WriteMeta: f.meta(batch.Revision), PreviewHash: revoke.Hash})
	must(t, e)
	pending := read()
	if pending["latest_output"] != nil {
		t.Fatal("invalid current output survived leaf revocation", pending)
	}
	ranges, ok := pending["pending_ranges"].([]any)
	if !ok || len(ranges) == 0 {
		t.Fatal("pending state missing", pending)
	}
	after := related()
	if after.Items[0].Version.ID != prior.Items[0].Version.ID || after.Items[0].Version.InputState != "revoked" {
		t.Fatal("fixed result identity/state lost", after)
	}
}
