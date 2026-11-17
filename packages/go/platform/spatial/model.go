// Package spatial owns immutable spatial publications and their scoped use.
package spatial

import (
	"context"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/location"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/jackc/pgx/v5"
	"io"
	"time"
)

type Scope = k.Scope
type Meta = k.Meta
type Result = k.Result
type Preview = k.Preview
type Error = k.Error
type Issue = k.Issue
type RevisionInput = k.RevisionInput

func Tenant(id string) Scope               { return k.Tenant(id) }
func Project(tenant, project string) Scope { return k.Project(tenant, project) }

type Asset struct {
	ID               string    `json:"id"`
	TenantID         string    `json:"tenant_id"`
	FacilityID       string    `json:"facility_id,omitempty"`
	Kind             string    `json:"kind"`
	Code             string    `json:"code"`
	Name             string    `json:"name"`
	Category         string    `json:"category"`
	SourceNote       string    `json:"source_note"`
	Status           string    `json:"status"`
	CurrentVersionID string    `json:"current_version_id,omitempty"`
	Revision         string    `json:"revision"`
	CreatedAt        time.Time `json:"created_at"`
}
type Version struct {
	ID                string      `json:"id"`
	TenantID          string      `json:"tenant_id"`
	AssetID           string      `json:"asset_id"`
	Number            int         `json:"number"`
	State             string      `json:"state"`
	BaseVersionID     string      `json:"base_version_id,omitempty"`
	SourceBlobID      string      `json:"-"`
	Format            string      `json:"format"`
	Manifest          Manifest    `json:"manifest"`
	ManifestHash      string      `json:"manifest_hash"`
	Settings          Settings    `json:"settings"`
	Candidates        []Candidate `json:"candidates"`
	ChangeNote        string      `json:"change_note"`
	Revision          string      `json:"revision"`
	ContentAvailable  bool        `json:"content_available"`
	CreatedAt         time.Time   `json:"created_at"`
	PublishedAt       *time.Time  `json:"published_at"`
	Source            *File       `json:"source,omitempty"`
	LatestUploadID    string      `json:"latest_upload_id,omitempty"`
	LatestJobID       string      `json:"latest_job_id,omitempty"`
	CanDownloadSource bool        `json:"can_download_source"`
}
type File struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	SHA256    string `json:"sha256"`
	Size      int64  `json:"size"`
	MediaType string `json:"media_type"`
}
type Manifest struct {
	Contract     string      `json:"contract"`
	Converter    string      `json:"converter"`
	SourceSHA256 string      `json:"source_sha256"`
	Format       string      `json:"format"`
	Files        []File      `json:"files"`
	Pages        []Page      `json:"pages"`
	Elements     []Element   `json:"elements"`
	Features     []Feature   `json:"features"`
	Coordinates  Coordinates `json:"coordinates"`
	Warnings     []string    `json:"warnings"`
}
type Coordinates struct {
	Unit         string    `json:"unit"`
	UpAxis       string    `json:"up_axis"`
	SourceUnit   string    `json:"source_unit"`
	SourceToView []float64 `json:"source_to_view"`
	ViewToSource []float64 `json:"view_to_source"`
	CRS          string    `json:"crs,omitempty"`
}
type Page struct {
	ID           string      `json:"id"`
	Number       int         `json:"number"`
	Width        int         `json:"width"`
	Height       int         `json:"height"`
	SourceWidth  float64     `json:"source_width"`
	SourceHeight float64     `json:"source_height"`
	Rotation     int         `json:"rotation"`
	CropBox      []float64   `json:"crop_box"`
	FileID       string      `json:"file_id"`
	Resolutions  []PageImage `json:"resolutions"`
}
type PageImage struct {
	FileID string `json:"file_id"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}
type Element struct {
	Key            string         `json:"key"`
	Name           string         `json:"name"`
	Type           string         `json:"type"`
	Parent         string         `json:"parent,omitempty"`
	TypeKey        string         `json:"type_key,omitempty"`
	Fragments      []Fragment     `json:"fragments"`
	GeometryHash   string         `json:"geometry_hash"`
	Properties     map[string]any `json:"properties"`
	TypeProperties map[string]any `json:"type_properties"`
	Transform      []float64      `json:"transform"`
}
type Fragment struct {
	Node     int  `json:"node"`
	Instance *int `json:"instance,omitempty"`
}
type Feature struct {
	Type       string            `json:"type"`
	ID         string            `json:"id"`
	Geometry   map[string]any    `json:"geometry"`
	Properties map[string]string `json:"properties"`
	BBox       []float64         `json:"bbox"`
}
type Camera struct {
	Position []float64 `json:"position"`
	Target   []float64 `json:"target"`
}
type Settings struct {
	Camera           *Camera           `json:"camera,omitempty"`
	PageID           string            `json:"page_id,omitempty"`
	SourceProperties bool              `json:"source_properties"`
	Basemap          string            `json:"basemap,omitempty"`
	HiddenElements   []string          `json:"hidden_elements,omitempty"`
	MainChannels     map[string]string `json:"main_channels,omitempty"`
}
type Placement struct {
	Geometry      *location.AssociationGeometry `json:"geometry,omitempty"`
	TargetRef     *ObjectRef                    `json:"target_ref,omitempty"`
	ID            string                        `json:"id"`
	PointID       string                        `json:"point_id"`
	PageID        string                        `json:"page_id"`
	Anchor        []float64                     `json:"anchor"`
	ElementKey    string                        `json:"element_key"`
	LabelOffset   []float64                     `json:"label_offset"`
	MainChannelID string                        `json:"main_channel_id,omitempty"`
	BasisHash     string                        `json:"basis_hash"`
	CopiedFrom    string                        `json:"copied_from,omitempty"`
	Object        *Object                       `json:"object,omitempty"`
	BasisChanged  bool                          `json:"basis_changed"`
}
type ComponentLink struct {
	ID            string      `json:"id"`
	ComponentID   string      `json:"component_id"`
	ElementKey    string      `json:"element_key"`
	PageID        string      `json:"page_id"`
	Region        [][]float64 `json:"region"`
	BasisHash     string      `json:"basis_hash"`
	MainChannelID string      `json:"main_channel_id,omitempty"`
	Object        *Object     `json:"object,omitempty"`
	BasisChanged  bool        `json:"basis_changed"`
}
type FeatureLink struct {
	FeatureKey string `json:"feature_key"`
	FacilityID string `json:"facility_id"`
}
type Candidate struct {
	Kind          string         `json:"kind"`
	State         string         `json:"state"`
	Reason        string         `json:"reason"`
	Placement     *Placement     `json:"placement,omitempty"`
	ComponentLink *ComponentLink `json:"component_link,omitempty"`
}
type Draft struct {
	Asset          Asset           `json:"asset"`
	Version        Version         `json:"version"`
	Placements     []Placement     `json:"placements"`
	ComponentLinks []ComponentLink `json:"component_links"`
	FeatureLinks   []FeatureLink   `json:"feature_links"`
}
type ObjectRef struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}
type Object struct {
	ObjectRef
	FacilityID     string          `json:"facility_id"`
	Code           string          `json:"code"`
	Name           string          `json:"name"`
	Status         string          `json:"status"`
	Retired        bool            `json:"retired"`
	BasisHash      string          `json:"basis_hash"`
	Position       json.RawMessage `json:"position,omitempty"`
	Longitude      *float64        `json:"longitude,omitempty"`
	Latitude       *float64        `json:"latitude,omitempty"`
	PositionNotice string          `json:"position_notice,omitempty"`
	OwnerKind      string          `json:"owner_kind,omitempty"`
	OwnerID        string          `json:"owner_id,omitempty"`
}

// Readers must require complete object grants, never an ancestor context projection.
type ObjectReader interface {
	ObjectTx(context.Context, pgx.Tx, access.Actor, Scope, ObjectRef) (Object, error)
	Objects(context.Context, access.Actor, Scope, Filter) (List[Object], error)
}
type MonitoringReader interface {
	Summaries(context.Context, access.Actor, Scope, ObjectQuery) (json.RawMessage, error)
}
type ArtifactReader interface {
	Related(context.Context, access.Actor, Scope, RelatedInput) (json.RawMessage, error)
}
type BlobStore interface {
	Put(context.Context, string, io.Reader, int64, string) error
	Get(context.Context, string) (io.ReadCloser, error)
	Delete(context.Context, string) error
}
type Converter interface {
	Convert(context.Context, string, string, string) (Manifest, error)
	Ready(context.Context) error
}
type Filter struct {
	OwnerID    string    `json:"owner_id"`
	Query      string    `json:"q"`
	Kind       string    `json:"kind"`
	FacilityID string    `json:"facility_id"`
	State      string    `json:"state"`
	PageID     string    `json:"page_id"`
	ElementKey string    `json:"element_key"`
	BBox       []float64 `json:"bbox,omitempty"`
	Cursor     string    `json:"-"`
	Limit      int       `json:"limit"`
}
type List[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor"`
}
type AssetInput struct {
	Meta
	Revision   string `json:"expected_revision,omitempty"`
	Kind       string `json:"kind"`
	FacilityID string `json:"facility_id,omitempty"`
	Code       string `json:"code"`
	Name       string `json:"name"`
	Category   string `json:"category"`
	SourceNote string `json:"source_note"`
}
type VersionInput struct {
	Meta
	Revision      string `json:"expected_revision"`
	BaseVersionID string `json:"base_version_id,omitempty"`
	NewSource     bool   `json:"new_source"`
	ChangeNote    string `json:"change_note"`
}
type DraftInput struct {
	Meta
	Revision             string          `json:"expected_revision"`
	Placements           []Placement     `json:"placements,omitempty"`
	RemovePlacements     []string        `json:"remove_placements,omitempty"`
	ComponentLinks       []ComponentLink `json:"component_links,omitempty"`
	RemoveComponentLinks []string        `json:"remove_component_links,omitempty"`
	FeatureLinks         []FeatureLink   `json:"feature_links,omitempty"`
	RemoveFeatureLinks   []string        `json:"remove_feature_links,omitempty"`
	Settings             *Settings       `json:"settings,omitempty"`
	ClearCandidates      bool            `json:"clear_candidates"`
}
type ControlInput struct {
	Meta
	Revision             string `json:"expected_revision"`
	VersionID            string `json:"version_id,omitempty"`
	ReplacementVersionID string `json:"replacement_version_id,omitempty"`
}
type UploadInput struct {
	Meta
	Revision  string `json:"expected_revision"`
	VersionID string `json:"version_id"`
	Filename  string `json:"filename"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256"`
}
type Upload struct {
	ID        string `json:"id"`
	VersionID string `json:"version_id"`
	Filename  string `json:"filename"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256"`
	State     string `json:"state"`
	Revision  string `json:"revision"`
	PartSize  int64  `json:"part_size"`
}
type Job struct {
	TaskRef         *taskrecord.Ref `json:"task_ref,omitempty"`
	ID              string          `json:"id"`
	TenantID        string          `json:"tenant_id"`
	VersionID       string          `json:"version_id"`
	SourceBlobID    string          `json:"-"`
	State           string          `json:"state"`
	Stage           string          `json:"stage"`
	ErrorCode       string          `json:"error_code"`
	Report          json.RawMessage `json:"report"`
	Attempt         int             `json:"attempt"`
	Fence           int64           `json:"-"`
	Revision        string          `json:"revision"`
	VersionRevision string          `json:"-"`
}
type Grant struct {
	ID             string     `json:"id"`
	AssetID        string     `json:"asset_id"`
	VersionID      string     `json:"version_id"`
	FacilityLinkID string     `json:"facility_link_id,omitempty"`
	SourceDownload bool       `json:"source_download"`
	Default        bool       `json:"is_default"`
	Revision       string     `json:"revision"`
	EndedAt        *time.Time `json:"ended_at"`
}
type GrantChange struct {
	VersionID      string `json:"version_id"`
	SourceDownload bool   `json:"source_download"`
	Default        bool   `json:"is_default"`
}
type AccessInput struct {
	Meta
	Revision string        `json:"expected_revision"`
	Add      []GrantChange `json:"add"`
	End      []string      `json:"end"`
}
type AccessDetail struct {
	Revision string  `json:"revision"`
	Grants   []Grant `json:"grants"`
}
type View struct {
	Revision  string   `json:"revision"`
	VersionID string   `json:"version_id"`
	Settings  Settings `json:"settings"`
}
type ViewInput struct {
	Meta
	Revision  string   `json:"expected_revision"`
	VersionID string   `json:"version_id"`
	Settings  Settings `json:"settings"`
}
type RelatedInput struct {
	Owner       ObjectRef `json:"owner"`
	Descendants bool      `json:"descendants"`
	From        string    `json:"from,omitempty"`
	To          string    `json:"to,omitempty"`
	Cursor      string    `json:"cursor,omitempty"`
	Limit       int       `json:"limit,omitempty"`
}
type ObjectQuery struct {
	Objects      []ObjectRef       `json:"objects"`
	MainChannels map[string]string `json:"main_channels,omitempty"`
}

func DecodeRequest(raw []byte, out any) error { return k.Strict(raw, out) }
