package displayread

import (
	"context"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
)

type Plan struct {
	Authority   Authority
	Method      string
	Request     any
	Scopes      map[string]Scope
	Evidence    Evidence
	Query       QueryInfo
	ParentQuery string
}

func (p Plan) Actor() Authority { return p.Authority }

// ReadSubject retains live authorization callbacks and clips selected channels
// to this immutable query's windows. Recursive dependencies still undergo the
// base subject's independently delegated input qualification.
func (p Plan) ReadSubject() *access.ReadSubject {
	sub := *p.Authority.Subject
	limits := map[string][]access.ReadWindow{}
	for _, scope := range p.Scopes {
		if scope.Domain == "channels" {
			for id, windows := range scope.Windows {
				limits[id] = append(limits[id], windows...)
			}
		}
	}
	clip := func(base func(context.Context, pgx.Tx, []string) (map[string][]access.ReadWindow, error)) func(context.Context, pgx.Tx, []string) (map[string][]access.ReadWindow, error) {
		return func(ctx context.Context, tx pgx.Tx, ids []string) (map[string][]access.ReadWindow, error) {
			if base == nil {
				return nil, access.ErrDenied
			}
			values, e := base(ctx, tx, ids)
			if e != nil {
				return nil, e
			}
			for _, id := range ids {
				if ceiling, ok := limits[id]; ok {
					clipped := []access.ReadWindow{}
					for _, w := range values[id] {
						for _, c := range ceiling {
							if v, ok := Intersect(w, c); ok {
								clipped = append(clipped, v)
							}
						}
					}
					values[id] = Union(clipped)
				}
			}
			return values, nil
		}
	}
	sub.Windows = clip(sub.Windows)
	if sub.HistoryWindows != nil {
		sub.HistoryWindows = clip(sub.HistoryWindows)
	}
	return &sub
}

type need struct {
	key, domain     string
	target          Target
	fields, minimum []string
}

func needs(method string, request any) []need {
	out := []need{}
	add := func(key, domain string, t *Target, fields []string, minimum ...string) {
		if t != nil {
			out = append(out, need{key, domain, *t, fields, minimum})
		}
	}
	resource := func(t *Target) { add("resource_target", "resources", t, nil, "resource.metadata") }
	channel := func(t *Target) { add("channel_target", "channels", t, nil, "resource.metadata") }
	device := func(t *Target) { add("device_target", "devices", t, nil, "device.metadata", "device.outputs") }
	switch q := request.(type) {
	case *ResourcesRequest:
		add("target", "resources", &q.Target, q.FieldGroups, "resource.metadata")
	case *ResourceDetailRequest:
		add("target", "resources", &q.Target, q.FieldGroups, "resource.metadata")
	case *ResourceTreeRequest:
		add("target", "resources", &q.Target, q.FieldGroups, "resource.metadata")
	case *DevicesRequest:
		minimum := []string{"device.metadata"}
		if q.Filters.CategoryID != "" || q.Filters.CategoryGroup != "" || q.Filters.DictionaryReleaseID != "" {
			minimum = append(minimum, "device.classification")
		}
		add("target", "devices", &q.Target, q.FieldGroups, minimum...)
		resource(q.ResourceTarget)
		add("channel_target", "channels", q.ChannelTarget, nil, "source.binding")
	case *DeviceGetRequest:
		add("target", "devices", &q.Target, q.FieldGroups, "device.metadata")
		resource(q.ResourceTarget)
	case *DeviceOutputsRequest:
		add("target", "devices", &q.Target, q.FieldGroups, "device.metadata", "device.outputs")
	case *DeviceHistoryRequest:
		minimum := []string{"device.metadata", "history"}
		if q.Kind == "output_generation" {
			minimum = append(minimum, "device.outputs")
		}
		if q.Kind == "installation" {
			minimum = append(minimum, "device.installations")
		}
		add("target", "devices", &q.Target, q.FieldGroups, minimum...)
		resource(q.ResourceTarget)
	case *SourceBindingsRequest:
		add("target", "channels", &q.Target, q.FieldGroups, "source.binding")
		device(q.DeviceTarget)
	case *SourceHistoryRequest:
		add("target", "channels", &q.Target, q.FieldGroups, "source.binding", "history")
		add("device_target", "devices", q.DeviceTarget, nil, "device.metadata", "device.outputs", "history")
	case *MapFacilitiesRequest:
		add("target", "resources", &q.Target, q.FieldGroups, "resource.metadata", "resource.position")
	case *MapFeaturesRequest:
		minimum := []string{"spatial.metadata", "spatial.geometry"}
		if q.Search != "" {
			minimum = append(minimum, "spatial.properties")
		}
		add("target", "spatial", &q.Target, q.FieldGroups, minimum...)
		resource(q.ResourceTarget)
	case *SpatialAssetsRequest:
		add("target", "spatial", &q.Target, q.FieldGroups, "spatial.metadata")
		resource(q.ResourceTarget)
	case *SpatialVersionsRequest:
		add("target", "spatial", &q.Target, q.FieldGroups, "spatial.metadata")
	case *SpatialVersionRequest:
		minimum := []string{"spatial.metadata"}
		if method != "spatial.manifest" {
			minimum = append(minimum, "spatial.geometry")
		}
		add("target", "spatial", &q.Target, q.FieldGroups, minimum...)
		resource(q.ResourceTarget)
	case *SpatialLocationsRequest:
		add("target", "resources", &q.Target, q.FieldGroups, "resource.metadata")
		add("spatial_target", "spatial", &q.SpatialTarget, nil, "spatial.metadata", "spatial.geometry")
	case *SpatialSettingsRequest:
		add("target", "spatial", &q.Target, q.FieldGroups, "spatial.metadata", "spatial.geometry")
		channel(q.ChannelTarget)
		resource(q.ResourceTarget)
	case *SpatialFileRequest:
		add("target", "spatial", &q.Target, q.FieldGroups, "spatial.metadata", "spatial.geometry")
	case *ChannelsRequest:
		add("target", "channels", &q.Target, q.FieldGroups, "resource.metadata")
	case *MonitoringRequest:
		minimum := []string{"monitoring.values"}
		if q.History {
			minimum = append(minimum, "history")
		}
		add("target", "channels", &q.Target, q.FieldGroups, minimum...)
	case *DeviceCategoryStatisticsRequest:
		add("target", "devices", &q.Target, q.FieldGroups, "statistics.counts", "statistics.classification")
		if q.ChannelTarget != nil {
			add("channel_target", "channels", q.ChannelTarget, nil, "source.binding")
		}
	case *StructureStatisticsRequest:
		add("target", "resources", &q.Target, q.FieldGroups, "statistics.counts")
		if q.ChannelTarget != nil {
			add("channel_target", "channels", q.ChannelTarget, nil, "statistics.counts")
		}
	case *ResourceStatisticsRequest:
		add("target", "resources", &q.Target, q.FieldGroups, "statistics.counts")
		if q.ChannelTarget != nil {
			add("channel_target", "channels", q.ChannelTarget, nil, "statistics.counts")
		}
	case *DeviceStatisticsRequest:
		minimum := []string{"statistics.counts"}
		if q.Filters.CategoryID != "" || q.Filters.CategoryGroup != "" || q.Filters.DictionaryReleaseID != "" {
			minimum = append(minimum, "statistics.classification")
		}
		add("target", "devices", &q.Target, q.FieldGroups, minimum...)
		resource(q.ResourceTarget)
		if q.ChannelTarget != nil {
			add("channel_target", "channels", q.ChannelTarget, nil, "source.binding")
		}
	case *MonitoringStatisticsRequest:
		for i := range q.Targets {
			add(fmt.Sprintf("targets/%d", i), "channels", &q.Targets[i], q.FieldGroups, "statistics.counts")
		}
		resource(q.ResourceTarget)
	}
	return out
}
func resolvePlan(ctx context.Context, tx pgx.Tx, a Authority, method string, request any) (Plan, error) {
	p := Plan{Authority: a, Method: method, Request: request, Scopes: map[string]Scope{}}
	var e error
	p.Evidence, e = a.Validate(ctx, tx, method, nil)
	if e != nil {
		return p, e
	}
	for _, n := range needs(method, request) {
		scope, e := a.Resolve(ctx, tx, n.target, n.domain, n.fields, n.minimum)
		if e != nil {
			return p, e
		}
		if (method == "resources.hierarchy" || method == "statistics.structure") && scope.StructureModel != 2 {
			return p, StructureUpgradeRequired()
		}
		p.Scopes[n.key] = scope
	}
	mergedChannels := map[string]bool{}
	for _, scope := range p.Scopes {
		if scope.Domain == "channels" {
			for _, m := range scope.Members {
				if m.Kind == "channel" {
					mergedChannels[m.ID] = true
				}
			}
		}
	}
	if len(mergedChannels) > MaxMembers {
		return p, Problem("budget_exceeded", "合并读取范围最多 50,000 个通道。", 429)
	}
	// Explicit time bounds are checked against each view, before unioning windows.
	var timeRequest *MonitoringTime
	switch q := request.(type) {
	case *MonitoringRequest:
		timeRequest = &q.MonitoringTime
	case *MonitoringStatisticsRequest:
		timeRequest = &q.MonitoringTime
	}
	if timeRequest != nil && timeRequest.Range != nil {
		w, e := ParseWindow(timeRequest.Range, "/range")
		if e != nil {
			return p, e
		}
		for _, scope := range p.Scopes {
			if scope.Domain != "channels" {
				continue
			}
			for _, windows := range scope.Windows {
				for _, ceiling := range windows {
					if !Covers(ceiling, w) {
						return p, Invalid("/range", "显式时段超过视图或授权上限。")
					}
				}
			}
		}
	}
	if method == "monitoring.trends" && len(p.Scopes["target"].Members) > MaxTrendChannels {
		return p, Problem("budget_exceeded", "趋势最多 50 个通道，请明确收窄选择。", 429)
	}
	return p, nil
}

// InspectTx checks a subscription's current definition without materializing data.
func InspectTx(ctx context.Context, tx pgx.Tx, a Authority, method string, raw []byte) (Plan, error) {
	request, e := DecodeRequest(method, raw)
	if e != nil {
		return Plan{}, e
	}
	if p := paging(request); p != nil && (p.QueryID != "" || p.Cursor != "") {
		return Plan{}, Invalid("/query", "订阅不接受外部查询游标。")
	}
	if q, ok := request.(*MonitoringRequest); ok && q.Target.Selection != "" {
		return Plan{}, Invalid("/target", "订阅需要命名读取视图。")
	}
	return resolvePlan(ctx, tx, a, method, request)
}
func planFields(p Plan) []string {
	fields := []string{}
	for _, s := range p.Scopes {
		fields = append(fields, s.FieldGroups...)
	}
	slices.Sort(fields)
	return slices.Compact(fields)
}
func paging(request any) *Paging {
	switch q := request.(type) {
	case *StructureStatisticsRequest:
		return &q.Paging
	case *CategoriesRequest:
		return &q.Paging
	case *DeviceCategoryStatisticsRequest:
		return &q.Paging
	case *ResourcesRequest:
		return &q.Paging
	case *ResourceTreeRequest:
		return &q.Paging
	case *DevicesRequest:
		return &q.Paging
	case *DeviceOutputsRequest:
		return &q.Paging
	case *DeviceHistoryRequest:
		return &q.Paging
	case *SourceBindingsRequest:
		return &q.Paging
	case *SourceHistoryRequest:
		return &q.Paging
	case *MapFacilitiesRequest:
		return &q.Paging
	case *MapFeaturesRequest:
		return &q.Paging
	case *SpatialAssetsRequest:
		return &q.Paging
	case *SpatialVersionsRequest:
		return &q.Paging
	case *SpatialVersionRequest:
		return &q.Paging
	case *SpatialLocationsRequest:
		return &q.Paging
	case *ChannelsRequest:
		return &q.Paging
	case *MonitoringRequest:
		return &q.Paging
	case *ResourceStatisticsRequest:
		return &q.Paging
	case *DeviceStatisticsRequest:
		return &q.Paging
	case *MonitoringStatisticsRequest:
		return &q.Paging
	}
	return nil
}
func isStatistics(method string) bool { return strings.HasPrefix(method, "statistics.") }
