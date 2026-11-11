package engineering

import (
	"context"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

func (s *Service) Create(ctx context.Context, a access.Actor, scope Scope, in CreateInput, meta access.Meta) (Result, error) {
	out := Result{}
	e := s.transact(ctx, a, scope.operation("create"), scope.Tenant, true, func(tx pgx.Tx) error {
		in.Content = canonicalContent(in.Content)
		var e error
		var replay bool
		out, replay, e = receipt(ctx, tx, a, scope.key(), "create", in.WriteMeta, []any{scope.Kind, in})
		if e != nil || replay {
			return e
		}
		if e = checkCode(in.Code); e != nil {
			return e
		}
		if e = requireReferenceReads(ctx, tx, a, scope, in.Content, nil); e != nil {
			return e
		}
		issues, e := checkedDraft(resolve(ctx, tx), scope.Kind, in.Content, nil)
		if e != nil {
			return e
		}
		out, e = createTx(ctx, tx, a, scope, in.Code, in.Content, "", issues)
		if e != nil {
			return e
		}
		d, e := definitionTx(ctx, tx, scope, out.DefinitionID, false)
		if e != nil {
			return e
		}
		out, e = complete(ctx, tx, a, scope, "create", in.WriteMeta, []any{scope.Kind, in}, out, nil, auditSummary(d, "", nil), meta)
		return e
	})
	return out, e
}
func createTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, code string, c Content, source string, issues []Issue) (Result, error) {
	out := Result{Revision: 1, Issues: issues}
	var sourceSnapshot *Source
	if source != "" {
		v, e := resolve(ctx, tx).version(source)
		if e != nil {
			return out, e
		}
		sourceSnapshot = &Source{VersionID: v.ID, DefinitionID: v.DefinitionID, Code: v.Code, Name: v.Content.Name, Number: v.Number}
	}
	e := tx.QueryRow(ctx, `INSERT INTO engineering_definitions(kind,tenant_id,code,source_version,created_by,source_snapshot) VALUES($1,NULLIF($2,'')::uuid,$3,NULLIF($4,'')::uuid,$5,$6) RETURNING id::text`, scope.Kind, scope.owner(), code, source, a.ID, sourceSnapshot).Scan(&out.DefinitionID)
	if e != nil {
		return out, e
	}
	_, e = tx.Exec(ctx, `INSERT INTO engineering_drafts(definition_id,revision,content,updated_by) VALUES($1,1,$2,$3)`, out.DefinitionID, c, a.ID)
	return out, e
}
func (s *Service) Save(ctx context.Context, a access.Actor, scope Scope, id string, in SaveInput, meta access.Meta) (Result, error) {
	out := Result{}
	e := s.transact(ctx, a, scope.operation("save"), scope.Tenant, false, func(tx pgx.Tx) error {
		in.Content = canonicalContent(in.Content)
		d, e := definitionTx(ctx, tx, scope, id, true)
		if e != nil {
			return e
		}
		var replay bool
		out, replay, e = receipt(ctx, tx, a, scope.key(), "save", in.WriteMeta, []any{id, in})
		if e != nil || replay {
			return e
		}
		if e = revision(d, in.Revision); e != nil {
			return e
		}
		if d.Draft == nil {
			return missingDraft()
		}
		if e = requireReferenceReads(ctx, tx, a, scope, in.Content, &d.Draft.Content); e != nil {
			return e
		}
		issues, e := checkedDraft(resolve(ctx, tx), scope.Kind, in.Content, &d.Draft.Content)
		if e != nil {
			return e
		}
		changes := Differences(d.Draft.Content, in.Content)
		out = Result{DefinitionID: id, Revision: d.Revision, Issues: issues}
		if len(changes) > 0 {
			out.Revision++
			if _, e = tx.Exec(ctx, `UPDATE engineering_definitions SET revision=$2 WHERE id=$1`, id, out.Revision); e != nil {
				return e
			}
			if _, e = tx.Exec(ctx, `UPDATE engineering_drafts SET revision=$2,content=$3,updated_at=clock_timestamp(),updated_by=$4 WHERE definition_id=$1`, id, out.Revision, in.Content, a.ID); e != nil {
				return e
			}
		}
		if len(changes) == 0 {
			_, e = tx.Exec(ctx, `INSERT INTO engineering_operation_receipts(scope_key,actor_id,request_id,operation,payload_hash,result) VALUES($1,$2,$3,'save',$4,$5)`, scope.key(), a.ID, in.RequestID, hash([]any{id, in}), out)
			return e
		}
		d.Draft.Content = in.Content
		out, e = complete(ctx, tx, a, scope, "save", in.WriteMeta, []any{id, in}, out, nil, auditSummary(d, "", changes), meta)
		return e
	})
	return out, e
}
func (s *Service) NewDraft(ctx context.Context, a access.Actor, scope Scope, id string, in NewDraftInput, meta access.Meta) (Result, error) {
	out := Result{}
	e := s.transact(ctx, a, scope.operation("new-draft"), scope.Tenant, false, func(tx pgx.Tx) error {
		d, e := definitionTx(ctx, tx, scope, id, true)
		if e != nil {
			return e
		}
		var replay bool
		out, replay, e = receipt(ctx, tx, a, scope.key(), "new-draft", in.WriteMeta, []any{id, in})
		if e != nil || replay {
			return e
		}
		if e = revision(d, in.Revision); e != nil {
			return e
		}
		if d.Draft != nil {
			return problem("draft_exists", "该定义已有当前草稿。", 409)
		}
		v, e := resolve(ctx, tx).version(in.VersionID)
		if e != nil {
			return e
		}
		if v.DefinitionID != id {
			return access.ErrNotFound
		}
		out = Result{DefinitionID: id, Revision: d.Revision + 1, Issues: []Issue{}}
		if _, e = tx.Exec(ctx, `UPDATE engineering_definitions SET revision=$2 WHERE id=$1`, id, out.Revision); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO engineering_drafts(definition_id,revision,base_version,content,updated_by) VALUES($1,$2,$3,$4,$5)`, id, out.Revision, v.ID, v.Content, a.ID); e != nil {
			return e
		}
		out, e = complete(ctx, tx, a, scope, "new-draft", in.WriteMeta, []any{id, in}, out, nil, auditSummary(d, v.ID, nil), meta)
		return e
	})
	return out, e
}
func (s *Service) DeleteDraft(ctx context.Context, a access.Actor, scope Scope, id string, in RevisionInput, meta access.Meta) (Result, error) {
	out := Result{}
	e := s.transact(ctx, a, scope.operation("delete-draft"), scope.Tenant, false, func(tx pgx.Tx) error {
		// Deleted identities retain their kind and owner; receipts cannot cross resource permissions.
		var owned bool
		if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM engineering_definitions WHERE id::text=$1 AND kind=$2 AND scope_key=$3)`, id, scope.Kind, scope.key()).Scan(&owned); e != nil {
			return e
		}
		if !owned {
			return access.ErrNotFound
		}
		var e error
		var replay bool
		out, replay, e = receipt(ctx, tx, a, scope.key(), "delete-draft", in.WriteMeta, []any{id, in})
		if e != nil || replay {
			return e
		}
		d, e := definitionTx(ctx, tx, scope, id, true)
		if e != nil {
			return e
		}
		if e = revision(d, in.Revision); e != nil {
			return e
		}
		if d.Draft == nil {
			return missingDraft()
		}
		if _, e = tx.Exec(ctx, `DELETE FROM engineering_drafts WHERE definition_id=$1`, id); e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE engineering_definitions SET revision=revision+1,deleted_at=CASE WHEN EXISTS(SELECT 1 FROM engineering_versions WHERE definition_id=$1) THEN NULL ELSE clock_timestamp() END WHERE id=$1`, id)
		if e != nil {
			return e
		}
		out = Result{DefinitionID: id, Revision: d.Revision + 1, Issues: []Issue{}}
		out, e = complete(ctx, tx, a, scope, "delete-draft", in.WriteMeta, []any{id, in}, out, auditSummary(d, "", nil), nil, meta)
		return e
	})
	return out, e
}
func (s *Service) Copy(ctx context.Context, a access.Actor, scope Scope, in CopyInput, meta access.Meta) (Result, error) {
	out := Result{}
	e := s.transact(ctx, a, scope.operation("copy"), scope.Tenant, true, func(tx pgx.Tx) error {
		var e error
		var replay bool
		out, replay, e = receipt(ctx, tx, a, scope.key(), "copy", in.WriteMeta, []any{scope.Kind, in})
		if e != nil || replay {
			return e
		}
		if e = checkCode(in.Code); e != nil {
			return e
		}
		r := resolve(ctx, tx)
		v, e := r.version(in.SourceVersion)
		if e != nil {
			return e
		}
		var owner string
		if e = tx.QueryRow(ctx, `SELECT COALESCE(tenant_id::text,'') FROM engineering_definitions WHERE id=$1`, v.DefinitionID).Scan(&owner); e != nil {
			return e
		}
		if v.Kind != scope.Kind || (owner != "" && owner != scope.owner()) {
			return access.ErrNotFound
		}
		if scope.Tenant != "" && owner == "" {
			if e = access.RequireScopedTx(ctx, tx, a.ID, access.EngineeringOperation(false, true, string(scope.Kind), "detail"), scope.Tenant); e != nil {
				return e
			}
		}
		if !v.Usable {
			return &Error{Code: "dependency_unavailable", Message: "来源或其依赖已不可新使用。", Status: 409, Issues: v.Issues}
		}
		if e = requireReferenceReads(ctx, tx, a, scope, v.Content, nil); e != nil {
			return e
		}
		issues, e := checkedDraft(r, scope.Kind, v.Content, nil)
		if e != nil {
			return e
		}
		out, e = createTx(ctx, tx, a, scope, in.Code, v.Content, v.ID, issues)
		if e != nil {
			return e
		}
		d, e := definitionTx(ctx, tx, scope, out.DefinitionID, false)
		if e != nil {
			return e
		}
		out, e = complete(ctx, tx, a, scope, "copy", in.WriteMeta, []any{scope.Kind, in}, out, nil, auditSummary(d, "", nil), meta)
		return e
	})
	return out, e
}
func (s *Service) Publish(ctx context.Context, a access.Actor, scope Scope, id string, in RevisionInput, meta access.Meta) (Result, error) {
	out := Result{}
	e := s.transact(ctx, a, scope.operation("publish"), scope.Tenant, true, func(tx pgx.Tx) error {
		d, e := definitionTx(ctx, tx, scope, id, true)
		if e != nil {
			return e
		}
		var replay bool
		out, replay, e = receipt(ctx, tx, a, scope.key(), "publish", in.WriteMeta, []any{id, in})
		if e != nil || replay {
			return e
		}
		if e = revision(d, in.Revision); e != nil {
			return e
		}
		if d.Draft == nil {
			return missingDraft()
		}
		c := d.Draft.Content
		if e = requireReferenceReads(ctx, tx, a, scope, c, nil); e != nil {
			return e
		}
		r := resolve(ctx, tx)
		issues := ValidateContent(scope.Kind, c)
		refs, e := r.checkReferences(c, true, nil)
		if e != nil {
			return e
		}
		issues = append(issues, refs...)
		rows, e := tx.Query(ctx, `SELECT content FROM engineering_versions WHERE definition_id=$1 ORDER BY number`, id)
		if e != nil {
			return e
		}
		previous := []Content{}
		for rows.Next() {
			v := Content{}
			if e = rows.Scan(&v); e != nil {
				rows.Close()
				return e
			}
			previous = append(previous, v)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		for _, old := range previous {
			issues = append(issues, r.stable(old, c)...)
		}
		if e = diagnosticError(issues); e != nil {
			return e
		}
		schemas := map[string]any{}
		if scope.Kind == TypeDefinition {
			schemas = Schemas(c)
			for _, doc := range schemas {
				if _, e = compile(doc.(map[string]any)); e != nil {
					return invalid([]Issue{issue("schema_invalid", "属性规则无法编译。", "/fields", "")})
				}
			}
		}
		var category *Category
		if c.Unit != nil {
			v, e := r.category(c.Unit.Category)
			if e != nil {
				return e
			}
			category = &v
		}
		deps, e := r.dependencies(c)
		if e != nil {
			return e
		}
		digests := map[string]string{}
		for _, v := range deps {
			digests[v.ID] = v.Digest
		}
		digest := hash([]any{c, schemas, category, digests})
		out = Result{DefinitionID: id, Revision: d.Revision + 1, Issues: []Issue{}}
		e = tx.QueryRow(ctx, `INSERT INTO engineering_versions(definition_id,number,content,digest,schema_version,schemas,unit_category,published_by) SELECT $1,COALESCE(MAX(number),0)+1,$2,$3,'1',$4,$5,$6 FROM engineering_versions WHERE definition_id=$1 RETURNING id::text`, id, c, digest, schemas, category, a.ID).Scan(&out.VersionID)
		if e != nil {
			return e
		}
		for _, ref := range references(c) {
			if _, e = tx.Exec(ctx, `INSERT INTO engineering_dependencies(version_id,target_id,path,kind) VALUES($1,$2,$3,$4)`, out.VersionID, ref.ID, ref.Path, ref.Kind); e != nil {
				return e
			}
		}
		if _, e = tx.Exec(ctx, `INSERT INTO engineering_version_states(version_id,updated_by,reason) VALUES($1,$2,$3)`, out.VersionID, a.ID, in.Reason); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `DELETE FROM engineering_drafts WHERE definition_id=$1`, id); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE engineering_definitions SET revision=$2 WHERE id=$1`, id, out.Revision); e != nil {
			return e
		}
		out, e = complete(ctx, tx, a, scope, "publish", in.WriteMeta, []any{id, in}, out, nil, auditSummary(d, out.VersionID, nil), meta)
		return e
	})
	return out, e
}
func (s *Service) SetState(ctx context.Context, a access.Actor, scope Scope, id, version string, in StateInput, meta access.Meta) (Result, error) {
	out := Result{}
	e := s.transact(ctx, a, scope.operation("state"), scope.Tenant, true, func(tx pgx.Tx) error {
		d, e := definitionTx(ctx, tx, scope, id, false)
		if e != nil {
			return e
		}
		var replay bool
		out, replay, e = receipt(ctx, tx, a, scope.key(), "state", in.WriteMeta, []any{id, version, in})
		if e != nil || replay {
			return e
		}
		r := resolve(ctx, tx)
		v, e := r.version(version)
		if e != nil {
			return e
		}
		if v.DefinitionID != id {
			return access.ErrNotFound
		}
		if in.Revision != v.StateRevision {
			return access.ErrConflict
		}
		if in.Enabled {
			for _, id := range v.Dependencies {
				dep, e := r.version(id)
				if e != nil {
					return e
				}
				if !dep.Usable {
					return &Error{Code: "dependency_unavailable", Message: "依赖仍不可用，暂不能恢复该版本。", Status: 409, Issues: dep.Issues}
				}
			}
		}
		if v.Enabled == in.Enabled {
			return problem("state_unchanged", "版本状态已是目标状态，请重新读取。", 409)
		}
		if _, e = tx.Exec(ctx, `UPDATE engineering_version_states SET enabled=$2,revision=revision+1,reason=$3,updated_by=$4,updated_at=clock_timestamp() WHERE version_id=$1`, version, in.Enabled, in.Reason, a.ID); e != nil {
			return e
		}
		out = Result{DefinitionID: id, VersionID: version, Revision: v.StateRevision + 1, Issues: []Issue{}}
		summary := auditSummary(d, version, nil)
		summary["name"] = v.Content.Name
		summary["number"] = v.Number
		out, e = complete(ctx, tx, a, scope, "state", in.WriteMeta, []any{id, version, in}, out, map[string]any{"enabled": v.Enabled}, map[string]any{"definition": summary, "enabled": in.Enabled}, meta)
		return e
	})
	return out, e
}
