package spatialhttp

import (
	"errors"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	sp "github.com/WenshuaiDev/InfraPro/packages/go/platform/spatial"
	identityhttp "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/identity"
	"github.com/gin-gonic/gin"
	"io"
	"mime"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

func invalid(message string) error {
	return &sp.Error{Code: "invalid_request", Message: message, Status: 400, Issues: []sp.Issue{}}
}
func reply(c *gin.Context, status int, out any, e error) {
	if e != nil {
		access.ReportOperationFailure(c.Request.Context(), e)
		var se *sp.Error
		var me *monitoring.Error
		var de *derivedmonitoring.Error
		if errors.As(e, &se) {
			c.AbortWithStatusJSON(se.Status, se)
		} else if errors.As(e, &me) {
			c.AbortWithStatusJSON(me.Status, me)
		} else if errors.As(e, &de) {
			c.AbortWithStatusJSON(de.Status, de)
		} else {
			identityhttp.RespondError(c, e)
		}
		return
	}
	c.JSON(status, out)
}
func body(c *gin.Context, out any) error {
	data, e := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 256*1024))
	if e != nil {
		return invalid("请求过大或未能完整读取。")
	}
	return sp.DecodeRequest(data, out)
}
func filter(c *gin.Context) (sp.Filter, error) {
	out := sp.Filter{OwnerID: c.Query("owner_id"), Query: c.Query("q"), Kind: c.Query("kind"), FacilityID: c.Query("facility_id"), State: c.Query("state"), PageID: c.Query("page_id"), ElementKey: c.Query("element_key"), Cursor: c.Query("cursor")}
	for key, values := range c.Request.URL.Query() {
		if len(values) != 1 || !slices.Contains([]string{"q", "kind", "owner_id", "facility_id", "state", "page_id", "element_key", "cursor", "limit", "bbox"}, key) {
			return out, invalid("不支持或重复的查询参数。")
		}
	}
	var e error
	if c.Query("limit") != "" {
		out.Limit, e = strconv.Atoi(c.Query("limit"))
		if e != nil {
			return out, invalid("分页大小无效。")
		}
	}
	out.BBox, e = sp.ParseBBox(c.Query("bbox"))
	return out, e
}
func Register(r *gin.Engine, identityService *identity.Service, s *sp.Service) {
	for _, policy := range access.Policies() {
		parts := strings.SplitN(string(policy.Operation), ":", 3)
		if len(parts) != 3 || parts[0] != "Spatial" {
			continue
		}
		entry, action := parts[1], parts[2]
		r.Handle(policy.Method, policy.Path, func(c *gin.Context) {
			for _, p := range c.Params {
				if p.Key == "file_id" || p.Key == "part" || p.Key == "tile" {
					continue
				}
				if !access.ValidID(p.Value) {
					reply(c, 0, nil, &sp.Error{Code: "spatial_not_found", Message: "对象不存在或不可访问。", Status: 404})
					return
				}
			}
			f, e := filter(c)
			if e != nil {
				reply(c, 0, nil, e)
				return
			}
			ctx := c.Request.Context()
			actor := identityService.AccessActor(identityhttp.Principal(c))
			scope := sp.Tenant(c.Param("tenant_id"))
			if entry == "project" {
				scope = sp.Project(scope.Tenant, c.Param("project_id"))
			}
			qualification, err := s.Qualification(ctx, actor, scope, action)
			if err != nil {
				reply(c, 0, nil, err)
				return
			}
			c.Header("X-Spatial-Authorization-Revision", qualification)
			id := c.Param("id")
			var out any
			status := 200
			switch action {
			case "assets.list":
				out, e = s.Assets(ctx, actor, scope, f)
			case "assets.detail":
				out, e = s.Asset(ctx, actor, scope, id)
			case "assets.create", "assets.update":
				var in sp.AssetInput
				if e = body(c, &in); e == nil {
					out, e = s.SaveAsset(ctx, actor, scope, id, in)
				}
			case "assets.versions":
				out, e = s.Versions(ctx, actor, scope, id, f)
			case "assets.history":
				out, e = s.History(ctx, actor, scope, id, f)
			case "versions.create":
				var in sp.VersionInput
				if e = body(c, &in); e == nil {
					out, e = s.CreateVersion(ctx, actor, scope, id, in)
				}
			case "versions.draft":
				out, e = s.Draft(ctx, actor, scope, id)
			case "versions.edit":
				var in sp.DraftInput
				if e = body(c, &in); e == nil {
					out, e = s.EditDraft(ctx, actor, scope, id, in)
				}
			case "versions.delete":
				var in sp.RevisionInput
				if e = body(c, &in); e == nil {
					out, e = s.DeleteDraft(ctx, actor, scope, id, in)
				}
			case "versions.validate":
				var in sp.RevisionInput
				if e = body(c, &in); e == nil {
					out, e = s.Validate(ctx, actor, scope, id, in)
				}
			case "versions.publish-preview", "versions.withdraw-preview", "assets.current-preview", "assets.retire-preview":
				var in sp.ControlInput
				if e = body(c, &in); e == nil {
					out, e = s.Preview(ctx, actor, scope, id, strings.TrimSuffix(action, "-preview"), in)
				}
			case "versions.publish", "versions.withdraw", "assets.current", "assets.retire", "assets.restore":
				var in sp.ControlInput
				if e = body(c, &in); e == nil {
					out, e = s.Control(ctx, actor, scope, id, action, in)
				}
			case "uploads.create":
				var in sp.UploadInput
				if e = body(c, &in); e == nil {
					out, e = s.StartUpload(ctx, actor, scope, in)
				}
			case "uploads.detail":
				out, e = s.Upload(ctx, actor, scope, id)
			case "uploads.part":
				part, err := strconv.Atoi(c.Param("part"))
				if err != nil {
					e = invalid("分片序号无效。")
				} else {
					_ = http.NewResponseController(c.Writer).SetReadDeadline(time.Now().Add(2 * time.Minute))
					_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(3 * time.Minute))
					out, e = s.UploadPart(ctx, actor, scope, id, part, c.GetHeader("X-Content-SHA256"), http.MaxBytesReader(c.Writer, c.Request.Body, sp.UploadPartSize+1))
				}
			case "uploads.complete", "uploads.cancel":
				var in sp.RevisionInput
				if e = body(c, &in); e == nil {
					_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(5 * time.Minute))
					if action == "uploads.complete" {
						out, e = s.CompleteUpload(ctx, actor, scope, id, in)
					} else {
						out, e = s.CancelUpload(ctx, actor, scope, id, in)
					}
				}
			case "versions.process":
				var in sp.RevisionInput
				if e = body(c, &in); e == nil {
					out, e = s.Process(ctx, actor, scope, id, in)
					status = 202
				}
			case "jobs.detail":
				out, e = s.Job(ctx, actor, scope, id)
			case "jobs.cancel", "jobs.retry":
				var in sp.RevisionInput
				if e = body(c, &in); e == nil {
					out, e = s.ControlJob(ctx, actor, scope, id, action, in)
				}
			case "versions.manifest":
				out, e = s.Manifest(ctx, actor, scope, id)
			case "versions.pages":
				out, e = s.Pages(ctx, actor, scope, id, f)
			case "versions.elements", "versions.properties":
				out, e = s.Elements(ctx, actor, scope, id, f)
			case "versions.placements":
				out, e = s.Placements(ctx, actor, scope, id, f)
			case "versions.component-links":
				out, e = s.ComponentLinks(ctx, actor, scope, id, f)
			case "versions.features":
				out, e = s.Features(ctx, actor, scope, id, f)
			case "versions.files", "versions.source":
				download(c, s, actor, scope, id, c.Param("file_id"), action == "versions.source")
				return
			case "objects.detail":
				out, e = s.Object(ctx, actor, scope, sp.ObjectRef{Kind: f.Kind, ID: id})
			case "objects":
				out, e = s.Objects(ctx, actor, scope, f)
			case "basemaps.tile":
				data, err := sp.LocalTile(c.Param("tile"))
				if err != nil {
					reply(c, 0, nil, err)
				} else {
					c.Header("Cache-Control", "private, no-store")
					c.Data(200, "image/png", data)
				}
				return
			case "basemaps":
				out, e = s.Basemaps(ctx, actor, scope)
			case "summaries", "locate":
				var in sp.ObjectQuery
				if e = body(c, &in); e == nil {
					if action == "summaries" {
						out, e = s.Summaries(ctx, actor, scope, in)
					} else {
						out, e = s.Locate(ctx, actor, scope, in)
					}
				}
			case "related-artifacts":
				var in sp.RelatedInput
				if e = body(c, &in); e == nil {
					out, e = s.RelatedArtifacts(ctx, actor, scope, in)
				}
			case "access.detail":
				out, e = s.Access(ctx, actor, scope, c.Param("project_id"))
			case "access.preview", "access.save":
				var in sp.AccessInput
				if e = body(c, &in); e == nil {
					if action == "access.preview" {
						out, e = s.PreviewAccess(ctx, actor, scope, c.Param("project_id"), in)
					} else {
						out, e = s.SaveAccess(ctx, actor, scope, c.Param("project_id"), in)
					}
				}
			case "views.detail":
				out, e = s.View(ctx, actor, scope, id)
			case "views.save":
				var in sp.ViewInput
				if e = body(c, &in); e == nil {
					out, e = s.SaveView(ctx, actor, scope, id, in)
				}
			default:
				e = invalid("未实现的空间操作。")
			}
			if e == nil && access.SpatialReadOperation(policy.Operation) {
				var current string
				current, e = s.Qualification(ctx, actor, scope, action)
				if e == nil && current != qualification {
					e = &sp.Error{Code: "authorization_changed", Message: "授权已变化，请重新读取。", Status: 409}
				}
			}
			reply(c, status, out, e)
		})
	}
}
func parseRange(raw string, size int64) (start, length int64, partial bool, err error) {
	if raw == "" {
		return 0, size, false, nil
	}
	if !strings.HasPrefix(raw, "bytes=") || strings.Contains(raw, ",") {
		return 0, 0, false, invalid("只支持单一字节范围。")
	}
	parts := strings.Split(strings.TrimPrefix(raw, "bytes="), "-")
	if len(parts) != 2 {
		return 0, 0, false, invalid("字节范围无效。")
	}
	if parts[0] == "" {
		suffix, e := strconv.ParseInt(parts[1], 10, 64)
		if e != nil || suffix <= 0 {
			return 0, 0, false, invalid("字节范围无效。")
		}
		if suffix > size {
			suffix = size
		}
		return size - suffix, suffix, true, nil
	}
	start, e := strconv.ParseInt(parts[0], 10, 64)
	if e != nil || start < 0 || start >= size {
		return 0, 0, false, invalid("字节范围超出文件。")
	}
	end := size - 1
	if parts[1] != "" {
		end, e = strconv.ParseInt(parts[1], 10, 64)
		if e != nil || end < start {
			return 0, 0, false, invalid("字节范围无效。")
		}
		if end >= size {
			end = size - 1
		}
	}
	return start, end - start + 1, true, nil
}
func download(c *gin.Context, s *sp.Service, a access.Actor, scope sp.Scope, version, id string, source bool) {
	_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(5 * time.Minute))
	file, e := s.OpenFile(c.Request.Context(), a, scope, version, id, source)
	if e != nil {
		reply(c, 0, nil, e)
		return
	}
	defer file.Close()
	start, length, partial, e := parseRange(c.GetHeader("Range"), file.Size)
	if e != nil {
		c.Header("Content-Range", fmt.Sprintf("bytes */%d", file.Size))
		c.AbortWithStatus(http.StatusRequestedRangeNotSatisfiable)
		return
	}
	c.Header("Accept-Ranges", "bytes")
	c.Header("Content-Type", file.MediaType)
	c.Header("Content-Length", strconv.FormatInt(length, 10))
	c.Header("ETag", `"`+file.SHA256+`"`)
	c.Header("Cache-Control", "private, no-store")
	disposition := "inline"
	if source {
		disposition = "attachment"
	}
	c.Header("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": file.Name}))
	if partial {
		c.Header("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, start+length-1, file.Size))
		c.Status(http.StatusPartialContent)
	}
	e = file.StreamWrite(c.Request.Context(), start, length, func(data []byte) (int, error) {
		_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(2 * time.Second))
		return c.Writer.Write(data)
	})
	if e != nil {
		if !c.Writer.Written() {
			c.Header("Content-Length", "")
			reply(c, 0, nil, e)
		} else {
			c.Abort()
		}
	}
}
