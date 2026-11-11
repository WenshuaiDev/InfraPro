package resources

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	"github.com/jackc/pgx/v5"
)

type CatalogVersion struct {
	Version      engineering.Version   `json:"version"`
	Dependencies []engineering.Version `json:"dependencies"`
}

func (s *Service) CatalogList(ctx context.Context, a access.Actor, scope Scope, kind engineering.Kind, f engineering.Filter) (engineering.PublicVersionPage, error) {
	out := engineering.PublicVersionPage{}
	if scope.Entry != "project" {
		return out, access.ErrDenied
	}
	e := s.transact(ctx, a, access.ProjectResourceOperation(string(kind), "list"), scope, false, func(tx pgx.Tx, _ authorization) error {
		var e error
		out, e = engineering.PublicVersionsTx(ctx, tx, kind, f)
		return e
	})
	return out, e
}
func (s *Service) CatalogVersion(ctx context.Context, a access.Actor, scope Scope, kind engineering.Kind, id string) (CatalogVersion, error) {
	out := CatalogVersion{Dependencies: []engineering.Version{}}
	if scope.Entry != "project" {
		return out, access.ErrDenied
	}
	e := s.transact(ctx, a, access.ProjectResourceOperation(string(kind), "version"), scope, false, func(tx pgx.Tx, _ authorization) error {
		var e error
		out.Version, out.Dependencies, e = engineering.ResolvePublicTx(ctx, tx, id, kind, true)
		return e
	})
	return out, e
}
