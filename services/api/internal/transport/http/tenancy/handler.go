package tenancyhttp

import (
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strconv"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tenancy"
	identityhttp "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/identity"
	"github.com/gin-gonic/gin"
)

func decode[T any](c *gin.Context) (T, bool) {
	var v T
	d := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 32*1024))
	d.DisallowUnknownFields()
	var rest any
	if d.Decode(&v) != nil || d.Decode(&rest) != io.EOF {
		identityhttp.RespondError(c, access.Validation("request", "请求内容无效或包含不支持的字段。"))
		return v, false
	}
	return v, true
}
func reply(c *gin.Context, v any, e error) {
	if e != nil {
		identityhttp.RespondError(c, e)
		return
	}
	if out, ok := v.(tenancy.Result); ok {
		c.Header("X-Authorization-Revision", out.Revision)
	}
	if out, ok := v.(tenancy.Context); ok {
		c.Header("X-Authorization-Revision", out.Revision)
	}
	c.JSON(200, v)
}
func filter(c *gin.Context) tenancy.Filter {
	n := 0
	if c.Query("limit") != "" {
		var e error
		n, e = strconv.Atoi(c.Query("limit"))
		if e != nil {
			n = -1
		}
	}
	return tenancy.Filter{Query: c.Query("q"), Cursor: c.Query("cursor"), Limit: n, Unavailable: c.Query("administrator_unavailable") == "true", Event: c.Query("event_type"), From: c.Query("from"), To: c.Query("to")}
}
func query(allowed ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		for key := range c.Request.URL.Query() {
			if !slices.Contains(allowed, key) {
				identityhttp.RespondError(c, access.Validation(key, "不支持的查询字段。"))
				return
			}
		}
		c.Next()
	}
}
func Register(r *gin.Engine, i *identity.Service, s *tenancy.Service) {
	actor := func(c *gin.Context) access.Actor { return i.AccessActor(identityhttp.Principal(c)) }
	meta := func(c *gin.Context) access.Meta {
		return access.Meta{ActorType: "account", IP: c.ClientIP(), UserAgent: c.Request.UserAgent(), RequestID: c.Writer.Header().Get("X-Request-ID")}
	}
	r.GET("/api/v1/platform/tenants", query("q", "cursor", "limit", "administrator_unavailable"), func(c *gin.Context) {
		if v := c.Query("administrator_unavailable"); v != "" && v != "true" && v != "false" {
			identityhttp.RespondError(c, access.Validation("administrator_unavailable", "筛选值无效。"))
			return
		}
		v, e := s.List(c.Request.Context(), actor(c), filter(c))
		reply(c, v, e)
	})
	r.POST("/api/v1/platform/tenants", query(), func(c *gin.Context) {
		in, ok := decode[tenancy.CreateInput](c)
		if !ok {
			return
		}
		v, e := s.Create(c.Request.Context(), actor(c), in, meta(c))
		if e != nil {
			reply(c, v, e)
			return
		}
		status := 201
		if v.Replayed {
			status = 200
		}
		c.Header("X-Authorization-Revision", v.Revision)
		c.JSON(status, v)
	})
	r.GET("/api/v1/platform/tenant-admin-candidates", query("q", "cursor", "limit"), func(c *gin.Context) { v, e := s.Candidates(c.Request.Context(), actor(c), filter(c)); reply(c, v, e) })
	r.GET("/api/v1/account/tenants", query("q", "cursor", "limit"), func(c *gin.Context) {
		v, e := s.Memberships(c.Request.Context(), actor(c), "", filter(c))
		reply(c, v, e)
	})
	r.GET("/api/v1/access/accounts/:id/tenant-memberships", query("q", "cursor", "limit"), func(c *gin.Context) {
		v, e := s.Memberships(c.Request.Context(), actor(c), c.Param("id"), filter(c))
		reply(c, v, e)
	})
	r.GET("/api/v1/tenants/:tenant_id/context", query(), func(c *gin.Context) {
		v, e := s.Context(c.Request.Context(), actor(c), c.Param("tenant_id"))
		reply(c, v, e)
	})
	for _, platform := range []bool{true, false} {
		path := "/api/v1/tenants/:tenant_id/profile"
		if platform {
			path = "/api/v1/platform/tenants/:tenant_id"
		}
		r.GET(path, query(), func(c *gin.Context) {
			v, e := s.Get(c.Request.Context(), actor(c), c.Param("tenant_id"), platform)
			reply(c, v, e)
		})
		r.PATCH(path, query(), func(c *gin.Context) {
			in, ok := decode[tenancy.UpdateInput](c)
			if !ok {
				return
			}
			v, e := s.Update(c.Request.Context(), actor(c), c.Param("tenant_id"), in, platform, meta(c))
			reply(c, v, e)
		})
	}
	r.POST("/api/v1/platform/tenants/:tenant_id/administrator-transfer", query(), func(c *gin.Context) {
		in, ok := decode[tenancy.TransferInput](c)
		if !ok {
			return
		}
		v, e := s.Transfer(c.Request.Context(), actor(c), c.Param("tenant_id"), in, meta(c))
		reply(c, v, e)
	})
	r.GET("/api/v1/tenants/:tenant_id/audit-events", query("q", "cursor", "limit", "event_type", "from", "to"), func(c *gin.Context) {
		v, e := s.Audit(c.Request.Context(), actor(c), c.Param("tenant_id"), filter(c))
		reply(c, v, e)
	})
}
