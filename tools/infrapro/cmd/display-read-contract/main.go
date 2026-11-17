// The wire schema is generated from the same method-specific DTOs decoded by
// the server. Semantic constraints are applied below, then checked in CI.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/display"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
)

type schema = map[string]any
type DirectObservation monitoring.Observation

func main() {
	raw, e := os.ReadFile("contracts/display/v1/protocol.json")
	must(e)
	var protocol struct {
		Methods map[string]string `json:"x-method-capabilities"`
	}
	must(json.Unmarshal(raw, &protocol))
	defs := map[string]schema{"ReadJSONValue": {"anyOf": []schema{{"type": "null"}, {"type": "boolean"}, {"type": "string"}, {"type": "number"}, {"type": "array", "items": schema{"$ref": "#/$defs/ReadJSONValue"}}, {"type": "object", "additionalProperties": schema{"$ref": "#/$defs/ReadJSONValue"}}}}}
	var of func(reflect.Type) schema
	var properties func(reflect.Type) (map[string]any, []string)
	properties = func(t reflect.Type) (map[string]any, []string) {
		for t.Kind() == reflect.Pointer {
			t = t.Elem()
		}
		props := map[string]any{}
		required := []string{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			tag := strings.Split(f.Tag.Get("json"), ",")
			if tag[0] == "-" {
				continue
			}
			if f.Anonymous && tag[0] == "" {
				p, r := properties(f.Type)
				for k, v := range p {
					props[k] = v
				}
				if f.Type.Kind() != reflect.Pointer {
					required = append(required, r...)
				}
				continue
			}
			name := tag[0]
			if name == "" {
				name = f.Name
			}
			props[name] = of(f.Type)
			optional := strings.Contains(f.Tag.Get("json"), ",omitempty")
			if !optional && !slices.Contains([]string{"quality", "quality_reason", "data_quality", "reason_flags"}, name) {
				required = append(required, name)
			}
		}
		return props, required
	}
	of = func(t reflect.Type) schema {
		if t.Kind() == reflect.Pointer {
			return schema{"anyOf": []schema{of(t.Elem()), {"type": "null"}}}
		}
		if t == reflect.TypeFor[json.RawMessage]() {
			return schema{"$ref": "#/$defs/ReadJSONValue"}
		}
		if t == reflect.TypeFor[monitoring.Observation]() {
			return schema{"oneOf": []schema{of(reflect.TypeFor[DirectObservation]()), of(reflect.TypeFor[monitoring.CalculatedObservation]())}}
		}
		if t == reflect.TypeFor[time.Time]() {
			return schema{"type": "string", "format": "date-time"}
		}
		switch t.Kind() {
		case reflect.String:
			return schema{"type": "string"}
		case reflect.Bool:
			return schema{"type": "boolean"}
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint64:
			return schema{"type": "integer"}
		case reflect.Float32, reflect.Float64:
			return schema{"type": "number"}
		case reflect.Slice, reflect.Array:
			return schema{"type": "array", "items": of(t.Elem())}
		case reflect.Map:
			return schema{"type": "object", "additionalProperties": of(t.Elem())}
		case reflect.Interface:
			return schema{"$ref": "#/$defs/ReadJSONValue"}
		case reflect.Struct:
			name := "Read" + t.Name()
			if strings.HasPrefix(t.Name(), "Page[") {
				f, _ := t.FieldByName("Items")
				name = "Read" + f.Type.Elem().Name() + "Page"
			}
			if strings.HasPrefix(t.Name(), "StatisticsGroup[") {
				f, _ := t.FieldByName("Totals")
				name = "Read" + f.Type.Name() + "Group"
			}
			name = regexp.MustCompile(`[^A-Za-z0-9_]`).ReplaceAllString(name, "")
			if _, ok := defs[name]; !ok {
				defs[name] = schema{}
				p, r := properties(t)
				defs[name] = schema{"type": "object", "properties": p, "additionalProperties": false}
				if len(r) > 0 {
					defs[name]["required"] = r
				}
			}
			return schema{"$ref": "#/$defs/" + name}
		}
		panic("unsupported wire type " + t.String())
	}
	methods := map[string]schema{}
	for method := range protocol.Methods {
		request, err := displayread.NewRequest(method)
		if err == nil {
			methods[method] = of(reflect.TypeOf(request).Elem())
		}
	}
	for _, value := range []any{displayread.Definition{}, displayread.ViewSchema{}, displayread.Selector{}, displayread.QueryInfo{}, displayread.Result{}} {
		of(reflect.TypeOf(value))
	}
	responses := map[string]schema{}
	outputs := map[string]any{
		"statistics.structure": displayread.StructureStatistics{}, "classification.categories": displayread.CategoryPage{}, "statistics.deviceCategories": displayread.DeviceCategoryStatistics{}, "resources.hierarchy": displayread.Page[displayread.ResourceSummary]{}, "resources.types": displayread.Page[displayread.ResourceType]{}, "resources.query": displayread.Page[displayread.ResourceSummary]{}, "resources.tree": displayread.Page[displayread.ResourceSummary]{}, "resources.detail": displayread.ResourceDetail{},
		"devices.list": displayread.Page[displayread.DeviceDetail]{}, "devices.get": displayread.DeviceDetail{}, "devices.outputs": displayread.Page[displayread.DeviceOutput]{}, "devices.history": displayread.Page[displayread.DeviceChange]{},
		"sources.bindings": displayread.Page[displayread.SourceBindingItem]{}, "sources.bindingHistory": displayread.Page[displayread.SourceBindingItem]{},
		"map.facilities": displayread.Page[displayread.FacilityMapItem]{}, "map.features": displayread.Page[displayread.MapFeature]{}, "map.basemaps": displayread.Page[displayread.Basemap]{}, "map.readTile": displayread.FileChunk{},
		"spatial.assets": displayread.Page[displayread.SpatialAsset]{}, "spatial.versions": displayread.Page[displayread.SpatialVersion]{}, "spatial.manifest": displayread.SpatialManifest{}, "spatial.objects": displayread.Page[displayread.SpatialElement]{}, "spatial.placements": displayread.Page[displayread.SpatialPlacement]{}, "spatial.componentLinks": displayread.Page[displayread.SpatialComponentLink]{}, "spatial.locations": displayread.Page[displayread.SpatialLocation]{}, "spatial.settings": displayread.SpatialSettings{}, "spatial.readFile": displayread.FileChunk{},
		"channels.list": displayread.Page[displayread.ChannelSummary]{}, "channels.get": displayread.ChannelSummary{},
		"monitoring.latest": displayread.Page[monitoring.ChannelStatistic]{}, "monitoring.details": displayread.Page[monitoring.Observation]{}, "monitoring.trends": displayread.Page[monitoring.TrendSeries]{}, "monitoring.statistics": displayread.Page[monitoring.ChannelStatistic]{}, "monitoring.overview": monitoring.Overview{},
		"statistics.resources": displayread.ResourceStatistics{}, "statistics.devices": displayread.DeviceStatistics{}, "statistics.monitoring": displayread.MonitoringStatistics{}, "queries.drilldown": displayread.DrilldownSelection{}, "views.describe": display.ReadViewDescription{},
	}
	for method, value := range outputs {
		responses[method] = of(reflect.TypeOf(value))
	}
	responses["views.list"] = schema{"type": "object", "properties": schema{"items": schema{"type": "array", "items": responses["views.describe"]}}, "required": []string{"items"}, "additionalProperties": false}
	// Empty pages legitimately satisfy every history item schema.
	responses["devices.history"] = schema{"anyOf": []schema{responses["devices.history"], of(reflect.TypeFor[displayread.Page[displayread.DeviceInstallation]]()), of(reflect.TypeFor[displayread.Page[displayread.DeviceOutput]]())}}
	for _, value := range []any{display.ReadScopeOptionsInput{}, display.ReadScopePreviewInput{}, display.ReadScopePreview{}, displayread.ScopeOptionsPage{}} {
		of(reflect.TypeOf(value))
	}

	responses["map.basemaps"] = schema{"type": "object", "properties": schema{"items": schema{"type": "array", "items": of(reflect.TypeFor[displayread.Basemap]())}}, "required": []string{"items"}, "additionalProperties": false}
	for _, name := range []string{"ReadChannelStatisticPage", "ReadObservationPage", "ReadTrendSeriesPage"} {
		p := defs[name]["properties"].(map[string]any)
		p["filters"] = schema{"type": "object", "properties": schema{"range": schema{"anyOf": []schema{of(reflect.TypeFor[displayread.Window]()), {"type": "null"}}}, "all_history": schema{"type": "boolean"}, "qualities": schema{"type": "array", "items": schema{"type": "string"}}, "timezone": schema{"type": "string"}}, "required": []string{"range", "all_history", "qualities", "timezone"}, "additionalProperties": false}
		p["monitoring_snapshot"] = of(reflect.TypeFor[displayread.MonitoringSnapshot]())
		p["coverage"] = of(reflect.TypeFor[displayread.Page[displayread.CoverageSummary]]())
		p["effective_windows"] = of(reflect.TypeFor[displayread.Page[displayread.WindowSummary]]())
	}
	defs["ReadChannelStatisticPage"]["properties"].(map[string]any)["items"] = schema{"type": "array", "items": schema{"anyOf": []schema{of(reflect.TypeFor[monitoring.ChannelStatistic]()), of(reflect.TypeFor[monitoring.CalendarStatistic]())}}}
	for _, name := range []string{"ReadResourceSummaryPage", "ReadResourceTypePage"} {
		props := defs[name]["properties"].(map[string]any)
		props["structure_model"] = schema{"enum": []int{1, 2}}
		props["structure_projection"] = schema{"enum": []string{"legacy", "canonical"}}
	}
	defs["ReadFacilityMapItemPage"]["properties"].(map[string]any)["position_counts"] = schema{"type": "object", "additionalProperties": schema{"type": "string"}}
	defs["ReadFacilityMapItemPage"]["properties"].(map[string]any)["matched_count"] = schema{"type": "string"}
	for name, definition := range defs {
		props, ok := definition["properties"].(map[string]any)
		if !ok {
			continue
		}
		for key, value := range props {
			v := value.(schema)
			switch key {
			case "limit":
				v["minimum"] = 1
				v["maximum"] = 200
				if name == "ReadMonitoringRequest" || name == "ReadQueryRequest" {
					v["maximum"] = 1000
				}
				if name == "ReadMapFacilitiesRequest" || name == "ReadMapFeaturesRequest" {
					v["maximum"] = 500
				}
			case "search":
				v["maxLength"] = 120
			case "query_id", "id", "asset_id":
				if strings.HasSuffix(name, "Request") && name != "ReadDefinition" {
					v["format"] = "uuid"
				}
			case "point_ids":
				v["maxItems"] = 100
				v["uniqueItems"] = true
			case "ids", "facility_ids", "type_definition_ids", "manufacturer_ids", "product_ids", "product_version_ids", "output_ids", "channel_ids":
				v["maxItems"] = 500
				v["uniqueItems"] = true
				v["items"] = schema{"type": "string", "format": "uuid"}
			case "field_groups":
				v["uniqueItems"] = true
				v["maxItems"] = 20
				v["items"] = schema{"type": "string", "enum": []string{"device.classification", "statistics.classification", "resource.metadata", "resource.attributes", "resource.position", "device.metadata", "device.serial_number", "device.description", "device.outputs", "device.installations", "source.binding", "history", "spatial.metadata", "spatial.geometry", "spatial.properties", "monitoring.values", "monitoring.quality", "statistics.counts"}}
			case "targets":
				v["minItems"] = 1
				v["maxItems"] = 16
			case "from", "to":
				v["format"] = "date-time"
			case "cursor":
				v["maxLength"] = 4096
			case "structure_model":
				v["enum"] = []int{1, 2}
			case "domain":
				v["enum"] = []string{"resources", "channels", "devices", "spatial"}
			}
		}
	}
	enum := func(name, field string, values ...string) {
		defs[name]["properties"].(map[string]any)[field].(schema)["enum"] = values
	}
	enum("ReadReadScopePreviewInput", "purpose", "config", "grant")
	enum("ReadReadScopeOptionsInput", "purpose", "config", "grant")
	enum("ReadDeviceCategoryStatisticsRequest", "population", "authorized", "monitoring_bound")
	enum("ReadDeviceCategoryStatisticsRequest", "mode", "direct", "inclusive", "frontier")
	enum("ReadSelector", "kind", "project", "facilities", "ids")
	enum("ReadResourceTreeRequest", "mode", "root", "children", "path")
	enum("ReadDeviceHistoryRequest", "kind", "installation", "device_change", "output_generation")
	enum("ReadResourceStatisticsRequest", "group_by", "none", "facility_type", "facility")
	enum("ReadDeviceStatisticsRequest", "group_by", "none", "manufacturer", "product", "product_model", "facility_type", "facility", "monitoring_section", "borehole")
	enum("ReadDeviceStatisticsRequest", "membership", "nearest", "subtree")
	enum("ReadDeviceFilters", "category_scope", "exact", "descendants")
	enum("ReadDeviceFilters", "category_group", "sensor", "non_sensor", "unclassified", "unavailable")
	enum("ReadDeviceStatisticsRequest", "population", "authorized", "monitoring_bound")
	enum("ReadMonitoringStatisticsRequest", "group_by", "none", "facility_type", "facility", "monitoring_section", "borehole")
	enum("ReadMonitoringStatisticsRequest", "membership", "nearest", "subtree")
	enum("ReadStructureStatisticsRequest", "group_by", "none", "facility_type", "facility", "monitoring_section", "borehole")
	enum("ReadStructureStatisticsRequest", "membership", "nearest", "subtree")
	enum("ReadDrilldownRequest", "basis", "raw_active", "raw_samples", "derived_logical", "derived_samples", "resource_count", "device_count")
	enum("ReadDrilldownRequest", "purpose", "details", "trends", "statistics", "resources", "devices")
	enum("ReadQueryRequest", "section", "coverage", "effective_windows")
	enum("ReadMetricRef", "kind", "engineering", "derived")
	enum("ReadRef", "kind", "facility", "component", "monitoring_section", "borehole", "point", "channel", "facility_type", "manufacturer", "product")
	enum("ReadResourceSummary", "kind", "facility", "component", "monitoring_section", "borehole", "point", "channel")
	enum("ReadResourceDetailRequest", "kind", "facility", "component", "monitoring_section", "borehole", "point", "channel")
	enum("ReadResourceSummary", "visibility", "full", "context")
	enum("ReadResourceSummary", "status", "normal", "retired")
	enum("ReadResourceDetail", "structure_projection", "legacy", "canonical")
	enum("ReadResourceDetail", "borehole_position_state", "unassigned", "restricted", "not_recorded", "provided", "basis_needs_review")
	enum("ReadClassification", "state", "classified", "unclassified", "unavailable")
	enum("ReadClassification", "origin", "product_default", "device_override")
	enum("ReadClassification", "category_status", "active", "inactive")
	enum("ReadCategory", "status", "active", "inactive")
	enum("ReadCategoryGroup", "visibility", "full", "unclassified", "unavailable", "outside_frontier")
	enum("ReadStructureGroup", "visibility", "full", "restricted", "unassigned")
	enum("ReadChannelSummary", "source_kind", "direct", "derived")
	enum("ReadAssociationGeometry", "kind", "region", "path", "plane")
	// Resource identity and channel ownership have distinct closed kind sets.
	for name, kinds := range map[string][]string{"ReadStructureRef": {"facility", "component", "monitoring_section", "borehole", "point", "channel"}, "ReadOwnerRef": {"facility", "component", "monitoring_section", "borehole", "point"}} {
		defs[name] = schema{"type": "object", "properties": schema{"kind": schema{"type": "string", "enum": kinds}, "id": schema{"type": "string", "format": "uuid"}}, "required": []string{"kind", "id"}, "additionalProperties": false}
	}
	defs["ReadChannelSummary"]["properties"].(map[string]any)["owner_ref"] = schema{"$ref": "#/$defs/ReadOwnerRef"}
	for _, name := range []string{"ReadSelector", "ReadChannelFilters"} {
		defs[name]["properties"].(map[string]any)["owner_refs"] = schema{"type": "array", "items": schema{"$ref": "#/$defs/ReadOwnerRef"}, "maxItems": 500}
	}
	for _, field := range []string{"parent_ref", "owner_ref"} {
		defs["ReadResourceSummary"]["properties"].(map[string]any)[field] = schema{"anyOf": []schema{{"$ref": "#/$defs/ReadStructureRef"}, {"type": "null"}}}
	}
	defs["ReadViewSchema"]["required"] = []string{"domain"}
	defs["ReadViewSchema"]["properties"].(map[string]any)["purpose"].(schema)["maxLength"] = 240
	defs["ReadSpatialFileRequest"]["properties"].(map[string]any)["file_id"].(schema)["pattern"] = displayread.NamePattern.String()
	target := defs["ReadTarget"]
	target["oneOf"] = []schema{{"required": []string{"view"}, "not": schema{"required": []string{"selection"}}}, {"required": []string{"selection"}, "not": schema{"required": []string{"view"}}}}
	result := schema{"$schema": "https://json-schema.org/draft/2020-12/schema", "$id": "urn:infrapro:display:reads:1", "$defs": defs, "x-method-requests": methods, "x-method-data": responses}
	out, e := json.MarshalIndent(result, "", "  ")
	must(e)
	fmt.Println(string(out))
}
func must(e error) {
	if e != nil {
		panic(e)
	}
}
