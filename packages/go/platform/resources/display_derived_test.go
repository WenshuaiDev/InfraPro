//go:build monitoringintegration

package resources_test

import (
	"bytes"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/display"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/google/uuid"
	"testing"
)

func TestDisplayDerivedFixedArtifactAndRecursiveRevocation(t *testing.T) {
	f := monitoringSetup(t)
	service, ch, cfg := derivedChannelSetup(t, f)
	service.WithStorage(f.storage)
	must(t, dm.InitializeStorage(ctx, f.storage))
	batch := f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-01T00:00:00Z,2,good\n2024-01-01T00:01:00Z,5,good\n2024-01-01T00:02:00Z,9,good\n")))
	meta := func(rev string) dm.WriteMeta {
		return dm.WriteMeta{RequestID: uuid.NewString(), Revision: rev, Reason: "display derived contract"}
	}
	tenantScope := dm.TenantScope(f.tenant)
	from, to := "1704067200000000000", "1704067380000000000"
	trial, e := service.ChannelTrial(ctx, f.actor, tenantScope, ch.Resource.ID, dm.TrialInput{WriteMeta: meta(ch.ConfigRevision), ConfigID: cfg.ID, ConfigHash: cfg.ConfigHash, OutputRange: dm.NanosecondRange{From: from, To: &to}}, access.Meta{})
	must(t, e)
	// Under load the owning module legitimately hands interactive work to its
	// durable queue. The SDK fixture must complete that job before using proof.
	if trial.Trial == nil {
		_, e = f.service.RunTask(ctx, "display-trial-background", f.tenant, trial.Job.ID)
		must(t, e)
		trial, e = service.Run(ctx, f.actor, tenantScope, trial.ID)
		must(t, e)
	}
	if trial.Trial == nil {
		t.Fatalf("trial proof missing after background work: %s", trial.Job.State)
	}
	prep, e := service.PreviewControl(ctx, f.actor, tenantScope, ch.Resource.ID, dm.ControlPreviewInput{Revision: ch.ControlRevision, ControlProposal: dm.ControlProposal{Action: "enable", AutoStartNS: &from, StopNS: &to, TrialReceiptID: trial.Trial.ReceiptID}})
	must(t, e)
	_, e = service.ControlChannel(ctx, f.actor, tenantScope, ch.Resource.ID, dm.ControlInput{WriteMeta: meta(ch.ControlRevision), PreviewID: prep.ID, PreviewHash: prep.Hash}, access.Meta{})
	must(t, e)
	_, e = service.ScheduleTenant(ctx, f.tenant)
	must(t, e)
	var job string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT job_id::text FROM derived_runs WHERE tenant_id=$1 AND mode='automatic'`, f.tenant).Scan(&job))
	_, e = f.service.RunTask(ctx, "display-derived", f.tenant, job)
	must(t, e)
	p := f.project(t, "DisplayDerived")
	var facility string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT facility_id::text FROM engineering_channels WHERE id=$1`, f.channel).Scan(&facility))
	g := f.accessInput(t, p, facility, "create_link")
	g.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}}
	for _, id := range []string{f.channel, ch.Resource.ID} {
		id := id
		g.Add = append(g.Add, res.GrantInput{Kind: res.Channel, ChannelID: &id, Capabilities: []string{"read"}})
	}
	f.grant(t, p, facility, g)
	rev := "0"
	for _, id := range []string{f.channel, ch.Resource.ID} {
		var rg string
		must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM project_resource_grants WHERE project_id=$1 AND channel_id=$2 AND ended_at IS NULL`, p.id, id).Scan(&rg))
		r, e := f.service.GrantData(ctx, f.actor, f.tenant, p.id, md.GrantInput{WriteMeta: f.meta(rev), ChannelID: id, ResourceGrantID: rg, Range: &md.Range{}, Capabilities: []string{"read"}})
		must(t, e)
		rev = r.AccessRevision
	}
	scope := dm.ProjectScope(f.tenant, p.id)
	created, e := service.CreateAnalysis(ctx, p.actor, scope, dm.CreateAnalysisInput{WriteMeta: meta(""), Code: "DISPLAY-FIXED", Name: "固定分析成果", Body: dm.AnalysisBody{Rule: cfg.Rule, InputBindings: cfg.InputBindings, Parameters: cfg.Parameters}}, access.Meta{})
	must(t, e)
	valid, e := service.ValidateAnalysis(ctx, p.actor, scope, created.Config.ID, dm.ValidateInput{DraftRevision: created.Config.Draft.Revision})
	must(t, e)
	published, e := service.PublishAnalysis(ctx, p.actor, scope, created.Config.ID, dm.PublishAnalysisInput{PublishMetricInput: dm.PublishMetricInput{WriteMeta: meta(created.Config.Revision), DraftRevision: created.Config.Draft.Revision, ValidationHash: valid.ValidationHash}}, access.Meta{})
	must(t, e)
	run, e := service.StartAnalysis(ctx, p.actor, scope, dm.AnalysisRunInput{WriteMeta: meta(""), ConfigVersionID: published.Version.ID, ConfigHash: published.Version.ConfigHash, OutputRange: dm.NanosecondRange{From: from, To: &to}}, false, access.Meta{})
	must(t, e)
	_, e = f.service.RunTask(ctx, "display-artifact", f.tenant, run.Job.ID)
	must(t, e)
	b := runtime.Compose(f.env.Service.Access(), f.storage)
	h := createDisplayHarness(t, f.resourceFixture, p, b.Displays, map[string]display.Binding{"derived": {Kind: "channels", IDs: []string{ch.Resource.ID}}, "fixed": {Kind: "artifacts", IDs: []string{run.Analysis.ArtifactVersionID}}}, display.Grant{Capabilities: []string{"monitoring.latest", "artifacts.read"}, Scope: display.ReadScope{Channels: []string{f.channel, ch.Resource.ID}, Artifacts: []string{run.Analysis.ArtifactVersionID}}})
	latest := h.query(t, "monitoring.latest", display.Query{Target: display.Target{Binding: "derived"}})
	raw, _ := json.Marshal(latest)
	if !bytes.Contains(raw, []byte("4.000000000000000000")) {
		t.Fatal("derived value missing", string(raw))
	}
	for _, method := range []string{"artifacts.list", "artifacts.get", "artifacts.read"} {
		result := h.query(t, method, display.Query{Target: display.Target{Binding: "fixed"}})
		raw, _ := json.Marshal(result)
		if !bytes.Contains(raw, []byte(run.Analysis.ArtifactVersionID)) {
			t.Fatal(method, string(raw))
		}
	}
	before, e := f.service.PreviewRevokeBatch(ctx, f.actor, f.scope, batch.ID, batch.Revision)
	must(t, e)
	_, e = f.service.RevokeBatch(ctx, f.actor, f.scope, batch.ID, md.RevokeInput{WriteMeta: f.meta(batch.Revision), PreviewHash: before.Hash})
	must(t, e)
	after := h.query(t, "monitoring.latest", display.Query{Target: display.Target{Binding: "derived"}})
	raw, _ = json.Marshal(after)
	if !bytes.Contains(raw, []byte("pending_ranges")) {
		t.Fatal("pending status lost", string(raw))
	}
	q, _ := json.Marshal(display.Query{Target: display.Target{Binding: "fixed"}})
	if _, e = h.s.Query(ctx, h.session, display.Call{Method: "artifacts.read", Params: q}); e == nil {
		t.Fatal("revoked fixed input leaked through subject")
	}
	if _, e = h.s.Prepare(ctx, p.actor, h.scope, h.id, display.Command{Meta: display.Meta{Revision: h.revision}, ConfigID: h.config}); e == nil {
		t.Fatal("prepare accepted revoked artifact inputs")
	}

}
