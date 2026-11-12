package resources_test

import (
	"errors"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/projects"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
)

func monitoringCode(t *testing.T, err error, want string) {
	t.Helper()
	var e *md.Error
	if !errors.As(err, &e) || e.Code != want {
		t.Fatalf("want %s, got %v", want, err)
	}
}

func TestMonitoringExplicitTimeGrantsAndRevocation(t *testing.T) {
	f := resourcesSetup(t)
	project := f.project(t, "MonitoringA")
	other := f.project(t, "MonitoringB")
	s := md.New(f.env.Service.Access())
	facility := f.facility(t, "MONITORING")
	point, e := f.write(t, target(facility, res.Point, ""), "create", map[string]any{"code": "P", "name": "测点"})
	must(t, e)
	ct := target(facility, res.Channel, "")
	ct.Point = point.ID
	channel, e := f.write(t, ct, "create", map[string]any{"code": "C", "name": "监测通道", "metric_version_id": f.metric, "unit_version_id": f.unit, "timestamp_meaning": "observation_time"})
	must(t, e)
	ct.ID = channel.ID
	grant := f.accessInput(t, project, facility.ID, "create_link")
	grant.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}, {Kind: res.Channel, ChannelID: &channel.ID, Capabilities: []string{"read"}}}
	f.grant(t, project, facility.ID, grant)
	var resourceGrant string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM project_resource_grants WHERE project_id=$1 AND channel_id=$2 AND ended_at IS NULL`, project.id, channel.ID).Scan(&resourceGrant))
	scope := md.ProjectScope(f.tenant, project.id)
	_, _, e = s.AuthorizedWindows(ctx, project.actor, scope, []string{channel.ID}, "read")
	monitoringCode(t, e, "monitoring_not_found")
	from, to := "2024-01-01T00:00:00.000000003Z", "2024-01-01T00:00:00.000000008Z"
	in := md.GrantInput{WriteMeta: md.WriteMeta{RequestID: f.wm(t).RequestID, ExpectedRevision: "0", Reason: "仅授权五纳秒历史"}, ChannelID: channel.ID, ResourceGrantID: resourceGrant, Range: &md.Range{From: &from, To: &to}, Capabilities: []string{"read"}}
	result, e := s.GrantData(ctx, f.actor, f.tenant, project.id, in)
	must(t, e)
	replay, e := s.GrantData(ctx, f.actor, f.tenant, project.id, in)
	must(t, e)
	if !replay.Replayed || replay.ID != result.ID {
		t.Fatal("grant receipt lost")
	}
	changed := in
	changed.Capabilities = []string{"read", "export"}
	_, e = s.GrantData(ctx, f.actor, f.tenant, project.id, changed)
	monitoringCode(t, e, "request_id_reused")
	windows, revision, e := s.AuthorizedWindows(ctx, project.actor, scope, []string{channel.ID}, "read")
	must(t, e)
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).UnixNano()
	start := base + 2
	if len(windows[channel.ID]) != 1 || !windows[channel.ID][0].Contains(base+3, nil) || windows[channel.ID][0].Contains(base+8, nil) || windows[channel.ID][0].Contains(base+5, &start) || revision == "" {
		t.Fatal("nanosecond/full interval authorization", windows)
	}
	_, _, e = s.AuthorizedWindows(ctx, project.actor, scope, []string{channel.ID}, "export")
	monitoringCode(t, e, "monitoring_not_found")
	_, _, e = s.AuthorizedWindows(ctx, other.actor, md.ProjectScope(f.tenant, other.id), []string{channel.ID}, "read")
	monitoringCode(t, e, "monitoring_not_found")
	_, _, e = s.AuthorizedWindows(ctx, f.actor, scope, []string{channel.ID}, "read")
	if e == nil {
		t.Fatal("tenant authority leaked into project entry")
	}
	page, e := s.ListDataGrants(ctx, f.actor, f.tenant, project.id, "")
	must(t, e)
	if len(page.Items) != 1 || !page.Items[0].Effective || *page.Items[0].Range.From != from || page.AccessRevision != "1" {
		t.Fatal(page)
	}
	preview, e := s.PreviewRevokeData(ctx, f.actor, f.tenant, project.id, result.ID)
	must(t, e)
	_, e = s.RevokeData(ctx, f.actor, f.tenant, project.id, result.ID, md.RevokeInput{WriteMeta: md.WriteMeta{RequestID: f.wm(t).RequestID, ExpectedRevision: result.Revision, Reason: "撤回范围"}, PreviewHash: "stale"})
	monitoringCode(t, e, "revision_changed")
	revoke := md.RevokeInput{WriteMeta: md.WriteMeta{RequestID: f.wm(t).RequestID, ExpectedRevision: result.Revision, Reason: "撤回范围"}, PreviewHash: preview.Hash}
	result, e = s.RevokeData(ctx, f.actor, f.tenant, project.id, result.ID, revoke)
	must(t, e)
	_, _, e = s.AuthorizedWindows(ctx, project.actor, scope, []string{channel.ID}, "read")
	monitoringCode(t, e, "monitoring_not_found")
	_, e = f.env.Pool.Exec(ctx, `UPDATE monitoring_data_grants SET revoked_at=NULL,revoked_by=NULL,revoke_reason=NULL WHERE id=$1`, result.ID)
	if e == nil {
		t.Fatal("revoked data grant revived")
	}
	// A new data grant is bound to this exact resource-grant identity.
	in.RequestID = f.wm(t).RequestID
	in.ExpectedRevision = result.AccessRevision
	in.Capabilities = []string{"read", "export"}
	_, e = s.GrantData(ctx, f.actor, f.tenant, project.id, in)
	must(t, e)
	change := f.accessInput(t, project, facility.ID, "change_grants")
	change.End = []res.GrantEnd{{ID: resourceGrant, Revision: "1"}}
	f.grant(t, project, facility.ID, change)
	_, _, e = s.AuthorizedWindows(ctx, project.actor, scope, []string{channel.ID}, "read")
	monitoringCode(t, e, "monitoring_not_found")
	change = f.accessInput(t, project, facility.ID, "change_grants")
	change.Add = []res.GrantInput{{Kind: res.Channel, ChannelID: &channel.ID, Capabilities: []string{"read"}}}
	f.grant(t, project, facility.ID, change)
	_, _, e = s.AuthorizedWindows(ctx, project.actor, scope, []string{channel.ID}, "read")
	monitoringCode(t, e, "monitoring_not_found")
	// FK and trigger reject a cross-project resource grant even from direct SQL.
	_, e = f.env.Pool.Exec(ctx, `INSERT INTO monitoring_data_grants(tenant_id,project_id,channel_id,resource_grant_id,permitted,capabilities,granted_by,reason) VALUES($1,$2,$3,$4,'(,)'::int8range,ARRAY['read'],$5,'cross-project fixture')`, f.tenant, other.id, channel.ID, resourceGrant, f.actor.ID)
	if e == nil {
		t.Fatal("cross-project grant accepted")
	}
	// Archive through the real project workflow. Read/export keep operating,
	// but import and even tenant-governed data authorization are blocked.
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM project_resource_grants WHERE project_id=$1 AND channel_id=$2 AND ended_at IS NULL`, project.id, channel.ID).Scan(&in.ResourceGrantID))
	page, e = s.ListDataGrants(ctx, f.actor, f.tenant, project.id, "")
	must(t, e)
	in.RequestID = f.wm(t).RequestID
	in.ExpectedRevision = page.AccessRevision
	in.Capabilities = []string{"read", "export", "import"}
	_, e = s.GrantData(ctx, f.actor, f.tenant, project.id, in)
	must(t, e)
	ps := projects.New(f.env.Service.Access())
	archive := projects.Change{ChangeMeta: projects.ChangeMeta{WriteMeta: projects.WriteMeta{RequestID: f.wm(t).RequestID, Reason: "验证归档监测只读"}}}
	pr, e := ps.Preview(ctx, project.actor, f.tenant, project.id, "", access.OpProjectArchive, archive)
	must(t, e)
	if !pr.CanSubmit {
		t.Fatal(pr.BlockedReason)
	}
	archive.ProjectVersion, archive.Version, archive.Hash = pr.ProjectVersion, pr.Version, pr.Hash
	_, e = ps.Change(ctx, project.actor, f.tenant, project.id, "", access.OpProjectArchive, archive, access.Meta{})
	must(t, e)
	for _, capability := range []string{"read", "export"} {
		_, _, e = s.AuthorizedWindows(ctx, project.actor, scope, []string{channel.ID}, capability)
		must(t, e)
	}
	_, _, e = s.AuthorizedWindows(ctx, project.actor, scope, []string{channel.ID}, "import")
	code(t, e, "project_archived")
	in.RequestID = f.wm(t).RequestID
	_, e = s.GrantData(ctx, f.actor, f.tenant, project.id, in)
	monitoringCode(t, e, "project_archived")
}

func TestMonitoringPermanentReferencesComposeWithDevices(t *testing.T) {
	f := resourcesSetup(t)
	business := runtime.Compose(f.env.Service.Access())
	f.srv = business.Resources
	facility := f.facility(t, "PERMANENT")
	point, e := f.write(t, target(facility, res.Point, ""), "create", map[string]any{"code": "P", "name": "测点"})
	must(t, e)
	ct := target(facility, res.Channel, "")
	ct.Point = point.ID
	channel, e := f.write(t, ct, "create", map[string]any{"code": "C", "name": "历史通道", "metric_version_id": f.metric, "unit_version_id": f.unit, "timestamp_meaning": "observation_time"})
	must(t, e)
	ct.ID = channel.ID
	_, e = f.env.Pool.Exec(ctx, `INSERT INTO monitoring_channel_versions(tenant_id,channel_id,first_published_sequence) VALUES($1,$2,1)`, f.tenant, channel.ID)
	must(t, e)
	detail, e := business.Resources.Detail(ctx, f.actor, res.TenantScope(f.tenant), ct)
	must(t, e)
	if detail.Actions.Delete {
		t.Fatal("resource UI permits deleting a permanent monitoring reference")
	}
	_, e = f.write(t, ct, "delete", map[string]any{"expected_revision": 1, "reason": "验证监测引用保护"})
	if e == nil {
		t.Fatal("monitoring channel deleted")
	}
	_, e = f.env.Pool.Exec(ctx, `UPDATE engineering_channels SET deleted_at=clock_timestamp() WHERE id=$1`, channel.ID)
	if e == nil {
		t.Fatal("database bypassed monitoring protection")
	}
	// Composition retains the existing device module rather than replacing it.
	if business.Devices == nil || business.Sources == nil || business.Monitoring == nil {
		t.Fatal("composition lost an existing module")
	}
}
