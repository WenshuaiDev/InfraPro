package devicecatalog_test

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	c "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	d "github.com/WenshuaiDev/InfraPro/packages/go/platform/devices"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/projects"
	r "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	src "github.com/WenshuaiDev/InfraPro/packages/go/platform/sources"
	"github.com/jackc/pgx/v5"
	"testing"
	"time"
)

func TestSourceInvalidationBothCommitOrders(t *testing.T) {
	for _, kind := range []string{"disable", "retire", "replace", "revoke"} {
		for _, first := range []string{"change", "bind"} {
			t.Run(kind+"/"+first, func(t *testing.T) {
				f := setup(t)
				b := runtime.Compose(f.env.Service.Access())
				tenant := d.TenantScope(f.tenant)
				target := f.sourceTargets(t, b)[0]
				m, e := f.s.CreateManufacturer(ctx, f.actor(), c.PlatformScope(), c.ManufacturerCreate{Meta: f.meta(t), Code: "factory", Name: "厂家"}, access.Meta{})
				must(t, e)
				product := f.product(t, c.PlatformScope(), f.actor(), m.ID, "sensor", []c.Slot{f.fixed()})
				device, e := b.Devices.Create(ctx, f.owner, tenant, d.DeviceCreate{Meta: f.meta(t), Code: "DEV", Name: "设备", ProductVersionID: product.VersionID}, access.Meta{})
				must(t, e)
				slots, e := b.Devices.Slots(ctx, f.owner, tenant, device.ID, d.Filter{})
				must(t, e)
				output := *slots.Items[0].Current
				scope := tenant
				var change func(access.Actor) error
				switch kind {
				case "disable":
					in := d.DeviceMutation{Action: "state", Status: "disabled", DeviceUpdate: d.DeviceUpdate{RevisionInput: d.RevisionInput{Meta: f.meta(t), Revision: device.Revision}}}
					p, e := b.Devices.PreviewDevice(ctx, f.owner, tenant, device.ID, in)
					must(t, e)
					in.PreviewHash = p.Hash
					change = func(a access.Actor) error {
						_, e := b.Devices.Mutate(ctx, a, tenant, device.ID, in, access.Meta{})
						return e
					}
				case "retire":
					raw, _ := json.Marshal(map[string]any{"request_id": f.meta(t).RequestID, "reason": "来源状态竞争", "expected_revision": 1})
					resource := r.Target{Kind: r.Channel, Facility: target.FacilityID, Point: target.PointID, ID: target.ChannelID}
					p, e := b.Resources.PreviewMutation(ctx, f.owner, r.TenantScope(f.tenant), resource, "retire", raw)
					must(t, e)
					var fields map[string]any
					must(t, json.Unmarshal(raw, &fields))
					fields["preview_hash"] = p.Hash
					raw, _ = json.Marshal(fields)
					change = func(a access.Actor) error {
						_, e := b.Resources.Mutate(ctx, a, r.TenantScope(f.tenant), resource, "retire", raw, access.Meta{})
						return e
					}
				case "replace":
					in := d.OutputReplace{RevisionInput: d.RevisionInput{Meta: f.meta(t), Revision: output.Revision}, OutputRevision: device.OutputRevision, OutputFields: d.OutputFields{Name: "新含义", ObservationDirection: "横向"}}
					p, e := b.Devices.PreviewOutput(ctx, f.owner, tenant, device.ID, output.ID, "replace", in)
					must(t, e)
					in.PreviewHash = p.Hash
					change = func(a access.Actor) error {
						_, e := b.Devices.ReplaceOutput(ctx, a, tenant, device.ID, output.ID, "replace", in, access.Meta{})
						return e
					}
				case "revoke":
					var member string
					must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM tenant_memberships WHERE tenant_id=$1 AND account_id=$2`, f.tenant, f.owner.ID).Scan(&member))
					project, e := projects.New(f.env.Service.Access()).Create(ctx, f.owner, f.tenant, projects.CreateInput{WriteMeta: projects.WriteMeta{RequestID: f.meta(t).RequestID, Reason: "并发测试"}, Fields: projects.Fields{Code: "RACE", Name: "项目", Purpose: "撤权"}, Owner: member, Administrator: member}, access.Meta{})
					must(t, e)
					scope = d.ProjectScope(f.tenant, project.ProjectID)
					grant := d.GrantChangeSet{Meta: f.meta(t), AccessRevision: "1", ProjectRevision: project.Version, Add: []d.GrantAdd{{Kind: "output", ID: output.ID, CanUseSource: true}}}
					p, e := b.Devices.PreviewGrants(ctx, f.owner, f.tenant, project.ProjectID, grant)
					must(t, e)
					grant.PreviewHash = p.Hash
					_, e = b.Devices.ChangeGrants(ctx, f.owner, f.tenant, project.ProjectID, grant, access.Meta{})
					must(t, e)
					rg := r.AccessInput{RequestID: f.meta(t).RequestID, Reason: "来源资格", Action: "create_link", AccessRevision: "1", Add: []r.GrantInput{{Kind: r.Facility, Capabilities: []string{"read"}}, {Kind: r.Channel, ChannelID: &target.ChannelID, Capabilities: []string{"read", "source_manage"}}}}
					rp, e := b.Resources.PreviewAccessChange(ctx, f.owner, f.tenant, project.ProjectID, target.FacilityID, rg)
					must(t, e)
					rg.PreviewHash = rp.Hash
					_, e = b.Resources.ChangeAccess(ctx, f.owner, f.tenant, project.ProjectID, target.FacilityID, rg, access.Meta{})
					must(t, e)
					gs, e := b.Devices.Grants(ctx, f.owner, f.tenant, project.ProjectID, d.Filter{})
					must(t, e)
					revoke := d.GrantChangeSet{Meta: f.meta(t), AccessRevision: gs.AccessRevision, ProjectRevision: gs.ProjectRevision, End: []d.GrantEnd{{ID: gs.Items[0].ID, Revision: gs.Items[0].Revision}}}
					p, e = b.Devices.PreviewGrants(ctx, f.owner, f.tenant, project.ProjectID, revoke)
					must(t, e)
					revoke.PreviewHash = p.Hash
					change = func(a access.Actor) error {
						_, e := b.Devices.ChangeGrants(ctx, a, f.tenant, project.ProjectID, revoke, access.Meta{})
						return e
					}
				}
				current, e := b.Sources.Current(ctx, f.owner, scope, target)
				must(t, e)
				in := src.Bind(src.BindInput{Revisions: src.Revisions{Meta: f.meta(t), SourceRevision: current.SourceRevision, ChannelRevision: current.ChannelRevision, DeviceAccessRevision: current.DeviceAccessRevision, ResourceAccessRevision: current.ResourceAccessRevision, LinkRevision: current.LinkRevision}, NewEnd: src.NewEnd{OutputID: output.ID, NewDeviceRevision: device.Revision, NewOutputRevision: output.Revision}})
				p, e := b.Sources.Preview(ctx, f.owner, scope, target, in)
				must(t, e)
				in.PreviewHash = p.Hash
				bind := func(a access.Actor) error {
					_, e := b.Sources.Mutate(ctx, a, scope, target, in, access.Meta{})
					return e
				}
				actor := f.owner
				original := actor.Check
				entered, release := make(chan struct{}), make(chan struct{})
				actor.Check = func(c context.Context, tx pgx.Tx, recent bool) error {
					if e := original(c, tx, recent); e != nil {
						return e
					}
					close(entered)
					<-release
					return nil
				}
				one, two := change, bind
				if first == "bind" {
					one, two = bind, change
				}
				done1, done2 := make(chan error, 1), make(chan error, 1)
				go func() { done1 <- one(actor) }()
				select {
				case <-entered:
				case <-time.After(5 * time.Second):
					t.Fatal("first transaction did not reach barrier")
				}
				go func() { done2 <- two(f.owner) }()
				select {
				case e := <-done2:
					close(release)
					t.Fatalf("second bypassed coordination: %v", e)
				case <-time.After(30 * time.Millisecond):
				}
				close(release)
				must(t, <-done1)
				second := <-done2
				if first == "change" && second == nil {
					t.Fatal("source committed after endpoint invalidation")
				}
				if second != nil {
					var de *c.Error
					var ae *access.Error
					var re *r.Error
					if errors.As(second, &de) {
						if de.Status >= 500 {
							t.Fatal(second)
						}
					} else if errors.As(second, &ae) {
						if ae.Status >= 500 {
							t.Fatal(second)
						}
					} else if errors.As(second, &re) {
						if re.Status >= 500 {
							t.Fatal(second)
						}
					} else {
						t.Fatal(second)
					}
				}
				now, e := b.Sources.Current(ctx, f.owner, tenant, target)
				must(t, e)
				if now.Source.Configured != (first == "bind") {
					t.Fatal("source commit order was not preserved")
				}
			})
		}
	}
}
