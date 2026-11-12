package devices

import "time"

type Grant struct {
	ID           string     `json:"id"`
	Kind         string     `json:"kind"`
	DeviceID     string     `json:"device_id"`
	OutputID     *string    `json:"output_id"`
	CanUseSource bool       `json:"can_use_source"`
	Revision     string     `json:"revision"`
	Name         string     `json:"name"`
	DeviceName   string     `json:"device_name"`
	DeviceCode   string     `json:"device_code"`
	GrantedAt    time.Time  `json:"granted_at"`
	EndedAt      *time.Time `json:"ended_at"`
}
type GrantAdd struct {
	Kind         string `json:"kind"`
	ID           string `json:"target_id"`
	CanUseSource bool   `json:"use_source"`
}
type GrantUpdate struct {
	ID           string `json:"grant_id"`
	Revision     string `json:"expected_revision"`
	CanUseSource bool   `json:"use_source"`
}
type GrantEnd struct {
	ID       string `json:"grant_id"`
	Revision string `json:"expected_revision"`
}
type GrantChangeSet struct {
	Meta
	AccessRevision  string        `json:"expected_device_access_revision"`
	ProjectRevision string        `json:"expected_project_revision"`
	Add             []GrantAdd    `json:"add"`
	Update          []GrantUpdate `json:"update"`
	End             []GrantEnd    `json:"end"`
}
type GrantState struct {
	ProjectID       string `json:"project_id"`
	ProjectName     string `json:"project_name"`
	ProjectStatus   string `json:"project_status"`
	ProjectRevision string `json:"project_revision"`
	AccessRevision  string `json:"device_access_revision"`
}
type GrantPage struct {
	GrantState
	Items      []Grant `json:"items"`
	NextCursor string  `json:"next_cursor"`
}
type GrantCandidate struct {
	Kind       string    `json:"kind"`
	ID         string    `json:"id"`
	DeviceID   string    `json:"device_id"`
	DeviceCode string    `json:"device_code"`
	DeviceName string    `json:"device_name"`
	Name       string    `json:"name"`
	Ended      bool      `json:"ended"`
	CreatedAt  time.Time `json:"created_at"`
}
type GrantHistory struct {
	ID        string    `json:"id"`
	Revision  string    `json:"access_revision"`
	ActorName string    `json:"actor_name"`
	Reason    string    `json:"reason"`
	Before    []Grant   `json:"before"`
	After     []Grant   `json:"after"`
	ChangedAt time.Time `json:"changed_at"`
}
