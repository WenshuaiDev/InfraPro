package resources_test

import (
	"bytes"
	"encoding/json"
	"sync"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
)

func TestStructureMoveHiddenDescendantsAndPositionDisclosure(t *testing.T) {
	f := resourcesSetup(t)
	facility := f.typedFacility(t)
	root := res.StructureRef{Kind: res.Facility, ID: facility.ID}
	section := f.structureCreate(t, facility, res.MonitoringSection, "S", root, nil)
	hole := f.structureCreate(t, facility, res.Borehole, "SECRET_HOLE", res.StructureRef{Kind: res.MonitoringSection, ID: section.ID}, nil)
	point := f.structureCreate(t, facility, res.Point, "P", res.StructureRef{Kind: res.Borehole, ID: hole.ID}, nil)
	position := f.recordInput(t, facility, res.Point, point.ID, "position")
	position.Position = &res.BoreholePosition{BoreholeID: hole.ID, BoreholeRevision: 1, DistanceM: ptr(0.0), ReferenceStart: "保密孔口依据", Direction: "沿孔", Unit: "m"}
	f.saveRecord(t, facility, res.Point, point.ID, position)
	p := f.project(t, "MovePermissions")
	g := f.accessInput(t, p, facility.ID, "create_link")
	g.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read", "update", "create_monitoring_section", "create_borehole"}}, {Kind: res.MonitoringSection, MonitoringSectionID: &section.ID, Capabilities: []string{"read", "update"}}, {Kind: res.Point, PointID: &point.ID, Capabilities: []string{"read", "update"}}}
	f.grant(t, p, facility.ID, g)
	scope := res.ProjectScope(f.tenant, p.id)
	readPosition := func(restricted bool) {
		t.Helper()
		page, e := f.srv.StructureRecords(ctx, p.actor, scope, target(facility, res.Point, point.ID), "positions", "", 100)
		must(t, e)
		if restricted {
			raw, _ := json.Marshal(page)
			if page.PositionState != "restricted" || page.CurrentPosition != nil || len(page.Items) != 0 || bytes.Contains(raw, []byte(hole.ID)) || bytes.Contains(raw, []byte("保密")) {
				t.Fatal("hidden hole disclosed", string(raw))
			}
		} else if page.PositionState != "provided" || page.CurrentPosition == nil || len(page.Items) != 1 {
			t.Fatal(page)
		}
	}
	readPosition(true)
	move := func() res.MoveInput {
		in := f.moveInput(t, facility, res.MonitoringSection, section.ID, root)
		current := f.accessInput(t, p, facility.ID, "change_grants")
		in.AccessRevision, in.LinkRevision = current.AccessRevision, current.LinkRevision
		return in
	}
	denied := func() {
		t.Helper()
		preview, e := f.srv.PreviewMove(ctx, p.actor, scope, target(facility, res.MonitoringSection, section.ID), move())
		raw, _ := json.Marshal(struct {
			Preview any
			Error   any
		}{preview, e})
		if e == nil || bytes.Contains(raw, []byte(hole.ID)) || bytes.Contains(raw, []byte("SECRET_HOLE")) || len(preview.Affected) != 0 {
			t.Fatal("hidden/unmaintainable subtree was disclosed or moved", string(raw))
		}
	}
	denied()
	g = f.accessInput(t, p, facility.ID, "change_grants")
	g.Add = []res.GrantInput{{Kind: res.Borehole, BoreholeID: &hole.ID, Capabilities: []string{"read"}}}
	f.grant(t, p, facility.ID, g)
	readPosition(false)
	denied() // Seeing the hole is insufficient maintenance delegation.
	var grantID, revision string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text,revision::text FROM project_resource_grants WHERE project_id=$1 AND borehole_id=$2 AND ended_at IS NULL`, p.id, hole.ID).Scan(&grantID, &revision))
	g = f.accessInput(t, p, facility.ID, "change_grants")
	g.Update = []res.GrantUpdate{{ID: grantID, Revision: revision, Capabilities: []string{"read", "update"}}}
	f.grant(t, p, facility.ID, g)
	in := move()
	preview, e := f.srv.PreviewMove(ctx, p.actor, scope, target(facility, res.MonitoringSection, section.ID), in)
	must(t, e)
	if len(preview.Affected) != 3 {
		t.Fatal(preview)
	}
	in.PreviewHash = preview.Hash
	result, e := f.srv.Move(ctx, p.actor, scope, target(facility, res.MonitoringSection, section.ID), in, access.Meta{})
	must(t, e)
	if result.ID != section.ID {
		t.Fatal(result)
	}
	_, e = f.srv.Move(ctx, p.actor, scope, target(facility, res.MonitoringSection, section.ID), in, access.Meta{})
	must(t, e)
	// Receipt replay must reauthorize its historical affected set after withdrawal.
	must(t, f.env.Pool.QueryRow(ctx, `SELECT revision::text FROM project_resource_grants WHERE id=$1`, grantID).Scan(&revision))
	g = f.accessInput(t, p, facility.ID, "change_grants")
	g.End = []res.GrantEnd{{ID: grantID, Revision: revision}}
	f.grant(t, p, facility.ID, g)
	readPosition(true)
	if _, e = f.srv.Move(ctx, p.actor, scope, target(facility, res.MonitoringSection, section.ID), in, access.Meta{}); e == nil {
		t.Fatal("withdrawn descendant replay succeeded")
	}
	// Archived projects retain permitted reads and refuse fresh edits.
	_, e = f.env.Pool.Exec(ctx, `UPDATE projects SET status='archived',archived_at=now(),archive_reason='结构归档验证',archived_by=$2,version=version+1 WHERE id=$1`, p.id, p.actor.ID)
	must(t, e)
	_, e = f.srv.Detail(ctx, p.actor, scope, target(facility, res.MonitoringSection, section.ID))
	must(t, e)
	if _, e = f.srv.PreviewMove(ctx, p.actor, scope, target(facility, res.MonitoringSection, section.ID), move()); e == nil {
		t.Fatal("archived project allowed move")
	}
}

func TestStructureConcurrentMovesCannotCreateCycle(t *testing.T) {
	f := resourcesSetup(t)
	facility := f.typedFacility(t)
	root := res.StructureRef{Kind: res.Facility, ID: facility.ID}
	a := f.structureCreate(t, facility, res.MonitoringSection, "A", root, nil)
	b := f.structureCreate(t, facility, res.Borehole, "B", root, nil)
	inputs := []res.MoveInput{f.moveInput(t, facility, res.MonitoringSection, a.ID, res.StructureRef{Kind: res.Borehole, ID: b.ID}), f.moveInput(t, facility, res.Borehole, b.ID, res.StructureRef{Kind: res.MonitoringSection, ID: a.ID})}
	targets := []res.Target{target(facility, res.MonitoringSection, a.ID), target(facility, res.Borehole, b.ID)}
	for i := range inputs {
		p, e := f.srv.PreviewMove(ctx, f.actor, res.TenantScope(f.tenant), targets[i], inputs[i])
		must(t, e)
		inputs[i].PreviewHash = p.Hash
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := range inputs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, e := f.srv.Move(ctx, f.actor, res.TenantScope(f.tenant), targets[i], inputs[i], access.Meta{})
			results <- e
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	ok := 0
	for e := range results {
		if e == nil {
			ok++
		}
	}
	if ok != 1 {
		t.Fatalf("expected one serialized move, got %d", ok)
	}
	for _, target := range targets {
		d, e := f.srv.Detail(ctx, f.actor, res.TenantScope(f.tenant), target)
		must(t, e)
		if len(d.Ancestors) > 2 {
			t.Fatal(d.Ancestors)
		}
	}
}
