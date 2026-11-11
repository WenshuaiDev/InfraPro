package resources_test

import (
	"context"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/projects"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/jackc/pgx/v5"
	"testing"
	"time"
)

// Actor.Check runs only after the authorization lock is acquired. This barrier
// fixes transaction ordering without relying on which goroutine happens to run first.
func gateActor(a access.Actor) (access.Actor, <-chan struct{}, chan<- struct{}) {
	entered, release := make(chan struct{}), make(chan struct{})
	original := a.Check
	a.Check = func(c context.Context, tx pgx.Tx, recent bool) error {
		if e := original(c, tx, recent); e != nil {
			return e
		}
		close(entered)
		<-release
		return nil
	}
	return a, entered, release
}
func TestRevocationAndProjectWriteBothCommitOrders(t *testing.T) {
	for _, first := range []string{"revoke", "write"} {
		t.Run(first, func(t *testing.T) {
			f := resourcesSetup(t)
			p := f.project(t, "ProjectA")
			fac := f.facility(t, "RACE")
			pt, e := f.write(t, target(fac, res.Point, ""), "create", map[string]any{"code": "P", "name": "原名称"})
			must(t, e)
			in := f.accessInput(t, p, fac.ID, "create_link")
			in.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}, {Kind: res.Point, PointID: &pt.ID, Capabilities: []string{"read", "update"}}}
			f.grant(t, p, fac.ID, in)
			revoke := f.accessInput(t, p, fac.ID, "revoke_link")
			preview, e := f.srv.PreviewAccessChange(ctx, f.actor, f.tenant, p.id, fac.ID, revoke)
			must(t, e)
			revoke.PreviewHash = preview.Hash
			raw, _ := json.Marshal(f.projectIntent(t, p, target(fac, res.Point, pt.ID), map[string]any{"expected_revision": 1, "name": "已提交名称", "purpose": "并发更新用途"}))
			firstDone, secondDone := make(chan error, 1), make(chan error, 1)
			tenant, writer := f.actor, p.actor
			var entered <-chan struct{}
			var release chan<- struct{}
			if first == "revoke" {
				tenant, entered, release = gateActor(tenant)
			} else {
				writer, entered, release = gateActor(writer)
			}
			change := func() error {
				_, e := f.srv.ChangeAccess(ctx, tenant, f.tenant, p.id, fac.ID, revoke, access.Meta{})
				return e
			}
			write := func() error {
				_, e := f.srv.Mutate(ctx, writer, res.ProjectScope(f.tenant, p.id), target(fac, res.Point, pt.ID), "update", raw, access.Meta{})
				return e
			}
			one, two := change, write
			if first == "write" {
				one, two = write, change
			}
			go func() { firstDone <- one() }()
			<-entered
			go func() { secondDone <- two() }()
			select {
			case e := <-secondDone:
				t.Fatalf("second transaction bypassed lock: %v", e)
			case <-time.After(30 * time.Millisecond):
			}
			close(release)
			must(t, <-firstDone)
			if first == "revoke" {
				code(t, <-secondDone, "resource_not_found")
			} else {
				code(t, <-secondDone, "preview_changed")
			}
			d, e := f.srv.Detail(ctx, f.actor, res.TenantScope(f.tenant), target(fac, res.Point, pt.ID))
			must(t, e)
			if (d.Name == "已提交名称") != (first == "write") {
				t.Fatal(d.Name)
			}
		})
	}
}
func TestArchiveAndProjectCreateBothCommitOrders(t *testing.T) {
	for _, first := range []string{"archive", "create"} {
		t.Run(first, func(t *testing.T) {
			f := resourcesSetup(t)
			p := f.project(t, "ProjectA")
			fac := f.facility(t, "ARCHIVE")
			in := f.accessInput(t, p, fac.ID, "create_link")
			in.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read", "create_point"}}}
			f.grant(t, p, fac.ID, in)
			ps := projects.New(f.env.Service.Access())
			pr, e := ps.Preview(ctx, p.actor, f.tenant, p.id, "", access.OpProjectArchive, projects.Change{})
			must(t, e)
			archive := projects.Change{ChangeMeta: projects.ChangeMeta{WriteMeta: projects.WriteMeta{RequestID: f.wm(t).RequestID, Reason: "归档竞争"}, Version: pr.Version, ProjectVersion: pr.ProjectVersion, Hash: pr.Hash}}
			raw, _ := json.Marshal(f.projectIntent(t, p, target(fac, res.Point, ""), map[string]any{"code": "NEW", "name": "归档前创建"}))
			archiver, creator := p.actor, p.actor
			var entered <-chan struct{}
			var release chan<- struct{}
			if first == "archive" {
				archiver, entered, release = gateActor(archiver)
			} else {
				creator, entered, release = gateActor(creator)
			}
			ar := func() error {
				_, e := ps.Change(ctx, archiver, f.tenant, p.id, "", access.OpProjectArchive, archive, access.Meta{})
				return e
			}
			cr := func() error {
				_, e := f.srv.Mutate(ctx, creator, res.ProjectScope(f.tenant, p.id), target(fac, res.Point, ""), "create", raw, access.Meta{})
				return e
			}
			one, two := ar, cr
			if first == "create" {
				one, two = cr, ar
			}
			x, y := make(chan error, 1), make(chan error, 1)
			go func() { x <- one() }()
			<-entered
			go func() { y <- two() }()
			select {
			case e := <-y:
				t.Fatal("lock bypass", e)
			case <-time.After(30 * time.Millisecond):
			}
			close(release)
			must(t, <-x)
			if first == "archive" {
				code(t, <-y, "project_archived")
			} else {
				must(t, <-y)
			}
			page, e := f.srv.List(ctx, p.actor, res.ProjectScope(f.tenant, p.id), target(fac, res.Point, ""), res.Filter{})
			must(t, e)
			want := 0
			if first == "create" {
				want = 1
			}
			if len(page.Items) != want {
				t.Fatal(page)
			}
		})
	}
}
func TestDeleteAndNewReferenceBothCommitOrders(t *testing.T) {
	for _, first := range []string{"delete", "grant"} {
		t.Run(first, func(t *testing.T) {
			f := resourcesSetup(t)
			p := f.project(t, "ProjectA")
			fac := f.facility(t, "DELETE")
			target := target(fac, res.Facility, fac.ID)
			in := f.accessInput(t, p, fac.ID, "create_link")
			in.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}}
			pr, e := f.srv.PreviewAccessChange(ctx, f.actor, f.tenant, p.id, fac.ID, in)
			must(t, e)
			in.PreviewHash = pr.Hash
			fields := map[string]any{"request_id": f.wm(t).RequestID, "expected_revision": 1, "reason": "删除与新授权竞争"}
			raw, _ := json.Marshal(fields)
			dp, e := f.srv.PreviewMutation(ctx, f.actor, res.TenantScope(f.tenant), target, "delete", raw)
			must(t, e)
			fields["preview_hash"] = dp.Hash
			raw, _ = json.Marshal(fields)
			deleter, granter := f.actor, f.actor
			var entered <-chan struct{}
			var release chan<- struct{}
			if first == "delete" {
				deleter, entered, release = gateActor(deleter)
			} else {
				granter, entered, release = gateActor(granter)
			}
			del := func() error {
				_, e := f.srv.Mutate(ctx, deleter, res.TenantScope(f.tenant), target, "delete", raw, access.Meta{})
				return e
			}
			grant := func() error {
				_, e := f.srv.ChangeAccess(ctx, granter, f.tenant, p.id, fac.ID, in, access.Meta{})
				return e
			}
			one, two := del, grant
			if first == "grant" {
				one, two = grant, del
			}
			x, y := make(chan error, 1), make(chan error, 1)
			go func() { x <- one() }()
			<-entered
			go func() { y <- two() }()
			select {
			case e := <-y:
				t.Fatal("lock bypass", e)
			case <-time.After(30 * time.Millisecond):
			}
			close(release)
			must(t, <-x)
			want := "resource_not_found"
			if first == "grant" {
				want = "resource_referenced"
			}
			code(t, <-y, want)
		})
	}
}
