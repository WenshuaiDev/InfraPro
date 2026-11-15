package access

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"
)

// The v33 -> v43 upgrade publishes the complete workspace capability bundle.
// Only an existing built-in administrator in a previously unused scope may
// receive its first publication. No resource grants or lifecycle states change.
// Partial publication, existing catalog permissions and subsequent expansions
// still follow the normal frozen-project guard.
func initialProjectWorkspacePublication(ctx context.Context, tx pgx.Tx, before ProjectAuthorization, added []Code, previousCatalog map[Code]Permission) (bool, error) {
	codes := []Code{
		ProjectSpatialView, ProjectSpatialSourceDownload, ProjectSpatialViewsManage,
		ProjectDisplayView, ProjectDisplaySettings, ProjectDisplayManage, ProjectDisplayUpload,
		ProjectDisplayConfigure, ProjectDisplayGrant, ProjectDisplayPreview, ProjectDisplayPublish, ProjectDisplayTerminals,
		ProjectMonitoringJobsRead, ProjectMonitoringJobsManage,
		ProjectAttachmentsRead, ProjectAttachmentsDownload, ProjectAttachmentsManage,
		ProjectAuditExport, ProjectAuditTechnical, ProjectAuditManage,
	}
	if len(added) != len(codes) || !before.Administrator || !slices.ContainsFunc(before.Roles, func(r Role) bool { return r.BuiltinKey == ProjectAdminKey && r.Status == "active" }) {
		return false, nil
	}
	for _, code := range codes {
		if _, exists := previousCatalog[code]; exists || !slices.Contains(added, code) {
			return false, nil
		}
	}
	var initial bool
	e := tx.QueryRow(ctx, `SELECT
 NOT EXISTS(SELECT 1 FROM project_spatial_grants WHERE tenant_id=$1 AND project_id=$2)
 AND NOT EXISTS(SELECT 1 FROM project_spatial_views WHERE tenant_id=$1 AND project_id=$2)
 AND NOT EXISTS(SELECT 1 FROM display_settings WHERE tenant_id=$1 AND project_id=$2 AND (enabled OR revision<>1))
 AND NOT EXISTS(SELECT 1 FROM display_instances WHERE tenant_id=$1 AND project_id=$2)
 AND NOT EXISTS(SELECT 1 FROM file_spaces WHERE tenant_id=$1 AND project_id=$2)
 AND NOT EXISTS(SELECT 1 FROM audit_exports WHERE tenant_id=$1 AND project_id=$2)`, before.TenantID, before.ProjectID).Scan(&initial)
	return initial, e
}
