package monitoring

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicecoord"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type Service struct {
	maxCSVBytes, maxXLSXBytes int64
	displayQualification      func(context.Context, pgx.Tx, access.Actor, Scope, map[string][]Window) error
	readSubjects              func(context.Context, string) (*access.ReadSubject, error)
	workers                   WorkerOptions
	resultExportTTL           time.Duration
	access                    *access.Service
	storage                   *Storage
	rawSystem                 RawSystemAuthorizer
	executors                 map[string]TaskExecutor
	calculationReader         CalculationReader
	calculationMaintenance    CalculationMaintenance
	scheduler                 TaskScheduler
}

func (s *Service) WithDisplayQualification(fn func(context.Context, pgx.Tx, access.Actor, Scope, map[string][]Window) error) {
	s.displayQualification = fn
}

func New(a *access.Service) *Service {
	return &Service{access: a, maxCSVBytes: MaxCSVBytes, maxXLSXBytes: MaxXLSXBytes, workers: DefaultWorkerOptions(), resultExportTTL: 7 * 24 * time.Hour}
}
func (s *Service) WithUploadLimits(csv, xlsx int64) *Service {
	s.maxCSVBytes, s.maxXLSXBytes = csv, xlsx
	return s
}
func (s *Service) WithStorage(storage *Storage) *Service { s.storage = storage; return s }
func (s *Service) requireStorage() error {
	if s.storage == nil {
		return problem("monitoring_storage_unavailable", "监测存储尚未配置或不可用。", 503)
	}
	return nil
}

// Scope revisions are access-control metadata. This read requires current
// workspace membership, including for import-only users, and returns no data.
func (s *Service) AuthorizationRevision(ctx context.Context, a access.Actor, scope Scope) (string, error) {
	var out string
	e := s.transact(ctx, a, scope, "jobs.list", false, false, func(_ pgx.Tx, auth authorization) error { out = authorizationVersion(auth); return nil })
	return out, e
}

type authorization struct {
	Scope
	Account, Revision, ProjectStatus, ResourceRevision, DataRevision string
	Subject                                                          *access.ReadSubject
}

// Lock order agrees with existing resource/device operations. A governance
// change and publication share authorization locks before tenant/channel locks.
func (s *Service) transact(ctx context.Context, a access.Actor, scope Scope, action string, governance, write bool, fn func(pgx.Tx, authorization) error) error {
	if !scope.valid() {
		return notFound()
	}
	op := access.MonitoringOperation(scope.Entry, action)
	if e := access.CheckReadBoundary(a, scope.Entry, op, write || governance); e != nil {
		return e
	}
	err := s.db(ctx, func(tx pgx.Tx) error {
		policy, ok := access.PolicyFor(op)
		if !ok || a.Check == nil {
			return access.ErrDenied
		}
		rev, e := access.Lock(ctx, tx, governance)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, a.ID); e != nil {
			return e
		}
		if e = a.Check(ctx, tx, policy.Recent); e != nil {
			return e
		}
		auth := authorization{Scope: scope, Account: a.ID, Revision: rev, ResourceRevision: "0", DataRevision: "0", Subject: a.ReadSubject()}
		if scope.Entry == "project" {
			p, e := access.RequireActorProjectTx(ctx, tx, a, op, scope.Tenant, scope.Project)
			if e != nil {
				return e
			}
			auth.ProjectStatus = p.ProjectStatus
			e = tx.QueryRow(ctx, `SELECT r.revision::text,COALESCE(m.revision,0)::text FROM project_resource_policies r LEFT JOIN monitoring_access_states m USING(tenant_id,project_id) WHERE r.tenant_id=$1 AND r.project_id=$2`, scope.Tenant, scope.Project).Scan(&auth.ResourceRevision, &auth.DataRevision)
			if e != nil {
				return e
			}
		} else {
			if e = access.RequireScopedTx(ctx, tx, a.ID, op, scope.Tenant); e != nil {
				return e
			}
		}
		if _, e = tx.Exec(ctx, `SELECT id FROM engineering_catalog_state WHERE id=1 FOR SHARE`); e != nil {
			return e
		}
		if e = devicecoord.Catalog(ctx, tx, false); e != nil {
			return e
		}
		if e = devicecoord.Tenant(ctx, tx, scope.Tenant, write); e != nil {
			return e
		}
		var mode string
		if e = tx.QueryRow(ctx, `SELECT mode FROM monitoring_environment_state WHERE id=1`).Scan(&mode); e != nil {
			return e
		}
		if mode == "restoring" || mode == "read_only" && write {
			return problem("monitoring_restoring", "监测环境正在恢复核对，当前操作暂不可用。", 503)
		}
		return fn(tx, auth)
	})
	s.access.LogDenial(op, a.ID, err)
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		switch pe.Code {
		case "23505":
			return revisionChanged()
		case "23503", "23514":
			return invalid("request", "对象归属、授权或保留依据约束不满足。")
		default:
			return problem("monitoring_unavailable", "监测操作暂未完成，请核对原请求收据后重试。", 503)
		}
	}
	return err
}

func revisionNumber(s string) (int64, error) {
	n, e := strconv.ParseInt(s, 10, 64)
	if e != nil || n < 0 || strconv.FormatInt(n, 10) != s {
		return 0, invalid("expected_revision", "需要十进制修订号字符串。")
	}
	return n, nil
}

func (s *Service) ResourceReferencesTx(ctx context.Context, tx pgx.Tx, tenant, kind, id string) (string, bool, error) {
	var hash string
	var found bool
	err := tx.QueryRow(ctx, `WITH RECURSIVE affected(kind,id) AS (
 SELECT kind,id FROM engineering_resource_nodes WHERE tenant_id=$1 AND kind=$2 AND id=$3
 UNION SELECT n.kind,n.id FROM engineering_resource_nodes n JOIN affected a ON n.parent_kind=a.kind AND n.parent_id=a.id WHERE n.tenant_id=$1
 ) SELECT COALESCE(string_agg(v.channel_id::text||':'||v.first_published_sequence::text,',' ORDER BY v.channel_id),''),count(*)>0 FROM monitoring_channel_versions v JOIN affected a ON a.kind='channel' AND a.id=v.channel_id WHERE v.tenant_id=$1 AND v.first_published_sequence IS NOT NULL`, tenant, kind, id).Scan(&hash, &found)
	return digest(hash), found, err
}
func (s *Service) HasResourceReferences(ctx context.Context, tx pgx.Tx, id string) (bool, error) {
	var used bool
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM monitoring_channel_versions v JOIN engineering_channels c ON c.tenant_id=v.tenant_id AND c.id=v.channel_id JOIN engineering_facilities f ON f.id=c.facility_id WHERE v.first_published_sequence IS NOT NULL AND (c.metric_version_id=$1 OR c.unit_version_id=$1 OR f.type_version_id=$1))`, id).Scan(&used)
	return used, e
}

func (s *Service) WithReadSubjects(resolve func(context.Context, string) (*access.ReadSubject, error)) *Service {
	s.readSubjects = resolve
	return s
}
