package access

import "strings"

const (
	TenantAttachmentsRead      Code = "tenant.attachments.read"
	TenantAttachmentsDownload  Code = "tenant.attachments.download"
	TenantAttachmentsManage    Code = "tenant.attachments.manage"
	ProjectAttachmentsRead     Code = "project.attachments.read"
	ProjectAttachmentsDownload Code = "project.attachments.download"
	ProjectAttachmentsManage   Code = "project.attachments.manage"
)

func filesCodes(scope string) []Code {
	return []Code{Code(scope + ".attachments.read"), Code(scope + ".attachments.download"), Code(scope + ".attachments.manage")}
}
func filesCatalog() []Permission {
	out := []Permission{}
	for _, scope := range []string{"tenant", "project"} {
		for i, c := range filesCodes(scope) {
			risk := "high"
			if i == 0 {
				risk = "normal"
			}
			out = append(out, Permission{Code: c, Name: []string{"查看普通附件", "下载普通附件与预览", "管理普通附件"}[i], Group: "文件与附件", Scope: scope, Description: "查看还需本空间资料查看权；下载另需附件下载权；管理另需资料修改权，不包含下载。", Risk: risk, Lifecycle: "active", TenantCustomRoleAssignable: scope == "tenant", ProjectCustomRoleAssignable: scope == "project"})
		}
	}
	return out
}
func FilesOperation(scope, action string) Operation {
	return Operation("Files:" + scope + ":" + action)
}

type filesRoute struct{ action, method, path, capability string }

var filesRoutes = []filesRoute{
	{"capabilities", "GET", "/capabilities", "read"},
	{"attachments.list", "GET", "/attachments", "read"}, {"attachments.detail", "GET", "/attachments/:id", "read"},
	{"attachments.revisions", "GET", "/attachments/:id/revisions", "read"}, {"attachments.update", "PATCH", "/attachments/:id", "manage"},
	{"attachments.revert", "POST", "/attachments/:id/revert", "manage"}, {"attachments.trash", "POST", "/attachments/:id/trash", "manage"}, {"attachments.restore", "POST", "/attachments/:id/restore", "manage"},
	{"content", "GET", "/attachments/:id/revisions/:revision_id/content", "download"}, {"content.head", "HEAD", "/attachments/:id/revisions/:revision_id/content", "download"},
	{"preview", "GET", "/attachments/:id/revisions/:revision_id/preview", "download"}, {"preview.head", "HEAD", "/attachments/:id/revisions/:revision_id/preview", "download"},
	{"uploads.list", "GET", "/uploads", "read"}, {"uploads.detail", "GET", "/uploads/:id", "read"}, {"uploads.create", "POST", "/uploads", "manage"},
	{"uploads.part", "PUT", "/uploads/:id/parts/:part", "manage"}, {"uploads.complete", "POST", "/uploads/:id/complete", "manage"}, {"uploads.cancel", "POST", "/uploads/:id/cancel", "manage"}, {"uploads.reconfirm", "POST", "/uploads/:id/reconfirm", "manage"},
	{"jobs.detail", "GET", "/jobs/:id", "read"}, {"requests.detail", "GET", "/requests/:request_id", "read"},
}

func filesPolicies() []Policy {
	out := []Policy{}
	for _, scope := range []string{"tenant", "project"} {
		base := "/api/v1/tenants/:tenant_id"
		if scope == "project" {
			base += "/projects/:project_id"
		}
		for _, r := range filesRoutes {
			codes := []Code{Code(scope + ".profile.read"), Code(scope + ".attachments.read")}
			if r.capability != "read" {
				codes = append(codes, Code(scope+".attachments."+r.capability))
			}
			if r.capability == "manage" {
				codes = append(codes, Code(scope+".profile.update"))
			}
			out = append(out, Policy{FilesOperation(scope, r.action), r.method, base + "/files" + r.path, scope, codes, false, false})
		}
	}
	return out
}
func FilesReadOperation(op Operation) bool {
	for _, r := range filesRoutes {
		if strings.HasSuffix(string(op), ":"+r.action) {
			return r.capability != "manage"
		}
	}
	return false
}
