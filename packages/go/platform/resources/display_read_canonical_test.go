//go:build monitoringintegration

package resources_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	catalog "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	devices "github.com/WenshuaiDev/InfraPro/packages/go/platform/devices"
	dr "github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	eng "github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	src "github.com/WenshuaiDev/InfraPro/packages/go/platform/sources"
	"github.com/google/uuid"
)

// This is the specification's deterministic fixture, not a mocked Provider.
// Raw observations pass through CSV adoption/revocation and both CH replicas;
// calculated rows pass through the real derived scheduler and publication.
func TestDisplayReadCanonicalCountsAndOpaqueDrilldowns(t *testing.T) {
	f := monitoringSetup(t)
	p := f.project(t, "SDKCanonical")
	// This fixed 14-channel/120-observation fixture previously made over
	// 410,000 SQL calls by requalifying identical recursive inputs. Bound work
	// independently of machine speed while retaining every count and page check.
	b := runtime.Compose(displayReadPlanAccess(t, f.resourceFixture, 350000), f.storage)
	options := b.Displays.Options()
	options.Lease = 15 * time.Minute
	b.Displays.WithOptions(options)
	bridge := simple("桥梁")
	bridge.Components = []eng.Component{{Key: "span", Name: "跨"}, {Key: "beam", Name: "梁"}}
	bridge.Relations = []eng.Relation{{Parent: "root", Child: "span"}, {Parent: "span", Child: "beam"}}
	firstType := f.fixture.publish(t, eng.TypeDefinition, f.create(t, eng.TypeDefinition, "bridge", bridge))
	draft, e := f.s.NewDraft(ctx, f.fixture.actor(f.p), eng.Scope{Kind: eng.TypeDefinition}, firstType.DefinitionID, eng.NewDraftInput{RevisionInput: eng.RevisionInput{WriteMeta: f.wm(t), Revision: firstType.Revision}, VersionID: firstType.VersionID}, access.Meta{})
	must(t, e)
	secondType := f.fixture.publish(t, eng.TypeDefinition, draft)
	tunnel := f.fixture.publish(t, eng.TypeDefinition, f.create(t, eng.TypeDefinition, "tunnel", func() eng.Content { v := bridge; v.Name = "隧道"; return v }()))
	facilities, points, components, channels := []res.Result{}, [][]res.Result{}, [][]res.Result{}, []string{}
	channelPoints, channelFacilities := []string{}, []string{}
	for i, typ := range []string{firstType.VersionID, secondType.VersionID, tunnel.VersionID} {
		position := map[string]any{"description": "SDK 合成验收坐标"}
		if i == 0 {
			position["geographic"] = map[string]any{"crs": "EPSG:4326", "longitude": 116.3, "latitude": 39.9}
		}
		if i == 1 {
			position["projected"] = map[string]any{"crs": "EPSG:3857", "x": 0, "y": 0, "unit": "m", "axis_description": "easting/northing"}
		}
		if i == 2 {
			position["linear_references"] = []any{map[string]any{"route_code": "G-test", "start_station": "K10+000", "reference_name": "合成桩号"}}
		}
		facility, e := f.write(t, res.Target{Kind: res.Facility}, "create", map[string]any{"code": fmt.Sprintf("F%d", i+1), "name": fmt.Sprintf("F%d 合成设施", i+1), "type_version_id": typ, "values": map[string]any{}, "position": position})
		must(t, e)
		facilities = append(facilities, facility)
		cc, pp := []res.Result{}, []res.Result{}
		for j := 0; j < []int{3, 2, 1}[i]; j++ {
			in := map[string]any{"code": fmt.Sprintf("C%d", j+1), "name": "合成构件", "category_key": "span", "expected_structure_revision": j + 1}
			if j == 2 {
				in["parent_component_id"] = cc[0].ID
				in["category_key"] = "beam"
			}
			c, e := f.write(t, target(facility, res.Component, ""), "create", in)
			must(t, e)
			cc = append(cc, c)
		}
		for j := 0; j < []int{4, 2, 2}[i]; j++ {
			in := map[string]any{"code": fmt.Sprintf("P%d", j+1), "name": "合成测点"}
			if j > 0 {
				in["component_id"] = cc[(j-1)%len(cc)].ID
			}
			v, e := f.write(t, target(facility, res.Point, ""), "create", in)
			must(t, e)
			pp = append(pp, v)
		}
		components = append(components, cc)
		points = append(points, pp)
		for j := 0; j < []int{5, 4, 3}[i]; j++ {
			pt := pp[j%len(pp)].ID
			ct := target(facility, res.Channel, "")
			ct.Point = pt
			v, e := f.write(t, ct, "create", map[string]any{"code": fmt.Sprintf("D%02d", len(channels)+1), "name": "合成原始通道", "metric_version_id": f.metric, "unit_version_id": f.unit, "timestamp_meaning": "observation_time"})
			must(t, e)
			channels = append(channels, v.ID)
			channelPoints = append(channelPoints, pt)
			channelFacilities = append(channelFacilities, facility.ID)
		}
	}
	hidden := f.facility(t, "F4_PRIVATE")
	mapping := md.Mapping{Format: "csv", Mode: "long", Encoding: "utf-8", Delimiter: ",", HeaderRow: 1, ChannelColumn: "channel", TimeColumn: "observed_at", ValueColumn: "value", QualityColumn: "quality", Timezone: "UTC", EmptyWide: "error"}
	for i, id := range channels {
		mapping.Channels = append(mapping.Channels, md.ChannelMapping{Alias: fmt.Sprintf("D%02d", i+1), Channel: id, Unit: "mm"})
	}
	base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.FixedZone("CST", 8*3600))
	var revoked, active strings.Builder
	revoked.WriteString("channel,observed_at,value,quality\n")
	active.WriteString("channel,observed_at,value,quality\n")
	for i := range channels {
		for minute := range 10 {
			quality := "good"
			if minute == 1 && (i == 1 || i == 10) {
				quality = "bad"
			}
			if minute == 1 && i == 6 {
				quality = "suspect"
			}
			value := minute + 1
			if minute >= 8 {
				value = 0
			}
			out := &active
			if minute == 0 && (i == 0 || i == 5 || i == 9) {
				out = &revoked
			}
			fmt.Fprintf(out, "D%02d,%s,%d,%s\n", i+1, base.Add(time.Duration(minute)*time.Minute).Format(time.RFC3339), value, quality)
		}
	}
	batch := f.publish(t, f.validate(t, f.uploadBytes(t, "revoke.csv", []byte(revoked.String()), mapping)))
	preview, e := f.service.PreviewRevokeBatch(ctx, f.actor, f.scope, batch.ID, batch.Revision)
	must(t, e)
	_, e = f.service.RevokeBatch(ctx, f.actor, f.scope, batch.ID, md.RevokeInput{WriteMeta: f.meta(batch.Revision), PreviewHash: preview.Hash})
	must(t, e)
	f.publish(t, f.validate(t, f.uploadBytes(t, "active.csv", []byte(active.String()), mapping)))
	derived := b.DerivedMonitoring
	must(t, dm.InitializeStorage(ctx, f.storage))
	rule, e := derived.ExpandTemplate(ctx, f.actor, dm.TenantScope(f.tenant), "difference", dm.ExpandTemplateInput{Inputs: []dm.InputSpec{{Key: "x", Shape: "numeric", SourceKinds: []string{"direct"}, MetricVersionID: f.metric, Quantity: dm.Quantity{BaseUnitVersionID: f.unit, BasePower: 1, Mode: "level", LevelScale: "ratio"}, Time: dm.TimeSemantics{Kind: "instant"}, Direction: "not_applicable", Reference: "not_applicable"}}})
	must(t, e)
	rule.Nodes = []dm.Node{{ID: "result", Op: "formula", Params: dm.NodeParams{Expression: "x / x", Variables: map[string]string{"x": "input:x"}}}}
	rule.Outputs[0].SemanticKey = "canonical_ratio"
	rule.Outputs[0].Quantity = nil
	definition, e := derived.CreateMetric(ctx, f.actor, dm.TenantScope(f.tenant), dm.CreateMetricInput{WriteMeta: restoreMeta(""), Code: "canonical_ratio", Name: "固定比值", Rule: dr.Bytes(rule)}, access.Meta{})
	must(t, e)
	valid, e := derived.ValidateMetric(ctx, f.actor, dm.TenantScope(f.tenant), definition.Definition.ID, dm.ValidateInput{DraftRevision: definition.Definition.Draft.Revision})
	must(t, e)
	published, e := derived.PublishMetric(ctx, f.actor, dm.TenantScope(f.tenant), definition.Definition.ID, dm.PublishMetricInput{WriteMeta: restoreMeta(definition.Definition.Revision), DraftRevision: definition.Definition.Draft.Revision, ValidationHash: valid.ValidationHash}, access.Meta{})
	must(t, e)
	calculated := []string{}
	lo, hi := strconv.FormatInt(base.UnixNano(), 10), strconv.FormatInt(base.Add(10*time.Minute).UnixNano(), 10)
	for i, input := range []string{channels[3], channels[7], channels[11]} {
		owner := dm.Owner{Kind: res.Facility, ID: facilities[i].ID, FacilityID: facilities[i].ID}
		if i == 1 {
			owner.Kind = res.Component
			owner.ID = components[i][0].ID
		}
		if i == 2 {
			owner.Kind = res.Point
			owner.ID = points[i][0].ID
		}
		ch, e := derived.CreateChannel(ctx, f.actor, dm.TenantScope(f.tenant), dm.CreateChannelInput{WriteMeta: restoreMeta(""), Code: fmt.Sprintf("X%02d", i+1), Name: "固定派生", Owner: owner, DefinitionVersionID: published.Version.ID, InputBindings: map[string]dm.InputBinding{"x": {ChannelID: input}}, Parameters: map[string]string{}}, access.Meta{})
		must(t, e)
		calculated = append(calculated, ch.Channel.Resource.ID)
		configs, e := derived.ChannelConfigs(ctx, f.actor, dm.TenantScope(f.tenant), ch.Channel.Resource.ID, dm.MetricFilter{})
		must(t, e)
		cfg := configs.Items[0]
		trial, e := derived.ChannelTrial(ctx, f.actor, dm.TenantScope(f.tenant), ch.Channel.Resource.ID, dm.TrialInput{WriteMeta: restoreMeta(ch.Channel.ConfigRevision), ConfigID: cfg.ID, ConfigHash: cfg.ConfigHash, OutputRange: dm.NanosecondRange{From: lo, To: &hi}}, access.Meta{})
		must(t, e)
		if trial.Trial == nil {
			_, e = b.Monitoring.RunTask(ctx, "canonical-trial", f.tenant, trial.Job.ID)
			must(t, e)
			trial, e = derived.Run(ctx, f.actor, dm.TenantScope(f.tenant), trial.ID)
			must(t, e)
		}
		if trial.Trial == nil {
			t.Fatal("missing completed trial")
		}
		enable, e := derived.PreviewControl(ctx, f.actor, dm.TenantScope(f.tenant), ch.Channel.Resource.ID, dm.ControlPreviewInput{Revision: ch.Channel.ControlRevision, ControlProposal: dm.ControlProposal{Action: "enable", AutoStartNS: &lo, StopNS: &hi, TrialReceiptID: trial.Trial.ReceiptID}})
		must(t, e)
		_, e = derived.ControlChannel(ctx, f.actor, dm.TenantScope(f.tenant), ch.Channel.Resource.ID, dm.ControlInput{WriteMeta: restoreMeta(ch.Channel.ControlRevision), PreviewID: enable.ID, PreviewHash: enable.Hash}, access.Meta{})
		must(t, e)
	}
	_, e = derived.ScheduleTenant(ctx, f.tenant)
	must(t, e)
	jobs, e := f.env.Pool.Query(ctx, `SELECT job_id::text FROM derived_runs WHERE tenant_id=$1 AND mode='automatic' ORDER BY created_at`, f.tenant)
	must(t, e)
	ids := []string{}
	for jobs.Next() {
		var id string
		must(t, jobs.Scan(&id))
		ids = append(ids, id)
	}
	must(t, jobs.Err())
	jobs.Close()
	if len(ids) != 3 {
		t.Fatalf("expected three automatic publications, got %d", len(ids))
	}
	for _, id := range ids {
		_, e = b.Monitoring.RunTask(ctx, "canonical-derived", f.tenant, id)
		must(t, e)
		job, e := b.Monitoring.Job(ctx, f.actor, f.scope, id)
		must(t, e)
		if job.State != "succeeded" {
			t.Fatalf("automatic job failed: %+v", job)
		}
	}
	for i, facility := range facilities {
		grant := f.accessInput(t, p, facility.ID, "create_link")
		grant.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}}
		for _, c := range components[i] {
			id := c.ID
			grant.Add = append(grant.Add, res.GrantInput{Kind: res.Component, ComponentID: &id, Capabilities: []string{"read"}})
		}
		for _, pt := range points[i] {
			id := pt.ID
			grant.Add = append(grant.Add, res.GrantInput{Kind: res.Point, PointID: &id, Capabilities: []string{"read"}})
		}
		for j, id := range channels {
			if channelFacilities[j] == facility.ID {
				id := id
				grant.Add = append(grant.Add, res.GrantInput{Kind: res.Channel, ChannelID: &id, Capabilities: []string{"read"}})
			}
		}
		id := calculated[i]
		grant.Add = append(grant.Add, res.GrantInput{Kind: res.Channel, ChannelID: &id, Capabilities: []string{"read"}})
		f.grant(t, p, facility.ID, grant)
	}
	revision := "0"
	for _, id := range append(append([]string{}, channels...), calculated...) {
		var rg string
		must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM project_resource_grants WHERE project_id=$1 AND channel_id=$2 AND ended_at IS NULL`, p.id, id).Scan(&rg))
		g, e := f.service.GrantData(ctx, f.actor, f.tenant, p.id, md.GrantInput{WriteMeta: f.meta(revision), ChannelID: id, ResourceGrantID: rg, Range: &md.Range{}, Capabilities: []string{"read", "history"}})
		must(t, e)
		revision = g.AccessRevision
	}
	canonicalDevices(t, f, p, b, channels, channelFacilities, channelPoints)
	scopes := []dr.Definition{{ID: "resources", Domain: "resources", Selector: dr.Selector{Kind: "project"}, FieldGroups: []string{"resource.metadata", "resource.attributes", "resource.position", "statistics.counts"}}, {ID: "raw", Domain: "channels", Selector: dr.Selector{Kind: "project", SourceKinds: []string{"direct"}}, FieldGroups: []string{"resource.metadata", "statistics.counts", "monitoring.values", "monitoring.quality", "history", "source.binding"}}, {ID: "derived", Domain: "channels", Selector: dr.Selector{Kind: "ids", IDs: calculated[:2]}, FieldGroups: []string{"resource.metadata", "statistics.counts", "monitoring.values", "monitoring.quality"}}, {ID: "count_channels", Domain: "channels", Selector: dr.Selector{Kind: "ids", IDs: append(append([]string{}, channels...), calculated[:2]...)}, FieldGroups: []string{"resource.metadata", "statistics.counts", "source.binding"}}, {ID: "devices", Domain: "devices", Selector: dr.Selector{Kind: "project"}, FieldGroups: []string{"device.metadata", "device.outputs", "statistics.counts"}}}
	views := map[string]dr.Definition{}
	for _, s := range scopes {
		v := s
		v.ID = ""
		v.ScopeID = s.ID
		views[s.ID] = v
	}
	h := createReadDisplay(t, f.resourceFixture, p, b, scopes, views, []string{"views.read", "queries.read", "resources.directory", "resources.detail", "channels.read", "devices.read", "sources.directory", "map.read", "statistics.resources", "statistics.devices", "statistics.monitoring", "monitoring.details", "monitoring.latest", "monitoring.trends", "monitoring.statistics", "monitoring.overview"})
	rt, ct, dt := dr.Target{View: "resources"}, dr.Target{View: "count_channels"}, dr.Target{View: "devices"}
	resource := readData[dr.ResourceStatistics](t, readCall(t, h, "statistics.resources", dr.ResourceStatisticsRequest{ReadFields: dr.ReadFields{FieldGroups: []string{"statistics.counts", "resource.metadata"}}, Target: rt, ChannelTarget: &ct, GroupBy: "facility_type"}))
	checkResource := func(v dr.ResourceCounts, f, c, p, ch string) {
		t.Helper()
		if v.FacilityCount != f || v.ComponentCount != c || v.PointCount != p || v.ChannelCount == nil || *v.ChannelCount != ch {
			t.Fatalf("resource totals: %+v", v)
		}
	}
	checkResource(resource.Totals, "3", "6", "8", "14")
	if *resource.Totals.DirectCount != "12" || *resource.Totals.DerivedCount != "2" {
		t.Fatal(resource.Totals)
	}
	if len(resource.Groups) != 2 {
		t.Fatal("type versions did not group by definition", resource.Groups)
	}
	for _, g := range resource.Groups {
		if g.Ref.ID == firstType.DefinitionID {
			checkResource(g.Totals, "2", "5", "6", "11")
		} else {
			checkResource(g.Totals, "1", "1", "2", "3")
		}
	}
	types := readData[dr.Page[dr.ResourceType]](t, readCall(t, h, "resources.types", dr.ResourcesRequest{Target: rt}))
	if types.Total != "2" {
		t.Fatal(types)
	}
	for _, typ := range types.Items {
		if typ.ID == firstType.DefinitionID && len(typ.Versions) != 2 {
			t.Fatal("fixed type versions missing", typ)
		}
	}
	roots := readData[dr.Page[dr.ResourceSummary]](t, readCall(t, h, "resources.tree", dr.ResourceTreeRequest{Target: rt, Mode: "root"}))
	if roots.Total != "3" {
		t.Fatal("tree roots escaped facilities", roots)
	}
	children := readData[dr.Page[dr.ResourceSummary]](t, readCall(t, h, "resources.tree", dr.ResourceTreeRequest{Target: rt, Mode: "children", Parent: &dr.Ref{Kind: "facility", ID: facilities[0].ID}}))
	if !slices.ContainsFunc(children.Items, func(v dr.ResourceSummary) bool { return v.ID == points[0][0].ID }) {
		t.Fatal("facility-owned point missing from children", children)
	}
	narrow := dr.Target{View: "resources", IDs: []string{components[0][2].ID}}
	path := readData[dr.Page[dr.ResourceSummary]](t, readCall(t, h, "resources.tree", dr.ResourceTreeRequest{Target: narrow, Mode: "path", Node: &dr.Ref{Kind: "component", ID: components[0][2].ID}}))
	if len(path.Items) != 3 || path.Items[0].ID != facilities[0].ID || path.Items[2].Visibility != "full" {
		t.Fatal("nested component path", path)
	}
	for _, ancestor := range path.Items[:2] {
		if ancestor.Visibility != "context" || ancestor.Type != nil || ancestor.Status != "" || ancestor.Revision != "" || ancestor.FacilityID != "" {
			t.Fatal("context ancestor gained full fields", ancestor)
		}
	}
	withoutChannels := readData[dr.ResourceStatistics](t, readCall(t, h, "statistics.resources", dr.ResourceStatisticsRequest{Target: rt, GroupBy: "none"}))
	if withoutChannels.Totals.ChannelCount != nil || withoutChannels.Totals.ChannelsState == "available" {
		t.Fatal("missing channel target fabricated channel counts", withoutChannels)
	}
	mapped := readData[struct {
		Items []dr.FacilityMapItem
		Total string
	}](t, readCall(t, h, "map.facilities", dr.MapFacilitiesRequest{Target: rt}))
	if mapped.Total != "3" {
		t.Fatal(mapped)
	}
	for _, item := range mapped.Items {
		if item.Facility.ID == facilities[2].ID {
			if item.PositionState != "missing" || item.Longitude != nil {
				t.Fatal(item)
			}
		} else if item.PositionState != "located" || item.Longitude == nil {
			t.Fatal(item)
		}
		if item.Facility.ID == facilities[1].ID && *item.Longitude != 0 {
			t.Fatal("projected origin lost", item)
		}
	}
	if bytes.Contains(dr.Bytes(resource), []byte(hidden.ID)) {
		t.Fatal("hidden F4 leaked")
	}
	for _, population := range []string{"authorized", "monitoring_bound"} {
		for _, groupBy := range []string{"manufacturer", "facility", "product", "product_model"} {
			r := readData[dr.DeviceStatistics](t, readCall(t, h, "statistics.devices", dr.DeviceStatisticsRequest{ReadFields: dr.ReadFields{FieldGroups: []string{"statistics.counts", "device.metadata"}}, Target: dt, ChannelTarget: &ct, ResourceTarget: &rt, Population: population, Relation: "source", GroupBy: groupBy}))
			want := "6"
			if population == "monitoring_bound" {
				want = "5"
			}
			if r.Totals.UniqueDeviceCount != want {
				t.Fatal(population, groupBy, r)
			}
			if groupBy == "manufacturer" {
				counts := map[string]bool{}
				for _, g := range r.Groups {
					counts[g.Totals.UniqueDeviceCount] = true
				}
				if !counts["3"] || (population == "monitoring_bound" && !counts["2"]) {
					t.Fatal(r)
				}
			}
			if groupBy == "product" || groupBy == "product_model" {
				counts := []string{}
				for _, g := range r.Groups {
					counts = append(counts, g.Totals.UniqueDeviceCount)
				}
				slices.Sort(counts)
				want := []string{"1", "1", "2", "2"}
				if population == "monitoring_bound" {
					want = []string{"1", "2", "2"}
				}
				if !slices.Equal(counts, want) || r.Additivity != "disjoint" {
					t.Fatalf("product versions or same-name manufacturers merged: %s", dr.Bytes(r))
				}
			}
			if groupBy == "facility" && population == "authorized" {
				unassigned := ""
				for _, g := range r.Groups {
					if g.Key == "unassigned" {
						unassigned = g.Totals.UniqueDeviceCount
					}
				}
				if unassigned != "1" {
					t.Fatalf("unassigned authorized device: %s", dr.Bytes(r))
				}
			}
			if groupBy == "facility" && population == "monitoring_bound" {
				if r.Additivity != "overlapping" || len(r.Groups) != 3 {
					t.Fatal(r)
				}
				for _, g := range r.Groups {
					for i, facility := range facilities {
						if g.Ref != nil && g.Ref.ID == facility.ID && g.Totals.UniqueDeviceCount != []string{"3", "2", "1"}[i] {
							t.Fatal(g)
						}
					}
				}
			}
		}
	}
	query := dr.MonitoringStatisticsRequest{Targets: []dr.Target{{View: "raw"}, {View: "derived"}}, ResourceTarget: &rt, SourceKinds: []string{"direct", "derived"}, GroupBy: "facility", MonitoringTime: dr.MonitoringTime{AllHistory: true}, Paging: dr.Paging{Limit: 1}}
	first := awaitDisplayRead(t, h, b, readCall(t, h, "statistics.monitoring", query))
	stats := readData[dr.MonitoringStatistics](t, first)
	checkCounts := func(v dr.MonitoringCounts, ever, active, samples, logical, valid, none string) {
		t.Helper()
		if v.Raw.Ever == nil || *v.Raw.Ever != ever || *v.Raw.Active != active || *v.Raw.Samples != samples || v.Derived.Logical == nil || *v.Derived.Logical != logical || *v.Derived.Valid != valid || *v.Derived.NoResult != none || v.Derived.Samples == nil || *v.Derived.Samples != valid {
			t.Fatalf("monitoring counts: %s", dr.Bytes(v))
		}
	}
	checkCounts(stats.Totals, "120", "117", "114", "20", "16", "4")
	if stats.Totals.Raw.Qualities["good"] != "114" || stats.Totals.Raw.Qualities["bad"] != "2" || stats.Totals.Raw.Qualities["suspect"] != "1" {
		t.Fatal(stats.Totals)
	}
	groups := append([]dr.StatisticsGroup[dr.MonitoringCounts]{}, stats.Groups...)
	page := stats
	for page.HasMore {
		r := readCall(t, h, "queries.get", dr.QueryRequest{QueryID: first.Query.ID, Cursor: page.NextCursor, Limit: 1})
		page = readData[dr.MonitoringStatistics](t, r)
		checkCounts(page.Totals, "120", "117", "114", "20", "16", "4")
		groups = append(groups, page.Groups...)
	}
	if len(groups) != 3 {
		t.Fatal(groups)
	}
	for _, g := range groups {
		for i, facility := range facilities {
			if g.Ref != nil && g.Ref.ID == facility.ID {
				checkCounts(g.Totals, []string{"50", "40", "30"}[i], []string{"49", "39", "29"}[i], []string{"48", "38", "28"}[i], []string{"10", "10", "0"}[i], []string{"8", "8", "0"}[i], []string{"2", "2", "0"}[i])
			}
		}
	}
	details := func(basis, group string, want int) {
		t.Helper()
		sel := readData[dr.DrilldownSelection](t, readCall(t, h, "queries.drilldown", dr.DrilldownRequest{QueryID: first.Query.ID, Basis: basis, Purpose: "details", GroupKey: group}))
		r := awaitDisplayRead(t, h, b, readCall(t, h, "monitoring.details", dr.MonitoringRequest{Target: dr.Target{Selection: sel.Selection}, Paging: dr.Paging{Limit: 7}}))
		p := readData[dr.Page[json.RawMessage]](t, r)
		seen := len(p.Items)
		all := append([]json.RawMessage{}, p.Items...)
		for p.HasMore {
			r = readCall(t, h, "queries.get", dr.QueryRequest{QueryID: r.Query.ID, Cursor: p.NextCursor, Limit: 7})
			p = readData[dr.Page[json.RawMessage]](t, r)
			seen += len(p.Items)
			all = append(all, p.Items...)
		}
		if seen != want || p.Total != strconv.Itoa(want) {
			t.Fatal(basis, group, "drilldown count", seen, p.Total)
		}
		if basis == "derived_logical" {
			missing := 0
			for _, v := range all {
				var row struct {
					Value  *string `json:"value"`
					Status string  `json:"calculation_status"`
				}
				must(t, json.Unmarshal(v, &row))
				if row.Value == nil {
					missing++
				}
			}
			if missing != 4 {
				t.Fatal("no-result rows must have no value", missing)
			}
		}
	}
	details("raw_samples", "", 114)
	details("raw_active", "", 117)
	details("derived_samples", "", 16)
	details("derived_logical", "", 20)
	for _, g := range groups {
		n, _ := strconv.Atoi(*g.Totals.Raw.Samples)
		details("raw_samples", g.Key, n)
	}
	query.GroupBy = "facility_type"
	query.Limit = 200
	typed := readData[dr.MonitoringStatistics](t, awaitDisplayRead(t, h, b, readCall(t, h, "statistics.monitoring", query)))
	for _, g := range typed.Groups {
		if g.Ref != nil && g.Ref.ID == firstType.DefinitionID {
			checkCounts(g.Totals, "90", "88", "86", "20", "16", "4")
		}
	}
	from, to := base.Format(time.RFC3339), base.Add(5*time.Minute).Format(time.RFC3339)
	query.AllHistory = false
	query.Range = &dr.Window{From: &from, To: &to}
	query.GroupBy = "none"
	timed := readData[dr.MonitoringStatistics](t, awaitDisplayRead(t, h, b, readCall(t, h, "statistics.monitoring", query)))
	checkCounts(timed.Totals, "60", "57", "54", "10", "10", "0")
	// Every named monitoring family reaches the real owning provider. Using one
	// fixed direct channel keeps these checks separate from aggregate totals.
	request := dr.MonitoringRequest{Target: dr.Target{View: "raw", IDs: []string{channels[0]}}, MonitoringTime: dr.MonitoringTime{AllHistory: true}, ReadFields: dr.ReadFields{FieldGroups: []string{"resource.metadata", "statistics.counts", "monitoring.values", "monitoring.quality"}}}
	for _, method := range []string{"monitoring.latest", "monitoring.statistics"} {
		value := readData[dr.Page[md.ChannelStatistic]](t, awaitDisplayRead(t, h, b, readCall(t, h, method, request)))
		if value.Total != "1" || value.Items[0].Channel != channels[0] || value.Items[0].Last == nil || value.Items[0].N == "0" {
			t.Fatalf("%s missed real data: %s", method, dr.Bytes(value))
		}
	}
	trend := readData[dr.Page[md.TrendSeries]](t, awaitDisplayRead(t, h, b, readCall(t, h, "monitoring.trends", request)))
	if trend.Total != "1" || len(trend.Items) != 1 {
		t.Fatal("named trends missed selected channel", trend)
	}
	overview := readData[md.Overview](t, awaitDisplayRead(t, h, b, readCall(t, h, "monitoring.overview", request)))
	if overview.Ever != "10" || overview.Active == "0" || overview.Samples == "0" {
		t.Fatal("named overview missed real counts", overview)
	}
	t.Logf("canonical actual: resources=%s monitoring=%s time=%s", dr.Bytes(resource.Totals), dr.Bytes(stats.Totals), dr.Bytes(timed.Totals))
}

func canonicalDevices(t *testing.T, f monitoringFixture, p projectFixture, b runtime.Business, channels, facilities, points []string) {
	t.Helper()
	c := catalog.New(f.env.Service.Access())
	admin := f.fixture.actor(f.p)
	meta := func() catalog.Meta {
		return catalog.Meta{RequestID: uuid.NewString(), Reason: "SDK canonical device fixture"}
	}
	makers := []string{}
	for i := range 2 {
		m, e := c.CreateManufacturer(ctx, admin, catalog.PlatformScope(), catalog.ManufacturerCreate{Meta: meta(), Code: fmt.Sprintf("M%d", i+1), Name: fmt.Sprintf("M%d", i+1)}, access.Meta{})
		must(t, e)
		makers = append(makers, m.ID)
	}
	products := []string{}
	for i := range 4 {
		p, e := c.CreateProduct(ctx, admin, catalog.PlatformScope(), catalog.ProductCreate{Meta: meta(), ManufacturerID: makers[i/2], Code: fmt.Sprintf("P%d", i+1), Name: fmt.Sprintf("P%d", i+1), Model: "same-model", OutputSlots: []catalog.Slot{{Key: "a", Name: "A", Mode: "fixed", MetricVersionID: f.metric, UnitVersionID: &f.unit, TimestampMeaning: "observation_time"}, {Key: "b", Name: "B", Mode: "fixed", MetricVersionID: f.metric, UnitVersionID: &f.unit, TimestampMeaning: "observation_time"}}}, access.Meta{})
		must(t, e)
		p, e = c.Publish(ctx, admin, catalog.PlatformScope(), p.ID, catalog.DraftRevision{Meta: meta(), Revision: "1"}, access.Meta{})
		must(t, e)
		products = append(products, p.VersionID)
	}
	ds := []devices.Result{}
	outs := [][]devices.Output{}
	for i, product := range []int{0, 0, 1, 2, 2, 3, 3, 3} {
		d, e := b.Devices.Create(ctx, f.actor, devices.TenantScope(f.tenant), devices.DeviceCreate{Meta: meta(), Code: fmt.Sprintf("V%d", i+1), Name: fmt.Sprintf("V%d", i+1), SerialNumber: fmt.Sprintf("secret-%d", i+1), ProductVersionID: products[product]}, access.Meta{})
		must(t, e)
		ds = append(ds, d)
		slots, e := b.Devices.Slots(ctx, f.actor, devices.TenantScope(f.tenant), d.ID, devices.Filter{})
		must(t, e)
		outs = append(outs, []devices.Output{*slots.Items[0].Current, *slots.Items[1].Current})
	}
	for _, pair := range [][3]int{{0, 0, 0}, {0, 1, 1}, {1, 0, 2}, {1, 1, 5}, {2, 0, 3}, {3, 0, 6}, {3, 1, 7}, {4, 0, 9}, {4, 1, 10}, {6, 0, 4}, {7, 0, 11}} {
		output := outs[pair[0]][pair[1]]
		j := pair[2]
		target := src.Target{FacilityID: facilities[j], PointID: points[j], ChannelID: channels[j]}
		cmd := src.Bind(src.BindInput{Revisions: src.Revisions{Meta: meta(), SourceRevision: "0", ChannelRevision: "1"}, NewEnd: src.NewEnd{OutputID: output.ID, NewOutputRevision: output.Revision, NewDeviceRevision: output.DeviceRevision}})
		preview, e := b.Sources.Preview(ctx, f.actor, devices.TenantScope(f.tenant), target, cmd)
		must(t, e)
		cmd.PreviewHash = preview.Hash
		_, e = b.Sources.Mutate(ctx, f.actor, devices.TenantScope(f.tenant), target, cmd, access.Meta{})
		must(t, e)
	}
	state, e := b.Devices.Grants(ctx, f.actor, f.tenant, p.id, devices.Filter{})
	must(t, e)
	g := devices.GrantChangeSet{Meta: meta(), AccessRevision: state.AccessRevision, ProjectRevision: state.ProjectRevision}
	for i := range 6 {
		g.Add = append(g.Add, devices.GrantAdd{Kind: "device", ID: ds[i].ID})
		for _, o := range outs[i] {
			g.Add = append(g.Add, devices.GrantAdd{Kind: "output", ID: o.ID})
		}
	}
	g.Add = append(g.Add, devices.GrantAdd{Kind: "output", ID: outs[6][0].ID})
	preview, e := b.Devices.PreviewGrants(ctx, f.actor, f.tenant, p.id, g)
	must(t, e)
	g.PreviewHash = preview.Hash
	_, e = b.Devices.ChangeGrants(ctx, f.actor, f.tenant, p.id, g, access.Meta{})
	must(t, e)
}
