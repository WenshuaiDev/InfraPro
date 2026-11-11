package engineering

import (
	"context"
	"github.com/jackc/pgx/v5"
)

// ResolvePublicTx is for trusted business services only. Callers authorize the
// existing resource or new adoption, and hold the catalog lock before resource locks.
func ResolvePublicTx(ctx context.Context, tx pgx.Tx, id string, kind Kind, newUse bool) (Version, []Version, error) {
	r := resolve(ctx, tx)
	v, e := r.public(id, kind)
	if e != nil {
		return v, nil, e
	}
	if v.SchemaVersion != MetamodelVersion {
		return v, nil, problem("definition_unavailable", "不支持该定义格式。", 409)
	}
	if newUse && !v.Usable {
		return v, nil, problem("definition_unavailable", "所选定义或其固定依赖已不可新采用。", 409)
	}
	deps, e := r.dependencies(v.Content)
	return v, deps, e
}

// UsageReader only reveals existence, including retained historical references.
type UsageReader interface {
	HasResourceReferences(context.Context, pgx.Tx, string) (bool, error)
}

func (s *Service) WithUsageReader(r UsageReader) *Service { s.usage = r; return s }

// PublicVersionPage contains adoptable published versions only. Definitions used
// by existing resources are resolved through ResolvePublicTx(newUse=false).
type PublicVersionSummary struct {
	ID           string `json:"id"`
	DefinitionID string `json:"definition_id"`
	Code         string `json:"code"`
	Name         string `json:"name"`
	Number       int    `json:"number"`
}
type PublicVersionPage struct {
	Items  []PublicVersionSummary `json:"items"`
	Total  int                    `json:"total"`
	Offset int                    `json:"offset"`
	Limit  int                    `json:"limit"`
}

func PublicVersionsTx(ctx context.Context, tx pgx.Tx, kind Kind, f Filter) (PublicVersionPage, error) {
	out := PublicVersionPage{Items: []PublicVersionSummary{}, Offset: f.Offset, Limit: f.Limit}
	if out.Limit == 0 {
		out.Limit = 25
	}
	if kind != TypeDefinition && kind != MetricDefinition && kind != UnitDefinition || out.Limit < 1 || out.Limit > 100 || f.Offset < 0 || f.Offset > 100000 || len([]rune(f.Query)) > 200 {
		return out, problem("invalid_request", "公共目录筛选无效。", 400)
	}
	where := ` FROM engineering_versions v JOIN engineering_definitions d ON d.id=v.definition_id WHERE d.kind=$1 AND d.tenant_id IS NULL AND d.deleted_at IS NULL AND NOT EXISTS(SELECT 1 FROM blocked b WHERE b.id=v.id) AND ($2='' OR v.content->>'name' ILIKE '%'||$2||'%' OR d.code ILIKE '%'||$2||'%')`
	if e := tx.QueryRow(ctx, blockedCTE+` SELECT count(*)`+where, kind, f.Query).Scan(&out.Total); e != nil {
		return out, e
	}
	rows, e := tx.Query(ctx, blockedCTE+` SELECT v.id::text,d.id::text,d.code,v.content->>'name',v.number`+where+` ORDER BY d.code,v.number DESC,v.id LIMIT $3 OFFSET $4`, kind, f.Query, out.Limit, out.Offset)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		v := PublicVersionSummary{}
		if e = rows.Scan(&v.ID, &v.DefinitionID, &v.Code, &v.Name, &v.Number); e != nil {
			return out, e
		}
		out.Items = append(out.Items, v)
	}
	return out, rows.Err()
}
