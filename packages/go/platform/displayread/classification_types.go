package displayread

// Dictionary labels are separately enabled; they never imply device membership.
type CategoriesRequest struct {
	Paging
	ReleaseID string `json:"dictionary_release_id,omitempty"`
	Search    string `json:"search,omitempty"`
}
type Category struct {
	ID          string   `json:"id"`
	Code        string   `json:"code"`
	Name        string   `json:"name"`
	ParentID    string   `json:"parent_id,omitempty"`
	Description string   `json:"description"`
	Order       int      `json:"order"`
	Selectable  bool     `json:"selectable"`
	Status      string   `json:"status"`
	Path        []string `json:"path,omitempty"`
	NamePath    []string `json:"name_path,omitempty"`
	Sensor      bool     `json:"sensor"`
}
type CategoryPage struct {
	Page[Category]
	DictionaryReleaseID string `json:"dictionary_release_id"`
	DictionaryRevision  string `json:"dictionary_revision"`
	SensorRootID        string `json:"sensor_root_id"`
}
type Classification struct {
	State               string   `json:"state"`
	Origin              string   `json:"origin"`
	CategoryID          string   `json:"category_id,omitempty"`
	SelectedReleaseID   string   `json:"selected_release_id,omitempty"`
	DictionaryReleaseID string   `json:"dictionary_release_id,omitempty"`
	OverrideRevision    string   `json:"override_revision"`
	ProductRevision     string   `json:"product_classification_revision"`
	RecordID            string   `json:"record_id,omitempty"`
	Name                string   `json:"name,omitempty"`
	Path                []string `json:"path"`
	NamePath            []string `json:"name_path"`
	Sensor              *bool    `json:"sensor"`
	CategoryStatus      string   `json:"category_status,omitempty"`
}
type DeviceCategoryStatisticsRequest struct {
	Paging
	ReadFields
	Target        Target        `json:"target"`
	ChannelTarget *Target       `json:"channel_target,omitempty"`
	Population    string        `json:"population"`
	ReleaseID     string        `json:"dictionary_release_id,omitempty"`
	Mode          string        `json:"mode"`
	FrontierIDs   []string      `json:"frontier_ids,omitempty"`
	Filters       DeviceFilters `json:"filters,omitempty"`
}
type CategoryCounts struct {
	UniqueDeviceCount       string `json:"unique_device_count"`
	SensorDeviceCount       string `json:"sensor_device_count"`
	UsedSensorCategoryCount string `json:"used_sensor_category_count"`
	NonSensorDeviceCount    string `json:"non_sensor_device_count"`
	UnclassifiedDeviceCount string `json:"unclassified_device_count"`
	UnavailableDeviceCount  string `json:"unavailable_device_count"`
}
type CategoryGroup struct {
	Key           string    `json:"group_key"`
	Label         string    `json:"label"`
	Visibility    string    `json:"visibility"`
	Category      *Category `json:"category,omitempty"`
	DirectCount   string    `json:"direct_device_count"`
	IncludedCount string    `json:"included_device_count"`
	Count         string    `json:"unique_device_count"`
}
type DeviceCategoryStatistics struct {
	StatisticsMeta
	DictionaryReleaseID    string          `json:"dictionary_release_id"`
	ClassificationRevision string          `json:"classification_revision"`
	ClassificationDigest   string          `json:"classification_digest"`
	Mode                   string          `json:"mode"`
	Totals                 CategoryCounts  `json:"totals"`
	Groups                 []CategoryGroup `json:"groups"`
}
