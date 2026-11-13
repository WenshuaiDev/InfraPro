package derivedmonitoring_test

import (
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/projects"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tenancy"
	"github.com/google/uuid"
)

func TestDerivedProjectDelegationHasNoTenantFallbackOrDataGrant(t *testing.T) {
	f := channelsSetup(t)
	actor := f.actor(f.admin)
	v := f.publish(t, f.create(t, "shared").Definition)
	tenant, e := tenancy.New(f.e.Service.Access()).Context(ctx, actor, f.scope.Tenant)
	must(t, e)
	ps := projects.New(f.e.Service.Access())
	p, e := ps.Create(ctx, actor, f.scope.Tenant, projects.CreateInput{Fields: projects.Fields{Name: "派生项目", Code: "calculation", Purpose: "明确计算委托"}, WriteMeta: projects.WriteMeta{RequestID: uuid.NewString(), Reason: "权限验证"}, Owner: tenant.MembershipID, Administrator: tenant.MembershipID}, access.Meta{})
	must(t, e)
	scope := dm.ProjectScope(f.scope.Tenant, p.ProjectID)
	detail := func() resources.AccessDetail {
		t.Helper()
		d, e := f.r.AccessDetail(ctx, actor, f.scope.Tenant, p.ProjectID, f.facility, resources.AccessFilter{})
		must(t, e)
		return d
	}
	grant := func(in resources.AccessInput) {
		t.Helper()
		d := detail()
		in.RequestID = uuid.NewString()
		in.Reason = "计算委托验证"
		in.AccessRevision = d.Policy.Revision
		if d.Link != nil {
			in.LinkRevision = d.Link.Revision
		}
		preview, e := f.r.PreviewAccessChange(ctx, actor, f.scope.Tenant, p.ProjectID, f.facility, in)
		must(t, e)
		in.PreviewHash = preview.Hash
		_, e = f.r.ChangeAccess(ctx, actor, f.scope.Tenant, p.ProjectID, f.facility, in, access.Meta{})
		must(t, e)
	}
	grant(resources.AccessInput{Action: "create_link", Add: []resources.GrantInput{{Kind: resources.Facility, Capabilities: []string{"read"}}, {Kind: resources.Point, PointID: &f.point, Capabilities: []string{"read", "create_channel"}}, {Kind: resources.Channel, ChannelID: &f.direct, Capabilities: []string{"read"}}}})
	in := dm.CreateChannelInput{WriteMeta: wm(""), Code: "SHARED", Name: "项目建档", Owner: dm.Owner{Kind: resources.Point, ID: f.point, FacilityID: f.facility}, DefinitionVersionID: v.Version.ID, InputBindings: map[string]dm.InputBinding{"x": {ChannelID: f.direct}}, Parameters: map[string]string{}}
	_, e = f.s.CreateChannel(ctx, actor, scope, in, access.Meta{})
	code(t, e, "permission_denied")
	var parentGrant, inputGrant resources.Grant
	for _, g := range detail().Grants {
		if g.ResourceID == f.point {
			parentGrant = g
		}
		if g.ResourceID == f.direct {
			inputGrant = g
		}
	}
	grant(resources.AccessInput{Action: "change_grants", Update: []resources.GrantUpdate{{ID: parentGrant.ID, Revision: parentGrant.Revision, Capabilities: []string{"read", "create_channel", "create_derived_channel"}}}})
	created, e := f.s.CreateChannel(ctx, actor, scope, in, access.Meta{})
	must(t, e)
	if created.Channel.Resource.CreatedBy != "" || created.Channel.Resource.UpdatedBy != "" {
		t.Fatal("project response revealed account metadata")
	}
	var grants int
	var calculation, source bool
	must(t, f.e.Pool.QueryRow(ctx, `SELECT count(*) FROM monitoring_data_grants WHERE tenant_id=$1 AND project_id=$2 AND channel_id=$3`, f.scope.Tenant, p.ProjectID, created.Channel.Resource.ID).Scan(&grants))
	if grants != 0 {
		t.Fatal("calculation creation granted data access")
	}
	must(t, f.e.Pool.QueryRow(ctx, `SELECT can_manage_calculation,can_source_manage FROM project_resource_grants WHERE tenant_id=$1 AND project_id=$2 AND channel_id=$3 AND ended_at IS NULL`, f.scope.Tenant, p.ProjectID, created.Channel.Resource.ID).Scan(&calculation, &source))
	if !calculation || source {
		t.Fatal("creation conflated calculation and device source grants")
	}
	monitor := monitoring.New(f.e.Service.Access())
	reader := monitor.RawReader()
	readScope := monitoring.ProjectScope(scope.Tenant, scope.Project)
	lo, hi := int64(100), int64(200)
	need := monitoring.RawFreezeRequest{Needs: []monitoring.RawNeed{{Channel: f.direct, From: &lo, To: &hi}}}
	_, e = reader.FreezeUser(ctx, actor, readScope, need)
	monitoringCode(t, e, "monitoring_not_found")
	from, to := time.Unix(0, lo).UTC().Format(time.RFC3339Nano), time.Unix(0, hi).UTC().Format(time.RFC3339Nano)
	_, e = monitor.GrantData(ctx, actor, scope.Tenant, scope.Project, monitoring.GrantInput{WriteMeta: monitoring.WriteMeta{RequestID: uuid.NewString(), ExpectedRevision: "0", Reason: "试算完整输入授权"}, ChannelID: f.direct, ResourceGrantID: inputGrant.ID, Range: &monitoring.Range{From: &from, To: &to}, Capabilities: []string{"read"}})
	must(t, e)
	rawSnapshot, e := reader.FreezeUser(ctx, actor, readScope, need)
	must(t, e)
	lo = 99 // The one-nanosecond halo cannot be silently clipped to the grant.
	_, e = reader.FreezeUser(ctx, actor, readScope, need)
	monitoringCode(t, e, "monitoring_not_found")
	lo = 100
	_, e = reader.CheckUser(ctx, actor, readScope, rawSnapshot.ID, true)
	if e == nil {
		t.Fatal("read grant became historical replay authority")
	}
	// A finite data grant may support a finite calculation, but cannot become
	// an unlimited tenant mandate through the project maintenance delegation.
	past, edge := "1900-01-01T00:00:00Z", from
	_, e = monitor.GrantData(ctx, actor, scope.Tenant, scope.Project, monitoring.GrantInput{WriteMeta: monitoring.WriteMeta{RequestID: uuid.NewString(), ExpectedRevision: "1", Reason: "显式授权有限试算 halo"}, ChannelID: f.direct, ResourceGrantID: inputGrant.ID, Range: &monitoring.Range{From: &past, To: &edge}, Capabilities: []string{"read"}})
	must(t, e)
	monitor.WithStorage(new(monitoring.Storage))
	f.s.WithMonitoring(monitor)
	must(t, f.s.RegisterTasks())
	configuration, e := f.s.ChannelConfigs(ctx, actor, scope, created.Channel.Resource.ID, dm.MetricFilter{})
	must(t, e)
	cfg := configuration.Items[0]
	endNS, startNS := "200", "100"
	trial, e := f.s.ChannelTrial(ctx, actor, scope, created.Channel.Resource.ID, dm.TrialInput{WriteMeta: wm(created.Channel.ConfigRevision), ConfigID: cfg.ID, ConfigHash: cfg.ConfigHash, OutputRange: dm.NanosecondRange{From: startNS, To: &endNS}}, access.Meta{})
	must(t, e)
	if trial.Trial == nil {
		t.Fatal("finite empty trial did not finish", trial)
	}
	control := dm.ControlPreviewInput{Revision: created.Channel.ControlRevision, ControlProposal: dm.ControlProposal{Action: "enable", AutoStartNS: &startNS, TrialReceiptID: trial.Trial.ReceiptID, AllowWaiting: true}}
	_, e = f.s.PreviewControl(ctx, actor, scope, created.Channel.Resource.ID, control)
	monitoringCode(t, e, "monitoring_not_found")
	control.StopNS = &endNS
	controlPreview, e := f.s.PreviewControl(ctx, actor, scope, created.Channel.Resource.ID, control)
	must(t, e)
	enabled, e := f.s.ControlChannel(ctx, actor, scope, created.Channel.Resource.ID, dm.ControlInput{WriteMeta: wm(created.Channel.ControlRevision), PreviewID: controlPreview.ID, PreviewHash: controlPreview.Hash}, access.Meta{})
	must(t, e)
	automatic, _ := queueAutomaticFixture(t, f, monitor, enabled.Channel, trial.PlanHash, controlPreview.InputRanges, 100, 200)
	permit := monitoring.RawSystemPermit{Tenant: scope.Tenant, RunID: automatic, MandateRevision: enabled.Channel.MandateRevision}
	systemSnapshot, e := reader.FreezeSystem(ctx, permit, monitoring.RawFreezeRequest{Needs: controlPreview.InputRanges})
	must(t, e)
	grant(resources.AccessInput{Action: "change_grants", End: []resources.GrantEnd{{ID: inputGrant.ID, Revision: inputGrant.Revision}}})
	_, e = reader.CheckUser(ctx, actor, readScope, rawSnapshot.ID, false)
	monitoringCode(t, e, "monitoring_not_found")
	_, e = reader.CheckSystem(ctx, permit, systemSnapshot.ID, false)
	must(t, e)
	// Losing the input's read grant must not prevent an authorized operator
	// from stopping the output channel that remains delegated to this project.
	pause, e := f.s.PreviewControl(ctx, actor, scope, created.Channel.Resource.ID, dm.ControlPreviewInput{Revision: enabled.Channel.ControlRevision, ControlProposal: dm.ControlProposal{Action: "pause"}})
	must(t, e)
	_, e = f.s.ControlChannel(ctx, actor, scope, created.Channel.Resource.ID, dm.ControlInput{WriteMeta: wm(enabled.Channel.ControlRevision), PreviewID: pause.ID, PreviewHash: pause.Hash}, access.Meta{})
	must(t, e)
	configs, e := f.s.ChannelConfigs(ctx, actor, scope, created.Channel.Resource.ID, dm.MetricFilter{})
	must(t, e)
	if len(configs.Items) != 1 || !configs.Items[0].InputsRestricted || configs.Items[0].InputBindings != nil || configs.Items[0].Rule != nil {
		t.Fatal("revoked input leaked through saved configuration", configs)
	}
	deps, e := f.s.ChannelDependencies(ctx, actor, scope, created.Channel.Resource.ID)
	must(t, e)
	if !deps.Restricted || len(deps.Items) != 0 {
		t.Fatal("restricted graph leaked input metadata", deps)
	}
	_, e = f.s.CreateChannel(ctx, actor, scope, in, access.Meta{})
	code(t, e, "resource_not_found")
	preview, e := ps.Preview(ctx, actor, f.scope.Tenant, p.ProjectID, "", access.OpProjectArchive, projects.Change{})
	must(t, e)
	_, e = ps.Change(ctx, actor, f.scope.Tenant, p.ProjectID, "", access.OpProjectArchive, projects.Change{ChangeMeta: projects.ChangeMeta{WriteMeta: projects.WriteMeta{RequestID: uuid.NewString(), Reason: "归档验证"}, Version: preview.Version, ProjectVersion: preview.ProjectVersion, Hash: preview.Hash}}, access.Meta{})
	must(t, e)
	_, e = f.s.Channel(ctx, actor, scope, created.Channel.Resource.ID)
	must(t, e)
	in.RequestID = uuid.NewString()
	_, e = f.s.CreateChannel(ctx, actor, scope, in, access.Meta{})
	code(t, e, "project_archived")
}
