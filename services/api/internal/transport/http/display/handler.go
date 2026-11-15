package displayhttp

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/display"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/spatial"
	identityhttp "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/identity"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"strconv"
	"strings"
)

func PublicError(e error) *display.Error {
	var de *display.Error
	var ae *access.Error
	var me *monitoring.Error
	var re *resources.Error
	var derived *derivedmonitoring.Error
	var sp *spatial.Error
	if errors.As(e, &de) {
		return de
	}
	if errors.As(e, &ae) {
		return &display.Error{Code: ae.Code, Message: ae.Message, Status: ae.Status}
	}
	if errors.As(e, &me) {
		return &display.Error{Code: me.Code, Message: me.Message, Status: me.Status}
	}
	if errors.As(e, &re) {
		return &display.Error{Code: re.Code, Message: re.Message, Status: re.Status}
	}
	if errors.As(e, &derived) {
		return &display.Error{Code: derived.Code, Message: derived.Message, Status: derived.Status}
	}
	if errors.As(e, &sp) {
		return &display.Error{Code: sp.Code, Message: sp.Message, Status: sp.Status}
	}
	if errors.Is(e, context.DeadlineExceeded) {
		return &display.Error{Code: "budget_exceeded", Message: "查询超过时间预算。", Status: 429}
	}
	ie := identity.PublicError(e)
	if ie.Status < 500 || ie.Code == "operation_status_unknown" {
		return &display.Error{Code: ie.Code, Message: ie.Message, Status: ie.Status}
	}
	return &display.Error{Code: "dependency_unavailable", Message: "当前依赖暂不可用，请稍后重试。", Status: 503}
}
func reply(c *gin.Context, out any, e error) {
	access.ReportOperationFailure(c.Request.Context(), e)
	if e != nil {
		p := PublicError(e)
		c.AbortWithStatusJSON(p.Status, p)
		return
	}
	c.JSON(200, out)
}
func Register(r *gin.Engine, auth *identity.Service, s *display.Service) {
	for _, policy := range access.Policies() {
		if !strings.HasPrefix(string(policy.Operation), "Display:") {
			continue
		}
		action := strings.TrimPrefix(string(policy.Operation), "Display:")
		r.Handle(policy.Method, policy.Path, func(c *gin.Context) {
			ctx := c.Request.Context()
			p := identityhttp.Principal(c)
			a := auth.AccessActor(p)
			id := c.Param("id")
			scope := display.Scope{Tenant: c.Param("tenant_id"), Project: c.Param("project_id")}
			if action == "locate" {
				out, e := s.Locate(ctx, a, id)
				reply(c, out, e)
				return
			}
			for _, v := range []string{scope.Tenant, scope.Project} {
				if !access.ValidID(v) {
					reply(c, nil, access.ErrProjectNotFound)
					return
				}
			}
			switch action {
			case "list":
				out, e := s.List(ctx, a, scope)
				reply(c, out, e)
				return
			case "detail":
				out, e := s.Detail(ctx, a, scope, id)
				reply(c, out, e)
				return
			case "upload.get":
				out, e := s.Upload(ctx, a, scope, id, c.Param("upload_id"))
				reply(c, out, e)
				return
			case "upload.part":
				part, e := strconv.Atoi(c.Param("part"))
				if e != nil {
					reply(c, nil, &display.Error{Code: "query_invalid", Message: "分片编号无效。", Status: 422})
					return
				}
				b, e := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, display.PartSize))
				if e == nil {
					e = s.UploadPart(ctx, a, scope, id, c.Param("upload_id"), part, c.GetHeader("X-Content-SHA256"), b)
				}
				reply(c, map[string]bool{"received": e == nil}, e)
				return
			}
			var in display.Command
			b, e := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20))
			if e == nil {
				e = display.Decode(b, &in)
			}
			if e != nil {
				reply(c, nil, e)
				return
			}
			switch action {
			case "prepare":
				out, e := s.Prepare(ctx, a, scope, id, in)
				reply(c, out, e)
			case "tool.authorize":
				in.SessionID = p.Session.ID
				out, e := s.AuthorizeTool(ctx, a, scope, id, in)
				reply(c, out, e)
			case "launch", "preview":
				in.SessionID = p.Session.ID
				out, e := s.Launch(ctx, a, scope, id, in, action == "preview")
				reply(c, out, e)
			default:
				target := c.Param("upload_id")
				if target == "" {
					target = c.Param("terminal_id")
				}
				if target == "" {
					target = c.Param("package_id")
				}
				out, e := s.Command(ctx, a, scope, id, action, target, in)
				reply(c, out, e)
			}
		})
	}
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, e error) {
	p := PublicError(e)
	if p.Status == 429 {
		w.Header().Set("Retry-After", "2")
	}
	writeJSON(w, p.Status, p)
}
