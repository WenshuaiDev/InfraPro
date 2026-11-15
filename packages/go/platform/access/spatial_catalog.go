package access

import "strings"

const (
	TenantSpatialRead                Code = "tenant.spatial.read"
	TenantSpatialManage              Code = "tenant.spatial.manage"
	TenantSpatialPlacementManage     Code = "tenant.spatial.placement.manage"
	TenantSpatialPublish             Code = "tenant.spatial.publish"
	TenantSpatialSourceDownload      Code = "tenant.spatial.source.download"
	TenantProjectSpatialAccessRead   Code = "tenant.project.spatial_access.read"
	TenantProjectSpatialAccessManage Code = "tenant.project.spatial_access.manage"
	ProjectSpatialView               Code = "project.spatial.view"
	ProjectSpatialSourceDownload     Code = "project.spatial.source.download"
	ProjectSpatialViewsManage        Code = "project.spatial.views.manage"
)

func spatialCodes(scope string) []Code {
	if scope == "tenant" {
		return []Code{TenantSpatialRead, TenantSpatialManage, TenantSpatialPlacementManage, TenantSpatialPublish, TenantSpatialSourceDownload, TenantProjectSpatialAccessRead, TenantProjectSpatialAccessManage}
	}
	return []Code{ProjectSpatialView, ProjectSpatialSourceDownload, ProjectSpatialViewsManage}
}
func spatialCatalog() []Permission {
	out := []Permission{}
	names := map[Code]string{TenantSpatialRead: "查看空间资料", TenantSpatialManage: "管理空间资料与处理", TenantSpatialPlacementManage: "编辑空间挂载与关联", TenantSpatialPublish: "发布与撤回空间版本", TenantSpatialSourceDownload: "下载空间原件", TenantProjectSpatialAccessRead: "查看项目空间资料授权", TenantProjectSpatialAccessManage: "管理项目空间资料授权", ProjectSpatialView: "浏览项目空间", ProjectSpatialSourceDownload: "下载获准空间原件", ProjectSpatialViewsManage: "保存项目空间视图"}
	for _, scope := range []string{"tenant", "project"} {
		for _, c := range spatialCodes(scope) {
			risk := "high"
			if strings.HasSuffix(string(c), "read") || c == ProjectSpatialView {
				risk = "normal"
			}
			out = append(out, Permission{Code: c, Name: names[c], Group: "工程空间", Scope: scope, Description: "资料版本、业务资源、来源原件和监测数据分别授权。", Risk: risk, Lifecycle: "active", TenantCustomRoleAssignable: scope == "tenant", ProjectCustomRoleAssignable: scope == "project"})
		}
	}
	return out
}
func SpatialOperation(scope, action string) Operation {
	return Operation("Spatial:" + scope + ":" + action)
}

type spatialRoute struct {
	action, method, path, permission string
	tenantOnly, projectOnly, read    bool
}

var spatialRoutes = []spatialRoute{
	{"assets.list", "GET", "/assets", "", false, false, true},
	{"assets.detail", "GET", "/assets/:id", "", false, false, true},
	{"assets.create", "POST", "/assets", "manage", true, false, false},
	{"assets.update", "PATCH", "/assets/:id", "manage", true, false, false},
	{"assets.versions", "GET", "/assets/:id/versions", "", false, false, true},
	{"versions.create", "POST", "/assets/:id/versions", "manage", true, false, false},
	{"versions.draft", "GET", "/versions/:id/draft", "", true, false, true},
	{"versions.edit", "PATCH", "/versions/:id/draft", "placement.manage", true, false, false},
	{"versions.delete", "DELETE", "/versions/:id/draft", "manage", true, false, false},
	{"versions.validate", "POST", "/versions/:id/validate", "publish", true, false, true},
	{"versions.publish-preview", "POST", "/versions/:id/publish-preview", "publish", true, false, true},
	{"versions.publish", "POST", "/versions/:id/publish", "publish", true, false, false},
	{"versions.withdraw-preview", "POST", "/versions/:id/withdraw-preview", "publish", true, false, true},
	{"versions.withdraw", "POST", "/versions/:id/withdraw", "publish", true, false, false},
	{"assets.current-preview", "POST", "/assets/:id/current-preview", "publish", true, false, true},
	{"assets.current", "POST", "/assets/:id/current", "publish", true, false, false},
	{"assets.retire-preview", "POST", "/assets/:id/retire-preview", "manage", true, false, true},
	{"assets.retire", "POST", "/assets/:id/retire", "manage", true, false, false},
	{"assets.restore", "POST", "/assets/:id/restore", "manage", true, false, false},
	{"assets.history", "GET", "/assets/:id/history", "", true, false, true},
	{"uploads.detail", "GET", "/uploads/:id", "", true, false, true},
	{"uploads.create", "POST", "/uploads", "manage", true, false, false},
	{"uploads.part", "PUT", "/uploads/:id/parts/:part", "manage", true, false, false},
	{"uploads.complete", "POST", "/uploads/:id/complete", "manage", true, false, false},
	{"uploads.cancel", "POST", "/uploads/:id/cancel", "manage", true, false, false},
	{"versions.process", "POST", "/versions/:id/process", "manage", true, false, false},
	{"jobs.detail", "GET", "/jobs/:id", "", true, false, true},
	{"jobs.cancel", "POST", "/jobs/:id/cancel", "manage", true, false, false},
	{"jobs.retry", "POST", "/jobs/:id/retry", "manage", true, false, false},
	{"versions.manifest", "GET", "/versions/:id/manifest", "", false, false, true},
	{"versions.pages", "GET", "/versions/:id/pages", "", false, false, true},
	{"versions.elements", "GET", "/versions/:id/elements", "", false, false, true},
	{"versions.properties", "GET", "/versions/:id/properties", "", false, false, true},
	{"versions.placements", "GET", "/versions/:id/placements", "", false, false, true},
	{"versions.component-links", "GET", "/versions/:id/component-links", "", false, false, true},
	{"versions.features", "GET", "/versions/:id/features", "", false, false, true},
	{"versions.files", "GET", "/versions/:id/files/:file_id", "", false, false, true},
	{"versions.source", "GET", "/versions/:id/source", "source.download", false, false, true},
	{"objects", "GET", "/objects", "", false, false, true},
	{"objects.detail", "GET", "/objects/:id", "", false, false, true},
	{"basemaps", "GET", "/basemaps", "", false, false, true},
	{"basemaps.tile", "GET", "/basemaps/local/tiles/:tile", "", false, false, true},
	{"summaries", "POST", "/summaries", "", false, false, true},
	{"locate", "POST", "/locate", "", false, false, true},
	{"related-artifacts", "POST", "/related-artifacts", "", false, false, true},
	{"views.detail", "GET", "/views/:id", "", false, true, true},
	{"views.save", "PUT", "/views/:id", "views.manage", false, true, false},
}

func spatialPolicies() []Policy {
	out := []Policy{}
	for _, scope := range []string{"tenant", "project"} {
		base := "/api/v1/tenants/:tenant_id"
		read := TenantSpatialRead
		if scope == "project" {
			base += "/projects/:project_id"
			read = ProjectSpatialView
		}
		for _, r := range spatialRoutes {
			if scope == "tenant" && r.projectOnly || scope == "project" && r.tenantOnly {
				continue
			}
			codes := []Code{read}
			if r.permission != "" {
				codes = append(codes, Code(scope+".spatial."+r.permission))
			}
			out = append(out, Policy{SpatialOperation(scope, r.action), r.method, base + "/spatial" + r.path, scope, codes, false, false})
		}
	}
	for _, r := range []struct {
		action, method, path string
		manage               bool
	}{{"access.detail", "GET", "", false}, {"access.preview", "POST", "/preview", true}, {"access.save", "PUT", "", true}} {
		codes := []Code{TenantProjectRead, TenantProjectSpatialAccessRead}
		if r.manage {
			codes = append(codes, TenantProjectSpatialAccessManage)
		}
		out = append(out, Policy{SpatialOperation("tenant", r.action), r.method, "/api/v1/tenants/:tenant_id/projects/:project_id/spatial-access" + r.path, "tenant", codes, false, false})
	}
	return out
}
func SpatialReadOperation(op Operation) bool {
	for _, r := range spatialRoutes {
		if op == SpatialOperation("tenant", r.action) || op == SpatialOperation("project", r.action) {
			return r.read
		}
	}
	return false
}
