package resources_test

import (
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/projects"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"testing"
)

func TestGovernanceResourceCountsUseCurrentExplicitGrants(t *testing.T) {
	f := resourcesSetup(t)
	a, b := f.project(t, "CountA"), f.project(t, "CountB")
	facility := f.facility(t, "COUNT")
	point, err := f.write(t, target(facility, res.Point, ""), "create", map[string]any{"code": "P", "name": "统计测点"})
	must(t, err)
	ct := target(facility, res.Channel, "")
	ct.Point = point.ID
	channel, err := f.write(t, ct, "create", map[string]any{"code": "CH", "name": "统计通道", "metric_version_id": f.metric, "unit_version_id": f.unit, "timestamp_meaning": "observation_time"})
	must(t, err)
	root := res.GrantInput{Kind: res.Facility, Capabilities: []string{"read"}}
	in := f.accessInput(t, a, facility.ID, "create_link")
	in.Add = []res.GrantInput{root, {Kind: res.Point, PointID: &point.ID, Capabilities: []string{"read"}}}
	f.grant(t, a, facility.ID, in)
	in = f.accessInput(t, b, facility.ID, "create_link")
	in.Add = []res.GrantInput{root, {Kind: res.Channel, ChannelID: &channel.ID, Capabilities: []string{"read"}}}
	f.grant(t, b, facility.ID, in)
	ps := projects.New(f.env.Service.Access())
	check := func(p projectFixture, facilities, points int) {
		t.Helper()
		g, err := ps.Governance(ctx, f.actor, f.tenant, p.id)
		must(t, err)
		s := g.Statistics
		if s == nil || s.Facilities == nil || *s.Facilities != facilities || s.Points == nil || *s.Points != points {
			t.Fatalf("unexpected counts: %+v", s)
		}
	}
	check(a, 1, 1)
	check(b, 1, 0) // The point above an authorized channel is context, not a point grant.
	f.grant(t, a, facility.ID, f.accessInput(t, a, facility.ID, "revoke_link"))
	check(a, 0, 0)
	check(b, 1, 0)
	in = f.accessInput(t, a, facility.ID, "create_link")
	in.Add = []res.GrantInput{root}
	f.grant(t, a, facility.ID, in)
	check(a, 1, 0) // Ended grants from the old link must not return.
}
