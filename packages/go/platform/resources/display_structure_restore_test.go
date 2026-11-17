//go:build monitoringintegration

package resources_test

import (
	dr "github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	r "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"testing"
)

func prepareStructureRestore(t *testing.T, f monitoringFixture, p projectFixture) {
	t.Helper()
	b := runtime.Compose(f.env.Service.Access(), f.storage)
	x := structureDisplaySetup(t, f.resourceFixture, b, p)
	x.addDevices(t)
	position := f.recordInput(t, x.nodes["A"], r.Point, x.nodes["P1"].ID, "position")
	position.Position = &r.BoreholePosition{BoreholeID: x.nodes["H1"].ID, BoreholeRevision: 1, DistanceM: ptr(0.0), ReferenceStart: "恢复验证孔口", Direction: "沿孔", Unit: "m"}
	f.saveRecord(t, x.nodes["A"], r.Point, x.nodes["P1"].ID, position)
	ref := f.recordInput(t, x.nodes["A"], r.Point, x.nodes["P1"].ID, "reference")
	ref.Target = &r.StructureRef{Kind: r.MonitoringSection, ID: x.nodes["S1"].ID}
	ref.Purpose = "恢复保留独立参照"
	f.saveRecord(t, x.nodes["A"], r.Point, x.nodes["P1"].ID, ref)
}

func verifyStructureRestoreRead(t *testing.T, h *displayHarness) {
	t.Helper()
	s := readData[dr.StructureStatistics](t, readCall(t, h, "statistics.structure", dr.StructureStatisticsRequest{Target: dr.Target{View: "r"}, GroupBy: "none", Membership: "nearest"}))
	if s.Totals.MonitoringSectionCount != "5" || s.Totals.BoreholeCount != "4" {
		t.Fatal("restored canonical structure differs", s)
	}
	d := readData[dr.DeviceCategoryStatistics](t, readCall(t, h, "statistics.deviceCategories", dr.DeviceCategoryStatisticsRequest{Target: dr.Target{View: "d"}, Population: "authorized", Mode: "direct"}))
	if d.Totals.UniqueDeviceCount != "6" || d.Totals.SensorDeviceCount != "4" || d.Totals.UsedSensorCategoryCount != "3" {
		t.Fatal("restored fixed category basis differs", d)
	}
}
