package monitoringhttp

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	identityhttp "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/identity"
	"github.com/gin-gonic/gin"
)

func reply(c *gin.Context, status int, value any, err error) {
	if err != nil {
		var e *md.Error
		if errors.As(err, &e) {
			c.AbortWithStatusJSON(e.Status, e)
			return
		}
		identityhttp.RespondError(c, err)
		return
	}
	c.JSON(status, value)
}
func invalid(message string) error {
	return &md.Error{Code: "invalid_request", Message: message, Status: 400}
}
func body(c *gin.Context, out any) error {
	raw, e := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 256*1024))
	if e != nil {
		var large *http.MaxBytesError
		if errors.As(e, &large) {
			return &md.Error{Code: "input_too_large", Message: "单次 JSON 请求最多 256 KiB。", Status: 413}
		}
		return invalid("请求读取失败。")
	}
	return md.DecodeRequest(raw, out)
}

func Register(r *gin.Engine, i *identity.Service, s *md.Service) {
	registerData(r, i, s)
	for _, policy := range access.Policies() {
		op := string(policy.Operation)
		if !strings.HasPrefix(op, "Monitoring:tenant:access.") {
			continue
		}
		action := strings.TrimPrefix(op, "Monitoring:tenant:")
		r.Handle(policy.Method, policy.Path, func(c *gin.Context) {
			c.Request = c.Request.WithContext(md.TraceContext(c.Request.Context(), ""))
			c.Header("X-Monitoring-Trace-ID", md.TraceID(c.Request.Context()))
			defer func() {
				md.LogOperation(c.Request.Context(), action, c.Writer.Status(), int64(c.Writer.Size()), c.Request.Context().Err() != nil)
			}()
			for _, p := range c.Params {
				if !access.ValidID(p.Value) {
					reply(c, 0, nil, &md.Error{Code: "monitoring_not_found", Message: "监测数据不存在或当前不可访问。", Status: 404})
					return
				}
			}
			for key, values := range c.Request.URL.Query() {
				if action != "access.list" || key != "cursor" || len(values) != 1 {
					reply(c, 0, nil, invalid("不支持或重复的查询参数。"))
					return
				}
			}
			a := i.AccessActor(identityhttp.Principal(c))
			ctx := c.Request.Context()
			tenant, project, id := c.Param("tenant_id"), c.Param("project_id"), c.Param("id")
			switch action {
			case "access.list":
				v, e := s.ListDataGrants(ctx, a, tenant, project, c.Query("cursor"))
				reply(c, 200, v, e)
			case "access.grant":
				var in md.GrantInput
				if e := body(c, &in); e != nil {
					reply(c, 0, nil, e)
					return
				}
				v, e := s.GrantData(ctx, a, tenant, project, in)
				reply(c, 200, v, e)
			case "access.preview":
				var in struct {
					Revision string `json:"expected_revision"`
				}
				if e := body(c, &in); e != nil {
					reply(c, 0, nil, e)
					return
				}
				v, e := s.PreviewRevokeData(ctx, a, tenant, project, id)
				if e == nil && v.Grant.Revision != in.Revision {
					e = &md.Error{Code: "revision_changed", Message: "授权已变化，请刷新。", Status: 409}
				}
				reply(c, 200, v, e)
			case "access.revoke":
				var in md.RevokeInput
				if e := body(c, &in); e != nil {
					reply(c, 0, nil, e)
					return
				}
				v, e := s.RevokeData(ctx, a, tenant, project, id, in)
				reply(c, 200, v, e)
			}
		})
	}
}
