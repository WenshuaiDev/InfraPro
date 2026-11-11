package resources

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

type UsageProject struct {
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
	Code      string `json:"code"`
	Status    string `json:"status"`
	LinkID    string `json:"link_id"`
}
type UsagePage struct {
	Items      []UsageProject `json:"items"`
	NextCursor string         `json:"next_cursor"`
}

func (s *Service) FacilityUsage(ctx context.Context, a access.Actor, tenant, facility string, f AccessFilter) (UsagePage, error) {
	out := UsagePage{Items: []UsageProject{}}
	e := s.transact(ctx, a, access.ResourceAccessOperation("usage", "detail"), TenantScope(tenant), false, func(tx pgx.Tx, auth authorization) error {
		if !access.ValidID(facility) {
			return notFound()
		}
		if f.Limit == 0 {
			f.Limit = 25
		}
		if f.Limit < 1 || f.Limit > 100 {
			return problem("invalid_request", "分页大小须为 1–100。", 400)
		}
		if _, e := lockFacility(ctx, tx, scoped(auth.Scope, Target{Kind: Facility, ID: facility}), false, false); e != nil {
			return e
		}
		p := AccessPolicy{TenantID: tenant}
		if e := tx.QueryRow(ctx, `SELECT revision::text FROM authorization_state WHERE id=1`).Scan(&p.Revision); e != nil {
			return e
		}
		id, e := accessPageCursor(auth, p, facility, "usage", f.Cursor)
		if e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT p.id::text,p.name,p.code,p.status,l.id::text FROM project_facility_links l JOIN projects p ON p.id=l.project_id WHERE l.tenant_id=$1 AND l.facility_id=$2 AND l.status='active' AND ($3='' OR l.id>NULLIF($3,'')::uuid) ORDER BY l.id LIMIT $4`, tenant, facility, id, f.Limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			v := UsageProject{}
			if e = rows.Scan(&v.ProjectID, &v.Name, &v.Code, &v.Status, &v.LinkID); e != nil {
				return e
			}
			out.Items = append(out.Items, v)
		}
		if e = rows.Err(); e != nil {
			return e
		}
		if len(out.Items) > f.Limit {
			out.Items = out.Items[:f.Limit]
			out.NextCursor = accessNextCursor(auth, p, facility, "usage", out.Items[f.Limit-1].LinkID)
		}
		return nil
	})
	return out, e
}
