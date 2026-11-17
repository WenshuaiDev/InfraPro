package resources

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dr "github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	"github.com/jackc/pgx/v5"
	"strconv"
)

func (s *Service) DisplayOptionsTx(ctx context.Context, tx pgx.Tx, a access.Actor, tenant, project string, members []dr.Member) ([]dr.ScopeOption, error) {
	values, e := s.DisplayDirectoryTx(ctx, tx, a, tenant, project, dr.Scope{StructureModel: 2, Domain: "resources", Members: members}, false)
	if e != nil {
		return nil, e
	}
	out := []dr.ScopeOption{}
	types := map[string]bool{}
	for _, v := range values {
		out = append(out, dr.ScopeOption{ID: v.ID, Kind: v.Kind, Code: v.Code, Name: v.Name, Visibility: v.Visibility})
		if v.Kind == "facility" && v.Type != nil && !types[v.Type.DefinitionID] {
			types[v.Type.DefinitionID] = true
			out = append(out, dr.ScopeOption{ID: v.Type.DefinitionID, Kind: "facility_type", Code: strconv.Itoa(len(types)), Name: v.Type.Name, Visibility: "full"})
		}
	}
	return out, nil
}
