package devicecatalog_test

import (
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	c "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	d "github.com/WenshuaiDev/InfraPro/packages/go/platform/devices"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/projects"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"strings"
	"testing"
)

func TestProjectOutputHistoryQualifiesBothGenerations(t *testing.T) {
	f := setup(t)
	b := runtime.Compose(f.env.Service.Access())
	tenant := d.TenantScope(f.tenant)
	var member string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM tenant_memberships WHERE tenant_id=$1 AND account_id=$2`, f.tenant, f.owner.ID).Scan(&member))
	p, e := projects.New(f.env.Service.Access()).Create(ctx, f.owner, f.tenant, projects.CreateInput{Fields: projects.Fields{Code: "history", Name: "项目", Purpose: "历史边界"}, WriteMeta: projects.WriteMeta{RequestID: f.meta(t).RequestID, Reason: "验证"}, Owner: member, Administrator: member}, access.Meta{})
	must(t, e)
	project := d.ProjectScope(f.tenant, p.ProjectID)
	m, e := f.s.CreateManufacturer(ctx, f.actor(), c.PlatformScope(), c.ManufacturerCreate{Meta: f.meta(t), Code: "history", Name: "厂家"}, access.Meta{})
	must(t, e)
	product := f.product(t, c.PlatformScope(), f.actor(), m.ID, "history", []c.Slot{f.fixed()})
	device, e := b.Devices.Create(ctx, f.owner, tenant, d.DeviceCreate{Meta: f.meta(t), Code: "history", Name: "设备", ProductVersionID: product.VersionID}, access.Meta{})
	must(t, e)
	slots, e := b.Devices.Slots(ctx, f.owner, tenant, device.ID, d.Filter{})
	must(t, e)
	old := slots.Items[0].Current
	grant := d.GrantChangeSet{Meta: f.meta(t), ProjectRevision: p.Version, AccessRevision: "1", Add: []d.GrantAdd{{Kind: "output", ID: old.ID}}}
	preview, e := b.Devices.PreviewGrants(ctx, f.owner, f.tenant, p.ProjectID, grant)
	must(t, e)
	grant.PreviewHash = preview.Hash
	_, e = b.Devices.ChangeGrants(ctx, f.owner, f.tenant, p.ProjectID, grant, access.Meta{})
	must(t, e)
	detail, e := b.Devices.Detail(ctx, f.owner, tenant, device.ID)
	must(t, e)
	replace := d.OutputReplace{RevisionInput: d.RevisionInput{Meta: f.meta(t), Revision: old.Revision}, OutputRevision: detail.OutputRevision, OutputFields: d.OutputFields{Name: "新代次仅供新授权", ObservationDirection: "new-direction"}}
	preview, e = b.Devices.PreviewOutput(ctx, f.owner, tenant, device.ID, old.ID, "replace", replace)
	must(t, e)
	replace.PreviewHash = preview.Hash
	next, e := b.Devices.ReplaceOutput(ctx, f.owner, tenant, device.ID, old.ID, "replace", replace, access.Meta{})
	must(t, e)
	history, e := b.Devices.Changes(ctx, f.owner, project, "", old.ID, d.Filter{Kind: "output"})
	must(t, e)
	raw, _ := json.Marshal(history)
	if len(history.Items) != 1 || strings.Contains(string(raw), next.OutputID) || strings.Contains(string(raw), "新代次仅供新授权") {
		t.Fatalf("new generation leaked: %s", raw)
	}
	state, e := b.Devices.Grants(ctx, f.owner, f.tenant, p.ProjectID, d.Filter{})
	must(t, e)
	grant = d.GrantChangeSet{Meta: f.meta(t), ProjectRevision: state.ProjectRevision, AccessRevision: state.AccessRevision, Add: []d.GrantAdd{{Kind: "output", ID: next.OutputID}}, End: []d.GrantEnd{{ID: state.Items[0].ID, Revision: state.Items[0].Revision}}}
	preview, e = b.Devices.PreviewGrants(ctx, f.owner, f.tenant, p.ProjectID, grant)
	must(t, e)
	grant.PreviewHash = preview.Hash
	_, e = b.Devices.ChangeGrants(ctx, f.owner, f.tenant, p.ProjectID, grant, access.Meta{})
	must(t, e)
	history, e = b.Devices.Changes(ctx, f.owner, project, "", next.OutputID, d.Filter{Kind: "output"})
	must(t, e)
	raw, _ = json.Marshal(history)
	if len(history.Items) != 1 || strings.Contains(string(raw), old.ID) || !strings.Contains(string(raw), "新代次仅供新授权") {
		t.Fatalf("replacement history wrong qualification: %s", raw)
	}
}
