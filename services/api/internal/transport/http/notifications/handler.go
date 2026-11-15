package notificationshttp

import (
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strconv"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/notifications"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tenancy"
	identityhttp "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/identity"
	"github.com/gin-gonic/gin"
)

func decode[T any](c *gin.Context) (T, bool) {
	var v T
	d := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 16*1024))
	d.DisallowUnknownFields()
	var rest any
	if d.Decode(&v) != nil || d.Decode(&rest) != io.EOF {
		identityhttp.RespondError(c, access.Validation("request", "通知请求无效或包含不支持的字段。"))
		return v, false
	}
	return v, true
}
func reply(c *gin.Context, v any, e error) {
	if e != nil {
		identityhttp.RespondError(c, e)
		return
	}
	switch out := v.(type) {
	case notifications.Page:
		c.Header("X-Authorization-Revision", out.AuthorizationRevision)
	case notifications.Summary:
		c.Header("X-Authorization-Revision", out.AuthorizationRevision)
	case tenancy.AcceptanceResult:
		c.Header("X-Authorization-Revision", out.Revision)
	}
	c.JSON(200, v)
}
func query(allowed ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-store")
		for k := range c.Request.URL.Query() {
			if !slices.Contains(allowed, k) || len(c.Request.URL.Query()[k]) != 1 {
				identityhttp.RespondError(c, access.Validation("query", "不支持的通知查询参数。"))
				return
			}
		}
		c.Next()
	}
}
func filter(c *gin.Context) notifications.Filter {
	limit := 0
	if c.Query("limit") != "" {
		var e error
		limit, e = strconv.Atoi(c.Query("limit"))
		if e != nil {
			limit = -1
		}
	}
	return notifications.Filter{Status: c.Query("status"), Category: c.Query("category"), Tenant: c.Query("tenant_id"), Project: c.Query("project_id"), Scope: c.Query("scope"), Cursor: c.Query("cursor"), Limit: limit}
}
func Register(r *gin.Engine, i *identity.Service, s *notifications.Service, t *tenancy.Service) {
	actor := func(c *gin.Context) access.Actor { return i.AccessActor(identityhttp.Principal(c)) }
	meta := func(c *gin.Context) access.Meta {
		return access.Meta{ActorType: "account", RequestID: c.Writer.Header().Get("X-Request-ID")}
	}
	r.GET("/api/v1/me/notifications", query("status", "category", "tenant_id", "project_id", "scope", "cursor", "limit"), func(c *gin.Context) { v, e := s.List(c.Request.Context(), actor(c), filter(c)); reply(c, v, e) })
	r.GET("/api/v1/me/notifications/summary", query(), func(c *gin.Context) { v, e := s.Summary(c.Request.Context(), actor(c)); reply(c, v, e) })
	r.GET("/api/v1/me/notifications/:id", query(), func(c *gin.Context) { v, e := s.Get(c.Request.Context(), actor(c), c.Param("id")); reply(c, v, e) })
	r.PATCH("/api/v1/me/notifications/:id", query(), func(c *gin.Context) {
		in, ok := decode[notifications.Change](c)
		if !ok {
			return
		}
		v, e := s.Change(c.Request.Context(), actor(c), c.Param("id"), in)
		reply(c, v, e)
	})
	r.POST("/api/v1/me/notifications/read-all", query(), func(c *gin.Context) {
		in, ok := decode[notifications.ReadAllInput](c)
		if !ok {
			return
		}
		v, e := s.ReadAll(c.Request.Context(), actor(c), in)
		reply(c, gin.H{"changed": v}, e)
	})
	r.GET("/api/v1/me/notifications/:id/target", query(), func(c *gin.Context) { v, e := s.Target(c.Request.Context(), actor(c), c.Param("id")); reply(c, v, e) })
	r.GET("/api/v1/me/notification-preferences", query(), func(c *gin.Context) { v, e := s.Preferences(c.Request.Context(), actor(c)); reply(c, v, e) })
	r.PUT("/api/v1/me/notification-preferences", query(), func(c *gin.Context) {
		in, ok := decode[notifications.Preferences](c)
		if !ok {
			return
		}
		v, e := s.SavePreferences(c.Request.Context(), actor(c), in)
		reply(c, v, e)
	})
	r.GET("/api/v1/me/tenant-invitations", query("cursor", "limit"), func(c *gin.Context) {
		f := filter(c)
		v, e := t.PersonalInvitations(c.Request.Context(), actor(c), tenancy.Filter{Cursor: f.Cursor, Limit: f.Limit})
		reply(c, v, e)
	})
	r.GET("/api/v1/me/tenant-invitations/:id", query(), func(c *gin.Context) {
		v, e := t.PersonalInvitation(c.Request.Context(), actor(c), c.Param("id"), meta(c))
		reply(c, v, e)
	})
	r.POST("/api/v1/me/tenant-invitations/:id/accept", query(), func(c *gin.Context) {
		in, ok := decode[struct {
			Version string `json:"expected_version"`
		}](c)
		if !ok {
			return
		}
		v, e := t.AcceptPersonalInvitation(c.Request.Context(), actor(c), c.Param("id"), in.Version, meta(c))
		reply(c, v, e)
	})
}
