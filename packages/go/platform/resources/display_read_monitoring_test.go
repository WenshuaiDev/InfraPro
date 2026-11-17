//go:build monitoringintegration

package resources_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/display"
	dr "github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
)

func awaitDisplayRead(t *testing.T, h *displayHarness, b runtime.Business, first display.Envelope) display.Envelope {
	t.Helper()
	if first.Query == nil {
		t.Fatal("missing durable query")
	}
	id := first.Query.ID
	until := time.Now().Add(115 * time.Second)
	result := first
	for result.State == "pending" && time.Now().Before(until) {
		_, e := b.DisplayReads.ProcessOne(ctx, "display-read-test")
		must(t, e)
		// The owning worker drains its queue while this one durable display
		// query waits. Do not artificially run only one channel summary per poll.
		for range 64 {
			worked, e := b.Monitoring.RunOne(ctx, "monitoring-read-test")
			if errors.Is(e, context.Canceled) && ctx.Err() == nil {
				// The real worker retries a job whose heartbeat was interrupted.
				// Keep polling the same display query and its original deadline.
				t.Log("owning worker interrupted; retrying its durable job")
				break
			}
			must(t, e)
			if !worked {
				break
			}
		}
		result = readCall(t, h, "queries.get", dr.QueryRequest{QueryID: id})
		if result.Query.ID != id {
			t.Fatal("pending changed query identity")
		}
		if result.State == "pending" {
			time.Sleep(320 * time.Millisecond)
		}
	}
	if result.State == "pending" {
		t.Fatal("query missed deadline")
	}
	return result
}
func TestDisplayReadMonitoringFixedCountsAndWindows(t *testing.T) {
	f := monitoringSetup(t)
	first := f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2026-11-01T00:00:00+08:00,1,good\n")))
	preview, e := f.service.PreviewRevokeBatch(ctx, f.actor, f.scope, first.ID, first.Revision)
	must(t, e)
	_, e = f.service.RevokeBatch(ctx, f.actor, f.scope, first.ID, md.RevokeInput{WriteMeta: f.meta(first.Revision), PreviewHash: preview.Hash})
	must(t, e)
	data := "observed_at,value,quality\n"
	for i, quality := range []string{"good", "good", "good", "bad", "suspect", "good"} {
		data += fmt.Sprintf("2026-11-01T00:0%d:00+08:00,9007199254740993.125,%s\n", i+1, quality)
	}
	f.publish(t, f.validate(t, f.upload(t, data)))
	p := f.project(t, "NamedMonitoring")
	var facility, resourceGrant string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT facility_id::text FROM engineering_channels WHERE id=$1`, f.channel).Scan(&facility))
	g := f.accessInput(t, p, facility, "create_link")
	g.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}, {Kind: res.Channel, ChannelID: &f.channel, Capabilities: []string{"read"}}}
	f.grant(t, p, facility, g)
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM project_resource_grants WHERE project_id=$1 AND channel_id=$2 AND ended_at IS NULL`, p.id, f.channel).Scan(&resourceGrant))
	_, e = f.service.GrantData(ctx, f.actor, f.tenant, p.id, md.GrantInput{WriteMeta: f.meta("0"), ChannelID: f.channel, ResourceGrantID: resourceGrant, Range: &md.Range{}, Capabilities: []string{"read", "history"}})
	must(t, e)
	fields := []string{"resource.metadata", "statistics.counts", "monitoring.values", "monitoring.quality", "history"}
	scope := dr.Definition{ID: "raw", Domain: "channels", Selector: dr.Selector{Kind: "ids", IDs: []string{f.channel}}, FieldGroups: fields}
	view := scope
	view.ID = ""
	view.ScopeID = "raw"
	views := map[string]dr.Definition{"raw": view}
	for name, ends := range map[string][2]string{"early": {"2026-11-01T00:00:00+08:00", "2026-11-01T00:02:00+08:00"}, "late": {"2026-11-01T00:04:00+08:00", "2026-11-01T00:07:00+08:00"}, "overlap": {"2026-11-01T00:01:00+08:00", "2026-11-01T00:05:00+08:00"}} {
		v := view
		v.TimeWindow = &dr.Window{From: &ends[0], To: &ends[1]}
		views[name] = v
	}
	b := runtime.Compose(f.env.Service.Access(), f.storage)
	h := createReadDisplay(t, f.resourceFixture, p, b, []dr.Definition{scope}, views, []string{"views.read", "queries.read", "channels.read", "statistics.monitoring", "monitoring.latest", "monitoring.details", "monitoring.trends", "monitoring.statistics", "monitoring.overview"})
	options, e := h.s.ReadScopeOptions(ctx, p.actor, h.scope, h.id, display.ReadScopeOptionsInput{Purpose: "grant", Domain: "channels", Kind: "metric_version"})
	must(t, e)
	if options.Total != "1" || options.Items[0].SourceKind != "direct" {
		t.Fatalf("metric options: %+v", options)
	}
	target := dr.Target{View: "raw"}
	channels := readData[dr.Page[dr.ChannelSummary]](t, readCall(t, h, "channels.list", dr.ChannelsRequest{Target: target}))
	if channels.Total != "1" || channels.Items[0].SourceKind != "direct" {
		t.Fatalf("channels: %+v", channels)
	}
	query := dr.MonitoringStatisticsRequest{Targets: []dr.Target{target}, SourceKinds: []string{"direct"}, GroupBy: "none", MonitoringTime: dr.MonitoringTime{AllHistory: true}}
	assertCounts := func(result display.Envelope, ever, active, samples string) {
		t.Helper()
		v := readData[struct{ Totals dr.MonitoringCounts }](t, result)
		if v.Totals.Raw.Ever == nil || *v.Totals.Raw.Ever != ever || *v.Totals.Raw.Active != active || *v.Totals.Raw.Samples != samples || v.Totals.Derived.State != "not_requested" {
			t.Fatalf("counts: %s", dr.Bytes(result))
		}
		if bytes.Contains(dr.Bytes(result), []byte(f.channel)) {
			t.Fatal("summary disclosed channel identity")
		}
	}
	initial := readCall(t, h, "statistics.monitoring", query)
	if initial.State != "pending" {
		t.Fatal("expected durable pending", initial.State)
	}
	complete := awaitDisplayRead(t, h, b, initial)
	assertCounts(complete, "7", "6", "4")
	cached := readCall(t, h, "statistics.monitoring", query)
	if cached.State != "ready" || cached.Query.ID == complete.Query.ID || cached.Query.SnapshotID != complete.Query.SnapshotID {
		t.Fatal("cache must retain snapshot with independent consumer", cached.State)
	}
	readCall(t, h, "queries.release", dr.QueryControlRequest{QueryID: cached.Query.ID})
	readCall(t, h, "queries.release", dr.QueryControlRequest{QueryID: cached.Query.ID})
	assertCounts(readCall(t, h, "queries.get", dr.QueryRequest{QueryID: complete.Query.ID}), "7", "6", "4")
	from, to := "2026-11-01T00:00:00+08:00", "2026-11-01T00:05:00+08:00"
	query.AllHistory = false
	query.Range = &dr.Window{From: &from, To: &to}
	assertCounts(awaitDisplayRead(t, h, b, readCall(t, h, "statistics.monitoring", query)), "5", "4", "3")
	query.AllHistory = true
	query.Range = nil
	query.Targets = []dr.Target{{View: "early"}, {View: "late"}}
	assertCounts(awaitDisplayRead(t, h, b, readCall(t, h, "statistics.monitoring", query)), "5", "4", "2")
	query.Targets = []dr.Target{{View: "overlap"}, {View: "late"}}
	assertCounts(awaitDisplayRead(t, h, b, readCall(t, h, "statistics.monitoring", query)), "6", "6", "4")
	detail := awaitDisplayRead(t, h, b, readCall(t, h, "monitoring.details", dr.MonitoringRequest{Target: target, MonitoringTime: dr.MonitoringTime{AllHistory: true}, Paging: dr.Paging{Limit: 2}}))
	observations := readData[dr.Page[md.Observation]](t, detail)
	want, _ := new(big.Rat).SetString("9007199254740993.125")
	for _, r := range observations.Items {
		got, ok := new(big.Rat).SetString(r.Value)
		if !ok || got.Cmp(want) != 0 {
			t.Fatal("lost exact decimal", r.Value)
		}
	}
	selection := readData[dr.DrilldownSelection](t, readCall(t, h, "queries.drilldown", dr.DrilldownRequest{QueryID: complete.Query.ID, Basis: "raw_samples", Purpose: "details"}))
	for _, token := range []string{complete.Query.ID, selection.Selection + "changed"} {
		if _, err := h.s.Query(ctx, h.session, display.Call{Method: "monitoring.details", Params: dr.Bytes(dr.MonitoringRequest{Target: dr.Target{Selection: token}})}); err == nil {
			t.Fatal("forged selection accepted")
		}
	}
	if _, err := h.s.Query(ctx, h.session, display.Call{Method: "queries.drilldown", Params: dr.Bytes(dr.DrilldownRequest{QueryID: complete.Query.ID, GroupKey: "forged-group", Basis: "raw_samples", Purpose: "details"})}); err == nil {
		t.Fatal("unknown parent group accepted")
	}
	// New data becomes visible to a new query while the completed query stays fixed.
	f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2026-11-01T00:07:00+08:00,8,good\n")))
	child := awaitDisplayRead(t, h, b, readCall(t, h, "monitoring.details", dr.MonitoringRequest{Target: dr.Target{Selection: selection.Selection}}))
	if got := readData[dr.Page[md.Observation]](t, child); got.Total != "4" {
		t.Fatal("drilldown escaped parent watermark", got.Total)
	}
	assertCounts(readCall(t, h, "queries.get", dr.QueryRequest{QueryID: complete.Query.ID}), "7", "6", "4")
	query.Targets = []dr.Target{target}
	assertCounts(awaitDisplayRead(t, h, b, readCall(t, h, "statistics.monitoring", query)), "8", "7", "5")
	sub, e := h.s.Subscribe(ctx, h.session, display.SubscriptionInput{Kind: "statistics.monitoring", Query: dr.Bytes(query), Generation: "stats-v1"})
	must(t, e)
	events, e := h.s.Poll(ctx, h.session)
	must(t, e)
	if len(events) != 1 || events[0].Result == nil || events[0].Result.Query == nil {
		t.Fatal("named statistics subscription missing snapshot", events)
	}
	if events[0].Result.State == "pending" {
		t.Fatal("unchanged warm statistics repeated background work")
	}
	replay, e := h.s.Poll(ctx, h.session)
	must(t, e)
	if len(replay) != 1 || replay[0].Sequence != events[0].Sequence {
		t.Fatal("missing durable replay")
	}
	must(t, h.s.Ack(ctx, h.session, sub, events[0].Generation, events[0].Sequence))
	events, e = h.s.Poll(ctx, h.session)
	must(t, e)
	if len(events) != 0 {
		t.Fatal("unchanged fixed statistics rescanned")
	}
	must(t, h.s.CancelSubscription(ctx, h.session, sub))
	// An independent summary-only subject has method capabilities, but neither
	// those capabilities nor another subject's opaque token grant value fields.
	summaryScope, summaryView := scope, view
	summaryScope.FieldGroups = []string{"statistics.counts"}
	summaryView.FieldGroups = []string{"statistics.counts"}
	summary := createReadDisplay(t, f.resourceFixture, p, b, []dr.Definition{summaryScope}, map[string]dr.Definition{"raw": summaryView}, []string{"views.read", "queries.read", "statistics.monitoring", "monitoring.details"})
	for _, call := range []display.Call{
		{Method: "queries.get", Params: dr.Bytes(dr.QueryRequest{QueryID: complete.Query.ID})},
		{Method: "monitoring.details", Params: dr.Bytes(dr.MonitoringRequest{Target: dr.Target{Selection: selection.Selection}})},
		{Method: "monitoring.details", Params: dr.Bytes(dr.MonitoringRequest{Target: target, MonitoringTime: dr.MonitoringTime{AllHistory: true}})},
	} {
		if _, err := summary.s.Query(ctx, summary.session, call); err == nil {
			t.Fatal("summary-only or cross-subject value access", call.Method)
		}
	}
	summaryResult := awaitDisplayRead(t, summary, b, readCall(t, summary, "statistics.monitoring", query))
	assertCounts(summaryResult, "8", "7", "5")
	_, e = summary.s.Query(ctx, summary.session, display.Call{Method: "queries.drilldown", Params: dr.Bytes(dr.DrilldownRequest{QueryID: summaryResult.Query.ID, Basis: "raw_samples", Purpose: "details"})})
	var fieldError *dr.Error
	if !errors.As(e, &fieldError) || fieldError.Code != "field_not_granted" {
		t.Fatalf("summary-only drilldown must require numeric fields: %v (%s)", e, dr.Bytes(e))
	}
	// Close only this fixture's real ClickHouse clients. The disposable
	// servers and other tests remain available; a new detail query must fail
	// as a dependency error rather than returning a fabricated empty page.
	f.storage.Close()
	faultFrom, faultTo := "2026-11-01T00:07:00+08:00", "2026-11-01T00:08:00+08:00"
	fault := readCall(t, h, "monitoring.details", dr.MonitoringRequest{Target: target, MonitoringTime: dr.MonitoringTime{Range: &dr.Window{From: &faultFrom, To: &faultTo}}})
	_, e = b.DisplayReads.ProcessOne(ctx, "closed-clickhouse")
	must(t, e)
	_, e = h.s.Query(ctx, h.session, display.Call{Method: "queries.get", Params: dr.Bytes(dr.QueryRequest{QueryID: fault.Query.ID})})
	var dependency *dr.Error
	if !errors.As(e, &dependency) || dependency.Code != "dependency_unavailable" || dependency.StatusCode() != 503 {
		t.Fatalf("ClickHouse failure misreported: %v (%s)", e, dr.Bytes(e))
	}
	h.command(t, "grant", "", display.Command{Grant: &display.Grant{Revoked: true}})
	_, e = h.s.Query(ctx, h.session, display.Call{Method: "queries.get", Params: dr.Bytes(dr.QueryRequest{QueryID: complete.Query.ID})})
	if e == nil {
		t.Fatal("revoked statistics still readable")
	}
}
