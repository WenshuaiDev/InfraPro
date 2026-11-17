package displayread

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

type ScopeOption struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Code       string `json:"code"`
	Name       string `json:"name"`
	SourceKind string `json:"source_kind,omitempty"`
	VersionID  string `json:"version_id,omitempty"`
	Visibility string `json:"visibility"`
}
type ScopeOptionsPage struct {
	Items      []ScopeOption `json:"items"`
	Total      string        `json:"total"`
	NextCursor string        `json:"next_cursor"`
	HasMore    bool          `json:"has_more"`
}
type OptionsReader func(context.Context, pgx.Tx, access.Actor, string, string, []Member) ([]ScopeOption, error)

// DefaultSummary contains only defaults for authorized assets; hidden default
// versions are represented by state without their identity.
type DefaultVersion struct {
	AssetID   string `json:"asset_id"`
	Name      string `json:"name"`
	State     string `json:"state"`
	VersionID string `json:"version_id,omitempty"`
	Number    string `json:"number,omitempty"`
}
type DefaultSummary struct {
	Total   string           `json:"total"`
	Items   []DefaultVersion `json:"items"`
	HasMore bool             `json:"has_more"`
}
type DefaultsReader func(context.Context, pgx.Tx, access.Actor, string, string, []Member) (DefaultSummary, error)
