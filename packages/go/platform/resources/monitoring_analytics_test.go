//go:build monitoringintegration

package resources_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	eng "github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
)

func (f monitoringFixture) analyticAs(t *testing.T, actor access.Actor, scope md.Scope, action string, in md.QueryInput) md.AnalyticResult {
	t.Helper()
	for n := 0; n < 10; n++ {
		out, e := f.service.Analytics(ctx, actor, scope, action, in)
		must(t, e)
		if out.State == "ready" {
			return out
		}
		if out.State != "pending" {
			t.Fatal("analytic task failed", out)
		}
		in.Snapshot = out.Snapshot.ID
		worked, e := f.service.RunOne(ctx, "analytic-test-worker")
		must(t, e)
		if !worked {
			t.Fatal("no worker for pending analytic")
		}
	}
	t.Fatal("analytic did not converge")
	return md.AnalyticResult{}
}
func (f monitoringFixture) analytic(t *testing.T, action string, in md.QueryInput) md.AnalyticResult {
	return f.analyticAs(t, f.actor, f.scope, action, in)
}
func TestMonitoringSummaryCountsSnapshotsAndRevoke(t *testing.T) {
	f := monitoringSetup(t)
	a := f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-01T00:00:00Z,2,good\n2024-01-02T00:00:00Z,4,unmarked\n2024-01-03T00:00:00Z,6,suspect\n2024-01-04T00:00:00Z,100,bad\n")))
	q := f.query(t)
	q.Qualities = nil
	q.Limit = 200
	stats := f.analytic(t, "statistics", q)
	q.Snapshot = stats.Snapshot.ID
	if len(stats.Channels) != 1 {
		t.Fatal(stats)
	}
	v := stats.Channels[0]
	if v.Ever != "4" || v.Active != "4" || v.Samples != "2" || v.Mean == nil || *v.Mean != "3.000000000000" || *v.Stddev != "1.000000000000" || v.Last.At != "2024-01-02T00:00:00Z" || *v.Latest != "2024-01-04T00:00:00Z" {
		t.Fatal(v)
	}
	o := f.analytic(t, "overview", q)
	if o.Overview.Ever != "4" || o.Overview.Samples != "2" {
		t.Fatal(o)
	}
	f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2024-06-01T00:00:00Z,8,good\n")))
	old := f.analytic(t, "overview", q)
	if old.Overview.Active != "4" {
		t.Fatal("append moved old summary", old)
	}
	fresh := f.query(t)
	fresh.Qualities = nil
	current := f.analytic(t, "overview", fresh)
	if current.Overview.Active != "5" || current.Overview.Samples != "3" {
		t.Fatal(current)
	}
	preview, e := f.service.PreviewRevokeBatch(ctx, f.actor, f.scope, a.ID, a.Revision)
	must(t, e)
	_, e = f.service.RevokeBatch(ctx, f.actor, f.scope, a.ID, md.RevokeInput{WriteMeta: f.meta(a.Revision), PreviewHash: preview.Hash})
	must(t, e)
	_, e = f.service.Analytics(ctx, f.actor, f.scope, "overview", q)
	monitoringCode(t, e, "snapshot_stale")
	after := f.analytic(t, "statistics", fresh)
	v = after.Channels[0]
	if v.Ever != "5" || v.Active != "1" || v.Samples != "1" || *v.Mean != "8.000000000000" || *v.Stddev != "0.000000000000" {
		t.Fatal("revocation summary", v)
	}
	var heads, dirty int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM monitoring_summary_heads WHERE tenant_id=$1`, f.tenant).Scan(&heads))
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM monitoring_dirty_ranges WHERE tenant_id=$1`, f.tenant).Scan(&dirty))
	if heads != 1 || dirty != 0 {
		t.Fatal(heads, dirty)
	}
}
func TestMonitoringExactSummaryCalendarAndTrend(t *testing.T) {
	f := monitoringSetup(t)
	f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2024-03-10T06:59:59.999999999Z,999999999999999999999999.000000000001,good\n2024-03-10T07:00:00Z,999999999999999999999999.000000000003,good\n2024-03-11T03:59:59.999999999Z,999999999999999999999999.000000000001,unmarked\n2024-03-11T04:00:00Z,999999999999999999999999.000000000003,good\n")))
	q := f.query(t)
	q.Limit = 200
	q.Timezone = "America/New_York"
	q.Period = "day"
	stats := f.analytic(t, "statistics", q)
	if len(stats.Groups) != 2 {
		t.Fatal(stats)
	}
	g := stats.Groups[0]
	if g.Samples != "3" || g.Start != "2024-03-10T00:00:00-05:00" || g.End != "2024-03-11T00:00:00-04:00" || g.MinCount != "2" || *g.Mean != "999999999999999999999999.000000000002" {
		t.Fatal(g)
	}
	start, _ := time.Parse(time.RFC3339, g.Start)
	end, _ := time.Parse(time.RFC3339, g.End)
	if end.Sub(start) != 23*time.Hour {
		t.Fatal(g)
	}
	q.Period = ""
	trends := f.analytic(t, "trends", q)
	if len(trends.Trends) != 1 || trends.Trends[0].Summary || len(trends.Trends[0].Buckets[0].Points) != 4 {
		t.Fatal(trends)
	}
	q.Qualities = []string{"bad"}
	empty := f.analytic(t, "statistics", q)
	if empty.Channels[0].Samples != "0" || empty.Channels[0].Mean != nil || empty.Channels[0].Min != nil {
		t.Fatal(empty)
	}
}

func TestMonitoringUnboundedEndpointAndStableNavigation(t *testing.T) {
	f := monitoringSetup(t)
	f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n1900-01-01T00:00:00Z,1,good\n2262-04-11T23:47:16.854775806Z,2,good\n2262-04-11T23:47:16.854775807Z,4,good\n")))
	q := f.query(t)
	q.AllHistory = false
	from := "2262-04-11T23:47:16.854775806Z"
	q.Range = &md.Range{From: &from}
	stats := f.analytic(t, "statistics", q)
	if len(stats.Channels) != 1 || stats.Channels[0].N != "2" || *stats.Channels[0].Mean != "3.000000000000" {
		t.Fatal(stats)
	}
	q.Snapshot = stats.Snapshot.ID
	details, e := f.service.Details(ctx, f.actor, f.scope, q)
	must(t, e)
	if len(details.Items) != 2 || details.Items[0].At != "2262-04-11T23:47:16.854775807Z" {
		t.Fatal(details)
	}
	trend := f.analytic(t, "trends", q)
	if len(trend.Trends) != 1 || trend.Trends[0].Samples != "2" {
		t.Fatal(trend)
	}
	q.Period = "day"
	calendar := f.analytic(t, "statistics", q)
	if len(calendar.Groups) != 1 || calendar.Groups[0].N != "2" || !calendar.Groups[0].Partial {
		t.Fatal(calendar)
	}
	to := "1900-01-01T00:00:00.000000001Z"
	q.Range = &md.Range{To: &to}
	q.Snapshot = ""
	q.Period = ""
	stats = f.analytic(t, "statistics", q)
	if stats.Channels[0].N != "1" {
		t.Fatal(stats)
	}
	var point string
	must(t, f.env.Pool.QueryRow(ctx, "SELECT point_id::text FROM engineering_channels WHERE id=$1", f.channel).Scan(&point))
	points, e := f.service.Points(ctx, f.actor, f.scope, md.NavigationQuery{Point: point})
	must(t, e)
	if len(points.Items) != 1 || points.Items[0].ID != point {
		t.Fatal(points)
	}
	channels, e := f.service.Channels(ctx, f.actor, f.scope, point, "", f.channel)
	must(t, e)
	if len(channels.Items) != 1 || channels.Items[0].ID != f.channel {
		t.Fatal(channels)
	}
}
func TestMonitoringTrendBoundedWholeWindow(t *testing.T) {
	f := monitoringSetup(t)
	base := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	var csv strings.Builder
	csv.WriteString("observed_at,value,quality\n")
	for n := 0; n < 2400; n++ {
		v := n % 17
		if n == 1200 {
			v = 99999
		}
		fmt.Fprintf(&csv, "%s,%d,good\n", base.Add(time.Duration(n)*time.Minute).Format(time.RFC3339Nano), v)
	}
	f.publish(t, f.validate(t, f.upload(t, csv.String())))
	q := f.query(t)
	q.PlotWidth = 160
	q.Limit = 200
	result := f.analytic(t, "trends", q)
	series := result.Trends[0]
	count, points, spike, last := 0, 0, false, false
	for _, b := range series.Buckets {
		var n int
		fmt.Sscan(b.Count, &n)
		count += n
		points += len(b.Points)
		for _, p := range b.Points {
			if p.Value == "99999.000000000000" {
				spike = true
			}
			if p.At == base.Add(2399*time.Minute).Format(time.RFC3339Nano) {
				last = true
			}
		}
	}
	if !series.Summary || count != 2400 || points > 320 || !spike || !last {
		t.Fatal("bounded trend truncated or lost extrema", count, points, spike, last, series.Resolution)
	}
}

func TestMonitoringIntervalQueryAndEnumStatistics(t *testing.T) {
	f := monitoringSetup(t)
	var facility, point string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT facility_id::text,point_id::text FROM engineering_channels WHERE id=$1`, f.channel).Scan(&facility, &point))
	interval, e := f.write(t, res.Target{Kind: res.Channel, Facility: facility, Point: point}, "create", map[string]any{"code": "INTERVAL", "name": "区间平均", "metric_version_id": f.metric, "unit_version_id": f.unit, "timestamp_meaning": "interval_end", "aggregation_description": "前一小时算术平均"})
	must(t, e)
	f.channel = interval.ID
	b := f.uploadBytes(t, "interval.csv", []byte("observed_at,start,value\n2024-01-01T01:00:00Z,2024-01-01T00:00:00Z,3\n"), md.Mapping{Format: "csv", Mode: "single", Encoding: "utf-8", Delimiter: ",", HeaderRow: 1, TimeColumn: "observed_at", IntervalColumn: "start", ValueColumn: "value", Timezone: "UTC", EmptyWide: "error", Channels: []md.ChannelMapping{{Alias: "C", Channel: f.channel, Unit: "mm"}}})
	f.publish(t, f.validate(t, b))
	q := f.query(t)
	q.AllHistory = false
	lo, hi := "2024-01-01T00:30:00Z", "2024-01-01T02:00:00Z"
	q.Range = &md.Range{From: &lo, To: &hi}
	d, e := f.service.Details(ctx, f.actor, f.scope, q)
	must(t, e)
	if len(d.Items) != 1 {
		t.Fatal("interval start incorrectly became viewport lower bound", d)
	}
	stats := f.analytic(t, "statistics", q)
	if stats.Channels[0].N != "1" || *stats.Channels[0].Mean != "3.000000000000" {
		t.Fatal(stats)
	}
	definition := simple("状态")
	definition.Metric = &eng.Metric{Shape: "enum", Options: []eng.Option{{Key: "ok", Name: "正常"}, {Key: "wet", Name: "湿润"}}}
	metric := f.resourceFixture.publish(t, eng.MetricDefinition, f.create(t, eng.MetricDefinition, "sample_state", definition))
	channel, e := f.write(t, res.Target{Kind: res.Channel, Facility: facility, Point: point}, "create", map[string]any{"code": "ENUM", "name": "合成状态", "metric_version_id": metric.VersionID, "timestamp_meaning": "observation_time"})
	must(t, e)
	f.channel = channel.ID
	b = f.uploadBytes(t, "enum.csv", []byte("observed_at,value\n2024-01-01T00:00:00Z,ok\n2024-01-02T00:00:00Z,wet\n2024-01-03T00:00:00Z,ok\n"), md.Mapping{Format: "csv", Mode: "single", Encoding: "utf-8", Delimiter: ",", HeaderRow: 1, TimeColumn: "observed_at", ValueColumn: "value", Timezone: "UTC", EmptyWide: "error", Channels: []md.ChannelMapping{{Alias: "E", Channel: f.channel}}})
	f.publish(t, f.validate(t, b))
	result := f.analytic(t, "statistics", f.query(t))
	v := result.Channels[0]
	if v.N != "3" || v.Mean != nil || v.Stddev != nil || len(v.States) != 2 || v.States[0].Value != "ok" || v.States[0].Count != "2" || v.States[0].Proportion != "0.666666666667" {
		t.Fatal(v)
	}
}

func TestMonitoringSnapshotViewsAndTileBudget(t *testing.T) {
	f := monitoringSetup(t)
	var data strings.Builder
	data.WriteString("observed_at,value,quality\n")
	for n := 0; n < 240; n++ {
		fmt.Fprintf(&data, "%s,%d,good\n", time.Date(2024, 1, 1, 0, n, 0, 0, time.UTC).Format(time.RFC3339Nano), n)
	}
	f.publish(t, f.validate(t, f.upload(t, data.String())))
	q := f.query(t)
	q.Limit = 1
	root := f.analytic(t, "overview", q)
	rootSelection := q.Selection
	q.Snapshot = root.Snapshot.ID
	from, to := "2024-01-01T00:00:00Z", "2024-01-01T02:00:00Z"
	q.ViewRange = &md.Range{From: &from, To: &to}
	v := f.analytic(t, "statistics", q)
	if v.Channels[0].N != "120" || *v.Channels[0].Mean != "59.500000000000" {
		t.Fatal(v)
	}
	page, e := f.service.Details(ctx, f.actor, f.scope, q)
	must(t, e)
	if len(page.Items) != 1 || page.Items[0].At != "2024-01-01T01:59:00Z" || !page.HasMore {
		t.Fatal(page)
	}
	q.Cursor = page.NextCursor
	from2, to2 := "2024-01-01T02:00:00Z", "2024-01-01T04:00:00Z"
	q.ViewRange = &md.Range{From: &from2, To: &to2}
	_, e = f.service.Details(ctx, f.actor, f.scope, q)
	if e == nil {
		t.Fatal("cursor accepted in a different viewport")
	}
	q.Cursor = ""
	v = f.analytic(t, "statistics", q)
	if v.Channels[0].N != "120" || *v.Channels[0].Mean != "179.500000000000" || v.Snapshot.ID != root.Snapshot.ID {
		t.Fatal(v)
	}
	q.PlotBudget = 4
	tr := f.analytic(t, "trends", q)
	points := 0
	for _, b := range tr.Trends[0].Buckets {
		points += len(b.Points) + len(b.States)
	}
	if points > 4 || tr.Trends[0].Samples != "120" || !tr.Trends[0].Summary {
		t.Fatal(tr)
	}
	// A bounded base snapshot cannot be enlarged using a view.
	q.Snapshot = ""
	q.AllHistory = false
	q.Range = &md.Range{From: &from, To: &to}
	q.ViewRange = nil
	q.PlotBudget = 0
	bounded := f.analytic(t, "overview", q)
	q.Snapshot = bounded.Snapshot.ID
	q.ViewRange = &md.Range{From: &from2, To: &to2}
	_, e = f.service.Analytics(ctx, f.actor, f.scope, "overview", q)
	if e == nil {
		t.Fatal("view escaped the fixed base range")
	}
	// Appending does not change an existing tile's watermark or exact contents.
	q = md.QueryInput{Selection: rootSelection, AllHistory: true, Qualities: []string{"good", "unmarked", "suspect", "bad"}}
	q.Snapshot = root.Snapshot.ID
	q.ViewRange = &md.Range{From: &from2, To: &to2}
	f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-01T03:59:30Z,999,good\n")))
	after := f.analytic(t, "overview", q)
	if after.Overview.Active != "120" {
		t.Fatal(after)
	}
	exported := f.completeExport(t, md.ExportInput{WriteMeta: f.meta("0"), Query: q, Format: "csv", CSVMode: "exact", Kind: "details"})
	if exported.Rows != "120" {
		t.Fatal("viewport export widened its range", exported)
	}
}
