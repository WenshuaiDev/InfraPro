package derivedmonitoring_test

import (
	"encoding/json"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/google/uuid"
)

func TestDerivedResourceLifecyclePreservesCalculationHistory(t *testing.T) {
	f := channelsSetup(t)
	actor := f.actor(f.admin)
	v := f.publish(t, f.create(t, "lifecycle").Definition)
	in := dm.CreateChannelInput{WriteMeta: wm(""), Code: "DELTA", Name: "变化", Owner: dm.Owner{Kind: resources.Point, ID: f.point, FacilityID: f.facility}, DefinitionVersionID: v.Version.ID, InputBindings: map[string]dm.InputBinding{"x": {ChannelID: f.direct}}, Parameters: map[string]string{}}
	c, e := f.s.CreateChannel(ctx, actor, f.scope, in, access.Meta{})
	must(t, e)
	target := resources.Target{Kind: resources.Channel, Facility: f.facility, Point: f.point, ID: c.Channel.Resource.ID}
	change := func(action string, revision int) resources.Result {
		t.Helper()
		in := map[string]any{"request_id": uuid.NewString(), "expected_revision": revision, "reason": "资源生命周期验证"}
		raw, e := json.Marshal(in)
		must(t, e)
		preview, e := f.r.PreviewMutation(ctx, actor, resources.TenantScope(f.scope.Tenant), target, action, raw)
		must(t, e)
		if !preview.CanSubmit {
			t.Fatal(preview)
		}
		in["preview_hash"] = preview.Hash
		raw, e = json.Marshal(in)
		must(t, e)
		result, e := f.r.Mutate(ctx, actor, resources.TenantScope(f.scope.Tenant), target, action, raw, access.Meta{})
		must(t, e)
		return result
	}
	r := change("retire", 1)
	current, e := f.s.Channel(ctx, actor, f.scope, target.ID)
	must(t, e)
	if current.ControlState != "retired" || current.ControlRevision != "2" || current.StopNS == nil {
		t.Fatal("resource retirement left calculation enabled", current)
	}
	r = change("restore", r.Revision)
	current, e = f.s.Channel(ctx, actor, f.scope, target.ID)
	must(t, e)
	if current.ControlState != "paused" || current.ControlRevision != "3" || current.MandateRevision != "3" {
		t.Fatal("restore resumed computation", current)
	}
	raw, e := json.Marshal(map[string]any{"request_id": uuid.NewString(), "expected_revision": r.Revision, "reason": "禁止删除固定配置"})
	must(t, e)
	preview, e := f.r.PreviewMutation(ctx, actor, resources.TenantScope(f.scope.Tenant), target, "delete", raw)
	must(t, e)
	if preview.CanSubmit || preview.BlockedCode != "resource_referenced" {
		t.Fatal("non-draft calculation became deletable", preview)
	}
	inputTarget := resources.Target{Kind: resources.Channel, Facility: f.facility, Point: f.point, ID: f.direct}
	raw, e = json.Marshal(map[string]any{"request_id": uuid.NewString(), "expected_revision": 1, "reason": "输入引用保护"})
	must(t, e)
	preview, e = f.r.PreviewMutation(ctx, actor, resources.TenantScope(f.scope.Tenant), inputTarget, "delete", raw)
	must(t, e)
	if preview.CanSubmit || !preview.CalculationReferences {
		t.Fatal("input history not protected", preview)
	}
	// An unused draft can leave the catalog while its reserved identity and
	// initial configuration remain available to audit and reference checks.
	in.Code = "UNUSED"
	in.RequestID = uuid.NewString()
	unused, e := f.s.CreateChannel(ctx, actor, f.scope, in, access.Meta{})
	must(t, e)
	target.ID = unused.Channel.Resource.ID
	change("delete", 1)
	var configs int
	must(t, f.e.Pool.QueryRow(ctx, `SELECT count(*) FROM derived_channel_configs WHERE tenant_id=$1 AND channel_id=$2`, f.scope.Tenant, target.ID).Scan(&configs))
	if configs != 1 {
		t.Fatal("draft deletion erased configuration")
	}
}

func TestDerivedAncestorRetirementStopsAndDoesNotResumeChildren(t *testing.T) {
	f := channelsSetup(t)
	actor := f.actor(f.admin)
	v := f.publish(t, f.create(t, "ancestor_lifecycle").Definition)
	created, e := f.s.CreateChannel(ctx, actor, f.scope, dm.CreateChannelInput{WriteMeta: wm(""), Code: "ANCESTOR_DELTA", Name: "祖先生命周期", Owner: dm.Owner{Kind: resources.Point, ID: f.point, FacilityID: f.facility}, DefinitionVersionID: v.Version.ID, InputBindings: map[string]dm.InputBinding{"x": {ChannelID: f.direct}}, Parameters: map[string]string{}}, access.Meta{})
	must(t, e)
	mutate := func(target resources.Target, action string, revision int) resources.Result {
		t.Helper()
		body := map[string]any{"request_id": uuid.NewString(), "expected_revision": revision, "reason": "祖先生命周期验证"}
		raw, e := json.Marshal(body)
		must(t, e)
		preview, e := f.r.PreviewMutation(ctx, actor, resources.TenantScope(f.scope.Tenant), target, action, raw)
		must(t, e)
		if !preview.CanSubmit {
			t.Fatal(preview)
		}
		body["preview_hash"] = preview.Hash
		raw, e = json.Marshal(body)
		must(t, e)
		result, e := f.r.Mutate(ctx, actor, resources.TenantScope(f.scope.Tenant), target, action, raw, access.Meta{})
		must(t, e)
		return result
	}
	point := resources.Target{Kind: resources.Point, ID: f.point, Facility: f.facility}
	retired := mutate(point, "retire", 1)
	channel, e := f.s.Channel(ctx, actor, f.scope, created.Channel.Resource.ID)
	must(t, e)
	if channel.ControlState != "retired" || channel.StopNS == nil {
		t.Fatal("ancestor retirement did not stop owned calculation", channel)
	}
	mutate(point, "restore", retired.Revision)
	channel, e = f.s.Channel(ctx, actor, f.scope, created.Channel.Resource.ID)
	must(t, e)
	if channel.ControlState != "paused" {
		t.Fatal("ancestor restoration resumed calculation", channel)
	}
}
