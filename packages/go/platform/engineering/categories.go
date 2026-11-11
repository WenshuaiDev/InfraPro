package engineering

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

type CategoryInput struct {
	WriteMeta
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Revision    int    `json:"revision"`
}

func (s *Service) Categories(ctx context.Context, a access.Actor, tenant string) ([]Category, error) {
	out := []Category{}
	op := access.Operation("EngineeringCategories:list")
	if tenant != "" {
		op = "EngineeringCategories:catalog"
	}
	e := s.transact(ctx, a, op, tenant, false, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `SELECT key,name,description,revision,EXISTS(SELECT 1 FROM engineering_versions v WHERE v.unit_category->>'key'=c.key) OR EXISTS(SELECT 1 FROM engineering_drafts d WHERE d.content->'unit'->>'category'=c.key OR d.content->'metric'->>'category'=c.key) FROM engineering_unit_categories c ORDER BY key`)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			v := Category{}
			if e = rows.Scan(&v.Key, &v.Name, &v.Description, &v.Revision, &v.Used); e != nil {
				return e
			}
			out = append(out, v)
		}
		return rows.Err()
	})
	return out, e
}
func (s *Service) SaveCategory(ctx context.Context, a access.Actor, in CategoryInput, meta access.Meta) (Result, error) {
	out := Result{}
	e := s.transact(ctx, a, "EngineeringCategories:save", "", true, func(tx pgx.Tx) error {
		var e error
		var replay bool
		out, replay, e = receipt(ctx, tx, a, "public", "category-save", in.WriteMeta, in)
		if e != nil || replay {
			return e
		}
		if e = checkCode(in.Key); e != nil {
			return e
		}
		issues := []Issue{}
		bounded(&issues, in.Name, "/name", 100, true)
		bounded(&issues, in.Description, "/description", 2000, true)
		if len(issues) > 0 {
			return invalid(issues)
		}
		if in.Revision == 0 {
			_, e = tx.Exec(ctx, `INSERT INTO engineering_unit_categories(key,name,description) VALUES($1,$2,$3)`, in.Key, in.Name, in.Description)
		} else {
			tag, err := tx.Exec(ctx, `UPDATE engineering_unit_categories SET name=$2,description=$3,revision=revision+1 WHERE key=$1 AND revision=$4`, in.Key, in.Name, in.Description, in.Revision)
			e = err
			if e == nil && tag.RowsAffected() != 1 {
				return access.ErrConflict
			}
		}
		if e != nil {
			return e
		}
		out = Result{DefinitionID: in.Key, Revision: in.Revision + 1, Issues: []Issue{}}
		out, e = complete(ctx, tx, a, Scope{}, "category-save", in.WriteMeta, in, out, nil, map[string]any{"key": in.Key, "name": in.Name, "description": in.Description}, meta)
		return e
	})
	return out, e
}
func (s *Service) DeleteCategory(ctx context.Context, a access.Actor, k string, in RevisionInput, meta access.Meta) (Result, error) {
	out := Result{}
	e := s.transact(ctx, a, "EngineeringCategories:delete", "", true, func(tx pgx.Tx) error {
		var e error
		var replay bool
		out, replay, e = receipt(ctx, tx, a, "public", "category-delete", in.WriteMeta, []any{k, in})
		if e != nil || replay {
			return e
		}
		tag, e := tx.Exec(ctx, `DELETE FROM engineering_unit_categories WHERE key=$1 AND revision=$2`, k, in.Revision)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return access.ErrConflict
		}
		out = Result{DefinitionID: k, Revision: in.Revision, Issues: []Issue{}}
		out, e = complete(ctx, tx, a, Scope{}, "category-delete", in.WriteMeta, []any{k, in}, out, map[string]string{"key": k}, nil, meta)
		return e
	})
	return out, e
}
