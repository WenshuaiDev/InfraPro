package derivedmonitoring_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	eng "github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

type resourceFixture struct {
	fixture
	r                                  *resources.Service
	facility, component, point, direct string
}

func (f resourceFixture) write(t *testing.T, target resources.Target, input map[string]any) resources.Result {
	t.Helper()
	input["request_id"] = uuid.NewString()
	raw, e := json.Marshal(input)
	must(t, e)
	v, e := f.r.Mutate(ctx, f.actor(f.admin), resources.TenantScope(f.scope.Tenant), target, "create", raw, access.Meta{})
	must(t, e)
	return v
}
func channelsSetup(t *testing.T) resourceFixture {
	t.Helper()
	f := resourceFixture{fixture: setup(t)}
	f.r = resources.New(f.e.Service.Access())
	f.s.WithResources(f.r)
	f.r.WithExternalReferences(f.s)
	publish := func(kind eng.Kind, code string, content eng.Content) eng.Result {
		t.Helper()
		v, e := f.engineering.Create(ctx, f.actor(f.platform), eng.Scope{Kind: kind}, eng.CreateInput{WriteMeta: ewm(), Code: code, Content: content}, access.Meta{})
		must(t, e)
		v, e = f.engineering.Publish(ctx, f.actor(f.platform), eng.Scope{Kind: kind}, v.DefinitionID, eng.RevisionInput{WriteMeta: ewm(), Revision: v.Revision}, access.Meta{})
		must(t, e)
		return v
	}
	yes := true
	typ := publish(eng.TypeDefinition, "bridge", eng.Content{MetamodelVersion: "2", Structure: &eng.Structure{MonitoringSection: &eng.StructureCapability{Supported: &yes, Fields: []eng.Field{}}, Borehole: &eng.StructureCapability{Supported: &yes, Fields: []eng.Field{}}}, Name: "桥梁", Components: []eng.Component{{Key: "span", Name: "桥跨"}}, Relations: []eng.Relation{{Parent: "root", Child: "span", MinCount: 0}}})
	metric := publish(eng.MetricDefinition, "displacement", eng.Content{MetamodelVersion: "1", Name: "位移", Metric: &eng.Metric{Shape: "decimal", Category: "length", StandardUnit: f.unit.VersionID, AllowedUnits: []string{f.unit.VersionID}}})
	f.facility = f.write(t, resources.Target{Kind: resources.Facility}, map[string]any{"code": "F", "name": "设施", "type_version_id": typ.VersionID, "values": map[string]any{}}).ID
	f.component = f.write(t, resources.Target{Kind: resources.Component, Facility: f.facility}, map[string]any{"code": "C", "name": "构件", "category_key": "span", "expected_structure_revision": 1, "values": map[string]any{}}).ID
	f.point = f.write(t, resources.Target{Kind: resources.Point, Facility: f.facility}, map[string]any{"code": "P", "name": "测点", "component_id": f.component}).ID
	f.direct = f.write(t, resources.Target{Kind: resources.Channel, Facility: f.facility, Point: f.point}, map[string]any{"code": "DIRECT", "name": "原始位移", "metric_version_id": metric.VersionID, "unit_version_id": f.unit.VersionID, "timestamp_meaning": "observation_time", "observation_direction": "up", "reference_basis": "fixed"}).ID
	return f
}
func TestDerivedChannelOwnershipAtomicityAndInputContracts(t *testing.T) {
	f := channelsSetup(t)
	actor := f.actor(f.admin)
	definition := f.publish(t, f.create(t, "difference").Definition)
	containers := []dm.Owner{}
	for _, kind := range []resources.Kind{resources.MonitoringSection, resources.Borehole} {
		root, e := f.r.Detail(ctx, actor, resources.TenantScope(f.scope.Tenant), resources.Target{Kind: resources.Facility, Facility: f.facility, ID: f.facility})
		must(t, e)
		node := f.write(t, resources.Target{Kind: kind, Facility: f.facility}, map[string]any{"code": string(kind), "name": string(kind), "expected_structure_revision": root.StructureRevision})
		containers = append(containers, dm.Owner{Kind: kind, ID: node.ID, FacilityID: f.facility})
	}
	for _, owner := range append(containers, []dm.Owner{{Kind: resources.Facility, ID: f.facility, FacilityID: f.facility}, {Kind: resources.Component, ID: f.component, FacilityID: f.facility}, {Kind: resources.Point, ID: f.point, FacilityID: f.facility}}...) {
		parentDetail, e := f.r.Detail(ctx, actor, resources.TenantScope(f.scope.Tenant), resources.Target{Kind: owner.Kind, Facility: f.facility, ID: owner.ID})
		must(t, e)
		if !parentDetail.Actions.CreateDerivedChannel {
			t.Fatal("authorized owner hides derived creation", parentDetail.Actions)
		}
		in := dm.CreateChannelInput{WriteMeta: wm(""), Owner: owner, Code: "DELTA", Name: "派生变化", DefinitionVersionID: definition.Version.ID, InputBindings: map[string]dm.InputBinding{"x": {ChannelID: f.direct}}, Parameters: map[string]string{}}
		v, e := f.s.CreateChannel(ctx, actor, f.scope, in, access.Meta{})
		must(t, e)
		if v.Channel.ControlState != "draft" || v.Channel.Resource.SourceKind != "derived" || v.Channel.Resource.OwnerKind != string(owner.Kind) || v.Channel.Resource.MetricVersionID != "" || v.Channel.Resource.UnitVersionID != nil {
			t.Fatal(v)
		}
		replay, e := f.s.CreateChannel(ctx, actor, f.scope, in, access.Meta{})
		must(t, e)
		if !replay.Replayed || replay.Channel.Resource.ID != v.Channel.Resource.ID {
			t.Fatal("channel replay changed identity")
		}
		resource, e := f.r.Detail(ctx, actor, resources.TenantScope(f.scope.Tenant), resources.Target{Kind: resources.Channel, Facility: f.facility, ID: v.Channel.Resource.ID})
		must(t, e)
		if resource.SourceKind != "derived" || len(resource.Ancestors) < 1 || !resource.Actions.ManageCalculation {
			t.Fatal("derived resource lost owner ancestry", resource)
		}
		var parent string
		must(t, f.e.Pool.QueryRow(ctx, `SELECT parent_id::text FROM engineering_resource_nodes WHERE tenant_id=$1 AND kind='channel' AND id=$2`, f.scope.Tenant, resource.ID).Scan(&parent))
		if parent != owner.ID {
			t.Fatal("wrong tree edge", parent)
		}
		page, e := f.s.ChannelConfigs(ctx, actor, f.scope, resource.ID, dm.MetricFilter{})
		must(t, e)
		if len(page.Items) != 1 || page.Items[0].InputBindings["x"].ChannelID != f.direct || page.Items[0].InputsRestricted {
			t.Fatal("configuration missing binding", page)
		}
		directory, e := md.New(f.e.Service.Access()).ChannelDirectory(ctx, actor, md.TenantScope(f.scope.Tenant), md.ChannelNavigationQuery{OwnerKind: string(owner.Kind), OwnerID: owner.ID, SourceKind: "derived", Search: "派生"})
		must(t, e)
		if len(directory.Items) != 1 || directory.Items[0].ID != resource.ID || directory.Items[0].OwnerID != owner.ID || directory.Items[0].Metric != nil || directory.Items[0].Unit != "mm" || directory.Items[0].OutputContract == "" {
			t.Fatal("derived owner directory", directory)
		}
		if (directory.Items[0].Point != nil) != (owner.Kind == resources.Point) {
			t.Fatal("fabricated point for owner", directory)
		}
		owned, e := f.s.Channels(ctx, actor, f.scope, dm.ChannelFilter{OwnerKind: string(owner.Kind), OwnerID: owner.ID, FacilityID: f.facility})
		must(t, e)
		if len(owned.Items) != 1 || owned.Items[0].Resource.ID != resource.ID || owned.Items[0].OwnerName == "" || owned.Items[0].FacilityName != "设施" {
			t.Fatal("owner-scoped management directory", owned)
		}
		deps, e := f.s.ChannelDependencies(ctx, actor, f.scope, resource.ID)
		must(t, e)
		if len(deps.Items) != 1 || deps.Items[0].SourceKind != "direct" {
			t.Fatal(deps)
		}
		_, e = f.e.Pool.Exec(ctx, `UPDATE derived_channel_configs SET parameters='{}' WHERE id=$1`, page.Items[0].ID)
		var pe *pgconn.PgError
		if !errors.As(e, &pe) || pe.Code != "23514" {
			t.Fatal("mutable execution config", e)
		}
		// Device heads cannot target a point-owned derived channel even though the legacy composite FK exists.
		if owner.Kind == resources.Point {
			_, e = f.e.Pool.Exec(ctx, `INSERT INTO monitoring_source_heads(tenant_id,facility_id,point_id,channel_id) VALUES($1,$2,$3,$4)`, f.scope.Tenant, f.facility, f.point, resource.ID)
			if !errors.As(e, &pe) || pe.Code != "23514" {
				t.Fatal("derived channel accepted device source", e)
			}
			// Same point shares the direct/derived code namespace.
			in.RequestID = uuid.NewString()
			in.Code = "DIRECT"
			_, e = f.s.CreateChannel(ctx, actor, f.scope, in, access.Meta{})
			code(t, e, "definition_conflict")
		}
	}
	page, e := f.s.Channels(ctx, actor, f.scope, dm.ChannelFilter{Limit: 2})
	must(t, e)
	if len(page.Items) != 2 || page.NextCursor == "" {
		t.Fatal(page)
	}
	next, e := f.s.Channels(ctx, actor, f.scope, dm.ChannelFilter{Limit: 2, Cursor: page.NextCursor})
	must(t, e)
	if len(next.Items) != 2 || next.NextCursor == "" {
		t.Fatal(next)
	}
	last, e := f.s.Channels(ctx, actor, f.scope, dm.ChannelFilter{Limit: 2, Cursor: next.NextCursor})
	must(t, e)
	if len(last.Items) != 1 || last.NextCursor != "" {
		t.Fatal(last)
	}
	_, e = f.e.Pool.Exec(ctx, `INSERT INTO engineering_channels(tenant_id,code,name,created_by,updated_by,facility_id,source_kind,owner_kind,derived_metric_version_id,quantity_spec,output_contract_hash,output_semantics,timestamp_meaning) SELECT tenant_id,'incomplete','不完整',created_by,updated_by,facility_id,source_kind,owner_kind,derived_metric_version_id,quantity_spec,output_contract_hash,output_semantics,timestamp_meaning FROM engineering_channels WHERE tenant_id=$1 AND owner_kind='facility' LIMIT 1`, f.scope.Tenant)
	var pe *pgconn.PgError
	if !errors.As(e, &pe) || pe.Code != "23514" {
		t.Fatal("partial resource committed without calculation extension", e)
	}
	var quantity dm.Quantity
	must(t, json.Unmarshal(page.Items[0].Resource.QuantitySpec, &quantity))
	if quantity.Mode != "difference" || quantity.BaseUnitVersionID != f.unit.VersionID {
		t.Fatal(quantity)
	}
	// The incremental migration and derived creation leave direct resources usable.
	raw, e := f.r.Detail(ctx, actor, resources.TenantScope(f.scope.Tenant), resources.Target{Kind: resources.Channel, Facility: f.facility, Point: f.point, ID: f.direct})
	must(t, e)
	if raw.SourceKind != "direct" || raw.OwnerKind != "point" || raw.MetricVersionID == "" {
		t.Fatal(raw)
	}
}
func TestDerivedChannelDepthLimitRollsBackResource(t *testing.T) {
	f := channelsSetup(t)
	actor := f.actor(f.admin)
	var rule dm.Rule
	must(t, json.Unmarshal(f.rule, &rule))
	rule.Nodes = []dm.Node{{ID: "result", Op: "abs", Inputs: []string{"input:x"}}}
	raw, e := json.Marshal(rule)
	must(t, e)
	d, e := f.s.CreateMetric(ctx, actor, f.scope, dm.CreateMetricInput{WriteMeta: wm(""), Code: "absolute", Name: "绝对值", Rule: raw}, access.Meta{})
	must(t, e)
	v := f.publish(t, d.Definition)
	input := f.direct
	for depth := 1; depth <= 9; depth++ {
		in := dm.CreateChannelInput{WriteMeta: wm(""), Code: fmt.Sprintf("DEPTH%d", depth), Name: "深度验证", Owner: dm.Owner{Kind: resources.Point, ID: f.point, FacilityID: f.facility}, DefinitionVersionID: v.Version.ID, InputBindings: map[string]dm.InputBinding{"x": {ChannelID: input}}, Parameters: map[string]string{}}
		result, e := f.s.CreateChannel(ctx, actor, f.scope, in, access.Meta{})
		if depth == 9 {
			code(t, e, "dependency_depth_exceeded")
		} else {
			must(t, e)
			input = result.Channel.Resource.ID
		}
	}
	var count int
	must(t, f.e.Pool.QueryRow(ctx, `SELECT count(*) FROM engineering_channels WHERE tenant_id=$1 AND code='DEPTH9'`, f.scope.Tenant).Scan(&count))
	if count != 0 {
		t.Fatal("failed graph left a resource")
	}
}
