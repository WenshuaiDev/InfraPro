package resources

import (
	"context"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	"github.com/jackc/pgx/v5"
)

// DisplayMembersTx materializes only full current project visibility. Ancestors
// used for navigation are projected separately and never count as members.
func (s *Service) DisplayMembersTx(ctx context.Context, tx pgx.Tx, a access.Actor, tenant, project string, selector displayread.Selector) ([]displayread.Member, error) {
	if _, e := access.RequireActorProjectTx(ctx, tx, a, access.ProjectResourceOperation("facility", "list"), tenant, project); e != nil {
		return nil, e
	}
	rows, e := tx.Query(ctx, `WITH visible AS MATERIALIZED (SELECT * FROM project_resource_visibility($1,$2,NULL) WHERE full_access)
 SELECT n.id::text,n.kind FROM visible v JOIN engineering_resource_nodes n ON n.tenant_id=$1 AND n.kind=v.kind AND n.id=v.id
 JOIN engineering_facilities f ON f.tenant_id=n.tenant_id AND f.id=n.facility_id JOIN engineering_versions t ON t.id=f.type_version_id
 WHERE n.deleted_at IS NULL AND f.deleted_at IS NULL
 AND (COALESCE(cardinality($3::uuid[]),0)=0 OR n.id=ANY($3)) AND (COALESCE(cardinality($4::uuid[]),0)=0 OR n.facility_id=ANY($4))
 AND (COALESCE(cardinality($5::uuid[]),0)=0 OR t.definition_id=ANY($5)) AND (COALESCE(cardinality($6::text[]),0)=0 OR n.kind=ANY($6))
 AND (COALESCE(cardinality($7::text[]),0)=0 OR n.status=ANY($7)) ORDER BY n.kind,n.id LIMIT 50001`, tenant, project, selector.IDs, selector.FacilityIDs, selector.TypeIDs, selector.Kinds, selector.Statuses)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []displayread.Member{}
	for rows.Next() {
		var m displayread.Member
		if e = rows.Scan(&m.ID, &m.Kind); e != nil {
			return nil, e
		}
		out = append(out, m)
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	return displayread.NormalizeMembers(out)
}
