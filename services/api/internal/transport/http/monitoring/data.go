package monitoringhttp

import (
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	identityhttp "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/identity"
	"github.com/gin-gonic/gin"
)

func bind[T any, R any](c *gin.Context, status int, call func(T) (R, error)) {
	var in T
	if e := body(c, &in); e != nil {
		reply(c, 0, nil, e)
		return
	}
	out, e := call(in)
	reply(c, status, out, e)
}
func registerData(r *gin.Engine, i *identity.Service, s *md.Service) {
	implemented := map[string]bool{}
	for _, name := range []string{"exports.create", "exports.files", "exports.download", "points.list", "channels.list", "overview", "latest", "statistics", "trends", "imports.list", "imports.detail", "imports.create", "upload.part", "upload.complete", "imports.mapping", "imports.validate", "imports.publish", "imports.issues", "imports.resolve", "imports.cancel", "imports.retry", "imports.readopt", "batches.detail", "batches.preview", "batches.revoke", "sources.download", "jobs.list", "jobs.detail", "jobs.cancel", "selections.create", "details", "snapshots.renew"} {
		implemented[name] = true
	}
	for _, policy := range access.Policies() {
		parts := strings.Split(string(policy.Operation), ":")
		if len(parts) != 3 || parts[0] != "Monitoring" || !implemented[parts[2]] {
			continue
		}
		action := parts[2]
		r.Handle(policy.Method, policy.Path, func(c *gin.Context) {
			c.Request = c.Request.WithContext(md.TraceContext(c.Request.Context(), ""))
			c.Header("X-Monitoring-Trace-ID", md.TraceID(c.Request.Context()))
			defer func() {
				md.LogOperation(c.Request.Context(), action, c.Writer.Status(), int64(c.Writer.Size()), c.Request.Context().Err() != nil)
			}()
			for _, p := range c.Params {
				if p.Key == "part" {
					continue
				}
				if !access.ValidID(p.Value) {
					reply(c, 0, nil, &md.Error{Code: "monitoring_not_found", Message: "监测数据不存在或当前不可访问。", Status: 404})
					return
				}
			}
			paged := action == "imports.list" || action == "imports.issues" || action == "jobs.list" || action == "points.list" || action == "channels.list"
			for key, values := range c.Request.URL.Query() {
				if len(values) != 1 || !(paged && key == "cursor" || action == "points.list" && (key == "search" || key == "facility_id" || key == "point_id") || action == "channels.list" && key == "channel_id") {
					reply(c, 0, nil, invalid("不支持或重复的查询参数。"))
					return
				}
			}
			a := i.AccessActor(identityhttp.Principal(c))
			scope := md.TenantScope(c.Param("tenant_id"))
			if parts[1] == "project" {
				scope = md.ProjectScope(scope.Tenant, c.Param("project_id"))
			}
			ctx, id := c.Request.Context(), c.Param("id")
			revision, revisionError := s.AuthorizationRevision(ctx, a, scope)
			if revisionError != nil {
				reply(c, 0, nil, revisionError)
				return
			}
			c.Header("X-Monitoring-Authorization-Revision", revision)
			switch action {
			case "points.list":
				v, e := s.Points(ctx, a, scope, md.NavigationQuery{Cursor: c.Query("cursor"), Search: c.Query("search"), Facility: c.Query("facility_id"), Point: c.Query("point_id")})
				reply(c, 200, v, e)
			case "channels.list":
				v, e := s.Channels(ctx, a, scope, id, c.Query("cursor"), c.Query("channel_id"))
				reply(c, 200, v, e)
			case "overview", "latest", "statistics", "trends":
				var in md.QueryInput
				if e := body(c, &in); e != nil {
					reply(c, 0, nil, e)
					return
				}
				v, e := s.Analytics(ctx, a, scope, action, in)
				status := 200
				if v.State != "ready" {
					status = 202
				}
				reply(c, status, v, e)
			case "imports.list":
				v, e := s.Imports(ctx, a, scope, c.Query("cursor"))
				reply(c, 200, v, e)
			case "imports.detail", "batches.detail":
				v, e := s.Import(ctx, a, scope, id)
				reply(c, 200, v, e)
			case "imports.create":
				bind(c, 201, func(in md.ImportInput) (md.ImportBatch, error) { return s.CreateImport(ctx, a, scope, in) })
			case "upload.part":
				part, e := strconv.Atoi(c.Param("part"))
				if e != nil || strconv.Itoa(part) != c.Param("part") {
					reply(c, 0, nil, invalid("分片序号无效。"))
					return
				}
				controller := http.NewResponseController(c.Writer)
				_ = controller.SetReadDeadline(time.Now().Add(2 * time.Minute))
				_ = controller.SetWriteDeadline(time.Now().Add(3 * time.Minute))
				v, e := s.UploadPart(ctx, a, scope, id, c.GetHeader("X-Expected-Revision"), part, c.GetHeader("X-Content-SHA256"), http.MaxBytesReader(c.Writer, c.Request.Body, md.UploadPartSize+1))
				reply(c, 200, v, e)
			case "upload.complete":
				_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(30 * time.Minute))
				bind(c, 200, func(in md.UploadComplete) (md.ImportBatch, error) { return s.CompleteUpload(ctx, a, scope, id, in) })
			case "imports.mapping":
				bind(c, 200, func(in md.MappingInput) (md.ImportBatch, error) { return s.SetMapping(ctx, a, scope, id, in) })
			case "imports.validate", "imports.publish", "imports.retry":
				kind := strings.TrimPrefix(action, "imports.")
				if kind == "retry" {
					kind = "validate"
				}
				bind(c, 202, func(in md.WriteMeta) (md.ImportBatch, error) { return s.QueueImport(ctx, a, scope, id, kind, in) })
			case "imports.issues":
				v, e := s.Issues(ctx, a, scope, id, c.Query("cursor"))
				reply(c, 200, v, e)
			case "imports.resolve":
				bind(c, 200, func(in md.ResolveInput) (md.ImportBatch, error) { return s.ResolveImport(ctx, a, scope, id, in) })
			case "imports.cancel":
				bind(c, 200, func(in md.WriteMeta) (md.ImportBatch, error) { return s.CancelImport(ctx, a, scope, id, in) })
			case "imports.readopt":
				bind(c, 201, func(in md.ReadoptInput) (md.ImportBatch, error) { return s.Readopt(ctx, a, scope, id, in) })
			case "batches.preview":
				bind(c, 200, func(in struct {
					Revision string `json:"expected_revision"`
				}) (md.BatchRevokePreview, error) {
					return s.PreviewRevokeBatch(ctx, a, scope, id, in.Revision)
				})
			case "batches.revoke":
				bind(c, 200, func(in md.RevokeInput) (md.BatchRevokeResult, error) { return s.RevokeBatch(ctx, a, scope, id, in) })
			case "sources.download", "exports.download":
				var file md.Download
				var e error
				if action == "sources.download" {
					file, e = s.Source(ctx, a, scope, id)
				} else {
					file, e = s.ExportDownload(ctx, a, scope, id, c.Param("part"))
				}
				if e != nil {
					reply(c, 0, nil, e)
					return
				}
				_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(30 * time.Minute))
				c.Header("Content-Type", file.ContentType)
				c.Header("Content-Length", strconv.FormatInt(file.Size, 10))
				c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": file.Name}))
				c.Header("Cache-Control", "private, no-store")
				c.Header("X-Content-Type-Options", "nosniff")
				c.Header("X-Content-SHA256", file.SHA256)
				if e = file.Write(ctx, c.Writer); e != nil {
					if !c.Writer.Written() {
						c.Header("Content-Length", "")
						reply(c, 0, nil, e)
					} else {
						c.Abort()
					}
					return
				}
			case "exports.create":
				bind(c, 202, func(in md.ExportInput) (md.ExportRecord, error) { return s.CreateExport(ctx, a, scope, in) })
			case "exports.files":
				v, e := s.ExportFiles(ctx, a, scope, id)
				reply(c, 200, v, e)
			case "jobs.list":
				v, e := s.Jobs(ctx, a, scope, c.Query("cursor"))
				reply(c, 200, v, e)
			case "jobs.detail":
				v, e := s.Job(ctx, a, scope, id)
				reply(c, 200, v, e)
			case "jobs.cancel":
				bind(c, 200, func(in md.WriteMeta) (md.Job, error) { return s.CancelJob(ctx, a, scope, id, in) })
			case "selections.create":
				bind(c, 201, func(in md.SelectionInput) (md.Selection, error) { return s.Select(ctx, a, scope, in) })
			case "details":
				bind(c, 200, func(in md.QueryInput) (md.DetailPage, error) { return s.Details(ctx, a, scope, in) })
			case "snapshots.renew":
				bind(c, 200, func(in md.QueryInput) (md.Snapshot, error) { return s.RenewSnapshot(ctx, a, scope, id, in) })
			}
		})

	}
}
