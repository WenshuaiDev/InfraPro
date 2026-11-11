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
