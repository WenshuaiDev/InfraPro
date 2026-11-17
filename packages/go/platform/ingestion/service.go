package ingestion

import (
	"context"
	"encoding/json"
	"errors"
	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicecoord"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/parsing"
	"github.com/jackc/pgx/v5"
	"strconv"
	"strings"
)

type Service struct {
	access  *access.Service
	parser  *parsing.Service
	objects parsing.ObjectStore
	options Options
}

func New(a *access.Service, p *parsing.Service) *Service {
	return &Service{access: a, parser: p, options: Options{StorageBudget: 1 << 40, QueueBudget: 100000, Relays: []RelayConfig{}}}
}
func (s *Service) WithOptions(o Options) *Service             { s.options = o; return s }
func (s *Service) WithStorage(o parsing.ObjectStore) *Service { s.objects = o; return s }
func op(scope Scope, kind, action string) access.Operation {
	return access.IngestionOperation(scope.Entry, kind, action)
}
func scopeKey(s Scope) string {
	if s.Entry == "platform" {
		return "platform"
	}
	return "tenant:" + s.Tenant
}
func (s *Service) transact(ctx context.Context, a access.Actor, scope Scope, kind, action string, write bool, fn func(pgx.Tx, k.Authorization) error) error {
	return k.Transact(ctx, s.access, a, scope, op(scope, kind, action), k.Locks{CatalogWrite: write && scope.Entry == "platform", TenantWrite: write && scope.Entry == "tenant"}, fn)
}
func meta(m *Meta) error {
	if e := m.Normalize(); e != nil {
		return e
	}
	return m.RequireReason()
}
func read[T any](row pgx.Row) (T, error) {
	var out T
	var b []byte
	e := row.Scan(&b)
	if e == pgx.ErrNoRows {
		return out, k.NotFound("ingestion")
	}
	if e != nil {
		return out, e
	}
	e = json.Unmarshal(b, &out)
	return out, e
}
func num(s string) int64    { n, _ := strconv.ParseInt(s, 10, 64); return n }
func numstr(n int64) string { return strconv.FormatInt(n, 10) }
func replay(ctx context.Context, tx pgx.Tx, a k.Authorization, kind string, m Meta, input any) (Result, bool, error) {
	var out Result
	var hash string
	e := tx.QueryRow(ctx, `SELECT payload_hash,result FROM ingestion_operation_receipts WHERE scope_key=$1 AND actor_id=$2 AND operation=$3 AND request_id=$4`, scopeKey(a.Scope), a.Account, kind, m.RequestID).Scan(&hash, &out)
	if e == pgx.ErrNoRows {
		return out, false, nil
	}
	if e != nil {
		return out, false, e
	}
	if hash != k.Hash(input) {
		return out, false, k.Problem("request_conflict", "相同请求 ID 已用于不同内容。", 409)
	}
	out.Replayed = true
	return out, true, nil
}
func complete(ctx context.Context, tx pgx.Tx, a k.Authorization, kind string, m Meta, input any, out Result, before, after any) error {
	if _, e := tx.Exec(ctx, `INSERT INTO ingestion_operation_receipts(scope_key,actor_id,operation,request_id,payload_hash,result) VALUES($1,$2,$3,$4,$5,$6)`, scopeKey(a.Scope), a.Account, kind, m.RequestID, k.Hash(input), k.JSON(out)); e != nil {
		return e
	}
	if _, e := tx.Exec(ctx, `INSERT INTO ingestion_changes(scope_key,actor_id,operation,target_id,request_id,reason,before_snapshot,after_snapshot) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, scopeKey(a.Scope), a.Account, kind, out.ID, m.RequestID, m.Reason, k.JSON(before), k.JSON(after)); e != nil {
		return e
	}
	return access.Audit(ctx, tx, "ingestion."+kind, "ingestion_"+strings.Split(kind, ".")[0], out.ID, "", before, after, access.Meta{ActorID: a.Account, ActorScope: a.Entry, TenantID: a.Tenant, RequestID: m.RequestID, Reason: m.Reason})
}

const sourceJSON = `to_jsonb(s)||jsonb_build_object('revision',s.revision::text,'applied_generation',s.applied_generation::text,'applied_revision',s.applied_revision::text)`
const revisionJSON = `to_jsonb(r)||jsonb_build_object('revision',r.revision::text,'expected_generation',r.expected_generation::text,'confirmed',EXISTS(SELECT 1 FROM ingestion_source_confirmations c WHERE c.source_id=r.source_id AND c.revision=r.revision AND c.digest=r.digest))`
const terminalJSON = `to_jsonb(t)||jsonb_build_object('revision',t.revision::text)`
const associationJSON = `to_jsonb(a)||jsonb_build_object('terminal_revision',a.terminal_revision::text,'start_sequence',a.start_sequence::text,'end_sequence',a.end_sequence::text)`
const grantJSON = `to_jsonb(g)||jsonb_build_object('generation',g.generation::text)`
const receiptJSON = `to_jsonb(r)||jsonb_build_object('source_revision',r.source_revision::text,'source_sequence',r.source_sequence::text,'current_generation',r.current_generation::text)`
const decisionJSON = `to_jsonb(d)||jsonb_build_object('generation',d.generation::text,'recovery_generation',d.recovery_generation::text,'fence',d.fence::text,'grant_generation',d.grant_generation::text)`
const planJSON = `to_jsonb(p)||jsonb_build_object('fence',p.fence::text)`

func sourceTx(ctx context.Context, tx pgx.Tx, scope Scope, id string) (Source, error) {
	return read[Source](tx.QueryRow(ctx, `SELECT `+sourceJSON+` FROM ingestion_sources s WHERE id=$1 AND ($2='platform' OR tenant_id=$3::uuid)`, id, scope.Entry, k.Nullable(scope.Tenant)))
}
func revisionTx(ctx context.Context, tx pgx.Tx, id, revision string) (SourceRevision, error) {
	return read[SourceRevision](tx.QueryRow(ctx, `SELECT `+revisionJSON+` FROM ingestion_source_revisions r WHERE source_id=$1 AND revision=$2`, id, revision))
}
func terminalTx(ctx context.Context, tx pgx.Tx, tenant, id string) (Terminal, error) {
	return read[Terminal](tx.QueryRow(ctx, `SELECT `+terminalJSON+` FROM ingestion_terminals t WHERE tenant_id=$1 AND id=$2`, tenant, id))
}
func associationTx(ctx context.Context, tx pgx.Tx, tenant, id string) (Association, error) {
	return read[Association](tx.QueryRow(ctx, `SELECT `+associationJSON+` FROM ingestion_device_associations a WHERE tenant_id=$1 AND id=$2`, tenant, id))
}
func receiptTx(ctx context.Context, tx pgx.Tx, id string) (Receipt, error) {
	return read[Receipt](tx.QueryRow(ctx, `SELECT `+receiptJSON+` FROM ingestion_receipts r WHERE id=$1`, id))
}
func (s *Service) machineTx(ctx context.Context, tenant string, write bool, fn func(pgx.Tx) error) error {
	return s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, false); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `SELECT id FROM engineering_catalog_state WHERE id=1 FOR SHARE`); e != nil {
			return e
		}
		if e := devicecoord.Catalog(ctx, tx, false); e != nil {
			return e
		}
		if tenant != "" {
			if e := devicecoord.Tenant(ctx, tx, tenant, write); e != nil {
				return e
			}
		}
		return fn(tx)
	})
}
func businessTx(ctx context.Context, tx pgx.Tx, s Source, device string) (string, error) {
	var active bool
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM tenants t JOIN projects p ON p.tenant_id=t.id WHERE t.id=$1 AND t.status='active' AND p.id=$2 AND p.status='active')`, s.TenantID, s.ProjectID).Scan(&active)
	if e != nil {
		return "", e
	}
	if !active {
		return "", k.Problem("routing_scope_mismatch", "租户或来源项目当前不可用。", 409)
	}
	if device == "" {
		return "", nil
	}
	var grant string
	e = tx.QueryRow(ctx, `SELECT g.id::text FROM devices d JOIN device_products p ON p.id=d.product_id JOIN device_manufacturers m ON m.id=p.manufacturer_id JOIN project_device_grants g ON g.tenant_id=d.tenant_id AND g.device_id=d.id AND g.kind='device' AND g.project_id=$3 AND g.ended_at IS NULL WHERE d.tenant_id=$1 AND d.id=$2 AND d.deleted_at IS NULL AND d.status='enabled' AND p.manufacturer_id=$4 AND m.deleted_at IS NULL AND m.status='active' AND (($5='public' AND m.owner_tenant_id IS NULL) OR ($5='tenant' AND m.owner_tenant_id=$1))`, s.TenantID, device, s.ProjectID, s.ManufacturerID, s.ManufacturerScope).Scan(&grant)
	if errors.Is(e, pgx.ErrNoRows) {
		return "", k.Problem("routing_scope_mismatch", "设备、项目授权与入口厂家不一致。", 409)
	}
	return grant, e
}
func code(e error) string {
	if e == nil {
		return ""
	}
	var ce *c.Error
	if errors.As(e, &ce) {
		return ce.Code
	}
	var v *k.Error
	if errors.As(e, &v) {
		return v.Code
	}
	if errors.Is(e, access.ErrDenied) {
		return "authorization_revoked"
	}
	return parsing.ErrorCode(e)
}

func filterContext(a k.Authorization, f Filter, kind string) string {
	f.Cursor = ""
	return k.CursorContext(a, f.Filter, kind+":"+k.Hash(f))
}
