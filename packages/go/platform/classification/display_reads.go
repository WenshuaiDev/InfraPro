package classification

import (
	"context"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dr "github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	"github.com/jackc/pgx/v5"
	"strconv"
	"strings"
)

// PublishedTx resolves immutable dictionary content for an already authorized
// owning-domain transaction. It never loads classification mappings or devices.
func PublishedTx(ctx context.Context, tx pgx.Tx, release string) (Release, error) {
	return releaseTx(ctx, tx, release)
}
func (s *Service) DisplayCategoriesTx(ctx context.Context, tx pgx.Tx, p dr.Plan) (dr.Materialized, error) {
	out := dr.Materialized{Rows: []json.RawMessage{}}
	o := p.Authority.Owner
	if _, e := access.RequireActorProjectTx(ctx, tx, p.Authority.Subject.ReadActor(), access.DeviceOperation("project", "classification", "categories"), o.Tenant, o.Project); e != nil {
		return out, e
	}
	q := p.Request.(*dr.CategoriesRequest)
	r, e := releaseTx(ctx, tx, q.ReleaseID)
	if e != nil {
		return out, e
	}
	for _, n := range r.Content.Nodes {
		if q.Search == "" || strings.Contains(strings.ToLower(n.Name+" "+n.Code), strings.ToLower(q.Search)) {
			out.Rows = append(out.Rows, dr.Bytes(n))
		}
	}
	out.Metadata = map[string]any{"dictionary_release_id": r.ID, "dictionary_revision": strconv.Itoa(r.Number), "sensor_root_id": r.Content.SensorRootID}
	return out, nil
}
