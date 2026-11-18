package resources_test

import (
	"errors"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	eng "github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
)

func TestChannelPinsOneOfMetricVersionUnits(t *testing.T) {
	f := resourcesSetup(t)
	admin := f.fixture.actor(f.p)
	scope := eng.Scope{Kind: eng.MetricDefinition}
	unit := func(code, name string) string {
		c := simple(name)
		c.Unit = &eng.Unit{Category: "length", Symbol: code}
		return f.publish(t, eng.UnitDefinition, f.create(t, eng.UnitDefinition, code, c)).VersionID
	}
	cm, meter := unit("cm", "厘米"), unit("m", "米")
	c := simple("位移")
	c.Metric = &eng.Metric{Shape: "decimal", Category: "length", StandardUnit: f.unit, AllowedUnits: []string{f.unit, cm}}
	v1 := f.publish(t, scope.Kind, f.create(t, scope.Kind, "multiple_units", c))
	facility := f.facility(t, "F")
	point, err := f.write(t, target(facility, res.Point, ""), "create", map[string]any{"code": "P", "name": "测点"})
	must(t, err)
	channel := target(facility, res.Channel, "")
	channel.Point = point.ID
	create := func(code, metric, unit string) (res.Result, error) {
		return f.write(t, channel, "create", map[string]any{"code": code, "name": code, "metric_version_id": metric, "unit_version_id": unit, "timestamp_meaning": "observation_time"})
	}
	rejected := func(err error) {
		t.Helper()
		var failure *res.Error
		if !errors.As(err, &failure) || len(failure.Issues) == 0 || failure.Issues[0].Code != "metric_unit_mismatch" {
			t.Fatalf("expected unit mismatch, got %v", err)
		}
	}
	_, err = create("MM", v1.VersionID, f.unit)
	must(t, err)
	history, err := create("CM", v1.VersionID, cm)
	must(t, err)
	_, err = create("INVALID", v1.VersionID, meter)
	rejected(err)
	draft, err := f.s.NewDraft(ctx, admin, scope, v1.DefinitionID, eng.NewDraftInput{RevisionInput: eng.RevisionInput{WriteMeta: f.wm(t), Revision: v1.Revision}, VersionID: v1.VersionID}, access.Meta{})
	must(t, err)
	c.Metric.AllowedUnits = []string{f.unit, meter}
	saved, err := f.s.Save(ctx, admin, scope, v1.DefinitionID, eng.SaveInput{WriteMeta: f.wm(t), Revision: draft.Revision, Content: c}, access.Meta{})
	must(t, err)
	v2 := f.publish(t, scope.Kind, saved)
	_, err = create("NEW-CM", v2.VersionID, cm)
	rejected(err)
	_, err = create("NEW-M", v2.VersionID, meter)
	must(t, err)
	channel.ID = history.ID
	current, err := f.srv.Detail(ctx, f.actor, res.TenantScope(f.tenant), channel)
	must(t, err)
	if current.MetricVersionID != v1.VersionID || current.UnitVersionID == nil || *current.UnitVersionID != cm {
		t.Fatal("publishing a new metric reinterpreted a historical channel")
	}
}
