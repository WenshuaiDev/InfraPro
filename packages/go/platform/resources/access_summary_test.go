package resources_test

import (
	"encoding/json"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"strings"
	"testing"
)

func TestAccessWorkspaceSummarySearchPointChildrenAndBeforeValues(t *testing.T) {
	f := resourcesSetup(t)
	p := f.project(t, "Workspace")
	fac := f.facility(t, "WORKSPACE-FIRST")
	other := f.facility(t, "WORKSPACE-SECOND")
	point, e := f.write(t, target(fac, res.Point, ""), "create", map[string]any{"code": "P1", "name": "测点一"})
	must(t, e)
	second, e := f.write(t, target(fac, res.Point, ""), "create", map[string]any{"code": "P2", "name": "测点二"})
	must(t, e)
	channelTarget := target(fac, res.Channel, "")
	channelTarget.Point = point.ID
	channel, e := f.write(t, channelTarget, "create", map[string]any{"code": "C1", "name": "通道一", "metric_version_id": f.metric, "unit_version_id": f.unit, "timestamp_meaning": "observation_time"})
	must(t, e)
	channelTarget.Point = second.ID
	_, e = f.write(t, channelTarget, "create", map[string]any{"code": "C2", "name": "通道二", "metric_version_id": f.metric, "unit_version_id": f.unit, "timestamp_meaning": "observation_time"})
	must(t, e)
	in := f.accessInput(t, p, fac.ID, "create_link")
	in.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read", "update"}}, {Kind: res.Point, PointID: &point.ID, Capabilities: []string{"read", "create_channel"}}, {Kind: res.Channel, ChannelID: &channel.ID, Capabilities: []string{"read"}}}
	f.grant(t, p, fac.ID, in)
	in = f.accessInput(t, p, other.ID, "create_link")
	in.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}}
	f.grant(t, p, other.ID, in)
	overview, e := f.srv.AccessDetail(ctx, f.actor, f.tenant, p.id, "", res.AccessFilter{Limit: 1})
	must(t, e)
	expected := res.AccessSummary{Facilities: 2, Points: 1, Channels: 1, Maintainable: 2}
	if overview.Summary != expected || len(overview.Links) != 1 || overview.NextCursor == "" {
		t.Fatalf("page-limited summary: %+v", overview)
	}
	for _, link := range overview.Links {
		if link.Summary == nil || link.Summary.Facilities != 1 {
			t.Fatal("missing per-facility summary", link)
		}
	}
	filtered, e := f.srv.AccessDetail(ctx, f.actor, f.tenant, p.id, "", res.AccessFilter{Query: "FIRST", Limit: 1})
	must(t, e)
	if len(filtered.Links) != 1 || filtered.Links[0].FacilityID != fac.ID || filtered.Summary != expected {
		t.Fatal("search or global summary incorrect", filtered)
	}
	_, e = f.srv.AccessDetail(ctx, f.actor, f.tenant, p.id, "", res.AccessFilter{Query: "FIRST", Cursor: overview.NextCursor, Limit: 1})
	if e == nil {
		t.Fatal("cursor reused across search filters")
	}
	children, e := f.srv.AccessDetail(ctx, f.actor, f.tenant, p.id, fac.ID, res.AccessFilter{CandidateKind: res.Channel, CandidatePoint: point.ID})
	must(t, e)
	if len(children.Candidates.Items) != 1 || children.Candidates.Items[0].ID != channel.ID || len(children.CandidateGrants) != 1 {
		t.Fatal("point children mixed", children)
	}
	if children.Summary.Facilities != 1 || children.Summary.Points != 1 || children.Summary.Channels != 1 || children.Summary.Maintainable != 2 {
		t.Fatal("facility summary", children.Summary)
	}
	in = f.accessInput(t, p, fac.ID, "change_grants")
	g := children.CandidateGrants[0]
	in.Update = []res.GrantUpdate{{ID: g.ID, Revision: g.Revision, Capabilities: []string{"read", "update"}}}
	preview, e := f.srv.PreviewAccessChange(ctx, f.actor, f.tenant, p.id, fac.ID, in)
	must(t, e)
	raw, e := json.Marshal(preview)
	must(t, e)
	if !strings.Contains(string(raw), `"before":[`) || len(preview.Before) != 1 || len(preview.Before[0].Capabilities) != 1 || len(preview.Update[0].Capabilities) != 2 {
		t.Fatalf("missing original capabilities: %s", raw)
	}
	f.grant(t, p, fac.ID, in)
	in = f.accessInput(t, p, fac.ID, "revoke_link")
	f.grant(t, p, fac.ID, in)
	remaining, e := f.srv.AccessDetail(ctx, f.actor, f.tenant, p.id, "", res.AccessFilter{})
	must(t, e)
	if remaining.Summary != (res.AccessSummary{Facilities: 1}) {
		t.Fatal("ended grants counted", remaining.Summary)
	}
}
