package projects

import (
	"context"
	"errors"
	"slices"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

// Null counts mean the viewer cannot read that domain, never zero resources.
type GovernanceStatistics struct {
	Facilities *int `json:"facilities"`
	Points     *int `json:"points"`
	Devices    *int `json:"devices"`
	Members    *int `json:"members"`
}

type GovernanceOwner struct {
	MembershipID string `json:"tenant_membership_id"`
	DisplayName  string `json:"display_name"`
	Username     string `json:"username"`
}

type GovernanceOverview struct {
	Total         int               `json:"total"`
	Active        int               `json:"active"`
	Archived      int               `json:"archived"`
	Attention     int               `json:"attention"`
	FilteredTotal int               `json:"filtered_total"`
	Owners        []GovernanceOwner `json:"owners"`
}

type ProjectPage struct {
	access.Page[Context]
	Overview *GovernanceOverview `json:"overview,omitempty"`
}

// The overview ignores the page cursor; only filtered_total follows the filters.
func governanceOverviewTx(ctx context.Context, tx pgx.Tx, tenant string, f Filter) (*GovernanceOverview, error) {
	out := &GovernanceOverview{Owners: []GovernanceOwner{}}
	err := tx.QueryRow(ctx, `WITH scoped AS (
 SELECT p.*,COALESCE(am.status='active' AND tm.status='active' AND a.status='active',false) AS administrator_available
 FROM projects p
 LEFT JOIN project_memberships am ON am.id=p.administrator_membership_id
 LEFT JOIN tenant_memberships tm ON tm.id=am.tenant_membership_id
 LEFT JOIN accounts a ON a.id=am.account_id
 WHERE p.tenant_id=$1
 )
 SELECT count(*),count(*) FILTER(WHERE status='active'),count(*) FILTER(WHERE status='archived'),
 count(*) FILTER(WHERE NOT administrator_available),
 count(*) FILTER(WHERE ($2='' OR name ILIKE '%'||$2||'%' OR code ILIKE '%'||$2||'%')
 AND ($3 IN ('','all') OR status=$3)
 AND ($4='' OR owner_tenant_membership_id::text=$4)
 AND ($5='' OR administrator_available=($5='false')))
 FROM scoped`, tenant, f.Query, f.Status, f.Owner, f.Unavailable).Scan(&out.Total, &out.Active, &out.Archived, &out.Attention, &out.FilteredTotal)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT DISTINCT tm.id::text,a.display_name,a.username
 FROM projects p JOIN tenant_memberships tm ON tm.id=p.owner_tenant_membership_id
 JOIN accounts a ON a.id=tm.account_id WHERE p.tenant_id=$1
 ORDER BY a.display_name,a.username,tm.id::text`, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var owner GovernanceOwner
		if err = rows.Scan(&owner.MembershipID, &owner.DisplayName, &owner.Username); err != nil {
			return nil, err
		}
		out.Owners = append(out.Owners, owner)
	}
	return out, rows.Err()
}

// Aggregate the bounded current page in one query. Explicit resource grants
// match the authorization workspace; context ancestors and outputs are excluded.
func governanceStatisticsTx(ctx context.Context, tx pgx.Tx, actor, tenant, revision string, ids []string) (map[string]*GovernanceStatistics, error) {
	out := make(map[string]*GovernanceStatistics, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	_, tenantAuth, err := access.TenantSnapshotTx(ctx, tx, actor, tenant, revision)
	if err != nil {
		return nil, err
	}
	canResources := slices.Contains(tenantAuth.Permissions, access.TenantEngineeringResourceRead) && slices.Contains(tenantAuth.Permissions, access.TenantEngineeringResourceAuthorizationRead)
	canDevices := slices.Contains(tenantAuth.Permissions, access.TenantDeviceRead) && slices.Contains(tenantAuth.Permissions, access.TenantDeviceAuthorizationRead)
	rows, err := tx.Query(ctx, `WITH resource_counts AS (
 SELECT g.project_id,count(*) FILTER(WHERE g.resource_kind='facility') AS facilities,
 count(*) FILTER(WHERE g.resource_kind='point') AS points
 FROM project_resource_grants g
 JOIN project_facility_links l ON l.id=g.link_id AND l.tenant_id=g.tenant_id AND l.status='active'
 JOIN engineering_resource_nodes n ON n.tenant_id=g.tenant_id AND n.kind=g.resource_kind
 AND n.id=COALESCE(g.component_id,g.point_id,g.channel_id,g.monitoring_section_id,g.borehole_id,g.facility_id) AND n.deleted_at IS NULL
 WHERE $3 AND g.tenant_id=$1 AND g.project_id=ANY($2::uuid[]) AND g.ended_at IS NULL
 GROUP BY g.project_id
 ), device_counts AS (
 SELECT g.project_id,count(*) AS devices FROM project_device_grants g
 JOIN devices d ON d.tenant_id=g.tenant_id AND d.id=g.device_id AND d.deleted_at IS NULL
 WHERE $4 AND g.tenant_id=$1 AND g.project_id=ANY($2::uuid[]) AND g.kind='device' AND g.ended_at IS NULL
 GROUP BY g.project_id
 ), member_counts AS (
 SELECT project_id,count(*) AS members FROM project_memberships
 WHERE tenant_id=$1 AND project_id=ANY($2::uuid[]) AND status<>'ended' GROUP BY project_id
 )
 SELECT p.id::text,
 CASE WHEN $3 THEN COALESCE(r.facilities,0) END,
 CASE WHEN $3 THEN COALESCE(r.points,0) END,
 CASE WHEN $4 THEN COALESCE(d.devices,0) END,
 COALESCE(m.members,0)
 FROM projects p LEFT JOIN resource_counts r ON r.project_id=p.id
 LEFT JOIN device_counts d ON d.project_id=p.id LEFT JOIN member_counts m ON m.project_id=p.id
 WHERE p.tenant_id=$1 AND p.id=ANY($2::uuid[])`, tenant, ids, canResources, canDevices)
	if err != nil {
		return nil, err
	}
	memberCounts := map[string]int{}
	for rows.Next() {
		var id string
		var count int
		v := &GovernanceStatistics{}
		if err = rows.Scan(&id, &v.Facilities, &v.Points, &v.Devices, &count); err != nil {
			rows.Close()
			return nil, err
		}
		out[id], memberCounts[id] = v, count
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	// Use the owning authorization implementation, including role lifecycle and
	// membership state, instead of inventing a tenant-admin bypass for members.
	for id, v := range out {
		auth, err := access.ProjectSnapshotTx(ctx, tx, actor, tenant, id, revision)
		if errors.Is(err, access.ErrProjectNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if slices.Contains(auth.Permissions, access.ProjectMemberRead) {
			count := memberCounts[id]
			v.Members = &count
		}
	}
	return out, nil
}
