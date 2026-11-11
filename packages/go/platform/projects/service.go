package projects

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"strings"
)

type Service struct{ access *access.Service }

func New(a *access.Service) *Service { return &Service{access: a} }
func (s *Service) transact(ctx context.Context, a access.Actor, op access.Operation, tenant, project string, fn func(pgx.Tx, string, access.ProjectAuthorization) error) error {
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		p, ok := access.PolicyFor(op)
		if !ok || a.Check == nil {
			return access.ErrDenied
		}
		rev, e := access.Lock(ctx, tx, p.ChangesGraph || p.Method == "PATCH")
		if e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT id::text FROM accounts WHERE id::text=$1 OR ($3 AND id IN(SELECT account_id FROM tenant_memberships WHERE tenant_id::text=$2)) ORDER BY id FOR UPDATE`, a.ID, tenant, p.ChangesGraph)
		if e != nil {
			return e
		}
		for rows.Next() {
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if e = a.Check(ctx, tx, p.Recent); e != nil {
			return e
		}
		var auth access.ProjectAuthorization
		switch p.Class {
		case "project", "project_context":
			auth, e = access.RequireProjectTx(ctx, tx, a.ID, op, tenant, project)
		case "tenant":
			e = access.RequireScopedTx(ctx, tx, a.ID, op, tenant)
		case "permission":
			e = access.RequireTx(ctx, tx, a.ID, op)
		case "authenticated":
			if op != access.OpMyProjects {
				return access.ErrDenied
			}
		default:
			return access.ErrDenied
		}
		if e != nil {
			return e
		}
		if project != "" {
			if !access.ValidID(project) {
				return access.ErrProjectNotFound
			}
			var id string
			suffix := ""
			if p.Method != "GET" && !strings.HasSuffix(string(op), "Preview") {
				suffix = " FOR UPDATE"
			}
			e = tx.QueryRow(ctx, `SELECT id::text FROM projects WHERE tenant_id::text=$1 AND id=$2`+suffix, tenant, project).Scan(&id)
			if e == pgx.ErrNoRows {
				return access.ErrProjectNotFound
			}
			if e != nil {
				return e
			}
		}
		auth.Revision = rev
		return fn(tx, rev, auth)
	})
	s.access.LogDenial(op, a.ID, e)
	var pe *pgconn.PgError
	if errors.As(e, &pe) {
		switch pe.Code {
		case "23505":
			switch pe.ConstraintName {
			case "project_codes_pkey":
				return problem("project_code_taken", "该编号已被本租户其他项目使用。", 409)
			case "project_one_current_member":
				return problem("membership_exists", "已有当前成员关系；暂停关系请使用恢复操作。", 409)
			case "roles_project_name", "roles_project_preset_name":
				return problem("role_name_taken", "本项目或预置角色名称已被使用。", 409)
			}
			return access.ErrConflict
		case "55P03", "57014", "40P01":
			return problem("unavailable", "操作繁忙，请核对结果后重试。", 503)
		}
		return problem("internal_error", "操作未完成，请通过请求编号联系维护人员。", 500)
	}
	return e
}
func hash(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func ids(rows pgx.Rows) ([]string, error) {
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if e := rows.Scan(&id); e != nil {
			return nil, e
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
func newID(ctx context.Context, tx pgx.Tx) (string, error) {
	var id string
	e := tx.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&id)
	return id, e
}
func (s *Service) receipt(ctx context.Context, tx pgx.Tx, a access.Actor, tenant, project, target string, op access.Operation, m WriteMeta, payload any) (Result, bool, error) {
	var r Result
	var old string
	e := tx.QueryRow(ctx, `SELECT payload_hash,project_id::text,object_id::text,version::text FROM project_operation_receipts WHERE tenant_id=$1 AND actor_id=$2 AND operation=$3 AND client_request_id=$4`, tenant, a.ID, op, m.RequestID).Scan(&old, &r.ProjectID, &r.ObjectID, &r.Version)
	if e == pgx.ErrNoRows {
		return r, false, nil
	}
	if e != nil {
		return r, false, e
	}
	if old != hash([]any{project, payload}) {
		return r, false, problem("request_payload_conflict", "该请求编号已用于不同内容，请核对原操作。", 409)
	}
	r.Replayed = true
	e = tx.QueryRow(ctx, `SELECT p.version::text,s.revision::text FROM projects p CROSS JOIN authorization_state s WHERE p.id=$1 AND s.id=1`, r.ProjectID).Scan(&r.ProjectVersion, &r.Revision)
	return r, true, e
}
func complete(ctx context.Context, tx pgx.Tx, a access.Actor, tenant, project, target, ver string, op access.Operation, m WriteMeta, payload any, before, after any, meta access.Meta) (Result, error) {
	r := Result{ProjectID: project, ObjectID: target, Version: ver}
	if e := access.Bump(ctx, tx); e != nil {
		return r, e
	}
	meta.ActorID = a.ID
	meta.TenantID = tenant
	meta.ProjectID = project
	meta.ActorScope = "project"
	meta.Reason = m.Reason
	if e := access.Audit(ctx, tx, "project."+string(op), "project", target, "", before, after, meta); e != nil {
		return r, e
	}
	if op == access.OpProjectCreate || op == access.OpProjectTransfer || op == access.OpProjectRecoverAdmin {
		summary, e := projectTx(ctx, tx, tenant, project)
		if e != nil {
			return r, e
		}
		meta.ProjectID = ""
		meta.ActorScope = "tenant"
		if e = access.Audit(ctx, tx, "tenant.project.governance", "project", project, "", nil, summary.Summary, meta); e != nil {
			return r, e
		}
	}
	_, e := tx.Exec(ctx, `INSERT INTO project_operation_receipts(tenant_id,actor_id,operation,client_request_id,payload_hash,project_id,object_id,version) VALUES($1,$2,$3,$4,$5,$6,$7,$8::bigint)`, tenant, a.ID, op, m.RequestID, hash([]any{func() string {
		if op == access.OpProjectCreate {
			return ""
		}
		return project
	}(), payload}), project, target, ver)
	if e != nil {
		return r, e
	}
	e = tx.QueryRow(ctx, `SELECT p.version::text,s.revision::text FROM projects p CROSS JOIN authorization_state s WHERE p.id=$1 AND s.id=1`, project).Scan(&r.ProjectVersion, &r.Revision)
	return r, e
}
func active(p Project) error {
	if p.Status != "active" {
		return problem("project_archived", "项目已归档，仅允许查看、撤权和必要管理交接。", 409)
	}
	return nil
}
func candidateTx(ctx context.Context, tx pgx.Tx, tenant, member string) (Person, error) {
	var v Person
	if !access.ValidID(member) {
		return v, access.Validation("tenant_membership_id", "请选择有效租户成员。")
	}
	e := tx.QueryRow(ctx, `SELECT tm.id::text,a.id::text,a.username,a.display_name FROM tenant_memberships tm JOIN accounts a ON a.id=tm.account_id WHERE tm.tenant_id=$1 AND tm.id=$2 AND tm.status='active' AND a.status='active'`, tenant, member).Scan(&v.MembershipID, &v.AccountID, &v.Username, &v.DisplayName)
	v.Available = e == nil
	if e == pgx.ErrNoRows {
		return v, problem("candidate_unavailable", "候选成员已不可用，请重新选择。", 409)
	}
	return v, e
}
func bindTx(ctx context.Context, tx pgx.Tx, tenant, project, member, parent, account, actor, key string) error {
	if _, e := tx.Exec(ctx, `INSERT INTO project_memberships(id,tenant_id,project_id,tenant_membership_id,account_id,joined_by,status_changed_by) VALUES($1,$2,$3,$4,$5,$6,$6)`, member, tenant, project, parent, account, actor); e != nil {
		return e
	}
	return bindRoleTx(ctx, tx, tenant, project, member, actor, key)
}
func bindRoleTx(ctx context.Context, tx pgx.Tx, tenant, project, member, actor, key string) error {
	tag, e := tx.Exec(ctx, `INSERT INTO project_member_roles(tenant_id,project_id,membership_id,role_id,assigned_by) SELECT $1,$2,$3,id,$4 FROM roles WHERE builtin_key=$5 AND binding_scope='project' AND status='active' AND deleted_at IS NULL`, tenant, project, member, actor, key)
	if e == nil && tag.RowsAffected() != 1 {
		return access.ErrCatalog
	}
	return e
}
func reserve(ctx context.Context, tx pgx.Tx, tenant, project, code string) error {
	var owner string
	if _, e := tx.Exec(ctx, `INSERT INTO project_codes(tenant_id,code,project_id) VALUES($1,$2,$3) ON CONFLICT(tenant_id,code) DO NOTHING`, tenant, code, project); e != nil {
		return e
	}
	e := tx.QueryRow(ctx, `SELECT project_id::text FROM project_codes WHERE tenant_id=$1 AND code=$2`, tenant, code).Scan(&owner)
	if e == nil && owner != project {
		return problem("project_code_taken", "该编号已被本租户其他项目使用。", 409)
	}
	return e
}
func (s *Service) Create(ctx context.Context, a access.Actor, tenant string, in CreateInput, m access.Meta) (Result, error) {
	var out Result
	e := s.transact(ctx, a, access.OpProjectCreate, tenant, "", func(tx pgx.Tx, _ string, _ access.ProjectAuthorization) error {
		if e := in.Fields.normalize(); e != nil {
			return e
		}
		if e := in.WriteMeta.normalize(); e != nil {
			return e
		}
		in.Owner = strings.ToLower(in.Owner)
		in.Administrator = strings.ToLower(in.Administrator)
		var e error
		var replay bool
		out, replay, e = s.receipt(ctx, tx, a, tenant, "", "", access.OpProjectCreate, in.WriteMeta, in)
		if e != nil || replay {
			return e
		}
		if _, e = candidateTx(ctx, tx, tenant, in.Owner); e != nil {
			return e
		}
		admin, e := candidateTx(ctx, tx, tenant, in.Administrator)
		if e != nil {
			return e
		}
		pid, e := newID(ctx, tx)
		if e != nil {
			return e
		}
		mid, e := newID(ctx, tx)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO projects(id,tenant_id,name,code,purpose,description,owner_tenant_membership_id,administrator_membership_id,planned_start,planned_end,created_by,updated_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9,'')::date,NULLIF($10,'')::date,$11,$11)`, pid, tenant, in.Name, in.Code, in.Purpose, in.Description, in.Owner, mid, in.PlannedStart, in.PlannedEnd, a.ID)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO project_resource_policies(tenant_id,project_id) VALUES($1,$2)`, tenant, pid); e != nil {
			return e
		}
		if e = reserve(ctx, tx, tenant, pid, in.Code); e != nil {
			return e
		}
		if e = bindTx(ctx, tx, tenant, pid, mid, in.Administrator, admin.AccountID, a.ID, access.ProjectAdminKey); e != nil {
			return e
		}
		after, e := projectTx(ctx, tx, tenant, pid)
		if e != nil {
			return e
		}
		out, e = complete(ctx, tx, a, tenant, pid, pid, "1", access.OpProjectCreate, in.WriteMeta, in, nil, after, m)
		return e
	})
	return out, e
}
func (s *Service) Update(ctx context.Context, a access.Actor, tenant, project string, in UpdateInput, m access.Meta) (Result, error) {
	var out Result
	e := s.transact(ctx, a, access.OpProjectUpdate, tenant, project, func(tx pgx.Tx, _ string, _ access.ProjectAuthorization) error {
		if e := in.Fields.normalize(); e != nil {
			return e
		}
		if e := in.WriteMeta.normalize(); e != nil {
			return e
		}
		var e error
		var replay bool
		out, replay, e = s.receipt(ctx, tx, a, tenant, project, project, access.OpProjectUpdate, in.WriteMeta, in)
		if e != nil || replay {
			return e
		}
		old, e := projectTx(ctx, tx, tenant, project)
		if e != nil {
			return e
		}
		if e = active(old); e != nil {
			return e
		}
		if e = version(old.Version, in.Version); e != nil {
			return e
		}
		if e = version(old.Version, in.ProjectVersion); e != nil {
			return e
		}
		if e = reserve(ctx, tx, tenant, project, in.Code); e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE projects SET name=$2,code=$3,purpose=$4,description=$5,planned_start=NULLIF($6,'')::date,planned_end=NULLIF($7,'')::date,version=version+1,updated_at=clock_timestamp(),updated_by=$8 WHERE id=$1`, project, in.Name, in.Code, in.Purpose, in.Description, in.PlannedStart, in.PlannedEnd, a.ID)
		if e != nil {
			return e
		}
		after, e := projectTx(ctx, tx, tenant, project)
		if e != nil {
			return e
		}
		out, e = complete(ctx, tx, a, tenant, project, project, after.Version, access.OpProjectUpdate, in.WriteMeta, in, old, after, m)
		return e
	})
	return out, e
}
