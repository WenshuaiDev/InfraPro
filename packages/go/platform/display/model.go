// Package display owns hosted application delivery, not monitoring semantics.
package display

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
	"io"
	"time"
)

const PartSize = int64(4 << 20)

type Error struct {
	Code    string  `json:"code"`
	Message string  `json:"message"`
	Status  int     `json:"-"`
	Issues  []Issue `json:"issues,omitempty"`
}

func (e *Error) Error() string { return e.Message }

type Issue struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func problem(code, message string, status int) *Error {
	return &Error{Code: code, Message: message, Status: status}
}
func invalid(path, message string) *Error {
	return &Error{Code: "query_invalid", Message: "请求未通过校验。", Status: 422, Issues: []Issue{{path, "invalid", message}}}
}
func hidden() error { return problem("not_found", "对象不存在或不可访问。", 404) }
func conflict() error {
	return problem("revision_conflict", "内容或授权已经变化，请重新核对。", 409)
}
func jsonBytes(v any) []byte    { b, _ := json.Marshal(v); return b }
func hash(v any) string         { return hashBytes(jsonBytes(v)) }
func hashBytes(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func secret() string            { return rand.Text() + rand.Text() }
func nullable(v string) any {
	if v == "" {
		return nil
	}
	return v
}

type Scope struct {
	Tenant  string `json:"tenant_id"`
	Project string `json:"project_id"`
}
type Meta struct {
	Revision  string `json:"expected_revision"`
	RequestID string `json:"request_id"`
	Reason    string `json:"reason"`
}
type Settings struct {
	Enabled  bool   `json:"enabled"`
	Revision string `json:"revision"`
}
type Instance struct {
	ID string `json:"id"`
	Scope
	Name      string    `json:"name"`
	State     string    `json:"state"`
	Revision  string    `json:"revision"`
	Current   string    `json:"current_release_id"`
	Draft     string    `json:"draft_config_id"`
	Viewers   []string  `json:"viewers"`
	CreatedAt time.Time `json:"created_at"`
}
type Package struct {
	ID        string    `json:"id"`
	Hash      string    `json:"sha256"`
	State     string    `json:"state"`
	Manifest  Manifest  `json:"manifest"`
	CreatedAt time.Time `json:"created_at"`
}
type Config struct {
	ID         string             `json:"id"`
	PackageID  string             `json:"package_id"`
	Parameters map[string]any     `json:"parameters"`
	Bindings   map[string]Binding `json:"bindings"`
	Mode       string             `json:"mode"`
	CreatedAt  time.Time          `json:"created_at"`
}
type Binding struct {
	Kind    string   `json:"kind"`
	IDs     []string `json:"ids"`
	Dynamic *Dynamic `json:"dynamic,omitempty"`
	From    *string  `json:"from,omitempty"`
	To      *string  `json:"to,omitempty"`
}
type Dynamic struct {
	FacilityIDs   []string `json:"facility_ids"`
	ComponentIDs  []string `json:"component_ids,omitempty"`
	MetricID      string   `json:"metric_version_id,omitempty"`
	ValueKind     string   `json:"value_kind,omitempty"`
	SourceKind    string   `json:"source_kind,omitempty"`
	IncludeFuture bool     `json:"include_future"`
	BaselineIDs   []string `json:"baseline_ids,omitempty"`
}
type ReadScope struct {
	Channels  []string `json:"channels"`
	Resources []string `json:"resources"`
	Spatial   []string `json:"spatial"`
	Artifacts []string `json:"artifacts"`
	Dynamic   *Dynamic `json:"dynamic,omitempty"`
	From      *string  `json:"from,omitempty"`
	To        *string  `json:"to,omitempty"`
}
type Grant struct {
	Revision     string    `json:"revision"`
	Capabilities []string  `json:"capabilities"`
	Scope        ReadScope `json:"scope"`
	Fields       []string  `json:"fields"`
	ValidUntil   time.Time `json:"valid_until"`
	Revoked      bool      `json:"revoked"`
	IssuedBy     string    `json:"issued_by"`
}
type Release struct {
	ID           string          `json:"id"`
	PackageID    string          `json:"package_id"`
	ConfigID     string          `json:"config_id"`
	Requirements json.RawMessage `json:"requirements"`
	Withdrawn    bool            `json:"withdrawn"`
	CreatedAt    time.Time       `json:"created_at"`
}
type Capability struct {
	Name         string        `json:"name"`
	Version      int           `json:"version"`
	Availability string        `json:"availability"`
	Reason       string        `json:"reason,omitempty"`
	Permissions  []access.Code `json:"required_permissions"`
	Subscribe    bool          `json:"subscribe"`
	MaxTargets   int           `json:"max_targets"`
}
type Call struct {
	Method     string          `json:"method"`
	Params     json.RawMessage `json:"params"`
	Generation string          `json:"generation"`
}
type Envelope struct {
	State         string            `json:"state"`
	Data          any               `json:"data"`
	ReadAt        string            `json:"read_at"`
	Authorization string            `json:"authorization_revision"`
	Generation    string            `json:"generation"`
	SourceVector  map[string]string `json:"source_vector"`
}
type Provider interface {
	Read(context.Context, *access.ReadSubject, Call) (any, error)
}
type ProviderFunc func(context.Context, *access.ReadSubject, Call) (any, error)

func (f ProviderFunc) Read(c context.Context, s *access.ReadSubject, r Call) (any, error) {
	return f(c, s, r)
}

type BlobStore interface {
	Put(context.Context, string, io.Reader, int64, string) error
	Get(context.Context, string) (io.ReadCloser, error)
	Delete(context.Context, string) error
}
type Options struct {
	RuntimeOrigin, AssetOrigin, ConsoleOrigin, ShellDirectory               string
	Lease, SwitchGrace, EventRetention                                      time.Duration
	MaxSessions, MaxSubscriptions, MaxQueries, MaxInstances, MaxPackageJobs int
	MaxResultBytes, MaxFileBytes                                            int64
	Secure                                                                  bool
}

func DefaultOptions() Options {
	return Options{RuntimeOrigin: "http://127.0.0.1:5180", AssetOrigin: "http://127.0.0.1:5181", ConsoleOrigin: "http://127.0.0.1:5173", ShellDirectory: "apps/display-runtime/dist", EventRetention: 90 * 24 * time.Hour, Lease: 60 * time.Second, SwitchGrace: 30 * time.Second, MaxSessions: 100, MaxSubscriptions: 16, MaxQueries: 8, MaxInstances: 100, MaxPackageJobs: 8, MaxResultBytes: 8 << 20, MaxFileBytes: 4 << 20}
}

type Service struct {
	descriptors     map[string]Capability
	telemetry       telemetry
	access          *access.Service
	blobs           BlobStore
	options         Options
	providers       map[string]Provider
	budget          *Budget
	resolvePerson   func(context.Context, pgx.Tx, string, string) error
	qualifyBindings func(context.Context, pgx.Tx, *access.ReadSubject, map[string]Binding) error
}

func New(a *access.Service) *Service {
	descriptors := map[string]Capability{}
	for name, c := range capabilities {
		descriptors[name] = c
	}
	return &Service{descriptors: descriptors, access: a, options: DefaultOptions(), providers: map[string]Provider{}, budget: NewBudget()}
}
func (s *Service) WithStorage(b BlobStore) *Service { s.blobs = b; return s }
func (s *Service) WithOptions(o Options) *Service   { s.options = o; return s }
func (s *Service) WithPersonCheck(f func(context.Context, pgx.Tx, string, string) error) *Service {
	s.resolvePerson = f
	return s
}

// WithBindingQualification joins publication's transaction to the owning domain
// proof checks. It performs metadata qualification, never executes application code.
func (s *Service) WithBindingQualification(f func(context.Context, pgx.Tx, *access.ReadSubject, map[string]Binding) error) *Service {
	s.qualifyBindings = f
	return s
}
func (s *Service) Register(capability string, p Provider) error {
	if _, ok := capabilities[capability]; !ok || p == nil {
		return fmt.Errorf("unknown display capability")
	}
	if s.providers[capability] != nil {
		return fmt.Errorf("duplicate display provider")
	}
	s.providers[capability] = p
	c := s.descriptors[capability]
	c.Availability = "available"
	c.Reason = ""
	s.descriptors[capability] = c
	return nil
}
func (s *Service) Options() Options { return s.options }

// RegisterCapability adds the owning module's permission and subscription
// metadata for a known protocol capability. Registration occurs at startup.
func (s *Service) RegisterCapability(c Capability, p Provider) error {
	if c.Version != 1 || c.MaxTargets < 1 || c.MaxTargets > 500 {
		return fmt.Errorf("invalid display capability contract")
	}
	if e := s.Register(c.Name, p); e != nil {
		return e
	}
	c.Availability = "available"
	s.descriptors[c.Name] = c
	return nil
}

func (e *Error) StatusCode() int { return e.Status }
