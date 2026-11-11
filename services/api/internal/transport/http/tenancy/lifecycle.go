package tenancyhttp

import (
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tenancy"
	identityhttp "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/identity"
	"github.com/gin-gonic/gin"
)

func registerLifecycle(r *gin.Engine, i *identity.Service, s *tenancy.Service) {
	actor := func(c *gin.Context) access.Actor { return i.AccessActor(identityhttp.Principal(c)) }
	r.GET("/api/v1/account/tenants/:tenant_id/availability", query(), func(c *gin.Context) {
		v, e := s.Availability(c.Request.Context(), actor(c), c.Param("tenant_id"))
		reply(c, v, e)
	})
	for _, action := range []string{"disable", "enable"} {
		target := "disabled"
		if action == "enable" {
			target = "active"
		}
		path := "/api/v1/platform/tenants/:tenant_id/" + action
		r.POST(path+"/preview", query(), func(c *gin.Context) {
			in, ok := decode[*struct{}](c)
			if !ok {
				return
			}
			if in == nil {
				reply(c, nil, access.Validation("request", "请提交空 JSON 对象。"))
				return
			}
			v, e := s.PreviewStatus(c.Request.Context(), actor(c), c.Param("tenant_id"), target)
			reply(c, v, e)
		})
		r.POST(path, query(), func(c *gin.Context) {
			// Separate request shapes reject tenant_notice on restore, including explicit null.
			var in tenancy.StatusInput
			if target == "active" {
				body, ok := decode[struct {
					Version string `json:"expected_version"`
					Hash    string `json:"preview_hash"`
					Reason  string `json:"reason"`
				}](c)
				if !ok {
					return
				}
				in = tenancy.StatusInput{Version: body.Version, Hash: body.Hash, Reason: body.Reason}
			} else {
				body, ok := decode[tenancy.StatusInput](c)
				if !ok {
					return
				}
				in = body
			}
			v, e := s.ChangeStatus(c.Request.Context(), actor(c), c.Param("tenant_id"), target, in, access.Meta{ActorType: "account", IP: c.ClientIP(), UserAgent: c.Request.UserAgent(), RequestID: c.Writer.Header().Get("X-Request-ID")})
			if e == nil {
				c.Header("X-Authorization-Revision", v.Revision)
			}
			reply(c, v, e)
		})
	}
}
