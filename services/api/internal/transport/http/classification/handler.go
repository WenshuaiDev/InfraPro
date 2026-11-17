package classificationhttp

import (
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	c "github.com/WenshuaiDev/InfraPro/packages/go/platform/classification"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	h "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/devicecommon"
	ih "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/identity"
	"github.com/gin-gonic/gin"
)

func decode(ctx *gin.Context, v any) error {
	raw, e := io.ReadAll(http.MaxBytesReader(ctx.Writer, ctx.Request.Body, 8*1024*1024))
	if e != nil {
		return h.Invalid("分类请求最多 8 MiB。")
	}
	return c.Decode(raw, v)
}
func Register(r *gin.Engine, i *identity.Service, s *c.Service) {
	for _, p := range access.Policies() {
		parts := strings.Split(string(p.Operation), ":")
		if len(parts) != 4 || parts[0] != "Device" || parts[2] != "classification" {
			continue
		}
		entry, action := parts[1], parts[3]
		r.Handle(p.Method, p.Path, func(ctx *gin.Context) {
			a := i.AccessActor(ih.Principal(ctx))
			scope := c.PlatformScope()
			if entry == "tenant" {
				scope = c.TenantScope(ctx.Param("tenant_id"))
			} else if entry == "project" {
				scope = c.ProjectScope(ctx.Param("tenant_id"), ctx.Param("project_id"))
			}
			var result any
			var e error
			cctx := ctx.Request.Context()
			limit := 100
			for key, values := range ctx.Request.URL.Query() {
				if len(values) != 1 || ctx.Request.Method != "GET" || key != "q" && key != "cursor" && key != "limit" && key != "release_id" && key != "kind" {
					h.Reply(ctx, 0, nil, h.Invalid("查询参数无效。"))
					return
				}
			}
			if v := ctx.Query("limit"); v != "" {
				limit, e = strconv.Atoi(v)
				if e != nil {
					h.Reply(ctx, 0, nil, h.Invalid("分页参数无效。"))
					return
				}
			}
			switch action {
			case "dictionary":
				result, e = s.Dictionary(cctx, a)
			case "releases":
				result, e = s.Releases(cctx, a, scope, ctx.Query("cursor"), limit)
			case "categories":
				result, e = s.Categories(cctx, a, scope, ctx.Query("release_id"), ctx.Query("cursor"), ctx.Query("q"), limit)
			case "target":
				result, e = s.Target(cctx, a, scope, ctx.Param("kind"), ctx.Param("id"))
			case "targets":
				result, e = s.Targets(cctx, a, scope, ctx.Query("kind"), ctx.Query("cursor"), ctx.Query("q"), limit)
			case "history":
				result, e = s.History(cctx, a, scope, ctx.Param("kind"), ctx.Param("id"), ctx.Query("cursor"), limit)
			case "draft":
				var in c.SaveInput
				if e = decode(ctx, &in); e == nil {
					result, e = s.SaveDraft(cctx, a, in, h.Meta(ctx))
				}
			case "publish-preview", "publish":
				var in c.PublishInput
				if e = decode(ctx, &in); e == nil {
					if action == "publish-preview" {
						result, e = s.PreviewPublish(cctx, a, in)
					} else {
						result, e = s.Publish(cctx, a, in, h.Meta(ctx))
					}
				}
			case "preview", "apply":
				var in c.BatchInput
				if e = decode(ctx, &in); e == nil {
					if action == "preview" {
						result, e = s.PreviewBatch(cctx, a, scope, in)
					} else {
						result, e = s.ApplyBatch(cctx, a, scope, in, h.Meta(ctx))
					}
				}
			default:
				e = h.Invalid("不支持的分类操作。")
			}
			h.Reply(ctx, 200, result, e)
		})
	}
}
