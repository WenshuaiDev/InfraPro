package resources

import (
	"context"
	"encoding/json"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	"github.com/jackc/pgx/v5"
)

// DisplayMapItemsTx projects only each facility's own engineering position.
// Coordinate conversion and geographic filtering are owned by spatial.
func (s *Service) DisplayMapItemsTx(ctx context.Context, tx pgx.Tx, p displayread.Plan) ([]displayread.FacilityMapItem, error) {
	q := p.Request.(*displayread.MapFacilitiesRequest)
	o := p.Authority.Owner
	items, e := s.DisplayDirectoryTx(ctx, tx, p.Authority.Subject.ReadActor(), o.Tenant, o.Project, p.Scopes["target"], false)
	if e != nil {
		return nil, e
	}
	ids := []string{}
	out := []displayread.FacilityMapItem{}
	index := map[string]int{}
	for _, v := range items {
		if v.Kind == "facility" && matchesDisplayResource(v, q.Filters, q.Search) {
			ids = append(ids, v.ID)
			index[v.ID] = len(out)
			out = append(out, displayread.FacilityMapItem{Facility: v, Type: v.Type})
		}
	}
	rows, e := tx.Query(ctx, `SELECT id::text,position FROM engineering_facilities WHERE tenant_id=$1 AND id=ANY($2::uuid[]) AND deleted_at IS NULL`, o.Tenant, ids)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var raw []byte
		if e = rows.Scan(&id, &raw); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(raw, &out[index[id]].Position); e != nil {
			return nil, e
		}
	}
	return out, rows.Err()
}
