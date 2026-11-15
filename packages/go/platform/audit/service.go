package audit

import (
	"context"
	"io"
	"slices"
	"strconv"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	ar "github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/jackc/pgx/v5"
)

type BlobStore interface {
	Put(context.Context, string, io.Reader, int64, string) error
	Get(context.Context, string) (io.ReadCloser, error)
	Delete(context.Context, string) error
}
type Service struct {
	handoff       func(context.Context, pgx.Tx, int) (int, error)
	handoffReplay func(context.Context, pgx.Tx, string, int) (bool, error)
	restore       func(context.Context, pgx.Tx, string) error
	access        *access.Service
	disclosure    DisclosureResolver
	trace         TraceResolver
	objects       BlobStore
	options       config.Audit
	now           func() time.Time
}

func New(a *access.Service) *Service {
	s := &Service{access: a, options: config.DefaultAudit(), now: func() time.Time { return time.Now().UTC() }}
	if a != nil {
		a.WithAuditReader(s.LegacyEvents)
	}
	return s
}
func (s *Service) WithResolvers(d DisclosureResolver, t TraceResolver) *Service {
	s.disclosure = d
	s.trace = t
	return s
}
func (s *Service) WithStorage(b BlobStore) *Service        { s.objects = b; return s }
func (s *Service) WithOptions(v config.Audit) *Service     { s.options = v; return s }
func (s *Service) WithClock(now func() time.Time) *Service { s.now = now; return s }

type authorization struct {
	access.AuditAuthorization
	Epoch, Secret, Projection, Mode, Vector string
}

func (s *Service) authorizeTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope) (authorization, error) {
	out := authorization{}
	if a.ReadSubject() != nil || a.Check == nil || !scope.Valid() {
		return out, access.ErrDenied
	}
	if e := tx.QueryRow(ctx, `SELECT mode,restore_epoch::text,cursor_secret,projection_version::text FROM audit_protocol WHERE id=1 AND version=1 FOR SHARE`).Scan(&out.Mode, &out.Epoch, &out.Secret, &out.Projection); e != nil {
		return out, Problem("audit_protocol_unavailable", 503)
	}
	revision, e := access.Lock(ctx, tx, false)
	if e != nil {
		return out, e
	}
	if e = a.Check(ctx, tx, false); e != nil {
		return out, e
	}
	out.AuditAuthorization, e = access.AuditAuthorizationTx(ctx, tx, a, scope, revision)
	if e != nil {
		return out, e
	}
	out.Vector, e = s.vectorTx(ctx, tx, a, out)
	if e != nil {
		return out, e
	}
	return out, nil
}
func (s *Service) transaction(ctx context.Context, a access.Actor, scope Scope, fn func(pgx.Tx, authorization) error) error {
	return s.access.Transact(ctx, func(tx pgx.Tx) error {
		auth, e := s.authorizeTx(ctx, tx, a, scope)
		if e != nil {
			return e
		}
		return fn(tx, auth)
	})
}
func (s *Service) Capabilities(ctx context.Context, a access.Actor, scope Scope) (Capabilities, error) {
	out := Capabilities{}
	e := s.transaction(ctx, a, scope, func(_ pgx.Tx, auth authorization) error {
		profiles := []string{"standard"}
		if auth.Has("technical.read") {
			profiles = append(profiles, "technical")
		}
		out = Capabilities{Read: true, Export: auth.Has("export"), Technical: auth.Has("technical.read"), Manage: auth.Has("exports.manage"), FieldProfiles: profiles, Epoch: auth.Epoch, AuthorizationRevision: auth.Vector, ProjectionVersion: auth.Projection, MembershipKey: auth.MembershipKey, MaxRows: strconv.Itoa(s.options.ExportMaxRows), MaxBytes: strconv.FormatInt(s.options.ExportMaxBytes, 10), ResultTTLSeconds: strconv.FormatInt(int64(s.options.ResultTTL.Seconds()), 10), StorageAvailable: s.objects != nil}
		return nil
	})
	return out, e
}
func visibleClasses(auth authorization) []string {
	switch auth.Scope.Kind {
	case "platform":
		return []string{"platform", "platform_tenant_notice"}
	case "tenant":
		classes := []string{"tenant", "platform_tenant_notice"}
		if slices.Contains(auth.Permissions, access.TenantProjectRead) {
			classes = append(classes, "tenant_project_governance")
		}
		return classes
	default:
		return []string{"project"}
	}
}
func scopeAllows(auth authorization, r rawEvent) bool {
	if !slices.Contains(visibleClasses(auth), r.Visibility) {
		return false
	}
	switch auth.Scope.Kind {
	case "platform":
		return r.Scope.Project == "" && (r.Scope.Tenant == "" || slices.Contains(auth.Permissions, access.TenantRead))
	case "tenant":
		return r.Scope.Tenant == auth.Scope.Tenant && r.Scope.Project == ""
	default:
		return r.Scope.Tenant == auth.Scope.Tenant && r.Scope.Project == auth.Scope.Project
	}
}

// RevisionResolver exposes finite authorization versions, without domain records.
type RevisionResolver interface {
	RevisionTx(context.Context, pgx.Tx, Scope) (string, error)
}

func (s *Service) vectorTx(ctx context.Context, tx pgx.Tx, a access.Actor, auth authorization) (string, error) {
	resource := ""
	var e error
	if resolver, ok := s.disclosure.(RevisionResolver); ok {
		resource, e = resolver.RevisionTx(ctx, tx, auth.Scope)
		if e != nil {
			return "", sourceUnavailable(e)
		}
	}
	membership := ""
	if auth.Scope.Kind != "platform" {
		e = tx.QueryRow(ctx, `SELECT COALESCE((SELECT version::text FROM tenant_memberships WHERE id::text=split_part($1,':',1)),'')||':'||COALESCE((SELECT version::text FROM project_memberships WHERE id::text=split_part($1,':',2)),'')`, auth.MembershipKey).Scan(&membership)
		if e != nil {
			return "", e
		}
	}
	return ar.Hash([]string{a.ID, auth.Revision, auth.AccountVersion, auth.MembershipKey, membership, auth.Scope.Key(), auth.Epoch, auth.Projection, resource}), nil
}

func (s *Service) WithRestoreCoordinator(fn func(context.Context, pgx.Tx, string) error) *Service {
	s.restore = fn
	return s
}
