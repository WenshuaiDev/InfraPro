//go:build monitoringintegration

package resources_test

import (
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dr "github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	r "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"strings"
	"testing"
	"time"
)

func TestDisplayStructureSeventyFormalObservations(t *testing.T) {
	f := monitoringSetup(t)
	x := structureDisplaySetup(t, f.resourceFixture, runtime.Compose(f.env.Service.Access(), f.storage))
	options := x.b.Displays.Options()
	options.Lease = 15 * time.Minute
	x.b.Displays.WithOptions(options)
	mapping := md.Mapping{Format: "csv", Mode: "long", Encoding: "utf-8", Delimiter: ",", HeaderRow: 1, ChannelColumn: "channel", TimeColumn: "observed_at", ValueColumn: "value", QualityColumn: "quality", Timezone: "UTC", EmptyWide: "error"}
	var data strings.Builder
	data.WriteString("channel,observed_at,value,quality\n")
	base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	for i := 1; i <= 7; i++ {
		code := fmt.Sprintf("P%d", i)
		mapping.Channels = append(mapping.Channels, md.ChannelMapping{Alias: code, Channel: x.channels[code], Unit: "mm"})
		for minute := range 10 {
			fmt.Fprintf(&data, "%s,%s,%d,good\n", code, base.Add(time.Duration(minute)*time.Minute).Format(time.RFC3339), minute)
		}
	}
	f.publish(t, f.validate(t, f.uploadBytes(t, "sbm-seventy.csv", []byte(data.String()), mapping)))
	x.display(t, false)
	rt := dr.Target{View: "resources"}
	query := dr.MonitoringStatisticsRequest{Targets: []dr.Target{{View: "channels"}}, ResourceTarget: &rt, GroupBy: "monitoring_section", Membership: "nearest", SourceKinds: []string{"direct"}, MonitoringTime: dr.MonitoringTime{AllHistory: true}}
	first := awaitDisplayRead(t, x.h, x.b, readCall(t, x.h, "statistics.monitoring", query))
	stats := readData[dr.MonitoringStatistics](t, first)
	if stats.Totals.Raw.Active == nil || *stats.Totals.Raw.Active != "70" || *stats.Totals.Raw.Ever != "70" || *stats.Totals.Raw.Samples != "70" || stats.StructureModel != 2 || stats.Additivity != "disjoint" {
		t.Fatalf("formal 70: %s", dr.Bytes(stats))
	}
	expected := map[string]string{x.nodes["S1"].ID: "20", x.nodes["S1a"].ID: "10", x.nodes["S2"].ID: "10", x.nodes["S3"].ID: "10", x.nodes["S4"].ID: "10", "unassigned": "10"}
	if len(stats.Groups) != 6 {
		t.Fatal(stats.Groups)
	}
	for _, g := range stats.Groups {
		if g.Totals.Raw.Active == nil || *g.Totals.Raw.Active != expected[g.Key] {
			t.Fatalf("nearest: %s", dr.Bytes(g))
		}
	}
	query.Membership = "subtree"
	subtree := readData[dr.MonitoringStatistics](t, awaitDisplayRead(t, x.h, x.b, readCall(t, x.h, "statistics.monitoring", query)))
	if subtree.Additivity != "overlapping" || *subtree.Totals.Raw.Active != "70" {
		t.Fatal(subtree)
	}
	for _, g := range subtree.Groups {
		if g.Key == x.nodes["S1"].ID && *g.Totals.Raw.Active != "30" {
			t.Fatal(g)
		}
	}
	selection := readData[dr.DrilldownSelection](t, readCall(t, x.h, "queries.drilldown", dr.DrilldownRequest{QueryID: first.Query.ID, GroupKey: x.nodes["S1a"].ID, Basis: "raw_active", Purpose: "details"}))
	move := f.moveInput(t, x.nodes["A"], r.Point, x.nodes["P3"].ID, r.StructureRef{Kind: r.MonitoringSection, ID: x.nodes["S1"].ID})
	preview, e := f.srv.PreviewMove(ctx, f.actor, r.TenantScope(f.tenant), target(x.nodes["A"], r.Point, x.nodes["P3"].ID), move)
	must(t, e)
	move.PreviewHash = preview.Hash
	_, e = f.srv.Move(ctx, f.actor, r.TenantScope(f.tenant), target(x.nodes["A"], r.Point, x.nodes["P3"].ID), move, access.Meta{})
	must(t, e)
	details := readData[dr.Page[map[string]any]](t, awaitDisplayRead(t, x.h, x.b, readCall(t, x.h, "monitoring.details", dr.MonitoringRequest{Target: dr.Target{Selection: selection.Selection}})))
	if details.Total != "10" {
		t.Fatal("old query drifted after structure move", details)
	}
	for _, row := range details.Items {
		if row["channel_id"] != x.channels["P3"] {
			t.Fatal(row)
		}
	}
	query.Membership = "nearest"
	current := readData[dr.MonitoringStatistics](t, awaitDisplayRead(t, x.h, x.b, readCall(t, x.h, "statistics.monitoring", query)))
	for _, g := range current.Groups {
		if g.Key == x.nodes["S1"].ID && *g.Totals.Raw.Active != "30" {
			t.Fatal("new monitoring grouping stale", g)
		}
		if g.Key == x.nodes["S1a"].ID && *g.Totals.Raw.Active != "0" {
			t.Fatal(g)
		}
	}
}
