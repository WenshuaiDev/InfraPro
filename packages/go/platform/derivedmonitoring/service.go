package derivedmonitoring

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/compute"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/resultstore"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicecoord"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type Service struct {
	readiness       func(context.Context) error
	options         config.Derived
	access          *access.Service
	resources       *resources.Service
	monitoring      *monitoring.Service
	raw             monitoring.RawReader
	results         *resultstore.Store
	schedulerMu     sync.Mutex
	schedulerCursor string
}

func New(a *access.Service) *Service {
	return &Service{access: a, resources: resources.New(a), options: config.DefaultDerived()}
}
func (s *Service) WithResources(r *resources.Service) *Service { s.resources = r; return s }
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
func jsonBytes(v any) []byte   { b, _ := json.Marshal(v); return b }
func fingerprint(v any) string { sum := sha256.Sum256(jsonBytes(v)); return hex.EncodeToString(sum[:]) }
func revisionNumber(text string) (int64, error) {
	v, e := strconv.ParseInt(text, 10, 64)
	if e != nil || v < 1 || strconv.FormatInt(v, 10) != text {
		return 0, invalid("/expected_revision", "需要正整数修订号字符串。")
	}
	return v, nil
}
func checkMeta(m WriteMeta, revision, reason bool) error {
	if !access.ValidID(m.RequestID) {
		return invalid("/request_id", "需要有效 UUID 请求编号。")
	}
	if revision {
		if _, e := revisionNumber(m.Revision); e != nil {
			return e
		}
	}
	if reason {
		if e := access.ValidReason(m.Reason); e != nil {
			return e
		}
	}
	return nil
}

var codePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

func checkName(name, description string) error {
	if strings.TrimSpace(name) == "" || utf8.RuneCountInString(name) > 200 {
		return invalid("/name", "名称为 1–200 字符。")
	}
	if utf8.RuneCountInString(description) > 4000 {
		return invalid("/description", "说明最多 4000 字符。")
	}
	return nil
}
func compile(rule json.RawMessage, mode string, values map[string]string) (*compute.Plan, error) {
	return compileLimits(rule, mode, values, compute.DefaultLimits())
}
func (s *Service) WithOptions(o config.Derived) *Service {
	if o.RuleInputs != 0 {
		s.options = o
	}
	return s
}
func (s *Service) compile(rule json.RawMessage, mode string, values map[string]string) (*compute.Plan, error) {
	limits := compute.DefaultLimits()
	limits.Inputs = s.options.RuleInputs
	limits.Nodes = s.options.RuleNodes
	limits.Parameters = s.options.RuleParameters
	limits.WindowSamples = s.options.WindowSamples
	limits.OutputRows = s.options.OutputRows
	return compileLimits(rule, mode, values, limits)
}
func compileLimits(rule json.RawMessage, mode string, values map[string]string, limits compute.Limits) (*compute.Plan, error) {
	p, e := compute.Compile(rule, mode, values, limits)
	if e == nil {
		return p, nil
	}
	var i *compute.Issue
	if errors.As(e, &i) {
		status := 422
		if i.Code == "malformed_request" || i.Code == "duplicate_field" || i.Code == "unknown_field" {
			status = 400
		}
		err := problem(i.Code, "计算规则未通过校验。", status)
		err.Issues = []Issue{*i}
		return nil, err
	}
	return nil, e
}

// All user entry points recheck current actor and workspace under authorization
// locks. Project paths never fall back to tenant membership. Graph state is the
// final configuration lock before channel/definition locks.
func (s *Service) transact(ctx context.Context, a access.Actor, scope Scope, action string, write bool, fn func(pgx.Tx) error) error {
	if !scope.valid() {
		return notFound()
	}
	op := access.DerivedOperation(scope.Entry, action)
	err := s.access.Transact(ctx, func(tx pgx.Tx) error {
		policy, ok := access.PolicyFor(op)
		if !ok || a.Check == nil {
			return access.ErrDenied
		}
		if _, e := access.Lock(ctx, tx, scope.Entry == "project" && action == "channels.create"); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, a.ID); e != nil {
			return e
		}
		if e := a.Check(ctx, tx, policy.Recent); e != nil {
			return e
		}
		if scope.Entry == "project" {
			if _, e := access.RequireProjectTx(ctx, tx, a.ID, op, scope.Tenant, scope.Project); e != nil {
				return e
			}
		} else {
			if e := access.RequireScopedTx(ctx, tx, a.ID, op, scope.Tenant); e != nil {
				return e
			}
		}
		if _, e := tx.Exec(ctx, `SELECT id FROM engineering_catalog_state WHERE id=1 FOR SHARE`); e != nil {
			return e
		}
		if e := devicecoord.Catalog(ctx, tx, false); e != nil {
			return e
		}
		if e := devicecoord.Tenant(ctx, tx, scope.Tenant, write); e != nil {
			return e
		}
		var mode string
		if e := tx.QueryRow(ctx, `SELECT mode FROM monitoring_environment_state WHERE id=1`).Scan(&mode); e != nil {
			return e
		}
		if mode == "restoring" || write && mode != "ready" {
			return problem("dependency_unavailable", "监测环境尚未就绪。", 503)
		}
		lock := " FOR SHARE"
		if write {
			lock = " FOR UPDATE"
		}
		var revision string
		if e := tx.QueryRow(ctx, `SELECT revision::text FROM derived_graph_states WHERE tenant_id=$1`+lock, scope.Tenant).Scan(&revision); e != nil {
			return e
		}
		return fn(tx)
	})
	s.access.LogDenial(op, a.ID, err)
	var re *resources.Error
	if errors.As(err, &re) {
		return problem(re.Code, re.Message, re.Status)
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "23505":
			return problem("definition_conflict", "编号已占用或保留，请重新读取。", 409)
		case "23503", "23514":
			return problem("reference_conflict", "操作不满足引用或历史保护约束。", 409)
		default:
			return problem("unavailable", "操作暂未完成，请使用原请求编号核对后重试。", 503)
		}
	}
	return err
}
func receipt(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, action string, meta WriteMeta, payload any) (MetricResult, bool, error) {
	var result MetricResult
	var oldAction, oldHash string
	e := tx.QueryRow(ctx, `SELECT operation,payload_hash,result FROM derived_operation_receipts WHERE tenant_id=$1 AND scope_key=$2 AND actor_id=$3 AND request_id=$4`, scope.Tenant, scope.key(), a.ID, meta.RequestID).Scan(&oldAction, &oldHash, &result)
	if e == pgx.ErrNoRows {
		return result, false, nil
	}
	if e != nil {
		return result, false, e
	}
	if oldAction != action || oldHash != fingerprint(payload) {
		return result, false, problem("idempotency_conflict", "此请求编号已用于不同操作或内容。", 409)
	}
	result.Replayed = true
	return result, true, nil
}
func complete(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, action string, meta WriteMeta, payload any, result MetricResult, before any, audit access.Meta) error {
	audit.ActorID = a.ID
	audit.ActorType = "account"
	audit.ActorScope = scope.Entry
	audit.TenantID = scope.Tenant
	audit.ProjectID = scope.Project
	audit.Reason = meta.Reason
	if e := access.Audit(ctx, tx, "derived."+action, "derived_metric_definition", result.Definition.ID, "", before, result, audit); e != nil {
		return e
	}
	if _, e := tx.Exec(ctx, `INSERT INTO derived_changes(tenant_id,project_id,actor_kind,actor_id,operation,target_kind,target_id,request_id,reason,before,after) VALUES($1,$2,'user',$3,$4,'metric_definition',$5,$6,$7,$8,$9)`, scope.Tenant, nullable(scope.Project), a.ID, action, result.Definition.ID, meta.RequestID, meta.Reason, jsonBytes(before), jsonBytes(result)); e != nil {
		return e
	}
	_, e := tx.Exec(ctx, `INSERT INTO derived_operation_receipts(tenant_id,project_id,actor_id,request_id,operation,payload_hash,result) VALUES($1,$2,$3,$4,$5,$6,$7)`, scope.Tenant, nullable(scope.Project), a.ID, meta.RequestID, action, fingerprint(payload), jsonBytes(result))
	return e
}

// Public catalog references are validated under the engineering catalog lock;
// names cannot substitute for fixed unit/metric versions or infer scale type.
func publicReferences(ctx context.Context, tx pgx.Tx, p *compute.Plan, newUse bool) (map[string]engineering.Version, error) {
	expected := map[string]engineering.Kind{}
	add := func(id string, kind engineering.Kind) error {
		if id == "" {
			return nil
		}
		if !access.ValidID(id) {
			return invalid("/rule", "公共指标和单位必须引用有效版本 UUID。")
		}
		if previous, ok := expected[id]; ok && previous != kind {
			return invalid("/rule", "同一版本不能兼作指标和单位。")
		}
		expected[id] = kind
		return nil
	}
	for _, in := range p.Rule.Inputs {
		if e := add(in.MetricVersionID, engineering.MetricDefinition); e != nil {
			return nil, e
		}
		if e := add(in.Quantity.BaseUnitVersionID, engineering.UnitDefinition); e != nil {
			return nil, e
		}
	}
	for _, param := range p.Rule.Parameters {
		if param.Quantity != nil {
			if e := add(param.Quantity.BaseUnitVersionID, engineering.UnitDefinition); e != nil {
				return nil, e
			}
		}
	}
	ids := make([]string, 0, len(expected))
	for id := range expected {
		ids = append(ids, id)
	}
	versions, e := engineering.ResolvePublicBatchTx(ctx, tx, ids, newUse)
	if e != nil {
		return nil, problem("dependency_unavailable", "所引用的公共定义版本不可用。", 422)
	}
	for id, kind := range expected {
		if versions[id].Kind != kind {
			return nil, invalid("/rule", "所引用公共版本的种类不匹配。")
		}
	}
	for _, in := range p.Rule.Inputs {
		if in.MetricVersionID != "" {
			metric := versions[in.MetricVersionID].Content.Metric
			if metric == nil || metric.Shape == "enum" {
				return nil, invalid("/rule/inputs", "派生输入必须是数值指标。")
			}
			found := false
			for _, unit := range metric.AllowedUnits {
				found = found || unit == in.Quantity.BaseUnitVersionID
			}
			if !found {
				return nil, invalid("/rule/inputs", "输入单位不在固定指标允许单位中。")
			}
		}
	}
	return versions, nil
}
func (s *Service) HasResourceReferences(ctx context.Context, tx pgx.Tx, id string) (bool, error) {
	var used bool
	e := tx.QueryRow(ctx, `WITH RECURSIVE refs(id) AS(SELECT $1::uuid UNION SELECT d.version_id FROM engineering_dependencies d JOIN refs r ON d.target_id=r.id) SELECT EXISTS(SELECT 1 FROM derived_metric_references WHERE engineering_version_id IN(SELECT id FROM refs))`, id).Scan(&used)
	return used, e
}
