// Package engineering owns versioned infrastructure definitions and monitoring templates.
// Business facility types are data; only these four metamodel responsibilities are fixed.
package engineering

import (
	"encoding/json"
	"time"
)

type Kind string

const (
	TypeDefinition     Kind = "type-definitions"
	MetricDefinition   Kind = "metrics"
	UnitDefinition     Kind = "units"
	MonitoringTemplate Kind = "monitoring-templates"
	MetamodelVersion        = "1"
	MaxSafeInteger          = 9007199254740991.0
	MaxContentBytes         = 256 * 1024
	MaxFields               = 100
	MaxComponents           = 100
	MaxNodes                = 500
)

type Issue struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Path     string `json:"path"`
	Key      string `json:"key"`
	Severity string `json:"severity"`
}
type Error struct {
	Code, Message string
	Status        int
	Issues        []Issue
}

func (e *Error) Error() string { return e.Message }
func problem(code, message string, status int) *Error {
	return &Error{Code: code, Message: message, Status: status, Issues: []Issue{}}
}
func invalid(issues []Issue) error {
	return &Error{Code: "definition_invalid", Message: "请修正标记的问题后重试。", Status: 422, Issues: issues}
}

type Option struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}
type Field struct {
	Key         string          `json:"key"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Shape       string          `json:"shape"`
	Required    bool            `json:"required"`
	Default     json.RawMessage `json:"default,omitempty"`
	Minimum     *float64        `json:"minimum,omitempty"`
	Maximum     *float64        `json:"maximum,omitempty"`
	MinLength   *int            `json:"min_length,omitempty"`
	MaxLength   *int            `json:"max_length,omitempty"`
	MinItems    *int            `json:"min_items,omitempty"`
	MaxItems    *int            `json:"max_items,omitempty"`
	Options     []Option        `json:"options,omitempty"`
	UnitVersion string          `json:"unit_version,omitempty"`
	Group       string          `json:"group"`
	Order       int             `json:"order"`
	Multiline   bool            `json:"multiline"`
	Precision   *int            `json:"precision,omitempty"`
}
type Component struct {
	Key         string  `json:"key"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Fields      []Field `json:"fields"`
}
type Relation struct {
	Parent   string `json:"parent"`
	Child    string `json:"child"`
	MinCount int    `json:"min_count"`
	MaxCount *int   `json:"max_count,omitempty"`
	Order    int    `json:"order"`
}
type Unit struct {
	Symbol   string `json:"symbol"`
	Category string `json:"category"`
}
type Metric struct {
	Shape        string   `json:"shape"`
	Category     string   `json:"category"`
	StandardUnit string   `json:"standard_unit"`
	AllowedUnits []string `json:"allowed_units"`
	Minimum      *float64 `json:"minimum,omitempty"`
	Maximum      *float64 `json:"maximum,omitempty"`
	Options      []Option `json:"options"`
}
type Group struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Order       int    `json:"order"`
}
type Target struct {
	TypeVersion string `json:"type_version"`
	Component   string `json:"component"`
}
type Binding struct {
	MetricVersion string `json:"metric_version"`
	UnitVersion   string `json:"unit_version"`
	ResourceKind  string `json:"resource_kind"`
	MinCount      int    `json:"min_count"`
	MaxCount      *int   `json:"max_count,omitempty"`
}
type Item struct {
	Key         string    `json:"key"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Enabled     bool      `json:"enabled"`
	Group       string    `json:"group"`
	Order       int       `json:"order"`
	Targets     []Target  `json:"targets"`
	Bindings    []Binding `json:"bindings"`
}
type Template struct {
	Purpose      string   `json:"purpose"`
	TypeVersions []string `json:"type_versions"`
	Groups       []Group  `json:"groups"`
	Items        []Item   `json:"items"`
}
type Content struct {
	MetamodelVersion string      `json:"metamodel_version"`
	Name             string      `json:"name"`
	Description      string      `json:"description"`
	ReleaseNotes     string      `json:"release_notes"`
	Fields           []Field     `json:"fields,omitempty"`
	Components       []Component `json:"components,omitempty"`
	Relations        []Relation  `json:"relations,omitempty"`
	Unit             *Unit       `json:"unit,omitempty"`
	Metric           *Metric     `json:"metric,omitempty"`
	Template         *Template   `json:"template,omitempty"`
}
type Category struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Revision    int    `json:"revision"`
	Used        bool   `json:"used"`
}
type Draft struct {
	Revision    int       `json:"revision"`
	BaseVersion string    `json:"base_version"`
	Content     Content   `json:"content"`
	UpdatedAt   time.Time `json:"updated_at"`
}
type Version struct {
	ID            string         `json:"id"`
	DefinitionID  string         `json:"definition_id"`
	Kind          Kind           `json:"kind"`
	Code          string         `json:"code"`
	Number        int            `json:"number"`
	Content       Content        `json:"content"`
	Digest        string         `json:"digest"`
	SchemaVersion string         `json:"schema_version"`
	Schemas       map[string]any `json:"schemas"`
	UnitCategory  *Category      `json:"unit_category,omitempty"`
	PublishedAt   time.Time      `json:"published_at"`
	PublishedBy   string         `json:"published_by"`
	Enabled       bool           `json:"enabled"`
	StateRevision int            `json:"state_revision"`
	StateReason   string         `json:"state_reason"`
	Usable        bool           `json:"usable"`
	Issues        []Issue        `json:"issues"`
	Dependencies  []string       `json:"dependencies"`
}
type Source struct {
	VersionID    string `json:"version_id"`
	DefinitionID string `json:"definition_id"`
	Code         string `json:"code"`
	Name         string `json:"name"`
	Number       int    `json:"number"`
}
type Definition struct {
	Source        *Source   `json:"source,omitempty"`
	ID            string    `json:"id"`
	Kind          Kind      `json:"kind"`
	Code          string    `json:"code"`
	TenantID      string    `json:"tenant_id"`
	SourceVersion string    `json:"source_version"`
	CreatedAt     time.Time `json:"created_at"`
	Revision      int       `json:"revision"`
	Draft         *Draft    `json:"draft"`
	Versions      []Version `json:"versions"`
	Dependencies  []Version `json:"dependencies"`
}
type Summary struct {
	ID            string `json:"id"`
	Kind          Kind   `json:"kind"`
	Code          string `json:"code"`
	Name          string `json:"name"`
	Latest        int    `json:"latest"`
	HasDraft      bool   `json:"has_draft"`
	Usable        bool   `json:"usable"`
	SourceVersion string `json:"source_version"`
}
type Filter struct {
	Query, Status string
	Offset, Limit int
}
type Page struct {
	Items  []Summary `json:"items"`
	Total  int       `json:"total"`
	Offset int       `json:"offset"`
	Limit  int       `json:"limit"`
}
type WriteMeta struct {
	RequestID string `json:"request_id"`
	Reason    string `json:"reason"`
}
type CreateInput struct {
	WriteMeta
	Code    string  `json:"code"`
	Content Content `json:"content"`
}
type SaveInput struct {
	WriteMeta
	Revision int     `json:"revision"`
	Content  Content `json:"content"`
}
type RevisionInput struct {
	WriteMeta
	Revision int `json:"revision"`
}
type NewDraftInput struct {
	RevisionInput
	VersionID string `json:"version_id"`
}
type CopyInput struct {
	WriteMeta
	Code          string `json:"code"`
	SourceVersion string `json:"source_version"`
}
type StateInput struct {
	WriteMeta
	Revision int  `json:"revision"`
	Enabled  bool `json:"enabled"`
}
type Result struct {
	DefinitionID string  `json:"definition_id"`
	VersionID    string  `json:"version_id"`
	Revision     int     `json:"revision"`
	Replayed     bool    `json:"replayed"`
	Issues       []Issue `json:"issues"`
}
type Node struct {
	ID       string         `json:"id"`
	Parent   string         `json:"parent"`
	Category string         `json:"category"`
	Values   map[string]any `json:"values"`
}
type Sample struct {
	Values map[string]any `json:"values"`
	Nodes  []Node         `json:"nodes"`
}
type PreviewInput struct {
	VersionID   string          `json:"version_id"`
	Revision    int             `json:"revision"`
	TypeVersion string          `json:"type_version"`
	Sample      *Sample         `json:"sample,omitempty"`
	Value       json.RawMessage `json:"value,omitempty"`
	UnitVersion string          `json:"unit_version"`
}
type Preview struct {
	Valid        bool      `json:"valid"`
	Issues       []Issue   `json:"issues"`
	Content      Content   `json:"content"`
	Dependencies []Version `json:"dependencies"`
	SampleOnly   bool      `json:"sample_only"`
}
type Difference struct {
	Path   string `json:"path"`
	Before any    `json:"before"`
	After  any    `json:"after"`
}
type Impact struct {
	ResourceReferences bool      `json:"resource_references"`
	Public             []Summary `json:"public"`
	PrivateReferences  bool      `json:"private_references"`
}

// Normalize absent collections at the write boundary so every stored snapshot and
// generated HTTP type has the same JSON representation. Never alter sample values.
func canonicalContent(c Content) Content {
	b, e := json.Marshal(c)
	if e != nil {
		return c
	}
	var out Content
	if json.Unmarshal(b, &out) != nil {
		return c
	}
	for i := range out.Components {
		if out.Components[i].Fields == nil {
			out.Components[i].Fields = []Field{}
		}
	}
	if m := out.Metric; m != nil {
		if m.AllowedUnits == nil {
			m.AllowedUnits = []string{}
		}
		if m.Options == nil {
			m.Options = []Option{}
		}
	}
	if t := out.Template; t != nil {
		if t.TypeVersions == nil {
			t.TypeVersions = []string{}
		}
		if t.Groups == nil {
			t.Groups = []Group{}
		}
		if t.Items == nil {
			t.Items = []Item{}
		}
		for i := range t.Items {
			if t.Items[i].Targets == nil {
				t.Items[i].Targets = []Target{}
			}
			if t.Items[i].Bindings == nil {
				t.Items[i].Bindings = []Binding{}
			}
		}
	}
	return out
}
