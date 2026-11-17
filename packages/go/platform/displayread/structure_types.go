package displayread

type StructureStatisticsRequest struct {
	Paging
	ReadFields
	Target        Target          `json:"target"`
	ChannelTarget *Target         `json:"channel_target,omitempty"`
	Filters       ResourceFilters `json:"filters,omitempty"`
	GroupBy       string          `json:"group_by"`
	Membership    string          `json:"membership"`
}
type StructureCounts struct {
	ResourceCounts
	MonitoringSectionCount string `json:"monitoring_section_count"`
	BoreholeCount          string `json:"borehole_count"`
	SectionTypeCount       string `json:"section_type_count"`
}
type StructureGroupRef struct {
	Key        string `json:"group_key"`
	Label      string `json:"label"`
	Visibility string `json:"visibility"`
	Ref        *Ref   `json:"ref,omitempty"`
}
type StructureGroup struct {
	StructureGroupRef
	Totals         StructureCounts  `json:"totals"`
	Self           *StructureCounts `json:"self,omitempty"`
	DirectChildren *StructureCounts `json:"direct_children,omitempty"`
	Descendants    *StructureCounts `json:"descendants,omitempty"`
}
type StructureStatistics struct {
	StatisticsMeta
	StructureModel  int                          `json:"structure_model"`
	Membership      string                       `json:"membership"`
	Totals          StructureCounts              `json:"totals"`
	Groups          []StructureGroup             `json:"groups"`
	Applicability   map[string]map[string]string `json:"applicability"`
	ContextExcluded bool                         `json:"context_excluded"`
	Filters         ResourceFilters              `json:"filters"`
}
