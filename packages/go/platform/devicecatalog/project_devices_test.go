package devicecatalog_test

import (
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	c "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	d "github.com/WenshuaiDev/InfraPro/packages/go/platform/devices"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/projects"
	r "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	src "github.com/WenshuaiDev/InfraPro/packages/go/platform/sources"
	"strings"
	"testing"
)

func TestProjectOutputOnlyGrantsAndTwoEndedSourceAuthority(t *testing.T) {
	f := setup(t)
	b := runtime.Compose(f.env.Service.Access())
	tenant := d.TenantScope(f.tenant)
	targets := f.sourceTargets(t, b)
	var member string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM tenant_memberships WHERE tenant_id=$1 AND account_id=$2`, f.tenant, f.owner.ID).Scan(&member))
	ps := projects.New(f.env.Service.Access())
	project, e := ps.Create(ctx, f.owner, f.tenant, projects.CreateInput{Fields: projects.Fields{Code: "project", Name: "设备使用项目", Purpose: "授权边界"}, WriteMeta: projects.WriteMeta{RequestID: f.meta(t).RequestID, Reason: "验证"}, Owner: member, Administrator: member}, access.Meta{})
	must(t, e)
	scope := d.ProjectScope(f.tenant, project.ProjectID)
	m, e := f.s.CreateManufacturer(ctx, f.actor(), c.PlatformScope(), c.ManufacturerCreate{Meta: f.meta(t), Code: "factory", Name: "厂家"}, access.Meta{})
	must(t, e)
	p := f.product(t, c.PlatformScope(), f.actor(), m.ID, "sensor", []c.Slot{f.fixed(), {Key: "sibling", Name: "兄弟槽位", Mode: "configurable", AllowedShapes: []string{"decimal"}}})
	device, e := b.Devices.Create(ctx, f.owner, tenant, d.DeviceCreate{Meta: f.meta(t), Code: "sensor", Name: "设备", SerialNumber: "private-serial", Description: "private-description", ProductVersionID: p.VersionID}, access.Meta{})
	must(t, e)
	slots, e := b.Devices.Slots(ctx, f.owner, tenant, device.ID, d.Filter{})
	must(t, e)
	var output d.Output
	for _, v := range slots.Items {
		if v.Current != nil {
			output = *v.Current
		}
	}
	_, e = b.Devices.Output(ctx, f.owner, scope, "", output.ID)
	wantCode(t, e, "device_not_found")
	grant := d.GrantChangeSet{Meta: f.meta(t), AccessRevision: "1", ProjectRevision: project.Version, Add: []d.GrantAdd{{Kind: "output", ID: output.ID, CanUseSource: true}}}
	preview, e := b.Devices.PreviewGrants(ctx, f.owner, f.tenant, project.ProjectID, grant)
	must(t, e)
	grant.PreviewHash = preview.Hash
	_, e = b.Devices.ChangeGrants(ctx, f.owner, f.tenant, project.ProjectID, grant, access.Meta{})
	must(t, e)
	_, e = b.Devices.Detail(ctx, f.owner, scope, device.ID)
	wantCode(t, e, "device_not_found")
	visible, e := b.Devices.Output(ctx, f.owner, scope, "", output.ID)
	must(t, e)
	encoded, _ := json.Marshal(visible)
	for _, secret := range []string{"private-serial", "private-description", "sibling", "installation"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("output-only projection leaked %s", secret)
		}
	}

	roots, e := b.Devices.Tree(ctx, f.owner, scope, d.Filter{})
	must(t, e)
	if len(roots.Items) != 1 || roots.Items[0].ID != m.ID || roots.Items[0].Visibility != "context" {
		t.Fatalf("project ancestor projection: %+v", roots)
	}
	productNodes, e := b.Devices.Tree(ctx, f.owner, scope, d.Filter{Level: "product", ParentID: m.ID})
	must(t, e)
	if len(productNodes.Items) != 1 || productNodes.Items[0].ID != p.ID {
		t.Fatal(productNodes)
	}
	deviceNodes, e := b.Devices.Tree(ctx, f.owner, scope, d.Filter{Level: "device", ParentID: p.ID})
	must(t, e)
	if len(deviceNodes.Items) != 1 || deviceNodes.Items[0].Visibility != "context" {
		t.Fatal(deviceNodes)
	}
	outputNodes, e := b.Devices.Tree(ctx, f.owner, scope, d.Filter{Level: "output", ParentID: device.ID})
	must(t, e)
	if len(outputNodes.Items) != 1 || outputNodes.Items[0].ID != output.ID {
		t.Fatal(outputNodes)
	}
	state, e := ps.Context(ctx, f.owner, f.tenant, project.ProjectID)
	must(t, e)
	if state.DeviceAccessRevision != "2" {
		t.Fatal("missing device authorization revision")
	}
	// Channel update is independent of source_manage.
	accessState, e := b.Resources.AccessDetail(ctx, f.owner, f.tenant, project.ProjectID, targets[0].FacilityID, r.AccessFilter{})
	must(t, e)
	rg := r.AccessInput{RequestID: f.meta(t).RequestID, Action: "create_link", AccessRevision: accessState.Policy.Revision, Reason: "明确资源读取", Add: []r.GrantInput{{Kind: r.Facility, Capabilities: []string{"read"}}, {Kind: r.Channel, ChannelID: &targets[0].ChannelID, Capabilities: []string{"read", "update"}}}}
	rp, e := b.Resources.PreviewAccessChange(ctx, f.owner, f.tenant, project.ProjectID, targets[0].FacilityID, rg)
	must(t, e)
	rg.PreviewHash = rp.Hash
	_, e = b.Resources.ChangeAccess(ctx, f.owner, f.tenant, project.ProjectID, targets[0].FacilityID, rg, access.Meta{})
	must(t, e)
	current, e := b.Sources.Current(ctx, f.owner, scope, targets[0])
	must(t, e)
	if current.Actions.Bind {
		t.Fatal("update implied source_manage")
	}
	in := src.Bind(src.BindInput{Revisions: src.Revisions{Meta: f.meta(t), SourceRevision: current.SourceRevision, ChannelRevision: current.ChannelRevision, ResourceAccessRevision: current.ResourceAccessRevision, DeviceAccessRevision: current.DeviceAccessRevision, LinkRevision: current.LinkRevision}, NewEnd: src.NewEnd{OutputID: output.ID, NewOutputRevision: "1", NewDeviceRevision: "1"}})
	_, e = b.Sources.Preview(ctx, f.owner, scope, targets[0], in)
	wantCode(t, e, "permission_denied")
	accessState, e = b.Resources.AccessDetail(ctx, f.owner, f.tenant, project.ProjectID, targets[0].FacilityID, r.AccessFilter{})
	must(t, e)
	var channelGrant r.Grant
	for _, g := range accessState.Grants {
		if g.Kind == r.Channel {
			channelGrant = g
		}
	}
	rg = r.AccessInput{RequestID: f.meta(t).RequestID, Action: "change_grants", AccessRevision: accessState.Policy.Revision, LinkRevision: accessState.Link.Revision, Reason: "明确来源维护", Update: []r.GrantUpdate{{ID: channelGrant.ID, Revision: channelGrant.Revision, Capabilities: []string{"read", "update", "source_manage"}}}}
	rp, e = b.Resources.PreviewAccessChange(ctx, f.owner, f.tenant, project.ProjectID, targets[0].FacilityID, rg)
	must(t, e)
	rg.PreviewHash = rp.Hash
	_, e = b.Resources.ChangeAccess(ctx, f.owner, f.tenant, project.ProjectID, targets[0].FacilityID, rg, access.Meta{})
	must(t, e)
	current, e = b.Sources.Current(ctx, f.owner, scope, targets[0])
	must(t, e)

	reverse, e := b.Sources.ChannelCandidates(ctx, f.owner, scope, output.ID, src.Filter{})
	must(t, e)
	if len(reverse.Items) != 1 || reverse.Items[0].ChannelID != targets[0].ChannelID {
		t.Fatalf("unauthorized reverse candidates: %+v", reverse)
	}
	in.ResourceAccessRevision = current.ResourceAccessRevision
	in.LinkRevision = current.LinkRevision
	preview, e = b.Sources.Preview(ctx, f.owner, scope, targets[0], in)
	must(t, e)
	in.PreviewHash = preview.Hash
	bound, e := b.Sources.Mutate(ctx, f.owner, scope, targets[0], in, access.Meta{})
	must(t, e)

	reverseHistory, e := b.Sources.OutputHistory(ctx, f.owner, scope, output.ID, src.Filter{})
	must(t, e)
	if len(reverseHistory.Items) != 1 || reverseHistory.Items[0].ChannelID != targets[0].ChannelID {
		t.Fatal(reverseHistory)
	}
	grants, e := b.Devices.Grants(ctx, f.owner, f.tenant, project.ProjectID, d.Filter{})
	must(t, e)
	revoke := d.GrantChangeSet{Meta: f.meta(t), AccessRevision: grants.AccessRevision, ProjectRevision: grants.ProjectRevision, End: []d.GrantEnd{{ID: grants.Items[0].ID, Revision: grants.Items[0].Revision}}}
	preview, e = b.Devices.PreviewGrants(ctx, f.owner, f.tenant, project.ProjectID, revoke)
	must(t, e)
	revoke.PreviewHash = preview.Hash
	_, e = b.Devices.ChangeGrants(ctx, f.owner, f.tenant, project.ProjectID, revoke, access.Meta{})
	must(t, e)
	current, e = b.Sources.Current(ctx, f.owner, scope, targets[0])
	must(t, e)
	if !current.Source.Configured || current.Source.Visibility != "restricted" || current.Source.Output != nil || current.Actions.Unbind {
		t.Fatal("revoked source leaked or became unconfigured")
	}
	_, e = b.Sources.Mutate(ctx, f.owner, scope, targets[0], in, access.Meta{})
	wantCode(t, e, "device_not_found")
	// Tenant cleanup remains explicit and does not revive any project grant.
	unbind := src.Unbind(src.UnbindInput{Revisions: src.Revisions{Meta: f.meta(t), SourceRevision: bound.SourceRevision, ChannelRevision: "1"}, OldEnd: src.OldEnd{BindingID: bound.BindingID, BindingRevision: bound.Revision, OldOutputRevision: "1", OldDeviceRevision: "1"}})
	preview, e = b.Sources.Preview(ctx, f.owner, tenant, targets[0], unbind)
	must(t, e)
	unbind.PreviewHash = preview.Hash
	_, e = b.Sources.Mutate(ctx, f.owner, tenant, targets[0], unbind, access.Meta{})
	must(t, e)
}
