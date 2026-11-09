package accesshttp

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	identityhttp "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/identity"
	"github.com/gin-gonic/gin"
)

func decode[T any](c *gin.Context) (T, bool) {
	var v T
	d := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 32*1024))
	d.DisallowUnknownFields()
	if d.Decode(&v) != nil {
		identityhttp.RespondError(c, access.Validation("request", "请求内容无效或包含不支持的字段。"))
		return v, false
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		identityhttp.RespondError(c, access.Validation("request", "请求只能包含一个对象。"))
		return v, false
	}
	return v, true
}
func reply(c *gin.Context, v any, e error) {
	if e != nil {
		identityhttp.RespondError(c, e)
		return
	}
	if v == nil {
		c.Status(204)
	} else {
		c.JSON(200, v)
	}
}
func meta(c *gin.Context, reason string) access.Meta {
	return access.Meta{Reason: reason, ActorType: "account", IP: c.ClientIP(), UserAgent: c.Request.UserAgent(), RequestID: c.Writer.Header().Get("X-Request-ID")}
}

type mutation struct {
	Reason  string `json:"reason"`
	Version string `json:"expected_version"`
}

func filter(c *gin.Context) access.Filter {
	limit := 0
	if c.Query("limit") != "" {
		n, e := strconv.Atoi(c.Query("limit"))
		if e != nil {
			limit = -1
		} else {
			limit = n
		}
	}
	return access.Filter{Cursor: c.Query("cursor"), Query: c.Query("q"), Status: c.Query("status"), RoleID: c.Query("role_id"), ObjectID: c.Query("object_id"), ObjectType: c.Query("object_type"), From: c.Query("from"), To: c.Query("to"), Limit: limit}
}
func Register(r *gin.Engine, s *identity.Service) {
	a := s.Access()
	g := r.Group("/api/v1/access")
	actor := func(c *gin.Context) access.Actor { return s.AccessActor(identityhttp.Principal(c)) }
	g.GET("/permissions", func(c *gin.Context) { v, e := a.Permissions(c.Request.Context(), actor(c)); reply(c, v, e) })
	g.GET("/roles", func(c *gin.Context) { v, e := a.Roles(c.Request.Context(), actor(c), filter(c)); reply(c, v, e) })
	g.GET("/roles/:id", func(c *gin.Context) { v, e := a.Role(c.Request.Context(), actor(c), c.Param("id")); reply(c, v, e) })
	g.POST("/roles", func(c *gin.Context) {
		v, ok := decode[struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Reason      string `json:"reason"`
		}](c)
		if !ok {
			return
		}
		out, e := a.CreateRole(c.Request.Context(), actor(c), v.Name, v.Description, meta(c, v.Reason))
		reply(c, out, e)
	})
	g.PATCH("/roles/:id", func(c *gin.Context) {
		v, ok := decode[struct {
			mutation
			Name        string `json:"name"`
			Description string `json:"description"`
		}](c)
		if !ok {
			return
		}
		out, e := a.ChangeRole(c.Request.Context(), actor(c), c.Param("id"), access.OpRoleUpdate, access.RoleChange{ExpectedVersion: v.Version, Name: v.Name, Description: v.Description}, meta(c, v.Reason))
		reply(c, out, e)
	})
	g.PUT("/roles/:id/permissions", func(c *gin.Context) {
		v, ok := decode[struct {
			mutation
			Codes []access.Code `json:"permission_codes"`
		}](c)
		if !ok {
			return
		}
		out, e := a.ChangeRole(c.Request.Context(), actor(c), c.Param("id"), access.OpRoleSetPermissions, access.RoleChange{ExpectedVersion: v.Version, Permissions: v.Codes}, meta(c, v.Reason))
		reply(c, out, e)
	})
	g.PUT("/roles/:id/status", func(c *gin.Context) {
		v, ok := decode[struct {
			mutation
			Status string `json:"status"`
		}](c)
		if !ok {
			return
		}
		out, e := a.ChangeRole(c.Request.Context(), actor(c), c.Param("id"), access.OpRoleSetStatus, access.RoleChange{ExpectedVersion: v.Version, Status: v.Status}, meta(c, v.Reason))
		reply(c, out, e)
	})
	g.POST("/roles/:id/archive", func(c *gin.Context) {
		v, ok := decode[mutation](c)
		if !ok {
			return
		}
		_, e := a.ChangeRole(c.Request.Context(), actor(c), c.Param("id"), access.OpRoleDelete, access.RoleChange{ExpectedVersion: v.Version}, meta(c, v.Reason))
		reply(c, nil, e)
	})
	g.GET("/accounts", func(c *gin.Context) { v, e := a.Accounts(c.Request.Context(), actor(c), filter(c)); reply(c, v, e) })
	g.GET("/accounts/:id", func(c *gin.Context) { v, e := a.Account(c.Request.Context(), actor(c), c.Param("id")); reply(c, v, e) })
	g.GET("/accounts/:id/authorization", func(c *gin.Context) {
		v, e := a.Authorization(c.Request.Context(), actor(c), c.Param("id"))
		reply(c, v, e)
	})
	g.PUT("/accounts/:id/roles", func(c *gin.Context) {
		v, ok := decode[struct {
			mutation
			IDs []string `json:"role_ids"`
		}](c)
		if !ok {
			return
		}
		out, e := a.AssignRoles(c.Request.Context(), actor(c), c.Param("id"), v.Version, v.IDs, meta(c, v.Reason))
		reply(c, out, e)
	})
	for _, enabled := range []bool{false, true} {
		path := "/accounts/:id/disable"
		if enabled {
			path = "/accounts/:id/enable"
		}
		g.POST(path, func(c *gin.Context) {
			v, ok := decode[mutation](c)
			if !ok {
				return
			}
			out, e := s.SetAccountStatus(c.Request.Context(), identityhttp.Principal(c), c.Param("id"), v.Version, enabled, meta(c, v.Reason))
			reply(c, out, e)
		})
	}
	g.GET("/audit-events", func(c *gin.Context) { v, e := a.AuditEvents(c.Request.Context(), actor(c), filter(c)); reply(c, v, e) })
}
