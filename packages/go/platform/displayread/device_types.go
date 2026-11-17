package displayread

import (
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/location"
	"time"
)

type NamedRef struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Code       string `json:"code,omitempty"`
	Visibility string `json:"visibility,omitempty"`
}
type ProductVersionRef struct {
	ID           string `json:"id"`
	ProductID    string `json:"product_id"`
	Code         string `json:"code"`
	Name         string `json:"name"`
	Number       string `json:"number"`
	AdoptedModel string `json:"adopted_model"`
}
type DeviceDetail struct {
	Classification      *Classification     `json:"classification,omitempty"`
	ID                  string              `json:"id"`
	Code                string              `json:"code"`
	Name                string              `json:"name"`
	Status              string              `json:"status"`
	Revision            string              `json:"revision"`
	Visibility          string              `json:"visibility"`
	Product             ProductVersionRef   `json:"product_version_ref"`
	Manufacturer        NamedRef            `json:"manufacturer_ref"`
	AdoptedModel        string              `json:"adopted_model"`
	SerialNumber        *string             `json:"serial_number,omitempty"`
	Description         *string             `json:"description,omitempty"`
	CurrentInstallation *DeviceInstallation `json:"current_installation,omitempty"`
}
type DeviceInstallation struct {
	ID                  string             `json:"id"`
	DeviceID            string             `json:"device_id"`
	Revision            string             `json:"revision"`
	Visibility          string             `json:"visibility"`
	Target              *Ref               `json:"target_ref,omitempty"`
	TargetName          string             `json:"target_name,omitempty"`
	TargetStatus        string             `json:"target_status,omitempty"`
	InstalledAt         time.Time          `json:"installed_at"`
	UninstalledAt       *time.Time         `json:"uninstalled_at"`
	Position            *location.Position `json:"position,omitempty"`
	LocationDescription *string            `json:"location_description,omitempty"`
}
type OutputSemantics struct {
	Metric                 MetricRef `json:"metric_ref"`
	UnitVersionID          *string   `json:"unit_version_id"`
	ValueShape             string    `json:"value_shape"`
	TimestampMeaning       string    `json:"timestamp_meaning"`
	AggregationDescription string    `json:"aggregation_description"`
	ObservationDirection   string    `json:"observation_direction"`
	ReferenceBasis         string    `json:"reference_basis"`
}
type DeviceOutput struct {
	ID               string     `json:"id"`
	DeviceID         string     `json:"device_id"`
	Device           NamedRef   `json:"device_ref"`
	SlotKey          string     `json:"slot_key"`
	Generation       string     `json:"generation"`
	Name             string     `json:"name"`
	Revision         string     `json:"revision"`
	StartedAt        time.Time  `json:"started_at"`
	EndedAt          *time.Time `json:"ended_at"`
	State            string     `json:"state"`
	ProductVersionID string     `json:"product_version_id"`
	OutputSemantics
	Definitions []DefinitionVersion `json:"definitions"`
}
type DeviceHistoryValues struct {
	Code             string  `json:"code,omitempty"`
	Name             string  `json:"name,omitempty"`
	Status           string  `json:"status,omitempty"`
	Revision         string  `json:"revision,omitempty"`
	ProductVersionID string  `json:"product_version_id,omitempty"`
	SerialNumber     *string `json:"serial_number,omitempty"`
	Description      *string `json:"description,omitempty"`
}
type DeviceChange struct {
	ID            string               `json:"id"`
	ChangedAt     time.Time            `json:"changed_at"`
	Operation     string               `json:"operation"`
	ChangedFields []string             `json:"changed_fields"`
	Before        *DeviceHistoryValues `json:"before,omitempty"`
	After         *DeviceHistoryValues `json:"after,omitempty"`
}
type SourceProjection struct {
	Configured       bool             `json:"configured"`
	Visibility       string           `json:"visibility"`
	ID               string           `json:"id,omitempty"`
	Revision         string           `json:"revision,omitempty"`
	Device           *NamedRef        `json:"device_ref,omitempty"`
	Output           *DeviceOutput    `json:"output,omitempty"`
	StartedAt        *time.Time       `json:"started_at,omitempty"`
	EndedAt          *time.Time       `json:"ended_at,omitempty"`
	OutputSemantics  *OutputSemantics `json:"output_semantics,omitempty"`
	ChannelSemantics *OutputSemantics `json:"channel_semantics,omitempty"`
}
type SourceBindingItem struct {
	Channel Ref               `json:"channel_ref"`
	Point   *Ref              `json:"point_ref,omitempty"`
	State   string            `json:"state"`
	Source  *SourceProjection `json:"source,omitempty"`
}
type DeviceCounts struct {
	UniqueDeviceCount string            `json:"unique_device_count"`
	StatusCounts      map[string]string `json:"status_counts"`
}
