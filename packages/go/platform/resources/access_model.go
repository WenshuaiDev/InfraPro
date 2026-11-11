package resources

import "time"

type AccessPolicy struct {
	TenantID            string `json:"tenant_id"`
	ProjectID           string `json:"project_id"`
	ProjectName         string `json:"project_name"`
	ProjectStatus       string `json:"project_status"`
	ProjectVersion      string `json:"project_version"`
	AllowFacilityCreate bool   `json:"allow_facility_create"`
	Revision            string `json:"revision"`
}
type Link struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenant_id"`
	ProjectID    string    `json:"project_id"`
	FacilityID   string    `json:"facility_id"`
	FacilityName string    `json:"facility_name"`
	FacilityCode string    `json:"facility_code"`
	Status       string    `json:"status"`
	Revision     string    `json:"revision"`
	CreatedAt    time.Time `json:"created_at"`
}
type Grant struct {
	ID           string   `json:"id"`
	LinkID       string   `json:"link_id"`
	Kind         Kind     `json:"kind"`
	ResourceID   string   `json:"resource_id"`
	ComponentID  *string  `json:"component_id,omitempty"`
	PointID      *string  `json:"point_id,omitempty"`
	ChannelID    *string  `json:"channel_id,omitempty"`
	Name         string   `json:"name"`
	Code         string   `json:"code"`
	Capabilities []string `json:"capabilities"`
	Source       string   `json:"source"`
	Revision     string   `json:"revision"`
}
type AccessDetail struct {
	Policy          AccessPolicy `json:"policy"`
	Links           []Link       `json:"links"`
	NextCursor      string       `json:"next_cursor"`
	Link            *Link        `json:"link"`
	Grants          []Grant      `json:"grants"`
	GrantNextCursor string       `json:"grant_next_cursor"`
	Candidates      Page         `json:"candidates"`
	CandidateGrants []Grant      `json:"candidate_grants"`
}
type GrantInput struct {
	Kind         Kind     `json:"kind"`
	ComponentID  *string  `json:"component_id,omitempty"`
	PointID      *string  `json:"point_id,omitempty"`
	ChannelID    *string  `json:"channel_id,omitempty"`
	Capabilities []string `json:"capabilities"`
}
type GrantUpdate struct {
	ID           string   `json:"grant_id"`
	Revision     string   `json:"expected_revision"`
	Capabilities []string `json:"capabilities"`
}
type GrantEnd struct {
	ID       string `json:"grant_id"`
	Revision string `json:"expected_revision"`
}
type CurrentSelection struct {
	RootKind Kind              `json:"root_kind"`
	RootID   string            `json:"root_id"`
	Kinds    map[Kind][]string `json:"kinds"`
}
type AccessInput struct {
	RequestID           string            `json:"request_id"`
	Action              string            `json:"action"`
	AccessRevision      string            `json:"expected_access_revision"`
	LinkRevision        string            `json:"expected_link_revision,omitempty"`
	Reason              string            `json:"reason"`
	PreviewHash         string            `json:"preview_hash,omitempty"`
	AllowFacilityCreate *bool             `json:"allow_facility_create,omitempty"`
	Add                 []GrantInput      `json:"add,omitempty"`
	Update              []GrantUpdate     `json:"update,omitempty"`
	End                 []GrantEnd        `json:"end,omitempty"`
	SelectCurrent       *CurrentSelection `json:"select_current,omitempty"`
}
type AccessPreview struct {
	Before  []Grant       `json:"-"`
	Hash    string        `json:"preview_hash"`
	Policy  AccessPolicy  `json:"policy"`
	Link    *Link         `json:"link"`
	Add     []Grant       `json:"add"`
	Update  []Grant       `json:"update"`
	End     []Grant       `json:"end"`
	Context []ContextNode `json:"retained_context"`
	Changed bool          `json:"changed"`
}
type AccessResult struct {
	ProjectID      string `json:"project_id"`
	LinkID         string `json:"link_id,omitempty"`
	LinkRevision   string `json:"link_revision,omitempty"`
	AccessRevision string `json:"access_revision"`
	Changed        bool   `json:"changed"`
	Replayed       bool   `json:"replayed"`
}
type AccessFilter struct {
	Cursor        string
	Limit         int
	GrantCursor   string
	CandidateKind Kind
	Candidate     Filter
}
type AccessChange struct {
	ID         string    `json:"id"`
	Revision   string    `json:"access_revision"`
	Operation  string    `json:"operation"`
	ActorID    string    `json:"actor_id"`
	ActorScope string    `json:"actor_scope"`
	Reason     string    `json:"reason"`
	Before     any       `json:"before"`
	After      any       `json:"after"`
	ChangedAt  time.Time `json:"changed_at"`
}
type AccessChanges struct {
	Items      []AccessChange `json:"items"`
	NextCursor string         `json:"next_cursor"`
}

// ContextNode deliberately has no private fields, revisions, counts or actions.
type ContextNode struct {
	ID                string  `json:"id"`
	Kind              Kind    `json:"kind"`
	Code              string  `json:"code"`
	Name              string  `json:"name"`
	CategoryKey       string  `json:"category_key,omitempty"`
	ParentComponentID *string `json:"parent_component_id,omitempty"`
	ComponentID       *string `json:"component_id,omitempty"`
	PointID           string  `json:"point_id,omitempty"`
	FacilityID        string  `json:"facility_id,omitempty"`
	Status            string  `json:"status"`
	Visibility        string  `json:"visibility"`
}
