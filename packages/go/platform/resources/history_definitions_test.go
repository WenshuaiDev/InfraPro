package resources_test

import (
	"encoding/json"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	eng "github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
)

func TestDeletedChannelHistoryRetainsAuthorizedFixedDefinitions(t *testing.T) {
	f := resourcesSetup(t)
	facility := f.facility(t, "HISTORY")
	point, e := f.write(t, target(facility, res.Point, ""), "create", map[string]any{"code": "P", "name": "测点"})
	must(t, e)
	content := simple("历史状态指标")
	content.Metric = &eng.Metric{Shape: "enum", Options: []eng.Option{{Key: "ok", Name: "稳定"}, {Key: "fail", Name: "异常"}}}
	published := f.publish(t, eng.MetricDefinition, f.create(t, eng.MetricDefinition, "history_enum", content))
	ct := target(facility, res.Channel, "")
	ct.Point, ct.Tenant = point.ID, f.tenant
	channel, e := f.write(t, ct, "create", map[string]any{"code": "CH", "name": "删除的状态通道", "metric_version_id": published.VersionID, "timestamp_meaning": "observation_time"})
	must(t, e)
	ct.ID = channel.ID
	reader, _, _ := f.reader(t)
	_, e = f.write(t, ct, "delete", map[string]any{"expected_revision": 1, "reason": "误建"})
	must(t, e)
	_, e = f.srv.Detail(ctx, reader, ct)
	code(t, e, "resource_not_found")
	ft := target(facility, res.Facility, facility.ID)
	ft.Tenant = f.tenant
	history, e := f.srv.Changes(ctx, reader, ft, res.Filter{Kind: "channel", ID: channel.ID})
	must(t, e)
	if len(history.Items) != 2 {
		t.Fatalf("want create/delete events, got %d", len(history.Items))
	}
	// Check the serialized public response, including readers without catalog access.
	payload, e := json.Marshal(history)
	must(t, e)
	var result struct {
		Definitions []eng.Version `json:"definitions"`
	}
	must(t, json.Unmarshal(payload, &result))
	versions := map[string]eng.Version{}
	for _, v := range result.Definitions {
		versions[v.ID] = v
	}
	metric := versions[published.VersionID]
	if metric.Content.Name != content.Name || metric.Content.Metric == nil || metric.Content.Metric.Options[0].Name != "稳定" || versions[f.typ].ID == "" {
		t.Fatalf("missing fixed historical interpretation: %s", payload)
	}
	if len(versions) != len(result.Definitions) {
		t.Fatal("duplicate definitions")
	}
	_, e = f.s.Detail(ctx, reader, eng.Scope{Tenant: f.tenant, Catalog: true, Kind: eng.MetricDefinition}, published.DefinitionID)
	code(t, e, "permission_denied")
	_, e = f.srv.Changes(ctx, f.fixture.actor(f.p), ft, res.Filter{})
	code(t, e, "tenant_not_found")
	empty, e := f.srv.Changes(ctx, reader, ft, res.Filter{Kind: "channel", ID: f.wm(t).RequestID})
	must(t, e)
	payload, e = json.Marshal(empty)
	must(t, e)
	must(t, json.Unmarshal(payload, &result))
	if len(result.Definitions) != 0 {
		t.Fatal("unmatched history leaked definitions")
	}
	// Keep authorization enforced after a current membership is suspended.
	_, e = f.env.Pool.Exec(ctx, `UPDATE tenant_memberships SET status='suspended' WHERE tenant_id=$1 AND account_id=$2`, f.tenant, reader.ID)
	must(t, e)
	_, e = f.srv.Changes(ctx, reader, ft, res.Filter{})
	code(t, e, "tenant_not_found")
}

func TestDeletedFacilityHistoryRetainsDisabledMetricUnit(t *testing.T) {
	f := resourcesSetup(t)
	facility := f.facility(t, "HISTORY.UNIT")
	point, e := f.write(t, target(facility, res.Point, ""), "create", map[string]any{"code": "P", "name": "测点"})
	must(t, e)
	ct := target(facility, res.Channel, "")
	ct.Point, ct.Tenant = point.ID, f.tenant
	channel, e := f.write(t, ct, "create", map[string]any{"code": "CH", "name": "数值通道", "metric_version_id": f.metric, "unit_version_id": f.unit, "timestamp_meaning": "observation_time"})
	must(t, e)
	ct.ID = channel.ID
	detail, e := f.srv.Detail(ctx, f.actor, ct)
	must(t, e)
	for _, v := range detail.Definitions {
		if v.ID == f.unit {
			_, e = f.s.SetState(ctx, f.fixture.actor(f.p), eng.Scope{Kind: eng.UnitDefinition}, v.DefinitionID, v.ID, eng.StateInput{WriteMeta: f.wm(t), Revision: v.StateRevision, Enabled: false}, access.Meta{})
			must(t, e)
		}
	}
	for _, target := range []res.Target{ct, target(facility, res.Point, point.ID), target(facility, res.Facility, facility.ID)} {
		_, e = f.write(t, target, "delete", map[string]any{"expected_revision": 1, "reason": "删除后保留历史解释"})
		must(t, e)
	}
	ft := target(facility, res.Facility, facility.ID)
	ft.Tenant = f.tenant
	reader, _, _ := f.reader(t)
	history, e := f.srv.Changes(ctx, reader, ft, res.Filter{Kind: "channel", ID: channel.ID, Limit: 1})
	must(t, e)
	if len(history.Items) != 1 || history.NextCursor == "" || len(history.Definitions) != 3 {
		t.Fatal("expected paginated history with type, metric and unit", history)
	}
	for _, v := range history.Definitions {
		if v.ID == f.unit && (v.Enabled || v.Content.Unit == nil || v.Content.Unit.Symbol != "mm") {
			t.Fatal("historical disabled unit lost its meaning", v)
		}
	}
	filtered, e := f.srv.Changes(ctx, reader, ft, res.Filter{Kind: "point", ID: point.ID})
	must(t, e)
	if len(filtered.Definitions) != 1 || filtered.Definitions[0].ID != f.typ {
		t.Fatal("definitions escaped the authorized history filter", filtered.Definitions)
	}
}
