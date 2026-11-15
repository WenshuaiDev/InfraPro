// Package files owns ordinary attachments, not professional domain artifacts.
package files

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/jackc/pgx/v5"
	"io"
	"time"
)

type Scope struct {
	Kind      string `json:"kind"`
	TenantID  string `json:"tenant_id"`
	ProjectID string `json:"project_id,omitempty"`
}
type HostRef struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

func (s Scope) Host() HostRef {
	if s.Kind == "project" {
		return HostRef{"project.profile", s.ProjectID}
	}
	return HostRef{"tenant.profile", s.TenantID}
}

type WriteMeta struct {
	auditBefore, auditAfter   any
	auditWorker               bool
	auditJob, auditGeneration string
	RequestID                 string `json:"request_id"`
	Reason                    string `json:"reason"`
	Epoch                     string `json:"-"`
}
type RevisionCommand struct {
	WriteMeta
	ExpectedRevision string `json:"expected_revision"`
}
type StartUploadInput struct {
	WriteMeta
	Host                       HostRef `json:"host"`
	Intent                     string  `json:"intent"`
	AttachmentID               *string `json:"attachment_id"`
	ExpectedAttachmentRevision *string `json:"expected_attachment_revision"`
	Filename                   string  `json:"filename"`
	DisplayName                string  `json:"display_name"`
	Description                string  `json:"description"`
	SizeBytes                  string  `json:"size_bytes"`
	SHA256                     string  `json:"sha256"`
	DeclaredMediaType          string  `json:"declared_media_type"`
}
type PartInput struct {
	Part          int
	Size          int64
	SHA256, Epoch string
}
type PartReceipt struct {
	Part       int       `json:"part"`
	SizeBytes  string    `json:"size_bytes"`
	SHA256     string    `json:"sha256"`
	ReceivedAt time.Time `json:"received_at"`
}
type ReconfirmInput struct {
	WriteMeta
	ExpectedGeneration         string  `json:"expected_generation"`
	ExpectedControlRevision    string  `json:"expected_control_revision"`
	ExpectedAttachmentRevision *string `json:"expected_attachment_revision"`
}
type AttachmentCommand struct {
	RevisionCommand
	Action      string  `json:"-"`
	DisplayName *string `json:"display_name,omitempty"`
	Description *string `json:"description,omitempty"`
	RevisionID  string  `json:"revision_id,omitempty"`
}
type CommandResult struct {
	UploadID             *string         `json:"upload_id"`
	Revision             string          `json:"revision"`
	State                string          `json:"state"`
	JobID                *string         `json:"job_id"`
	TaskRef              *taskrecord.Ref `json:"task_ref"`
	AttachmentID         *string         `json:"attachment_id"`
	AttachmentRevisionID *string         `json:"attachment_revision_id"`
	JobGeneration        *string         `json:"job_generation"`
	JobControlRevision   *string         `json:"job_control_revision"`
	Changed              bool            `json:"changed"`
	Replayed             bool            `json:"replayed"`
}
type Action struct {
	Kind           string `json:"kind"`
	Enabled        bool   `json:"enabled"`
	ReasonCode     string `json:"reason_code"`
	RequiresReason bool   `json:"requires_reason"`
}
type HostAction string

const (
	Metadata HostAction = "metadata"
	Content  HostAction = "content"
	Mutate   HostAction = "mutate"
)

type HostAuthorization struct {
	Host                HostRef `json:"host"`
	Scope               Scope   `json:"scope"`
	Status              string  `json:"status"`
	Version             string  `json:"version"`
	AccountID           string  `json:"-"`
	TenantMembershipID  string  `json:"-"`
	ProjectMembershipID string  `json:"-"`
	CanRead             bool    `json:"can_read"`
	CanDownload         bool    `json:"can_download"`
	CanManage           bool    `json:"can_manage"`
	ManagePermission    bool    `json:"-"`
	WritePermission     bool    `json:"-"`
	Reason              string  `json:"reason"`
}
type HostAdapter interface {
	ResolveTx(context.Context, pgx.Tx, access.Actor, Scope, HostRef, HostAction) (HostAuthorization, error)
}
type BlobStore interface {
	Put(context.Context, string, io.Reader, int64, string) error
	Get(context.Context, string) (io.ReadCloser, error)
	Delete(context.Context, string) error
}
type Capabilities struct {
	HostAuthorization
	CanViewTrash   bool     `json:"can_view_trash"`
	RestoreEpoch   string   `json:"restore_epoch"`
	Mode           string   `json:"mode"`
	AcceptUploads  bool     `json:"accept_uploads"`
	Formats        []string `json:"formats"`
	MaxFileBytes   string   `json:"max_file_bytes"`
	PartBytes      string   `json:"part_bytes"`
	MaxConcurrency int      `json:"max_concurrency"`
}
type AttachmentRevision struct {
	ID                     string    `json:"id"`
	RevisionNo             string    `json:"revision_no"`
	FileID                 string    `json:"file_id"`
	OriginalName           string    `json:"original_name"`
	Format                 string    `json:"format"`
	MediaType              string    `json:"media_type"`
	SizeBytes              string    `json:"size_bytes"`
	SHA256                 string    `json:"sha256"`
	FileState              string    `json:"file_state"`
	PreviewState           string    `json:"preview_state"`
	PreviewReason          string    `json:"preview_reason"`
	Operation              string    `json:"operation"`
	Reason                 string    `json:"reason"`
	CreatedBy              string    `json:"created_by"`
	CreatedAt              time.Time `json:"created_at"`
	PreviousRevisionID     *string   `json:"previous_revision_id"`
	RevertedFromRevisionID *string   `json:"reverted_from_revision_id"`
	JobID                  *string   `json:"job_id"`
	JobGeneration          *string   `json:"job_generation"`
}
type AttachmentDetail struct {
	ID              string             `json:"id"`
	Host            HostRef            `json:"host"`
	DisplayName     string             `json:"display_name"`
	Description     string             `json:"description"`
	Status          string             `json:"status"`
	Revision        string             `json:"revision"`
	CurrentRevision AttachmentRevision `json:"current_revision"`
	CreatedBy       string             `json:"created_by"`
	UpdatedBy       string             `json:"updated_by"`
	CreatedAt       time.Time          `json:"created_at"`
	UpdatedAt       time.Time          `json:"updated_at"`
	PurgeAfter      *time.Time         `json:"purge_after"`
	AllowedActions  []Action           `json:"allowed_actions"`
}
type AttachmentFilter struct {
	Host                   HostRef
	State, Q, Cursor       string
	CreatedFrom, CreatedTo *time.Time
	Limit                  int
}
type AttachmentPage struct {
	Items                 []AttachmentDetail `json:"items"`
	NextCursor            string             `json:"next_cursor"`
	ContentRevision       string             `json:"content_revision"`
	AuthorizationRevision string             `json:"authorization_revision"`
}
type RevisionPage struct {
	Items      []AttachmentRevision `json:"items"`
	NextCursor string               `json:"next_cursor"`
}
type UploadDetail struct {
	ID                         string        `json:"id"`
	Host                       HostRef       `json:"host"`
	Intent                     string        `json:"intent"`
	AttachmentID               *string       `json:"attachment_id"`
	ExpectedAttachmentRevision *string       `json:"expected_attachment_revision"`
	Filename                   string        `json:"filename"`
	DisplayName                string        `json:"display_name"`
	Description                string        `json:"description"`
	SizeBytes                  string        `json:"size_bytes"`
	SHA256                     string        `json:"sha256"`
	PartSize                   string        `json:"part_size"`
	PartCount                  int           `json:"part_count"`
	State                      string        `json:"state"`
	Revision                   string        `json:"revision"`
	RestoreEpoch               string        `json:"restore_epoch"`
	ExpiresAt                  time.Time     `json:"expires_at"`
	CreatedAt                  time.Time     `json:"created_at"`
	Parts                      []PartReceipt `json:"parts"`
	Job                        *JobSummary   `json:"job"`
	PreparedUntil              *time.Time    `json:"prepared_until"`
	AllowedActions             []Action      `json:"allowed_actions"`
}
type UploadFilter struct {
	View, State, Cursor string
	Limit               int
}
type UploadPage struct {
	Items      []UploadDetail `json:"items"`
	NextCursor string         `json:"next_cursor"`
}
type JobSummary struct {
	ID              string         `json:"id"`
	Generation      string         `json:"generation"`
	ControlRevision string         `json:"control_revision"`
	State           string         `json:"state"`
	Phase           string         `json:"phase"`
	TaskRef         taskrecord.Ref `json:"task_ref"`
}
type JobDetail struct {
	JobSummary
	UploadID           string         `json:"upload_id"`
	OriginatorID       string         `json:"originator_id"`
	RequestedBy        string         `json:"requested_by"`
	Attempt            int            `json:"attempt"`
	ErrorCode          string         `json:"error_code"`
	ResultFileID       *string        `json:"result_file_id"`
	ResultAttachmentID *string        `json:"result_attachment_id"`
	ResultRevisionID   *string        `json:"result_revision_id"`
	ResultState        string         `json:"result_state"`
	ResultReadable     bool           `json:"result_readable"`
	CreatedAt          time.Time      `json:"created_at"`
	FinishedAt         *time.Time     `json:"finished_at"`
	PreparedUntil      *time.Time     `json:"prepared_until"`
	AllowedActions     []Action       `json:"allowed_actions"`
	Report             map[string]any `json:"report"`
}
type Protection struct {
	SpaceID, HostID, FileID, Module, OwnerKind, OwnerID, OwnerRevision, Kind string
	ExpiresAt                                                                *time.Time
}
type Release struct {
	Protection
	Reason string
}
