// Package classification owns stable device categories and classification metadata.
// It does not define hardware outputs, units, parsing contracts or source bindings.
package classification

import (
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"time"
)

type Scope = k.Scope
type Meta = k.Meta

const DictionaryCode = "device_category"
const MaxDepth = 128
const MaxCategories = 10000
const MaxBatchTargets = 5000

type Node struct {
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
type Content struct {
	DictionaryCode string `json:"dictionary_code"`
	SensorRootID   string `json:"sensor_root_id"`
	Nodes          []Node `json:"nodes"`
}
type Dictionary struct {
	Revision         string  `json:"revision"`
	DraftRevision    string  `json:"draft_revision"`
	Draft            Content `json:"draft"`
	CurrentReleaseID string  `json:"current_release_id"`
	CanManage        bool    `json:"can_manage"`
}
type Release struct {
	ID        string    `json:"id"`
	Number    int       `json:"number"`
	Content   Content   `json:"content"`
	Hash      string    `json:"content_hash"`
	CreatedAt time.Time `json:"created_at"`
	Reason    string    `json:"reason"`
}
type SaveInput struct {
	Meta
	Revision string  `json:"expected_revision"`
	Content  Content `json:"content"`
}
type PublishInput struct {
	Meta
	Revision string `json:"expected_revision"`
}
type CategoryChange struct {
	ID     string `json:"id"`
	Before *Node  `json:"before"`
	After  Node   `json:"after"`
}
type PublishPreview struct {
	Hash                        string           `json:"preview_hash"`
	Changes                     []CategoryChange `json:"changes"`
	ExistingReferencesPreserved bool             `json:"existing_references_preserved"`
	AffectsNewGrouping          bool             `json:"affects_new_grouping"`
}
type Result struct {
	ID        string `json:"id"`
	Revision  string `json:"revision"`
	ReleaseID string `json:"release_id,omitempty"`
	Changed   bool   `json:"changed"`
	Replayed  bool   `json:"replayed"`
	Applied   int    `json:"applied,omitempty"`
}
type Page[T any] struct {
	DictionaryReleaseID string `json:"dictionary_release_id,omitempty"`
	Items               []T    `json:"items"`
	NextCursor          string `json:"next_cursor"`
}
type Selection struct {
	Mode       string `json:"mode"` // classify, unclassified (product), or inherit (device)
	ReleaseID  string `json:"dictionary_release_id,omitempty"`
	CategoryID string `json:"category_id,omitempty"`
}
type Target struct {
	Kind           string `json:"kind"`
	ID             string `json:"id"`
	Revision       string `json:"expected_classification_revision"`
	EntityRevision string `json:"expected_entity_revision"`
}
type BatchInput struct {
	Meta
	Targets   []Target  `json:"targets"`
	Selection Selection `json:"selection"`
	PreviewID string    `json:"preview_id,omitempty"`
}
type Effective struct {
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
type TargetDetail struct {
	Target
	Name      string    `json:"name"`
	Effective Effective `json:"effective"`
}
type AssignmentChange struct {
	Target                    Target    `json:"target"`
	Before                    Effective `json:"before"`
	After                     Effective `json:"after"`
	VisibleInheritedDevices   int       `json:"visible_inherited_devices"`
	InheritedDevicesMayChange bool      `json:"inherited_devices_may_change"`
}
type BatchPreview struct {
	ID               string             `json:"preview_id"`
	Hash             string             `json:"preview_hash"`
	Changes          []AssignmentChange `json:"changes"`
	ExpiresAt        time.Time          `json:"expires_at"`
	PreservesOutputs bool               `json:"preserves_outputs"`
}

func PlatformScope() Scope                      { return k.Platform() }
func TenantScope(id string) Scope               { return k.Tenant(id) }
func ProjectScope(tenant, project string) Scope { return k.Project(tenant, project) }
func Decode(raw []byte, v any) error            { return k.StrictLimit(raw, v, 8*1024*1024) }
