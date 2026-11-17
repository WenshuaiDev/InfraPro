package displayread

type StatisticsMeta struct {
	DefinitionsVersion string            `json:"definitions_version"`
	Population         string            `json:"population"`
	GroupBy            string            `json:"group_by"`
	Additivity         string            `json:"additivity"`
	CountBasis         map[string]string `json:"count_basis"`
	QueryID            string            `json:"query_id"`
	GroupCount         string            `json:"group_count"`
	NextCursor         string            `json:"next_cursor"`
	HasMore            bool              `json:"has_more"`
}
type ResourceStatistics struct {
	StatisticsMeta
	Totals          ResourceCounts                    `json:"totals"`
	Groups          []StatisticsGroup[ResourceCounts] `json:"groups"`
	Filters         ResourceFilters                   `json:"filters"`
	ContextExcluded bool                              `json:"context_excluded"`
}
type DeviceStatistics struct {
	StatisticsMeta
	Membership     string                          `json:"membership,omitempty"`
	StructureModel int                             `json:"structure_model,omitempty"`
	Totals         DeviceCounts                    `json:"totals"`
	Groups         []StatisticsGroup[DeviceCounts] `json:"groups"`
	Filters        DeviceFilters                   `json:"filters"`
	Relation       string                          `json:"relation"`
}
type MonitoringSnapshot struct {
	RawWatermark        string `json:"raw_watermark"`
	DerivedHeadDigest   string `json:"derived_head_digest"`
	RouteRevisionDigest string `json:"route_revision_digest,omitempty"`
}
type MonitoringStatistics struct {
	Membership     string `json:"membership,omitempty"`
	StructureModel int    `json:"structure_model,omitempty"`
	StatisticsMeta
	Totals           MonitoringCounts                    `json:"totals"`
	Groups           []StatisticsGroup[MonitoringCounts] `json:"groups"`
	RequestedRange   *Window                             `json:"requested_range"`
	AllHistory       bool                                `json:"all_history"`
	Qualities        []string                            `json:"qualities"`
	Timezone         string                              `json:"timezone"`
	SourceKinds      []string                            `json:"source_kinds"`
	Snapshot         MonitoringSnapshot                  `json:"monitoring_snapshot"`
	Coverage         Page[CoverageSummary]               `json:"coverage"`
	EffectiveWindows Page[WindowSummary]                 `json:"effective_windows"`
}
