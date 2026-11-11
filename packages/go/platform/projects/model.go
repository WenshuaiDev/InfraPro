// Package projects owns complete project commands and their transactional boundaries.
package projects

import (
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"golang.org/x/text/unicode/norm"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type Fields struct {
	Name         string `json:"name"`
	Code         string `json:"code"`
	Purpose      string `json:"purpose"`
	Description  string `json:"description"`
	PlannedStart string `json:"planned_start"`
	PlannedEnd   string `json:"planned_end"`
}
type WriteMeta struct {
	RequestID string `json:"client_request_id"`
	Reason    string `json:"reason"`
}
type ChangeMeta struct {
	WriteMeta
	Version        string `json:"expected_version"`
	ProjectVersion string `json:"expected_project_version"`
	Hash           string `json:"preview_hash"`
}
type CreateInput struct {
	Fields
	WriteMeta
	Owner         string `json:"owner_tenant_membership_id"`
	Administrator string `json:"administrator_tenant_membership_id"`
}
type UpdateInput struct {
	Fields
	ChangeMeta
}
type CandidateInput struct {
	ChangeMeta
	Candidate string `json:"tenant_membership_id"`
}
type AddMemberInput struct {
	WriteMeta
	ProjectVersion string `json:"expected_project_version"`
	Candidate      string `json:"tenant_membership_id"`
}
type RoleInput struct {
	ChangeMeta
	Name          string        `json:"name"`
	Description   string        `json:"description"`
	Permissions   []access.Code `json:"permission_codes"`
	SourceVersion string        `json:"source_version"`
}
type PermissionsInput struct {
	ChangeMeta
	Permissions []access.Code `json:"permission_codes"`
}
type RolesInput struct {
	ChangeMeta
	Roles        []string          `json:"role_ids"`
	RoleVersions map[string]string `json:"role_versions"`
}
type StatusInput struct {
	ChangeMeta
	Status string `json:"status"`
}

// Change is internal to the service adapter; HTTP decodes operation-specific inputs.
type Change struct {
	ChangeMeta
	Candidate    string            `json:"tenant_membership_id,omitempty"`
	Permissions  []access.Code     `json:"permission_codes,omitempty"`
	Roles        []string          `json:"role_ids,omitempty"`
	RoleVersions map[string]string `json:"role_versions,omitempty"`
	Status       string            `json:"status,omitempty"`
}
type Person struct {
	MembershipID string `json:"tenant_membership_id"`
	AccountID    string `json:"account_id"`
	Username     string `json:"username"`
	DisplayName  string `json:"display_name"`
	Available    bool   `json:"available"`
}
type Summary struct {
	ID                        string    `json:"id"`
	TenantID                  string    `json:"tenant_id"`
	TenantName                string    `json:"tenant_name"`
	Name                      string    `json:"name"`
	Code                      string    `json:"code"`
	Status                    string    `json:"status"`
	Version                   string    `json:"version"`
	Owner                     Person    `json:"owner"`
	Administrator             Person    `json:"administrator"`
	AdministratorMembershipID string    `json:"administrator_membership_id"`
	ManagementGap             bool      `json:"management_gap"`
	PlannedStart              string    `json:"planned_start"`
	PlannedEnd                string    `json:"planned_end"`
	CreatedAt                 time.Time `json:"created_at"`
	UpdatedAt                 time.Time `json:"updated_at"`
}
type Project struct {
	Summary
	Purpose             string `json:"purpose"`
	Description         string `json:"description"`
	CreatedBy           string `json:"created_by"`
	UpdatedBy           string `json:"updated_by"`
	ArchivedAt          string `json:"archived_at"`
	ArchivedBy          string `json:"archived_by"`
	ArchiveReason       string `json:"archive_reason"`
	ManagementGapReason string `json:"management_gap_reason"`
}
type Context struct {
	ResourceAccessRevision string                      `json:"resource_access_revision"`
	CanCreateFacility      bool                        `json:"can_create_facility"`
	Project                Summary                     `json:"project"`
	Authorization          access.ProjectAuthorization `json:"authorization"`
	CanEnter               bool                        `json:"can_enter"`
}
type Member struct {
	ID                     string    `json:"id"`
	TenantMembershipID     string    `json:"tenant_membership_id"`
	AccountID              string    `json:"account_id"`
	Username               string    `json:"username"`
	DisplayName            string    `json:"display_name"`
	AccountStatus          string    `json:"account_status"`
	TenantMembershipStatus string    `json:"tenant_membership_status"`
	Status                 string    `json:"status"`
	Version                string    `json:"version"`
	Administrator          bool      `json:"is_current_administrator"`
	JoinedAt               time.Time `json:"joined_at"`
	StatusReason           string    `json:"status_reason"`
	EndedAt                string    `json:"ended_at"`
}
type Result struct {
	ProjectID      string `json:"project_id"`
	ObjectID       string `json:"object_id"`
	Version        string `json:"version"`
	ProjectVersion string `json:"project_version"`
	Revision       string `json:"authorization_revision"`
	Replayed       bool   `json:"replayed"`
}
type MemberImpact struct {
	MembershipID      string                      `json:"membership_id"`
	DisplayName       string                      `json:"display_name"`
	Status            string                      `json:"status"`
	Added             []access.Code               `json:"added"`
	Removed           []access.Code               `json:"removed"`
	RestorableAdded   []access.Code               `json:"restorable_added"`
	RestorableRemoved []access.Code               `json:"restorable_removed"`
	Before            access.ProjectAuthorization `json:"before"`
	After             access.ProjectAuthorization `json:"after"`
}
type Preview struct {
	Version         string            `json:"expected_version"`
	ProjectVersion  string            `json:"expected_project_version"`
	Hash            string            `json:"preview_hash"`
	CatalogChecksum string            `json:"catalog_checksum"`
	CanSubmit       bool              `json:"can_submit"`
	BlockedReason   string            `json:"blocked_reason"`
	Before          any               `json:"before"`
	After           any               `json:"after"`
	Members         []MemberImpact    `json:"members"`
	MemberCount     int               `json:"member_count"`
	LosingMembers   int               `json:"losing_members"`
	RoleVersions    map[string]string `json:"role_versions"`
	Notes           []string          `json:"notes"`
}
type Filter struct {
	Query, Cursor, Status, Owner, Unavailable, Event, From, To, ObjectID, ActorID string
	Limit                                                                         int
}

func problem(code, message string, status int) error {
	return &access.Error{Code: code, Message: message, Status: status}
}

var codePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{2,31}$`)

func textField(field, v string, min, max int) error {
	if !utf8.ValidString(v) || utf8.RuneCountInString(v) < min || utf8.RuneCountInString(v) > max {
		return access.Validation(field, "内容长度或格式不符合要求。")
	}
	return nil
}
func (f *Fields) normalize() error {
	f.Name = strings.TrimSpace(norm.NFKC.String(f.Name))
	f.Code = strings.ToLower(strings.TrimSpace(f.Code))
	f.Purpose = strings.TrimSpace(f.Purpose)
	f.Description = strings.TrimSpace(f.Description)
	for _, v := range []struct {
		key, value string
		min, max   int
	}{{"name", f.Name, 1, 100}, {"purpose", f.Purpose, 1, 1000}, {"description", f.Description, 0, 2000}} {
		if e := textField(v.key, v.value, v.min, v.max); e != nil {
			return e
		}
	}
	if !codePattern.MatchString(f.Code) {
		return access.Validation("code", "编号需为 3–32 位小写字母、数字、下划线或连字符，并以字母或数字开头。")
	}
	for _, d := range []string{f.PlannedStart, f.PlannedEnd} {
		if d != "" {
			parsed, e := time.Parse("2006-01-02", d)
			if e != nil || parsed.Format("2006-01-02") != d {
				return access.Validation("planned_start", "日期格式应为 YYYY-MM-DD。")
			}
		}
	}
	if f.PlannedStart != "" && f.PlannedEnd != "" && f.PlannedStart > f.PlannedEnd {
		return access.Validation("planned_end", "结束日期不得早于开始日期。")
	}
	return nil
}
func version(have, want string) error {
	v, e := strconv.ParseInt(want, 10, 64)
	if e != nil || v < 1 || strconv.FormatInt(v, 10) != want {
		return access.Validation("expected_version", "请提交有效版本号。")
	}
	if have != want {
		return access.ErrConflict
	}
	return nil
}
func (m *WriteMeta) normalize() error {
	m.Reason = strings.TrimSpace(m.Reason)
	m.RequestID = strings.ToLower(m.RequestID)
	if e := access.ValidReason(m.Reason); e != nil {
		return e
	}
	if !access.ValidID(m.RequestID) {
		return access.Validation("client_request_id", "请求编号需为 UUID。")
	}
	return nil
}
