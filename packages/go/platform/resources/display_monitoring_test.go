//go:build monitoringintegration

package resources_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/display"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/google/uuid"
	"net/url"
	"testing"
	"time"
)

func TestDisplayRealMonitoringAndDurableSubscriptions(t *testing.T) {
	f := monitoringSetup(t)
	f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-01T00:00:00.000000001Z,9007199254740993.125,good\n2024-01-01T00:00:00.000000002Z,12.25,suspect\n")))
	p := f.project(t, "DisplayMonitoring")
	var facility string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT facility_id::text FROM engineering_channels WHERE id=$1`, f.channel).Scan(&facility))
	g := f.accessInput(t, p, facility, "create_link")
	g.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}, {Kind: res.Channel, ChannelID: &f.channel, Capabilities: []string{"read"}}}
	f.grant(t, p, facility, g)
	var rg string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM project_resource_grants WHERE project_id=$1 AND channel_id=$2 AND ended_at IS NULL`, p.id, f.channel).Scan(&rg))
	_, e := f.service.GrantData(ctx, f.actor, f.tenant, p.id, md.GrantInput{WriteMeta: f.meta("0"), ChannelID: f.channel, ResourceGrantID: rg, Range: &md.Range{}, Capabilities: []string{"read", "source"}})
	must(t, e)
	b := runtime.Compose(f.env.Service.Access(), f.storage)
	h := createDisplayHarness(t, f.resourceFixture, p, b.Displays, map[string]display.Binding{"data": {Kind: "channels", IDs: []string{f.channel}}, "resource": {Kind: "resources", IDs: []string{facility, f.channel}}}, display.Grant{Capabilities: []string{"resources.read", "channels.read", "monitoring.latest", "monitoring.details", "monitoring.trends", "monitoring.statistics", "monitoring.overview", "sources.read"}, Fields: []string{"metadata", "values", "quality", "source"}, Scope: display.ReadScope{Channels: []string{f.channel}, Resources: []string{facility, f.channel}}})
	for _, kind := range []string{"monitoring", "resource"} {
		target, e := h.s.Navigation(ctx, h.session, kind, f.channel)
		must(t, e)
		parsed, e := url.Parse(target)
		must(t, e)
		path := "/t/" + f.tenant + "/p/" + p.id + "/monitoring"
		if kind == "resource" {
			path = "/t/" + f.tenant + "/p/" + p.id + "/resources/facilities/" + facility
			var point string
			must(t, f.env.Pool.QueryRow(ctx, `SELECT point_id::text FROM engineering_channels WHERE id=$1`, f.channel).Scan(&point))
			if parsed.Query().Get("point") != point {
				t.Fatal("resource navigation lost the channel's owning point", target)
			}
		}
		if parsed.Path != path || parsed.Query().Get("channel") != f.channel {
			t.Fatal("navigation did not select the requested channel", target)
		}
	}
	q := display.Query{Target: display.Target{Binding: "data"}, AllHistory: true, Qualities: []string{"good", "suspect", "bad", "unmarked"}, PlotWidth: 800, PlotBudget: 1000}
	for _, method := range []string{"channels.list", "monitoring.latest", "monitoring.details", "monitoring.trends", "monitoring.statistics", "monitoring.overview", "sources.get", "sources.history"} {
		r := h.query(t, method, q)
		raw, _ := json.Marshal(r)
		if r.State == "" {
			t.Fatal(method, string(raw))
		}
		if method == "monitoring.details" && !bytes.Contains(raw, []byte("9007199254740993.125")) {
			t.Fatal("exact number lost", string(raw))
		}
	}
	raw, _ := json.Marshal(q)
	sub, e := h.s.Subscribe(ctx, h.session, display.SubscriptionInput{Kind: "latest", Query: raw, Generation: "search-1"})
	must(t, e)
	events, e := h.s.Poll(ctx, h.session)
	must(t, e)
	if len(events) != 1 || events[0].Subscription != sub || events[0].Sequence != "1" {
		t.Fatal(events)
	}
	replay, e := h.s.Poll(ctx, h.session)
	must(t, e)
	if len(replay) != 1 || replay[0].Sequence != "1" {
		t.Fatal("unapplied event not replayed")
	}
	must(t, h.s.Ack(ctx, h.session, sub, events[0].Generation, "1"))
	if e = h.s.Ack(ctx, h.session, sub, events[0].Generation, "3"); e == nil {
		t.Fatal("cursor skip accepted")
	}
	_, e = f.env.Pool.Exec(ctx, `UPDATE display_subscriptions SET updated_at=clock_timestamp()-interval '10 seconds' WHERE id=$1`, sub)
	must(t, e)
	next, e := h.s.Poll(ctx, h.session)
	must(t, e)
	if len(next) != 1 || next[0].Sequence != "2" {
		t.Fatal(next)
	}
	_, e = f.env.Pool.Exec(ctx, `UPDATE display_subscriptions SET updated_at=clock_timestamp()-interval '6 minutes' WHERE id=$1`, sub)
	must(t, e)
	reset, e := h.s.Poll(ctx, h.session)
	must(t, e)
	if len(reset) != 1 || reset[0].Kind != "reset" {
		t.Fatal(reset)
	}
	must(t, h.s.CancelSubscription(ctx, h.session, sub))
	events, e = h.s.Poll(ctx, h.session)
	must(t, e)
	if len(events) != 0 {
		t.Fatal("cancel delivered data")
	}
	_, e = f.env.Pool.Exec(ctx, `UPDATE display_sessions SET lease_until=clock_timestamp()-interval '1 second' WHERE id=$1`, h.session.ID)
	must(t, e)
	if _, e = h.s.Poll(ctx, h.session); e == nil {
		t.Fatal("expired lease delivered")
	}
	_ = time.Second
}

func TestDisplayDynamicScopesAndExplicitTime(t *testing.T) {
	f := monitoringSetup(t)
	p := f.project(t, "DisplayDynamic")
	var facility string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT facility_id::text FROM engineering_channels WHERE id=$1`, f.channel).Scan(&facility))
	from, to := "2024-01-01T00:00:00Z", "2025-01-01T00:00:00Z"
	dataRev := "0"
	grantChannel := func(id, action string) {
		g := f.accessInput(t, p, facility, action)
		if action == "create_link" {
			g.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}}
		}
		g.Add = append(g.Add, res.GrantInput{Kind: res.Channel, ChannelID: &id, Capabilities: []string{"read"}})
		f.grant(t, p, facility, g)
		var rg string
		must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM project_resource_grants WHERE project_id=$1 AND channel_id=$2 AND ended_at IS NULL`, p.id, id).Scan(&rg))
		v, e := f.service.GrantData(ctx, f.actor, f.tenant, p.id, md.GrantInput{WriteMeta: f.meta(dataRev), ChannelID: id, ResourceGrantID: rg, Range: &md.Range{From: &from, To: &to}, Capabilities: []string{"read"}})
		must(t, e)
		dataRev = v.AccessRevision
	}
	grantChannel(f.channel, "create_link")
	dynamic := func(future bool) *display.Dynamic {
		return &display.Dynamic{FacilityIDs: []string{facility}, IncludeFuture: future}
	}
	h := createDisplayHarness(t, f.resourceFixture, p, runtime.Compose(f.env.Service.Access(), f.storage).Displays, map[string]display.Binding{"frozen": {Kind: "channels", IDs: []string{}, Dynamic: dynamic(false)}, "live": {Kind: "channels", IDs: []string{}, Dynamic: dynamic(true)}}, display.Grant{Capabilities: []string{"channels.read", "monitoring.latest"}, Scope: display.ReadScope{Dynamic: dynamic(true)}})
	ids := createInputChannels(t, f, 2)
	grantChannel(ids[1], "change_grants")
	bindings, e := h.s.Bindings(ctx, h.session, "")
	must(t, e)
	if len(bindings["frozen"].(map[string]any)["ids"].([]string)) != 1 || len(bindings["live"].(map[string]any)["ids"].([]string)) != 2 {
		t.Fatal("dynamic future policy", bindings)
	}
	// An explicit required time range must be fully covered by current grants.
	outside := "2023-01-01T00:00:00Z"
	_, e = h.s.Command(ctx, p.actor, h.scope, h.id, "config", "", display.Command{Meta: display.Meta{Revision: h.revision, RequestID: uuid.NewString(), Reason: "time proof"}, PackageID: h.session.PackageID, Mode: "fixed", Parameters: map[string]any{}, Bindings: map[string]display.Binding{"live": {Kind: "channels", IDs: []string{f.channel}, From: &outside, To: &to}, "frozen": {Kind: "channels", IDs: []string{f.channel}}}})
	if e == nil {
		t.Fatal("ungranted explicit history accepted")
	}
	var de *display.Error
	if !errors.As(e, &de) || de.Code != "query_invalid" {
		t.Fatalf("wrong time proof error: %v", e)
	}
	h.command(t, "grant", "", display.Command{Grant: &display.Grant{Capabilities: []string{"channels.read", "monitoring.latest"}, Scope: display.ReadScope{Channels: []string{f.channel}}, ValidUntil: time.Now().Add(time.Hour)}})
	verifier := "0123456789012345678901234567890123456789"
	launch, e := h.s.Launch(ctx, p.actor, h.scope, h.id, display.Command{SessionID: h.login, Challenge: displayHash([]byte(verifier)), Audience: h.s.Options().RuntimeOrigin}, false)
	must(t, e)
	_, _, session, e := h.s.Exchange(ctx, launch["code"].(string), verifier, h.s.Options().RuntimeOrigin)
	must(t, e)
	bindings, e = h.s.Bindings(ctx, session, "")
	must(t, e)
	if len(bindings["live"].(map[string]any)["ids"].([]string)) != 1 {
		t.Fatal("binding expanded beyond grant")
	}
	// Concurrent subscriptions cannot overrun the session budget.
	options := h.s.Options()
	options.MaxSubscriptions = 1
	h.s.WithOptions(options)
	raw, _ := json.Marshal(display.Query{Target: display.Target{Binding: "live"}})
	results := make(chan error, 2)
	for range 2 {
		go func() {
			_, e := h.s.Subscribe(ctx, session, display.SubscriptionInput{Kind: "latest", Query: raw})
			results <- e
		}()
	}
	success := 0
	for range 2 {
		if <-results == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatal("subscription budget race", success)
	}
}
