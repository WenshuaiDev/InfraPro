package displayread

import (
	"encoding/json"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
)

type Paging struct {
	Limit   int    `json:"limit,omitempty"`
	Cursor  string `json:"cursor,omitempty"`
	QueryID string `json:"query_id,omitempty"`
}
type ReadFields struct {
	FieldGroups []string `json:"field_groups,omitempty"`
}
type ResourceFilters struct {
	Kinds       []string `json:"kinds,omitempty"`
	FacilityIDs []string `json:"facility_ids,omitempty"`
	TypeIDs     []string `json:"type_definition_ids,omitempty"`
	Statuses    []string `json:"statuses,omitempty"`
}
type ResourcesRequest struct {
	Paging
	ReadFields
	Target  Target          `json:"target"`
	Search  string          `json:"search,omitempty"`
	Filters ResourceFilters `json:"filters,omitempty"`
	Sort    string          `json:"sort,omitempty"`
}
type ResourceDetailRequest struct {
	ReadFields
	Target Target `json:"target"`
	ID     string `json:"id"`
	Kind   string `json:"kind,omitempty"`
}
type ResourceTreeRequest struct {
	Paging
	ReadFields
	Target Target `json:"target"`
	Mode   string `json:"mode"`
	Parent *Ref   `json:"parent_ref,omitempty"`
	Node   *Ref   `json:"node_ref,omitempty"`
}
type DeviceFilters struct {
	CategoryID          string   `json:"category_id,omitempty"`
	CategoryScope       string   `json:"category_scope,omitempty"`
	CategoryGroup       string   `json:"category_group,omitempty"`
	DictionaryReleaseID string   `json:"dictionary_release_id,omitempty"`
	ManufacturerIDs     []string `json:"manufacturer_ids,omitempty"`
	ProductIDs          []string `json:"product_ids,omitempty"`
	ProductVersionIDs   []string `json:"product_version_ids,omitempty"`
	Statuses            []string `json:"statuses,omitempty"`
	InstallationState   string   `json:"installation_state,omitempty"`
	FacilityIDs         []string `json:"facility_ids,omitempty"`
	Relation            string   `json:"relation,omitempty"`
}
type DevicesRequest struct {
	Paging
	ReadFields
	Target         Target        `json:"target"`
	Search         string        `json:"search,omitempty"`
	Filters        DeviceFilters `json:"filters,omitempty"`
	ResourceTarget *Target       `json:"resource_target,omitempty"`
	ChannelTarget  *Target       `json:"channel_target,omitempty"`
}
type DeviceGetRequest struct {
	ReadFields
	Target         Target  `json:"target"`
	ID             string  `json:"id"`
	ResourceTarget *Target `json:"resource_target,omitempty"`
}
type DeviceOutputsRequest struct {
	Paging
	ReadFields
	Target Target `json:"target"`
	ID     string `json:"id"`
}
type DeviceHistoryRequest struct {
	Paging
	ReadFields
	Target         Target  `json:"target"`
	ID             string  `json:"id"`
	Kind           string  `json:"kind"`
	ResourceTarget *Target `json:"resource_target,omitempty"`
}
type SourceBindingsRequest struct {
	Paging
	ReadFields
	Target       Target   `json:"target"`
	DeviceTarget *Target  `json:"device_target,omitempty"`
	PointIDs     []string `json:"point_ids,omitempty"`
	ChannelIDs   []string `json:"channel_ids,omitempty"`
}
type SourceHistoryRequest struct {
	Paging
	ReadFields
	Target       Target  `json:"target"`
	DeviceTarget *Target `json:"device_target,omitempty"`
	ID           string  `json:"id"`
}
type BBox struct {
	West  float64 `json:"west"`
	South float64 `json:"south"`
	East  float64 `json:"east"`
	North float64 `json:"north"`
}
type MapFacilitiesRequest struct {
	Paging
	ReadFields
	Target        Target          `json:"target"`
	Search        string          `json:"search,omitempty"`
	Filters       ResourceFilters `json:"filters,omitempty"`
	BBox          *BBox           `json:"bbox,omitempty"`
	PositionState string          `json:"position_state,omitempty"`
}
type MapFeaturesRequest struct {
	Paging
	ReadFields
	Target         Target  `json:"target"`
	ID             string  `json:"id"`
	ResourceTarget *Target `json:"resource_target,omitempty"`
	BBox           *BBox   `json:"bbox,omitempty"`
	Search         string  `json:"search,omitempty"`
}
type TileRequest struct {
	BasemapID string `json:"basemap_id"`
	Z         int    `json:"z"`
	X         int    `json:"x"`
	Y         int    `json:"y"`
}
type EmptyRequest struct{}
type SpatialFilters struct {
	FacilityIDs []string `json:"facility_ids,omitempty"`
	AssetKinds  []string `json:"asset_kinds,omitempty"`
	Statuses    []string `json:"statuses,omitempty"`
}
type SpatialAssetsRequest struct {
	Paging
	ReadFields
	Target         Target         `json:"target"`
	Filters        SpatialFilters `json:"filters,omitempty"`
	Search         string         `json:"search,omitempty"`
	ResourceTarget *Target        `json:"resource_target,omitempty"`
}
type SpatialVersionsRequest struct {
	Paging
	ReadFields
	Target  Target `json:"target"`
	AssetID string `json:"asset_id"`
}
type SpatialVersionRequest struct {
	Paging
	ReadFields
	Target         Target  `json:"target"`
	ID             string  `json:"id"`
	ResourceTarget *Target `json:"resource_target,omitempty"`
	Search         string  `json:"search,omitempty"`
}
type SpatialLocationsRequest struct {
	Kind string `json:"kind,omitempty"`
	Paging
	ReadFields
	Target        Target `json:"target"`
	SpatialTarget Target `json:"spatial_target"`
	ID            string `json:"id"`
}
type SpatialSettingsRequest struct {
	ReadFields
	Target         Target  `json:"target"`
	ID             string  `json:"id"`
	ChannelTarget  *Target `json:"channel_target,omitempty"`
	ResourceTarget *Target `json:"resource_target,omitempty"`
}
type SpatialFileRequest struct {
	ReadFields
	Target Target `json:"target"`
	ID     string `json:"id"`
	FileID string `json:"file_id"`
	Offset int64  `json:"offset,omitempty"`
	Length int64  `json:"length,omitempty"`
}
type ChannelFilters struct {
	FacilityIDs []string    `json:"facility_ids,omitempty"`
	Owners      []Ref       `json:"owner_refs,omitempty"`
	SourceKinds []string    `json:"source_kinds,omitempty"`
	Metrics     []MetricRef `json:"metric_refs,omitempty"`
}
type ChannelsRequest struct {
	Paging
	ReadFields
	Target  Target         `json:"target"`
	ID      string         `json:"id,omitempty"`
	Search  string         `json:"search,omitempty"`
	Filters ChannelFilters `json:"filters,omitempty"`
}
type MonitoringTime struct {
	Range         *Window  `json:"range,omitempty"`
	AllHistory    bool     `json:"all_history,omitempty"`
	FollowSeconds int      `json:"follow_seconds,omitempty"`
	Timezone      string   `json:"timezone,omitempty"`
	Qualities     []string `json:"qualities,omitempty"`
}
type MonitoringRequest struct {
	Paging
	ReadFields
	MonitoringTime
	Target     Target         `json:"target"`
	Filters    ChannelFilters `json:"filters,omitempty"`
	PlotWidth  int            `json:"plot_width_px,omitempty"`
	PlotBudget int            `json:"plot_point_budget,omitempty"`
	Period     string         `json:"period,omitempty"`
	History    bool           `json:"history,omitempty"`
}
type ResourceStatisticsRequest struct {
	Paging
	ReadFields
	Target        Target          `json:"target"`
	ChannelTarget *Target         `json:"channel_target,omitempty"`
	Filters       ResourceFilters `json:"filters,omitempty"`
	GroupBy       string          `json:"group_by"`
}
type DeviceStatisticsRequest struct {
	Membership string `json:"membership,omitempty"`
	Paging
	ReadFields
	Target         Target        `json:"target"`
	ChannelTarget  *Target       `json:"channel_target,omitempty"`
	ResourceTarget *Target       `json:"resource_target,omitempty"`
	Population     string        `json:"population"`
	Relation       string        `json:"relation,omitempty"`
	GroupBy        string        `json:"group_by"`
	Filters        DeviceFilters `json:"filters,omitempty"`
}
type MonitoringStatisticsRequest struct {
	Membership string `json:"membership,omitempty"`
	Paging
	ReadFields
	MonitoringTime
	Targets        []Target `json:"targets"`
	SourceKinds    []string `json:"source_kinds"`
	ResourceTarget *Target  `json:"resource_target,omitempty"`
	GroupBy        string   `json:"group_by"`
}
type ViewRequest struct {
	Name string `json:"name"`
}
type QueryRequest struct {
	QueryID string `json:"query_id"`
	Cursor  string `json:"cursor,omitempty"`
	Limit   int    `json:"limit,omitempty"`
	Section string `json:"section,omitempty"`
}
type QueryControlRequest struct {
	QueryID string `json:"query_id"`
}
type DrilldownRequest struct {
	QueryID  string `json:"query_id"`
	GroupKey string `json:"group_key"`
	Basis    string `json:"basis"`
	Purpose  string `json:"purpose"`
}

// Method-specific wire DTOs are decoded before any scope resolution or I/O.
func NewRequest(method string) (any, error) {
	var request any
	switch method {
	case "views.list", "map.basemaps":
		request = &EmptyRequest{}
	case "views.describe":
		request = &ViewRequest{}
	case "resources.types", "resources.query":
		request = &ResourcesRequest{}
	case "resources.detail":
		request = &ResourceDetailRequest{}
	case "resources.tree", "resources.hierarchy":
		request = &ResourceTreeRequest{}
	case "devices.list":
		request = &DevicesRequest{}
	case "devices.get":
		request = &DeviceGetRequest{}
	case "devices.outputs":
		request = &DeviceOutputsRequest{}
	case "devices.history":
		request = &DeviceHistoryRequest{}
	case "sources.bindings":
		request = &SourceBindingsRequest{}
	case "sources.bindingHistory":
		request = &SourceHistoryRequest{}
	case "map.facilities":
		request = &MapFacilitiesRequest{}
	case "map.features":
		request = &MapFeaturesRequest{}
	case "map.readTile":
		request = &TileRequest{}
	case "spatial.assets":
		request = &SpatialAssetsRequest{}
	case "spatial.versions":
		request = &SpatialVersionsRequest{}
	case "spatial.manifest", "spatial.objects", "spatial.placements", "spatial.componentLinks":
		request = &SpatialVersionRequest{}
	case "spatial.locations":
		request = &SpatialLocationsRequest{}
	case "spatial.settings":
		request = &SpatialSettingsRequest{}
	case "spatial.readFile":
		request = &SpatialFileRequest{}
	case "channels.list", "channels.get":
		request = &ChannelsRequest{}
	case "monitoring.latest", "monitoring.details", "monitoring.trends", "monitoring.statistics", "monitoring.overview":
		request = &MonitoringRequest{}
	case "classification.categories":
		request = &CategoriesRequest{}
	case "statistics.deviceCategories":
		request = &DeviceCategoryStatisticsRequest{}
	case "statistics.structure":
		request = &StructureStatisticsRequest{}
	case "statistics.resources":
		request = &ResourceStatisticsRequest{}
	case "statistics.devices":
		request = &DeviceStatisticsRequest{}
	case "statistics.monitoring":
		request = &MonitoringStatisticsRequest{}
	case "queries.get":
		request = &QueryRequest{}
	case "queries.cancel", "queries.release":
		request = &QueryControlRequest{}
	case "queries.drilldown":
		request = &DrilldownRequest{}
	default:
		return nil, Invalid("/method", "未知读取方法。")
	}
	return request, nil
}

func DecodeRequest(method string, raw json.RawMessage) (any, error) {
	request, e := NewRequest(method)
	if e != nil {
		return nil, e
	}
	if len(raw) == 0 {
		raw = []byte(`{}`)
	}
	if e := Decode(raw, request); e != nil {
		return nil, e
	}
	if e := validateRequest(method, request); e != nil {
		return nil, e
	}
	return request, nil
}
func validateTarget(t Target, selection bool, path string) error {
	if t.Selection != "" {
		if !selection || t.View != "" || len(t.Selection) > 200 {
			return Invalid(path, "下钻选择不能与视图或标识混用。")
		}
		if t.IDs != nil && len(t.IDs) == 0 {
			return Invalid(path+"/ids", "显式标识不能为空。")
		}
		return ValidateIDs(t.IDs, path+"/ids", MaxIDs)
	}
	if !NamePattern.MatchString(t.View) {
		return Invalid(path+"/view", "必须指定命名读取视图。")
	}
	if t.IDs != nil && len(t.IDs) == 0 {
		return Invalid(path+"/ids", "显式标识不能为空。")
	}
	return ValidateIDs(t.IDs, path+"/ids", MaxIDs)
}
func validatePaging(p *Paging, max int) error {
	if p.Limit == 0 {
		p.Limit = PageSize
	}
	if p.Limit < 1 || p.Limit > max {
		return Invalid("/limit", "分页大小超过方法预算。")
	}
	if len(p.Cursor) > 4096 || p.Cursor != "" && p.QueryID == "" || p.QueryID != "" && !access.ValidID(p.QueryID) {
		return Invalid("/cursor", "分页须携带原查询及有效游标。")
	}
	return nil
}
func validateTime(t *MonitoringTime, latest bool, selection bool) error {
	n := 0
	if t.Range != nil {
		n++
	}
	if t.AllHistory {
		n++
	}
	if t.FollowSeconds != 0 {
		n++
	}
	if selection {
		if n != 0 || t.Qualities != nil || t.Timezone != "" {
			return Invalid("/target/selection", "下钻沿用父查询的时间和质量。")
		}
		return nil
	}
	if latest && n == 0 {
		t.AllHistory = true
		n = 1
	}
	if n != 1 {
		return Invalid("/range", "range、all_history、follow_seconds 必须明确选择一种。")
	}
	if t.Range != nil {
		if t.Range.From == nil || t.Range.To == nil {
			return Invalid("/range", "明确时段需要起止时刻。")
		}
		if _, e := ParseWindow(t.Range, "/range"); e != nil {
			return e
		}
	}
	if t.FollowSeconds < 0 || t.FollowSeconds > 31*86400 {
		return Invalid("/follow_seconds", "跟随窗口须为 1–2678400 秒。")
	}
	if t.Timezone == "" {
		t.Timezone = "Asia/Shanghai"
	}
	if _, e := time.LoadLocation(t.Timezone); e != nil {
		return Invalid("/timezone", "需要有效 IANA 时区。")
	}
	if len(t.Qualities) == 0 {
		t.Qualities = []string{"good", "unmarked"}
		if latest {
			t.Qualities = []string{"good", "unmarked", "suspect", "bad"}
		}
	}
	return enums(t.Qualities, []string{"good", "unmarked", "suspect", "bad"}, "/qualities")
}
func validateBBox(b *BBox) error {
	if b == nil {
		return nil
	}
	for _, n := range []float64{b.West, b.East, b.South, b.North} {
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return Invalid("/bbox", "坐标必须为有限数。")
		}
	}
	if b.West < -180 || b.West > 180 || b.East < -180 || b.East > 180 || b.South < -90 || b.North > 90 || b.South >= b.North || b.West == b.East {
		return Invalid("/bbox", "地理范围无效。")
	}
	return nil
}
func (b BBox) Contains(lon, lat float64) bool {
	return lat >= b.South && lat <= b.North && (b.West < b.East && lon >= b.West && lon <= b.East || b.West > b.East && (lon >= b.West || lon <= b.East))
}
func validateRequest(method string, request any) error {
	var page *Paging
	var target *Target
	var fields []string
	var id, search string
	var others []*Target
	max := MaxPageSize
	resourceFilter := func(f ResourceFilters) error {
		s := Selector{Kind: "project", FacilityIDs: f.FacilityIDs, TypeIDs: f.TypeIDs, Kinds: f.Kinds, Statuses: f.Statuses}
		return ValidateSelector("resources", &s, "/filters")
	}
	deviceFilter := func(f DeviceFilters) error {
		if f.CategoryID != "" && !access.ValidID(f.CategoryID) || f.DictionaryReleaseID != "" && !access.ValidID(f.DictionaryReleaseID) || !slices.Contains([]string{"", "exact", "descendants"}, f.CategoryScope) || !slices.Contains([]string{"", "sensor", "non_sensor", "unclassified", "unavailable"}, f.CategoryGroup) {
			return Invalid("/filters/category_id", "分类筛选无效。")
		}
		if f.CategoryScope != "" && f.CategoryID == "" {
			return Invalid("/filters/category_scope", "分类范围需要明确分类。")
		}
		if e := enums([]string{f.InstallationState}, []string{"", "installed", "uninstalled"}, "/filters/installation_state"); e != nil {
			return e
		}
		if e := enums([]string{f.Relation}, []string{"", "installation", "source"}, "/filters/relation"); e != nil {
			return e
		}
		if len(f.FacilityIDs) > 0 && f.Relation == "" {
			return Invalid("/filters/relation", "设施筛选须明确安装或来源关系。")
		}
		if e := ValidateIDs(f.FacilityIDs, "/filters/facility_ids", MaxIDs); e != nil {
			return e
		}
		if e := ValidateIDs(f.ProductVersionIDs, "/filters/product_version_ids", MaxIDs); e != nil {
			return e
		}
		s := Selector{Kind: "project", ManufacturerIDs: f.ManufacturerIDs, ProductIDs: f.ProductIDs, Statuses: f.Statuses}
		return ValidateSelector("devices", &s, "/filters")
	}
	channelFilter := func(f ChannelFilters) error {
		s := Selector{Kind: "ids", IDs: []string{"00000000-0000-4000-8000-000000000001"}, FacilityIDs: f.FacilityIDs, Owners: f.Owners, SourceKinds: f.SourceKinds, Metrics: f.Metrics}
		return ValidateSelector("channels", &s, "/filters")
	}
	switch q := request.(type) {
	case *EmptyRequest:
		return nil
	case *ViewRequest:
		if !NamePattern.MatchString(q.Name) {
			return Invalid("/name", "视图名称无效。")
		}
		return nil
	case *ResourcesRequest:
		page, target, fields, search = &q.Paging, &q.Target, q.FieldGroups, q.Search
		if !slices.Contains([]string{"", "code", "name", "created_at"}, q.Sort) {
			return Invalid("/sort", "排序不支持。")
		}
		if e := resourceFilter(q.Filters); e != nil {
			return e
		}
	case *ResourceDetailRequest:
		target, fields, id = &q.Target, q.FieldGroups, q.ID
		if q.Kind != "" && !slices.Contains([]string{"facility", "component", "monitoring_section", "borehole", "point", "channel"}, q.Kind) {
			return Invalid("/kind", "资源种类无效。")
		}
	case *ResourceTreeRequest:
		page, target, fields = &q.Paging, &q.Target, q.FieldGroups
		if !slices.Contains([]string{"root", "children", "path"}, q.Mode) || q.Mode == "root" && (q.Parent != nil || q.Node != nil) || q.Mode == "children" && (q.Parent == nil || q.Node != nil) || q.Mode == "path" && (q.Node == nil || q.Parent != nil) {
			return Invalid("/mode", "树模式及节点参数不匹配。")
		}
		for _, r := range []*Ref{q.Parent, q.Node} {
			if r != nil && (!access.ValidID(r.ID) || !slices.Contains([]string{"facility", "component", "monitoring_section", "borehole", "point", "channel"}, r.Kind)) {
				return Invalid("/node_ref", "资源引用无效。")
			}
		}
	case *DevicesRequest:
		page, target, fields, search = &q.Paging, &q.Target, q.FieldGroups, q.Search
		others = []*Target{q.ResourceTarget, q.ChannelTarget}
		if q.Target.Selection != "" && (q.ResourceTarget != nil || q.ChannelTarget != nil || q.Filters.Relation != "" || q.Filters.InstallationState != "" || len(q.Filters.FacilityIDs) > 0) {
			return Invalid("/filters", "下钻沿用固定成员，不接受新的安装或来源关系筛选。")
		}
		if e := deviceFilter(q.Filters); e != nil {
			return e
		}
		if q.Filters.Relation == "source" && q.ChannelTarget == nil {
			return Invalid("/channel_target", "来源关系需要通道视图。")
		}
		if len(q.Filters.FacilityIDs) > 0 && q.ResourceTarget == nil {
			return Invalid("/resource_target", "设施关系筛选需要资源视图。")
		}
	case *DeviceGetRequest:
		target, fields, id = &q.Target, q.FieldGroups, q.ID
		others = []*Target{q.ResourceTarget}
	case *DeviceOutputsRequest:
		page, target, fields, id = &q.Paging, &q.Target, q.FieldGroups, q.ID
	case *DeviceHistoryRequest:
		page, target, fields, id = &q.Paging, &q.Target, q.FieldGroups, q.ID
		others = []*Target{q.ResourceTarget}
		if !slices.Contains([]string{"installation", "device_change", "output_generation"}, q.Kind) {
			return Invalid("/kind", "历史类型无效。")
		}
	case *SourceBindingsRequest:
		page, target, fields = &q.Paging, &q.Target, q.FieldGroups
		others = []*Target{q.DeviceTarget}
		if (len(q.PointIDs) == 0) == (len(q.ChannelIDs) == 0) {
			return Invalid("/point_ids", "必须选择测点批次或通道批次。")
		}
		if e := ValidateIDs(q.PointIDs, "/point_ids", 100); e != nil {
			return e
		}
		if e := ValidateIDs(q.ChannelIDs, "/channel_ids", MaxIDs); e != nil {
			return e
		}
	case *SourceHistoryRequest:
		page, target, fields, id = &q.Paging, &q.Target, q.FieldGroups, q.ID
		others = []*Target{q.DeviceTarget}
	case *MapFacilitiesRequest:
		page, target, fields, search = &q.Paging, &q.Target, q.FieldGroups, q.Search
		max = MaxMapPageSize
		if !slices.Contains([]string{"", "all", "located", "missing", "unsupported"}, q.PositionState) {
			return Invalid("/position_state", "位置状态无效。")
		}
		if q.BBox != nil && q.PositionState != "" && q.PositionState != "all" && q.PositionState != "located" {
			return Invalid("/bbox", "地理范围只能匹配可定位设施。")
		}
		if e := validateBBox(q.BBox); e != nil {
			return e
		}
		if e := resourceFilter(q.Filters); e != nil {
			return e
		}
	case *MapFeaturesRequest:
		page, target, fields, id, search = &q.Paging, &q.Target, q.FieldGroups, q.ID, q.Search
		others = []*Target{q.ResourceTarget}
		max = MaxMapPageSize
		if e := validateBBox(q.BBox); e != nil {
			return e
		}
	case *TileRequest:
		if q.BasemapID != "local" || q.Z < 0 || q.Z > 18 || q.X < 0 || q.Y < 0 || q.X >= 1<<q.Z || q.Y >= 1<<q.Z {
			return Invalid("/basemap_id", "仅允许本地示意底图和合法 z/x/y。")
		}
		return nil
	case *SpatialAssetsRequest:
		page, target, fields, search = &q.Paging, &q.Target, q.FieldGroups, q.Search
		others = []*Target{q.ResourceTarget}
		s := Selector{Kind: "project", FacilityIDs: q.Filters.FacilityIDs, AssetKinds: q.Filters.AssetKinds}
		if e := ValidateSelector("spatial", &s, "/filters"); e != nil {
			return e
		}
		if e := enums(q.Filters.Statuses, []string{"normal", "retired"}, "/filters/statuses"); e != nil {
			return e
		}
	case *SpatialVersionsRequest:
		page, target, fields, id = &q.Paging, &q.Target, q.FieldGroups, q.AssetID
	case *SpatialVersionRequest:
		page, target, fields, id, search = &q.Paging, &q.Target, q.FieldGroups, q.ID, q.Search
		others = []*Target{q.ResourceTarget}
		if (method == "spatial.componentLinks" || method == "spatial.placements") && q.ResourceTarget == nil {
			return Invalid("/resource_target", "业务关联需要资源视图。")
		}
	case *SpatialLocationsRequest:
		if !slices.Contains([]string{"", "facility", "component", "monitoring_section", "borehole", "point"}, q.Kind) {
			return Invalid("/kind", "空间定位资源种类无效。")
		}
		page, target, fields, id = &q.Paging, &q.Target, q.FieldGroups, q.ID
		others = []*Target{&q.SpatialTarget}
	case *SpatialSettingsRequest:
		target, fields, id = &q.Target, q.FieldGroups, q.ID
		others = []*Target{q.ResourceTarget, q.ChannelTarget}
	case *SpatialFileRequest:
		target, fields, id = &q.Target, q.FieldGroups, q.ID
		if q.Offset < 0 || q.Length < 0 || q.Length > MaxFileBytes || !NamePattern.MatchString(q.FileID) {
			return Invalid("/file_id", "文件或分块范围无效。")
		}
	case *ChannelsRequest:
		page, target, fields, search = &q.Paging, &q.Target, q.FieldGroups, q.Search
		if method == "channels.get" {
			id = q.ID
		}
		if e := channelFilter(q.Filters); e != nil {
			return e
		}
	case *MonitoringRequest:
		page, target, fields = &q.Paging, &q.Target, q.FieldGroups
		if method == "monitoring.details" {
			max = MaxDetails
		}
		if e := validateTime(&q.MonitoringTime, method == "monitoring.latest", q.Target.Selection != ""); e != nil {
			return e
		}
		if e := channelFilter(q.Filters); e != nil {
			return e
		}
		if q.Target.Selection != "" && Hash(q.Filters) != Hash(ChannelFilters{}) {
			return Invalid("/filters", "下钻选择不可更换筛选。")
		}
		if q.History && method != "monitoring.details" {
			return Invalid("/history", "历史明细只适用于 details。")
		}
		if q.PlotWidth < 0 || q.PlotWidth > 16384 || q.PlotBudget < 0 || q.PlotBudget > 100000 {
			return Invalid("/plot_point_budget", "趋势预算无效。")
		}
		if !slices.Contains([]string{"", "hour", "day", "month", "year"}, q.Period) {
			return Invalid("/period", "统计周期无效。")
		}
	case *CategoriesRequest:
		page, search = &q.Paging, q.Search
		if q.ReleaseID != "" && !access.ValidID(q.ReleaseID) {
			return Invalid("/dictionary_release_id", "Invalid dictionary release.")
		}
	case *DeviceCategoryStatisticsRequest:
		page, target, fields = &q.Paging, &q.Target, q.FieldGroups
		others = []*Target{q.ChannelTarget}
		if !slices.Contains([]string{"authorized", "monitoring_bound"}, q.Population) {
			return Invalid("/population", "Choose the authorized or monitoring_bound population.")
		}
		if q.Population == "monitoring_bound" && q.ChannelTarget == nil {
			return Invalid("/channel_target", "Monitoring-bound population needs a channel view.")
		}
		if !slices.Contains([]string{"direct", "inclusive", "frontier"}, q.Mode) {
			return Invalid("/mode", "Choose direct, inclusive, or frontier.")
		}
		if q.ReleaseID != "" && !access.ValidID(q.ReleaseID) {
			return Invalid("/dictionary_release_id", "Invalid dictionary release.")
		}
		if e := ValidateIDs(q.FrontierIDs, "/frontier_ids", MaxIDs); e != nil {
			return e
		}
		if (q.Mode == "frontier") != (len(q.FrontierIDs) > 0) {
			return Invalid("/frontier_ids", "Frontier mode requires an explicit nonempty frontier.")
		}
		if e := deviceFilter(q.Filters); e != nil {
			return e
		}
		if len(q.Filters.FacilityIDs) > 0 || q.Filters.Relation != "" {
			return Invalid("/filters", "Category statistics use the selected device and channel populations.")
		}
	case *StructureStatisticsRequest:
		page, target, fields = &q.Paging, &q.Target, q.FieldGroups
		others = []*Target{q.ChannelTarget}
		if !slices.Contains([]string{"none", "facility", "facility_type", "monitoring_section", "borehole"}, q.GroupBy) {
			return Invalid("/group_by", "Unsupported structure grouping.")
		}
		if q.Membership == "" {
			q.Membership = "nearest"
		}
		if !slices.Contains([]string{"nearest", "subtree"}, q.Membership) {
			return Invalid("/membership", "Choose nearest or subtree.")
		}
		if e := resourceFilter(q.Filters); e != nil {
			return e
		}
	case *ResourceStatisticsRequest:
		page, target, fields = &q.Paging, &q.Target, q.FieldGroups
		others = []*Target{q.ChannelTarget}
		if !slices.Contains([]string{"none", "facility_type", "facility"}, q.GroupBy) {
			return Invalid("/group_by", "资源分组无效。")
		}
		if e := resourceFilter(q.Filters); e != nil {
			return e
		}
	case *DeviceStatisticsRequest:
		page, target, fields = &q.Paging, &q.Target, q.FieldGroups
		others = []*Target{q.ResourceTarget, q.ChannelTarget}
		if !slices.Contains([]string{"authorized", "monitoring_bound"}, q.Population) {
			return Invalid("/population", "必须选择设备统计口径。")
		}
		if !slices.Contains([]string{"none", "manufacturer", "product", "product_model", "facility_type", "facility", "monitoring_section", "borehole"}, q.GroupBy) {
			return Invalid("/group_by", "设备分组无效。")
		}
		facility := strings.HasPrefix(q.GroupBy, "facility") || NewStructureKind(q.GroupBy)
		if NewStructureKind(q.GroupBy) {
			if q.Membership == "" {
				q.Membership = "nearest"
			}
			if !slices.Contains([]string{"nearest", "subtree"}, q.Membership) {
				return Invalid("/membership", "请选择最近归属或子树包含。")
			}
		} else if q.Membership != "" {
			return Invalid("/membership", "此分组不支持层级口径。")
		}
		if facility && (q.ResourceTarget == nil || !slices.Contains([]string{"installation", "source"}, q.Relation)) || q.Population == "monitoring_bound" && (q.ChannelTarget == nil || q.Relation != "" && q.Relation != "source") || q.Relation == "source" && q.ChannelTarget == nil {
			return Invalid("/relation", "关系统计需要独立资源/通道视图。")
		}
		if e := deviceFilter(q.Filters); e != nil {
			return e
		}
		if len(q.Filters.FacilityIDs) > 0 && q.ResourceTarget == nil || q.Filters.Relation == "source" && q.ChannelTarget == nil {
			return Invalid("/filters/relation", "关系筛选需要独立资源/通道视图。")
		}
	case *MonitoringStatisticsRequest:
		page, fields = &q.Paging, q.FieldGroups
		others = []*Target{q.ResourceTarget}
		if len(q.Targets) < 1 || len(q.Targets) > 16 {
			return Invalid("/targets", "选择 1–16 个通道视图。")
		}
		for i := range q.Targets {
			others = append(others, &q.Targets[i])
		}
		if len(q.SourceKinds) == 0 {
			return Invalid("/source_kinds", "必须明确原始/派生种类。")
		}
		if e := enums(q.SourceKinds, []string{"direct", "derived"}, "/source_kinds"); e != nil {
			return e
		}
		if !slices.Contains([]string{"none", "facility_type", "facility", "monitoring_section", "borehole"}, q.GroupBy) {
			return Invalid("/group_by", "监测分组无效。")
		}
		if q.Membership == "" && NewStructureKind(q.GroupBy) {
			q.Membership = "nearest"
		}
		if q.Membership != "" && (!NewStructureKind(q.GroupBy) || !slices.Contains([]string{"nearest", "subtree"}, q.Membership)) {
			return Invalid("/membership", "Membership applies to structure grouping only.")
		}
		if q.GroupBy != "none" && q.ResourceTarget == nil {
			return Invalid("/resource_target", "设施标签需要资源视图。")
		}
		if e := validateTime(&q.MonitoringTime, false, false); e != nil {
			return e
		}
	case *QueryRequest:
		if q.Section != "" && !slices.Contains([]string{"coverage", "effective_windows"}, q.Section) {
			return Invalid("/section", "未知查询分区。")
		}
		if !access.ValidID(q.QueryID) || len(q.Cursor) > 4096 || q.Limit < 0 || q.Limit > MaxDetails {
			return Invalid("/query_id", "查询标识或分页无效。")
		}
		return nil
	case *QueryControlRequest:
		if !access.ValidID(q.QueryID) {
			return Invalid("/query_id", "查询标识无效。")
		}
		return nil
	case *DrilldownRequest:
		if !access.ValidID(q.QueryID) || len(q.GroupKey) > 512 || !slices.Contains([]string{"raw_active", "raw_samples", "derived_logical", "derived_samples", "device_count", "resource_count"}, q.Basis) || !slices.Contains([]string{"details", "trends", "statistics", "devices", "resources"}, q.Purpose) {
			return Invalid("/basis", "下钻参数无效。")
		}
		return nil
	}
	if page != nil {
		if e := validatePaging(page, max); e != nil {
			return e
		}
	}
	if target != nil {
		if e := validateTarget(*target, strings.HasPrefix(method, "monitoring.") || method == "devices.list" || method == "resources.query", "/target"); e != nil {
			return e
		}
	}
	for _, t := range others {
		if t != nil {
			if e := validateTarget(*t, false, "/target"); e != nil {
				return e
			}
		}
	}
	if id != "" && !access.ValidID(id) || id == "" && slices.Contains([]string{"resources.detail", "devices.get", "devices.outputs", "devices.history", "sources.bindingHistory", "map.features", "spatial.versions", "spatial.manifest", "spatial.objects", "spatial.placements", "spatial.componentLinks", "spatial.locations", "spatial.settings", "spatial.readFile", "channels.get"}, method) {
		return Invalid("/id", "需要有效对象标识。")
	}
	if len([]rune(search)) > 120 {
		return Invalid("/search", "搜索最多 120 字符。")
	}
	if len(fields) > 20 {
		return Invalid("/field_groups", "字段组超过限制。")
	}
	seen := map[string]bool{}
	for _, f := range fields {
		if seen[f] {
			return Invalid("/field_groups", "字段组重复。")
		}
		seen[f] = true
	}
	return nil
}
